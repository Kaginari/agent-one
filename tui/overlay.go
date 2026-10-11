package tui

import (
	"sort"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/x/ansi"
)

// Overlays drawn over the live area as Lip Gloss layers: the command palette (ctrl+k) and the
// toasts that slide in when a Subagent lands or the gate rules.

// --- the palette ---

type paletteState struct {
	query  string
	cursor int
	all    []MenuItem
}

type paletteHit struct {
	item  MenuItem
	score int
	idx   []int // matched rune positions in the name
}

// paletteActions are the UI's own entries beside the host's commands.
var paletteActions = []MenuItem{
	{Name: "board", Description: "the board, full screen"},
	{Name: "theme", Description: "flip dark and light — /theme <name> [dark|light|system] holds another palette"},
	{Name: "help", Description: "commands and shortcuts"},
	{Name: "copy", Description: "copy the last answer's markdown"},
	{Name: "mouse", Description: "the mouse to the terminal (its own selection) or back to the panes"},
	{Name: "sidebar", Description: "hide or show the sidebar (alt+s)"},
	{Name: "roles", Description: "every role and the model it runs on"},
	{Name: "settings", Description: "theme, mode, mouse and sidebar in one form"},
	{Name: "clear", Description: "clear the screen"},
	{Name: "quit", Description: "end the session"},
}

// noArgs are the commands the palette runs at once; the others land in the input to be finished.
var noArgs = map[string]bool{"board": true, "clear": true, "help": true, "quit": true, "agents": true, "usage": true,
	"status": true, "sessions": true, "compact": true, "theme": true, "copy": true, "mouse": true,
	"sidebar": true, "roles": true, "settings": true}

func (m *Model) openPalette() {
	seen := map[string]bool{}
	var all []MenuItem
	for _, it := range append(append([]MenuItem(nil), paletteActions...), m.host.Commands()...) {
		if !seen[it.Name] {
			seen[it.Name] = true
			all = append(all, it)
		}
	}
	m.palette = &paletteState{all: all}
	m.menu, m.comp = nil, nil
}

// fuzzy scores name against q: every rune of q in order; consecutive and word-start matches score
// higher. ok false when q is not a subsequence.
func fuzzy(name, q string) (score int, idx []int, ok bool) {
	if q == "" {
		return 0, nil, true
	}
	n, qs := []rune(strings.ToLower(name)), []rune(strings.ToLower(q))
	j, last := 0, -2
	for i, r := range n {
		if j < len(qs) && r == qs[j] {
			idx = append(idx, i)
			score += 10
			if i == last+1 {
				score += 15
			}
			if i == 0 || !unicode.IsLetter(n[i-1]) {
				score += 20
			}
			last = i
			j++
		}
	}
	if j < len(qs) {
		return 0, nil, false
	}
	return score - len(n), idx, true
}

func (p *paletteState) hits() []paletteHit {
	var out []paletteHit
	for _, it := range p.all {
		s, idx, ok := fuzzy(it.Name, p.query)
		if !ok {
			continue
		}
		out = append(out, paletteHit{item: it, score: s, idx: idx})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}

// paletteKey handles a key while the palette is open.
func (m *Model) paletteKey(k tea.KeyPressMsg) tea.Cmd {
	p := m.palette
	hs := p.hits()
	switch k.String() {
	case "esc", "ctrl+k", "ctrl+c":
		m.palette = nil
	case "up", "ctrl+p":
		p.cursor = step(p.cursor, max(len(hs), 1), true)
	case "down", "ctrl+n":
		p.cursor = step(p.cursor, max(len(hs), 1), false)
	case "backspace":
		if r := []rune(p.query); len(r) > 0 {
			p.query, p.cursor = string(r[:len(r)-1]), 0
		}
	case "enter":
		m.palette = nil
		if len(hs) == 0 {
			return nil
		}
		name := hs[min(p.cursor, len(hs)-1)].item.Name
		if name == "theme" {
			return m.themeSlash("")
		}
		if noArgs[name] {
			m.setInput("/" + name)
			return m.submit()
		}
		m.setInput("/" + name + " ")
	default:
		if k.Text != "" {
			p.query, p.cursor = p.query+k.Text, 0
		}
	}
	return nil
}

// paletteView is the palette's box.
func (m *Model) paletteView() string {
	t, p := m.theme, m.palette
	w := min(64, m.width-4)
	hs := p.hits()
	lines := []string{t.accent.Render("❯ ") + p.query + t.accent.Render("▏")}
	if len(hs) == 0 {
		lines = append(lines, t.dim.Render("  nothing matches"))
	}
	if p.cursor >= len(hs) {
		p.cursor = max(0, len(hs)-1)
	}
	start := max(0, p.cursor-7)
	namew := 0
	for i := start; i < len(hs) && i < start+8; i++ {
		namew = max(namew, len([]rune(hs[i].item.Name))+1)
	}
	namew = min(namew, 16)
	for i := start; i < len(hs) && i < start+8; i++ {
		h := hs[i]
		name := []rune(h.item.Name)
		var sb strings.Builder
		sb.WriteString("/")
		mi := 0
		for j, r := range name {
			if mi < len(h.idx) && h.idx[mi] == j {
				sb.WriteString(t.accent.Bold(true).Render(string(r)))
				mi++
			} else {
				sb.WriteString(string(r))
			}
		}
		nameStr := sb.String()
		nameStr += strings.Repeat(" ", max(namew-lipgloss.Width(nameStr), 0))
		desc := ansi.Truncate(h.item.Description, w-lipgloss.Width(nameStr)-8, "…")
		row := "  " + nameStr + "  " + t.dim.Render(desc)
		if i == p.cursor {
			row = t.accent.Render("› ") + nameStr + "  " + t.text.Render(desc)
		}
		lines = append(lines, row)
	}
	lines = append(lines, "", t.keys("↑↓ choose · enter runs · esc closes"))
	return t.panelIn(t.accent, t.title("palette"), lines, w)
}

// --- the toasts ---

const (
	toastLife = 4 * time.Second
	toastFPS  = 60
)

type toast struct {
	text   string
	style  lipgloss.Style
	born   time.Time
	x, vel float64 // cells still to slide in from the right
	spring harmonica.Spring
}

type evToastFrame struct{}

func toastTick() tea.Cmd {
	return tea.Tick(time.Second/toastFPS, func(time.Time) tea.Msg { return evToastFrame{} })
}

// Toast shows a one-line notice at the live area's top right; it slides in and leaves after a
// few seconds. The block it echoes is already in the scrollback: a toast is only a glance.
func (m *Model) Toast(text string, st lipgloss.Style) tea.Cmd {
	anim := len(m.toasts) == 0 || m.toasts[len(m.toasts)-1].x < 0.5
	m.toasts = append(m.toasts, toast{text: text, style: st, born: time.Now(), x: float64(lipgloss.Width(text) + 6),
		spring: harmonica.NewSpring(harmonica.FPS(toastFPS), 9, 0.7)})
	if len(m.toasts) > 3 {
		m.toasts = m.toasts[len(m.toasts)-3:]
	}
	if anim {
		return toastTick()
	}
	return nil
}

// stepToasts slides the toasts in and drops the expired ones; true while one still moves.
func (m *Model) stepToasts() bool {
	moving := false
	kept := m.toasts[:0]
	for _, tt := range m.toasts {
		if time.Since(tt.born) > toastLife {
			continue
		}
		tt.x, tt.vel = tt.spring.Update(tt.x, tt.vel, 0)
		if tt.x > 0.3 || tt.vel > 0.3 || tt.vel < -0.3 {
			moving = true
		} else {
			tt.x, tt.vel = 0, 0
		}
		kept = append(kept, tt)
	}
	m.toasts = kept
	return moving
}

// overlay composes the live area with the palette, the floating ask card, the verdict burst
// and the toasts as layers (lipgloss canvases do real cell compositing — the top layer's blank
// cells let the frame show through; there is no alpha anywhere). POLICY: the overlay never adds a
// row to the frame — toasts and palette draw over the base's own rows. The inline renderer
// tracks height exactly; a frame whose height waves at it leaves ghosts in the scrollback.
func (m *Model) overlay(base string) string {
	l := m.lay
	popup := l.pane && (m.menu != nil || m.comp != nil || m.shortcuts)
	form := l.pane && m.settings != nil
	if m.palette == nil && len(m.toasts) == 0 && !m.choiceFloat() && !m.burstOn && !popup && !form {
		return base
	}
	var layers []*lipgloss.Layer
	baseH := strings.Count(base, "\n") + 1
	// toasts slide in over the conversation's top-right rows (the window's, inline) — the base
	// keeps its height; the part still off-stage is clipped at the pane's edge
	right, y0 := m.width-1, 0
	if l.pane {
		right, y0 = l.chat.X+l.chat.W, l.chat.Y-l.topY
	}
	ty := y0
	for _, tt := range m.toasts {
		if m.theme.Depth && l.pane {
			// the toast floats highest: a slab in its verdict's hue, the deepest shadow, over
			// the conversation's top-right corner
			card := m.theme.toastCard(tt)
			cw := lipgloss.Width(card)
			x := right - cw + int(tt.x)
			if x >= right {
				continue
			}
			for i, row := range strings.Split(card, "\n") {
				layers = append(layers, lipgloss.NewLayer(ansi.Truncate(row, right-x, "")).X(max(x, 0)).Y(ty+i).Z(5))
			}
			ty += strings.Count(card, "\n") + 1
			continue
		}
		pill := tt.style.Padding(0, 1).Render(tt.text)
		x := right - lipgloss.Width(pill) + int(tt.x)
		if x >= right {
			ty++
			continue
		}
		layers = append(layers, lipgloss.NewLayer(ansi.Truncate(pill, right-x, "")).X(max(x, 0)).Y(ty).Z(2))
		ty++
	}
	if popup {
		// the slash menu, the @completions and the keys sheet rise from the input's top edge,
		// over the conversation's last rows — the frame never reflows for them
		w := min(l.mc, 84)
		room := l.inputSepY - l.chat.Y
		var box string
		switch {
		case m.shortcuts:
			box = m.theme.keysSheet(DefaultShortcuts(), w, room)
		case m.menu != nil:
			box = m.theme.panel(m.theme.title("commands"), []string{m.theme.Menu(m.menu.items, m.menu.cursor, w-4)}, w)
		default:
			box = m.theme.panel(m.theme.title("files"), []string{m.theme.Completions(m.comp.items, m.comp.cursor, w-4)}, w)
		}
		rows := strings.Split(box, "\n")
		if len(rows) > room {
			rows = rows[len(rows)-room:]
		}
		layers = append(layers, lipgloss.NewLayer(strings.Join(rows, "\n")).X(1).Y(l.inputSepY-l.topY-len(rows)).Z(3))
	}
	if m.choiceFloat() {
		// the darkened backdrop: the frame, stripped and held again in dim — an honest
		// darkening of real text, not a transparency trick
		dl := strings.Split(base, "\n")
		veil := m.theme.dim
		if m.theme.Depth {
			// under the depth the veil is the ground itself, a step deeper: every plane sinks
			// flat beneath the float
			veil = veil.Background(m.theme.veil())
		}
		for i, l := range dl {
			dl[i] = veil.Render(stripDecor(ansi.Strip(l)))
		}
		base = strings.Join(dl, "\n")
		box := m.theme.Choice(m.view, min(m.width*3/5, 100))
		if !m.theme.Depth {
			// elevation cue: the card's top edge gradients accent → border
			first, rest, _ := strings.Cut(box, "\n")
			box = Gradient(ansi.Strip(first), false, m.theme.accent.GetForeground(), m.theme.border.GetForeground()) + "\n" + rest
		}
		box = m.pinChoice(box) // growth-only pin: the card never reflows as the cursor moves
		bw, bh := lipgloss.Width(box), strings.Count(box, "\n")+1
		fh := m.frameH()
		x, y := centerRect(m.width, fh, bw, bh)
		y += dropOffset(time.Since(m.choiceAt))
		if y+bh > fh {
			y = max(0, fh-bh)
		}
		layers = append(layers, lipgloss.NewLayer(box).X(x).Y(y).Z(3))
	}
	if form {
		sv := m.settingsView()
		x, y := centerRect(l.mc+2, l.H-l.topY, lipgloss.Width(sv), strings.Count(sv, "\n")+1)
		layers = append(layers, lipgloss.NewLayer(sv).X(x).Y(y).Z(4))
	}
	if m.palette != nil {
		pv := m.paletteView()
		h := strings.Count(pv, "\n") + 1
		// the palette floats over the rows above the input — idle frames give it blank chat to
		// ride, busy frames it simply covers (it is a modal focus)
		y := max(0, baseH-4-h)
		x := max(0, (m.width-lipgloss.Width(pv))/2)
		if l.pane {
			y = max(0, l.inputSepY-l.topY-h)
			x = max(0, 1+(l.mc-lipgloss.Width(pv))/2)
		}
		layers = append(layers, lipgloss.NewLayer(pv).X(x).Y(y).Z(4))
	}
	if burst := m.burstRain(baseH); burst != nil {
		layers = append(layers, burst...)
	}
	all := append([]*lipgloss.Layer{lipgloss.NewLayer(base)}, layers...)
	return lipgloss.NewCompositor(all...).Render()
}

// pinChoice growth-pads the modal card to the biggest size it has reached since it opened —
// state economy so the card's centre never reflows when the cursor moves between options or
// into the free-text row (Operator 2026-10-03: the popup was floating choice to choice).
func (m *Model) pinChoice(box string) string {
	lines := strings.Split(box, "\n")
	w := 0
	for _, l := range lines {
		if lw := ansi.StringWidth(l); lw > w {
			w = lw
		}
	}
	if w > m.choiceW {
		m.choiceW = w
	}
	if len(lines) > m.choiceH {
		m.choiceH = len(lines)
	}
	for i, l := range lines {
		if pad := m.choiceW - ansi.StringWidth(l); pad > 0 {
			lines[i] = l + strings.Repeat(" ", pad)
		}
	}
	for len(lines) < m.choiceH {
		lines = append(lines, strings.Repeat(" ", m.choiceW))
	}
	return strings.Join(lines, "\n")
}

// burstRain is the verdict burst's glyph rain over the frame (rows lines tall): deterministic
// columns out of the arm's instant, staggered starts, two rows of fall a tick; it switches
// itself off once the fall is spent. The glyphs rain where the landed Subagent cards just left
// the scrollback.
func (m *Model) burstRain(rows int) []*lipgloss.Layer {
	if !m.burstOn {
		return nil
	}
	age := time.Since(m.burstAt)
	tick := int(age / (time.Second / pulseFPS))
	if age < 0 || tick >= burstTicks {
		m.burstOn = false
		return nil
	}
	st := m.theme.ok
	if !m.burstGood {
		st = m.theme.bad
	}
	glyph := st.Render(burstGlyph)
	seed := uint32(m.burstAt.UnixNano()>>20) * 2654435761 // knuth: stable scatter per arm
	n := min(18, m.width/7)
	var out []*lipgloss.Layer
	for i := 0; i < n; i++ {
		h := seed*uint32(i+3) ^ (seed >> 7)
		x := int(h % uint32(max(m.width, 1)))
		y := int(h>>8%3) + tick*2
		if y > rows-2 {
			continue
		}
		out = append(out, lipgloss.NewLayer(glyph).X(x).Y(y).Z(2))
	}
	return out
}

// pill is a toast's style: dark text on the verdict's colour.
func (t Theme) pill(kind string) lipgloss.Style {
	bg := t.accent.GetForeground()
	switch kind {
	case "ok":
		bg = t.ok.GetForeground()
	case "bad":
		bg = t.bad.GetForeground()
	}
	return lipgloss.NewStyle().Background(bg).Foreground(t.onAccent()).Bold(true)
}

// toastCard is a toast under the depth: a float slab, its verdict's hue as the spine and the
// tint, the text bold in the hue's own face.
func (t Theme) toastCard(tt toast) string {
	hue := tt.style.GetBackground()
	if hue == nil {
		hue = t.accent.GetForeground()
	}
	fg := toRGB(hue)
	if !t.Dark {
		fg = fg.mix(hexRGB("#000000"), 0.25)
	}
	text := lipgloss.NewStyle().Foreground(fg.col()).Bold(true).Render(tt.text)
	return t.slab(t.surfToast(hue), []string{" " + text}, lipgloss.Width(tt.text)+7+shadowW, slabOpt{top: true, padR: 2})
}
