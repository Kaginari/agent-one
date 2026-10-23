package workspace

import (
	"path/filepath"

	"github.com/Kaginari/agent-one/queue"
)

// The shared shelf (`<workspaceDir>/shared/`): the workspace's own bookkeeping that every session,
// every agent and the human read — the work queue first. It is harness soil: the harness
// writes it (the queue's moves go through the queue tool or the q command, never an agent's
// write), so the ownership gate refuses an agent's write there and the tree watch never bills
// a change there to the agent whose turn it fell in (loop/snapshot.go).
const sharedShelf = "shared"

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
