package workspace

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The births ledger (`<workspaceDir>/attic/births/<member>.jsonl`): every time the harness births a
// agent — a zone-parent's provisioning passing its gate, a relief's young, a promotion, an evolution's
// split or rebirth — one line lands in that agent's own file, never rewritten. The doc says who a
// agent is; the ledger says when it came to be, which is what tenure reads. The attic is harness
// soil: no agent writes it, and the tree watch never bills it.

// birthRecord is one provisioning fact.
type birthRecord struct {
	At    time.Time `json:"at"`
	Agent string    `json:"agent"`
	Kind  string    `json:"kind"`           // provisioning | split | promote | evolve-split | rebirth
	From  string    `json:"from,omitempty"` // the agent it came from (the retiree, the base, the parent)
	By    string    `json:"by,omitempty"`   // who birthed it: the writer of a provisioning, else the harness's act
	Doc   string    `json:"doc,omitempty"`
}

// BirthLedgerPath is one agent's births file.
func (w *Workspace) BirthLedgerPath(agent string) string {
	return filepath.Join(w.Dir(), "attic", "births", strings.ToLower(agent)+".jsonl")
}

// logBirth appends one provisioning; a failure is a note (the provisioning itself stands on its doc).
func (w *Workspace) logBirth(r birthRecord) {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	b, err := json.Marshal(r)
	if err == nil {
		p := w.BirthLedgerPath(r.Agent)
		if err = os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			var f *os.File
			if f, err = os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				_, err = f.Write(append(b, '\n'))
				f.Close()
			}
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		w.Notes = append(w.Notes, "births ledger: "+r.Agent+": "+err.Error())
		return
	}
	if t, ok := w.born[r.Agent]; !ok || r.At.Before(t) || t.IsZero() {
		w.born[r.Agent] = r.At
	}
}

// Births is an agent's provisioning facts, oldest first (a torn line is skipped).
func (w *Workspace) Births(agent string) []birthRecord {
	f, err := os.Open(w.BirthLedgerPath(agent))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []birthRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r birthRecord
		if json.Unmarshal(sc.Bytes(), &r) == nil && !r.At.IsZero() {
			out = append(out, r)
		}
	}
	return out
}

// bornAt is when an agent was first provisioned: its first ledger line, else — an agent provisioned before the
// ledger is grandparented — the first commit that added its doc. ok is false when neither speaks.
func (w *Workspace) bornAt(agent string) (time.Time, bool) {
	w.mu.Lock()
	t, known := w.born[agent]
	w.mu.Unlock()
	if known {
		return t, !t.IsZero()
	}
	t = time.Time{}
	if bs := w.Births(agent); len(bs) > 0 {
		t = bs[0].At
		for _, b := range bs[1:] {
			if b.At.Before(t) {
				t = b.At
			}
		}
	} else if doc := w.docOf(agent); doc != "" {
		if out, err := w.gitOut("log --diff-filter=A --format=%at -- " + doc); err == nil {
			if ls := strings.Fields(out); len(ls) > 0 {
				if sec, err := strconv.ParseInt(ls[len(ls)-1], 10, 64); err == nil {
					t = time.Unix(sec, 0).UTC()
				}
			}
		}
	}
	w.mu.Lock()
	w.born[agent] = t
	w.mu.Unlock()
	return t, !t.IsZero()
}
