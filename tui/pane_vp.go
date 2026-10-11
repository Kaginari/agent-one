package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"
)

// The chat pane's viewport (A1 of the charm-libs wave): the transcript stops being a clip and
// becomes a scrollback — pgup/pgdn, shift+↑/↓ and the wheel walk it; new content follows the
// bottom only while the human never scrolled up. Present only while the panes own the window.

// ensureVP builds or resizes the chat viewport for the pane geometry (cw × paneH); true when
// the geometry changed.
func (m *Model) ensureVP(cw, paneH int) bool {
	if m.vpOK && m.vpCW == cw && m.vpH == paneH {
		return false
	}
	if !m.vpOK {
		m.vp = viewport.New(viewport.WithWidth(cw), viewport.WithHeight(paneH))
		m.vp.SoftWrap = false // the pane's blocks are pre-rendered at cw; hard lines only
		m.vp.FillHeight = true
		m.vp.MouseWheelEnabled = true
		m.vp.MouseWheelDelta = 3
		m.vp.Style = lipgloss.NewStyle()
		m.vpOK = true
	}
	// viewport v2 has no width/height setters on an existing model; rebuild loses offsets, so
	// keep the scroll fraction across a resize instead of jumping blind
	m.vp.SetWidth(cw)
	m.vp.SetHeight(paneH)
	m.vpCW, m.vpH = cw, paneH
	return true
}

// feedVP serves the transcript + live rows into the viewport — only when either changed (or
// the pane did): an idle frame re-reads nothing. Follow mode: the pane hugs the bottom while
// the human hasn't scrolled away (AtBottom before the feed), and leaves them where they climbed
// when they have. Internal discipline: soft-wrap off, gutter off.
func (m *Model) feedVP(live []string, cw, paneH int) []string {
	resized := m.ensureVP(cw, paneH)
	lines := m.paneChat(cw)
	if resized || m.vpVer != m.paneVer || m.vpTrim != m.liveTrim || !slices.Equal(m.vpLive, live) {
		if trim := min(m.liveTrim, len(lines)); trim > 0 {
			lines = lines[: len(lines)-trim : len(lines)-trim]
		}
		if len(live) > 0 {
			lines = lines[:len(lines):len(lines)] // a copy: the cache stays whole
			for _, l := range live {
				lines = append(lines, strings.Split(l, "\n")...) // one content line a row
			}
		}
		m.vpTrim = m.liveTrim
		atBottom := m.vp.AtBottom()
		m.vpLines = lines
		m.vp.SetContentLines(lines)
		if atBottom {
			m.vp.GotoBottom()
		}
		m.vpVer, m.vpLive = m.paneVer, append(m.vpLive[:0], live...)
	}
	// the visible window, read straight off the content: the viewport would wrap and pad every
	// row through a style on each frame, and the pane's rows are already cut to its width
	off := min(max(m.vp.YOffset(), 0), len(m.vpLines))
	rows := make([]string, 0, paneH)
	rows = append(rows, m.vpLines[off:min(off+paneH, len(m.vpLines))]...)
	for len(rows) < paneH {
		rows = append(rows, "")
	}
	return rows
}

// vpKey routes the pane-scroll keys into the chat viewport while the panes hold and no menu,
// popup or completion owns the key line: pgup/pgdn page, shift+↑/↓ walk a line. ctrl+u/ctrl+d
// stay the textarea's; shift+↑/↓ stay its selection while the input spans several rows.
func (m *Model) vpKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	if !m.paneMode() || m.menu != nil || m.comp != nil || m.shortcuts || !m.vpOK {
		return nil, false
	}
	switch k.String() {
	case "pgup":
		m.vp.PageUp()
	case "pgdown":
		m.vp.PageDown()
	case "shift+up", "shift+down":
		if m.input.LineCount() > 1 || m.input.LineInfo().Height > 1 {
			return nil, false
		}
		if k.String() == "shift+up" {
			m.vp.ScrollUp(1)
		} else {
			m.vp.ScrollDown(1)
		}
	default:
		return nil, false
	}
	return nil, true
}

// vpWheel routes the wheel into the pane viewport while panes own the window. The wheel over
// the rail column walks the rail instead — the clamp lives in splitPanes, where content height
// is known.
func (m *Model) vpWheel(msg tea.MouseWheelMsg) (tea.Cmd, bool) {
	if !m.paneMode() {
		return nil, false
	}
	if l := m.geom(); l.rc > 0 && msg.Mouse().X >= l.divX {
		switch msg.Button {
		case tea.MouseWheelUp:
			m.railScroll--
		case tea.MouseWheelDown:
			m.railScroll++
		}
		return nil, true
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return cmd, true
}
