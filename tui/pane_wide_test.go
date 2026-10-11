package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The wide-window reproducer (the two paste dumps, 2026-10-02): ~210×50 terminal, a busy turn
// with long prose streaming, tools landing in the scrollback, subagents pulsing, an ask card
// floating. POLICY: every frame row must be exactly m.width cells and the frame's grid must stay
// closed (checkFrame: the divider one column all the way down) — anything else is the smear in
// the paste.
func TestPaneWideWindowColumns(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 210, Height: 50})
	if m.railVisible() == 0 {
		t.Fatal("210×50 must carry the sidebar")
	}

	m.begin()
	prose := "Yo. Deck's unchanged — files-panel wave ready to launch, Go shim in hand. Give me an aim and I move.\n\n• Build is green.\n• The repair is riding as working-tree changes.\n\n"
	m.stream.WriteString(prose)
	m.Update(EvSubagent{Subagent: SubagentView{Name: "coord-agent-one", Rank: "coord", Role: "drafter", Model: "glm-latest", State: "thinking", Ask: "coordinate the files-panel slices"}})
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "glm-latest", State: "tool", Ask: "land the token file"}})

	checkFrame(t, m, "busy subagents + prose")

	// tools landing: blocks land in the transcript while the frame stands
	m.Update(EvToolStart{Tool: ToolView{ID: "x1", Name: "bash", Summary: "go build ./...", Class: "read", Status: "running"}})
	checkFrame(t, m, "tool running")
	m.flushStream(true)
	checkFrame(t, m, "after a landed block")

	// the ask card floats at width too: the rows and widths hold under it
	m.choice = &EvChoice{View: ChoiceView{Title: "Question", Why: "What do you want to run — wait for the subagent reports, or is there new work?", Options: []string{"wait", "new work"}}}
	m.choiceAt = time.Now().Add(-time.Second)
	f := m.Render()
	if got := strings.Count(f, "\n") + 1; got != 50 {
		t.Fatalf("choice floating: frame is %d rows, want 50", got)
	}
	for i, l := range strings.Split(f, "\n") {
		if w := ansi.StringWidth(l); w != 210 {
			t.Fatalf("choice floating row %d: width %d", i, w)
		}
	}
	m.choice = nil

	// the invariant at the rail edge windows
	for _, size := range [][2]int{{96, 30}, {120, 26}, {240, 80}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		checkFrame(t, m, fmt.Sprintf("%dx%d", size[0], size[1]))
	}
}
