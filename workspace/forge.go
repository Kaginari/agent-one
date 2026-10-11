package workspace

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/wire"
)

// The forge ledger: drafts (`@FORGE kind:name | text`) are filed by the harness from the
// report it holds — the dispatcher's own loop.Result, never a relayed digest — as shared notes
// tagged "forge", each carrying its run and its content hash. The working notes's marks ride the same
// ledger, append-only: "forge-registered" and "forge-rejected". Only the harness writes these
// tags; the remember tool refuses them.

// ForgeTags are the tags only the harness may write.
var ForgeTags = []string{"forge", "forge-registered", "forge-rejected", "skill"}

// ForgeNote is one ledger line as filed.
type ForgeNote struct {
	At   string `json:"at"`
	By   string `json:"by"`
	Tag  string `json:"tag"`
	Text string `json:"text"`
	Run  string `json:"run"`
	Hash string `json:"hash"`
}

// forgeMu serialises the check-then-append of a filing: two subagents finishing at once must not
// both file the same draft.
var forgeMu sync.Mutex

// ForgeLedgerPath is the shared notes file the ledger rides.
func ForgeLedgerPath(root, workspaceDir string) string {
	return filepath.Join(root, workspaceDir, "memory", "shared", "notes.jsonl")
}

// ReadForgeLedger reads every forge-tagged line (drafts and marks) in file order.
func ReadForgeLedger(path string) []ForgeNote {
	var out []ForgeNote
	for _, raw := range memory.ReadJSONL(path) {
		var n ForgeNote
		if json.Unmarshal(raw, &n) != nil {
			continue
		}
		switch n.Tag {
		case "forge", "forge-registered", "forge-rejected":
			if n.Tag == "forge" && n.Hash == "" {
				if f, ok := wire.ParseForge(n.Text); ok {
					n.Hash = f.Hash() // a draft filed before the hash rode the note
				}
			}
			out = append(out, n)
		}
	}
	return out
}

// FileForges files a run's drafts as forge notes, by the agent that drafted them. A draft whose
// hash the ledger already holds is not filed again: the first filer wins, so a line relayed
// up through a domain owner and a coordinator lands once, as the zone worker's.
func (w *Workspace) FileForges(by, run string, forges []wire.Forge) []string {
	if len(forges) == 0 {
		return nil
	}
	forgeMu.Lock()
	defer forgeMu.Unlock()
	seen := map[string]bool{}
	for _, n := range ReadForgeLedger(ForgeLedgerPath(w.Root, w.Lex.WorkspaceDir)) {
		if n.Tag == "forge" {
			seen[n.Hash] = true
		}
	}
	m, err := w.Memory()
	if err != nil {
		return []string{"forge note: " + err.Error()}
	}
	var holes []string
	for _, f := range forges {
		h := f.Hash()
		if seen[h] {
			continue
		}
		seen[h] = true
		extra := memory.OJ{{K: "run", V: run}, {K: "hash", V: h}}
		if _, err := m.Remember(f.Line(), memory.RememberOpts{As: by, Kind: "team", Tag: "forge", Extra: extra}); err != nil {
			holes = append(holes, "forge note: "+err.Error())
		}
	}
	return holes
}

// forgesOfResult is a run's drafts: the wire report's when the agent spoke on the wire, else the
// @FORGE lines of its prose answer.
func forgesOfResult(r *loop.Result) []wire.Forge {
	if r == nil {
		return nil
	}
	if len(r.Report.Forges) > 0 {
		return r.Report.Forges
	}
	if !r.IsWire && strings.Contains(r.Text, "@FORGE") {
		return wire.ForgesOf(r.Text)
	}
	return nil
}

// FileForgeMark appends the working notes's mark on a draft: "forge-registered" or "forge-rejected", with
// the draft's hash. Append-only — a mark is never struck, a draft never deleted.
func (w *Workspace) FileForgeMark(by, tag, text, hash string) error {
	if tag != "forge-registered" && tag != "forge-rejected" {
		return fmt.Errorf("forge mark: %q is not forge-registered or forge-rejected", tag)
	}
	m, err := w.Memory()
	if err != nil {
		return err
	}
	_, err = m.Remember(text, memory.RememberOpts{As: by, Kind: "team", Tag: tag, Extra: memory.OJ{{K: "hash", V: hash}}})
	return err
}
