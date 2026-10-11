package config

import (
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/theme"
)

// checkTheme refuses a palette or a mode the binary does not ship, naming the choices.
func (c *Config) checkTheme() error {
	c.UI.Theme = strings.ToLower(strings.TrimSpace(c.UI.Theme))
	c.UI.Mode = strings.ToLower(strings.TrimSpace(c.UI.Mode))
	if c.UI.Theme == "" {
		c.UI.Theme = theme.Default
	}
	if c.UI.Mode == "" {
		c.UI.Mode = "dark"
	}
	if _, ok := theme.Get(c.UI.Theme); !ok {
		return fmt.Errorf("%s: ui.theme: unknown theme %q (%s)", c.Where("ui.theme"), c.UI.Theme, strings.Join(theme.Names(), " | "))
	}
	switch c.UI.Mode {
	case "dark", "light", "system":
	default:
		return fmt.Errorf("%s: ui.mode: %q is not %s", c.Where("ui.mode"), c.UI.Mode, strings.Join(theme.Modes, " | "))
	}
	return nil
}

// ExplainTheme is `config explain theme`: the palette and mode in force with their origins, then
// every palette the binary ships, its swatches and source.
func (c *Config) ExplainTheme() string {
	var b strings.Builder
	fmt.Fprintf(&b, "ui.theme %s  (%s)\nui.mode  %s  (%s)\n\n", c.UI.Theme, c.Where("ui.theme"), c.UI.Mode, c.Where("ui.mode"))
	for _, p := range theme.All {
		mark := " "
		if p.Name == c.UI.Theme {
			mark = "▸"
		}
		var sw []string
		for _, s := range p.Swatches {
			sw = append(sw, s.Hex)
		}
		fmt.Fprintf(&b, "%s %-20s %s\n    %s\n", mark, p.Name, strings.Join(sw, " "), p.Source)
	}
	b.WriteString("\nswitch live: /theme <name> [dark|light|system] in the terminal, the dashboard's theme menu, or\nui.theme / ui.mode in config — the live switches write the workspace's config.local.yaml.\n")
	return b.String()
}
