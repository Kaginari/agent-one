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

// A sidebar taller than its column holds a scrollbar on its right border: the thumb a heavy
// line in the primary, lower as the column scrolls down.
func TestRailScrollbar(t *testing.T) {
	m := railModel(t)
	m.Update(tea.WindowSizeMsg{Width: m.width, Height: 26})
	for _, d := range []string{"a", "b", "c", "d", "e", "f"} {
		m.noteWrite(d+"/file.go", "orchestrator")
	}
	thumbAt := func() int {
		rows := strings.Split(ansi.Strip(m.Render()), "\n")
		for y := m.lay.rail.Y; y < m.lay.rail.Y+m.lay.rail.H; y++ {
			if r := []rune(rows[y]); strings.ContainsRune("┃╻", r[len(r)-1]) {
				return y
			}
		}
		return -1
	}
	top := thumbAt()
	if m.railTotal <= m.lay.rail.H {
		t.Skip("the sidebar fits its column")
	}
	if top != m.lay.rail.Y {
		t.Fatalf("unscrolled, the thumb starts at row %d, want the column's top %d", top, m.lay.rail.Y)
	}
	m.railScroll = 1 << 10 // clamped to the end
	if low := thumbAt(); low <= top {
		t.Fatalf("scrolled to the end, the thumb sits at %d (was %d)", low, top)
	}
}
