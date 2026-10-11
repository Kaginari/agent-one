package workspace

// Cross-session end-gate attribution commissioning (sealed 2026-10-03, per-rule6
// attribution gate, adapted from agent 1's design header). Prior session wrote a
// zone worker's doc pair; next session, a FRESH engine (new shell REPL in the same
// Workspace process — new Engine.Store / runtime.Mem) re-consults the same orchestrator.
// The sealed text requires the consult to hit a WORKSPACE-RESIDENT memory keyed by
// member name so consults pair cross-session. Redis-rollback covers the
// ignored-soil kit; the workspace lane lives in Workspace memory, so it survives
// Close(engine) and is cleared only by a NewWorkspace.
//
// Recording is driven through w.subagentGated — the exact block the after-hook
// fires (subagent.go after-hook, gated:429); driving the full dispatch loop would
// need a live LLM provider, which the fixture does not carry. Consult is
// exercised through w.subagentGatedTask (subagent.go:640), the per-owner-doc consult
// half that ownWrites consults. Engine "zone-x" is a valid Subagent per
// policyCheck, so the hook path records under the member name.

import (
	"context"
	"fmt"
	"testing"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/wire"

	"os"
	"path/filepath"
)

// TestGateCrossSessionSubagentPairingSurvivesClose proves arm 2's two halves:
// lane B written at subagentGated survives Close(engine), and a consult through
// a FRESH engine in the same Workspace pairs via the member name carried in
// w.sessions — no cross-member bleed, bound respected.
func TestGateCrossSessionSubagentPairingSurvivesClose(t *testing.T) {
	w := open(t, fixture(t)) // fixture has no .git — the ignored-soil case

	// (a) Drive the REAL recording block the after-hook calls, then Close.
	zoneE := w.Engine("zone-auth", Build{})
	w.subagentGated(zoneE, "zone-auth", []string{".agent-one/zone/auth/README.md"})
	w.Close(zoneE)

	rel := normRel(".agent-one/zone/auth/README.md")
	if got := w.subagentWrites["zone-auth"][rel].Writer; got == "" {
		t.Fatalf("lane B lost after Close: subagentWrites[zone-auth][%q] empty", rel)
	}

	// (b) Subagent-side consult through a FRESH engine pairs via s.Engine.As.
	fresh := w.Engine("zone-auth", Build{})
	if fresh == zoneE {
		t.Fatalf("fresh engine aliases the closed one — not a fresh consult")
	}
	// w.sessions only fills via the hook track() on a live turn (needs a real
	// provider); subagentGatedTask consults s.Engine directly, so a minimal
	// session shell over the fresh engine exercises the consult faithfully.
	freshSess := &loop.Session{Engine: fresh}
	if !w.subagentGatedTask(freshSess, ".agent-one/zone/auth/README.md") {
		t.Fatalf("fresh-engine consult did not pair cross-session for zone-auth doc")
	}

	// (c) A DIFFERENT member's doc is present in NO lane (no bleed).
	if w.subagentGatedTask(freshSess, ".agent-one/zone/api/README.md") {
		t.Fatalf("bleed: zone-api doc paired for zone-auth consult")
	}
	if got := w.subagentWrites["zone-api"][normRel(".agent-one/zone/api/README.md")].Writer; got != "" {
		t.Fatalf("bleed in workspace lane: zone-api has Writer %q", got)
	}
}

// TestGateCrossSessionSubagentWritesBound proves the workspace lane caps at
// subagentWritesBound=8 per member with the oldest evicted first.
func TestGateCrossSessionSubagentWritesBound(t *testing.T) {
	w := open(t, fixture(t))
	zoneE := w.Engine("zone-auth", Build{})

	for i := 0; i < subagentWritesBound+3; i++ {
		p := fmt.Sprintf(".agent-one/zone/auth/doc-%02d.md", i)
		w.subagentGated(zoneE, "zone-auth", []string{p})
	}
	m := w.subagentWrites["zone-auth"]
	if got := len(m); got > subagentWritesBound {
		t.Fatalf("subagentWrites over bound: got %d, bound %d", got, subagentWritesBound)
	}
	if got := m[normRel(".agent-one/zone/auth/doc-00.md")].Writer; got != "" {
		t.Fatalf("oldest doc-00 not evicted; Writer=%q", got)
	}
	last := fmt.Sprintf(".agent-one/zone/auth/doc-%02d.md", subagentWritesBound+2)
	if got := m[normRel(last)].Writer; got == "" {
		t.Fatalf("newest %q missing after bound evict", last)
	}
}

// TestGateArm2CrosssessionConsultThroughGate exercises the w.Gate end-gate
// level (deadlock close-out, zone worker policy-pair lane): the prior session sealed a
// doc subagent record via the real recording block arm A rides, Close dropped the
// engine-keyed ledger, and a FRESH session pairs the write through subagentPairedDoc
// arm 2 — the workspace-resident lane keyed by member name. Negative control: a
// doc with no subagent record anywhere fails arm 2's `rec.Writer == ""` engine-As
// short-circuit, so the same Gate shape FAILs Docs-as-code. gitToWorkspace sets a doc
// baseline; the test agents untracked write-dir rewrites drift from it.
func TestGateArm2CrosssessionConsultThroughGate(t *testing.T) {
	dir := initBaselineWorkspace(t)
	writeFixtureFile(t, dir, ".agent-one/zone/auth/README.md", "# auth\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", "auth doc baseline")
	w := open(t, dir)

	// Arm A: prior session commissions zone-auth's doc, then Closes.
	zoneE := w.Engine("zone-auth", Build{})
	w.subagentGated(zoneE, "zone-auth", []string{".agent-one/zone/auth/README.md"})
	w.Close(zoneE)

	// Arm B: FRESH engine embedded in a session shell — Gate consults
	// s.Engine.As directly, so track() is not needed for arm 2 to find lane B.
	fresh := w.Engine("zone-auth", Build{})
	sess := &loop.Session{Engine: fresh}
	writeFixtureFile(t, dir, ".agent-one/zone/auth/README.md", "# auth\ntouch\n")
	v := w.Gate(context.Background(), sess, GateOptions{Enabled: true, DocTruthful: true}, Orchestrator, []string{".agent-one/zone/auth/README.md"}, &wire.Report{}, false, "reshoot")
	if v.Word != "pass" {
		t.Fatalf("arm-2 cross-session consult at w.Gate level must PASS, got %q reasons=%v", v.Word, v.Reasons)
	}

	// Negative control: same drifted state, doc with no subagent record in any
	// lane, and no live session for the owner — every arm falls through.
	// RightAuthor: true is where the discrimination lives: the member-doc arm
	// fails an orchestrator-authored re-authoring with no pairing evidence. DocTruthful
	// by design only pairs NON-doc writes (its own-doc skip at the Docs-as-code lane
	// exempts owner docs), so it is on here only to keep the negative control's
	// options faithful to the real end-gate shape.
	neg := initBaselineWorkspace(t)
	writeFixtureFile(t, neg, ".agent-one/zone/api/README.md", "# api\n")
	git(t, neg, "add", "-A")
	git(t, neg, "commit", "-m", "api doc baseline")
	wn := open(t, neg)
	writeFixtureFile(t, neg, ".agent-one/zone/api/README.md", "# api\ntouch\n")
	// The live gate resolves as via s.Engine.As (gate.go:260), so the negative
	// control's session carries the orchestrator creed — the scene it pins is
	// orchestrator-authored re-authoring with no pairing evidence.
	n := wn.Gate(context.Background(), &loop.Session{Engine: wn.Engine(Orchestrator, Build{})}, GateOptions{Enabled: true, RightAuthor: true, DocTruthful: true}, Orchestrator, []string{".agent-one/zone/api/README.md"}, &wire.Report{}, false, "reshoot")
	if n.Word == "pass" {
		t.Fatalf("no-record doc passed pairing — arm-2 consult did not discriminate")
	}
}

// initBaselineWorkspace builds a real git work-tree from the census fixture so the
// Docs-as-code lane has a HEAD baseline to diff against (gitToWorkspace lives in the
// system tests, not this file). The doc baseline commit follows in-test.
func initBaselineWorkspace(t *testing.T) string {
	t.Helper()
	root := fixture(t)
	git(t, root, "init")
	git(t, root, "config", "user.email", "gate-test@agent-one.local")
	git(t, root, "config", "user.name", "gate-test")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "fixture baseline")
	return root
}

// writeFixtureFile writes a file inside the fixture workspace.
func writeFixtureFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
