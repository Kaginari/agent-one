package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// overflowedPane is a pane-mode model whose conversation runs past the chat pane, following
// the bottom as a human who never scrolled sees it.
func overflowedPane(t *testing.T, w, h int) *Model {
	t.Helper()
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.Update(evBgWait{})
	if !m.paneMode() {
		t.Fatalf("%d×%d is not pane mode", w, h)
	}
	for i := 0; i < 30; i++ {
		m.print(fixed(fmt.Sprintf("earlier block %02d", i)))
	}
	m.Render()
	if len(m.vpLines) <= m.lay.chat.H || !m.vp.AtBottom() {
		t.Fatalf("the transcript does not overflow at the bottom (%d rows in %d)", len(m.vpLines), m.lay.chat.H)
	}
	return m
}

// A slash command's output in pane mode joins the conversation as a block at the bottom: the
// viewport follows it, nothing is printed outside the frame (a tea.Println under the inline
// full-height frame corrupts it), and the scroll never jumps to the top.
func TestPaneSlashOutputLandsAtTheBottom(t *testing.T) {
	m := overflowedPane(t, 100, 30)
	m.Update(evSlashDone{lines: []string{"status first line", "@? sandbox: none — disabled by config", "status last line"}})
	frame := ansi.Strip(m.Render())
	for _, want := range []string{"sandbox: none", "status last line"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("the slash output's %q is not in the frame:\n%s", want, frame)
		}
	}
	if !m.vp.AtBottom() || m.vp.YOffset() == 0 {
		t.Fatalf("the viewport left the bottom: offset %d of %d rows", m.vp.YOffset(), len(m.vpLines))
	}
	if n := len(m.prints); n != 0 {
		t.Fatalf("%d print(s) queued outside the frame in pane mode", n)
	}
	if last := m.blocks[len(m.blocks)-1]; !strings.Contains(m.rendered(last, m.chatW()), "status last line") {
		t.Fatal("the slash output is not the conversation's last block")
	}
}

// The pane follows the bottom through a shrink it did not ask for — a smaller window, a draft
// growing the ask box: the next block still shows. The shrink used to read as a climb (AtBottom
// measured at the new height), and the conversation froze where it stood.
func TestPaneFollowsTheBottomAcrossAShrink(t *testing.T) {
	m := overflowedPane(t, 100, 34)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Render()
	m.print(fixed("after the window shrank"))
	if frame := ansi.Strip(m.Render()); !strings.Contains(frame, "after the window shrank") {
		t.Fatalf("a smaller window lost the bottom:\n%s", frame)
	}
	for i := 0; i < 4; i++ { // the draft grows the ask box row by row
		m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
		m.Render()
	}
	m.print(fixed("after the draft grew"))
	if frame := ansi.Strip(m.Render()); !strings.Contains(frame, "after the draft grew") {
		t.Fatalf("a taller ask box lost the bottom:\n%s", frame)
	}
}

// A content that fit the pane (the offset at 0) and then outgrew a shrunk pane keeps following:
// the conversation never stays pinned to its top.
func TestPaneNeverPinsToTheTop(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.Update(evBgWait{})
	for i := 0; i < 6; i++ {
		m.print(fixed(fmt.Sprintf("fits %02d", i)))
	}
	m.Render()
	if len(m.vpLines) > m.lay.chat.H || m.vp.YOffset() != 0 {
		t.Fatalf("want a conversation that fits: %d rows in %d", len(m.vpLines), m.lay.chat.H)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 26})
	m.Render()
	if len(m.vpLines) <= m.lay.chat.H {
		t.Fatalf("want it to overflow the shrunk pane: %d rows in %d", len(m.vpLines), m.lay.chat.H)
	}
	for i := 0; i < 20; i++ {
		m.print(fixed(fmt.Sprintf("line %02d", i)))
	}
	if frame := ansi.Strip(m.Render()); !strings.Contains(frame, "line 19") {
		t.Fatalf("the pane stayed at the top:\n%s", frame)
	}
}

// The welcome heads the conversation even when a first ask landed blocks before the terminal's
// background answer (or its wait) let the welcome draw.
func TestWelcomeHeadsAnEarlyTurn(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {60, 16}} {
		m := New(&fakeHost{}, NewTheme(true), DefaultWords())
		m.Attach(func(tea.Msg) {})
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.print(fixed("▌ an early ask"))
		m.Update(evBgWait{})
		s := m.Transcript()
		w, a := strings.Index(s, "vtest"), strings.Index(s, "an early ask")
		if w < 0 || a < 0 || w > a {
			t.Fatalf("%v: the welcome (%d) must come before the early ask (%d):\n%s", size, w, a, s)
		}
		if !m.paneMode() {
			// the scrollback is printed again whole, the welcome first
			var last printItem
			for len(m.prints) > 0 {
				last = <-m.prints
			}
			if !last.clear || strings.Index(last.text, "vtest") > strings.Index(last.text, "an early ask") {
				t.Fatalf("%v: the scrollback was not reprinted welcome-first: %+v", size, last)
			}
		}
	}
}
