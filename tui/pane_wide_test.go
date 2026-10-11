package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The wide-window reproducer (the two paste dumps, 2026-10-02): ~210×50 terminal, a busy turn
// with long prose streaming, tools landing in the scrollback, subagents pulsing, an ask card
// floating. POLICY: every frame row must be exactly m.width cells and the divider must sit at
// column cw on every pane row — anything else is the smear in the paste.
func TestPaneWideWindowColumns(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 210, Height: 50})
	if m.railVisible() == 0 {
		t.Fatal("210×50 must carry the sidebar")
	}
	cw := 210 - m.railVisible()

	m.begin()
	prose := "Yo. Deck's unchanged — files-panel wave ready to launch, Go shim in hand. Give me an aim and I move.\n\n• Build is green.\n• The repair is riding as working-tree changes.\n\n"
	m.stream.WriteString(prose)
	m.Update(EvSubagent{Subagent: SubagentView{Name: "coord-agent-one", Rank: "coord", Role: "drafter", Model: "glm-latest", State: "thinking", Ask: "coordinate the files-panel slices"}})
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "glm-latest", State: "tool", Ask: "land the token file"}})

	check := func(when string, mask func(i int) bool) {
		f := m.Render()
		rows := strings.Split(f, "\n")
		if len(rows) != 50 {
			t.Fatalf("%s: frame is %d rows, want 50", when, len(rows))
		}
		for i, l := range rows {
			if w := ansi.StringWidth(l); w != 210 {
				t.Fatalf("%s row %d: width %d, want 210: %q", when, i, w, ansi.Strip(l))
			}
		}
		// the pane edge is "░▒│": two shadow columns, then the divider at column cw+2
		strip := ansi.Strip(f)
		sr := strings.Split(strip, "\n")
		for i := 0; i < len(sr)-4; i++ {
			if mask != nil && mask(i) {
				continue
			}
			r := []rune(sr[i])
			if len(r) < cw+3 {
				t.Fatalf("%s row %d shorter than cw+3: %q", when, i, sr[i])
			}
			if r[cw] != '░' || r[cw+1] != '▒' {
				t.Fatalf("%s row %d: shadow missing at cols %d-%d, found %q%q in %q", when, i, cw, cw+1, r[cw], r[cw+1], sr[i])
			}
			if r[cw+2] != '│' {
				t.Fatalf("%s row %d: divider missing at col %d, found %q in %q", when, i, cw+2, r[cw+2], sr[i])
			}
		}
	}
	check("busy subagents + prose", nil)

	// tools landing: scrollback prints interleave while the frame stands
	m.Update(EvToolStart{Tool: ToolView{ID: "x1", Name: "bash", Summary: "go build ./...", Class: "read", Status: "running"}})
	check("tool running", nil)
	m.flushStream(true) // a print between frames
	check("after a scrollback print", nil)

	// the ask card floats at width too — its rows where it covers the divider are masked
	m.choice = &EvChoice{View: ChoiceView{Title: "Question", Why: "What do you want to run — wait for the subagent reports, or is there new work?", Options: []string{"wait", "new work"}}}
	m.choiceAt = time.Now().Add(-time.Second)
	box := m.theme.Choice(m.view, min(210*3/5, 100))
	bw := lipgloss.Width(box)
	bh := strings.Count(box, "\n") + 1
	x, y := centerRect(210, 50, bw, bh)
	inCard := func(i int) bool { return i >= y-1 && i <= y+bh && x+1 < cw } // rows the card may cover including the divider col
	check("choice floating", inCard)
	m.choice = nil

	// the invariant at the rail edge windows
	for _, size := range [][2]int{{96, 30}, {120, 26}, {240, 80}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		f := m.Render()
		if got := strings.Count(f, "\n") + 1; got != size[1] {
			t.Fatalf("at %dx%d: frame is %d rows, want %d", size[0], size[1], got, size[1])
		}
		for i, l := range strings.Split(f, "\n") {
			if w := ansi.StringWidth(l); w != size[0] {
				t.Fatalf("at %dx%d row %d: width %d: %q", size[0], size[1], i, w, ansi.Strip(l))
			}
		}
	}
}
