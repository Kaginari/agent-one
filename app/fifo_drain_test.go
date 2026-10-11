package app

import (
	"testing"

	"github.com/Kaginari/agent-one/wire"
)

// TestOrderBookDrains: an agent's queued @$ orders come back flat and drain; an agent that already
// ran and finished contributes its report so the next dispatch keeps the thread.
func TestOrderBookDrains(t *testing.T) {
	a := &App{}
	c := newSubagent(a)

	// coord-think finishes with a report; orchestrator's book orders a domain owner follow-up.
	c.Finish("coord-think", "@S DONE\n@F loop.go:42 the gate fired twice\n@E 9")
	c.Finish("orchestrator", "@S DONE\n@$ domain-auth: now gate the retry path\n@$ zone-ledger: count the retries\n@? a note with no order\n@E 40")

	got := c.OrderBook()
	if len(got) != 2 {
		t.Fatalf("order book: %v", got)
	}
	if got[0].Agent != "domain-auth" || got[0].Ask != "now gate the retry path" {
		t.Fatalf("first order: %+v", got[0])
	}
	if got[1].Agent != "zone-ledger" || got[1].Ask != "count the retries" {
		t.Fatalf("second order: %+v", got[1])
	}
	// a drained book stays drained.
	if again := c.OrderBook(); len(again) != 0 {
		t.Fatalf("drained the book twice: %v", again)
	}

	// the finished agent's report rides as thread when it exists *before* the order is drained.
	c.Finish("domain-auth", "@S DONE\n@F loop.go:88 retry gated\n@E 8")
	c.Finish("orchestrator", "@S DONE\n@$ domain-auth: gate the abort path too\n@$ coord-plan: plan the next layer\n@E 30")
	got = c.OrderBook()
	if len(got) != 2 {
		t.Fatalf("second drain: %v", got)
	}
	byAgent := map[string]string{}
	for _, o := range got {
		byAgent[o.Agent] = o.Report
	}
	if byAgent["domain-auth"] == "" || byAgent["domain-auth"][:2] != "@S" {
		t.Fatalf("the finished Subagent's report rides as thread: %+v", got)
	}
	if byAgent["coord-plan"] != "" {
		t.Fatalf("a fresh order carries no thread: %+v", got)
	}
}

// TestDispatchOrdersPrompt: the prompt a drained order becomes names the dispatch and carries
// any thread.
func TestDispatchOrdersPrompt(t *testing.T) {
	a := &App{}
	got := a.dispatchOrders([]struct{ Agent, Ask, Report string }{
		{Agent: "domain-a", Ask: "gate the retry path", Report: "@S DONE\n@F loop.go:88 fired\n"},
		{Agent: "zone-b", Ask: "count holes"},
	})
	if len(got) != 2 {
		t.Fatalf("dispatchOrders: %v", got)
	}
	if got[0] != "dispatch domain-a: gate the retry path\n\n(the same agent's earlier Subagent reported — the thread so far:)\n@S DONE\n@F loop.go:88 fired\n" {
		t.Fatalf("with thread: %q", got[0])
	}
	if got[1] != "dispatch zone-b: count holes" {
		t.Fatalf("without thread: %q", got[1])
	}
}

// silence the wire import — the test leans on the wire's Parse via Finish.
var _ = wire.Enqueue{}
