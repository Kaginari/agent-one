package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tierOf(t *testing.T, r *ReviewResult, name string) ReviewTier {
	t.Helper()
	for _, tier := range r.Tiers {
		if tier.Name == name {
			return tier
		}
	}
	t.Fatalf("no tier %q in %v", name, r.Tiers)
	return ReviewTier{}
}

func findingsContain(tier ReviewTier, signal, route string) bool {
	for _, f := range tier.Findings {
		if strings.Contains(f.Signal, signal) && strings.Contains(f.Route, route) {
			return true
		}
	}
	return false
}

// TestReviewTiers: each tier must be read with its own lens — the short tier must name the
// cross-agent question stem (attention), the shared tier the restated lesson (lessons), and a
// workspace with no long index must say so out loud with the friction family it finds in the
// episode stream (genomic). Every finding must carry its evolution route.
func TestReviewTiers(t *testing.T) {
	workspace, home := fixture(t)
	w, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	w.Home = home

	shortDir := filepath.Join(workspace, ".agent-one", "memory", "short")
	os.MkdirAll(shortDir, 0o755)
	os.WriteFile(filepath.Join(shortDir, "orchestrator.jsonl"), []byte(
		`{"q":"how does the dispatch gate rank the flow","results":[{"src":".agent-one/AGENT-ONE.md"}]}`+"\n"+
			`{"q":"how does the dispatch gate rank the flow","results":[{"src":".agent-one/log.md"},{"src":".agent-one/memory/shared/notes.jsonl"}]}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(shortDir, "coord-agent-one.jsonl"), []byte(
		`{"q":"how does the dispatch gate rank the flow","results":[{"src":".agent-one/AGENT-ONE.md"}]}`+"\n"), 0o644)

	sharedDir := filepath.Join(workspace, ".agent-one", "memory", "shared")
	os.MkdirAll(sharedDir, 0o755)
	dup := `{"at":"2026-10-02T00:00:00.000Z","by":"orchestrator","kind":"policy","text":"reindex last, after the writing wave lands, never mid-wave"}` + "\n"
	os.WriteFile(filepath.Join(sharedDir, "notes.jsonl"), []byte(
		dup+strings.Replace(dup, "orchestrator", "coord-agent-one", 1)), 0o644)

	log := filepath.Join(workspace, ".agent-one", "log.md")
	raw, _ := os.ReadFile(log)
	os.WriteFile(log, []byte(string(raw)+"\n- **Maintenance:** ⚙️ echo from before the policy note\n"), 0o644)

	r := w.Review()
	if len(r.Tiers) != 4 {
		t.Fatalf("tiers = %d, want 4", len(r.Tiers))
	}

	short := tierOf(t, r, "short — attention")
	if !findingsContain(short, "cross-agent question stem", "[provisioning]") {
		t.Fatalf("short tier missed the cross-agent stem or its provisioning route: %+v", short.Findings)
	}
	if !findingsContain(short, "junk-anchor", "[harness]") {
		t.Fatalf("short tier missed the junk share: %+v", short.Findings)
	}

	shared := tierOf(t, r, "shared — lessons")
	if !findingsContain(shared, "lesson restated", "[policy]") {
		t.Fatalf("shared tier missed the restated lesson: %+v", shared.Findings)
	}

	long := tierOf(t, r, "long — genomic")
	if !findingsContain(long, "no long-term index", "[harness]") {
		t.Fatalf("long tier missed the absent index: %+v", long.Findings)
	}
	if !findingsContain(long, "friction family maintenance-marker", "") {
		t.Fatalf("long tier missed the marker family in log.md: %+v", long.Findings)
	}

	value := tierOf(t, r, "value — decisions")
	if !findingsContain(value, "no plan-outcome rows yet", "[harness]") {
		t.Fatalf("value tier must say the learning loop is empty Ground Zero, got: %+v", value.Findings)
	}
}
