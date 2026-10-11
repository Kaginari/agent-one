package workspace

import (
	"testing"

	"github.com/Kaginari/agent-one/loop"
)

// TestLedgersSurviveRestarts covers the shelf policy (freeze review 2026-10-07): council
// strain, consult counts, stress-death memos and the subagent-write notebook are appended to
// instruments/ledgers.jsonl as they mutate, and a fresh Open replays them. A workspace that
// lost those at every restart relearned nothing.
func TestLedgersSurviveRestarts(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	// strain: a failed subagent under escalate-at 1 books a warrant ledger row
	w.councilLedger("domain-security", "FAIL", &loop.Result{Status: loop.Fail}, SubagentOptions{StressEscalateAt: 1, StressMaxConsults: 3})
	if w.stress["domain-security"] == 0 {
		t.Fatal("the strain did not land in the ledger")
	}
	// death memo + write attribution: noted the same way the real mutations do
	w.mu.Lock()
	w.ledgerNote(ledgerRow{Kind: "death", Agent: "zone-auth", Lane: "glm-5-3-conduit"})
	w.ledgerNote(ledgerRow{Kind: "subagentWrite", Agent: "zone-auth", Path: ".agent-one/zone/auth/README.md", Writer: "zone-auth"})
	w.mu.Unlock()

	// workspace: a fresh Open on the same workspace must carry the scars
	w2 := open(t, dir)
	func() {
		w2.mu.Lock()
		defer w2.mu.Unlock()
		if w2.stress["domain-security"] == 0 || w2.consults["domain-security"] == 0 {
			t.Fatalf("strain/consults did not replay: %+v %+v", w2.stress, w2.consults)
		}
		if w2.stressDeaths["zone-auth"].Lane != "glm-5-3-conduit" {
			t.Fatalf("the death memo did not replay: %+v", w2.stressDeaths)
		}
		if cw := w2.subagentWrites["zone-auth"][normRel(".agent-one/zone/auth/README.md")]; cw.Writer != "zone-auth" {
			t.Fatalf("the write attribution did not replay: %+v", w2.subagentWrites)
		}
	}()

	// the clean-landing policy replays too: strain booked, then cleared, stays cleared
	w2.councilLedger("domain-security", "FAIL", &loop.Result{Status: loop.Fail}, SubagentOptions{StressEscalateAt: 1})
	w2.councilLedger("domain-security", "DONE", &loop.Result{Status: loop.Done}, SubagentOptions{StressEscalateAt: 1})
	w3 := open(t, dir)
	w3.mu.Lock()
	defer w3.mu.Unlock()
	if _, ok := w3.stress["domain-security"]; ok {
		t.Fatalf("a clean landing must clear the ledger forever: %+v", w3.stress)
	}
}
