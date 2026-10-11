package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Kaginari/agent-one/theme"
)

// The look: which palette the session holds (ui.theme) and its variant (ui.mode: dark, light,
// or the system's — the terminal's background). /theme, /settings and the dashboard's menu
// switch it live; the host writes the choice to the workspace's config.local.yaml and tells the
// dashboard, which swaps its stylesheet.

// EvTheme is a look chosen elsewhere (the dashboard's menu, another surface): the terminal
// follows it.
type EvTheme struct {
	Name, Mode string
}

// ThemeHost is a host that keeps the look: it writes the choice where config reads it and
// tells every surface (EvTheme to the terminal, a theme event on the session stream). A host
// without it switches for the session only.
type ThemeHost interface {
	SetTheme(name, mode string) error
}

// evLookSaved is the host's answer to a switch.
type evLookSaved struct {
	name, mode string
	err        error
}

// DetectThemeFor builds the theme config chose: palette name in mode. <PREFIX>_THEME=dark|light
// in the environment still wins (the human's word for this terminal); "system" reads COLORFGBG,
// else asks the terminal for its background (until it answers: dark).
func DetectThemeFor(env func(string) string, prefix, name, mode string) Theme {
	if env == nil {
		env = os.Getenv
	}
	forced := func(dark bool) Theme {
		t := ThemeNamed(name, dark)
		t.Forced, t.Mode = true, mode
		return t
	}
	switch strings.ToLower(env(prefix + "THEME")) {
	case "light":
		return forced(false)
	case "dark":
		return forced(true)
	}
	switch mode {
	case "light":
		return forced(false)
	case "system":
	default:
		t := forced(true)
		t.Mode = "dark"
		return t
	}
	if v := env("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		bg := parts[len(parts)-1]
		return forced(!(bg == "7" || bg == "15"))
	}
	t := ThemeNamed(name, true) // until the terminal answers the background query
	t.Mode = "system"
	return t
}

// lookMode is the theme's mode word (a theme built without one reads its background).
func (t Theme) lookMode() string {
	if t.Mode != "" {
		return t.Mode
	}
	if t.Dark {
		return "dark"
	}
	return "light"
}

// applyLook holds palette name in mode: the frame is rebuilt in the new colours at once.
func (m *Model) applyLook(name, mode string) tea.Cmd {
	if _, ok := theme.Get(name); !ok {
		name = m.theme.Name
	}
	dark := m.theme.Dark
	switch mode {
	case "dark":
		dark = true
	case "light":
		dark = false
	case "system":
		if m.sysKnown {
			dark = m.sysDark
		}
	default:
		mode = m.theme.lookMode()
	}
	t := ThemeNamed(name, dark)
	t.Mode, t.Forced = mode, mode != "system"
	// the terminal's capabilities do not change with the palette
	t.Color, t.TrueColor = m.theme.Color, m.theme.TrueColor
	m.setTheme(t)
	return m.reflow()
}

// switchLook is a switch the human made here: held at once, then kept by the host.
func (m *Model) switchLook(name, mode string) tea.Cmd {
	cmd := m.applyLook(name, mode)
	name, mode = m.theme.Name, m.theme.lookMode()
	h, ok := m.host.(ThemeHost)
	if !ok {
		m.say(fmt.Sprintf("theme %s · %s — for this session", name, mode))
		return cmd
	}
	return tea.Batch(cmd, func() tea.Msg { return evLookSaved{name, mode, h.SetTheme(name, mode)} })
}

// themeSlash is /theme [name] [dark|light|system]: alone it opens the picker; a palette's
// name holds it in the mode in force; a mode word alone keeps the palette; "list" (or anything
// unknown) prints the palettes.
func (m *Model) themeSlash(rest string) tea.Cmd {
	name, mode := m.theme.Name, m.theme.lookMode()
	args := strings.Fields(strings.ToLower(rest))
	if len(args) == 0 {
		return m.openThemePick()
	}
	for _, a := range args {
		switch {
		case a == "dark" || a == "light" || a == "system":
			mode = a
		case a == "list":
			return m.themeList("")
		default:
			if _, ok := theme.Get(a); !ok {
				return m.themeList(fmt.Sprintf("no theme %q — ", a))
			}
			name = a
		}
	}
	return m.switchLook(name, mode)
}

// themeList prints the palettes, the one held marked, each with its swatches.
func (m *Model) themeList(lead string) tea.Cmd {
	cur := m.theme.Name
	lines := []string{lead + "themes (/theme <name> [dark|light|system]):"}
	for _, p := range theme.All {
		mark := "  "
		if p.Name == cur {
			mark = "▸ "
		}
		var sw []string
		for _, s := range p.Swatches {
			sw = append(sw, s.Hex)
		}
		lines = append(lines, fmt.Sprintf("%s%-20s %s", mark, p.Name, strings.Join(sw, " ")))
	}
	lines = append(lines, "mode: "+m.theme.lookMode())
	return m.print(func(w int) string { return m.theme.Notice(strings.Join(lines, "\n"), w) })
}
