package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The first screen (2026-10-10, VS Code): the frame fills the window, but the window's top row
// is not ours to keep — once the launching command line scrolls above the viewport, VS Code's
// sticky scroll pins it over that row, and it covered the zone worker's head and the welcome's top
// border ("everything is one line down"). The frame keeps the window's height exactly (the
// inline renderer's rectangle) and leaves its first row blank; the frame's policies hold under it
// (checkFrame), the welcome's card opens the conversation pane, and the sprite sits whole in the
// sidebar.
func TestPaneFirstScreenMatrix(t *testing.T) {
	head, _ := mascotArt("agent-one", false)
	if len(head) == 0 {
		t.Fatal("the agent-one mascot has no art")
	}
	for _, w := range []int{100, 110, 120, 130, 140, 160} {
		for _, h := range []int{26, 30, 34, 40, 50} {
			t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
				m := New(&fakeHost{}, NewTheme(true), DefaultWords())
				m.Update(tea.WindowSizeMsg{Width: w, Height: h})
				m.Update(evBgWait{}) // the welcome lands
				checkFrame(t, m, "first screen")
				l := m.lay
				strip := strings.Split(ansi.Strip(m.Render()), "\n")
				if l.topY != paneTop || l.chat.Y != paneTop+3 {
					t.Fatalf("the frame must start under the headroom: top %d, chat %d", l.topY, l.chat.Y)
				}
				if r := []rune(strip[l.chat.Y]); r[l.chat.X] != '╭' {
					t.Fatalf("the welcome card must open the conversation: %q", strip[l.chat.Y])
				}
				// the sprite, every row whole, in the sidebar
				side := strings.Join(strip[l.rail.Y:l.rail.Y+l.rail.H], "\n")
				for i, a := range head {
					want := strings.TrimSpace(ansi.Strip(a))
					if want != "" && !strings.Contains(side, want) {
						t.Fatalf("mascot row %d missing from the sidebar: %q", i, want)
					}
				}
			})
		}
	}
}

// A click lands where the human sees it: the window's row is the pane's row plus the headroom.
func TestPaneMouseCountsFromBelowTheHeadroom(t *testing.T) {
	stubCopy(t)
	m := selModel(t)
	y := rowOf(t, m, "bravo line") // a window row (the frame includes the headroom)
	m.Update(tea.MouseClickMsg{X: m.lay.chat.X, Y: y, Button: tea.MouseLeft})
	m.Update(tea.MouseClickMsg{X: m.lay.chat.X, Y: y, Button: tea.MouseLeft}) // a double click: the word
	if m.sel == nil || !m.sel.shown {
		t.Fatal("the double click selected nothing")
	}
	if got := m.selText(); got != "bravo" {
		t.Fatalf("a double click on row %d selected %q, want %q", y, got, "bravo")
	}
}
