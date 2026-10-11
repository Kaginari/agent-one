package tui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Depth: the frame's surfaces as layered planes, drawn in the cell grid with colour and the
// block elements alone. A ground (the terminal's own background), the planes on it (the
// conversation, the sidebar, the header and the status bar), the cards raised on the planes
// (the ask, the answer, a tool, a Subagent, a sidebar tile), the floats over everything (a toast,
// the ask card, the palette), and the one surface sunk below its plane: the input well.
//
//	elevation  -1 well   0 ground   1 plane   2 card   3 float
//
// Every surface is a step of one tonal scale taken from the ground — on a dark ground a closer
// surface is lighter, on a light ground whiter, the planes a soft grey under them — tinted a
// few percent toward the hue that says what it is (the human's magenta on the ask, the primary
// on the answer, a tool's class, a Subagent's rank lane). A raised surface holds a lit top face
// and a shaded bottom edge (half blocks), and casts a shadow one cell right and half a row
// down, tinted from the plane it falls on — never black. The well is the inverse: its top edge
// in shadow, its bottom edge catching the light. Truecolor only: a 256-colour or colourless
// terminal draws the flat frame.

// rgb is a colour in sRGB, 0–255 a channel.
type rgb struct{ r, g, b float64 }

func toRGB(c color.Color) rgb {
	if c == nil {
		return rgb{128, 128, 128}
	}
	r, g, b, _ := c.RGBA()
	return rgb{float64(r >> 8), float64(g >> 8), float64(b >> 8)}
}

func hexRGB(s string) rgb { return toRGB(lipgloss.Color(s)) }

// mix moves a toward b by t (0: a, 1: b).
func (a rgb) mix(b rgb, t float64) rgb {
	return rgb{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

func (a rgb) hex() string {
	c := func(v float64) int { return int(math.Round(min(max(v, 0), 255))) }
	return fmt.Sprintf("#%02x%02x%02x", c(a.r), c(a.g), c(a.b))
}

func (a rgb) col() color.Color { return lipgloss.Color(a.hex()) }

// lum is the relative luminance (WCAG 2).
func (a rgb) lum() float64 {
	ch := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(a.r) + 0.7152*ch(a.g) + 0.0722*ch(a.b)
}

// contrast is the WCAG contrast ratio of two colours.
func contrast(a, b rgb) float64 {
	la, lb := a.lum()+0.05, b.lum()+0.05
	if la < lb {
		la, lb = lb, la
	}
	return la / lb
}

// surf is one surface as the grid draws it: its face (the gradient's middle), the gloss of its
// lit top face, the crisp lit left bevel and the two-tone shaded right and bottom bevel, the
// shadow it casts (nil: none) and the shadow's far, fading column, a spine down its left edge
// in a hue (nil: none), and the rim glow an active surface holds (nil: none).
type surf struct {
	face, lit, shade, shadow, spine color.Color
	// band is the lit header strip's colour (a tile's title row); nil holds lit
	band color.Color
	// top and bot are the face's gradient: the first row a step lit, the last a step shaded
	top, bot color.Color
	// edge is the right bevel's inner half-tone (the shade's first tone); gloss the bright
	// half-row under the top edge
	edge, gloss color.Color
	// shadow2 is the shadow's far column: a half-tone toward the plane it falls on
	shadow2 color.Color
	glow    color.Color
}

// shadowW is the columns a raised surface's shadow takes right of it: the near one dark, the
// far one a half-width fade.
const shadowW = 2

// depthScale is the theme's tonal scale, taken from the palette's anchors.
type depthScale struct {
	ground, plane, side, well, card, float rgb
	shadow, deep                           rgb // the cast shadow on a plane; a float's
	lift, sink                             rgb // the tones a lit edge and a shaded one move toward
}

// newDepthScale reads the palette's tonal anchors: the well below the plane, the ground, the
// planes, the card, the float; the shadows are stepped from the plane toward the shade.
func newDepthScale(dark bool, r Roles) depthScale {
	d := depthScale{
		ground: hexRGB(r.Ground), plane: hexRGB(r.Plane), side: hexRGB(r.Side), well: hexRGB(r.Well),
		card: hexRGB(r.Card), float: hexRGB(r.Float), lift: hexRGB(r.Lift), sink: hexRGB(r.Shade),
	}
	if dark {
		d.shadow = d.plane.mix(d.sink, 0.78)
		d.deep = d.plane.mix(d.sink, 0.95)
	} else {
		d.shadow = d.plane.mix(d.sink, 0.22)
		d.deep = d.plane.mix(d.sink, 0.34)
	}
	return d
}

// raise builds a raised surface over the base face b, tinted k toward hue, casting shadow on
// plane: the gradient a step lit at the top and shaded at the bottom, the gloss a closer step
// still, the right bevel two tones deep, the shadow's far column fading into the plane.
func (t Theme) raise(b rgb, hue color.Color, k float64, shadow, plane rgb, spine color.Color) surf {
	if hue != nil {
		b = b.mix(toRGB(hue), k)
	}
	d := t.dep
	s := surf{face: b.col(), shadow: shadow.col(), shadow2: shadow.mix(plane, 0.5).col(), spine: spine}
	var top, bot, lit, gloss, shade, band rgb
	if t.Dark {
		top, bot = b.mix(d.lift, 0.065), b.mix(d.sink, 0.2)
		lit = b.mix(d.lift, 0.2)
		gloss = b.mix(d.lift, 0.3)
		shade = b.mix(d.sink, 0.42)
		band = b.mix(d.lift, 0.035)
	} else {
		top, bot = b.mix(d.lift, 0.8), b.mix(d.sink, 0.045)
		lit = b.mix(d.lift, 0.9)
		gloss = d.lift
		shade = b.mix(d.sink, 0.3)
		band = b.mix(d.plane, 0.45)
	}
	if hue != nil {
		h := toRGB(hue)
		// the band carries a breath more of the hue (the tile's name sits in its own colour),
		// the gloss a touch (a lit face in its own light)
		band = band.mix(h, k*0.6)
		gloss = gloss.mix(h, k*1.2)
		lit = lit.mix(h, k)
	}
	s.top, s.bot, s.lit, s.gloss, s.shade, s.band = top.col(), bot.col(), lit.col(), gloss.col(), shade.col(), band.col()
	s.edge = b.mix(shade, 0.4).col()
	return s
}

// glowing turns a surface's rim to light in hue: the gloss and the lit bevel lean into it and
// the shadow becomes its spill on the plane — an active surface (a running Subagent, a fresh
// toast) gives off its colour instead of a shade.
func (t Theme) glowing(s surf, hue color.Color, plane rgb) surf {
	if hue == nil || !t.Depth {
		return s
	}
	h := toRGB(hue)
	s.glow = hue
	s.gloss = toRGB(s.gloss).mix(h, 0.35).col()
	s.lit = toRGB(s.lit).mix(h, 0.3).col()
	k := 0.2
	if !t.Dark {
		k = 0.16
	}
	s.shadow = plane.mix(h, k).col()
	s.shadow2 = plane.mix(h, k*0.45).col()
	return s
}

// card is a raised card on a plane; floatSurf a float over the frame (the deepest shadow).
func (t Theme) card(hue color.Color, k float64, spine color.Color) surf {
	return t.raise(t.dep.card, hue, k, t.dep.shadow, t.dep.plane, spine)
}

func (t Theme) floatSurf(hue color.Color, k float64, spine color.Color) surf {
	return t.raise(t.dep.float, hue, k, t.dep.deep, t.dep.plane, spine)
}

// sgrBg is the escape that sets a background.
func sgrBg(c color.Color) string {
	if c == nil {
		return "\x1b[49m"
	}
	r := toRGB(c)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", int(r.r), int(r.g), int(r.b))
}

func sgrFg(c color.Color) string {
	r := toRGB(c)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", int(r.r), int(r.g), int(r.b))
}

// sgrBgEffect reads an SGR's parameters: reset when it clears the background (a full reset,
// or 49), set when it names one of its own.
func sgrBgEffect(params string) (reset, set bool) {
	if params == "" {
		return true, false
	}
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		p := ps[i]
		if j := strings.IndexByte(p, ':'); j >= 0 {
			// a colon-joined colour: one parameter
			if p[:j] == "48" {
				reset, set = false, true
			}
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil && p != "" {
			continue
		}
		switch {
		case p == "" || n == 0 || n == 49:
			reset, set = true, false
		case n == 38 || n == 48 || n == 58:
			if n == 48 {
				reset, set = false, true
			}
			if i+1 < len(ps) && ps[i+1] == "5" {
				i += 2
			} else if i+1 < len(ps) && ps[i+1] == "2" {
				i += 4
			}
		case n >= 40 && n <= 47, n >= 100 && n <= 107:
			reset, set = false, true
		}
	}
	return reset, set
}

// paint lays a background under a styled line, cell by cell: bgAt(x) is the colour at cell x.
// The line's own backgrounds (a chip, a code span) stay theirs; wherever the line clears its
// background (a reset, 49) the painted one comes back. The result ends reset.
func paint(s string, bgAt func(x int) color.Color) string {
	var b strings.Builder
	b.Grow(len(s) + 64)
	var state byte
	x := 0
	var cur color.Color
	curSet := false
	own := false
	for len(s) > 0 {
		seq, w, n, ns := ansi.DecodeSequence(s, state, nil)
		state = ns
		if n <= 0 {
			break
		}
		if w == 0 {
			b.WriteString(seq)
			if len(seq) > 2 && seq[0] == 0x1b && seq[1] == '[' && seq[len(seq)-1] == 'm' {
				reset, set := sgrBgEffect(seq[2 : len(seq)-1])
				if reset {
					own, curSet = false, false
				}
				if set {
					own = true
				}
			}
			s = s[n:]
			continue
		}
		if !own {
			c := bgAt(x)
			if !curSet || c != cur {
				b.WriteString(sgrBg(c))
				cur, curSet = c, true
			}
		}
		b.WriteString(seq)
		x += w
		s = s[n:]
	}
	b.WriteString("\x1b[m")
	return b.String()
}

// paintBg is paint in one colour — no cell walk needed: the colour opens the line and comes
// back after every escape that clears it.
func paintBg(s string, bg color.Color) string {
	set := sgrBg(bg)
	var b strings.Builder
	b.Grow(len(s) + len(set)*4 + 4)
	b.WriteString(set)
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			break
		}
		j := i + 2
		for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == ';' || s[j] == ':') {
			j++
		}
		if j >= len(s) {
			break
		}
		b.WriteString(s[:j+1])
		if s[j] == 'm' {
			if reset, _ := sgrBgEffect(s[i+2 : j]); reset {
				b.WriteString(set)
			}
		}
		s = s[j+1:]
	}
	b.WriteString(s)
	b.WriteString("\x1b[m")
	return b.String()
}

// gl draws one glyph in a foreground over a background (nil: the plane under it).
func gl(glyph string, fg, bg color.Color) string {
	out := ""
	if fg != nil {
		out += sgrFg(fg)
	}
	if bg != nil {
		out += sgrBg(bg)
	}
	return out + glyph + "\x1b[m"
}

// gls repeats a glyph n times in one escape run.
func gls(glyph string, n int, fg, bg color.Color) string {
	if n <= 0 {
		return ""
	}
	return gl(strings.Repeat(glyph, n), fg, bg)
}

// slabOpt shapes a slab: top draws the lit top face (a half row of gloss); band is how many of
// the first rows sit on the surface's lit band (a header strip); grad blends the face toward a
// second colour across the rows (the hero's gradient); flat keeps one face down every row (an
// answer, whose pieces join into one slab); padR is the right inner padding.
type slabOpt struct {
	top  bool
	band int
	grad color.Color
	flat bool
	padR int
}

// slabInner is the content width of a slab w cells wide (the spine or the lit left edge and
// the left padding, the right padding and the shaded right edge, and the shadow's columns
// taken off).
func slabInner(s surf, w int, o slabOpt) int {
	n := w - 2 - max(o.padR, 1) - 1 // the right edge's bevel cell
	if s.shadow != nil {
		n -= shadowW
	}
	return max(n, 1)
}

// slab raises rows on a surface, exactly w cells wide, with volume: the gloss of its lit top
// face (opt), each row on a face that steps from lit at the top to shaded at the bottom behind
// the spine (or a crisp lit left bevel) and the padding, the right bevel two tones deep, the
// shaded bottom edge, and the shadow — two cells right and half a row down, dark near the slab
// and fading away, its corners quarter blocks so it falls soft.
func (t Theme) slab(s surf, rows []string, w int, o slabOpt) string {
	sh := s.shadow != nil
	bw := w
	if sh {
		bw = w - shadowW
	}
	inner := slabInner(s, w, o)
	padR := max(o.padR, 1)
	n := len(rows)
	faceAt := func(i int) color.Color {
		if i < o.band {
			return s.band
		}
		if o.grad != nil && n > 1 {
			return toRGB(s.face).mix(toRGB(o.grad), float64(i)/float64(n-1)).col()
		}
		if o.flat || s.top == nil {
			return s.face
		}
		// the volume: the rows below the band step from the lit top to the shaded bottom
		k, m := i-o.band, n-o.band
		if m <= 1 {
			return s.face
		}
		return toRGB(s.top).mix(toRGB(s.bot), float64(k)/float64(m-1)).col()
	}
	gloss := s.gloss
	if gloss == nil {
		gloss = s.lit
	}
	edge := s.edge
	var out []string
	if o.top {
		top := ""
		if s.spine != nil {
			top = gls("▄", bw, gloss, nil)
		} else {
			top = gl("▗", gloss, nil) + gls("▄", bw-2, gloss, nil) + gl("▖", gloss, nil)
		}
		if sh {
			top += "  "
		}
		out = append(out, top)
	}
	for i, r := range rows {
		face := faceAt(i)
		// the bevel's side edges: lit on the left (a spine takes its place), two tones of shade
		// on the right — the inner half-tone under the outer edge
		left := gl("▏", s.lit, face)
		if s.spine != nil {
			left = gl("▌", s.spine, face)
		}
		eb := face
		if edge != nil {
			eb = toRGB(face).mix(toRGB(s.shade), 0.4).col()
		}
		agent := " " + fit(r, inner) + strings.Repeat(" ", padR)
		line := left + paintBg(agent, face) + gl("▕", s.shade, eb)
		if sh {
			if i == 0 {
				line += gl("▄", s.shadow, nil) + gl("▖", s.shadow2, nil)
			} else {
				line += gl(" ", nil, s.shadow) + gl("▌", s.shadow2, nil)
			}
		}
		out = append(out, line)
	}
	bot := ""
	if sh {
		bot = gl("▝", s.shade, nil) + gl("▀", s.shade, s.shadow2) + gls("▀", bw-2, s.shade, s.shadow) + gl(" ", nil, s.shadow) + gl("▘", s.shadow2, nil)
	} else {
		bot = gl("▝", s.shade, nil) + gls("▀", bw-2, s.shade, nil) + gl("▘", s.shade, nil)
	}
	out = append(out, bot)
	return strings.Join(out, "\n")
}

// well sinks rows into the plane: the top edge in shadow, the left edge dark, the right and
// bottom edges lit — the inverse of a raised slab; its face steps from shaded under the top lip
// to a breath lighter at the bottom. glow (nil: none) lights its rim in a hue — the ask's well
// while the human types. It is exactly w cells wide and rows+2 tall.
func (t Theme) well(rows []string, w int, glow color.Color) []string {
	d := t.dep
	face := d.well
	var dark, light rgb
	if t.Dark {
		dark = face.mix(d.sink, 0.6)
		light = d.plane.mix(d.lift, 0.1)
	} else {
		dark = face.mix(d.sink, 0.3)
		light = d.lift
	}
	if glow != nil {
		h := toRGB(glow)
		dark = dark.mix(h, 0.3)
		light = light.mix(h, 0.38)
	}
	out := []string{gl("▗", dark.col(), nil) + gls("▄", w-2, dark.col(), nil) + gl("▖", dark.col(), nil)}
	for i, r := range rows {
		f := t.wellFace(i, len(rows))
		out = append(out, gl("▐", dark.col(), f)+paintBg(" "+fit(r, w-4)+" ", f)+gl("▌", light.col(), f))
	}
	return append(out, gl("▝", light.col(), nil)+gls("▀", w-2, light.col(), nil)+gl("▘", light.col(), nil))
}

// wellFace is the well's face on row i of n: the first row in the lip's shade, the rest
// climbing back to the well's own tone.
func (t Theme) wellFace(i, n int) color.Color {
	face := t.dep.well
	deep := face.mix(t.dep.sink, 0.35)
	if !t.Dark {
		deep = face.mix(t.dep.sink, 0.08)
	}
	if n <= 1 {
		return face.col()
	}
	return deep.mix(face, min(float64(i)/float64(max(n-2, 1)), 1)).col()
}

// pill is a verdict as a small rounded pill: the hue's tint as its face, half-block caps
// rounding its ends.
func (t Theme) pillChip(hue color.Color, text string) string {
	if !t.Depth {
		return t.chip(hue, text)
	}
	face := t.chipFace(hue)
	fg := toRGB(hue)
	return gl("▐", face.col(), nil) + lipgloss.NewStyle().Foreground(fg.col()).Background(face.col()).Bold(true).Render(text) + gl("▌", face.col(), nil)
}

// chipFace is a chip's face in a hue: the card's tone tinted toward it.
func (t Theme) chipFace(hue color.Color) rgb {
	if t.Dark {
		return t.dep.card.mix(toRGB(hue), 0.16)
	}
	return t.dep.card.mix(toRGB(hue), 0.12)
}

// chipAround sets already styled text on a rounded chip in a hue: half-block caps, the face
// the hue's tint (the text keeps its own colours).
func (t Theme) chipAround(hue color.Color, styled string) string {
	if !t.Depth || hue == nil {
		return styled
	}
	f := t.chipFace(hue).col()
	return gl("▐", f, nil) + paintBg(styled, f) + gl("▌", f, nil)
}

// decor are the block elements the depth draws with: a selection copies text, never a bevel,
// a spine or a shadow.
const decor = "▀▄▌▐▎▍▏▕▔▁▂▗▖▝▘"

// stripDecor turns the depth's glyphs into spaces.
func stripDecor(s string) string {
	if !strings.ContainsAny(s, decor) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(decor, r) {
			return ' '
		}
		return r
	}, s)
}

// isDecorRow is true for a row drawn of depth glyphs and spaces alone (a top face, a bottom
// edge): it carries no text.
func isDecorRow(s string) bool {
	p := ansi.Strip(s)
	if strings.TrimSpace(p) == "" {
		return false
	}
	return strings.TrimSpace(stripDecor(p)) == ""
}

// the surfaces, by role — the renderers and the contrast policy read the same constructors

// surfUser is the human's ask: a card in the human's accent, its spine the accent.
func (t Theme) surfUser() surf {
	return t.card(t.magenta.GetForeground(), 0.08, t.magenta.GetForeground())
}

// surfAnswer is the model's answer: a card a breath toward the primary, its spine the primary.
// Its face stays flat (slabOpt.flat): a turn's pieces join into one slab.
func (t Theme) surfAnswer() surf {
	return t.card(t.accent.GetForeground(), 0.045, t.accent.GetForeground())
}

// surfTool is a tool chip in its class's hue: a light tint, the class's spine.
func (t Theme) surfTool(class string) surf {
	h := t.class(class).GetForeground()
	return t.card(h, 0.05, h)
}

// surfSubagent is a Subagent's card: unlike a tool's chip, it holds its rank's lane as a real slab
// tint, its band (the header) deeper in the lane, the lane's spine.
func (t Theme) surfSubagent(rank string, lane color.Color) surf {
	h := lane
	if h == nil {
		h = t.rank(rank).GetForeground()
	}
	s := t.card(h, 0.09, h)
	s.band = toRGB(s.face).mix(toRGB(h), 0.05).col()
	return s
}

// surfPlan is the plan's own family: a slab tinted in the plan's violet with its spine, in the
// chat (the plan tool's card) and in the sidebar (the plan tile).
func (t Theme) surfPlan() surf {
	h := t.plan.GetForeground()
	s := t.card(h, 0.09, h)
	s.band = toRGB(s.face).mix(toRGB(h), 0.05).col()
	return s
}

// surfTile is a sidebar tile: raised off the sidebar's plane, tinted toward its section's hue.
func (t Theme) surfTile(hue color.Color) surf {
	base := t.dep.side.mix(t.dep.card, 0.8)
	if !t.Dark {
		base = t.dep.card
	}
	return t.raise(base, hue, 0.06, t.dep.side.mix(t.dep.shadow, 0.85), t.dep.side, nil)
}

// surfHero is the welcome: the hero card, its face blended primary → accent across its rows.
func (t Theme) surfHero() (surf, color.Color) {
	s := t.card(t.accent.GetForeground(), 0.05, nil)
	return s, toRGB(t.dep.card.col()).mix(toRGB(t.magenta.GetForeground()), 0.06).col()
}

// surfToast is a notification: a float in its verdict's hue, glowing — it is fresh.
func (t Theme) surfToast(hue color.Color) surf {
	return t.glowing(t.floatSurf(hue, 0.05, hue), hue, t.dep.plane)
}

// surfFloat is the ask card and the popups: a float a breath toward the primary, the deepest
// shadow lit at its rim by a faint halo of the primary.
func (t Theme) surfFloat() surf {
	s := t.floatSurf(t.accent.GetForeground(), 0.035, nil)
	if !t.Depth {
		return s
	}
	h := toRGB(t.accent.GetForeground())
	s.gloss = toRGB(s.gloss).mix(h, 0.25).col()
	s.shadow = toRGB(s.shadow).mix(h, 0.12).col()
	s.shadow2 = toRGB(s.shadow2).mix(h, 0.18).col()
	return s
}

// codeBg is a code block's ground: the panel's step flat, sunk into the answer's slab (an inset
// a step below its face) under the depth.
func (t Theme) codeBg() color.Color {
	if !t.Depth {
		return t.paneBg()
	}
	f := toRGB(t.surfAnswer().face)
	if t.Dark {
		return f.mix(t.dep.well, 0.55).col()
	}
	return f.mix(t.dep.well, 0.7).col()
}

// insetCode renders an answer's markdown with each fenced code block sunk into its own inset
// well (codeBg) the width of the text column: the prose and the fences are rendered apart and
// joined by one blank row.
func (t Theme) insetCode(md string, width int) string {
	var segs []string
	var code []bool
	var cur []string
	fence := false
	flush := func(c bool) {
		if txt := strings.Join(cur, "\n"); strings.TrimSpace(txt) != "" {
			segs, code = append(segs, txt), append(code, c)
		}
		cur = nil
	}
	for _, l := range strings.Split(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			if !fence {
				flush(false)
				cur, fence = append(cur, l), true
				continue
			}
			cur = append(cur, l)
			flush(true)
			fence = false
			continue
		}
		cur = append(cur, l)
	}
	flush(fence)
	if len(segs) == 1 && !code[0] {
		return t.Markdown(md, width)
	}
	var out []string
	for i, sg := range segs {
		if i > 0 {
			out = append(out, "")
		}
		if !code[i] {
			if r := t.Markdown(sg, width); r != "" {
				out = append(out, r)
			}
			continue
		}
		bg := t.codeBg()
		ls := strings.Split(t.Markdown(sg, width-2), "\n")
		for len(ls) > 0 && strings.TrimSpace(ansi.Strip(ls[0])) == "" {
			ls = ls[1:]
		}
		for len(ls) > 0 && strings.TrimSpace(ansi.Strip(ls[len(ls)-1])) == "" {
			ls = ls[:len(ls)-1]
		}
		// the well's half-row lips: its face begins and ends mid-cell
		out = append(out, gls("▄", width, bg, nil))
		for _, l := range ls {
			out = append(out, paintBg(" "+fit(l, width-2)+" ", bg))
		}
		out = append(out, gls("▀", width, bg, nil))
	}
	return strings.Join(out, "\n")
}

// answerSeam is the row two joined pieces of an answer meet on: an empty row of the slab.
func (t Theme) answerSeam(w int) string {
	return strings.Split(t.slab(t.surfAnswer(), []string{""}, w, slabOpt{top: true, flat: true}), "\n")[1]
}

// headerTints are the header bar's two ends: a card's tone a breath toward the primary on the
// left, toward the magenta on the right.
func (t Theme) headerTints() (from, to rgb) {
	return t.dep.card.mix(toRGB(t.accent.GetForeground()), 0.07), t.dep.card.mix(toRGB(t.magenta.GetForeground()), 0.05)
}

// veil is the ground the frame sinks to under a floating ask card.
func (t Theme) veil() color.Color { return t.dep.well.mix(t.dep.ground, 0.25).col() }

// statusBg is the status bar's plane: the conversation's step, a breath toward the magenta.
func (t Theme) statusBg() color.Color {
	return t.dep.plane.mix(toRGB(t.magenta.GetForeground()), 0.02).col()
}
