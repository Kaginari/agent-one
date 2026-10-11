package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanDeckLifecycle(t *testing.T) {
	dir := t.TempDir()
	if got := planRead(dir, "coord-team"); len(got.Items) != 0 {
		t.Fatal("an unread deck is empty")
	}
	d := planDeck{Agent: "coord-team", Items: []planItem{
		{ID: "1", Text: "ask domain-engine for the runner slice", Status: "in_progress", Who: "domain-engine"},
		{ID: "2", Text: "ask domain-db for the store slice", Status: "pending", Who: "domain-db"},
	}}
	if err := planWrite(dir, d); err != nil {
		t.Fatal(err)
	}
	got := planRead(dir, "coord-team")
	if len(got.Items) != 2 || got.Items[1].Who != "domain-db" || got.Updated == "" {
		t.Fatalf("the deck round-trips: %+v", got)
	}
	if !strings.Contains(renderPlan(got), "in_progress") || !strings.Contains(renderPlan(got), "domain-engine") {
		t.Fatalf("the render carries status and ownership:\n%s", renderPlan(got))
	}
	// the file is the deck: on disk, machine-local, json-readable
	if b, err := os.ReadFile(planFile(dir, "coord-team")); err != nil || !strings.Contains(string(b), "domain-db") {
		t.Fatalf("the file is the truth: %v", err)
	} else {
		var back planDeck
		if json.Unmarshal(b, &back) != nil || back.Agent != "coord-team" {
			t.Fatal("the file decodes")
		}
	}
	// a slash in an agent name never leaves the dir (a dispatch id can look like one)
	p := planFile(dir, "domain/engine")
	if filepath.Dir(p) != dir || strings.Contains(filepath.Base(p), "/") {
		t.Fatalf("the deck stays machine-local and flat: %s", p)
	}
}

// An earlier session's deck is memory, never a gate-holder nor a live rail row: the stale
// orchestrator deck of 2026-10-02 must not pose as current work after the 2026-10-03 relaunch, and
// re-penning it this session makes it current again (the 'continue on last plan' lifeline).
func TestPlanSessionSemantics(t *testing.T) {
	dir := t.TempDir()
	boot := time.Now()
	old := planDeck{Agent: "orchestrator", Items: []planItem{{ID: "1", Text: "yesterday's item", Status: "pending"}}}
	if err := planWrite(dir, old); err != nil {
		t.Fatal(err)
	}
	// age the deck to before the boot
	var back planDeck
	b, _ := os.ReadFile(planFile(dir, "orchestrator"))
	_ = json.Unmarshal(b, &back)
	back.Updated = boot.Add(-2 * time.Hour).UTC().Format("2006-01-02T15:04:05Z")
	aged, _ := json.Marshal(back)
	if err := os.WriteFile(planFile(dir, "orchestrator"), aged, 0o644); err != nil {
		t.Fatal(err)
	}

	d := planRead(dir, "orchestrator")
	if !planStale(d, boot) && planUpdatedAt2(d) {
		t.Fatal("the aged deck must read stale")
	}
	if r := planOpenReason(dir, "orchestrator", boot); r != "" {
		t.Fatalf("a stale deck must never hold the gate: %q", r)
	}
	if got := renderPlanWith(d, boot); !strings.Contains(got, "stale deck") {
		t.Fatalf("a stale deck holds its banner:\n%s", got)
	}

	// continue on it: re-pen this session → current again, gate live again
	again := planDeck{Agent: "orchestrator", Items: []planItem{{ID: "1", Text: "yesterday's item, continued", Status: "in_progress"}}}
	if err := planWrite(dir, again); err != nil {
		t.Fatal(err)
	}
	d = planRead(dir, "orchestrator")
	if planStale(d, boot) {
		t.Fatal("a deck re-penned this session is current")
	}
	if r := planOpenReason(dir, "orchestrator", boot); !strings.Contains(r, "in_progress") {
		t.Fatalf("the gate reads the current deck: %q", r)
	}
	if got := renderPlanWith(d, boot); strings.Contains(got, "stale deck") {
		t.Fatalf("a current deck carries no banner:\n%s", got)
	}

	// unparseable stamp → fail-open (never a phantom gate, never a hidden deck)
	back.Updated = "not-a-date"
	fixed, _ := json.Marshal(back)
	if err := os.WriteFile(planFile(dir, "orchestrator"), fixed, 0o644); err != nil {
		t.Fatal(err)
	}
	d = planRead(dir, "orchestrator")
	if planStale(d, boot) {
		t.Fatal("an unparsable stamp is not stale — fail-open")
	}
}

func planUpdatedAt2(d planDeck) bool { t, ok := planUpdatedAt(d); return ok && t.Before(time.Now()) }

// scenario-3-2-1 on the deck: scored cards render with their evidence; a naked score names
// its own hole (a number without rows is decoration, the wire's honesty speaks).
func TestPlanScoreCards(t *testing.T) {
	dir := t.TempDir()
	d := planDeck{Agent: "orchestrator", Items: []planItem{
		{ID: "1", Text: "slice A", Status: "in_progress", Score: "0.9", Why: "zone quiet 14d, last commission DONE"},
		{ID: "2", Text: "slice B", Status: "pending", Score: "0.4"}, // naked
		{ID: "3", Text: "slice C", Status: "pending"},
	}}
	if err := planWrite(dir, d); err != nil {
		t.Fatal(err)
	}
	got := planRead(dir, "orchestrator")
	if got.Items[0].Score != "0.9" || got.Items[0].Why == "" {
		t.Fatalf("score fields round-trip the file: %+v", got.Items[0])
	}
	out := renderPlan(got)
	if !strings.Contains(out, "≈ 0.9 (zone quiet 14d") {
		t.Fatalf("scored card renders its why:\n%s", out)
	}
	if !strings.Contains(out, "@? why missing") {
		t.Fatalf("a naked score names its hole:\n%s", out)
	}
}
