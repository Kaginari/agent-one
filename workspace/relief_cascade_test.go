package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, s := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A split walks the retiree's children: each one's Reports-to line moves to the young whose half
// holds its claims, the retiree names its heir, and every doc the act wrote is handed back.
func TestReliefCascadeSplit(t *testing.T) {
	dir := writeWorkspace(t, map[string]string{
		".agent-one/AGENT-ONE.md":          policyText,
		".agent-one/log.md":                "# Chronicle\n",
		".agent-one/coord/e/README.md":     "# coord-e\n\n- **Rank:** Coordinator\n- **Reports to:** orchestrator\n",
		".agent-one/domain/wide/README.md": "# domain-wide\n\n- **Rank:** Domain owner\n- **Owns:** `a/`, `b/`\n- **Reports to:** coord-e\n",
		".agent-one/zone/a/README.md":      "# zone-a\n\n- **Rank:** Zone worker\n- **Owns:** `a/x/`\n- **Reports to:** domain-wide\n",
		".agent-one/zone/b/README.md":      "# zone-b\n\n- **Rank:** Zone worker\n- **Owns:** `b/y/`\n- **Domain owner:** domain-wide\n",
		"a/x/a.go":                         "package x\n",
		"b/y/b.go":                         "package y\n",
	})
	w := open(t, dir)
	act, wrote, ok := w.relieve("domain-wide")
	if !ok || !strings.Contains(act, "split into domain-wide-a + domain-wide-b") || !strings.Contains(act, "re-seated zone-a, zone-b") {
		t.Fatalf("split: %v %q", ok, act)
	}
	for kid, heir := range map[string]string{"zone-a": "domain-wide-a", "zone-b": "domain-wide-b"} {
		if c := w.Member(kid); c == nil || c.Parent != heir {
			t.Errorf("%s reports to %v, want %s", kid, c, heir)
		}
	}
	for _, doc := range []string{".agent-one/domain/wide-a/README.md", ".agent-one/domain/wide-b/README.md", ".agent-one/domain/wide/README.md", ".agent-one/zone/a/README.md", ".agent-one/zone/b/README.md"} {
		if !contains(wrote, doc) {
			t.Errorf("%s missing from the act's writes %v", doc, wrote)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".agent-one/domain/wide/README.md")); !strings.Contains(string(b), "- **Heir:** domain-wide-a") {
		t.Errorf("the retiree names no heir:\n%s", b)
	}
	if line := w.lineOf("zone-b"); strings.Join(line, ",") != "domain-wide-b,coord-e,orchestrator" {
		t.Errorf("the living chain: %v", line)
	}
}

// A retirement re-seats the children one hop up; a child the cascade never reached still
// resolves through the retiree's Heir mark.
func TestReliefCascadeRetire(t *testing.T) {
	dir := writeWorkspace(t, map[string]string{
		".agent-one/AGENT-ONE.md":         policyText,
		".agent-one/log.md":               "# Chronicle\n",
		".agent-one/coord/e/README.md":    "# coord-e\n\n- **Rank:** Coordinator\n- **Reports to:** orchestrator\n",
		".agent-one/domain/mid/README.md": "# domain-mid\n\n- **Rank:** Domain owner\n- **Owns:** `zone/`\n- **Reports to:** coord-e\n",
		".agent-one/zone/n/README.md":     "# zone-n\n\n- **Rank:** Zone worker\n- **Owns:** `zone/n/`\n- **Reports to:** domain-mid\n",
		"zone/n/n.go":                     "package n\n",
	})
	w := open(t, dir)
	act, wrote, ok := w.relieve("domain-mid")
	if !ok || !strings.Contains(act, "retired domain-mid") || !contains(wrote, ".agent-one/zone/n/README.md") {
		t.Fatalf("retire: %v %q %v", ok, act, wrote)
	}
	if c := w.Member("zone-n"); c == nil || c.Parent != "coord-e" {
		t.Fatalf("zone-n reports to %+v, want coord-e", c)
	}
	// a child provisioned naming the retired seat (the cascade never saw it) resolves through the Heir
	os.MkdirAll(filepath.Join(dir, ".agent-one/zone/late"), 0o755)
	os.WriteFile(filepath.Join(dir, ".agent-one/zone/late/README.md"), []byte("# zone-late\n\n- **Rank:** Zone worker\n- **Reports to:** domain-mid\n"), 0o644)
	if err := w.Reload(); err != nil {
		t.Fatal(err)
	}
	if c := w.Member("zone-late"); c == nil || c.Parent != "coord-e" {
		t.Fatalf("zone-late reports to %+v, want the heir coord-e", c)
	}
}
