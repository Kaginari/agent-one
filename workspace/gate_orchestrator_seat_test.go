package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/wire"
)

func writeFixtureTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, agent := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(agent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The orchestrator's own seat files — the system manifest (.agent-one/system.yaml) and the policy
// (.agent-one/AGENT-ONE.md) — are orchestrator's work product, of one class with a member's own doc. When
// the orchestrator authors one, Owner must resolve to the orchestrator (orchestrator), not fall to the gate
// holder (domain owner): before the fix the manifest fell to domain-core by Owner's gate-holder fallback, so
// orchestrator's lawful relocate of it failed both right-author and Docs-as-code (demanding domain owner's README).
func TestGateOrchestratorSeatFilesAreTheOrchestrators(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	opt := DefaultHooks().Gate
	opt.InvariantsHold = false // the fixture's verify lines are not at issue here
	// DocTruthful STAYS ON — the live gate's Docs-as-code check is half the live failure.
	for _, seat := range []string{".agent-one/system.yaml", ".agent-one/AGENT-ONE.md"} {
		v := w.Gate(context.Background(), nil, opt, Orchestrator, []string{seat}, &wire.Report{Status: "DONE"}, true, "relocate manifest")
		if strings.Contains(v.String(), "Right zone worker authored") {
			t.Errorf("%s: right-author fired on an orchestrator seat file written by the orchestrator: %s", seat, v)
		}
		if strings.Contains(v.String(), "Doc truthful") {
			t.Errorf("%s: Docs-as-code demanded an agent's README for the orchestrator's own seat file: %s", seat, v)
		}
	}
}

// The orchestrator-seat gate-fail reproduced live in the chronicle: `.agent-one/system.yaml belongs
// to domain-core, written by orchestrator`. The existing fixture workspaces lack a gate-holder whose
// ownership reaches the manifest, so it was invisible there. This test fixture supplies the
// missing shape: a domain owner gate-holder whose ownership prefix is `.agent-one/` — exactly the live
// orchestrator seat that fired.
func TestGateOrchestratorSeatLiveShape(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		".agent-one/AGENT-ONE.md":          policyText,
		".agent-one/log.md":                "# Chronicle\n\nAppend-only.\n\n---\n",
		".agent-one/domain/core/README.md": "# domain-core\n\n- **Rank:** Domain owner\n- **Owns:** `.agent-one/`\n- **Verify:** `true`\n",
		// no coordinator, no zone worker: the live orchestrator's domain owner seat alone is enough to fire the bug
	}
	writeFixtureTree(t, dir, files)
	w, err := Open(dir, Default(), Options{Ontology: true})
	if err != nil {
		t.Fatal(err)
	}
	opt := DefaultHooks().Gate
	opt.InvariantsHold = false
	for _, seat := range []string{".agent-one/system.yaml", ".agent-one/AGENT-ONE.md"} {
		v := w.Gate(context.Background(), nil, opt, Orchestrator, []string{seat}, &wire.Report{Status: "DONE"}, true, "relocate manifest")
		if strings.Contains(v.String(), "Right zone worker authored") {
			t.Errorf("[live-shape] %s: right-author fired on an orchestrator seat file written by the orchestrator: %s", seat, v)
		}
		if strings.Contains(v.String(), "Doc truthful") {
			t.Errorf("[live-shape] %s: Docs-as-code demanded an agent's README for the orchestrator's own seat file: %s", seat, v)
		}
	}
}

// The exemption is the orchestrator authoring its own seat, not a blanket pass on the manifest: a
// non-orchestrator agent writing the orchestrator seat file still fails right-author.
func TestGateOrchestratorSeatStillPolicesNonOrchestrator(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	opt := DefaultHooks().Gate
	opt.InvariantsHold = false
	v := w.Gate(context.Background(), nil, opt, "zone-auth", []string{".agent-one/system.yaml"}, &wire.Report{Status: "DONE"}, true, "x")
	if v.Word != "fail" || !strings.Contains(v.String(), "Right zone worker authored") {
		t.Errorf("a non-orchestrator agent writing the orchestrator seat file must fail right-author: %+v", v)
	}
}
