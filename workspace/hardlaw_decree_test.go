package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/tool"
)

// Hard policy (Operator decree 2026-09-30): orchestrator writing another member's soil is refused at
// the policy level with a dispatch redirection; only the owner's own README, the orchestrator seat
// files, and no-member soil stay lawful.
func TestPolicyOrchestratorCrossOwnershipRefused(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	pol := w.ToolPolicy(Orchestrator, OwnershipOptions{Enabled: true})
	if pol == nil {
		t.Fatal("policy nil")
	}

	call := func(paths ...string) error {
		abs := make([]string, 0, len(paths))
		for _, p := range paths {
			abs = append(abs, w.Root+"/"+p)
		}
		return pol(tool.Access{Tool: "write", Class: tool.Write, Paths: abs})
	}

	// 1. refused on zone-owned soil
	if err := call("src/auth/login.go"); err == nil || !strings.Contains(err.Error(), "dispatch") {
		t.Errorf("orchestrator on zone worker soil must refuse with a dispatch redirection: %v", err)
	}
	// 2. allowed on the owner's own doc — that's the pairing lever
	if err := call(".agent-one/zone/auth/README.md"); err != nil {
		t.Errorf("orchestrator writing the zone worker's own README is lawful (Docs-as-code pairing): %v", err)
	}
	// 3. allowed on the orchestrator seat files
	for _, seat := range []string{".agent-one/system.yaml", ".agent-one/AGENT-ONE.md"} {
		if err := call(seat); err != nil {
			t.Errorf("orchestrator on orchestrator seat %s is the orchestrator's own work: %v", seat, err)
		}
	}
	// 4. allowed on no-member soil
	if err := call("notes/freeform.txt"); err != nil {
		t.Errorf("orchestrator on no-member soil is lawful: %v", err)
	}
	// 5. rank-less name (a mount under orchestrator but not the literal string) also held
	pol2 := w.ToolPolicy("", OwnershipOptions{Enabled: true})
	if err := pol2(tool.Access{Tool: "write", Class: tool.Write, Paths: []string{w.Root + "/src/auth/login.go"}}); err == nil {
		t.Error("rank-less mount under the orchestrator must be held to the hard-policy refusal too")
	}
}

// Domain-TDD as code (Policy 3 + Operator decree 2026-09-30): a gate-holding rank dispatching an
// authoring zone worker whose doc carries no Verify line is refused at dispatch time.
func TestDispatchVerifyPrecondition(t *testing.T) {
	// zone-no-verify fixture: an authoring zone worker whose README carries no Verify lines
	dir := t.TempDir()
	writeDocHL(t, dir, ".agent-one/AGENT-ONE.md", policyText)
	writeDocHL(t, dir, ".agent-one/log.md", "# Chronicle\n\nAppend-only.\n\n---\n")
	writeDocHL(t, dir, ".agent-one/domain/core/README.md", "# domain-core\n\n- **Rank:** Domain owner\n- **Owns:** `workspace/`\n- **Verify:** `true`\n")
	writeDocHL(t, dir, ".agent-one/zone/dirty/README.md", "# zone-dirty\n\n- **Rank:** Zone worker\n- **Owns:** `workspace/dirty/`\n- **Reports to:** domain-core\n")

	w, err := Open(dir, Default(), Options{Ontology: true})
	if err != nil {
		t.Fatal(err)
	}

	// Locate the two members
	domain := w.Member("domain-core")
	zone := w.Member("zone-dirty")
	if domain == nil || zone == nil {
		t.Fatalf("fixture: domain owner=%v zone worker=%v", domain != nil, zone != nil)
	}
	if zone.VerifyCoded() {
		t.Fatal("zone-dirty must start verify-less for this test")
	}

	// Build a parent engine belonging to domain-core. The dispatcher reads only parent.As and
	// reaches for session/budget only when non-nil — w.Engine wires the lexicon and hooks.
	parentBuild := Build{Tools: tool.Builtins()}
	pe := w.Engine("domain-core", parentBuild)
	_ = loop.Engine{}

	disp := w.Dispatcher(pe, parentBuild)
	_, err = disp(context.Background(), tool.Env{Root: dir}, tool.Commission{Agent: "zone-dirty", Ask: "implement X"})
	if err == nil {
		t.Fatal("dispatch without zone worker verify must be refused")
	}
	if !strings.Contains(err.Error(), "no Verify line") || !strings.Contains(err.Error(), "TDD") {
		t.Errorf("refusal must name the missing verify line + TDD rule: %v", err)
	}

	// With the verify line coded, the dispatch is allowed (gate still may fail on the zone worker's
	// writing, but the *dispatch gate* must not answer with the verify-precondition error).
	writeDocHL(t, dir, zone.Doc, "# zone-dirty\n\n- **Rank:** Zone worker\n- **Owns:** `workspace/dirty/`\n- **Reports to:** domain-core\n\n## Verify\n- `test -d workspace/dirty`\n")
	// reload so the roster re-derives
	if err := w.Reload(); err != nil {
		t.Fatal(err)
	}
	zone = w.Member("zone-dirty")
	if !zone.VerifyCoded() {
		t.Fatal("zone-dirty must now read VerifyCoded == true")
	}
	// The dispatch now proceeds; we don't care about outcome — we assert the refusal error text
	// is no longer the verify-precondition one. A run failure (budget exhausted etc.) is fine
	// and out of this test's scope; we only check the dispatch gate doesn't answer with TDD.
	_, err2 := disp(context.Background(), tool.Env{Root: dir}, tool.Commission{Agent: "zone-dirty", Ask: "implement X"})
	if err2 != nil && strings.Contains(err2.Error(), "no Verify line") && strings.Contains(err2.Error(), "TDD") {
		t.Errorf("verify-precondition must not fire once the zone worker is coded: %v", err2)
	}
}

func writeDocHL(t *testing.T, dir, rel, agent string) {
	t.Helper()
	full := dir + "/" + rel
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(agent), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPolicyOrchestratorNeverAuthors — Policy 7 decree 2026-10-01 flag-gated via OwnershipOptions.OrchestratorNeverAuthors.
// When enabled, the orchestrator authors nothing but the policy; pairing exemption for owner's README dies under Policy 7.
func TestPolicyOrchestratorNeverAuthors(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	// helper call mimics the existing pattern
	makeCall := func(opt OwnershipOptions) func(paths ...string) error {
		pol := w.ToolPolicy(Orchestrator, opt)
		if pol == nil {
			return func(paths ...string) error { return nil }
		}
		return func(paths ...string) error {
			abs := make([]string, 0, len(paths))
			for _, p := range paths {
				abs = append(abs, w.Root+"/"+p)
			}
			return pol(tool.Access{Tool: "write", Class: tool.Write, Paths: abs})
		}
	}

	// 1. Flag on: orchestratorNeverAuthors true
	callOn := makeCall(OwnershipOptions{Enabled: true, OrchestratorNeverAuthors: true})
	// a) freeform no-member soil must REFUSE with Policy 7 + dispatch
	if err := callOn("notes/freeform.txt"); err == nil || !strings.Contains(err.Error(), "Policy 7") || !strings.Contains(err.Error(), "dispatch") {
		t.Errorf("OrchestratorNeverAuthors on: notes/freeform.txt must refuse with Policy 7 + dispatch, got %v", err)
	}
	// b) policy file passes
	if err := callOn(".agent-one/AGENT-ONE.md"); err != nil {
		t.Errorf("OrchestratorNeverAuthors on: .agent-one/AGENT-ONE.md must pass, got %v", err)
	}
	// c) owner's README is now refused (pairing exemption dies)
	if err := callOn(".agent-one/zone/auth/README.md"); err == nil || !strings.Contains(err.Error(), "Policy 7") {
		t.Errorf("OrchestratorNeverAuthors on: zone worker README must refuse with Policy 7, got %v", err)
	}

	// 2. Flag off (zero value): legacy behavior — no-member soil passes
	callOff := makeCall(OwnershipOptions{Enabled: true})
	if err := callOff("notes/freeform.txt"); err != nil {
		t.Errorf("OrchestratorNeverAuthors off: notes/freeform.txt must pass (legacy), got %v", err)
	}
}
