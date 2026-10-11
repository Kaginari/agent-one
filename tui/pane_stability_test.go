package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// The pane invariant (2026-10-02 paste-jumble finding): while the window size holds, every
// frame must be exactly m.height rows — a toast's slide-in/expiry, a palette, or a busy turn
// may never grow or shrink the live frame, or the inline renderer leaves ghosts behind.
func TestPaneFrameHeightIsInvariant(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 36})
	if m.railVisible() == 0 {
		t.Fatal("this test wants the sidebar")
	}
	rows := func() int { return strings.Count(m.Render(), "\n") + 1 }
	base := rows()
	// a live subagent + a toast firing (the subagent-landing toast path)
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "watch"}})
	if got := rows(); got != base {
		t.Fatalf("a live subagent changed the frame height: %d vs %d", got, base)
	}
	m.Toast("✓ zone-auth done", m.theme.pill("ok"))
	if got := rows(); got != base {
		t.Fatalf("a toast grew the frame: %d vs %d", got, base)
	}
	// toast expired
	m.toasts[0].born = time.Now().Add(-toastLife - time.Second)
	if m.stepToasts() {
		t.Fatal("the spent toast must not keep animating")
	}
	if got := rows(); got != base {
		t.Fatalf("an expired toast shrank the frame: %d vs %d", got, base)
	}
	// palette open
	m.openPalette()
	if got := rows(); got != base {
		t.Fatalf("the palette changed the frame height: %d vs %d", got, base)
	}
	m.palette = nil
	// the slash menu under the input (home rows grow — the pane absorbs it)
	m.menu = &menuState{items: []MenuItem{{Name: "status", Description: "x"}, {Name: "agents", Description: "x"}}}
	if got := rows(); got != base {
		t.Fatalf("the menu changed the frame height: %d vs %d", got, base)
	}
	m.menu = nil
	// a grown textarea (3 lines) at the home rows
	m.input.SetValue("one\ntwo\nthree")
	m.input.SetHeight(3)
	if got := rows(); got != base {
		t.Fatalf("a grown textarea changed the frame height: %d vs %d", got, base)
	}
	m.input.Reset()
	m.input.SetHeight(1)
	// a busy stream, long enough to need clipping
	m.begin()
	m.stream.WriteString(strings.Repeat("a streamed line that should keep the frame steady\n\n", 50))
	if got := rows(); got != base {
		t.Fatalf("a long stream changed the frame height: %d vs %d", got, base)
	}
	// no duplicate content line anywhere in the frame (the padRight ghost)
	for _, l := range strings.Split(ansi.Strip(m.Render()), "\n") {
		if c := strings.Count(l, "subagent zone-auth"); c > 1 {
			t.Fatalf("a duplicated subagent line in the frame: %q", l)
		}
	}
}

// The chat pane scrolls its transcript (the charm-libraries wave's A1 lift): pgup walks it up,
// new content stays put while scrolled, and once the human returns to the bottom the pane
// follows the tail again.
func TestChatPaneScrollsWithViewport(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 36})
	if m.railVisible() == 0 {
		t.Fatal("the viewport policy wants the sidebar")
	}
	// enough transcript to overflow the pane
	for i := 0; i < 30; i++ {
		m.Update(EvNotice{Text: "block " + strings.Repeat("x", 4) + string(rune('A'+i))})
	}
	m.Render() // feed the viewport
	if m.vp.AtBottom() == false {
		t.Fatal("a fresh pane starts anchored to the tail")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	first := m.Render()
	m.Update(EvNotice{Text: "tail block ZZZ"})
	second := m.Render()
	if strings.Contains(ansi.Strip(second), "block xxxxZ") && !strings.Contains(ansi.Strip(first), "block xxxxZ") {
		t.Fatal("new content yanked the scrolled pane to the tail — once scrolled, the human steers")
	}
	// back to the bottom: the pane follows the tail again
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m.vp.GotoBottom()
	m.Update(EvNotice{Text: "follow block ZZZ"})
	if !strings.Contains(ansi.Strip(m.Render()), "ZZZ") {
		t.Fatal("back at the bottom the pane must follow the tail again")
	}
}

// The paste-jumble regression at program level: during a busy turn a finished block prints to
// the scrollback; every frame rendered by the real program must carry the sidebar's divider
// and never exceed the window. Before the fix the stream flush + toast paths broke both.
func TestPaneHoldsOnRealProgram(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(true), DefaultWords())
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(130, 36))
	m.Attach(tm.Send)
	out := &tail{}
	out.follow(tm.Output())
	out.wait(t, "✦ agent-one")

	// busy turn with a running subagent: the sidebar rides
	tm.Send(EvTurnStart{Text: "dock the sidebar"})
	out.wait(t, "esc to interrupt")
	tm.Send(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "watch the pane"}})
	tm.Send(EvDelta{Text: "checking the pane invariant while the stream runs.\n\n"})
	tm.Send(EvToolEnd{Tool: ToolView{ID: "x1", Name: "bash", Class: "read", Status: "done", Ms: 3, Output: "ok"}})
	tm.Send(EvTurnDone{Status: "DONE", Verdict: "pass · no writes"})
	out.wait(t, "pass · no writes") // the gate's verdict banner prints to the scrollback
	ch := make(chan string, 1)
	tm.Send(evSnapshot{ch})
	fm := <-ch
	if got := strings.Count(fm, "\n") + 1; got != 36 {
		t.Fatalf("frame height drifted on program events: got %d rows, want 36", got)
	}
	if !strings.Contains(ansi.Strip(fm), "│") {
		t.Fatalf("the sidebar vanished mid-run:\n%s", ansi.Strip(fm))
	}
	// every frame row fits the window — no wraps into the next row
	for i, l := range strings.Split(ansi.Strip(fm), "\n") {
		if ansi.StringWidth(l) > 130 {
			t.Fatalf("row %d overruns the window (%d wide): %q", i+1, ansi.StringWidth(l), l)
		}
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t)
}
