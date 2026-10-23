package loop

import (
	"encoding/json"
	"fmt"
	"hash/fnv"

	"github.com/Kaginari/agent-one/tool"
)

// The stall guards are the guts' two hard stops (policy.guts.repin, policy.guts.actBudget). The gut
// nudges a run that slides; these end one that has stopped moving — in a CHECKPOINT, the
// resumable stop, never a FAIL: the work so far stays and the working notes carries it on.
//
//   - re-pin: the same read (tool + canonical input) answered the same (its output's hash, or
//     the tool's own word that it withheld an unchanged answer — tool.Result.Unchanged) with
//     nothing landed since. The second runs with a note naming the first; at the limit-th the
//     turn checkpoints. The answer is the witness, not the session's own writes: a file another
//     subagent or the human changed reads anew, and its pin starts over.
//   - act budget: model turns since the last landing. A recon loop reads forever; past the budget
//     the turn checkpoints, warned once at three quarters.
//
// Both count spin, not length: a landing and a new ask reset them.

// repinTools are the reads whose answer only a write can move. The shell is not one — a re-run
// of `go test` or a poll of a log is the gut's echo and thrash to price, not a pin.
var repinTools = map[string]bool{"read": true, "glob": true, "grep": true, "recall": true, "onto": true}

// bookkeeping tools write the agent's own working memory (or, for the queue, its note on the
// shared work list): a landing for the tree, not for the work — a recon loop that keeps its
// working notes tidy, or touches its queue item every step, is still a recon loop.
var bookkeeping = map[string]bool{"workingNotes": true, "plan": true, "queue": true}

// stall is one turn's spin ledger.
type stall struct {
	pins   map[uint64]*pin
	idle   int  // model turns since the last landing
	warned bool // the act budget's three-quarter warning was said
}

type pin struct {
	first  string // the step that first asked it
	answer uint64 // its output's hash
	n      int
}

// repin defaults to 3 and actBudget to 40; a negative value turns the guard off.
func (i *Guts) repin() int     { return knob(i, func(g *Guts) int { return g.Repin }, 3) }
func (i *Guts) actBudget() int { return knob(i, func(g *Guts) int { return g.ActBudget }, 40) }

func knob(i *Guts, f func(*Guts) int, def int) int {
	switch {
	case i == nil || f(i) < 0:
		return -1
	case f(i) > 0:
		return f(i)
	}
	return def
}

// signature is a read's identity: the tool and its input with key order and spacing folded out,
// so `{"path":"a"}` and `{ "path": "a" }` are one read.
func signature(name string, in json.RawMessage) uint64 {
	h := fnv.New64a()
	h.Write([]byte(name))
	h.Write([]byte{0})
	var v interface{}
	if json.Unmarshal(in, &v) == nil {
		if b, err := json.Marshal(v); err == nil { // maps marshal with sorted keys
			in = b
		}
	}
	h.Write(in)
	return h.Sum64()
}

// pinned reads one landed read against the re-pin guard: a note for the model on a repeat, or why
// the turn checkpoints at the limit.
func (s *Session) pinned(name string, class tool.Class, in json.RawMessage, res tool.Result, id string) (note, why string) {
	limit := s.Engine.Guts.repin()
	if limit <= 0 || class != tool.Read || !repinTools[name] {
		return "", ""
	}
	if s.stall.pins == nil {
		s.stall.pins = map[uint64]*pin{}
	}
	sig, answer := signature(name, in), hashOf(res.Output)
	p := s.stall.pins[sig]
	if p != nil && res.Unchanged { // withheld as already served: the same answer, said shorter
		p.answer = answer
	}
	if p == nil || p.answer != answer {
		s.stall.pins[sig] = &pin{first: id, answer: answer, n: 1}
		return "", ""
	}
	p.n++
	if p.n >= limit {
		return "", fmt.Sprintf("re-pin loop: %s read the same %s as %s, the same answer %d times with nothing landed between. Checkpoint: pin the finding on the working notes, then resume: agent-one resume %s", id, name, p.first, p.n, s.RunID)
	}
	return fmt.Sprintf("\n[re-pin] the same %s as %s, the same answer, nothing landed since. Pin it on the working notes; read it %d more time%s unchanged and the turn checkpoints.", name, p.first, limit-p.n, plural(limit-p.n)), ""
}

func hashOf(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// landed is a successful act of class write or above that is not bookkeeping: new evidence, so
// every pin is released and the act budget starts over.
func (s *Session) landed(name string, class tool.Class) {
	if class < tool.Write || bookkeeping[name] {
		return
	}
	s.stall = stall{}
}

// spin counts one model turn that asked for tools, before its steps run — a landing among them
// sets it back to zero.
func (s *Session) spin() {
	if s.Engine.Guts.actBudget() > 0 {
		s.stall.idle++
	}
}

// idleWarning is the note the turn's results carry at three quarters of the act budget, once.
func (s *Session) idleWarning() string {
	budget := s.Engine.Guts.actBudget()
	if warnAt := budget * 3 / 4; budget <= 0 || s.stall.warned || warnAt == 0 || s.stall.idle < warnAt {
		return ""
	}
	s.stall.warned = true
	return fmt.Sprintf("[act budget] %d turns with nothing landed — %d left before this turn checkpoints. Land what you know (a write, a dispatch) or close with your findings.", s.stall.idle, budget-s.stall.idle)
}

// budgetSpent is why the turn stops before the next model call, or "".
func (s *Session) budgetSpent() string {
	if budget := s.Engine.Guts.actBudget(); budget > 0 && s.stall.idle >= budget {
		return fmt.Sprintf("act budget: %d turns with no write or dispatch landed — a recon loop. Checkpoint: pin the findings on the working notes, then land or close; resume: agent-one resume %s", s.stall.idle, s.RunID)
	}
	return ""
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
