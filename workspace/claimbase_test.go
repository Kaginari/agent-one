package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/wire"
)

// An agent never grants itself soil another living agent held when the turn opened: zone-auth
// widens its own README onto src/api/handlers/ (a longer prefix under its rank-mate zone-api's
// src/api/) and writes there in the same turn — the policy refuses the write and the gate fails
// the claim, whatever the healed roster's longest prefix says.
func TestSelfClaimNeverTakesARankMatesSoil(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	os.MkdirAll(filepath.Join(dir, "src/api/handlers"), 0o755)
	auth := filepath.Join(dir, ".agent-one/zone/auth/README.md")
	if err := os.WriteFile(auth, []byte("# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`, `src/api/handlers/`\n- **Reports to:** domain-security\n- **Skills:** drafter\n- **Verify:** `test ! -f src/auth/BROKEN`\n\n## Invariants\n- tokens expire after one hour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pol := w.ToolPolicy("zone-auth", OwnershipOptions{Enabled: true})
	err := pol(writeAccess(dir, "src/api/handlers/h.go"))
	if err == nil || !strings.Contains(err.Error(), "zone-api") {
		t.Fatalf("a self-widened claim admitted a rank-mate's soil: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "src/api/handlers/h.go"), []byte("package handlers\n"), 0o644)
	v := w.Gate(context.Background(), nil, DefaultHooks().Gate, "zone-auth", []string{".agent-one/zone/auth/README.md", "src/api/handlers/h.go"}, &wire.Report{Status: "DONE"}, true, "x")
	if v.Word != "fail" || !strings.Contains(v.String(), "zone-api") {
		t.Fatalf("the gate passed a self-granted claim on a rank-mate's soil: %s", v)
	}
	// the failed claim does not become trusted at the gate's reload: the next turn is still refused
	if err := pol(writeAccess(dir, "src/api/handlers/h.go")); err == nil {
		t.Fatal("a claim the gate failed was honoured on the next turn")
	}
	v = w.Gate(context.Background(), nil, DefaultHooks().Gate, "zone-auth", []string{"src/api/handlers/h.go"}, &wire.Report{Status: "DONE"}, true, "x")
	if v.Word != "fail" {
		t.Fatalf("the next turn's gate passed the stolen soil: %s", v)
	}
	// the lawful widen: a gate other than the claimant's — the orchestrator's — carries the README
	v = w.Gate(context.Background(), nil, DefaultHooks().Gate, "", []string{".agent-one/zone/auth/README.md"}, &wire.Report{Status: "DONE"}, true, "x")
	if v.Word != "pass" {
		t.Fatalf("the orchestrator's widen did not pass: %s", v)
	}
	if err := pol(writeAccess(dir, "src/api/handlers/h.go")); err != nil {
		t.Fatalf("a claim the orchestrator granted was refused: %v", err)
	}
	v = w.Gate(context.Background(), nil, DefaultHooks().Gate, "zone-auth", []string{"src/api/handlers/h.go", ".agent-one/zone/auth/README.md"}, &wire.Report{Status: "DONE"}, true, "x")
	if v.Word != "pass" {
		t.Fatalf("a granted claim failed its agent's gate: %s", v)
	}
}

// A covering claim takes the soil under it: zone-auth claiming src/ would cover zone-api's
// src/api/ and its parent's — held back like a longer prefix. Open ground stays an agent's to claim.
func TestSelfClaimCoveringARankMateIsHeld(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	os.MkdirAll(filepath.Join(dir, "lib"), 0o755)
	auth := filepath.Join(dir, ".agent-one/zone/auth/README.md")
	os.WriteFile(auth, []byte("# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`, `src/`, `lib/`\n- **Reports to:** domain-security\n- **Verify:** `test ! -f src/auth/BROKEN`\n"), 0o644)
	pol := w.ToolPolicy("zone-auth", OwnershipOptions{Enabled: true})
	if err := pol(writeAccess(dir, "src/api/server.go")); err == nil {
		t.Fatal("a covering self-claim admitted a rank-mate's soil")
	}
	if err := pol(writeAccess(dir, "lib/x.go")); err != nil {
		t.Fatalf("a self-claim on open ground was refused: %v", err)
	}
}
