package app

import (
	"os"
	"testing"
	"time"
)

// railPlans merges live + archived decks: live first in name order, then the archive's decks
// with Archived=true so the rail renders them and 'c' can snap off them (tui/blocks.go:597).
func TestPlanRailMergeArchived(t *testing.T) {
	dir := t.TempDir()
	boot := time.Now()
	a := &App{Root: dir, Cfg: mustDefaults(t), planBoot: boot}
	a.Subagent = newSubagent(a)
	pdir := a.planDir()
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}

	writePlanFile(t, pdir, "zone-a", boot.Add(time.Hour), false)
	// pen the twin by hand into the archive (append-only path) — same name, older stamp,
	// deduped against the live deck so the rail shows it once, as live truth
	archive := planArchiveDir(pdir)
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	writePlanFile(t, archive, "zone-a", boot.Add(-time.Hour), true)
	writePlanFile(t, archive, "domain-host", boot.Add(-2*time.Hour), true)
	writePlanFile(t, archive, "coord-agent-one", boot.Add(-time.Hour), false)

	plans := railPlans(a, pdir, boot)
	if len(plans) != 3 {
		t.Fatalf("railPlans = %d plans, want 3 (live zone-a + archived coord-agent-one + domain-host)", len(plans))
	}
	// live first, in name order, then archive in name order; the archived zone-a twin is deduped
	wantOrder := []struct {
		rank     string
		archived bool
	}{
		{"zone", false}, // railPlans carves the rank's kind at the first '-'
		{"coord", true},
		{"domain", true},
	}
	for i, w := range wantOrder {
		if plans[i].Rank != w.rank {
			t.Errorf("plans[%d].Rank = %q, want %q", i, plans[i].Rank, w.rank)
		}
		if plans[i].Archived != w.archived {
			t.Errorf("plans[%d](%s).Archived = %v, want %v", i, plans[i].Rank, plans[i].Archived, w.archived)
		}
	}
	// the deduped entry is the live one (not the archived twin): live zone-a is post-boot → not stale
	if plans[0].Stale {
		t.Errorf("live zone-a flagged stale — the archived twin must not shadow the live deck")
	}
	if !plans[1].Stale || !plans[2].Stale {
		t.Errorf("archived pre-boot decks must render stale: %+v", plans[1:])
	}
	// items ride through from the archive
	if len(plans[1].Items) != 1 || plans[1].Items[0].Text != "sealed card" {
		t.Errorf("archived deck items not rendered: %+v", plans[1].Items)
	}
}

func TestPlanRailMergeNoArchiveDir(t *testing.T) {
	dir := t.TempDir()
	boot := time.Now()
	a := &App{Root: dir, Cfg: mustDefaults(t), planBoot: boot}
	a.Subagent = newSubagent(a)
	pdir := a.planDir()
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePlanFile(t, pdir, "zone-a", boot, false)

	plans := railPlans(a, pdir, boot)
	if len(plans) != 1 || plans[0].Rank != "zone" || plans[0].Archived {
		t.Fatalf("railPlans without an archive dir: %+v", plans)
	}
}
