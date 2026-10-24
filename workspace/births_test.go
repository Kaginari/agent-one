package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/value"
	"github.com/Kaginari/agent-one/wire"
)

// Every harness provisioning lands one line in the agent's own births file: a relief's young, and a
// provisioning that passed its gate — once, at the agent's first.
func TestBirthsLedger(t *testing.T) {
	dir := writeWorkspace(t, map[string]string{
		".agent-one/AGENT-ONE.md":          policyText,
		".agent-one/log.md":                "# Chronicle\n",
		".agent-one/coord/e/README.md":     "# coord-e\n\n- **Rank:** Coordinator\n- **Reports to:** orchestrator\n",
		".agent-one/domain/wide/README.md": "# domain-wide\n\n- **Rank:** Domain owner\n- **Owns:** `a/`, `b/`\n- **Reports to:** coord-e\n",
		"a/a.go":                           "package a\n",
		"b/b.go":                           "package b\n",
	})
	w := open(t, dir)
	if _, _, ok := w.relieve("domain-wide"); !ok {
		t.Fatal("no relief")
	}
	for _, y := range []string{"domain-wide-a", "domain-wide-b"} {
		bs := w.Births(y)
		if len(bs) != 1 || bs[0].Kind != "split" || bs[0].From != "domain-wide" || bs[0].Doc == "" {
			t.Errorf("%s births: %+v", y, bs)
		}
	}
	// a provisioning that passes is logged — once, at the agent's first, however often it passes
	gdir, gw := provisioningWorkspace(t)
	doc := ".agent-one/zone/pay/README.md"
	os.MkdirAll(filepath.Join(gdir, ".agent-one/zone/pay"), 0o755)
	os.WriteFile(filepath.Join(gdir, doc), []byte("# zone-pay\n\n- **Rank:** Zone worker\n- **Owns:** `src/pay/`\n- **Reports to:** domain-security\n- **Verify:** `true`\n"), 0o644)
	gate := gw.EndGate(DefaultHooks().Gate)
	for i := 0; i < 2; i++ {
		s := &loop.Session{Engine: &loop.Engine{As: "domain-security"}, Wrote: []string{doc}}
		if _, _, err := gate(context.Background(), s, &loop.Result{Report: wire.Report{Status: "DONE"}, IsWire: true}); err != nil {
			t.Fatalf("provisioning gate %d: %v", i, err)
		}
	}
	if bs := gw.Births("zone-pay"); len(bs) != 1 || bs[0].Kind != "provisioning" || bs[0].By != "domain-security" {
		t.Fatalf("zone-pay births: %+v", bs)
	}
}

// Tenure is whole days since the first provisioning; an agent with no births line is grandparented by
// the first commit of its doc; an agent neither speaks for reads zero.
func TestTenureSignal(t *testing.T) {
	dir := fixture(t)
	git(t, dir, "init", "-b", "main")
	git(t, dir, "add", "-A")
	old := time.Now().Add(-40 * 24 * time.Hour).Format(time.RFC3339)
	c := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "workspace")
	c.Dir, c.Env = dir, append(os.Environ(), "GIT_AUTHOR_DATE="+old, "GIT_COMMITTER_DATE="+old)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	w := open(t, dir)
	w.logBirth(birthRecord{At: time.Now().Add(-10*24*time.Hour - time.Hour), Agent: "zone-auth", Kind: "provisioning"})
	w.val.mu.Lock()
	for _, b := range []string{"zone-auth", "zone-api", "zone-ghost"} {
		w.appendValue(value.Row{Kind: value.KindRun, Agent: b, Status: "DONE", Word: "DONE"})
	}
	w.val.mu.Unlock()
	all := w.AllSignals()
	for agent, want := range map[string]float64{"zone-auth": 10, "zone-api": 40, "zone-ghost": 0} {
		if got := all[agent]["tenure"]; got != want {
			t.Errorf("%s tenure %v, want %v", agent, got, want)
		}
	}
}
