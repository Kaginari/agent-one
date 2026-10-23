package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"time"

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
// binary appended after the move) is read from both, their rows merged by time (mergeHomes).
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

// shelfReadPaths are the files a shelf is read from: the old location when it still holds a
// file, then the shared one. Writes go to the shared one only.
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

// mergeHomes is one shelf's rows read from each of its homes (in shelfReadPaths order),
// merged by their time: each file keeps its own order and the next row taken is the earliest
// of the homes' next rows (the older home first on a tie; a row with no time takes the time of
// the row before it in its file). A seed (seed true) carries its file's rows before it over from
// the legacy tally, so it counts once: the earliest seed's block stands, and any later seed
// with the rows it carried — an older binary that found its ledger missing after the move — is
// dropped; the rows after it stay.
func mergeHomes[T any](homes [][]T, at func(T) time.Time, seed func(T) bool) []T {
	first, firstAt := -1, time.Time{}
	from := make([]int, len(homes)) // per home, the first row read: past a dropped seed block
	for h, rows := range homes {
		for i, r := range rows {
			if seed != nil && seed(r) {
				if first < 0 || at(r).Before(firstAt) {
					first, firstAt = h, at(r)
				}
				from[h] = i + 1
				break
			}
		}
	}
	if first >= 0 {
		from[first] = 0
	}
	last := make([]time.Time, len(homes))
	when := func(h int) time.Time {
		if t := at(homes[h][from[h]]); !t.IsZero() {
			return t
		}
		return last[h]
	}
	var out []T
	for {
		next := -1
		for h := range homes {
			if from[h] < len(homes[h]) && (next < 0 || when(h).Before(when(next))) {
				next = h
			}
		}
		if next < 0 {
			return out
		}
		last[next] = when(next)
		out = append(out, homes[next][from[next]])
		from[next]++
	}
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
