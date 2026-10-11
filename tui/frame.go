package tui

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// The frame (pane mode): one box-drawn grid that owns the window under the headroom row —
//
//	╭──────────────────────────────────────────────────────────╮
//	│ ✦ agent-one v0.2.0   ~/workspace   model           ● idle       │  header
//	├─ ᴄᴏɴᴠᴇʀꜱᴀᴛɪᴏɴ ─────────────────────┬─ ꜱᴇꜱꜱɪᴏɴ ─────────┤
//	│ the transcript (a viewport)          │ vitals            │
//	│                                      ├─ ▾ ᴄᴏᴜʀᴛꜱ · 2 ────┤  sidebar sections
//	├─ ᴀꜱᴋ ────────────────────────────────┤ …                 │
//	│ › the input                          │                   │
//	├──────────────────────────────────────┴───────────────────┤
//	│ ctx ▰▰▱▱ 12% · 3.4k tok · $0.01            ? keys · …    │  status bar
//	╰──────────────────────────────────────────────────────────╯
//
// Boxes share their edges (one line where two meet, a junction where three do), every row is
// exactly the window's width, and the geometry is computed once per frame into a layout that
// the mouse reads back (selection, rail clicks, the wheel) — nothing scans glyphs.

// rect is a region of the window in cells, in window coordinates.
type rect struct{ X, Y, W, H int }

func (r rect) has(x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

// layout is the frame's geometry for one window.
type layout struct {
	pane bool // the frame owns the window; false: the session flows inline, one column
	W, H int
	mc   int // the main column's width between its borders
	rc   int // the sidebar column's width between the divider and the right border; 0: hidden
	divX int // the divider's column (the sidebar's left edge)

	topY, headerY, sepY   int
	inputSepY, footerSepY int
	footerY, bottomY      int
	chat, input, rail     rect // content rectangles (padding excluded)
	inputN                int  // the input's rows
	well                  int  // the input well's edge rows (2 under the depth, else 0)
}

// paneMode: the window is big enough for the frame (railSize's threshold); below it the
// session flows inline into the terminal's scrollback.
func (m *Model) paneMode() bool { return railSize(m.width, m.height) > 0 }

// layout computes the frame's geometry for the window, the sidebar's visibility and the
// input's current height.
func (m *Model) layout() layout {
	l := layout{W: m.width, H: m.height}
	if !m.paneMode() {
		return l
	}
	l.pane = true
	if rs := m.railVisible(); rs > 0 {
		l.rc = rs - 2
	}
	if l.rc > 0 {
		l.mc = l.W - 3 - l.rc
		l.divX = 1 + l.mc
	} else {
		l.mc = l.W - 2
	}
	l.topY = paneTop
	l.headerY = l.topY + 1
	l.sepY = l.topY + 2
	l.bottomY = l.H - 1
	l.footerY = l.H - 2
	l.footerSepY = l.H - 3
	l.inputN = max(m.input.Height(), 1)
	chatY := l.sepY + 1
	// the depth sinks the input into a well: its shadowed top edge and its lit bottom edge
	// take a row each, its side edges a cell each
	m.syncDepth()
	if m.theme.Depth {
		l.well = 2
	}
	// the conversation keeps at least four rows: a tall draft scrolls inside its box first
	for l.inputN > 1 && l.footerSepY-l.inputN-l.well-1-chatY < 4 {
		l.inputN--
	}
	l.inputSepY = l.footerSepY - l.inputN - l.well - 1
	l.chat = rect{X: 2, Y: chatY, W: l.mc - 2, H: max(l.inputSepY-chatY, 1)}
	l.input = rect{X: 2, Y: l.inputSepY + 1, W: l.mc - 2, H: l.inputN}
	if l.well > 0 {
		l.input = rect{X: 3, Y: l.inputSepY + 2, W: l.mc - 4, H: l.inputN}
	}
	if l.rc > 0 {
		l.rail = rect{X: l.divX + 2, Y: chatY, W: l.rc - 2, H: l.footerSepY - chatY}
	}
	return l
}

// geom is the frame's geometry as the human last saw it: the last frame's layout while the
// window still has its size, a fresh one otherwise (an event before the first frame).
func (m *Model) geom() layout {
	if m.lay.pane && m.lay.W == m.width && m.lay.H == m.height {
		return m.lay
	}
	return m.layout()
}

// chatW is the width the conversation's blocks render at: the chat pane's content width under
// the frame, the window's otherwise.
func (m *Model) chatW() int {
	if l := m.layout(); l.pane {
		return l.chat.W
	}
	return m.width
}

// fit truncates a styled line to w cells and pads it with plain spaces to exactly w.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	return s + padRight(s, w)
}

// rule is a horizontal border segment exactly w cells wide, a title set into its left end and
// an aside into its right (`─ title ─────── aside ─`), the dashes in the border's quiet colour.
// Titles arrive styled; a title that does not fit is cut, then the aside yields.
func (t Theme) rule(w int, title, aside string) string {
	if w <= 0 {
		return ""
	}
	b := t.border
	dash := func(n int) string {
		if n <= 0 {
			return ""
		}
		return b.Render(strings.Repeat("─", n))
	}
	if title == "" && aside == "" {
		return dash(w)
	}
	head := ""
	if title != "" {
		room := w - 4
		if room < 3 {
			return dash(w)
		}
		title = ansi.Truncate(title, room, "…")
		head = b.Render("─ ") + title + " "
	}
	tail := ""
	if aside != "" {
		tail = " " + aside + b.Render(" ─")
		if ansi.StringWidth(head)+ansi.StringWidth(tail)+1 > w {
			tail = ""
		}
	}
	return head + dash(w-ansi.StringWidth(head)-ansi.StringWidth(tail)) + tail
}

// title is a name as the frame sets it into a border: small caps, bold, on a gradient of the
// palette — the primary into the plan's violet unless the region names its own hues.
func (t Theme) title(s string) string {
	return t.titleIn(s, t.accent.GetForeground(), t.plan.GetForeground())
}

// titleIn is a title on its region's own gradient (a colourless terminal reads the label tone).
func (t Theme) titleIn(s string, stops ...color.Color) string {
	if !t.Color || len(stops) == 0 {
		return t.label.Bold(true).Render(smallCap(s))
	}
	if len(stops) == 1 {
		return lipgloss.NewStyle().Foreground(stops[0]).Bold(true).Render(smallCap(s))
	}
	return Gradient(smallCap(s), true, stops...)
}

// regionTitle is each region's title in its own hues: the conversation in the primary's cyan
// into the zone worker's blue, the session in the orchestrator's amber into the human's rose, the ask in
// the human's rose into the plan's violet.
func (t Theme) regionTitle(region string) string {
	switch region {
	case "conversation":
		return t.titleIn(region, t.accent.GetForeground(), t.lane("zone"))
	case "session":
		return t.titleIn(region, t.lane("orchestrator"), t.magenta.GetForeground())
	case "ask":
		return t.titleIn(region, t.magenta.GetForeground(), t.plan.GetForeground())
	}
	return t.title(region)
}

// railRow is one row of the sidebar column: a content row (rc-2 cells, the frame pads it), or a
// section rule (rc cells) the frame joins to the divider and the right border. key names the
// section a rule opens (its click folds the section); subagent names the agent a roster row shows.
type railRow struct {
	text     string
	sep      bool
	key      string
	subagent string
}

// frame draws the pane-mode window under the headroom: header, conversation │ sidebar, the
// input in the main column, the status bar — exactly l.H-l.topY rows of exactly l.W cells.
// Under the depth every region is painted on its own plane: the borders on the ground, the
// header raised on a gradient, the conversation and the sidebar each a plane, the input sunk in
// its well, the status bar a plane.
func (m *Model) frame(l layout, header string, chat, input []string, rail []railRow, convAside, askTitle, askAside, status string) []string {
	t := m.theme
	b := t.border
	g := func(s string) string { return b.Render(s) }
	// the planes: identity when the depth is off
	plane, sideP, headP := func(s string) string { return s }, func(s string) string { return s }, func(s string) string { return s }
	if t.Depth {
		d := t.dep
		ground := d.ground.col()
		g = func(s string) string { return gl(s, b.GetForeground(), ground) }
		pc, sc := d.plane.col(), d.side.col()
		plane = func(s string) string { return paintBg(s, pc) }
		sideP = func(s string) string { return paintBg(s, sc) }
		// the header: a raised bar, its face drifting from the primary's tint to the magenta's
		from, to := t.headerTints()
		hw := l.W - 2
		ramp := make([]color.Color, hw)
		for x := range ramp {
			ramp[x] = from.mix(to, float64(x)/float64(max(hw-1, 1))).col()
		}
		headP = func(s string) string { return paint(s, func(x int) color.Color { return ramp[min(x, hw-1)] }) }
	}
	rule := func(w int, title, aside string) string {
		r := t.rule(w, title, aside)
		if t.Depth {
			return paintBg(r, t.dep.ground.col())
		}
		return r
	}
	rows := make([]string, 0, l.H-l.topY)
	rows = append(rows, g("╭")+rule(l.W-2, "", "")+g("╮"))
	rows = append(rows, g("│")+headP(" "+fit(header, l.W-4)+" ")+g("│"))
	if l.rc > 0 {
		rows = append(rows, g("├")+rule(l.mc, t.regionTitle("conversation"), convAside)+g("┬")+rule(l.rc, t.regionTitle("session"), "")+g("┤"))
	} else {
		rows = append(rows, g("├")+rule(l.mc, t.regionTitle("conversation"), convAside)+g("┤"))
	}
	// the scrollbars: the chat's in its right padding column, the sidebar's on its border —
	// only while the content overflows
	var chatBar, railBar []rune
	if m.vpOK {
		chatBar = scrollThumb(l.chat.H, len(m.vpLines), m.vp.YOffset())
	}
	if l.rc > 0 {
		railBar = scrollThumb(l.rail.H, m.railTotal, m.railScroll)
	}
	planeBg := color.Color(nil)
	if t.Depth {
		planeBg = t.dep.plane.col()
	}
	// the sidebar's part of a row y (content or rule), the divider glyph it asks for, and the
	// right border
	side := func(y int, mainArm bool) string {
		if l.rc == 0 {
			if mainArm {
				return g("┤")
			}
			return g("│")
		}
		r := y - l.rail.Y
		var row railRow
		if r >= 0 && r < len(rail) {
			row = rail[r]
		}
		right := g("│")
		if r >= 0 && r < len(railBar) && railBar[r] != '▕' {
			// the sidebar's scrollbar rides its right border: the border is the track, the
			// thumb a heavy line in the primary, half-row ends
			glyph := map[rune]string{'▐': "┃", '▗': "╻", '▝': "╹"}[railBar[r]]
			if t.Depth {
				right = gl(glyph, t.accent.GetForeground(), t.dep.ground.col())
			} else {
				right = t.accent.Render(glyph)
			}
		}
		if row.sep {
			div := "├"
			if mainArm {
				div = "┼"
			}
			if right == g("│") {
				right = g("┤")
			}
			return g(div) + ruleText(t, row.text, l.rc) + right
		}
		div := "│"
		if mainArm {
			div = "┤"
		}
		if t.Depth {
			return g(div) + sideP(" "+fit(row.text, l.rc-1)) + right
		}
		return g(div) + sideP(" "+fit(row.text, l.rc-2)+" ") + right
	}
	for i := 0; i < l.chat.H; i++ {
		c := ""
		if i < len(chat) {
			c = chat[i]
		}
		pad := plane(" ")
		if i < len(chatBar) {
			pad = t.scrollGlyph(chatBar[i], planeBg)
		}
		rows = append(rows, g("│")+plane(" "+fit(c, l.chat.W))+pad+side(l.chat.Y+i, false))
	}
	rows = append(rows, g("├")+rule(l.mc, askTitle, askAside)+side(l.inputSepY, true))
	if l.well > 0 {
		// the well's rim glows in the primary while the human types into it
		var wellGlow color.Color
		if m.choice == nil && m.input.Value() != "" {
			wellGlow = t.accent.GetForeground()
		}
		rs := make([]string, l.inputN)
		for i := range rs {
			if i < len(input) {
				rs[i] = input[i]
			}
		}
		for i, w := range t.well(rs, l.mc, wellGlow) {
			rows = append(rows, g("│")+plane(w)+side(l.inputSepY+1+i, false))
		}
	} else {
		for i := 0; i < l.inputN; i++ {
			c := ""
			if i < len(input) {
				c = input[i]
			}
			rows = append(rows, g("│")+" "+fit(c, l.input.W)+" "+side(l.input.Y+i, false))
		}
	}
	if l.rc > 0 {
		rows = append(rows, g("├")+rule(l.mc, "", "")+g("┴")+rule(l.rc, "", "")+g("┤"))
	} else {
		rows = append(rows, g("├")+rule(l.mc, "", "")+g("┤"))
	}
	statusP := plane
	if t.Depth {
		sb := t.statusBg()
		statusP = func(s string) string { return paintBg(s, sb) }
	}
	rows = append(rows, g("│")+statusP(" "+fit(status, l.W-4)+" ")+g("│"))
	rows = append(rows, g("╰")+rule(l.W-2, "", "")+g("╯"))
	return rows
}

// ruleText is a sidebar section rule as the frame draws it: on the ground under the depth.
func ruleText(t Theme, text string, w int) string {
	r := fit(text, w)
	if t.Depth {
		return paintBg(r, t.dep.ground.col())
	}
	return r
}

// header is the frame's top bar: the brand on its gradient, the version, the workspace and the
// model on the left; the session's state chip (and the session's short id where it fits) on
// the right. Items yield right to left as the window narrows.
func (m *Model) header(w int) string {
	t := m.theme
	brand := t.Brand(m.words.Dist)
	if m.wel.Version != "" {
		brand += " " + t.dim.Render(m.wel.Version)
	}
	workspace := m.footer.Workspace
	if workspace == "" {
		workspace = m.wel.Workspace
	}
	state := m.stateChip()
	sess := ""
	if s := m.wel.Session; s != "" {
		if i := strings.LastIndex(s, "-"); i >= 0 && len(s)-i > 3 {
			s = s[i+1:]
		}
		sess = t.dim.Render("session ") + t.label.Render(s)
	}
	gap := "   "
	try := func(parts ...string) (string, bool) {
		var ps []string
		for _, p := range parts {
			if p != "" {
				ps = append(ps, p)
			}
		}
		left := strings.Join(ps, gap)
		right := state
		if sess != "" && ansi.StringWidth(left)+ansi.StringWidth(sess)+ansi.StringWidth(state)+8 <= w {
			right = sess + gap + state
		}
		pad := w - ansi.StringWidth(left) - ansi.StringWidth(right)
		if pad < 2 {
			return "", false
		}
		return left + strings.Repeat(" ", pad) + right, true
	}
	model := t.dim.Render(m.footer.Model)
	for _, cand := range [][]string{
		{brand, t.text.Render(tailPath(workspace, max(w/3, 24))), model},
		{brand, t.text.Render(shortPath(workspace)), model},
		{brand, t.text.Render(filepath.Base(workspace)), model},
		{brand, t.text.Render(filepath.Base(workspace))},
		{brand},
	} {
		if s, ok := try(cand...); ok {
			return s
		}
	}
	return ansi.Truncate(brand+gap+state, w, "…")
}

// shortPath keeps a path's last two components ("…/kaginari/agent-one").
func shortPath(p string) string {
	segs := strings.Split(strings.Trim(p, "/"), "/")
	if len(segs) <= 2 {
		return p
	}
	return "…/" + strings.Join(segs[len(segs)-2:], "/")
}

// stateChip is the session's state, one glyph and a word: idle dim, a running turn in the
// primary with its clock, a pending ask in the human's magenta.
func (m *Model) stateChip() string {
	t := m.theme
	switch {
	case m.choice != nil:
		return t.magenta.Render("◆ waiting for you")
	case m.busy:
		verb := strings.ToLower(strings.TrimSuffix(m.verb, "…"))
		if verb == "" {
			verb = "working"
		}
		if len([]rune(verb)) > 24 {
			verb = string([]rune(verb)[:23]) + "…"
		}
		return t.accent.Render("● "+verb) + t.dim.Render(" "+time.Since(m.turnStart).Round(time.Second).String())
	}
	return t.dim.Render("○ idle")
}

// StatusBar is the frame's bottom bar: the context meter, the session's spend, the live agents
// and any mode on the left (labels dim, values in the text tone); a transient message or the
// key hints on the right.
func (t Theme) StatusBar(f FooterView, width int) string {
	var parts []string
	ctx := t.dim.Render("ctx ") + t.dim.Render("—")
	if f.CtxKnown {
		meter := t.Meter(f.CtxPct, 10)
		if t.Depth {
			meter = t.gauge(float64(f.CtxPct)/100, 10, nil, t.ctxHue)
		}
		ctx = t.dim.Render("ctx ") + meter + " " + t.text.Render(fmt.Sprintf("%d%%", f.CtxPct))
		if f.CtxTokens > 0 && f.CtxLimit > 0 && width >= 100 {
			ctx += t.dim.Render(fmt.Sprintf(" %s/%s", humanTokens(f.CtxTokens), humanTokens(f.CtxLimit)))
		}
		switch {
		case f.Zone == "STRESS" || strings.Contains(f.Ctx, "stress"):
			ctx += " " + t.bad.Render("stress")
		case f.Zone == "NEAR":
			ctx += " " + t.warn.Render("near")
		}
	}
	parts = append(parts, ctx)
	if f.Tokens > 0 {
		parts = append(parts, t.text.Render(humanTokens(f.Tokens))+t.dim.Render(" tok"))
	}
	if f.Cost != "" {
		if strings.HasPrefix(f.Cost, "$") {
			parts = append(parts, t.text.Render(f.Cost))
		} else {
			parts = append(parts, t.dim.Render(f.Cost))
		}
	}
	if f.Live > 0 {
		word := f.Subagents
		if word == "" {
			word = "subagents"
		}
		if f.Live == 1 {
			word = strings.TrimSuffix(word, "s")
		}
		parts = append(parts, t.accent.Render(fmt.Sprintf("%d", f.Live))+t.dim.Render(" live "+word))
	}
	if f.Queued > 0 {
		parts = append(parts, t.text.Render(fmt.Sprintf("%d", f.Queued))+t.dim.Render(" queued"))
	}
	if f.Badge != "" {
		parts = append(parts, t.magenta.Render(f.Badge))
	}
	sep := t.dim.Render("  ·  ")
	right := t.keys(f.Hint)
	if f.Message != "" {
		right = t.label.Render(f.Message)
	}
	for len(parts) > 0 {
		left := strings.Join(parts, sep)
		if right == "" || ansi.StringWidth(left)+ansi.StringWidth(right)+3 <= width {
			if right == "" {
				return fit(left, width)
			}
			return left + strings.Repeat(" ", width-ansi.StringWidth(left)-ansi.StringWidth(right)) + right
		}
		if len(parts) == 1 {
			// the hints yield before the meter does
			right = ""
			continue
		}
		parts = parts[:len(parts)-1]
	}
	return fit(right, width)
}

// keys renders "a b · c d" hint pairs with the key in the label tone and its action dim.
func (t Theme) keys(hint string) string {
	if hint == "" {
		return ""
	}
	var out []string
	for _, h := range strings.Split(hint, " · ") {
		k, does, ok := strings.Cut(h, " ")
		if !ok {
			out = append(out, t.dim.Render(h))
			continue
		}
		out = append(out, t.label.Render(k)+" "+t.dim.Render(does))
	}
	return strings.Join(out, t.dim.Render(" · "))
}

// panel is a free-standing card: a rounded box in the border tone, a title set into its top
// edge, its rows padded one cell each side, exactly w cells wide.
func (t Theme) panel(title string, rows []string, w int) string {
	return t.panelIn(t.border, title, rows, w)
}

// panelIn is panel with its edge in another tone: a floating focus (the palette) holds the
// primary so it stands off the conversation it covers.
func (t Theme) panelIn(edge lipgloss.Style, title string, rows []string, w int) string {
	if t.Depth {
		// a float: the title on the lit band (its edge's hue as a spine), the deepest shadow
		s := t.surfFloat()
		if edge.GetForeground() == t.accent.GetForeground() {
			s.spine = t.accent.GetForeground()
		}
		var out []string
		o := slabOpt{top: true}
		if title != "" {
			out, o.band = append(out, title), 1
		}
		for _, r := range rows {
			out = append(out, strings.Split(r, "\n")...)
		}
		return t.slab(s, out, w, o)
	}
	b := edge
	t.border = edge
	out := []string{b.Render("╭") + t.rule(w-2, title, "") + b.Render("╮")}
	for _, r := range rows {
		for _, l := range strings.Split(r, "\n") {
			out = append(out, b.Render("│")+" "+fit(l, w-4)+" "+b.Render("│"))
		}
	}
	out = append(out, b.Render("╰")+t.rule(w-2, "", "")+b.Render("╯"))
	return strings.Join(out, "\n")
}
