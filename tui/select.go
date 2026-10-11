package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Selection and copy in the chat pane. While the panes own the window the program captures the
// mouse (the wheel scrolls the chat), so the terminal cannot select: a drag selects here instead,
// in content coordinates (a transcript line, a cell), and the release copies the plain text —
// OSC 52 for the terminal, and the desktop's own clipboard tool where OSC 52 never lands. /mouse
// hands the mouse back to the terminal; shift+drag does the same for one drag on most terminals.

// spot is a cell of the chat's content: a line of the transcript (plus the live rows) and a
// column on it.
type spot struct{ line, col int }

func (a spot) before(b spot) bool { return a.line < b.line || a.line == b.line && a.col < b.col }

type selection struct {
	anchor, head spot
	drag         bool // the button is still down
	shown        bool // a word, a line, or a drag that left its anchor: something is selected
	edge         int  // -1 / +1 while a drag rests on the pane's top / bottom row: autoscroll
	ticking      bool
}

// clickGap is how close a second click must follow to count as a double (or triple) click.
const clickGap = 400 * time.Millisecond

type evSelScroll struct{}

// selSpot maps a window cell to the chat's content, clamped to the chat pane: the borders, the
// input and the sidebar never enter a selection. ok false when the cell is not in the chat
// pane. The pane's origin comes from the last frame's layout.
func (m *Model) selSpot(x, y int, clamp bool) (spot, bool) {
	if !m.vpOK || m.vpCW <= 0 || m.vpH <= 0 {
		return spot{}, false
	}
	c := m.geom().chat
	x, y = x-c.X, y-c.Y
	if !clamp && (x < 0 || x >= m.vpCW || y < 0 || y >= m.vpH) {
		return spot{}, false
	}
	x, y = min(max(x, 0), m.vpCW-1), min(max(y, 0), m.vpH-1)
	line := m.vp.YOffset() + y
	if n := len(m.vpLines); line >= n {
		line = max(n-1, 0)
	}
	return spot{line, x}, true
}

// mouseSelect handles the chat pane's mouse: ok false leaves the event to the rail.
func (m *Model) mouseSelect(msg tea.Msg) (tea.Cmd, bool) {
	if !m.paneMode() || m.mouseTerm || m.choice != nil || m.palette != nil || m.pick != nil {
		return nil, false
	}
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return nil, false
		}
		p, ok := m.selSpot(msg.X, msg.Y, false)
		if !ok {
			m.sel = nil
			return nil, false
		}
		now := time.Now()
		if now.Sub(m.clickAt) < clickGap && m.clickSpot.line == p.line && abs(m.clickSpot.col-p.col) <= 1 {
			m.clicks = m.clicks%3 + 1
		} else {
			m.clicks = 1
		}
		m.clickAt, m.clickSpot = now, p
		m.sel = &selection{anchor: p, head: p, drag: true}
		switch m.clicks {
		case 2:
			m.sel.anchor, m.sel.head = m.wordAt(p)
			m.sel.shown = true
		case 3:
			w := m.lineWidth(p.line)
			m.sel.anchor, m.sel.head = spot{p.line, 0}, spot{p.line, max(w-1, 0)}
			m.sel.shown = w > 0
		}
		return nil, true
	case tea.MouseMotionMsg:
		if m.sel == nil || !m.sel.drag {
			return nil, false
		}
		p, _ := m.selSpot(msg.X, msg.Y, true)
		if p != m.sel.head {
			m.sel.head, m.sel.shown = p, true
		}
		m.sel.edge = 0
		switch {
		case msg.Y <= m.geom().chat.Y && !m.vp.AtTop():
			m.sel.edge = -1
		case msg.Y >= m.geom().chat.Y+m.vpH-1 && !m.vp.AtBottom():
			m.sel.edge = 1
		}
		if m.sel.edge != 0 && !m.sel.ticking {
			m.sel.ticking = true
			return selScrollTick(), true
		}
		return nil, true
	case tea.MouseReleaseMsg:
		if m.sel == nil || !m.sel.drag {
			return nil, false
		}
		m.sel.drag, m.sel.edge = false, 0
		if !m.sel.shown {
			m.sel = nil
			return nil, true
		}
		text := m.selText()
		if text == "" {
			return nil, true
		}
		n := strings.Count(text, "\n") + 1
		return m.copyText(text, fmt.Sprintf("copied %d line%s", n, plural(n))), true
	}
	return nil, false
}

func selScrollTick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return evSelScroll{} })
}

// selScroll walks the pane a line while a drag rests on its edge, the head riding along.
func (m *Model) selScroll() tea.Cmd {
	s := m.sel
	if s == nil || !s.drag || s.edge == 0 {
		if s != nil {
			s.ticking = false
		}
		return nil
	}
	if s.edge < 0 {
		m.vp.ScrollUp(1)
		s.head = spot{m.vp.YOffset(), 0}
	} else {
		m.vp.ScrollDown(1)
		s.head = spot{min(m.vp.YOffset()+m.vpH-1, max(len(m.vpLines)-1, 0)), m.vpCW - 1}
	}
	if (s.edge < 0 && m.vp.AtTop()) || (s.edge > 0 && m.vp.AtBottom()) {
		s.edge, s.ticking = 0, false
		return nil
	}
	return selScrollTick()
}

// selRange is the selected cells [c0, c1) on a content line; ok false when none are.
func (m *Model) selRange(line int) (c0, c1 int, ok bool) {
	s := m.sel
	if s == nil || !s.shown {
		return 0, 0, false
	}
	a, b := s.anchor, s.head
	if b.before(a) {
		a, b = b, a
	}
	if line < a.line || line > b.line {
		return 0, 0, false
	}
	w := m.lineWidth(line)
	if line > a.line {
		c0 = 0
	} else {
		c0 = a.col
	}
	c1 = w
	if line == b.line {
		c1 = min(b.col+1, w)
	}
	return c0, c1, c0 < c1
}

// lineWidth is a content line's text width, its trailing pad left out.
func (m *Model) lineWidth(line int) int {
	if line < 0 || line >= len(m.vpLines) {
		return 0
	}
	return ansi.StringWidth(strings.TrimRight(ansi.Strip(m.vpLines[line]), " "))
}

// wordAt is the run of non-space cells around p, as an anchor and a head.
func (m *Model) wordAt(p spot) (spot, spot) {
	if p.line >= len(m.vpLines) {
		return p, p
	}
	cells := []string{} // one entry per cell; a wide rune's second cell is ""
	for _, r := range ansi.Strip(m.vpLines[p.line]) {
		cells = append(cells, string(r))
		for i := 1; i < ansi.StringWidth(string(r)); i++ {
			cells = append(cells, "")
		}
	}
	if p.col >= len(cells) || strings.TrimSpace(cells[p.col]) == "" && cells[p.col] != "" {
		return p, p
	}
	space := func(i int) bool { return cells[i] != "" && strings.TrimSpace(cells[i]) == "" }
	a, b := p.col, p.col
	for a > 0 && !space(a-1) {
		a--
	}
	for b+1 < len(cells) && !space(b+1) {
		b++
	}
	return spot{p.line, a}, spot{p.line, b}
}

// selText is the selection as plain text: escapes stripped, each line's trailing pad trimmed.
func (m *Model) selText() string {
	s := m.sel
	if s == nil {
		return ""
	}
	a, b := s.anchor, s.head
	if b.before(a) {
		a, b = b, a
	}
	var out []string
	for l := a.line; l <= b.line && l < len(m.vpLines); l++ {
		c0, c1, ok := m.selRange(l)
		if !ok {
			out = append(out, "")
			continue
		}
		// the depth's glyphs (a spine, a bevel, a shadow) are drawing, not text: they copy as
		// nothing
		out = append(out, strings.TrimRight(stripDecor(ansi.Cut(ansi.Strip(m.vpLines[l]), c0, c1)), " "))
	}
	if m.theme.Depth {
		// a slab's padding indents every row it holds: the copy keeps only the relative indent
		pre := -1
		for _, l := range out {
			if strings.TrimSpace(l) == "" {
				continue
			}
			if n := len(l) - len(strings.TrimLeft(l, " ")); pre < 0 || n < pre {
				pre = n
			}
		}
		for i, l := range out {
			if len(l) >= pre && pre > 0 {
				out[i] = l[pre:]
			} else if strings.TrimSpace(l) == "" {
				out[i] = ""
			}
		}
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

// highlight paints the selection over the chat pane's visible rows (top is the first row's
// content line).
func (m *Model) highlight(rows []string, top int) {
	if m.sel == nil || !m.sel.shown {
		return
	}
	for i := range rows {
		if c0, c1, ok := m.selRange(top + i); ok {
			rows[i] = lipgloss.StyleRanges(rows[i], lipgloss.NewRange(c0, c1, m.theme.sel))
		}
	}
}

// lastAnswer is the source markdown of the newest turn that answered, its pieces joined.
func (m *Model) lastAnswer() string {
	turn := -1
	var parts []string
	for i := len(m.blocks) - 1; i >= 0; i-- {
		b := m.blocks[i]
		if !b.answer {
			continue
		}
		if turn == -1 {
			turn = b.turn
		}
		if b.turn != turn {
			break
		}
		parts = append([]string{strings.TrimSpace(b.src)}, parts...)
	}
	if s := strings.TrimSpace(m.stream.String()); s != "" && m.busy && (turn == -1 || turn == m.turnSeq) {
		parts = append(parts, s) // the paragraph still being written
	}
	return strings.Join(parts, "\n\n")
}

// copyAnswer copies the last answer's markdown (ctrl+y, /copy).
func (m *Model) copyAnswer() tea.Cmd {
	text := m.lastAnswer()
	if text == "" {
		m.say("no answer to copy yet")
		return nil
	}
	n := strings.Count(text, "\n") + 1
	return m.copyText(text, fmt.Sprintf("copied the answer · %d line%s", n, plural(n)))
}

// copyText puts text on every clipboard within reach and says so in a toast.
func (m *Model) copyText(text, note string) tea.Cmd {
	return tea.Batch(tea.SetClipboard(text), tea.SetPrimaryClipboard(text), copyNative(text), m.Toast("✓ "+note, m.theme.pill("ok")))
}

// toggleMouse hands the mouse to the terminal (its own selection, its own scrollback wheel) or
// takes it back for the panes.
func (m *Model) toggleMouse() tea.Cmd {
	m.mouseTerm = !m.mouseTerm
	m.sel = nil
	if m.mouseTerm {
		m.say("mouse: the terminal's — /mouse again takes it back")
	} else {
		m.say("mouse: the panes' — drag selects, the wheel scrolls")
	}
	return nil
}

// nativeCopy pipes text into the desktop's clipboard tool, the first present: some terminals
// (GNOME Terminal among them) ignore OSC 52. Never over SSH — the remote's clipboard is not the
// human's. Tests stub it.
var nativeCopy = func(text string) error {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return nil
	}
	var tools [][]string
	if runtime.GOOS == "darwin" {
		tools = append(tools, []string{"pbcopy"})
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		tools = append(tools, []string{"wl-copy"})
	}
	if os.Getenv("DISPLAY") != "" {
		tools = append(tools, []string{"xclip", "-selection", "clipboard"}, []string{"xsel", "-b", "-i"})
	}
	for _, t := range tools {
		if _, err := exec.LookPath(t[0]); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, t[0], t[1:]...)
		c.Stdin = bytes.NewBufferString(text)
		return c.Run()
	}
	return nil
}

// copyNative runs nativeCopy off the UI goroutine.
func copyNative(text string) tea.Cmd {
	return func() tea.Msg {
		_ = nativeCopy(text)
		return nil
	}
}
