package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestFooterHoldsTheHeldQueue: the relay hold is visible — rows the shared queue holds for the
// next wake read "N held" in the status bar until the wake claims them.
func TestFooterHoldsTheHeldQueue(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.Update(evBgWait{})
	m.Update(EvHeld{N: 2})
	if got := ansi.Strip(m.Render()); !strings.Contains(got, "2 held") {
		t.Fatalf("the held rows are not on the footer:\n%s", got)
	}
	m.Update(EvHeld{N: 0})
	if got := ansi.Strip(m.Render()); strings.Contains(got, " held") {
		t.Fatalf("claimed rows still read held:\n%s", got)
	}
	if got := ansi.Strip(NewTheme(true).Footer(FooterView{Model: "m", Held: 3}, 200)); !strings.Contains(got, "3 held") {
		t.Fatalf("the plain footer: %q", got)
	}
}
