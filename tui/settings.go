package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// /settings: the session's view settings as one huh form floating over the frame — the theme,
// who owns the mouse, the sidebar. The form is a tea.Model living inside ours: while it is open
// every message reaches it first (its own field and group messages included), and its
// completion applies the picks. esc closes it unchanged.

type settingsState struct {
	form                  *huh.Form
	theme, mouse, sidebar string
}

func (m *Model) openSettings() tea.Cmd {
	st := &settingsState{theme: "dark", mouse: "panes", sidebar: "shown"}
	if !m.theme.Dark {
		st.theme = "light"
	}
	if m.mouseTerm {
		st.mouse = "terminal"
	}
	if m.railOff {
		st.sidebar = "hidden"
	}
	st.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("theme").Inline(true).
			Options(huh.NewOption("dark", "dark"), huh.NewOption("light", "light")).Value(&st.theme),
		huh.NewSelect[string]().Title("mouse").Inline(true).
			Options(huh.NewOption("panes", "panes"), huh.NewOption("terminal", "terminal")).Value(&st.mouse),
		huh.NewSelect[string]().Title("sidebar").Inline(true).
			Options(huh.NewOption("shown", "shown"), huh.NewOption("hidden", "hidden")).Value(&st.sidebar),
	)).WithTheme(huh.ThemeFunc(m.huhStyles)).WithShowHelp(false).WithWidth(40)
	m.settings = st
	m.menu, m.comp, m.palette = nil, nil, nil
	return st.form.Init()
}

// settingsUpdate hands a message to the open form; handled reports that a key was its (keys
// never fall through to the input while the form is open).
func (m *Model) settingsUpdate(msg tea.Msg) (tea.Cmd, bool) {
	st := m.settings
	if k, ok := msg.(tea.KeyPressMsg); ok && (k.String() == "esc" || k.String() == "ctrl+c") {
		m.settings = nil
		return nil, true
	}
	fm, cmd := st.form.Update(msg)
	if f, ok := fm.(*huh.Form); ok {
		st.form = f
	}
	_, isKey := msg.(tea.KeyPressMsg)
	switch st.form.State {
	case huh.StateCompleted:
		m.settings = nil
		return tea.Batch(cmd, m.applySettings(st)), true
	case huh.StateAborted:
		m.settings = nil
	}
	return cmd, isKey
}

func (m *Model) applySettings(st *settingsState) tea.Cmd {
	var cmds []tea.Cmd
	if dark := st.theme == "dark"; dark != m.theme.Dark {
		t := NewTheme(dark)
		t.Forced = true
		m.setTheme(t)
		cmds = append(cmds, m.reflow())
	}
	if term := st.mouse == "terminal"; term != m.mouseTerm {
		cmds = append(cmds, m.toggleMouse())
	}
	if off := st.sidebar == "hidden"; off != m.railOff {
		cmds = append(cmds, m.toggleSidebar())
	}
	m.say("settings saved for this session")
	return tea.Batch(cmds...)
}

// settingsView is the form in its card: the primary's edge (a floating focus), the keys under.
func (m *Model) settingsView() string {
	t := m.theme
	rows := []string{m.settings.form.View(), "", t.keys("←→ change · tab next · enter save · esc close")}
	return t.panelIn(t.accent, t.title("settings"), rows, 50)
}

// huhStyles dresses huh in the theme: titles in the label tone, the picked value in the
// primary, the arrows dim, a thin primary bar beside the focused field.
func (m *Model) huhStyles(bool) *huh.Styles {
	t := m.theme
	s := huh.ThemeBase(t.Dark)
	s.Focused.Base = lipgloss.NewStyle().PaddingLeft(1).BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(t.accent.GetForeground())
	s.Focused.Card = s.Focused.Base
	s.Focused.Title = t.label.Bold(true).Width(9)
	s.Focused.Option = t.accent.Bold(true)
	s.Focused.NextIndicator = t.dim.MarginLeft(1).SetString("→")
	s.Focused.PrevIndicator = t.dim.MarginRight(1).SetString("←")
	s.Blurred = s.Focused
	s.Blurred.Base = s.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	s.Blurred.Card = s.Blurred.Base
	s.Blurred.Title = t.dim.Width(9)
	s.Blurred.Option = t.text
	s.Blurred.NextIndicator = lipgloss.NewStyle()
	s.Blurred.PrevIndicator = lipgloss.NewStyle()
	s.FieldSeparator = lipgloss.NewStyle().SetString("\n")
	return s
}
