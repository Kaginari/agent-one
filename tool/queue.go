package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/queue"
)

// QueueTool is `queue`: an agent's door to the workspace's shared work queue. Reading it (list,
// show) is free — class read, never gated, never a landing. Moving work (touch, requeue,
// close) is a write the harness makes as the acting agent, and only on work that agent holds
// or that its rank stands over (the store's May): the line lands on the shared shelf, which
// no agent writes by hand, so the move names no path for the ownership gate to bill. Claims
// are the wake's: an agent never claims here — work reaches it already claimed in its name.
func QueueTool(s *queue.Store, as string) *Tool {
	moves := map[string]bool{"touch": true, "requeue": true, "close": true}
	read := func(in json.RawMessage) (op, id, note string, drop, all bool) {
		var v struct {
			Op, ID, Note string
			Drop, All    bool
		}
		_ = json.Unmarshal(in, &v)
		return strings.ToLower(strings.TrimSpace(v.Op)), strings.TrimSpace(v.ID), strings.TrimSpace(v.Note), v.Drop, v.All
	}
	return &Tool{
		Name:        "queue",
		Description: "The workspace's shared work queue — what is held, who holds what. op list (all: true shows closed work too) and show <id> read it; on work you hold: touch <id> (a progress note), requeue <id> (hand it back, note why), close <id> (done; drop: true when it will not be done, note why). Work ordered to you arrives already claimed in your name.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"op":{"type":"string","enum":["list","show","touch","requeue","close"]},"id":{"type":"string"},"note":{"type":"string"},"drop":{"type":"boolean"},"all":{"type":"boolean"}},"required":["op"],"additionalProperties":false}`),
		Class:       Read,
		Classify: func(env Env, in json.RawMessage) Classification {
			if op, id, _, _, _ := read(in); moves[op] {
				return Classification{Class: Write, Why: "moves queue item " + id + " on the shared shelf (the harness writes it)"}
			}
			return Classification{Class: Read, Why: "reads the shared queue"}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			op, id, note, drop, all := read(in)
			if (op == "show" || moves[op]) && id == "" {
				return Result{Output: "queue " + op + " needs an id (queue list names them)", Err: true}
			}
			var it queue.Item
			var err error
			switch op {
			case "list", "":
				items, err := s.List()
				if err != nil {
					return Result{Output: "queue: " + err.Error(), Err: true}
				}
				var b strings.Builder
				for _, it := range items {
					if !all && it.State.Terminal() {
						continue
					}
					mark := ""
					if it.Owner == as {
						mark = "  ← yours"
					}
					b.WriteString(queue.Line(it) + mark + "\n")
				}
				if b.Len() == 0 {
					return Result{Output: "the queue is empty"}
				}
				return Result{Output: strings.TrimRight(b.String(), "\n")}
			case "show":
				var hist []queue.Item
				if it, hist, err = s.Get(id); err == nil {
					var b strings.Builder
					fmt.Fprintf(&b, "%s\nask: %s", queue.Line(it), it.Ask)
					for _, h := range hist {
						fmt.Fprintf(&b, "\n  r%d %s by %s", h.Rev, h.Op, h.By)
						if h.Note != "" {
							b.WriteString(" — " + h.Note)
						}
					}
					return Result{Output: b.String()}
				}
			case "touch":
				it, err = s.Touch(id, as, note)
			case "requeue":
				it, err = s.Requeue(id, as, note)
			case "close":
				to := queue.Done
				if drop {
					to = queue.Dropped
				}
				it, err = s.Close(id, as, to, note)
			default:
				return Result{Output: fmt.Sprintf("queue: unknown op %q (list, show, touch, requeue, close)", op), Err: true}
			}
			if err != nil {
				return Result{Output: err.Error(), Err: true}
			}
			return Result{Output: op + " " + it.ID + " — " + queue.Line(it)}
		},
	}
}
