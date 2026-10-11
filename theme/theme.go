// Package theme is the binary's one source of colour: named palettes as data, every role the
// terminal UI and the dashboard hold, a dark and a light variant each, plain hex — no styles,
// no logic. The terminal builds its theme from a palette (tui/palette.go adapts it); the board
// generates the dashboard's role stylesheet from the same palette (CSS). Config picks one by
// name (ui.theme) and a mode (ui.mode), and both faces follow it.
package theme

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Swatch is one published colour of a palette: its name and hex.
type Swatch struct {
	Name, Hex string
}

// Roles is one variant (dark or light) of a palette, role → hex. The same roles in every
// palette and both variants: a surface never knows which scheme it is in.
type Roles struct {
	// the tonal anchors of the depth, from the deepest to the closest surface: the well sunk
	// below the plane, the ground under the frame, the planes (the conversation, the sidebar),
	// a card raised on a plane, a float over everything; Lift is the tone a lit edge moves
	// toward, Shade the one a shaded edge and a shadow sink toward
	Well, Ground, Plane, Side, Card, Float, Lift, Shade string
	// the text tones: the agent, its secondary heading (Label), the quiet one (Muted), the
	// human's own words (User); Border is quieter than any text, BorderStrong an input's edge
	Text, Label, Muted, User, Border, BorderStrong string
	// the hues: the primary (brand, focus, the live state, the answer) and the text a primary
	// fill holds (PrimaryInk), the accent (the human's marks), the verdicts, the plan's family
	Primary, PrimaryInk, Accent, OK, Warn, Danger, Plan string
	// Classes are the tool classes' hues (read, write, outward, destructive); Lanes the rank
	// lanes (orchestrator, coordinator, domain owner, zone worker, service owner, auditor: a Subagent's slab, its chip, the files it
	// wrote, its share of the work, a rank's hue on the dashboard)
	Classes, Lanes map[string]string
	// Selection is a drag selection's face; SelText the text on it
	Selection, SelText string
}

// Palette is a named scheme: its published swatches, the two variants built on them, and the
// notes that say how each role was derived and where a swatch was retuned.
type Palette struct {
	Name, Label, Source string
	Swatches            []Swatch
	Dark, Light         Roles
}

// Variant is the roles for a background.
func (p Palette) Variant(dark bool) Roles {
	if dark {
		return p.Dark
	}
	return p.Light
}

// Default is the palette both faces hold when config names none.
const Default = "cyberpunk"

// Modes are ui.mode's words: dark, light, or the system's (the terminal's background, the
// browser's prefers-color-scheme).
var Modes = []string{"dark", "light", "system"}

// All is every palette, in the order a menu lists them.
var All = []Palette{Cyberpunk, NeonDystopia, CorporateRebellion, RainCity, Synthwave, Glitch, Chromecore, Realm}

// Get is a palette by name.
func Get(name string) (Palette, bool) {
	for _, p := range All {
		if p.Name == strings.ToLower(strings.TrimSpace(name)) {
			return p, true
		}
	}
	return Palette{}, false
}

// Must is a palette by name, the default when the name is unknown.
func Must(name string) Palette {
	if p, ok := Get(name); ok {
		return p
	}
	p, _ := Get(Default)
	return p
}

// Names are the palettes' names, in menu order.
func Names() []string {
	out := make([]string, len(All))
	for i, p := range All {
		out[i] = p.Name
	}
	return out
}

// Check refuses an unknown palette or mode, naming the choices.
func Check(name, mode string) error {
	if _, ok := Get(name); !ok {
		return fmt.Errorf("unknown theme %q (%s)", name, strings.Join(Names(), " | "))
	}
	switch mode {
	case "dark", "light", "system":
		return nil
	}
	return fmt.Errorf("unknown mode %q (%s)", mode, strings.Join(Modes, " | "))
}

// LaneKeys are the rank lanes every variant carries; ClassKeys the tool classes.
var (
	LaneKeys  = []string{"orchestrator", "coord", "domain", "zone", "service", "auditor"}
	ClassKeys = []string{"read", "write", "outward", "destructive"}
)

// FamilyLane is the lane a value family holds when its rank has no lane of its own (a custom
// rank): the root the orchestrator's, a planner the coordinator's, a gate the domain owner's, an author the zone worker's,
// a steward the service owner's, an auditor the auditor's; generic none.
var FamilyLane = map[string]string{"root": "orchestrator", "planner": "coord", "gate": "domain", "author": "zone", "steward": "service", "auditor": "auditor"}

// --- colour arithmetic: sRGB hex, WCAG luminance and contrast ---

// RGB is a colour as 0..1 channels.
type RGB struct{ R, G, B float64 }

// Hex parses #rgb or #rrggbb (the alpha of #rrggbbaa is ignored).
func Hex(s string) RGB {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) < 6 {
		return RGB{}
	}
	ch := func(i int) float64 {
		v, _ := strconv.ParseUint(s[i:i+2], 16, 8)
		return float64(v) / 255
	}
	return RGB{ch(0), ch(2), ch(4)}
}

// String is the colour as #rrggbb.
func (c RGB) String() string {
	b := func(v float64) int { return int(math.Round(math.Min(math.Max(v, 0), 1) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", b(c.R), b(c.G), b(c.B))
}

// Mix is c moved t of the way toward d.
func (c RGB) Mix(d RGB, t float64) RGB {
	return RGB{c.R + (d.R-c.R)*t, c.G + (d.G-c.G)*t, c.B + (d.B-c.B)*t}
}

// Luminance is WCAG's relative luminance.
func (c RGB) Luminance() float64 {
	lin := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// Contrast is WCAG's contrast ratio between two colours.
func Contrast(a, b RGB) float64 {
	la, lb := a.Luminance(), b.Luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TextRoles are a variant's text-bearing roles by name — every one must hold 4.5:1 on every
// surface (TestEveryTextRoleReads).
func (r Roles) TextRoles() map[string]string {
	out := map[string]string{"text": r.Text, "label": r.Label, "muted": r.Muted, "user": r.User,
		"primary": r.Primary, "accent": r.Accent, "ok": r.OK, "warn": r.Warn, "danger": r.Danger, "plan": r.Plan}
	for _, k := range sortedKeys(r.Lanes) {
		out["lane."+k] = r.Lanes[k]
	}
	for _, k := range sortedKeys(r.Classes) {
		out["class."+k] = r.Classes[k]
	}
	return out
}

// Surfaces are a variant's surfaces by name: every place a text role can sit.
func (r Roles) Surfaces() map[string]string {
	return map[string]string{"well": r.Well, "ground": r.Ground, "plane": r.Plane, "side": r.Side, "card": r.Card, "float": r.Float}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
