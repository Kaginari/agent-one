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

// persistentWorkspace is the fixture with a workspace-persistent zone worker whose Ownership claims `.agent-one` whole —
// the broad claim every seat, shelf and scratch path sits under.
func persistentWorkspace(t *testing.T) (string, *Workspace) {
	t.Helper()
	dir := fixture(t)
	if err := os.MkdirAll(filepath.Join(dir, ".agent-one/zone/persistent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agent-one/zone/persistent/README.md"), []byte("# zone-persistent\n\n- **Rank:** Zone worker\n- **Owns:** `.agent-one`\n- **Reports to:** domain-security\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, open(t, dir)
}

// ZoneOfPath is one reading for every ownership question: the harness's shelves and the scratch
// are nobody's, a seat is its member's even under a broader claim, then ownership.
func TestZoneOfPath(t *testing.T) {
	_, w := persistentWorkspace(t)
	for _, c := range []struct {
		rel, owner string
		soil       Soil
	}{
		{".agent-one/shared/queue.jsonl", "", SoilHarness},
		{".agent-one/instruments/loop/r1.jsonl", "", SoilHarness},
		{".agent-one/tmp/scratch/r1/x.txt", "", SoilScratch},
		{".agent-one/tmp/plans/zone-auth.json", "", SoilScratch},
		{".agent-one/zone/auth/README.md", "zone-auth", SoilSeat},
		{".agent-one/zone/auth/notes.md", "zone-auth", SoilSeat},
		{".agent-one/log.md", "zone-persistent", SoilOwnership},
		{"src/auth/login.go", "zone-auth", SoilOwnership},
		{"README.md", "", SoilOpen},
	} {
		z := w.ZoneOfPath(c.rel)
		got := ""
		if z.Member != nil {
			got = z.Member.Name
		}
		if z.Soil != c.soil || got != c.owner {
			t.Errorf("%s: %s/%q, want %s/%q", c.rel, z.Soil, got, c.soil, c.owner)
		}
		if o := w.Owner(c.rel); (o == nil) != (c.owner == "") || (o != nil && o.Name != c.owner) {
			t.Errorf("Owner(%s) disagrees with ZoneOfPath: %v", c.rel, o)
		}
	}
}

// A member's seat leaves the pairing set: its README and seat files never bill the agent whose
// claim covers `.agent-one`; the scratch is the writer's own nest; the shelves bill no member.
func TestGateSeatAndScratchSoil(t *testing.T) {
	_, w := persistentWorkspace(t)
	g := DefaultHooks().Gate
	rep := &wire.Report{Status: "DONE"}
	gate := func(as string, wrote ...string) Verdict {
		t.Helper()
		return w.Gate(context.Background(), nil, g, as, wrote, rep, true, "x")
	}
	for _, rel := range []string{".agent-one/zone/auth/README.md", ".agent-one/zone/auth/notes.md", ".agent-one/tmp/scratch/r1/x.txt", ".agent-one/tmp/plans/zone-auth.json"} {
		if v := gate("zone-auth", rel); v.Word != "pass" {
			t.Errorf("zone-auth writing %s: %s", rel, v)
		}
	}
	if v := gate("zone-auth", ".agent-one/tmp/plans/zone-api.json"); v.Word != "fail" || !strings.Contains(v.String(), "is zone-api's plan deck, written by zone-auth") {
		t.Errorf("another agent's deck: %s", v)
	}
	if v := gate("zone-auth", ".agent-one/shared/queue.jsonl"); strings.Contains(v.String(), "zone-persistent") {
		t.Errorf("the shared shelf billed the persistent agent: %s", v)
	}
	if v := gate("zone-api", ".agent-one/zone/auth/notes.md"); v.Word != "fail" || !strings.Contains(v.String(), "belongs to zone-auth, written by zone-api") {
		t.Errorf("a foreign agent on another's seat: %s", v)
	}
}

// The ownership policy admits the scratch to every agent — its own nest — but never another
// agent's plan deck.
func TestPolicyScratchNest(t *testing.T) {
	dir, w := persistentWorkspace(t)
	pol := w.ToolPolicy("zone-api", OwnershipOptions{Enabled: true})
	acc := func(rel string) tool.Access {
		return tool.Access{Class: tool.Write, Paths: []string{filepath.Join(dir, filepath.FromSlash(rel))}}
	}
	if err := pol(acc(".agent-one/tmp/scratch/r1/out.txt")); err != nil {
		t.Errorf("own scratch refused: %v", err)
	}
	if err := pol(acc(".agent-one/tmp/plans/zone-api.json")); err != nil {
		t.Errorf("own deck refused: %v", err)
	}
	if err := pol(acc(".agent-one/tmp/plans/zone-auth.json")); err == nil || !strings.Contains(err.Error(), "zone-auth's plan deck") {
		t.Errorf("another agent's deck: %v", err)
	}
	if err := pol(acc("src/auth/login.go")); err == nil {
		t.Error("foreign ownership admitted")
	}
}
