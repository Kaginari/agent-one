package workspace

import (
	"strings"
	"testing"
)

// Pins both legs of the configuration-mutation policy (Operator decree 2026-10-02): a burned lane
// climbs, an out-of-ladder or topmost lane holds (no silent mutation, no invention).
func TestLaneUp(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"openai/meta/muse-glimmer-30b", "openai/moonshotai/kimi-k3"},
		{"openai/moonshotai/kimi-k3", "openai/z-ai/glm-5.3"},
		{"openai/z-ai/glm-5.3", ""},                              // peak: split, don't mutate past it
		{"openai/some-future-model", ""},                         // off-ladder: advice belongs to config, not to the ladder
		{"openrouter/moonshotai/kimi-k3", "openai/z-ai/glm-5.3"}, // machine-config naming resolves
		{"moonshotai/kimi-k3", ""},                               // a bare vendor/model is no lane: the provider part is missing
	} {
		if got := laneUp(c.in); got != c.want {
			t.Errorf("laneUp(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// stratTargets on the fixture workspace: orchestrator may dispatch the coordinator only (Policy 2 + strict
// layer flow — zone/domain are deeper) and a coordinator may dispatch the domain owners it parents.
func TestStratTargets(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	thrn := w.Engine("orchestrator", Build{Tools: nil})
	got := w.stratTargets(thrn, SubagentOptions{StrictFlow: true})
	if !strings.Contains(got, "coord-core") || strings.Contains(got, "zone-auth") {
		t.Fatalf("orchestrator may reach only its child rank (coord-core here): %q", got)
	}
	coord := w.Engine("coord-core", Build{Tools: nil})
	got = w.stratTargets(coord, SubagentOptions{StrictFlow: true})
	if !strings.Contains(got, "domain-security") || strings.Contains(got, "zone-auth") {
		t.Fatalf("coord-core may reach its domain owners, never the zone workers directly: %q", got)
	}
}
