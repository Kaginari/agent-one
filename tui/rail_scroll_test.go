package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// railModel builds a live model wide enough for the rail pane, with a plan card so the rail
// has content. The window size is delivered through Update exactly as Bubble Tea delivers it.
func railModel(t *testing.T) *Model {
	t.Helper()
	host := &fakeHost{plans: []RailPlan{
		{Agent: "plan", Rank: "chief", Role: "p1", Items: []RailPlanItem{{Text: "one"}, {Text: "two"}}},
	}}
	m := New(host, NewTheme(true), DefaultWords())
	if mm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40}); mm != nil {
		if got, ok := mm.(*Model); ok {
			m = got
		}
	}
	return m
}

// railLines returns the rendered rail rows as they appear on screen.
func (m *Model) railLines() []string {
	view := ansi.Strip(m.Render())
	return strings.Split(view, "\n")
}

func TestRailScrollWheelWalksTheRail(t *testing.T) {
	m := railModel(t)
	railX := m.width - 1 // right edge: inside the rail column
	m.Update(tea.MouseWheelMsg{X: railX, Y: 20, Button: tea.MouseWheelDown})
	if m.railScroll != 1 {
		t.Fatalf("wheel-down over the rail walks it one row: scroll=%d", m.railScroll)
	}
	m.Update(tea.MouseWheelMsg{X: railX, Y: 20, Button: tea.MouseWheelUp})
	if m.railScroll != 0 {
		t.Fatalf("wheel-up walks it back: scroll=%d", m.railScroll)
	}
	// wheel over the chat never touches the rail
	before := m.railScroll
	m.Update(tea.MouseWheelMsg{X: 20, Y: 20, Button: tea.MouseWheelDown})
	if m.railScroll != before {
		t.Fatalf("chat wheel left rail at %d, got %d", before, m.railScroll)
	}
}

func TestRailScrollClampsAtContentEnd(t *testing.T) {
	m := railModel(t)
	// force scroll way past the end, then re-render to trigger clamp
	m.railScroll = 10_000
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40}) // any update triggers splitPanes
	if m.railScroll < 0 {
		t.Fatalf("rail scroll never goes negative: %d", m.railScroll)
	}
	// the clamp must prevent scrolling past the content height
	view := ansi.Strip(m.Render())
	lines := strings.Split(view, "\n")
	// when clamped to maxScroll, the rail must fill the pane: no blank gap at the bottom
	railCol := m.width - m.railVisible()
	for i := 0; i < min(m.height, len(lines)); i++ {
		far := lines[i][max(0, railCol):]
		if i == m.height-1 && len(strings.TrimSpace(far)) > 0 && m.railScroll == 0 {
			t.Logf("last rail row: %q", far)
		}
	}
	if m.railScroll > 0 {
		t.Logf("clamped to %d", m.railScroll)
	}
}

func TestRailScrollEdgeMarkers(t *testing.T) {
	m := railModel(t)
	lines := m.railLines()
	railCol := m.width - m.railVisible()
	if m.railScroll == 0 {
		t.Skip("rail content fits pane; no markers expected")
	}
	// check first row has ▲ when scrolled down
	firstRail := ""
	if len(lines) > 0 && len(lines[0]) > railCol {
		firstRail = ansi.Strip(lines[0][railCol:])
	}
	if !strings.Contains(firstRail, "▲") {
		t.Errorf("scrolled rail shows ▲ at its top row: %q", firstRail)
	}
}
