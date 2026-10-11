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
		if !strings.ContainsRune("│┤┃╹╻", at(y, last)) {
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
	if !strings.HasPrefix(strings.TrimLeft(string(cells[l.input.Y][l.input.X:l.input.X+l.input.W]), " "), "›") && m.choice == nil && m.input.ScrollYOffset() == 0 {
		t.Fatalf("%s: the input's prompt is not at the input's origin: %q", when, strip[l.input.Y])
	}
	checkScrollbars(t, m, when, cells, strip)
	if m.theme.Depth {
		checkDepth(t, m, when, cells, strip)
	}
}

// checkScrollbars holds the scrollbars to their policies: the chat's rides its right padding
// column only while the transcript overflows the pane — a track glyph on every row and a thumb
// (one or more rows of ▐ ▗ ▝) sized to the visible fraction; blank otherwise. The sidebar's
// thumb rides its right border only while its tiles overflow.
func checkScrollbars(t *testing.T, m *Model, when string, cells [][]rune, strip []string) {
	t.Helper()
	l := m.lay
	if m.menu != nil || m.comp != nil || m.palette != nil || len(m.toasts) > 0 {
		return // a float may cover the column
	}
	over := m.vpOK && len(m.vpLines) > l.chat.H
	thumb := 0
	for y := l.chat.Y; y < l.chat.Y+l.chat.H; y++ {
		c := cells[y][l.mc]
		switch {
		case !over && c != ' ':
			t.Fatalf("%s: the chat fits, yet its padding carries %q on row %d: %q", when, c, y, strip[y])
		case over && !strings.ContainsRune("▕▐▗▝", c):
			t.Fatalf("%s: the chat overflows, yet row %d has no scrollbar (%q): %q", when, y, c, strip[y])
		case strings.ContainsRune("▐▗▝", c):
			thumb++
		}
	}
	if over && (thumb == 0 || thumb > l.chat.H-1) {
		t.Fatalf("%s: the chat's thumb is %d rows of %d (content %d)", when, thumb, l.chat.H, len(m.vpLines))
	}
	if l.rc > 0 {
		railThumb := 0
		for y := l.rail.Y; y < l.rail.Y+l.rail.H; y++ {
			if strings.ContainsRune("┃╹╻", cells[y][l.W-1]) {
				railThumb++
			}
		}
		if (m.railTotal > l.rail.H) != (railThumb > 0) {
			t.Fatalf("%s: the sidebar's %d rows in %d show a %d-row thumb", when, m.railTotal, l.rail.H, railThumb)
		}
	}
}

// checkDepth holds a depth frame to the depth's policies: the input's well closed on all four sides
// (its top edge ▗▄…▄▖, its sides ▐ and ▌, its bottom edge ▝▀…▀▘) inside the main column; the
// conversation's padding columns clear, so a slab's shadow never spills past its pane; the
// sidebar's tiles inside the sidebar (the right border untouched, checked by checkFrame).
func checkDepth(t *testing.T, m *Model, when string, cells [][]rune, strip []string) {
	t.Helper()
	l := m.lay
	at := func(y, x int) rune { return cells[y][x] }
	if l.well != 2 {
		t.Fatalf("%s: the depth frame has no well (%d edge rows)", when, l.well)
	}
	top, bot := l.inputSepY+1, l.footerSepY-1
	if at(top, 1) != '▗' || at(top, l.mc) != '▖' || at(bot, 1) != '▝' || at(bot, l.mc) != '▘' {
		t.Fatalf("%s: the well's corners are open:\n%s\n%s", when, strip[top], strip[bot])
	}
	for x := 2; x < l.mc; x++ {
		if at(top, x) != '▄' || at(bot, x) != '▀' {
			t.Fatalf("%s: the well's edge breaks at col %d:\n%s\n%s", when, x, strip[top], strip[bot])
		}
	}
	for y := l.input.Y; y < l.input.Y+l.input.H; y++ {
		if at(y, 1) != '▐' || at(y, l.mc) != '▌' {
			t.Fatalf("%s: the well's sides are open on row %d: %q", when, y, strip[y])
		}
	}
	if top != l.input.Y-1 || bot != l.input.Y+l.input.H {
		t.Fatalf("%s: the well's edges do not hug the input: top %d, input %d+%d, bottom %d", when, top, l.input.Y, l.input.H, bot)
	}
	if m.menu != nil || m.comp != nil || m.palette != nil {
		return
	}
	for y := l.chat.Y; y < l.chat.Y+l.chat.H; y++ {
		if at(y, 1) != ' ' || !strings.ContainsRune(" ▕▐▗▝", at(y, l.mc)) {
			t.Fatalf("%s: a slab spilled into the conversation's padding on row %d: %q", when, y, strip[y])
		}
	}
}

// The size matrix: 60–200 columns × 26–60 rows (the frame's floor to a wide monitor), a first
// screen and a busy turn (stream, tools, subagents, a grown draft, the slash menu, a toast, the
// sidebar scrolled and hidden) — every frame keeps the policies.
func TestFrameSizeMatrix(t *testing.T) {
	// flat (the 256-colour and colourless fallback), then the depth on a dark and a light ground
	type look struct {
		name        string
		dark, depth bool
	}
	for _, lk := range []look{{"flat", true, false}, {"depth-dark", true, true}, {"depth-light", false, true}} {
		for _, w := range []int{60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200} {
			for _, h := range []int{26, 28, 30, 34, 40, 45, 50, 60} {
				if lk.name == "depth-light" && w%20 != 0 {
					continue // the light ground shares the dark's geometry: every other width
				}
				t.Run(fmt.Sprintf("%s/%dx%d", lk.name, w, h), func(t *testing.T) { frameMatrixCase(t, w, h, lk.dark, lk.depth) })
			}
		}
	}
}

// depthTheme is a theme as a truecolor terminal sees it: the depth turns on under the frame.
func depthTheme(dark bool) Theme {
	th := NewTheme(dark)
	th.Color, th.TrueColor = true, true
	return th
}

func frameMatrixCase(t *testing.T, w, h int, dark, depth bool) {
	host := &fakeHost{plans: []RailPlan{{Agent: "coord-agent-one", Rank: "coord", Role: "drafter", Items: []RailPlanItem{{Text: "frame the window", Status: "done"}, {Text: "the welcome", Status: "in_progress"}}}}}
	th := NewTheme(dark)
	if depth {
		th = depthTheme(dark)
	}
	m := New(host, th, DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.Update(evBgWait{})
	if depth != m.theme.Depth {
		t.Fatalf("depth %v, want %v", m.theme.Depth, depth)
	}
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

	// the ask's well: three rows empty, growing with the draft's rows to at most eight (or 35%
	// of the window), the conversation giving the rows up
	if m.lay.inputN != 3 {
		t.Fatalf("an empty ask's well is %d rows, want 3", m.lay.inputN)
	}
	for _, n := range []int{1, 4, 6, 8, 12} {
		m.input.SetValue(strings.TrimSuffix(strings.Repeat("a line of the draft\n", n), "\n"))
		checkFrame(t, m, fmt.Sprintf("draft of %d lines", n))
		want := min(max(n, 3), min(8, max(3, h*35/100)))
		if got := m.lay.inputN; got != want && !(got < want && m.lay.chat.H <= 4) {
			t.Fatalf("a %d-line draft's well is %d rows, want %d", n, got, want)
		}
	}
	m.input.SetValue(strings.Repeat("a long line that soft-wraps inside the box rather than scrolling sideways ", 4))
	checkFrame(t, m, "wrapped draft")
	if m.input.Height() < 3 || strings.Count(m.input.View(), "\n")+1 != m.input.Height() {
		t.Fatalf("the wrapped draft is %d rows", m.input.Height())
	}
	m.input.Reset()
	m.input.SetWidth(m.inputW)

	// the chat scrolled to its top: the scrollbar rides the pane while it overflows
	m.vp.SetYOffset(0)
	checkFrame(t, m, "chat scrolled up")
	m.vp.GotoBottom()

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
}
