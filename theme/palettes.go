package theme

// The palettes. Each starts from its published swatches (omoro.io's cyberpunk family; realm is
// the dashboard's own neon HUD) and derives every role deliberately: which swatch is the ground,
// the primary, the accent, the danger; the roles a palette lacks interpolated between its
// swatches. Where a swatch fails 4.5:1 as text it is retuned, and the comment says so.

func lanes(orchestrator, coord, domain, zone, service, auditor string) map[string]string {
	return map[string]string{"orchestrator": orchestrator, "coord": coord, "domain": domain, "zone": zone, "service": service, "auditor": auditor}
}

func classes(read, write, outward, destructive string) map[string]string {
	return map[string]string{"read": read, "write": write, "outward": outward, "destructive": destructive}
}

// Cyberpunk is omoro.io's "Cyberpunk" (neon dystopia — beauty in decay), five swatches: Void
// #0D1117, Smog Purple #1A0A2E, Danger Red #FF003C, Amber Haze #FF9900, Scanner Cyan #00FFFF.
//
// Dark is built on them: the ground is Void, the planes and cards step from Void toward Smog
// Purple, a lit edge rises toward a smog-lavender light; Scanner Cyan is the primary and Amber
// Haze the warning and the orchestrator's lane, both raw. Retuned for contrast or derived where the
// palette has no such colour: Danger Red is lifted to #ff7a8c as text (raw it is 4.6:1 on Void
// and under 4 on a raised card), its rose twin #ff6fa3 the human's accent; Smog Purple brought
// up to text lightness is the plan's violet (#b48cff) and, bluer, the auditor's indigo; a
// success register (#3cf0a0) and the domain owner's acid lime (#b4f03c) the palette lacks are interpolated
// between Scanner Cyan and Amber Haze; the zone worker's blue is Scanner Cyan turned toward blue
// (#2ec8ff). Light is the same scheme as deep ink on a smog-tinted paper.
var Cyberpunk = Palette{
	Name: "cyberpunk", Label: "Cyberpunk", Source: "https://www.omoro.io/palettes/cyberpunk",
	Swatches: []Swatch{{"Void", "#0D1117"}, {"Smog Purple", "#1A0A2E"}, {"Danger Red", "#FF003C"}, {"Amber Haze", "#FF9900"}, {"Scanner Cyan", "#00FFFF"}},
	Dark: Roles{
		Well: "#07090e", Ground: "#0d1117", Plane: "#14111f", Side: "#171326", Card: "#1f1835", Float: "#261d40",
		Lift: "#cbb8ff", Shade: "#030208",
		Text: "#e8e4f4", Label: "#cdc6e2", Muted: "#ada6c9", User: "#f1ecff", Border: "#3d3358", BorderStrong: "#5a4d80",
		Primary: "#00ffff", PrimaryInk: "#0d1117", Accent: "#ff6fa3", OK: "#3cf0a0", Warn: "#ff9900", Danger: "#ff7a8c", Plan: "#b48cff",
		Classes:   classes("#5cc8ff", "#ff6fd8", "#ff9900", "#ff7a8c"),
		Lanes:     lanes("#ff9900", "#ff6fd8", "#b4f03c", "#2ec8ff", "#ff7a8c", "#a09dff"),
		Selection: "#2a3f63", SelText: "#f4fbff",
	},
	Light: Roles{
		Well: "#e5e0ee", Ground: "#f5f3f9", Plane: "#ebe7f3", Side: "#e6e1f0", Card: "#fbfaff", Float: "#ffffff",
		Lift: "#ffffff", Shade: "#4a3f66",
		Text: "#1d1630", Label: "#352c4c", Muted: "#5a5270", User: "#1a1230", Border: "#bfb5d4", BorderStrong: "#8f84a8",
		Primary: "#00666e", PrimaryInk: "#ffffff", Accent: "#a90f48", OK: "#07653e", Warn: "#814700", Danger: "#b8112e", Plan: "#6236bd",
		Classes:   classes("#005f87", "#a3218e", "#814700", "#b8112e"),
		Lanes:     lanes("#874e00", "#a3218e", "#3d6200", "#005f87", "#b8112e", "#4b48c2"),
		Selection: "#c9f1f4", SelText: "#0b1424",
	},
}

// NeonDystopia is omoro.io's "Neon Dystopia" (rain-soaked city block: danger in every neon
// puddle): Dark Street #040408, Smog Sky #080830, Danger Signal #FF0040, System Green #00FF88,
// Radiation #CCFF00.
//
// Dark: the ground is Dark Street, the planes and cards climb toward Smog Sky's navy, a lit edge
// rises to a smog-blue light. System Green is the primary and the domain owner's lane, raw; Radiation the
// human's accent and the orchestrator's lane, raw. Danger Signal is the danger and the service owner's lane,
// lifted to #fe677c (raw it is 5.2:1 on the ground, 4.7 on a navy card, under 4.5 on a
// slab's lit rows). Derived: the
// plan's violet (#8c8cff) and the auditor's (#a0a0ff) are Smog Sky brought to text lightness; a
// success aqua (#3cf0c8) between System Green and the sky, so ok never reads as the primary;
// the warning amber (#ffb020) between Danger Signal and Radiation; the zone worker's cyan (#4cc8ff)
// Smog Sky turned toward green; the coordinator's rose (#ff6fa0) Danger Signal softened. The text tones
// are a green-tinted white. Light is deep ink on a navy-tinted paper: every hue sunk to ≥ 4.5:1
// (System Green #066939, Radiation #4a6000, Danger Signal #b6002c).
var NeonDystopia = Palette{
	Name: "neon-dystopia", Label: "Neon Dystopia", Source: "https://www.omoro.io/palettes/neon-dystopia",
	Swatches: []Swatch{{"Dark Street", "#040408"}, {"Smog Sky", "#080830"}, {"Danger Signal", "#FF0040"}, {"System Green", "#00FF88"}, {"Radiation", "#CCFF00"}},
	Dark: Roles{
		Well: "#020204", Ground: "#040408", Plane: "#070718", Side: "#08081f", Card: "#0e0e3a", Float: "#14144a",
		Lift: "#b8b8ff", Shade: "#000000",
		Text: "#e4f2ec", Label: "#c4d6ce", Muted: "#9aaea6", User: "#f0fff8", Border: "#26264f", BorderStrong: "#3d3d70",
		Primary: "#00ff88", PrimaryInk: "#040408", Accent: "#ccff00", OK: "#3cf0c8", Warn: "#ffb020", Danger: "#fe677c", Plan: "#8c8cff",
		Classes:   classes("#4cc8ff", "#ff6fa0", "#ffb020", "#fe677c"),
		Lanes:     lanes("#ccff00", "#ff6fa0", "#00ff88", "#4cc8ff", "#fe677c", "#a0a0ff"),
		Selection: "#1e2a50", SelText: "#f4fff8",
	},
	Light: Roles{
		Well: "#e2e2ee", Ground: "#f3f3f8", Plane: "#e9e9f3", Side: "#e4e4f0", Card: "#fafaff", Float: "#ffffff",
		Lift: "#ffffff", Shade: "#2a2a50",
		Text: "#0a0a1e", Label: "#24243e", Muted: "#4e4e68", User: "#08081a", Border: "#c0c0d8", BorderStrong: "#8a8aa8",
		Primary: "#066939", PrimaryInk: "#ffffff", Accent: "#4a6000", OK: "#066657", Warn: "#864d00", Danger: "#b6002c", Plan: "#3a3ab8",
		Classes:   classes("#005f8a", "#a01d8b", "#864d00", "#b6002c"),
		Lanes:     lanes("#4d6100", "#b0124f", "#066939", "#005f8a", "#b6002c", "#4b48c2"),
		Selection: "#c8f5dc", SelText: "#04140c",
	},
}

// CorporateRebellion is omoro.io's "Corporate Rebellion" (megacorp brand colours turned against
// the corp itself): Corp Shadow #0C0C10, Dark Grey #303038, Warning Orange #FF6600, Corp Blue
// #0088FF, Report White #E8E8E8.
//
// Dark: the ground is Corp Shadow, the planes and cards step toward Dark Grey (the border is Dark
// Grey lifted), the text is Report White raw. Warning Orange is the primary and the orchestrator's
// lane, Corp Blue the accent and the zone worker's lane — both lifted to read on a raised grey slab
// (orange #ff8550, blue #55a9ff; raw they hold 6.7 and 5.6:1 on the ground, 5.5 and 4.6 on a card,
// and fall under 4.5 on a slab's lit rows). Derived:
// the danger (#fe8184) Warning Orange turned red, the warning amber (#ffb300) orange turned
// yellow, a success green (#3cd68c) and the plan's violet (#a999fd, Corp Blue turned violet) the
// palette lacks. Light is ink on a cool report paper; orange sinks to #a13c00, blue to #005ab6.
var CorporateRebellion = Palette{
	Name: "corporate-rebellion", Label: "Corporate Rebellion", Source: "https://www.omoro.io/palettes/corporate-rebellion",
	Swatches: []Swatch{{"Corp Shadow", "#0C0C10"}, {"Dark Grey", "#303038"}, {"Warning Orange", "#FF6600"}, {"Corp Blue", "#0088FF"}, {"Report White", "#E8E8E8"}},
	Dark: Roles{
		Well: "#070709", Ground: "#0c0c10", Plane: "#131318", Side: "#17171d", Card: "#202028", Float: "#282830",
		Lift: "#d8d8e8", Shade: "#020203",
		Text: "#e8e8e8", Label: "#cacace", Muted: "#a3a3ad", User: "#f4f4f4", Border: "#3c3c46", BorderStrong: "#55555f",
		Primary: "#ff8550", PrimaryInk: "#0c0c10", Accent: "#55a9ff", OK: "#3cd68c", Warn: "#ffb300", Danger: "#fe8184", Plan: "#a999fd",
		Classes:   classes("#55a9ff", "#d98cff", "#ffb300", "#fe8184"),
		Lanes:     lanes("#ff8550", "#ffb300", "#3cd68c", "#55a9ff", "#fe8184", "#a999fd"),
		Selection: "#2a3550", SelText: "#f8f8f8",
	},
	Light: Roles{
		Well: "#e4e4e8", Ground: "#f2f2f4", Plane: "#eaeaee", Side: "#e5e5ea", Card: "#fbfbfc", Float: "#ffffff",
		Lift: "#ffffff", Shade: "#303038",
		Text: "#121216", Label: "#2c2c34", Muted: "#55555f", User: "#0c0c10", Border: "#c4c4cc", BorderStrong: "#8c8c96",
		Primary: "#a13c00", PrimaryInk: "#ffffff", Accent: "#005ab6", OK: "#0b6b3e", Warn: "#875000", Danger: "#b8112e", Plan: "#5a3cc0",
		Classes:   classes("#005ab6", "#9a2a9a", "#875000", "#b8112e"),
		Lanes:     lanes("#a13c00", "#875000", "#0b6b3e", "#005ab6", "#b8112e", "#5a3cc0"),
		Selection: "#cfe2ff", SelText: "#0c0c10",
	},
}

// RainCity is omoro.io's "Rain City" (Bladerunner rain: wet asphalt reflecting kanji signage):
// Night Rain #040C14, Wet Asphalt #0A2840, Neon Reflection #0060C8, Kanji Red #FF2020, Rain
// Silver #A0B8C8.
//
// Dark: the ground is Night Rain, the planes and cards climb to Wet Asphalt, Rain Silver is the
// quiet text tone raw. Neon Reflection is the primary and the zone worker's lane, lifted to #5da8ff (raw
// it is 3.3:1 on the ground); Kanji Red is the danger and the service owner's lane, lifted to #ff7b75 (raw
// 5.1:1 on the ground, 4.2 on an asphalt card). Derived: the human's accent (#ff7ab0) Kanji Red
// turned rose, a lantern amber (#ffb84d) for the warning and the orchestrator, a success jade
// (#3cd6a0) and the plan's violet (#a598ff) between Neon Reflection and Kanji Red. Light is ink
// on rain-silver paper; Neon Reflection sinks to #0052ad, Kanji Red to #b80810.
var RainCity = Palette{
	Name: "rain-city", Label: "Rain City", Source: "https://www.omoro.io/palettes/rain-city-cyber",
	Swatches: []Swatch{{"Night Rain", "#040C14"}, {"Wet Asphalt", "#0A2840"}, {"Neon Reflection", "#0060C8"}, {"Kanji Red", "#FF2020"}, {"Rain Silver", "#A0B8C8"}},
	Dark: Roles{
		Well: "#02070c", Ground: "#040c14", Plane: "#07131f", Side: "#081726", Card: "#0c2236", Float: "#102a42",
		Lift: "#b8d4ea", Shade: "#010306",
		Text: "#e2ecf2", Label: "#c4d4de", Muted: "#a0b8c8", User: "#f0f6fa", Border: "#1e3a52", BorderStrong: "#2e5270",
		Primary: "#5da8ff", PrimaryInk: "#040c14", Accent: "#ff7ab0", OK: "#3cd6a0", Warn: "#ffb84d", Danger: "#ff7b75", Plan: "#a598ff",
		Classes:   classes("#5cc0ff", "#ff7ab0", "#ffb84d", "#ff7b75"),
		Lanes:     lanes("#ffb84d", "#ff7ab0", "#3cd6a0", "#5da8ff", "#ff7b75", "#a598ff"),
		Selection: "#163a5c", SelText: "#f0f8ff",
	},
	Light: Roles{
		Well: "#dde5ec", Ground: "#eef3f7", Plane: "#e5ecf2", Side: "#e0e8ef", Card: "#f9fbfd", Float: "#ffffff",
		Lift: "#ffffff", Shade: "#0a2840",
		Text: "#071422", Label: "#1f3346", Muted: "#4a5c6c", User: "#040c14", Border: "#b8c8d6", BorderStrong: "#7d93a6",
		Primary: "#0052ad", PrimaryInk: "#ffffff", Accent: "#b0124f", OK: "#036747", Warn: "#864e00", Danger: "#b80810", Plan: "#5038c0",
		Classes:   classes("#0052ad", "#a01d8b", "#864e00", "#b80810"),
		Lanes:     lanes("#864e00", "#b0124f", "#036747", "#0052ad", "#b80810", "#5038c0"),
		Selection: "#cde3f7", SelText: "#040c14",
	},
}

// Synthwave is omoro.io's "Synthwave" (neon void — the 80s that never was): Void #0D0221, Dark
// Purple #1A0533, Electric Purple #9400D3, Neon Magenta #FF00FF, Electric Cyan #00FFFF.
//
// Dark: the ground is Void, the planes and cards climb to Dark Purple and past it. Neon Magenta is
// the primary and the coordinator's lane, softened to #ff4dff (a
// pure #ff00ff on a purple slab vibrates);
// Electric Cyan the human's accent, raw. Electric Purple, too dark to read (3.1:1), is the plan's
// family brought to text lightness (#c07aff). Derived: a sunset amber (#ffb03c) for the warning
// and the orchestrator, a success register (#3cf0a0), a rose danger (#ff6a8c), the zone worker's cyan (#00e5ff)
// a step from the accent. Light is ink on a lilac paper; magenta sinks to #a3009b, cyan to
// #00666e.
var Synthwave = Palette{
	Name: "synthwave", Label: "Synthwave", Source: "https://www.omoro.io/palettes/synthwave",
	Swatches: []Swatch{{"Void", "#0D0221"}, {"Dark Purple", "#1A0533"}, {"Electric Purple", "#9400D3"}, {"Neon Magenta", "#FF00FF"}, {"Electric Cyan", "#00FFFF"}},
	Dark: Roles{
		Well: "#07010f", Ground: "#0d0221", Plane: "#13042b", Side: "#160530", Card: "#200a42", Float: "#2a0f52",
		Lift: "#e0b8ff", Shade: "#030008",
		Text: "#f0e6ff", Label: "#d6c6f0", Muted: "#b39cd6", User: "#fbf4ff", Border: "#3e1f66", BorderStrong: "#5a3388",
		Primary: "#ff4dff", PrimaryInk: "#0d0221", Accent: "#00ffff", OK: "#3cf0a0", Warn: "#ffb03c", Danger: "#ff6a8c", Plan: "#c07aff",
		Classes:   classes("#5cd8ff", "#ff4dff", "#ffb03c", "#ff6a8c"),
		Lanes:     lanes("#ffb03c", "#ff4dff", "#3cf0a0", "#00e5ff", "#ff6a8c", "#c07aff"),
		Selection: "#3a1a66", SelText: "#fff4ff",
	},
	Light: Roles{
		Well: "#ebe2f4", Ground: "#f7f2fb", Plane: "#efe7f7", Side: "#eae1f4", Card: "#fdfaff", Float: "#ffffff",
		Lift: "#ffffff", Shade: "#1a0533",
		Text: "#1a0833", Label: "#33204f", Muted: "#5c4878", User: "#0d0221", Border: "#d0bfe6", BorderStrong: "#9a84b8",
		Primary: "#a3009b", PrimaryInk: "#ffffff", Accent: "#00666e", OK: "#07653e", Warn: "#8a4a00", Danger: "#b4092b", Plan: "#6a1fb0",
		Classes:   classes("#005f87", "#a3009b", "#8a4a00", "#b4092b"),
		Lanes:     lanes("#8a4a00", "#a3009b", "#07653e", "#005f87", "#b4092b", "#6a1fb0"),
		Selection: "#f0c8f0", SelText: "#0d0221",
	},
}

// Glitch is omoro.io's "Glitch" (data corruption — beauty in error): Void #000000, Alert Red
// #FF0000, Data Cyan #00FFFF, Error Magenta #FF00FF, Bleed White #FFFFFF.
//
// Dark: Void is the well (a ground must rise above its well, so the ground is Void a step up,
// #060606), the planes and cards neutral greys; Bleed White is the user's words raw and, a step
// down (#f2f2f2), the body text. Data Cyan is the primary, raw; Error Magenta the human's accent
// and the coordinator's lane, lifted to #ff64fe; Alert Red the danger and the service owner's lane, lifted to
// #ff7874 (raw it is 5.3:1 on Void and 4.4 on a grey card). Derived from the RGB split the
// palette is made of: red + green bleed to the warning yellow (#ffd23c), cyan + green to the
// success register (#3cf0a0), magenta + blue to the plan's violet (#b88eff). Light is black ink on
// white; every hue sinks to ≥ 4.5:1.
var Glitch = Palette{
	Name: "glitch", Label: "Glitch", Source: "https://www.omoro.io/palettes/glitch",
	Swatches: []Swatch{{"Void", "#000000"}, {"Alert Red", "#FF0000"}, {"Data Cyan", "#00FFFF"}, {"Error Magenta", "#FF00FF"}, {"Bleed White", "#FFFFFF"}},
	Dark: Roles{
		Well: "#000000", Ground: "#060606", Plane: "#0c0c0e", Side: "#101012", Card: "#18181c", Float: "#222226",
		Lift: "#ffffff", Shade: "#000000",
		Text: "#f2f2f2", Label: "#d4d4d4", Muted: "#a6a6a6", User: "#ffffff", Border: "#333338", BorderStrong: "#55555c",
		Primary: "#00ffff", PrimaryInk: "#000000", Accent: "#ff64fe", OK: "#3cf0a0", Warn: "#ffd23c", Danger: "#ff7874", Plan: "#b88eff",
		Classes:   classes("#4cb4ff", "#ff64fe", "#ffd23c", "#ff7874"),
		Lanes:     lanes("#ffd23c", "#ff64fe", "#3cf0a0", "#4cb4ff", "#ff7874", "#b88eff"),
		Selection: "#003a44", SelText: "#ffffff",
	},
	Light: Roles{
		Well: "#e6e6e6", Ground: "#f5f5f5", Plane: "#ececec", Side: "#e8e8e8", Card: "#fcfcfc", Float: "#ffffff",
		Lift: "#ffffff", Shade: "#000000",
		Text: "#0a0a0a", Label: "#262626", Muted: "#525252", User: "#000000", Border: "#c8c8c8", BorderStrong: "#8a8a8a",
		Primary: "#00666e", PrimaryInk: "#ffffff", Accent: "#a3009b", OK: "#07653e", Warn: "#745600", Danger: "#bb0000", Plan: "#5a2fc0",
		Classes:   classes("#0057a8", "#a3009b", "#745600", "#bb0000"),
		Lanes:     lanes("#745600", "#a3009b", "#07653e", "#0057a8", "#bb0000", "#5a2fc0"),
		Selection: "#c8f4f4", SelText: "#000000",
	},
}

// Chromecore is omoro.io's "Chromecore" (liquid metal — mirror and machine): Void Steel #1A2030,
// Dark Steel #4A5568, Gunmetal #8090A8, Steel #C0C8D8, Chrome White #E8E8F0.
//
// A monochrome palette: dark is built on its steps — Void Steel the ground, the cards climb toward
// Dark Steel (the border, raw), Chrome White the text raw. Steel was the label and Gunmetal the
// quiet tone; both are retuned: Gunmetal reads 5.0:1 on the ground but 3.9 on a raised card, so
// the quiet tone is Gunmetal lifted to #b3c3db and the label rises to #d4dae6 to keep the step.
// The hues the palette lacks are the chrome's sheen, pale and cool: the primary an ice blue
// (#9cc8ff, Gunmetal's hue given chroma), the accent an iridescent lilac (#e0b0ff), a pale register
// ok, a brass warning (#f0c878), a pale rose danger (#feaeb3), a lavender plan (#c6b8ff). Light
// is Void Steel ink on Chrome White paper, Dark Steel the quiet tone raw, Steel the border.
var Chromecore = Palette{
	Name: "chromecore", Label: "Chromecore", Source: "https://www.omoro.io/palettes/chromecore",
	Swatches: []Swatch{{"Void Steel", "#1A2030"}, {"Dark Steel", "#4A5568"}, {"Gunmetal", "#8090A8"}, {"Steel", "#C0C8D8"}, {"Chrome White", "#E8E8F0"}},
	Dark: Roles{
		Well: "#12161f", Ground: "#1a2030", Plane: "#1f2637", Side: "#222a3c", Card: "#2a3347", Float: "#333d54",
		Lift: "#e8e8f0", Shade: "#0a0d14",
		Text: "#e8e8f0", Label: "#d4dae6", Muted: "#b3c3db", User: "#f6f6fa", Border: "#4a5568", BorderStrong: "#6a7890",
		Primary: "#9cc8ff", PrimaryInk: "#1a2030", Accent: "#e0b0ff", OK: "#8ee0b4", Warn: "#f0c878", Danger: "#feaeb3", Plan: "#c6b8ff",
		Classes:   classes("#9cc8ff", "#e0b0ff", "#f0c878", "#feaeb3"),
		Lanes:     lanes("#f0c878", "#e0b0ff", "#8ee0b4", "#9cc8ff", "#feaeb3", "#c6b8ff"),
		Selection: "#3a4a66", SelText: "#f6f6fa",
	},
	Light: Roles{
		Well: "#dcdfe8", Ground: "#e8e8f0", Plane: "#e2e4ec", Side: "#dee1ea", Card: "#f4f5f9", Float: "#ffffff",
		Lift: "#ffffff", Shade: "#1a2030",
		Text: "#141a28", Label: "#2c3446", Muted: "#4a5568", User: "#1a2030", Border: "#c0c8d8", BorderStrong: "#8090a8",
		Primary: "#1859a2", PrimaryInk: "#ffffff", Accent: "#773ca5", OK: "#166442", Warn: "#755200", Danger: "#aa1e34", Plan: "#5a46b8",
		Classes:   classes("#1859a2", "#8a2f8a", "#755200", "#aa1e34"),
		Lanes:     lanes("#755200", "#773ca5", "#166442", "#1859a2", "#aa1e34", "#5a46b8"),
		Selection: "#c8d8f0", SelText: "#141a28",
	},
}

// Realm is the dashboard's own neon HUD (it was board/web/theme.css's literal values): a black
// ground, neon cyan and magenta brackets, a signal green.
//
// Lifted unchanged: the ground, panel (the card), sunken (the well), border, text, muted, primary,
// its ink, ok and warn, and the rank hues but two. The terminal's raised slabs need more: the
// planes and the float are stepped between the panel and the border, a label and the user's tone
// added; the magenta accent (#ff2bf1 → #fe41f0) and the danger (#ff4d6a → #fe677a) rise a step to
// read on a lane-tinted slab, the plan's violet is the auditor's hue (#a47bff → #a680ff). Light is
// theme.css's light block: the primary sinks #00707a → #006771, ok #00764a → #066942, warn
// #8a5300 → #865100, danger #c0002a → #bb0029, the service owner's lime #3d6e00 → #396800 — each for a
// gradient row of the terminal's light slabs.
var Realm = Palette{
	Name: "realm", Label: "Realm", Source: "board/web/theme.css (the dashboard's neon HUD)",
	Swatches: []Swatch{{"Ground", "#06070A"}, {"Panel", "#0C0F14"}, {"Neon Cyan", "#00F3FF"}, {"Neon Magenta", "#FF2BF1"}, {"Signal Green", "#2BFF95"}},
	Dark: Roles{
		Well: "#030405", Ground: "#06070a", Plane: "#090b0f", Side: "#0a0d11", Card: "#0c0f14", Float: "#141a21",
		Lift: "#bfefff", Shade: "#000000",
		Text: "#e6f7fa", Label: "#c0ced8", Muted: "#8c9aa8", User: "#f2fcfd", Border: "#243039", BorderStrong: "#3a4a57",
		Primary: "#00f3ff", PrimaryInk: "#001a1c", Accent: "#fe41f0", OK: "#2bff95", Warn: "#ffc23d", Danger: "#fe677a", Plan: "#a680ff",
		Classes:   classes("#3da9ff", "#fe41f0", "#ffc23d", "#fe677a"),
		Lanes:     lanes("#00f3ff", "#ffb627", "#fe41f0", "#3da9ff", "#b8ff3d", "#a680ff"),
		Selection: "#0a3a44", SelText: "#f2fcfd",
	},
	Light: Roles{
		Well: "#e3e8ee", Ground: "#eef2f6", Plane: "#e8edf2", Side: "#e4e9ef", Card: "#ffffff", Float: "#ffffff",
		Lift: "#ffffff", Shade: "#0a0e14",
		Text: "#0a0e14", Label: "#2a333f", Muted: "#4b5563", User: "#0a0e14", Border: "#cbd5df", BorderStrong: "#8e9ba8",
		Primary: "#006771", PrimaryInk: "#ffffff", Accent: "#a3009b", OK: "#066942", Warn: "#865100", Danger: "#bb0029", Plan: "#6a2fd6",
		Classes:   classes("#0057c2", "#a3009b", "#865100", "#bb0029"),
		Lanes:     lanes("#006771", "#865100", "#a3009b", "#0057c2", "#396800", "#6a2fd6"),
		Selection: "#c4eef2", SelText: "#0a0e14",
	},
}
