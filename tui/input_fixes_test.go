package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// typeText feeds runes as key presses, the way a terminal does.
func typeText(m *Model, s string) {
	for _, r := range s {
		k := tea.KeyPressMsg{Code: r, Text: string(r)}
		if r >= 'A' && r <= 'Z' {
			k.Mod = tea.ModShift
		}
		m.Update(k)
	}
}

func paneModel(t *testing.T) *Model {
	t.Helper()
	m := New(&fakeHost{}, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	if m.railVisible() == 0 {
		t.Fatal("this test wants the sidebar")
	}
	return m
}

// A typed word is never eaten by the plan chords: "Cat" with a live deck, "cc" on a pinned
// stale deck — the chords fire only on an empty input.
func TestPlanChordsNeverEatTypedWords(t *testing.T) {
	h := &fakeHost{plans: []RailPlan{
		{Agent: "zone-a", Rank: "zone", Stale: true, Items: []RailPlanItem{{Text: "old", Status: "pending"}}},
		{Agent: "orchestrator", Rank: "orchestrator", Items: []RailPlanItem{{Text: "now", Status: "in_progress"}}},
	}}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.Update(evPlans{plans: m.host.Plans()})
	typeText(m, "Cat")
	if got := m.input.Value(); got != "Cat" {
		t.Fatalf("a capital C was swallowed: %q", got)
	}
	m.setInput("")
	m.planIdx, m.planSticky = 0, true // a pinned stale deck: the lowercase chord is armed
	typeText(m, "xcc")
	if got := m.input.Value(); got != "xcc" {
		t.Fatalf("a lowercase c was swallowed mid-word: %q", got)
	}
}

// One long line wraps inside the box and the box grows with its visual rows, up to 8.
func TestInputGrowsWithAWrappedLine(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	typeText(m, strings.Repeat("word ", 30))
	if h := m.input.Height(); h < 3 {
		t.Fatalf("a 150-cell line in a 54-cell box grew to %d rows", h)
	}
	typeText(m, strings.Repeat("more ", 200))
	if h := m.input.Height(); h != 8 {
		t.Fatalf("the box caps at 8 rows, got %d", h)
	}
	if n := len([]rune(m.input.Value())); n < 1150 {
		t.Fatalf("the box stopped taking text at %d characters: past 8 rows it scrolls inside", n)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if h := m.input.Height(); h != 3 {
		t.Fatalf("a cleared box under the frame is its three rows again, got %d", h)
	}
}

// The ask's well under the frame starts at three rows and the conversation gives them up; the
// inline box starts at one.
func TestInputWellStartsAtThreeRows(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m.Render()
	if h := m.input.Height(); h != 3 || m.lay.inputN != 3 {
		t.Fatalf("the well is %d rows (layout %d), want 3", h, m.lay.inputN)
	}
	if v := ansi.Strip(m.input.View()); !strings.Contains(v, "sk anything") {
		t.Fatalf("the empty well shows no placeholder: %q", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	m.Render()
	if h := m.input.Height(); h != 1 {
		t.Fatalf("the inline box is %d rows, want 1", h)
	}
}

// Under panes the transcript lives in the chat viewport; quitting prints it to the scrollback
// at the full width, so the conversation stays in the terminal after exit.
func TestQuitLeavesThePaneTranscript(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(false), DefaultWords())
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(140, 40))
	m.Attach(tm.Send)
	out := &tail{}
	out.follow(tm.Output())
	line := "the answer " + strings.Repeat("x", 110) + " END"
	tm.Send(EvLines{Lines: []string{line}})
	time.Sleep(100 * time.Millisecond)
	if strings.Contains(out.String(), "END") {
		t.Fatal("setup: the pane clips the wide line, the scrollback never saw it")
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
	out.wait(t, line)
}

// Glamour drops raw HTML: angles outside code are escaped first, code keeps them verbatim.
func TestMarkdownKeepsAngles(t *testing.T) {
	th := NewTheme(false)
	got := ansi.Strip(th.Markdown("a Vec<T> and `Vec<U>` and providers.<name>.script, a -> b\n\n> quoted <x>\n\n```\nfn<T>() -> T\n```\n\nsee <https://example.com>", 80))
	for _, want := range []string{"Vec<T>", "Vec<U>", "providers.<name>.script", "a -> b", "quoted <x>", "fn<T>() -> T", "https://example.com"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q lost from:\n%s", want, got)
		}
	}
	if strings.Contains(got, `\<`) || strings.Contains(got, `\>`) {
		t.Fatalf("an escape leaked into the render:\n%s", got)
	}
	if e := escapeAngles("x\n\n    code <b>\n"); !strings.Contains(e, "code <b>") {
		t.Fatalf("indented code is code: %q", e)
	}
}

// A block that finishes under the board prints once when the board closes — never again from
// the catch-up of the next frame.
func TestBlockUnderTheBoardPrintsOnce(t *testing.T) {
	m := boardAt(t, 80, 24)
	for len(m.prints) > 0 {
		<-m.prints
	}
	m.Update(EvNotice{Text: "landed under the board"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.Render()
	m.Render()
	if n := strings.Count(drainPrints(m), "landed under the board"); n != 1 {
		t.Fatalf("the held block printed %d times", n)
	}
}

// The agents view's pipe borrows the input line and gives the human's draft back.
func TestPipeKeepsTheDraft(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking"}})
	typeText(m, "my draft")
	m.Update(ctrl('t'))
	m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if m.input.Value() != "" {
		t.Fatalf("the pipe opens empty: %q", m.input.Value())
	}
	typeText(m, "hold the ttl")
	m.Update(key(tea.KeyEnter))
	if got := m.input.Value(); got != "my draft" {
		t.Fatalf("the draft after a pipe: %q", got)
	}
	m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	typeText(m, "never skill")
	m.Update(key(tea.KeyEsc))
	if got := m.input.Value(); got != "my draft" {
		t.Fatalf("the draft after an abandoned pipe: %q", got)
	}
	if p := waitPiped(h, time.Second); len(p) != 1 {
		t.Fatalf("piped %v", p)
	}
}

// ctrl+c: a draft clears first (↑ brings it back), a busy turn interrupts without arming the
// exit, and only an idle, empty input quits on the second press.
func TestCtrlCClearsInterruptsThenExits(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	typeText(m, "half a note")
	m.Update(ctrl('c'))
	if m.input.Value() != "" || m.quit {
		t.Fatalf("the first ctrl+c clears the draft: %q quit=%v", m.input.Value(), m.quit)
	}
	m.Update(key(tea.KeyUp))
	if got := m.input.Value(); got != "half a note" {
		t.Fatalf("↑ brings the cleared draft back: %q", got)
	}
	m.setInput("")
	m.begin()
	for i := 0; i < 3; i++ {
		m.Update(ctrl('c'))
	}
	if m.quit {
		t.Fatal("a mashed ctrl+c on a busy turn quit the session")
	}
	if h.interrupted != 3 {
		t.Fatalf("each busy ctrl+c interrupts: %d", h.interrupted)
	}
	m.busy = false
	m.Update(ctrl('c'))
	if m.quit {
		t.Fatal("one idle ctrl+c only arms the exit")
	}
	m.Update(ctrl('c'))
	if !m.quit {
		t.Fatal("the second idle ctrl+c exits")
	}
}

// esc with a draft clears it, and the draft is one ↑ away.
func TestEscDraftIsRecoverable(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.remember("older")
	typeText(m, "a long careful prompt")
	m.Update(key(tea.KeyEsc))
	if m.input.Value() != "" {
		t.Fatalf("esc clears: %q", m.input.Value())
	}
	m.Update(key(tea.KeyUp))
	if got := m.input.Value(); got != "a long careful prompt" {
		t.Fatalf("↑ after esc: %q", got)
	}
}

// A long error wraps to the width; nothing is clipped.
func TestErrorWraps(t *testing.T) {
	msg := "provider anthropic: 400 bad request: the prompt is too long for the model's context window, trim it"
	got := ansi.Strip(NewTheme(false).Error(msg, 40))
	if strings.Contains(got, "…") || !strings.Contains(strings.Join(strings.Fields(got), " "), msg) {
		t.Fatalf("the error lost text:\n%s", got)
	}
	for _, l := range strings.Split(got, "\n") {
		if ansi.StringWidth(l) > 40 {
			t.Fatalf("an error row overruns: %q", l)
		}
	}
}

// Under panes ctrl+u/ctrl+d stay the textarea's; pgup/pgdn and shift+↑/↓ scroll the chat.
func TestPaneLeavesCtrlUToTheInput(t *testing.T) {
	m := paneModel(t)
	for i := 0; i < 60; i++ {
		m.Update(EvNotice{Text: "block"})
	}
	m.Render()
	typeText(m, "delete me")
	m.Update(ctrl('u'))
	if got := m.input.Value(); got != "" {
		t.Fatalf("ctrl+u deletes before the cursor: %q", got)
	}
	bottom := m.vp.YOffset()
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	if m.vp.YOffset() != bottom-1 {
		t.Fatalf("shift+↑ walks one line: %d → %d", bottom, m.vp.YOffset())
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.vp.YOffset() != bottom {
		t.Fatalf("pgdn pages back to the tail: %d, want %d", m.vp.YOffset(), bottom)
	}
}
