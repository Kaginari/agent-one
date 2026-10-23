package app

import (
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/queue"
	"github.com/Kaginari/agent-one/workspace"
)

// TestRelayHoldWaitsForTheWake: a background report's @$ orders are held on the shared queue —
// nothing dispatches them, the in-memory book stays empty — until the wake claims them, once,
// each in its agent's name; the agent's DONE report closes its claim.
func TestRelayHoldWaitsForTheWake(t *testing.T) {
	_, a := forgeWorkspace(t)
	defer a.Close()
	a.Cfg.Tools.Dispatch.StrictFlow = false // the orchestrator may order a zone worker directly here
	a.Subagent.Finish("domain-security", "@S DONE\n@$ zone-auth: tighten the ttl\n@$ zone-api: page the routes\n@E 0")
	q := a.Workspace.Queue()
	held, _ := q.Held()
	if len(held) != 2 || held[0].For != "zone-auth" || held[0].From != "domain-security" || a.heldCount() != 2 {
		t.Fatalf("the orders are held on the shared queue: %+v", held)
	}
	if book := a.Subagent.OrderBook(); len(book) != 0 {
		t.Fatalf("a held order also sat in the memory book: %+v", book)
	}
	// another session's wake took zone-api's first: this wake leaves it alone
	if _, err := queue.Open(q.Path(), nil).Claim("q2", "zone-api"); err != nil {
		t.Fatal(err)
	}
	lines := a.heldWakeLines()
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "dispatch zone-auth: tighten the ttl\n(queue q1, claimed for zone-auth") {
		t.Fatalf("the wake's prompts: %q", lines)
	}
	if again := a.heldWakeLines(); len(again) != 0 || a.heldCount() != 0 {
		t.Fatalf("a row claimed twice: %q", again)
	}
	a.settleHeld("zone-api", "subagent zone-api started in the background")
	a.settleHeld("zone-api", "@S FAIL — tests red\n@E 0")
	a.settleHeld("zone-auth", "@S DONE\n@E 0")
	one, _, _ := q.Get("q1")
	two, _, _ := q.Get("q2")
	if one.State != queue.Done || two.State != queue.Claimed || !strings.Contains(two.Note, "reported FAIL") {
		t.Fatalf("settled: %+v / %+v", one, two)
	}
}

// TestRelayHoldWithoutAWorkspace: a subagent with no workspace keeps its orders in the memory book, and the
// wake still drains them.
func TestRelayHoldWithoutAWorkspace(t *testing.T) {
	a := &App{}
	a.Subagent = newSubagent(a)
	a.Subagent.Finish("domain-x", "@S DONE\n@$ zone-y: count the retries\n@E 0")
	if lines := a.heldWakeLines(); len(lines) != 1 || lines[0] != "dispatch zone-y: count the retries" {
		t.Fatalf("%q", lines)
	}
}

// TestRelayHoldRefusedOrderIsTheThrones: an order the orchestrator may not dispatch itself (strict
// flow: a zone worker is two layers down) is never claimed in the agent's name with no subagent coming —
// the wake claims it for the orchestrator, notes the refusal on it, and hands the orchestrator the ask to
// route down the chain.
func TestRelayHoldRefusedOrderIsTheThrones(t *testing.T) {
	_, a := forgeWorkspace(t)
	defer a.Close()
	a.Subagent.Finish("domain-security", "@S DONE\n@$ zone-auth: tighten the ttl\n@E 0")
	lines := a.heldWakeLines()
	it, _, _ := a.Workspace.Queue().Get("q1")
	if it.State != queue.Claimed || it.Owner != workspace.Orchestrator || !strings.Contains(it.Note, "may not dispatch zone-auth") {
		t.Fatalf("the refused order is the orchestrator's, the refusal noted: %+v", it)
	}
	if len(lines) != 1 || strings.HasPrefix(lines[0], "dispatch zone-auth") || !strings.Contains(lines[0], "tighten the ttl") || !strings.Contains(lines[0], "may not dispatch zone-auth") {
		t.Fatalf("the wake hands the orchestrator the ask with the refusal: %q", lines)
	}
	if again := a.heldWakeLines(); len(again) != 0 {
		t.Fatalf("taken twice: %q", again)
	}
}
