package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/tool"
)

// twoLines is a workspace with two lanes and a service owner: coord-a → domain-a → zone-a, coord-b → domain-b →
// zone-b, service-rel under orchestrator. Every zone worker's verify is coded, so only the dispatch policy speaks.
func twoLines(t *testing.T) (string, *Workspace) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		".agent-one/AGENT-ONE.md":          policyText,
		".agent-one/log.md":                "# Chronicle\n\nAppend-only.\n\n---\n",
		".agent-one/coord/a/README.md":     "# coord-a\n\n- **Rank:** Coordinator\n- **Reports to:** orchestrator\n",
		".agent-one/coord/b/README.md":     "# coord-b\n\n- **Rank:** Coordinator\n- **Reports to:** orchestrator\n",
		".agent-one/domain/a/README.md":    "# domain-a\n\n- **Rank:** Domain owner\n- **Owns:** `a/`\n- **Reports to:** coord-a\n",
		".agent-one/domain/b/README.md":    "# domain-b\n\n- **Rank:** Domain owner\n- **Owns:** `b/`\n- **Reports to:** coord-b\n",
		".agent-one/zone/a/README.md":      "# zone-a\n\n- **Rank:** Zone worker\n- **Owns:** `a/`\n- **Reports to:** domain-a\n- **Verify:** `test -d a`\n",
		".agent-one/zone/b/README.md":      "# zone-b\n\n- **Rank:** Zone worker\n- **Owns:** `b/`\n- **Reports to:** domain-b\n- **Verify:** `test -d b`\n",
		".agent-one/service/rel/README.md": "# service-rel\n\n- **Rank:** Service owner\n- **Owns:** `.github/`\n- **Reports to:** orchestrator\n- **Verify:** `test -d .github`\n",
		"a/a.go":                           "package a\n",
		"b/b.go":                           "package b\n",
		".github/ci.yml":                   "on: push\n",
	}
	for p, agent := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(agent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, open(t, dir)
}

// The dispatch policy is member-level: a domain owner orders its own zone workers, a coordinator its own domain owners; a
// commission to another line's member is refused naming the parent that commissions it.
func TestDispatchFollowsTheLine(t *testing.T) {
	_, w := twoLines(t)
	strict := SubagentOptions{StrictFlow: true}
	may := func(from, to string, opt SubagentOptions) error {
		t.Helper()
		c := w.Member(to)
		if c == nil {
			t.Fatalf("no %s on the roster", to)
		}
		return w.mayDispatch(from, c, opt)
	}
	for _, ok := range [][2]string{{"domain-a", "zone-a"}, {"domain-b", "zone-b"}, {"coord-a", "domain-a"}, {"coord-b", "domain-b"}, {Orchestrator, "coord-a"}, {Orchestrator, "service-rel"}, {"domain-a", "domain-b"}} {
		if err := may(ok[0], ok[1], strict); err != nil {
			t.Errorf("%s → %s is its own line (or a sideways neighbour): %v", ok[0], ok[1], err)
		}
	}
	err := may("domain-a", "zone-b", strict)
	if err == nil || !strings.Contains(err.Error(), "not in your line") || !strings.Contains(err.Error(), "domain-b commissions it") {
		t.Fatalf("domain-a → zone-b must be refused naming domain-b: %v", err)
	}
	if err := may("coord-a", "domain-b", strict); err == nil || !strings.Contains(err.Error(), "coord-b commissions it") {
		t.Fatalf("coord-a → domain-b must be refused naming coord-b: %v", err)
	}
	// without strict flow a coordinator may reach down its own line two layers, never into another's
	open := SubagentOptions{}
	if err := may("coord-a", "zone-a", open); err != nil {
		t.Errorf("coord-a → zone-a is its own line (strict flow off): %v", err)
	}
	if err := may("coord-a", "zone-b", open); err == nil {
		t.Error("coord-a → zone-b crosses lines even with strict flow off")
	}
	// strict flow still holds one layer at a time
	if err := may("coord-a", "zone-a", strict); err == nil || !strings.Contains(err.Error(), "strict layer flow") {
		t.Errorf("coord-a → zone-a skips a layer under strict flow: %v", err)
	}
	// the escape (tools.dispatch.lineage: false): the rank policy alone decides
	if err := may("domain-a", "zone-b", SubagentOptions{StrictFlow: true, OpenLineage: true}); err != nil {
		t.Errorf("lineage off: any domain owner may order any zone worker: %v", err)
	}
	// the refusal list a dispatcher reads names only its own line
	pe := w.Engine("domain-a", Build{Tools: tool.Builtins()})
	if got := w.stratTargets(pe, strict); !strings.Contains(got, "zone-a") || strings.Contains(got, "zone-b") {
		t.Errorf("domain-a's lawful targets: %q", got)
	}
	// and the dispatcher itself refuses with the hint
	b := Build{Tools: tool.Builtins(), Subagent: strict}
	disp := w.Dispatcher(w.Engine("domain-a", b), b)
	if _, err := disp(context.Background(), tool.Env{Root: w.Root}, tool.Commission{Agent: "zone-b", Ask: "do it"}); err == nil || !strings.Contains(err.Error(), "domain-b commissions it") {
		t.Errorf("the dispatcher must refuse another line's zone worker: %v", err)
	}
}

// The service owner reports to orchestrator and nothing reports to it: it is never handed dispatch, whatever
// its tools list says; a domain owner, which commands zone workers, is.
func TestServiceHoldsNoDispatch(t *testing.T) {
	_, w := twoLines(t)
	b := Build{Tools: tool.Builtins(), Subagent: SubagentOptions{Enabled: true, MaxDepth: 3}, Depth: 1}
	has := func(as string) bool {
		e := w.Engine(as, b)
		defer w.Close(e)
		_, ok := e.Tools.Get("dispatch")
		return ok
	}
	if has("service-rel") {
		t.Error("the service owner commands no one: no dispatch on its shelf")
	}
	if !has("domain-a") {
		t.Error("a domain owner commands its zone workers: dispatch on its shelf")
	}
	if Ranks(DefaultRanks(Default())).Commands("service") || !Ranks(DefaultRanks(Default())).Commands("principal-domain-owner") {
		t.Error("Commands: service owner none; a principal domain owner commands its base's zone workers")
	}
}

// The TDD precondition reads the zone worker's doc fresh: a verify line the domain owner wrote earlier in the
// same turn (no reload since) is seen at the check.
func TestDispatchVerifyReadsTheDocFresh(t *testing.T) {
	dir, w := twoLines(t)
	bare := "# zone-a\n\n- **Rank:** Zone worker\n- **Owns:** `a/`\n- **Reports to:** domain-a\n"
	if err := os.WriteFile(filepath.Join(dir, ".agent-one/zone/a/README.md"), []byte(bare), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.Reload(); err != nil {
		t.Fatal(err)
	}
	b := Build{Tools: tool.Builtins(), Subagent: SubagentOptions{StrictFlow: true}}
	disp := w.Dispatcher(w.Engine("domain-a", b), b)
	if _, err := disp(context.Background(), tool.Env{Root: dir}, tool.Commission{Agent: "zone-a", Ask: "x"}); err == nil || !strings.Contains(err.Error(), "no Verify line") {
		t.Fatalf("an uncoded zone worker is refused: %v", err)
	}
	// the domain owner codes the verify this turn — the roster is not reloaded
	if err := os.WriteFile(filepath.Join(dir, ".agent-one/zone/a/README.md"), []byte(bare+"- **Verify:** `test -d a`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := w.Member("zone-a"); c.VerifyCoded() {
		t.Fatal("fixture: the roster's copy must still be stale")
	}
	if _, err := disp(context.Background(), tool.Env{Root: dir}, tool.Commission{Agent: "zone-a", Ask: "x"}); err != nil && strings.Contains(err.Error(), "no Verify line") {
		t.Fatalf("a verify written this turn must count: %v", err)
	}
}

// A rank's job reaches the agent's system prompt, in the distribution's word for the rank.
func TestPromptCarriesTheRanksJob(t *testing.T) {
	_, w := twoLines(t)
	p := w.Prompt("domain-a", PromptOptions{Member: true}, nil)
	domain, _ := w.Ranks.Get("domain")
	if !strings.Contains(p, "Your rank's job (Domain owner): "+strings.TrimSuffix(domain.Job, ".")) {
		t.Fatalf("the domain owner's job must ride its card:\n%s", p)
	}
}
