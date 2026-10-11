package tui

// A palette is a named colour scheme as data: every colour role the TUI holds, a dark and a
// light variant, plain hex — no styles, no logic. The theme is built from one palette by one
// constructor (fromPalette), so a palette can be lifted into a package the dashboard shares
// without a change. The published swatches ride along as provenance: which colours the scheme
// started from, and where each role was retuned for contrast.

// Swatch is one published colour of a palette: its name and hex.
type Swatch struct {
	Name, Hex string
}

// Roles is one variant (dark or light) of a palette, role → hex.
type Roles struct {
	// the tonal anchors of the depth, from the deepest to the closest surface: the well sunk
	// below the plane, the ground under the frame, the planes (the conversation, the sidebar),
	// a card raised on a plane, a float over everything; Lift is the tone a lit edge moves
	// toward, Shade the one a shaded edge and a shadow sink toward
	Well, Ground, Plane, Side, Card, Float, Lift, Shade string
	// the text tones: the agent, its secondary heading (Label), the quiet one (Muted), the
	// human's own words (User), and Border — quieter than any text
	Text, Label, Muted, User, Border string
	// the hues: the primary (brand, focus, the live state, the answer), the accent (the
	// human's marks), the verdicts, the plan's family
	Primary, Accent, OK, Warn, Danger, Plan string
	// Classes are the tool classes' hues (read, write, outward, destructive); Lanes the rank
	// lanes (a Subagent's slab, its chip, the files it wrote, its share of the work)
	Classes, Lanes map[string]string
	// Selection is a drag selection's face; SelText the text on it
	Selection, SelText string
}

// Palette is a named scheme: its published swatches and the two variants built on them.
type Palette struct {
	Name, Source string
	Swatches     []Swatch
	Dark, Light  Roles
}

// Cyberpunk is omoro.io's "Cyberpunk" (neon dystopia — beauty in decay), five swatches: Void
// #0D1117, Smog Purple #1A0A2E, Danger Red #FF003C, Amber Haze #FF9900, Scanner Cyan #00FFFF.
//
// Dark is built on them: the ground is Void, the planes and cards step from Void toward Smog
// Purple, a lit edge rises toward a smog-lavender light; Scanner Cyan is the primary and Amber
// Haze the warning and the orchestrator's lane, both raw. Retuned for contrast (≥ 4.5:1 on every
// surface and every row of every gradient, TestDepthContrast) or derived where the palette has
// no such colour: Danger Red is lifted to #ff7a8c as text (raw it is 4.6:1 on Void and under 4
// on a raised card), its rose twin #ff6fa3 the human's accent; Smog Purple brought up to text
// lightness is the plan's violet (#b48cff) and, bluer, the auditor's indigo; a success register
// (#3cf0a0) and the domain owner's acid lime (#b4f03c) the palette lacks are interpolated between
// Scanner Cyan and Amber Haze; the zone worker's blue is Scanner Cyan turned toward blue (#2ec8ff).
// Light is the same scheme as deep ink on a smog-tinted paper.
var Cyberpunk = Palette{
	Name:   "cyberpunk",
	Source: "https://www.omoro.io/palettes/cyberpunk",
	Swatches: []Swatch{
		{"Void", "#0D1117"}, {"Smog Purple", "#1A0A2E"}, {"Danger Red", "#FF003C"},
		{"Amber Haze", "#FF9900"}, {"Scanner Cyan", "#00FFFF"},
	},
	Dark: Roles{
		Well: "#07090e", Ground: "#0d1117", Plane: "#14111f", Side: "#171326", Card: "#1f1835", Float: "#261d40",
		Lift: "#cbb8ff", Shade: "#030208",
		Text: "#e8e4f4", Label: "#cdc6e2", Muted: "#ada6c9", User: "#f1ecff", Border: "#3d3358",
		Primary: "#00ffff", Accent: "#ff6fa3", OK: "#3cf0a0", Warn: "#ff9900", Danger: "#ff7a8c", Plan: "#b48cff",
		Classes: map[string]string{"read": "#5cc8ff", "write": "#ff6fd8", "outward": "#ff9900", "destructive": "#ff7a8c"},
		Lanes: map[string]string{
			"orchestrator": "#ff9900", "coord": "#ff6fd8", "domain": "#b4f03c", "zone": "#2ec8ff", "service": "#ff7a8c", "auditor": "#a09dff",
		},
		Selection: "#2a3f63", SelText: "#f4fbff",
	},
	Light: Roles{
		Well: "#e5e0ee", Ground: "#f5f3f9", Plane: "#ebe7f3", Side: "#e6e1f0", Card: "#fbfaff", Float: "#ffffff",
		Lift: "#ffffff", Shade: "#4a3f66",
		Text: "#1d1630", Label: "#352c4c", Muted: "#5a5270", User: "#1a1230", Border: "#bfb5d4",
		Primary: "#00666e", Accent: "#a90f48", OK: "#07653e", Warn: "#814700", Danger: "#b8112e", Plan: "#6236bd",
		Classes: map[string]string{"read": "#005f87", "write": "#a3218e", "outward": "#814700", "destructive": "#b8112e"},
		Lanes: map[string]string{
			"orchestrator": "#874e00", "coord": "#a3218e", "domain": "#3d6200", "zone": "#005f87", "service": "#b8112e", "auditor": "#4b48c2",
		},
		Selection: "#c9f1f4", SelText: "#0b1424",
	},
}

// Palettes are the schemes a theme can be built from, by name.
var Palettes = map[string]Palette{Cyberpunk.Name: Cyberpunk}

// roles is the variant for a background.
func (p Palette) roles(dark bool) Roles {
	if dark {
		return p.Dark
	}
	return p.Light
}
