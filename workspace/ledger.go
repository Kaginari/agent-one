package workspace

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// The ledger shelf (`<workspaceDir>/shared/ledgers.jsonl`): the council's strain, its
// consult counts, its stress-death memos and the subagent-write attribution notebook used to
// live process-local and die with it — a workspace that forgot its scars on every restart
// relearned nothing (freeze review 2026-10-07). Every mutation appends one JSON row;
// Open replays the shelf (last row wins per key) so rests survive restarts. An unreadable
// shelf never blocks a boot: the workspace simply opens ledgerless, exactly like before.

const ledgerShelf = sharedShelf + "/ledgers.jsonl"

type ledgerRow struct {
	Kind   string    `json:"kind"` // stress | consults | stressClear | death | subagentWrite | relief (a council relief: the zone split or retired — Lane names the act, Path the young)
	Agent  string    `json:"agent"`
	Value  int       `json:"value,omitempty"`
	Lane   string    `json:"lane,omitempty"`
	Path   string    `json:"path,omitempty"`
	Writer string    `json:"writer,omitempty"`
	At     time.Time `json:"at"`
}

// ledgerNote appends one row. Call sites write under the workspace lock, so the file's order is
// the memory's order — the replay stands on that.
func (w *Workspace) ledgerNote(r ledgerRow) {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	p := filepath.Join(w.Dir(), filepath.FromSlash(ledgerShelf))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// loadLedgers replays the shelf into the in-memory ledgers. Subagent-write rows replay through
// the same per-member bound as runtime writes; the clear row empties both strain ledgers,
// exactly like the run-time clean-landing policy.
func (w *Workspace) loadLedgers() {
	var homes [][]ledgerRow
	for _, p := range w.shelfReadPaths(ledgerShelf) {
		homes = append(homes, readLedger(p))
	}
	for _, r := range mergeHomes(homes, func(r ledgerRow) time.Time { return r.At }, nil) {
		w.replayLedger(r)
	}
}

// readLedger is one ledger file's rows, in file order; an unreadable file has none.
func readLedger(path string) []ledgerRow {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []ledgerRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r ledgerRow
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Agent != "" {
			out = append(out, r)
		}
	}
	return out
}

func (w *Workspace) replayLedger(r ledgerRow) {
	switch r.Kind {
	case "stress":
		w.stress[r.Agent] = r.Value
	case "consults":
		w.consults[r.Agent] = r.Value
	case "stressClear":
		delete(w.stress, r.Agent)
		delete(w.consults, r.Agent)
	case "death":
		w.stressDeaths[r.Agent] = stressEnd{Lane: r.Lane, At: r.At}
	case "subagentWrite":
		if r.Path == "" {
			return
		}
		bm := w.subagentWrites[r.Agent]
		if bm == nil {
			bm = map[string]subagentWrite{}
			w.subagentWrites[r.Agent] = bm
		}
		bm[normRel(r.Path)] = subagentWrite{Writer: or(r.Writer, r.Agent), At: r.At}
		for len(bm) > subagentWritesBound {
			oldest := ""
			for k, cw := range bm {
				if oldest == "" || cw.At.Before(bm[oldest].At) {
					oldest = k
				}
			}
			delete(bm, oldest)
		}
	}
}
