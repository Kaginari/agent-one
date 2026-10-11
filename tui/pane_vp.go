package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"
)

// The chat pane's viewport (A1 of the charm-libs wave): the transcript stops being a clip and
// becomes a scrollback — pgup/pgdn/ctrl+u/ctrl+d and the wheel walk it; new content follows the
// bottom only while the human never scrolled up. Present only while the panes own the window.

// ensureVP builds or resizes the chat viewport for the pane geometry (cw × paneH).
func (m *Model) ensureVP(cw, paneH int) {
	if m.vpOK && m.vpCW == cw && m.vpH == paneH {
		return
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
}

// feedVP serves the transcript + live rows into the viewport. Follow mode: the pane hugs the
// bottom while the human hasn't scrolled away (AtBottom before the feed), and leaves them where
// they climbed when they have. Internal discipline: soft-wrap off, gutter off (block landmarks
// are a later lift via LeftGutterFunc).
func (m *Model) feedVP(live []string, cw, paneH int) []string {
	m.ensureVP(cw, paneH)
	lines := m.paneChat(cw)
	if len(live) > 0 {
		lines = append(append([]string(nil), lines...), live...)
	}
	atBottom := m.vp.AtBottom()
	m.vp.SetContentLines(lines)
	if atBottom {
		m.vp.GotoBottom()
	}
	return strings.Split(m.vp.View(), "\n")
}

// vpKeys are the pane-scroll bindings the chat viewport owns (space stays the textarea's —
// pagedown-in-vp claims it but the input owns the floor).
var vpKeys = map[string]bool{"pgup": true, "pgdn": true, "ctrl+u": true, "ctrl+d": true}

// vpKey routes scroll keys into the pane viewport. Consumes true while panes hold and no menu,
// popup, or completion owns the key line.
func (m *Model) vpKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.railVisible() == 0 || m.menu != nil || m.comp != nil || m.shortcuts {
		return nil, false
	}
	if !vpKeys[k.String()] {
		return nil, false
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(k)
	return cmd, true
}

// vpWheel routes the wheel into the pane viewport while panes own the window. The wheel over
// the rail column walks the rail instead — the clamp lives in splitPanes, where content height
// is known.
func (m *Model) vpWheel(msg tea.MouseWheelMsg) (tea.Cmd, bool) {
	if m.railVisible() == 0 {
		return nil, false
	}
	if msg.Mouse().X > m.width-m.railVisible() {
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
