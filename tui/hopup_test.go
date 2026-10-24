package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// A Subagent that lands ESCALATE, FAIL or DENIED prints one loud line under its card, addressed
// one hop up with its why, and the sidebar's landed row holds the verdict, never a bare DONE;
// a clean landing prints none.
func TestLoudVerdictSpeaksOneHopUp(t *testing.T) {
	for _, tc := range []struct {
		exit, status, want string
		loud               bool
	}{
		{"ESCALATE", "ESCALATE — the gate's key", "▲ ESCALATE", true},
		{"FAIL", "FAIL", "✗ FAIL", true},
		{"", "DENIED: outward write", "⊘ DENIED", true},
		{"PASSED", "DONE", "", false},
	} {
		m := New(&fakeHost{}, NewTheme(false), DefaultWords())
		m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "x"}})
		rep := &ReportView{Status: tc.status}
		if tc.exit == "ESCALATE" {
			rep.Holes = []string{"the token store needs the gate's key"}
		}
		m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", State: "done", Exit: tc.exit, Up: "domain-auth", Elapsed: time.Second, Report: rep}})
		m.pulses["zone-auth"].doneAt = time.Now().Add(-flashHold)
		m.Update(evTick{at: time.Now()})
		var loud []string
		for _, b := range m.blocks {
			if s := ansi.Strip(b.render(100)); strings.Contains(s, "zone-auth → domain-auth") {
				loud = append(loud, s)
			}
		}
		var row *RailSubagent
		for _, c := range m.railView(40).Subagents {
			if c.Name == "zone-auth" {
				c := c
				row = &c
			}
		}
		if row == nil || (tc.loud && (!row.Failed || row.State == "done" || row.Hop != (tc.exit == "ESCALATE"))) || (!tc.loud && row.Failed) {
			t.Fatalf("%s: the sidebar's landed row must hold the verdict, got %+v", tc.status, row)
		}
		if !tc.loud {
			if len(loud) != 0 {
				t.Fatalf("a clean landing spoke up: %q", loud)
			}
			continue
		}
		if len(loud) != 1 || !strings.HasPrefix(strings.TrimSpace(loud[0]), tc.want) || strings.Contains(loud[0], "\n") {
			t.Fatalf("%s: want one line %q, got %q", tc.status, tc.want, loud)
		}
		if tc.exit == "ESCALATE" && !strings.Contains(loud[0], "the token store needs the gate's key") {
			t.Fatalf("the escalation lost its why: %q", loud[0])
		}
		if tc.exit == "" && !strings.Contains(loud[0], "outward write") {
			t.Fatalf("the status's why is the line's when no hole says one: %q", loud[0])
		}
	}
}
