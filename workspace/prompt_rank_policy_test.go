package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rank-policy slot (Operator 2026-10-03): each agent boots with the principles + its rank's page —
// one rank's words must never leak into another's context (context economy with intelligence).
func TestRankPolicyNoLeaking(t *testing.T) {
	dir := fixture(t)
	rl := filepath.Join(dir, ".agent-one", "policy", "rank")
	if err := os.MkdirAll(rl, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(rl, "zone.md"), []byte("# Rank policy — zone worker\n\n- ZONE-ONLY-MARKER the domain owner commands you one slice.\n"), 0o644)
	os.WriteFile(filepath.Join(rl, "coord.md"), []byte("# Rank policy — coordinator\n\n- COORD-ONLY-MARKER hold the 3 keep 2.\n"), 0o644)
	os.WriteFile(filepath.Join(rl, "orchestrator.md"), []byte("# Rank policy — orchestrator\n\n- ORCHESTRATOR-ONLY-MARKER author three cards.\n"), 0o644)

	w := open(t, dir)
	for _, tc := range []struct {
		as         string
		mustHold   string
		mustNotSee []string
	}{
		{"zone-auth", "ZONE-ONLY-MARKER", []string{"COORD-ONLY-MARKER", "ORCHESTRATOR-ONLY-MARKER"}},
		{"zone-api", "ZONE-ONLY-MARKER", []string{"COORD-ONLY-MARKER", "ORCHESTRATOR-ONLY-MARKER"}},
		{"coord-core", "COORD-ONLY-MARKER", []string{"ZONE-ONLY-MARKER", "ORCHESTRATOR-ONLY-MARKER"}},
	} {
		p := w.Prompt(tc.as, DefaultPrompt(), nil)
		if !strings.Contains(p, tc.mustHold) {
			t.Fatalf("%s: its own rank's page is missing\n%s", tc.as, p[:400])
		}
		for _, bad := range tc.mustNotSee {
			if strings.Contains(p, bad) {
				t.Fatalf("%s sees another rank's policy (%s) — a leak", tc.as, bad)
			}
		}
	}
	// the orchestrator never blanks its page — orchestrator belongs to a rank too
	p := w.Prompt("orchestrator", DefaultPrompt(), nil)
	if !strings.Contains(p, "ORCHESTRATOR-ONLY-MARKER") {
		t.Fatal("orchestrator's page never reached the orchestrator")
	}
	// a rank with no page yet gets NO phantom section — silent, never a lie
	if q := w.Prompt("nobody-nowhere", DefaultPrompt(), nil); strings.Contains(q, "Policy of your rank") {
		t.Fatalf("a rankless agent must not hold a policy section:\n%s", q[:300])
	}
}
