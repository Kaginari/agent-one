package tui

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// stubCopy records what the native clipboard path was handed.
func stubCopy(t *testing.T) func() []string {
	var mu sync.Mutex
	var got []string
	old := nativeCopy
	nativeCopy = func(s string) error {
		mu.Lock()
		got = append(got, s)
		mu.Unlock()
		return nil
	}
	t.Cleanup(func() { nativeCopy = old })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// run executes a command tree (batches and sequences flattened) and returns the messages.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, runQuick(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// runQuick skips the ticks: a toast's frame clock would sleep.
func runQuick(c tea.Cmd) []tea.Msg {
	if c == nil {
		return nil
	}
	ch := make(chan []tea.Msg, 1)
	go func() { ch <- run(c) }()
	select {
	case out := <-ch:
		return out
	case <-time.After(200 * time.Millisecond):
		return nil
	}
}

// rowOf is the frame row that shows text.
func rowOf(t *testing.T, m *Model, text string) int {
	t.Helper()
	for i, l := range strings.Split(ansi.Strip(m.Render()), "\n") {
		if strings.Contains(l, text) {
			return i
		}
	}
	t.Fatalf("%q is not on screen", text)
	return -1
}

func selModel(t *testing.T) *Model {
	m := paneModel(t)
	for i, w := range []string{"alpha", "bravo", "charlie", "delta"} {
		m.Update(EvNotice{Text: fmt.Sprintf("%s line %d", w, i)})
	}
	m.Render()
	return m
}

func TestDragSelectsAndCopiesPlainText(t *testing.T) {
	copied := stubCopy(t)
	m := selModel(t)
	y0, y1 := rowOf(t, m, "alpha line"), rowOf(t, m, "charlie line")
	m.Update(tea.MouseClickMsg{X: m.lay.chat.X, Y: y0, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: m.lay.chat.X + 3, Y: y1, Button: tea.MouseLeft})
	frame := m.Render()
	if !strings.Contains(frame, m.theme.sel.Render("alpha line 0")) {
		t.Fatalf("the selection is not drawn:\n%q", frame)
	}
	_, cmd := m.Update(tea.MouseReleaseMsg{X: m.lay.chat.X + 3, Y: y1, Button: tea.MouseLeft})
	msgs := run(cmd)
	want := "alpha line 0\n\nbravo line 1\n\nchar" // the head's cell is in
	if got := copied(); len(got) != 1 || got[0] != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
	var osc, primary bool
	for _, msg := range msgs {
		switch fmt.Sprintf("%T", msg) {
		case "tea.setClipboardMsg":
			osc = true
		case "tea.setPrimaryClipboardMsg":
			primary = true
		}
	}
	if !osc || !primary {
		t.Fatalf("OSC 52 clipboard %v, primary %v: %v", osc, primary, msgs)
	}
	if len(m.toasts) == 0 || !strings.Contains(m.toasts[len(m.toasts)-1].text, "copied 5 lines") {
		t.Fatal("no copied toast")
	}
	m.Update(key(tea.KeyEsc))
	if m.sel != nil {
		t.Fatal("esc clears the selection")
	}
}

// The borders, the divider and the sidebar never enter a selection: a drag past the chat's edge is clamped.
func TestDragClampsToTheChatPane(t *testing.T) {
	stubCopy(t)
	m := selModel(t)
	y := rowOf(t, m, "bravo line")
	m.Update(tea.MouseClickMsg{X: m.lay.chat.X, Y: y, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: m.width - 2, Y: y, Button: tea.MouseLeft})
	if got := m.selText(); got != "bravo line 1" {
		t.Fatalf("the selection left the chat pane: %q", got)
	}
	// a click on the rail is the rail's
	m.Update(tea.MouseReleaseMsg{X: m.width - 2, Y: y, Button: tea.MouseLeft})
	m.Update(tea.MouseClickMsg{X: m.width - 2, Y: y, Button: tea.MouseLeft})
	if m.sel != nil {
		t.Fatal("a rail click starts no selection")
	}
}

func TestDoubleAndTripleClick(t *testing.T) {
	stubCopy(t)
	m := selModel(t)
	y := rowOf(t, m, "charlie line")
	click := func() {
		m.Update(tea.MouseClickMsg{X: m.lay.chat.X + 2, Y: y, Button: tea.MouseLeft})
		m.Update(tea.MouseReleaseMsg{X: m.lay.chat.X + 2, Y: y, Button: tea.MouseLeft})
	}
	click()
	if m.sel != nil {
		t.Fatal("a plain click selects nothing")
	}
	click()
	if got := m.selText(); got != "charlie" {
		t.Fatalf("a double click selects the word: %q", got)
	}
	click()
	if got := m.selText(); got != "charlie line 2" {
		t.Fatalf("a triple click selects the line: %q", got)
	}
}

// A drag resting on the pane's top row scrolls the chat up, the selection riding along.
func TestDragAutoScrolls(t *testing.T) {
	stubCopy(t)
	m := paneModel(t)
	for i := 0; i < 80; i++ {
		m.Update(EvNotice{Text: fmt.Sprintf("row %d", i)})
	}
	m.Render()
	top := m.vp.YOffset()
	m.Update(tea.MouseClickMsg{X: m.lay.chat.X + 1, Y: m.lay.chat.Y + 8, Button: tea.MouseLeft})
	_, cmd := m.Update(tea.MouseMotionMsg{X: m.lay.chat.X + 1, Y: m.lay.chat.Y, Button: tea.MouseLeft})
	if cmd == nil || m.sel.edge != -1 {
		t.Fatal("a drag on the top edge arms the autoscroll")
	}
	for i := 0; i < 5; i++ {
		m.Update(evSelScroll{})
	}
	if m.vp.YOffset() != top-5 {
		t.Fatalf("five scroll ticks walk five lines: %d → %d", top, m.vp.YOffset())
	}
	if m.sel.head.line != m.vp.YOffset() {
		t.Fatalf("the head rides the top row: %d vs %d", m.sel.head.line, m.vp.YOffset())
	}
}

// ctrl+y and /copy copy the last answer's markdown, its streamed pieces joined.
func TestCopyLastAnswer(t *testing.T) {
	copied := stubCopy(t)
	m := paneModel(t)
	m.begin()
	m.printAnswer("an old answer")
	m.finish(EvTurnDone{})
	m.begin()
	m.stream.WriteString("# Title\n\nfirst `Vec<T>` paragraph\n\nsecond paragraph")
	m.flushStream(false)
	m.Update(EvToolStart{Tool: ToolView{ID: "t", Name: "bash"}})
	m.Update(EvToolEnd{Tool: ToolView{ID: "t", Name: "bash", Status: "done"}})
	m.stream.WriteString("after the tool")
	m.finish(EvTurnDone{Streamed: true})
	want := "# Title\n\nfirst `Vec<T>` paragraph\n\nsecond paragraph\n\nafter the tool"
	run(m.copyAnswer())
	if got := copied(); len(got) != 1 || got[0] != want {
		t.Fatalf("copied %q, want %q", got, want)
	}
	m.Update(ctrl('y'))
	m.setInput("/copy")
	_, cmd := m.Update(key(tea.KeyEnter))
	run(cmd)
	if got := copied(); len(got) != 2 || got[1] != want {
		t.Fatalf("/copy copied %q", got)
	}
}

// /mouse hands the mouse to the terminal and says so in the footer; again takes it back.
func TestMouseToggle(t *testing.T) {
	m := paneModel(t)
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("the panes capture the mouse")
	}
	m.setInput("/mouse")
	m.Update(key(tea.KeyEnter))
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("/mouse hands the mouse to the terminal")
	}
	if !strings.Contains(ansi.Strip(m.Render()), "mouse: terminal") {
		t.Fatal("the footer badge is missing")
	}
	m.Update(tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseLeft})
	if m.sel != nil {
		t.Fatal("no selection while the terminal owns the mouse")
	}
	m.setInput("/mouse")
	m.Update(key(tea.KeyEnter))
	if m.View().MouseMode != tea.MouseModeCellMotion || strings.Contains(ansi.Strip(m.Render()), "mouse: terminal") {
		t.Fatal("/mouse again takes it back")
	}
}
