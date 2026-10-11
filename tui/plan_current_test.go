package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The 'c — current' switch: a stale deck stays browsable, the banner names the hint, and
// pressing c walks the view to the current deck (the orchestrator default the coordinator pick uses at
// model.go:392-400). A live view keeps 'c' for typing; another stale view switches back.
func TestPlanCurrentSwitch(t *testing.T) {
	host := &fakeHost{plans: []RailPlan{
		{Agent: "zone-tui-board", Rank: "zone", Role: "p2", Stale: true,
			Items: []RailPlanItem{{Text: "yesterday's slice", Status: "pending"}}},
		{Agent: "coord-team", Rank: "coord", Role: "p1",
			Items: []RailPlanItem{{Text: "slice now", Status: "in_progress"}}},
		{Agent: "orchestrator", Rank: "orchestrator", Role: "p1",
			Items: []RailPlanItem{{Text: "the orchestrator's slice", Status: "in_progress"}}},
	}}
	m := New(host, NewTheme(true), DefaultWords())
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	if got, ok := mm.(*Model); ok {
		m = got
	}
	m.Update(evTick{at: time.Now()}) // presence refresh populates m.plans (model.go:385)
	if len(m.plans) == 0 {
		t.Fatal("evTick must populate m.plans — railVisible>0 not satisfied?")
	}
	if got := m.plans[m.planIdx]; got.Agent != "orchestrator" || got.Archived {
		t.Fatalf("the orchestrator deck leads by default: %+v", m.plans[m.planIdx])
	}

	// browse the stale deck: ctrl+p cycles the rail (input.go:45)
	m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}) // orchestrator(idx 2) → zone-tui-board(idx 0) [wrap]
	if got := m.plans[m.planIdx]; got.Agent != "zone-tui-board" || !got.Stale {
		t.Fatalf("the stale deck is now in view: %+v", got)
	}
	view := ansi.Strip(m.Render())
	if !strings.Contains(view, "yesterday's slice") {
		t.Fatal("the stale deck stays browsable")
	}
	if strings.Contains(view, "the orchestrator's slice") {
		t.Fatal("the live deck is not the view")
	}
	if !strings.Contains(view, "— current") && !strings.Contains(view, "— ᴄᴜʀʀᴇɴᴛ") {
		t.Fatalf("the stale banner names the hint:\n%s", view)
	}

	// 'c' switches to the current deck and frees the sticky pick
	m.planSticky = true // the 'c' chord requires a deliberate pin (2026-10-13 sticky gate)
	m.key(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.planSticky {
		t.Fatal("the switch hands the default back")
	}
	if got := m.plans[m.planIdx]; got.Agent != "orchestrator" || got.Archived {
		t.Fatalf("the current deck is in view: %+v", got)
	}
	if !strings.Contains(ansi.Strip(m.Render()), "the orchestrator's slice") {
		t.Fatal("the current deck renders")
	}

	// a live view keeps 'c' for typing
	m.key(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.input.Value() != "c" {
		t.Fatalf("plain c keeps typing on a live view: %q", m.input.Value())
	}
	m.setInput("")

	// and from yet another stale view the same key walks back to that live one
	m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if !m.plans[m.planIdx].Stale {
		t.Fatalf("setup: second stale view wanted, got %+v", m.plans[m.planIdx])
	}
	m.key(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if got := m.plans[m.planIdx]; got.Agent != "orchestrator" {
		t.Fatalf("the switch is not per-deck: %s", got.Agent)
	}

	// an archived deck answers 'c' the same way — it is a stale view once removed
	// (the archived pin rides the Plans-host side; here we assert the switch on it).
	// Flip the stale zone worker deck to archived, put the view back on it, press c → orchestrator.
	m.plans[0].Archived = true
	m.planIdx = 0
	m.planSticky = true // the 'c' chord requires a deliberate pin (2026-10-13 sticky gate)
	m.key(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if got := m.plans[m.planIdx]; got.Agent != "orchestrator" {
		t.Fatalf("an archived view switches too: %s", got.Agent)
	}
}

// 2026-10-13 relay (orchestrator→coord-agent-one→domain-host, Card A): a session booting onto a STALE
// default deck must never eat plain 'c' runes — the chord requires a deliberate sticky pin.
func TestPlanCurrentStaleDefaultTypesC(t *testing.T) {
	host := &fakeHost{plans: []RailPlan{
		{Agent: "orchestrator", Rank: "orchestrator", Role: "p1", Stale: true,
			Items: []RailPlanItem{{Text: "last session's intent", Status: "pending"}}},
	}}
	m := New(host, NewTheme(true), DefaultWords())
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	if got, ok := mm.(*Model); ok {
		m = got
	}
	m.Update(evTick{at: time.Now()})
	if len(m.plans) == 0 {
		t.Fatal("evTick must populate m.plans — railVisible>0 not satisfied?")
	}
	if got := m.plans[m.planIdx]; !got.Stale {
		t.Fatalf("setup: the only deck is stale, the default view faces it (fallback preserved): %+v", got)
	}
	if m.planSticky {
		t.Fatal("setup: the boot-default view is not a deliberate pin")
	}

	// every lowercase 'c' press types; nothing snaps, no banner fires
	for want := 1; want <= 3; want++ {
		m.key(tea.KeyPressMsg{Code: 'c', Text: "c"})
		if got := m.input.Value(); got != strings.Repeat("c", want) {
			t.Fatalf("press %d: plain c falls through to the input line on a boot-default stale view: %q", want, got)
		}
	}
	m.setInput("")
}

// 2026-10-13 relay: snapping from a stale view whose session has NO live deck says so and
// swallows nothing further — the snap target loops never land on another stale deck.
func TestPlanCurrentNoLiveDeck(t *testing.T) {
	host := &fakeHost{plans: []RailPlan{
		{Agent: "zone-a", Rank: "zone", Stale: true, Archived: true,
			Items: []RailPlanItem{{Text: "stale slice a", Status: "pending"}}},
		{Agent: "coord-b", Rank: "coord", Archived: true,
			Items: []RailPlanItem{{Text: "archived slice b", Status: "pending"}}},
	}}
	m := New(host, NewTheme(true), DefaultWords())
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	if got, ok := mm.(*Model); ok {
		m = got
	}
	m.Update(evTick{at: time.Now()})
	if len(m.plans) == 0 {
		t.Fatal("evTick must populate m.plans — railVisible>0 not satisfied?")
	}

	// deliberately pin the stale deck, then press 'c': the sticky chord fires…
	m.key(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}) // flips planSticky on
	m.planIdx = 0                                       // view the stale deck itself
	if !m.planSticky || !m.plans[m.planIdx].Stale {
		t.Fatalf("setup: ctrl+p pins the stale deck, got sticky=%v deck=%+v", m.planSticky, m.plans[m.planIdx])
	}
	before := m.planIdx
	m.key(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.planIdx != before {
		t.Fatalf("no live deck exists: the view stays put, got %d (was %d)", m.planIdx, before)
	}
	if m.message != "plan · no live deck" {
		t.Fatalf("the snap says 'plan · no live deck' instead of looping onto another stale deck, got %q", m.message)
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("the sticky chord still swallows the rune (banner, not typing): %q", got)
	}
	// …and the pin dropped, so the NEXT plain 'c' types — the loop is dead
	m.key(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if got := m.input.Value(); got != "c" {
		t.Fatalf("after the failed snap planSticky reset, plain c types: %q", got)
	}
	m.setInput("")
}

// 2026-10-13 relay: shift+C is the always-works escape hatch — a stale view, NOT sticky,
// snaps to the live orchestrator deck with the distinct uppercase banner.
func TestPlanCurrentShiftC(t *testing.T) {
	host := &fakeHost{plans: []RailPlan{
		{Agent: "zone-tui-board", Rank: "zone", Stale: true,
			Items: []RailPlanItem{{Text: "yesterday's slice", Status: "pending"}}},
		{Agent: "orchestrator", Rank: "orchestrator",
			Items: []RailPlanItem{{Text: "the orchestrator's slice", Status: "in_progress"}}},
	}}
	m := New(host, NewTheme(true), DefaultWords())
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	if got, ok := mm.(*Model); ok {
		m = got
	}
	m.Update(evTick{at: time.Now()})
	if len(m.plans) == 0 {
		t.Fatal("evTick must populate m.plans — railVisible>0 not satisfied?")
	}
	m.planIdx = 0 // view the stale zone worker deck WITHOUT a sticky pin (boot-style view)
	m.planSticky = false
	if !m.plans[m.planIdx].Stale {
		t.Fatalf("setup: stale deck wanted at idx 0, got %+v", m.plans[m.planIdx])
	}

	m.key(tea.KeyPressMsg{Code: 'C', Text: "C", Mod: tea.ModShift})
	if got := m.plans[m.planIdx]; got.Agent != "orchestrator" || got.Stale || got.Archived {
		t.Fatalf("shift+C snaps to the live orchestrator deck from any view: %+v", got)
	}
	liveBanner := "plan · orchestrator (2/2) · current"
	if m.message != liveBanner {
		t.Fatalf("the shift+C banner fires on a non-sticky view (distinct from the chord-gated path): want %q, got %q", liveBanner, m.message)
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("shift+C never types: %q", got)
	}
}
