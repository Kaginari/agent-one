package workspace

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Kaginari/agent-one/queue"
	"github.com/Kaginari/agent-one/value"
)

// The shared shelf (`<workspaceDir>/shared/`): the workspace's own bookkeeping that every session,
// every agent and the human read — the work queue, the council's ledgers, the value ledger.
// It is harness soil: the harness writes it (the queue's moves go through the queue tool or
// the q command, never an agent's write), so the ownership gate refuses an agent's write there
// and the tree watch never bills a change there to the agent whose turn it fell in
// (loop/snapshot.go).
const sharedShelf = "shared"

// movedShelves are the files that lived under instruments/ before the shared shelf, by their
// path on it. Open moves each one over once; a workspace where the old file came back (an older
// binary appended after the move) is read from both, the old file first.
var movedShelves = map[string]string{
	ledgerShelf: "instruments/ledgers.jsonl",
	value.Shelf: "instruments/value.jsonl",
}

// migrateShelf moves every old shelf file onto the shared shelf — a rename, so no line is
// rewritten (Policy 4) — when the shared one does not exist yet. A move that fails is a note, and
// the reads still find the old file.
func (w *Workspace) migrateShelf() {
	for rel, old := range movedShelves {
		from, to := filepath.Join(w.Dir(), filepath.FromSlash(old)), filepath.Join(w.Dir(), filepath.FromSlash(rel))
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if _, err := os.Stat(to); !errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := os.MkdirAll(filepath.Dir(to), 0o755)
		if err == nil {
			err = os.Rename(from, to)
		}
		if err != nil {
			w.Notes = append(w.Notes, "shared shelf: "+old+" stays where it was: "+err.Error())
		}
	}
}

// shelfReadPaths are the files a shelf is read from, oldest lines first: the old location
// when it still holds a file, then the shared one. Writes go to the shared one only.
func (w *Workspace) shelfReadPaths(rel string) []string {
	var out []string
	for _, p := range []string{movedShelves[rel], rel} {
		if p == "" {
			continue
		}
		if abs := filepath.Join(w.Dir(), filepath.FromSlash(p)); fileExists(abs) {
			out = append(out, abs)
		}
	}
	return out
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// SharedDir is the shared shelf's directory.
func (w *Workspace) SharedDir() string { return filepath.Join(w.Dir(), sharedShelf) }

// QueuePath is the shared work queue's file.
func (w *Workspace) QueuePath() string { return filepath.Join(w.SharedDir(), "queue.jsonl") }

// Queue opens the shared work queue under the rank policy: an agent moves the work it holds, and
// any member up the holder's Reports-to chain (orchestrator tops every chain) may move it too.
func (w *Workspace) Queue() *queue.Store { return queue.Open(w.QueuePath(), w.Outranks) }

// Outranks reports whether `by` stands above `owner` in the Reports-to chain. Work held by no
// one is the orchestrator's to move.
func (w *Workspace) Outranks(by, owner string) bool {
	if by == Orchestrator {
		return true
	}
	if owner == "" || by == "" {
		return false
	}
	for _, up := range w.lineOf(owner) {
		if up == by {
			return true
		}
	}
	return false
}
