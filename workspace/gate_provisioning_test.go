package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/wire"
)

// provisioningWorkspace is the fixture under git, committed, with an unclaimed src/pay/ on the tree.
func provisioningWorkspace(t *testing.T) (string, *Workspace) {
	t.Helper()
	dir := fixture(t)
	if err := os.MkdirAll(filepath.Join(dir, "src/pay"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src/pay/pay.go"), []byte("package pay\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "workspace")
	return dir, open(t, dir)
}

// A zone-parent may register its own child's README: new since the last commit, reporting to the
// writer, claiming live soil that is the writer's or nobody's. Each condition is load-bearing.
func TestGateProvisioningExemption(t *testing.T) {
	dir, w := provisioningWorkspace(t)
	g := DefaultHooks().Gate
	rep := &wire.Report{Status: "DONE"}
	register := func(as, rel, doc string) Verdict {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		v := w.Gate(context.Background(), nil, g, as, []string{rel}, rep, true, "x")
		os.RemoveAll(filepath.Dir(full))
		_ = w.Reload()
		return v
	}
	child := func(name, parent, terr string) string {
		return "# " + name + "\n\n- **Rank:** Zone worker\n- **Owns:** `" + terr + "`\n- **Reports to:** " + parent + "\n- **Verify:** `true`\n"
	}
	if v := register("domain-security", ".agent-one/zone/pay/README.md", child("zone-pay", "domain-security", "src/pay/")); v.Word != "pass" {
		t.Errorf("the parent's provisioning of its child: %s", v)
	}
	if v := register(Orchestrator, ".agent-one/coord/ops/README.md", "# coord-ops\n\n- **Rank:** Coordinator\n- **Reports to:** orchestrator\n"); v.Word != "pass" {
		t.Errorf("the orchestrator founding a coordinator: %s", v)
	}
	for why, c := range map[string][2]string{
		"another parent's child":   {".agent-one/zone/pay/README.md", child("zone-pay", "domain-other", "src/pay/")},
		"a third agent's soil":     {".agent-one/zone/pay/README.md", child("zone-pay", "domain-security", "src/auth/")},
		"a claim on nothing":       {".agent-one/zone/pay/README.md", child("zone-pay", "domain-security", "src/ghost/")},
		"a claim over a third one": {".agent-one/zone/pay/README.md", child("zone-pay", "domain-security", "src/")},
	} {
		if v := register("domain-security", c[0], c[1]); v.Word != "fail" || !strings.Contains(v.String(), "Right zone worker authored") {
			t.Errorf("%s passed as provisioning: %s", why, v)
		}
	}
	// one commit window: once the child's doc is committed, its re-authoring is the child's
	rel := ".agent-one/zone/pay/README.md"
	full := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(full), 0o755)
	os.WriteFile(full, []byte(child("zone-pay", "domain-security", "src/pay/")), 0o644)
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "born")
	os.WriteFile(full, []byte(child("zone-pay", "domain-security", "src/pay/")+"- more\n"), 0o644)
	_ = w.Reload()
	if v := w.Gate(context.Background(), nil, g, "domain-security", []string{rel}, rep, true, "x"); v.Word != "fail" {
		t.Errorf("provisioning outlived its commit window: %s", v)
	}
}

// Without git, "new" is read against the roster the turn opened with — and a heal between the
// README's write and the gate (a dispatch preflight, a policy check, a roster miss) does not
// make the child "known" before its provisioning.
func TestGateProvisioningNoGitSurvivesAHeal(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	os.MkdirAll(filepath.Join(dir, "src/pay"), 0o755)
	rel := ".agent-one/zone/pay/README.md"
	full := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte("# zone-pay\n\n- **Rank:** Zone worker\n- **Owns:** `src/pay/`\n- **Reports to:** domain-security\n- **Verify:** `true`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !w.SelfHeal() || w.Member("zone-pay") == nil {
		t.Fatal("the heal did not read the new seat")
	}
	if v := w.Gate(context.Background(), nil, DefaultHooks().Gate, "domain-security", []string{rel}, &wire.Report{Status: "DONE"}, true, "x"); v.Word != "pass" || len(v.Born) != 1 {
		t.Fatalf("a heal before the gate broke a no-git provisioning: %s", v)
	}
}
