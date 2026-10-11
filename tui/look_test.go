package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Kaginari/agent-one/theme"
)

// themeHost keeps the look the way the app does: it records every switch.
type themeHost struct {
	fakeHost
	set [][2]string
}

func (h *themeHost) SetTheme(name, mode string) error {
	h.set = append(h.set, [2]string{name, mode})
	return nil
}

// run drains a command tree into the model (batches, sequences), as the program would.
func runCmd(m *Model, c tea.Cmd) {
	if c == nil {
		return
	}
	switch msg := c().(type) {
	case tea.BatchMsg:
		for _, x := range msg {
			runCmd(m, x)
		}
	case evLookSaved, EvTheme:
		m.Update(msg)
	}
}

// /theme <name> holds the palette at once and hands it to the host to keep; a mode word keeps the
// palette; /theme alone opens the picker; an unknown name lists the palettes and changes
// nothing; the dashboard's choice (EvTheme) is followed live.
func TestThemeSwitchesLive(t *testing.T) {
	h := &themeHost{}
	th := DetectThemeFor(func(string) string { return "" }, "AGENT_ONE_", "cyberpunk", "dark")
	th.Color, th.TrueColor = true, true
	m := New(h, th, DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m.Update(evBgWait{})
	m.Render()
	if m.theme.Name != "cyberpunk" || !m.theme.Dark || m.theme.Mode != "dark" {
		t.Fatalf("start: %s %v %q", m.theme.Name, m.theme.Dark, m.theme.Mode)
	}
	runCmd(m, m.slash("/theme synthwave"))
	if m.theme.Name != "synthwave" || !m.theme.Dark || len(h.set) != 1 || h.set[0] != [2]string{"synthwave", "dark"} {
		t.Fatalf("/theme synthwave: %s %v %v", m.theme.Name, m.theme.Dark, h.set)
	}
	if !strings.Contains(m.message, "saved to .agent-one/config.local.yaml") {
		t.Fatalf("the save is said: %q", m.message)
	}
	runCmd(m, m.slash("/theme light"))
	if m.theme.Name != "synthwave" || m.theme.Dark || h.set[1] != [2]string{"synthwave", "light"} {
		t.Fatalf("/theme light keeps the palette: %s %v", m.theme.Name, m.theme.Dark)
	}
	runCmd(m, m.slash("/theme"))
	if m.pick == nil {
		t.Fatal("/theme alone opens the picker")
	}
	runCmd(m, m.pickKey(tea.KeyPressMsg{Code: tea.KeyLeft}))
	runCmd(m, m.pickKey(tea.KeyPressMsg{Code: tea.KeyEnter}))
	if m.pick != nil || !m.theme.Dark || h.set[2] != [2]string{"synthwave", "dark"} {
		t.Fatalf("the picker's mode, kept: %v %v", m.theme.Dark, h.set)
	}
	runCmd(m, m.slash("/theme vaporwave"))
	if m.theme.Name != "synthwave" || len(h.set) != 3 {
		t.Fatal("an unknown palette changed the look")
	}
	m.Update(EvTheme{Name: "glitch", Mode: "light"})
	if m.theme.Name != "glitch" || m.theme.Dark || len(h.set) != 3 {
		t.Fatalf("the dashboard's choice: %s %v (a followed switch is not saved again: %v)", m.theme.Name, m.theme.Dark, h.set)
	}
	// the frame is drawn in the new palette: the glitch light ground behind the conversation
	if f := m.Render(); !strings.Contains(f, "245;245;245") && !strings.Contains(f, "236;236;236") {
		t.Fatalf("the frame did not repaint in glitch light:\n%s", ansi.Strip(f)[:200])
	}
	// system follows the terminal's answer
	m.Update(EvTheme{Name: "glitch", Mode: "system"})
	m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	if m.theme.Dark || m.theme.Name != "glitch" {
		t.Fatal("system on a light terminal is light, in the same palette")
	}
}

// The environment's word for this terminal beats config; config's mode beats the terminal's
// answer; system asks.
func TestDetectThemeFor(t *testing.T) {
	env := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	if th := DetectThemeFor(env(map[string]string{"AGENT_ONE_THEME": "light"}), "AGENT_ONE_", "realm", "dark"); th.Dark || th.Name != "realm" {
		t.Fatal("AGENT_ONE_THEME=light wins")
	}
	if th := DetectThemeFor(env(nil), "AGENT_ONE_", "rain-city", "light"); th.Dark || !th.Forced || th.Name != "rain-city" {
		t.Fatal("mode light")
	}
	if th := DetectThemeFor(env(nil), "AGENT_ONE_", "nope", "system"); th.Forced || th.Name != "cyberpunk" || th.Mode != "system" {
		t.Fatalf("system asks the terminal, an unknown palette is the default: %+v", th.Name)
	}
	if th := DetectThemeFor(env(map[string]string{"COLORFGBG": "0;15"}), "AGENT_ONE_", "glitch", "system"); th.Dark {
		t.Fatal("system reads COLORFGBG")
	}
}

// The picker lists every palette with its swatches; moving wears the palette at once without
// keeping it; esc goes back to the look it opened on; enter keeps the one under the cursor.
func TestThemePickerPreviewsAndKeeps(t *testing.T) {
	h := &themeHost{}
	th := DetectThemeFor(func(string) string { return "" }, "AGENT_ONE_", "cyberpunk", "dark")
	th.Color, th.TrueColor = true, true
	m := New(h, th, DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m.Update(evBgWait{})
	m.Render()
	runCmd(m, m.slash("/theme"))
	f := ansi.Strip(m.Render())
	for _, p := range theme.All {
		if !strings.Contains(f, p.Label) {
			t.Fatalf("the picker lacks %s:\n%s", p.Label, f)
		}
	}
	if !strings.Contains(f, "● in use") || !strings.Contains(f, "[dark]") {
		t.Fatalf("the picker marks the palette and mode in use:\n%s", f)
	}
	runCmd(m, m.pickKey(tea.KeyPressMsg{Code: tea.KeyDown}))
	if m.theme.Name != theme.All[1].Name || len(h.set) != 0 {
		t.Fatalf("down previews %s unsaved: %s %v", theme.All[1].Name, m.theme.Name, h.set)
	}
	runCmd(m, m.pickKey(tea.KeyPressMsg{Code: tea.KeyRight}))
	if m.theme.Dark {
		t.Fatal("right previews light")
	}
	runCmd(m, m.pickKey(tea.KeyPressMsg{Code: tea.KeyEscape}))
	if m.pick != nil || m.theme.Name != "cyberpunk" || !m.theme.Dark || len(h.set) != 0 {
		t.Fatalf("esc goes back: %s %v %v", m.theme.Name, m.theme.Dark, h.set)
	}
	runCmd(m, m.slash("/theme"))
	for i := 0; i < 4; i++ {
		runCmd(m, m.pickKey(tea.KeyPressMsg{Code: tea.KeyDown}))
	}
	runCmd(m, m.pickKey(tea.KeyPressMsg{Code: tea.KeyEnter}))
	if want := theme.All[4].Name; m.pick != nil || m.theme.Name != want || len(h.set) != 1 || h.set[0] != [2]string{want, "dark"} {
		t.Fatalf("enter keeps %s: %s %v", want, m.theme.Name, h.set)
	}
}

// The slash menu offers the terminal's own commands beside the host's: /settings, /sidebar …
// were answered but never listed.
func TestSlashMenuListsTheUIsCommands(t *testing.T) {
	m := New(&themeHost{}, DetectThemeFor(func(string) string { return "" }, "AGENT_ONE_", "cyberpunk", "dark"), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	for _, q := range []string{"/sett", "/side", "/them"} {
		m.setInput(q)
		m.refreshMenus()
		if m.menu == nil || len(m.menu.items) == 0 || !strings.HasPrefix(m.menu.items[0].Name, q[1:]) {
			t.Fatalf("%s: the menu %v", q, m.menu)
		}
	}
}

// A save that fails stays in the conversation, not only in the passing status line.
func TestThemeSaveFailureStays(t *testing.T) {
	m := New(&themeHost{}, DetectThemeFor(func(string) string { return "" }, "AGENT_ONE_", "cyberpunk", "dark"), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m.Update(evBgWait{})
	_, cmd := m.Update(evLookSaved{name: "glitch", mode: "dark", err: errors.New("edit refused")})
	if cmd != nil {
		cmd()
	}
	if tr := m.Transcript(); !strings.Contains(tr, "not saved: edit refused") {
		t.Fatalf("the failure is not in the conversation:\n%s", tr)
	}
}
