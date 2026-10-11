// Package tui is the live session's terminal UI (canon/tui.md): the conversation flows into the
// terminal's own scrollback as readable blocks, a bordered input sits at the bottom, and every
// event of the loop — a streamed answer, a tool step, a diff, a Subagent, the gate, an approval —
// is drawn, not printed raw. Built on the Charm libraries; the one place the binary takes
// third-party code.
//
// The package is layered by the ladder: blocks.go is the view model (pure functions from views
// to styled text), model.go the Bubble Tea program over it, and the app wires the loop's
// events into both.
package tui

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// Theme is the palette: the board's rank and lane tokens (board.css) carried to the terminal,
// a dark and a light variant, and nothing at all when the terminal has no colour (NO_COLOR,
// a pipe): lipgloss drops every escape on an ASCII profile, so a plain run is the same text.
type Theme struct {
	Dark  bool
	Color bool
	// Forced is true when the environment chose dark or light: the terminal's own answer
	// about its background is then not asked for.
	Forced bool

	dim, text, accent, user, ok, bad, warn, border, tag lipgloss.Style
	// the typographic system's two extra roles: label is the secondary heading (a card's
	// title, a key in a key-value grid) one step brighter than dim; magenta is the HUD's second
	// accent — the human's own marks (the ask's bar, the waiting chip), never a second accent
	// in a region that already holds the primary cyan
	label, magenta, plan      lipgloss.Style
	classes                   map[string]lipgloss.Style
	ranks                     map[string]lipgloss.Style
	lanes                     map[string]color.Color
	add, del, ctxLine, lineNo lipgloss.Style
	sel                       lipgloss.Style // a drag selection in the chat pane

	// TrueColor: the terminal draws 24-bit colour — the depth's tonal steps need it; a
	// 256-colour terminal would band them, so it keeps the flat frame.
	TrueColor bool
	// Depth: the surfaces are drawn as raised and sunken planes (depth.go). The model turns it
	// on while the frame owns the window on a truecolor terminal; the scrollback stays flat.
	Depth bool
	dep   depthScale
	pal   Roles // the palette's variant the theme was built from
}

// Words are the labels a distribution renames (agent-one says Subagent, Operator, Workspace).
type Words struct {
	Subagent  string // "subagent" | "subagent"
	Subagents string // plural
	Workspace string // "workspace" | "workspace"
	Human     string // "the operator" | "Operator"
	Gate      string // "gate"
	Dist      string // binary name
	ConfigRel string // where a don't-ask-again rule lands, workspace-relative
	// SessionName is the session agent's name ("orchestrator" | "orchestrator"): its events drive
	// the spinner, a Subagent's its block.
	SessionName string
}

// DefaultWords is the agent-one vocabulary.
func DefaultWords() Words {
	return Words{Subagent: "subagent", Subagents: "subagents", Workspace: "workspace", Human: "the operator", Gate: "gate", Dist: "agent-one", ConfigRel: ".agent-one/config.local.yaml", SessionName: "orchestrator"}
}

// NewTheme builds the theme for a dark or light background from the default palette.
func NewTheme(dark bool) Theme { return NewThemeFrom(Cyberpunk, dark) }

// laneAlias maps the names that ride another rank's lane onto it: agent-one's ranks, the
// role hats (analyst reads in the zone worker's ground-truth hue, judge verdicts in the domain owner's,
// drafter drafts in the coordinator's) and the promoted ranks.
var laneAlias = map[string]string{
	"orchestrator": "orchestrator", "coord": "coord", "domain": "domain", "zone": "zone", "service": "service", "auditor": "auditor",
	"analyst": "zone", "judge": "domain", "drafter": "coord",
	"principal-domain-owner": "domain",
}

// NewThemeFrom builds the theme from a palette — the one constructor that reads a palette's
// roles; nothing else in the package names a colour of the scheme.
func NewThemeFrom(p Palette, dark bool) Theme {
	pr := colorprofile.Detect(os.Stdout, os.Environ())
	t := Theme{Dark: dark, Color: pr != colorprofile.Ascii && pr != colorprofile.NoTTY, TrueColor: pr == colorprofile.TrueColor}
	r := p.roles(dark)
	t.pal = r
	t.dep = newDepthScale(dark, r)
	fg := func(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)) }
	// the roles: borders sit quieter than any text; text, then label, then dim carry the
	// hierarchy; the primary is the brand, the focus and the live state, the accent the human's
	// marks; ok/warn/bad say verdicts; plan is the deck's own family
	t.dim = fg(r.Muted)
	t.label = fg(r.Label)
	t.text = fg(r.Text)
	t.accent = fg(r.Primary)
	t.magenta = fg(r.Accent)
	t.user = fg(r.User)
	t.ok = fg(r.OK)
	t.bad = fg(r.Danger)
	t.warn = fg(r.Warn)
	t.plan = fg(r.Plan)
	t.border = fg(r.Border)
	t.tag = lipgloss.NewStyle().Bold(true)
	t.add = fg(r.OK)
	t.del = fg(r.Danger)
	t.ctxLine = fg(toRGB(lipgloss.Color(r.Muted)).mix(hexRGB(r.Ground), 0.15).hex())
	t.lineNo = fg(toRGB(lipgloss.Color(r.Muted)).mix(hexRGB(r.Ground), 0.4).hex())
	t.sel = lipgloss.NewStyle().Background(lipgloss.Color(r.Selection)).Foreground(lipgloss.Color(r.SelText))
	t.classes = map[string]lipgloss.Style{}
	for k, h := range r.Classes {
		st := fg(h)
		if k == "outward" || k == "destructive" {
			st = st.Bold(true)
		}
		t.classes[k] = st
	}
	// the rank lanes: the colour a Subagent's slab, chip and motion hold, the files it wrote, its
	// share of the work; a rank's ◆ and name hold the same hue
	t.lanes = map[string]color.Color{}
	t.ranks = map[string]lipgloss.Style{}
	for k, h := range r.Lanes {
		t.lanes[k] = lipgloss.Color(h)
		t.ranks[k] = fg(h)
	}
	for k, to := range laneAlias {
		if h, ok := r.Lanes[to]; ok {
			t.lanes[k] = lipgloss.Color(h)
			t.ranks[k] = fg(h)
		}
	}
	return t
}

// OnGround is the terminal's answer about its background. The palette owns the frame's ground
// (the depth paints every cell of the frame), so the answer no longer re-steps the surfaces:
// it only told the theme dark from light, which the caller already settled.
func (t Theme) OnGround(bg color.Color) Theme { return t }

// Ground is the background the depth's scale was stepped from.
func (t Theme) Ground() color.Color { return t.dep.ground.col() }

// lane is a rank's pulse colour; nil when the rank is unknown (callers fall back to the accent).
func (t Theme) lane(rank string) color.Color {
	if c, ok := t.lanes[strings.ToLower(rank)]; ok {
		return c
	}
	return nil
}

// Lane exposes lane() to the host (the rail plan card paints a deck in its author's rank
// lane); the paper lane for "" is nil.
func (t Theme) Lane(rank string) color.Color { return t.lane(rank) }

// paneBg is the sidebar's background: one honest solid step off the session background —
// painted cell by cell, never a transparency story.
func (t Theme) paneBg() color.Color { return t.dep.side.col() }

// paneArt re-holds the pane's background behind the mascot's foreground pixels: lipgloss'
// per-glyph full resets (\x1b[m / \x1b[0m) would otherwise drop the background mid-line, so
// they are rewritten into bg-restores.
func (t Theme) paneArt(art string) string {
	r, g, b, _ := t.paneBg().RGBA()
	sgr := fmt.Sprintf("48;2;%d;%d;%d", int(r>>8), int(g>>8), int(b>>8))
	art = strings.ReplaceAll(art, "\x1b[0m", "\x1b["+sgr+"m")
	return strings.ReplaceAll(art, "\x1b[m", "\x1b["+sgr+"m")
}

// onAccent is the face a fill in the primary holds: deep ink on the dark theme's neon, paper
// on the light theme's deep teal.
func (t Theme) onAccent() color.Color {
	if t.Dark {
		return t.dep.ground.col()
	}
	return t.dep.float.col()
}

// chip is a small tag held in a hue: the hue as its face over a tint of itself sunk into the
// panel — lit, never a neon slab. A colourless terminal reads the same word in brackets-free
// plain text.
func (t Theme) chip(hue color.Color, text string) string {
	if hue == nil {
		hue = t.dim.GetForeground()
	}
	if !t.Color {
		return text
	}
	base := t.paneBg()
	if t.Depth {
		base = t.dep.card.col() // a chip on a raised tile tints the tile's own tone
	}
	bg := lipgloss.Blend1D(5, base, hue)[1]
	fg := hue
	if !t.Dark {
		// a lane's light-theme hue is a mid tone: its chip face sinks a third toward ink so the
		// word holds 4.5:1 on its own tint
		fg = lipgloss.Blend1D(4, hue, lipgloss.Color("#000000"))[1]
	}
	return lipgloss.NewStyle().Foreground(fg).Background(bg).Padding(0, 1).Render(text)
}

// laneLegend is the welcome's foreshadow of the pulse lanes: each rank a chip in its own hue —
// orchestrator golden, coordinator orange, domain owner green, zone worker blue (agent-one's vocabulary follows its four
// mapped ranks).
func (t Theme) laneLegend(dist string) string {
	order := []string{"orchestrator", "coord", "domain", "zone"}
	if dist == "agent-one" {
		order = []string{"orchestrator", "coord", "domain", "zone"}
	}
	var parts []string
	for _, r := range order {
		parts = append(parts, t.chip(t.lane(r), r))
	}
	return strings.Join(parts, " ")
}

// Brand is the distribution's name on its gradient: agent-one holds the palette's primary → plan
// violet → accent (the scheme's cyan → smog → rose); another distribution keeps its sprite's
// own stops.
func (t Theme) Brand(dist string) string {
	if !t.Color {
		return "✦ " + dist
	}
	if dist == "agent-one" {
		return Gradient("✦ "+dist, true, t.accent.GetForeground(), t.plan.GetForeground(), t.magenta.GetForeground())
	}
	return Title(dist)
}

// DetectTheme picks dark unless the environment says the background is light (COLORFGBG's
// background code, or <PREFIX>_THEME=light); the terminal is never queried, so a start is never
// held by an unanswered escape.
func DetectTheme(env func(string) string, prefix string) Theme {
	if env == nil {
		env = os.Getenv
	}
	forced := func(dark bool) Theme {
		t := NewTheme(dark)
		t.Forced = true
		return t
	}
	if v := strings.ToLower(env(prefix + "THEME")); v == "light" {
		return forced(false)
	} else if v == "dark" {
		return forced(true)
	}
	if v := env("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		bg := parts[len(parts)-1]
		return forced(!(bg == "7" || bg == "15"))
	}
	return NewTheme(true) // until the terminal answers the background query
}

func (t Theme) class(name string) lipgloss.Style {
	if s, ok := t.classes[name]; ok {
		return s
	}
	return t.dim
}

func (t Theme) rank(name string) lipgloss.Style {
	if s, ok := t.ranks[strings.ToLower(name)]; ok {
		return s
	}
	return t.dim
}
