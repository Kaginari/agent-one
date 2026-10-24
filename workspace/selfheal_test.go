package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/wire"
)

// The docs are the store, the roster their cache: a seat retired by another hand is no longer
// dispatchable, a claim granted on disk is honoured by the ownership policy — and a heal never
// makes a rewritten verify line trusted.
func TestSelfHealReadsTheDocs(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	api := filepath.Join(dir, ".agent-one/zone/api/README.md")
	b, _ := os.ReadFile(api)
	if err := os.WriteFile(api, append(b, []byte("\n"+retiredMark+" — retired by another session\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.MayOrder("domain-security", "zone-api", SubagentOptions{}); err == nil || !strings.Contains(err.Error(), `no agent named "zone-api"`) {
		t.Fatalf("a seat retired on disk is still dispatchable: %v", err)
	}
	// a claim widened on disk: zone-auth now holds docs/ too
	auth := filepath.Join(dir, ".agent-one/zone/auth/README.md")
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	if err := os.WriteFile(auth, []byte("# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`, `docs/`\n- **Reports to:** domain-security\n- **Verify:** `true`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pol := w.ToolPolicy("zone-auth", OwnershipOptions{Enabled: true})
	if err := pol(writeAccess(dir, "docs/x.md")); err != nil {
		t.Fatalf("a claim granted on disk was refused: %v", err)
	}
	// ...and the healed roster did not move the gate's trust: the rewritten line is a hole
	v := w.Gate(context.Background(), nil, DefaultHooks().Gate, "zone-auth", []string{"src/auth/login.go", ".agent-one/zone/auth/README.md"}, &wire.Report{Status: "DONE"}, true, "x")
	if !strings.Contains(strings.Join(v.Holes, "|"), "verify `true` was added this turn") || !strings.Contains(strings.Join(v.Holes, "|"), "verify `test ! -f src/auth/BROKEN` was removed or changed this turn") {
		t.Fatalf("a heal moved the gate's trust: %+v", v)
	}
}

func writeAccess(dir, rel string) tool.Access {
	return tool.Access{Class: tool.Write, Paths: []string{filepath.Join(dir, filepath.FromSlash(rel))}}
}
