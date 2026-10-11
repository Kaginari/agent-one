package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Kaginari/agent-one/theme"
)

// The theme picker (/theme alone, the palette's "theme", /settings' theme row): every palette
// as a row with its swatches, the mode under them — the dashboard's theme menu in the terminal.
// Moving the cursor holds the palette at once (a preview, not kept); enter keeps it (written
// where config reads it, the dashboard following); esc goes back to the look the picker opened on.

var pickModes = []string{"dark", "light", "system"}

type themePick struct {
	cursor, mode int
	origName     string // the look the picker opened on, worn again on esc
	origMode     string
}

func (m *Model) openThemePick() tea.Cmd {
	p := &themePick{origName: m.theme.Name, origMode: m.theme.lookMode()}
	for i, pl := range theme.All {
		if pl.Name == p.origName {
			p.cursor = i
		}
	}
	for i, md := range pickModes {
		if md == p.origMode {
			p.mode = i
		}
	}
	m.pick = p
	m.menu, m.comp, m.palette, m.settings = nil, nil, nil, nil
	return nil
}

// pickKey handles a key while the picker is open: every key is the picker's.
func (m *Model) pickKey(k tea.KeyPressMsg) tea.Cmd {
	p := m.pick
	n := len(theme.All)
	switch k.String() {
	case "esc", "ctrl+c", "q":
		m.pick = nil
		if m.theme.Name != p.origName || m.theme.lookMode() != p.origMode {
			return m.applyLook(p.origName, p.origMode)
		}
		return nil
	case "up", "k", "ctrl+p", "shift+tab":
		p.cursor = (p.cursor + n - 1) % n
	case "down", "j", "ctrl+n", "tab":
		p.cursor = (p.cursor + 1) % n
	case "left", "h":
		p.mode = (p.mode + len(pickModes) - 1) % len(pickModes)
	case "right", "l", "m":
		p.mode = (p.mode + 1) % len(pickModes)
	case "enter":
		m.pick = nil
		return m.switchLook(theme.All[p.cursor].Name, pickModes[p.mode])
	default:
		return nil
	}
	return m.applyLook(theme.All[p.cursor].Name, pickModes[p.mode])
}

// pickView is the picker's card: one row per palette (its label, its swatches as blocks, the
// one kept marked), then the mode as a segmented switch.
func (m *Model) pickView() string {
	t, p := m.theme, m.pick
	namew := 0
	for _, pl := range theme.All {
		namew = max(namew, lipgloss.Width(pl.Label))
	}
	var rows []string
	for i, pl := range theme.All {
		var sw strings.Builder
		for _, s := range pl.Swatches {
			sw.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(s.Hex)).Render("██"))
		}
		label := pl.Label + strings.Repeat(" ", namew-lipgloss.Width(pl.Label))
		kept := ""
		if pl.Name == p.origName {
			kept = t.dim.Render("  ● in use")
		}
		if i == p.cursor {
			rows = append(rows, t.accent.Render("› ")+t.accent.Bold(true).Render(label)+"  "+sw.String()+kept)
		} else {
			rows = append(rows, "  "+t.text.Render(label)+"  "+sw.String()+kept)
		}
	}
	var seg []string
	for i, md := range pickModes {
		if i == p.mode {
			seg = append(seg, t.accent.Bold(true).Render("["+md+"]"))
		} else {
			seg = append(seg, t.dim.Render(" "+md+" "))
		}
	}
	rows = append(rows, "", t.label.Render(fmt.Sprintf("%-6s", "mode"))+strings.Join(seg, " "),
		"", t.keys("↑↓ theme · ←→ mode · enter keep · esc back"))
	return t.panelIn(t.accent, t.title("theme"), rows, 56)
}
