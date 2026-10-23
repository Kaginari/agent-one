package app

import (
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/queue"
	"github.com/Kaginari/agent-one/wire"
	"github.com/Kaginari/agent-one/workspace"
)

// The relay hold. A background Subagent's @$ orders are held on the workspace's shared queue
// (Subagent.Finish) — seen by `q list`, the queue tool and every session of the workspace — and nothing
// dispatches them mid-turn. The wake (the turn-done drain of the REPL and the terminal, and a
// report's wake into an idle session) claims them, each in the name of the agent it is ordered
// to, and opens the next turn with their dispatch prompts; a row another session claimed first
// is that session's. The agent's report settles the claim: DONE closes it, anything else leaves
// it claimed with the exit noted, for the orchestrator or the human to requeue or close.

// queue is the workspace's shared queue; nil with no workspace (the order book stays in memory).
func (a *App) queue() *queue.Store {
	if a.Workspace == nil {
		return nil
	}
	return a.Workspace.Queue()
}

// holdOrders puts a report's @$ orders on the shared queue, held for the wake.
func (a *App) holdOrders(from string, orders []wire.Enqueue) {
	q := a.queue()
	for _, o := range orders {
		if _, err := q.Add(queue.Item{For: o.Agent, From: from, Ask: o.Ask}, from); err != nil {
			a.hole("queue: @$ " + o.Agent + " held nowhere: " + err.Error())
		}
	}
}

// heldWakeLines is the wake's claim: the in-memory order book (a subagent with no workspace) and every
// held row of the shared queue, claimed and rendered as the next turn's dispatch prompts.
func (a *App) heldWakeLines() []string {
	out := a.dispatchOrders(a.Subagent.OrderBook())
	q := a.queue()
	if q == nil {
		return out
	}
	held, err := q.Held()
	if err != nil {
		a.hole("queue: " + err.Error())
		return out
	}
	for _, it := range held {
		owner := it.For
		if owner == "" {
			owner = workspace.Orchestrator
		}
		if _, err := q.Claim(it.ID, owner); err != nil {
			continue // another session's wake took it, or its agent is not the claimant's
		}
		if it.For == "" {
			out = append(out, fmt.Sprintf("queued work %s (from %s): %s", it.ID, orStr(it.From, "the queue"), it.Ask))
			continue
		}
		text := fmt.Sprintf("dispatch %s: %s\n(queue %s, claimed for %s — it closes when %s reports DONE)", it.For, it.Ask, it.ID, it.For, it.For)
		if rep := a.Subagent.reportOf(it.For); rep != "" {
			text += fmt.Sprintf("\n\n(the same agent's earlier Subagent reported — the thread so far:)\n%s", rep)
		}
		out = append(out, text)
	}
	return out
}

// settleHeld closes the oldest queue item an agent holds when its report lands DONE; any other
// exit stays claimed with the word noted. A text that is no wire report (a background ack)
// settles nothing.
func (a *App) settleHeld(agent, report string) {
	rep, ok := wire.ParseReport(report)
	q := a.queue()
	if !ok || q == nil {
		return
	}
	items, err := q.List()
	if err != nil {
		return
	}
	agent = strings.ToLower(agent)
	for _, it := range items {
		if it.State != queue.Claimed || it.Owner != agent {
			continue
		}
		word := strings.ToUpper(strings.Fields(rep.Status + " ")[0])
		if word == "DONE" {
			_, _ = q.Close(it.ID, workspace.Orchestrator, queue.Done, agent+" reported DONE")
		} else {
			_, _ = q.Touch(it.ID, workspace.Orchestrator, agent+" reported "+orStr(word, "no status")+" — requeue or close it")
		}
		return
	}
}

// heldCount is how many rows the shared queue holds for a wake.
func (a *App) heldCount() int {
	q := a.queue()
	if q == nil {
		return 0
	}
	held, _ := q.Held()
	return len(held)
}
