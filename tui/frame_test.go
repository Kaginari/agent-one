package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// checkFrame holds a pane-mode frame to the frame's policies: exactly the window's rows, each
// exactly the window's width; the headroom row blank; one closed box — rounded corners, every
// row opened and closed by a border glyph; the divider one column all the way down, opened by ┬
// and closed by ┴; the input inside the main column only (never under the sidebar).
func checkFrame(t *testing.T, m *Model, when string) {
	t.Helper()
	f := m.Render()
	l := m.lay
	if !l.pane {
		t.Fatalf("%s: %dx%d is not in pane mode", when, m.width, m.height)
	}
	rows := strings.Split(f, "\n")
	if len(rows) != m.height {
		t.Fatalf("%s: frame is %d rows, want the window's %d", when, len(rows), m.height)
	}
	for i, r := range rows {
		if w := ansi.StringWidth(r); w != m.width {
			t.Fatalf("%s: row %d is %d cells, want %d: %q", when, i, w, m.width, ansi.Strip(r))
		}
	}
	strip := strings.Split(ansi.Strip(f), "\n")
	cells := make([][]rune, len(strip))
	for i, s := range strip {
		cells[i] = []rune(s)
		if len(cells[i]) != m.width {
			t.Fatalf("%s: row %d has %d runes (a wide glyph?): %q", when, i, len(cells[i]), s)
		}
	}
	if m.choiceFloat() {
		return // the modal floats over every region: the rectangle is the policy under it
	}
	if strings.TrimSpace(strip[0]) != "" {
		t.Fatalf("%s: the headroom row carries content: %q", when, strip[0])
	}
	at := func(y, x int) rune { return cells[y][x] }
	last := m.width - 1
	if at(l.topY, 0) != '╭' || at(l.topY, last) != '╮' || at(l.bottomY, 0) != '╰' || at(l.bottomY, last) != '╯' {
		t.Fatalf("%s: the frame's corners are open:\n%s", when, strings.Join(strip, "\n"))
	}
	for y := l.topY + 1; y < l.bottomY; y++ {
		if !strings.ContainsRune("│├", at(y, 0)) {
			t.Fatalf("%s: row %d opens with %q, not a border: %q", when, y, at(y, 0), strip[y])
		}
		if !strings.ContainsRune("│┤▲▼", at(y, last)) {
			t.Fatalf("%s: row %d closes with %q, not a border: %q", when, y, at(y, last), strip[y])
		}
	}
	for _, y := range []int{l.sepY, l.inputSepY, l.footerSepY} {
		if at(y, 0) != '├' {
			t.Fatalf("%s: rule row %d must open with ├: %q", when, y, strip[y])
		}
	}
	if l.rc > 0 {
		if at(l.sepY, l.divX) != '┬' || at(l.footerSepY, l.divX) != '┴' {
			t.Fatalf("%s: the divider must open with ┬ and close with ┴ at col %d", when, l.divX)
		}
		for y := l.sepY + 1; y < l.footerSepY; y++ {
			if !strings.ContainsRune("│├┤┼", at(y, l.divX)) {
				t.Fatalf("%s: row %d: divider col %d is %q: %q", when, y, l.divX, at(y, l.divX), strip[y])
			}
		}
		if !strings.ContainsRune("┤┼", at(l.inputSepY, l.divX)) {
			t.Fatalf("%s: the input's rule must stop at the divider (┤ or ┼), got %q", when, at(l.inputSepY, l.divX))
		}
		if l.input.X+l.input.W >= l.divX {
			t.Fatalf("%s: the input reaches under the sidebar: input ends at %d, divider at %d", when, l.input.X+l.input.W, l.divX)
		}
		// the ask's rule never crosses into the sidebar's column
		if strings.Contains(string(cells[l.inputSepY][l.divX+1:]), "ᴀꜱᴋ") {
			t.Fatalf("%s: the ask's title leaked under the sidebar: %q", when, strip[l.inputSepY])
		}
	}
	if !strings.HasPrefix(strings.TrimLeft(string(cells[l.input.Y][l.input.X:l.input.X+l.input.W]), " "), "›") && m.choice == nil {
		t.Fatalf("%s: the input's prompt is not at the input's origin: %q", when, strip[l.input.Y])
	}
}

// The size matrix: 60–200 columns × 26–60 rows (the frame's floor to a wide monitor), a first
// screen and a busy turn (stream, tools, subagents, a grown draft, the slash menu, a toast, the
// sidebar scrolled and hidden) — every frame keeps the policies.
func TestFrameSizeMatrix(t *testing.T) {
	for _, w := range []int{60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200} {
		for _, h := range []int{26, 28, 30, 34, 40, 45, 50, 60} {
			t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
				host := &fakeHost{plans: []RailPlan{{Agent: "coord-agent-one", Rank: "coord", Role: "drafter", Items: []RailPlanItem{{Text: "frame the window", Status: "done"}, {Text: "the welcome", Status: "in_progress"}}}}}
				m := New(host, NewTheme(true), DefaultWords())
				m.Update(tea.WindowSizeMsg{Width: w, Height: h})
				m.Update(evBgWait{})
				checkFrame(t, m, "first screen")

				m.setPlans(host.Plans())
				m.Update(EvTurnStart{Text: "redesign the frame"})
				m.Update(EvDelta{Text: strings.Repeat("a streamed paragraph that wraps across the conversation pane. ", 6) + "\n\n"})
				m.Update(EvToolEnd{Tool: ToolView{ID: "a", Name: "edit", Summary: "tui/frame.go", Class: "write", Status: "done", Ms: 3, Wrote: []string{"tui/frame.go", "tui/model.go"}}})
				m.Update(EvSubagent{Subagent: SubagentView{Name: "domain-review", Rank: "domain", Role: "judge", State: "tool", Ask: "review"}})
				m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "watch"}})
				m.Update(EvToolStart{Tool: ToolView{ID: "b", Name: "bash", Summary: "go test ./tui/", Class: "read"}})
				checkFrame(t, m, "mid-turn")

				m.Toast("✓ zone-auth done", m.theme.pill("ok"))
				m.toasts[0].x = 0
				checkFrame(t, m, "toast")

				m.input.SetValue("one\ntwo\nthree\nfour")
				m.input.SetHeight(4)
				checkFrame(t, m, "grown draft")
				m.input.Reset()
				m.input.SetHeight(1)

				m.setInput("/")
				m.menu = &menuState{items: []MenuItem{{Name: "status", Description: "the workspace"}, {Name: "agents", Description: "who"}}}
				checkFrame(t, m, "slash menu")
				m.menu = nil
				m.setInput("")

				m.railScroll = 3
				checkFrame(t, m, "rail scrolled")

				m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt})
				checkFrame(t, m, "sidebar hidden")
				m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt})

				m.choice = &EvChoice{View: ChoiceView{Title: "Question", Why: "wait, or new work?", Options: []string{"wait", "new work"}}}
				m.choiceAt = time.Now().Add(-time.Second)
				checkFrame(t, m, "ask card")
				m.choice = nil
			})
		}
	}
}
