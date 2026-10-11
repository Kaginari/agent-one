package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
// palette; /theme alone flips dark and light; an unknown name lists the palettes and changes
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
	if !m.theme.Dark || h.set[2] != [2]string{"synthwave", "dark"} {
		t.Fatal("/theme alone flips")
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
