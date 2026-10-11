package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The chat's scrollbar answers the mouse: a click on its column jumps the thumb there, a drag
// walks it to the end, the release lets go — and neither starts a selection.
func TestScrollbarDrag(t *testing.T) {
	m := New(&fakeHost{}, depthTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m.Update(evBgWait{})
	m.Update(EvTurnStart{Text: "go"})
	m.Render()
	for i := 0; i < 6; i++ {
		m.printAnswer(strings.Repeat("A paragraph long enough to fill the pane a few times over. ", 8))
	}
	m.Render()
	l := m.lay
	if len(m.vpLines) <= l.chat.H || !m.vp.AtBottom() {
		t.Fatalf("the transcript does not overflow (%d rows in %d)", len(m.vpLines), l.chat.H)
	}
	m.Update(tea.MouseClickMsg{X: l.mc, Y: l.chat.Y, Button: tea.MouseLeft})
	if m.barDrag != "chat" || m.vp.YOffset() != 0 || m.sel != nil {
		t.Fatalf("a click on the bar's top: drag %q, offset %d, selection %v", m.barDrag, m.vp.YOffset(), m.sel)
	}
	m.Update(tea.MouseMotionMsg{X: l.mc, Y: l.chat.Y + l.chat.H/2, Button: tea.MouseLeft})
	mid := m.vp.YOffset()
	if mid <= 0 || m.vp.AtBottom() {
		t.Fatalf("a drag to the bar's middle left the offset at %d", mid)
	}
	m.Update(tea.MouseMotionMsg{X: l.mc + 30, Y: l.chat.Y + l.chat.H + 5, Button: tea.MouseLeft})
	if !m.vp.AtBottom() {
		t.Fatalf("a drag past the bar's end did not reach the bottom: %d", m.vp.YOffset())
	}
	m.Update(tea.MouseReleaseMsg{X: l.mc, Y: l.chat.Y, Button: tea.MouseLeft})
	if m.barDrag != "" || m.sel != nil {
		t.Fatalf("the release kept the drag (%q) or selected (%v)", m.barDrag, m.sel)
	}
	// a click in the conversation's text still selects
	m.Update(tea.MouseClickMsg{X: l.chat.X + 3, Y: l.chat.Y + 2, Button: tea.MouseLeft})
	if m.sel == nil || m.barDrag != "" {
		t.Fatal("a click on the text no longer starts a selection")
	}
}

// The thumb is sized to the visible fraction and placed by the offset, at half-row precision.
func TestScrollThumb(t *testing.T) {
	if scrollThumb(10, 10, 0) != nil {
		t.Fatal("content that fits draws no bar")
	}
	count := func(b []rune) (n int) {
		for _, r := range b {
			switch r {
			case '▐':
				n += 2
			case '▗', '▝':
				n++
			}
		}
		return n
	}
	for _, c := range []struct{ rows, total, top int }{{10, 20, 0}, {10, 20, 10}, {10, 40, 15}, {20, 1000, 500}, {8, 9, 1}} {
		b := scrollThumb(c.rows, c.total, c.top)
		if len(b) != c.rows {
			t.Fatalf("%v: %d rows", c, len(b))
		}
		want := max(c.rows*2*c.rows/c.total, 2)
		if got := count(b); got != want {
			t.Fatalf("%v: thumb %d half rows, want %d: %q", c, got, want, string(b))
		}
		if c.top == 0 && b[0] == '▕' {
			t.Fatalf("%v: at the top the thumb starts lower: %q", c, string(b))
		}
		if c.top == c.total-c.rows && b[len(b)-1] == '▕' {
			t.Fatalf("%v: at the bottom the thumb ends higher: %q", c, string(b))
		}
	}
}
