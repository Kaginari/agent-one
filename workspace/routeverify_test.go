package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/loop"
)

// A lineage refusal names the lawful dispatch path, each hop proven against the dispatch policy:
// the nearest member of the target's line the dispatcher may order itself, else up with @? to
// where the lines meet and down from there.
func TestRouteVerification(t *testing.T) {
	_, w := twoLines(t)
	strict := SubagentOptions{StrictFlow: true}
	refuse := func(from, to string) string {
		t.Helper()
		err := w.mayDispatch(from, w.Member(to), strict)
		if err == nil {
			t.Fatalf("%s → %s went through", from, to)
		}
		return err.Error()
	}
	if got := refuse("domain-a", "zone-b"); !strings.Contains(got, "lawful route: commission domain-b, which orders down (domain-b → zone-b)") {
		t.Errorf("sideways route: %s", got)
	}
	if got := refuse("coord-a", "domain-b"); !strings.Contains(got, "lawful route: carry it up with @? (coord-a → orchestrator), then orchestrator → coord-b → domain-b") {
		t.Errorf("up-and-down route: %s", got)
	}
}

// A Reports-to hop resolves against the living chain: a hop nothing living holds ends the line,
// is never offered as the one who commissions, and no route is guessed through it.
func TestLineageLivingChain(t *testing.T) {
	dir, w := twoLines(t)
	stray := filepath.Join(dir, ".agent-one/zone/stray/README.md")
	os.MkdirAll(filepath.Dir(stray), 0o755)
	os.WriteFile(stray, []byte("# zone-stray\n\n- **Rank:** Zone worker\n- **Reports to:** domain-ghost\n- **Verify:** `true`\n"), 0o644)
	if err := w.Reload(); err != nil {
		t.Fatal(err)
	}
	if line, broken := w.lineage("zone-stray"); len(line) != 0 || broken != "domain-ghost" {
		t.Fatalf("lineage: %v broken at %q", line, broken)
	}
	err := w.mayDispatch("domain-a", w.Member("zone-stray"), SubagentOptions{})
	if err == nil || !strings.Contains(err.Error(), "its line breaks at domain-ghost") || strings.Contains(err.Error(), "domain-ghost commissions it") {
		t.Fatalf("a dead hop: %v", err)
	}
	if err := w.mayDispatch(Orchestrator, w.Member("zone-stray"), SubagentOptions{}); err != nil {
		t.Fatalf("orchestrator tops every chain: %v", err)
	}
}

// The lawful-target list is exactly what preflight accepts: a zone worker with no coded verdict is not
// offered to its gate holder, and an agent is named once.
func TestStratTargetsFaithful(t *testing.T) {
	dir, w := twoLines(t)
	bare := filepath.Join(dir, ".agent-one/zone/bare/README.md")
	os.MkdirAll(filepath.Dir(bare), 0o755)
	os.WriteFile(bare, []byte("# zone-bare\n\n- **Rank:** Zone worker\n- **Owns:** `a/`\n- **Reports to:** domain-a\n"), 0o644)
	w.AddForeign(Member{Name: "zone-a", Rank: "zone"})
	if err := w.Reload(); err != nil {
		t.Fatal(err)
	}
	got := w.stratTargets(&loop.Engine{As: "domain-a"}, SubagentOptions{StrictFlow: true})
	if strings.Contains(got, "zone-bare") || strings.Count(got, "zone-a") != 1 {
		t.Fatalf("domain-a's lawful targets: %q", got)
	}
	if err := w.MayOrder("domain-a", "zone-bare", SubagentOptions{StrictFlow: true}); err == nil {
		t.Fatal("preflight took an order with no coded verdict")
	}
}
