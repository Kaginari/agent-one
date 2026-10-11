package config

import (
	"strings"
	"testing"
)

// ui.theme and ui.mode: the defaults are cyberpunk and dark; the local layer wins over the
// project's; an unknown palette is refused at load naming every palette, an unknown mode naming
// the three; explain theme says what is in force and lists the palettes.
func TestThemeConfig(t *testing.T) {
	c := newWorkspace(t, nil).load()
	if c.UI.Theme != "cyberpunk" || c.UI.Mode != "dark" {
		t.Fatalf("defaults: %q %q", c.UI.Theme, c.UI.Mode)
	}
	w := newWorkspace(t, map[string]string{
		".agent-one/config.yaml":       "ui:\n  theme: synthwave\n",
		".agent-one/config.local.yaml": "ui:\n  theme: Rain-City\n  mode: system\n",
	})
	c = w.load()
	if c.UI.Theme != "rain-city" || c.UI.Mode != "system" || !strings.Contains(c.Where("ui.theme"), "config.local.yaml") {
		t.Fatalf("the local layer: %q %q %s", c.UI.Theme, c.UI.Mode, c.Where("ui.theme"))
	}
	if ex := c.ExplainTheme(); !strings.Contains(ex, "▸ rain-city") || !strings.Contains(ex, "glitch") || !strings.Contains(ex, "#040C14") {
		t.Fatalf("explain theme:\n%s", ex)
	}
	err := newWorkspace(t, map[string]string{".agent-one/config.yaml": "ui:\n  theme: vaporwave\n"}).loadErr()
	if err == nil || !strings.Contains(err.Error(), "ui.theme") || !strings.Contains(err.Error(), "cyberpunk | neon-dystopia | corporate-rebellion | rain-city | synthwave | glitch | chromecore | realm") {
		t.Fatalf("an unknown theme names the list: %v", err)
	}
	err = newWorkspace(t, map[string]string{".agent-one/config.yaml": "ui:\n  mode: dim\n"}).loadErr()
	if err == nil || !strings.Contains(err.Error(), "dark | light | system") {
		t.Fatalf("an unknown mode names the three: %v", err)
	}
}
