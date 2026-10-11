package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
	"github.com/charmbracelet/x/ansi"
)

// The view model: every block the session draws, as a pure function of a view and a width.
// Nothing here knows the loop or Bubble Tea; the golden tests render each block at 80 and 120
// columns with the escapes stripped.

const (
	collapseLines = 8  // tool output shown before "… +N lines"
	collapseDiff  = 20 // diff lines shown before collapsing
	indent        = "  "
)

// Welcome is the card the conversation opens with.
type Welcome struct {
	Dist      string
	Version   string
	Workspace string
	Model     string
	Roles     []string // "analyst → gemini/x"
	Off       int      // features switched off
	Session   string
	Handoff   string // a handoff waiting for this session: its path and age
	Holes     []string
	// WorkspaceWord labels the root row ("workspace" | "workspace"); "" reads "workspace".
	WorkspaceWord string
	// Dash is the board's URL ("" when the board did not start).
	Dash string
}

// Welcome renders the opening card: the workspace's sprite beside its name, the tagline and the
// lane legend; the session's key facts as a label/value grid (the roles summarised — /roles
// has them whole); what the engine could not settle; one row of where to start.
func (t Theme) Welcome(w Welcome, width int) string {
	if w.WorkspaceWord == "" {
		w.WorkspaceWord = "workspace"
	}
	cw := min(width, 84)
	inner := cw - 4
	if t.Depth {
		h, _ := t.surfHero()
		inner = slabInner(h, cw, slabOpt{top: true})
	}
	art, tag := Mascot(w.Dist)
	artW := 0
	for _, l := range art {
		artW = max(artW, ansi.StringWidth(l))
	}
	head := []string{t.Brand(w.Dist) + " " + t.dim.Render(w.Version)}
	if tag != "" {
		head = append(head, t.dim.Italic(true).Render(tag))
	}
	head = append(head, "", t.laneLegend(w.Dist))
	var rows []string
	if len(art) > 0 && inner >= artW+3+28 {
		for i := 0; i < max(len(art), len(head)); i++ {
			a := strings.Repeat(" ", artW)
			if i < len(art) {
				a = art[i] + padRight(art[i], artW)
			}
			h := ""
			if i < len(head) {
				h = ansi.Truncate(head[i], inner-artW-3, "…")
			}
			rows = append(rows, a+"   "+h)
		}
	} else {
		for _, h := range head {
			rows = append(rows, ansi.Truncate(h, inner, "…"))
		}
	}
	rows = append(rows, "")
	type kv struct{ k, v string }
	vw := inner - 9 // the value column: the widest label and its gap ride left of it
	facts := []kv{{w.WorkspaceWord, t.text.Render(tailPath(w.Workspace, vw))}, {"model", t.text.Render(w.Model)}}
	if s := roleSummary(w.Roles); s != "" {
		facts = append(facts, kv{"roles", s})
	}
	facts = append(facts, kv{"session", t.dim.Render(w.Session)})
	if w.Dash != "" {
		facts = append(facts, kv{"dash", lipgloss.NewStyle().Hyperlink(w.Dash).Underline(true).Foreground(t.accent.GetForeground()).Render(w.Dash)})
	}
	if w.Handoff != "" {
		facts = append(facts, kv{"handoff", t.text.Render(w.Handoff)})
	}
	if w.Off > 0 {
		facts = append(facts, kv{"off", t.dim.Render(fmt.Sprintf("%d feature%s switched off · /status", w.Off, plural(w.Off)))})
	}
	lw := 0
	for _, f := range facts {
		lw = max(lw, ansi.StringWidth(f.k))
	}
	for _, f := range facts {
		if f.v == "" || ansi.Strip(f.v) == "" {
			continue
		}
		k := t.dim.Render(smallCap(f.k) + strings.Repeat(" ", lw-ansi.StringWidth(f.k)))
		rows = append(rows, k+"  "+ansi.Truncate(f.v, inner-lw-2, "…"))
	}
	if len(w.Holes) > 0 {
		rows = append(rows, "")
		for _, h := range w.Holes {
			for i, l := range strings.Split(wrap(h, inner-2), "\n") {
				p := "  "
				if i == 0 {
					p = t.warn.Render("▲ ")
				}
				rows = append(rows, p+t.text.Render(ansi.Truncate(l, inner-2, "…")))
			}
		}
	}
	rows = append(rows, "", t.startHere(inner))
	if t.Depth {
		// the hero slab: its face drifts from the primary's tint to the magenta's, row by row
		h, end := t.surfHero()
		return t.slab(h, rows, cw, slabOpt{top: true, grad: end})
	}
	return t.panel("", rows, cw)
}

// tailPath keeps a path's end when it must be cut — the workspace's own name is the part that
// says where you are: "…/kaginari/agent-one", never "/home/monta/kag…".
func tailPath(p string, w int) string {
	if ansi.StringWidth(p) <= w || w < 8 {
		return p
	}
	r := []rune(p)
	for len(r) > 0 && ansi.StringWidth(string(r))+1 > w {
		r = r[1:]
	}
	s := string(r)
	if i := strings.Index(s, "/"); i > 0 && i < len(s)-1 {
		s = s[i:]
	}
	return "…" + s
}

// startHere is the welcome's last row: the first keys worth knowing, key in the label tone,
// its action dim — as many as fit.
func (t Theme) startHere(w int) string {
	pairs := [][2]string{{"⏎", "ask"}, {"/", "commands"}, {"ctrl+k", "palette"}, {"?", "keys"}, {"ctrl+t", "agents"}, {"esc", "interrupt"}}
	out := ""
	for _, p := range pairs {
		item := t.label.Bold(true).Render(p[0]) + " " + t.dim.Render(p[1])
		next := item
		if out != "" {
			next = out + t.dim.Render("   ") + item
		}
		if ansi.StringWidth(next) > w {
			break
		}
		out = next
	}
	return out
}

// Roles is /roles: every role and the model it runs on, a label/value grid in a card.
func (t Theme) Roles(roles []string, width int) string {
	if len(roles) == 0 {
		return t.dim.Render("no roles configured — every hat runs on the session's model")
	}
	lw := 0
	for _, o := range roles {
		name, _, _ := strings.Cut(o, " → ")
		lw = max(lw, ansi.StringWidth(name))
	}
	var rows []string
	for _, o := range roles {
		name, model, _ := strings.Cut(o, " → ")
		rows = append(rows, t.label.Render(name+strings.Repeat(" ", lw-ansi.StringWidth(name)))+"  "+t.text.Render(model))
	}
	return t.panel(t.title("roles"), rows, min(width, 84))
}

// roleSummary folds the roles to one line: roles sharing a model ride together
// ("analyst judge drafter → gemini-flash"), the model's provider prefix dropped.
func roleSummary(roles []string) string {
	if len(roles) == 0 {
		return ""
	}
	var order []string
	by := map[string][]string{}
	for _, o := range roles {
		name, model, ok := strings.Cut(o, " → ")
		if !ok {
			name, model = o, ""
		}
		if i := strings.LastIndex(model, "/"); i >= 0 {
			model = model[i+1:]
		}
		if _, seen := by[model]; !seen {
			order = append(order, model)
		}
		by[model] = append(by[model], name)
	}
	var parts []string
	for _, mo := range order {
		names := strings.Join(by[mo], " ")
		if mo == "" {
			parts = append(parts, names)
			continue
		}
		parts = append(parts, names+" → "+mo)
	}
	return fmt.Sprintf("%d · %s", len(roles), strings.Join(parts, " · "))
}

// User is the human's ask: a bar in the human's magenta down its left edge, the words in the
// bright text tone beside it.
func (t Theme) User(text string, width int) string {
	text = strings.TrimSpace(text)
	if t.Depth {
		// the ask as its own slab: magenta-tinted, the human's spine down its left edge
		s, o := t.surfUser(), slabOpt{top: true}
		lines := strings.Split(wrap(text, slabInner(s, width, o)), "\n")
		for i, l := range lines {
			lines[i] = t.user.Render(l)
		}
		return t.slab(s, lines, width, o)
	}
	lines := strings.Split(wrap(text, width-2), "\n")
	bar := t.magenta.Render("▌")
	for i, l := range lines {
		lines[i] = bar + " " + t.user.Render(l)
	}
	return strings.Join(lines, "\n")
}

// Assistant is the model's answer, rendered as markdown in the conversation's text column; the
// turn's first piece holds the primary's ✦ in the gutter.
func (t Theme) Assistant(md string, width int) string {
	return t.answer(md, width, true)
}

func (t Theme) answer(md string, width int, first bool) string {
	s, o, full := t.surfAnswer(), slabOpt{top: true}, width
	if t.Depth {
		width = slabInner(s, width, o)
	}
	out := ""
	if t.Depth {
		out = t.insetCode(md, width-2)
	} else {
		out = t.Markdown(md, width-2)
	}
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	if t.Depth {
		// the answer as a slab with the primary's spine: a turn's pieces join into one (the
		// pane joins them, dropping the seam's edges)
		for i, l := range lines {
			g := "  "
			if i == 0 && first {
				g = t.accent.Render("✦") + " "
			}
			lines[i] = g + l
		}
		return t.slab(s, lines, full, o)
	}
	for i, l := range lines {
		g := "  "
		if i == 0 && first {
			g = t.accent.Render("✦") + " "
		} else if strings.TrimSpace(ansi.Strip(l)) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = g + l
	}
	return strings.Join(lines, "\n")
}

// ToolView is one tool step as the block shows it.
type ToolView struct {
	ID      string
	Name    string
	Summary string
	Class   string
	Status  string // running · done · failed · denied · refused · would-ask · would-run · pending
	Ms      int64
	Output  string
	Why     string // a denial or refusal reason
	Wrote   []string
	Diff    *Diff
	Expand  bool
	Agent   string // the agent running it, when not the session
	Link    string // a file:// URL for the summary: a click opens the file (OSC 8)
}

// Tool renders a tool block as a card: an edge in the class's colour down its left side, the
// header line, the status line, the output or the diff.
func (t Theme) Tool(v ToolView, width int) string {
	if t.Depth {
		// a raised chip in its class's hue: the header on the lit band, the status under it
		s, o := t.surfTool(v.Class), slabOpt{band: 1}
		return t.slab(s, strings.Split(t.toolBody(v, slabInner(s, width, o)), "\n"), width, o)
	}
	edge := t.class(v.Class).Render("▎")
	lines := strings.Split(t.toolBody(v, width-2), "\n")
	for i, l := range lines {
		lines[i] = edge + " " + l
	}
	return strings.Join(lines, "\n")
}

func (t Theme) toolBody(v ToolView, width int) string {
	var b strings.Builder
	dot := t.dim.Render("●")
	switch v.Status {
	case "done":
		dot = t.ok.Render("●")
	case "failed", "denied", "refused":
		dot = t.bad.Render("●")
	case "would-ask", "would-run":
		dot = t.warn.Render("●")
	}
	tag := t.class(v.Class).Render(v.Class)
	head := dot + " " + t.text.Bold(true).Render(v.Name)
	if v.Agent != "" {
		head += t.dim.Render(" (" + v.Agent + ")")
	}
	room := width - ansi.StringWidth(head) - ansi.StringWidth(tag) - 4
	if s := oneLine(v.Summary); s != "" && room > 8 {
		s = ansi.Truncate(s, room, "…")
		if v.Link != "" {
			s = lipgloss.NewStyle().Hyperlink(v.Link).Underline(true).Render(s)
		}
		head += "  " + s
	}
	head += "  " + tag
	b.WriteString(head)
	b.WriteString("\n")
	// the status line
	st := ""
	switch v.Status {
	case "running", "pending":
		st = t.dim.Render("⎿ running…")
	case "done":
		parts := []string{t.ok.Render("ok"), elapsed(v.Ms)}
		if v.Diff != nil {
			parts = append(parts, v.Diff.Stat())
		} else if n := countLines(v.Output); n > 0 {
			parts = append(parts, fmt.Sprintf("%d line%s", n, plural(n)))
		}
		if !v.Expand && (v.Diff != nil || countLines(v.Output) > 0) {
			parts = append(parts, "ctrl+o")
		}
		st = t.dim.Render("⎿ " + strings.Join(parts, " · "))
	case "failed":
		st = t.dim.Render("⎿ ") + t.bad.Render("failed") + t.dim.Render(" · "+elapsed(v.Ms))
	case "denied":
		st = t.dim.Render("⎿ ") + t.bad.Render("denied at the gate") + t.dim.Render(reason(v.Why))
	case "refused":
		st = t.dim.Render("⎿ ") + t.bad.Render("refused") + t.dim.Render(reason(v.Why))
	case "would-ask":
		st = t.dim.Render("⎿ ") + t.warn.Render("dry run: would ask") + t.dim.Render(reason(v.Why))
	case "would-run":
		st = t.dim.Render("⎿ ") + t.warn.Render("dry run: would run")
	default:
		st = t.dim.Render("⎿ " + v.Status)
	}
	b.WriteString(indent + ansi.Truncate(st, width-2, "…"))
	// the agent rides only under ctrl+o: logs are for the asking, never by default
	if v.Diff != nil && v.Status == "done" {
		if d := t.diffLines(*v.Diff, width-4, v.Expand); v.Expand && d != "" {
			b.WriteString("\n" + d)
		}
		return b.String()
	}
	switch v.Status {
	case "running", "pending", "denied", "refused", "would-ask", "would-run":
		return b.String()
	}
	if !v.Expand {
		return b.String()
	}
	out := strings.TrimRight(v.Output, "\n")
	if out == "" || out == "(no output)" {
		return b.String()
	}
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		b.WriteString("\n" + indent + indent + ansi.Truncate(expandTabs(l), width-4, "…"))
	}
	return b.String()
}

func (t Theme) diffLines(d Diff, width int, expand bool) string {
	if d.Truncated {
		return indent + indent + t.dim.Render("(too large to diff line by line)")
	}
	lines := d.Lines
	hidden := 0
	if !expand && len(lines) > collapseDiff {
		lines = lines[:collapseDiff]
		hidden = len(d.Lines) - collapseDiff
	}
	var out []string
	numw := 1
	for _, l := range d.Lines {
		if n := max(l.Old, l.New); len(itoa(n)) > numw {
			numw = len(itoa(n))
		}
	}
	for _, l := range lines {
		var s string
		switch l.Kind {
		case '~':
			s = t.dim.Render(strings.Repeat(" ", numw) + " ⋯")
		case '+', '-':
			n, sign, fg := l.New, "+ ", t.add
			if l.Kind == '-' {
				n, sign, fg = l.Old, "- ", t.del
			}
			text := ansi.Truncate(expandTabs(l.Text), width-numw-4, "…")
			bg := t.diffBg(l.Kind)
			if d.Path != "" && t.Color {
				// the code in its own colours over the line's tint, the tint to the edge
				pad := width - numw - 3 - ansi.StringWidth(text)
				agent := t.Highlight(d.Path, text, t.text, bg) + lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", max(0, pad)))
				s = t.lineNo.Render(fmt.Sprintf("%*d", numw, n)) + " " + fg.Background(bg).Render(sign) + agent
			} else {
				s = t.lineNo.Render(fmt.Sprintf("%*d", numw, n)) + " " + fg.Render(sign+text)
			}
		default:
			text := ansi.Truncate(expandTabs(l.Text), width-numw-4, "…")
			if d.Path != "" && t.Color {
				s = t.lineNo.Render(fmt.Sprintf("%*d", numw, l.New)) + "   " + t.Highlight(d.Path, text, t.ctxLine, nil)
			} else {
				s = t.lineNo.Render(fmt.Sprintf("%*d", numw, l.New)) + " " + t.ctxLine.Render("  "+text)
			}
		}
		out = append(out, indent+indent+s)
	}
	if hidden > 0 {
		out = append(out, indent+indent+t.dim.Render(fmt.Sprintf("… +%d line%s (ctrl+o to expand)", hidden, plural(hidden))))
	}
	return strings.Join(out, "\n")
}

// SubagentView is a dispatched Subagent: who, where, on what, its state, and its report when in.
type SubagentView struct {
	Name     string
	Rank     string
	Role     string
	Model    string
	State    string // queued · thinking · tool · waiting on gate · done
	Exit     string // the run journal's t:end verdict (DONE · FAIL · ESCALATE · DENIED · INTERRUPTED) — the card face; lifecycle state alone never is
	Elapsed  time.Duration
	Ask      string
	Report   *ReportView
	Failed   bool
	Word     string // "subagent" | "subagent"
	Expanded bool
	started  time.Time
}

// ReportView is a wire report rendered for the operator.
type ReportView struct {
	Status   string
	Findings []string
	Verdicts []string
	Holes    []string
	Unsaid   []string // "kind: text"
	Other    []string
}

// Pulse is the live motion one Subagent row holds while it rides the live area (N15/N16 slice 1):
// the model computes it off the 200ms tick and the block stays a pure function of it. Its zero
// value is the static render the goldens pin — tombstones, printed blocks and tests carry none.
// Lane is never nil on a live row (the model falls back to the accent).
type Pulse struct {
	Phase      int         // frames since the state began, quarter rate once the agent cools idle
	Lane       color.Color // the rank's lane: the motion's colour
	Tool       string      // the tool's name while the state is "tool"
	Frames     []string    // the session spinner's frame set while the state is "tool"
	Bob        float64     // the gate-wait spring's position, in columns off the right edge
	Flash      float64     // 1 → 0: the landing flash's decay on a done/fail Subagent
	Principles [2]string   // the rank's two-row sprite beside the live card (N16 slice 4)
}

// Subagent renders a Subagent block. pulses carries at most one Pulse — a live row's motion; the
// call sites that print a finished Subagent pass none, and their render is byte-stable.
func (t Theme) Subagent(v SubagentView, width int, pulses ...Pulse) string {
	p := Pulse{}
	if len(pulses) > 0 {
		p = pulses[0]
	}
	live := p.Lane != nil
	// the rank principles (slice 4): a live card shifts right of its two-row sprite; too narrow a
	// window drops the principles rather than starving the card
	cw := width
	principles := live && p.Principles[0] != ""
	if principles {
		if cw = width - 8; cw < 30 {
			principles, cw = false, width
		}
	}
	// under the depth the card is a raised chip in the rank's lane: its text sits inside the
	// chip's padding, the principles stays on the plane beside it
	tw := cw
	var cs surf
	co := slabOpt{band: 1}
	if t.Depth {
		cs = t.surfSubagent(v.Rank, p.Lane)
		tw = slabInner(cs, cw, co)
	}
	word := v.Word
	if word == "" {
		word = "subagent"
	}
	var b strings.Builder
	dot := t.rank(v.Rank).Render("◆")
	head := dot + " " + t.dim.Render(word) + " " + t.rank(v.Rank).Bold(true).Render(v.Name)
	meta := []string{}
	if v.Rank != "" {
		meta = append(meta, v.Rank)
	}
	if v.Role != "" {
		meta = append(meta, v.Role)
	}
	if v.Model != "" {
		meta = append(meta, v.Model)
	}
	if len(meta) > 0 {
		head += t.dim.Render(" · " + strings.Join(meta, " · "))
	}
	// subagent-card-exit-verdict policy (harness leg 2a): the face shows the run journal's exit
	// verdict, never the lifecycle alone — a FAIL verdict reads `FAIL · 2m35s`, while plain
	// lifecycle `done` (dispatch returned) is demoted to the grey ask-line, the tooltip tier.
	face := v.Exit
	if face == "" && v.Report != nil && v.Report.Status != "" {
		// no lifted verdict (working-notes-filled view, golden, hand-built path): the report's first
		// @S token still outranks the bare lifecycle word — never "done" over a FAILED journal
		if tok := strings.ToUpper(strings.Fields(v.Report.Status)[0]); tok == "FAIL" || tok == "ESCALATE" || tok == "DENIED" || tok == "INTERRUPTED" {
			face = tok
		}
	}
	if face == "" {
		face = v.State
	}
	if face == "" {
		face = "queued"
	}
	// a done row's verdict colour split is decided once: the flash reads it, the settle does too
	failed := v.Failed
	if v.State == "done" {
		verdict := v.Exit
		if verdict == "" && v.Report != nil {
			verdict = v.Report.Status
		}
		failed = failed || strings.HasPrefix(verdict, "FAIL") || strings.HasPrefix(verdict, "DENIED") || strings.HasPrefix(verdict, "ESCALATE") || strings.HasPrefix(verdict, "INTERRUPTED")
	}
	verb := strings.ToUpper(face)
	aged := ""
	if v.Elapsed > 0 {
		aged = " · " + v.Elapsed.Round(time.Second).String()
	}
	fgLane := lipgloss.NewStyle().Foreground(p.Lane)
	var stS string
	bob := 0
	switch {
	case live && v.Exit == "" && v.State == "thinking":
		// thinking: the shimmer sweeps the verb, topped on the rank's lane
		stS = t.shimmerText(verb, shimmerPhase(p.Phase, len([]rune(verb))), t.dim.GetForeground(), p.Lane) + t.dim.Render(aged)
	case live && v.Exit == "" && v.State == "tool":
		// a tool runs: the spinner's frame set, held in the lane, names the tool
		frame := "•"
		if len(p.Frames) > 0 {
			phase := p.Phase
			if phase < 0 {
				phase = 0
			}
			frame = p.Frames[phase%len(p.Frames)]
		}
		if p.Tool != "" {
			verb += " " + p.Tool
		}
		stS = fgLane.Bold(true).Render(frame) + " " + fgLane.Render(verb) + t.dim.Render(aged)
	case live && v.Exit == "" && v.State == "waiting on gate":
		// waiting on the gate: a slow spring bobs the status off the right edge and back
		bob = int(math.Round(min(gateBobCols, max(0, p.Bob))))
		stS = fgLane.Render(verb) + t.dim.Render(aged)
	default:
		st := verb + aged
		stS = t.dim.Render(st)
		if v.State == "done" {
			if failed {
				stS = t.bad.Render(st)
			} else {
				stS = t.ok.Render(st)
			}
			// the sequin beat: while the row still pulses the verdict, do a one-shot echo
			// of the verdict word in its own API-green or fail-red glyph (sequin style: a
			// characterised string, not throat noise) — cleared on settle
			if p.Flash >= 0.1 {
				word := "PASS"
				if failed {
					word = verb
				}
				replay := lipgloss.NewStyle().Foreground(lipgloss.Color("#ededed")).Background(map[bool]color.Color{true: t.bad.GetForeground(), false: t.ok.GetForeground()}[failed]).Bold(true)
				stS += " " + replay.Render(" "+word+" ")
			}
		}
	}
	gap := tw - ansi.StringWidth(head) - ansi.StringWidth(stS) - 2 - bob
	if gap < 2 {
		b.WriteString(ansi.Truncate(head, tw-ansi.StringWidth(stS)-3, "…") + "  " + stS)
	} else {
		b.WriteString(head + strings.Repeat(" ", gap) + stS + strings.Repeat(" ", bob))
	}
	if a := oneLine(v.Ask); a != "" {
		if v.Exit != "" && v.Exit != v.State && v.State != "" {
			a += " · " + v.State // lifecycle, demoted
		}
		b.WriteString("\n" + indent + t.dim.Render("└ ") + ansi.Truncate(t.dim.Render(a), tw-4, "…"))
	}
	// the landing flash: for its first beat the just-done row reads as a lane (fail: bad)
	// silhouette — every glyph one colour — then settles to the natural verdict colours.
	flash := func(s string) string {
		if p.Flash < 0.45 || v.State != "done" {
			return s
		}
		st := lipgloss.NewStyle()
		if t.Color {
			fc := p.Lane
			if failed {
				fc = t.bad.GetForeground()
			}
			if fc == nil {
				fc = t.accent.GetForeground()
			}
			st = st.Foreground(fc)
		}
		if p.Flash >= 0.75 {
			st = st.Bold(true)
		}
		lines := strings.Split(ansi.Strip(s), "\n")
		for i, l := range lines {
			lines[i] = st.Render(l)
		}
		return strings.Join(lines, "\n")
	}
	// dress: the principles beside the live card, then the flash over the whole dressed card — the
	// land beat silhouettes the sprite with its row.
	dress := func(s string) string {
		if t.Depth {
			s = t.slab(cs, strings.Split(flash(s), "\n"), cw, co)
			if principles {
				lines := strings.Split(s, "\n")
				for i, l := range lines {
					mark := strings.Repeat(" ", 8)
					if i < 2 {
						mark = flash(p.Principles[i]) + "  "
					}
					lines[i] = mark + l
				}
				s = strings.Join(lines, "\n")
			}
			return s
		}
		if principles {
			lines := strings.Split(s, "\n")
			pad := indent + indent + indent + indent
			for i, l := range lines {
				mark := pad
				if i == 0 {
					mark = p.Principles[0] + "  "
				} else if i == 1 {
					mark = p.Principles[1] + "  "
				}
				lines[i] = mark + l
			}
			s = strings.Join(lines, "\n")
		}
		return flash(s)
	}
	if v.Report == nil {
		return dress(b.String())
	}
	r := v.Report
	status := t.ok.Bold(true).Render(r.Status)
	if v.Failed || strings.HasPrefix(r.Status, "FAIL") || strings.HasPrefix(r.Status, "DENIED") || strings.HasPrefix(r.Status, "ESCALATE") {
		status = t.bad.Bold(true).Render(r.Status)
	}
	b.WriteString("\n" + indent + t.dim.Render("⎿ ") + status)
	lines := 0
	total := len(r.Findings) + len(r.Verdicts) + len(r.Holes) + len(r.Unsaid) + len(r.Other)
	limit := total
	if !v.Expanded && total > collapseLines {
		limit = collapseLines
	}
	add := func(mark lipgloss.Style, sym, text string) {
		if lines >= limit {
			lines++
			return
		}
		lines++
		b.WriteString("\n" + indent + indent + mark.Render(sym) + " " + ansi.Truncate(text, tw-6, "…"))
	}
	for _, f := range r.Findings {
		add(t.ok, "●", f)
	}
	for _, v := range r.Verdicts {
		add(t.accent, "◆", v)
	}
	for _, h := range r.Holes {
		add(t.warn, "?", h)
	}
	for _, u := range r.Unsaid {
		add(t.dim, "∴", u)
	}
	for _, o := range r.Other {
		add(t.dim, "·", o)
	}
	if lines > limit {
		b.WriteString("\n" + indent + indent + t.dim.Render(fmt.Sprintf("… +%d line%s (ctrl+o to expand)", lines-limit, plural(lines-limit))))
	}
	return dress(b.String())
}

// ── the rail (N16 slice 2) ──

const (
	// the rail claims a third of the window (the human's 2/3 | 1/3 split), floored — below it
	// the session stays one column. Height under railMinH starves the cards.
	railClaim = 3
	railFloor = 28
	railMinH  = 26
)

// railSize sizes the rail for a window: a third, floored; 0 keeps the session one column.
func railSize(w, h int) int {
	if w < (railFloor+2)*2 || h < railMinH {
		return 0
	}
	rw := w / railClaim
	if rw < railFloor {
		rw = railFloor
	}
	return rw
}

// RailFile is one touched path in the ledger, held in the writing agent's rank lane; the
// session agent's touches ride its rose-gold lane like a Subagent's would.
type RailFile struct {
	Path  string
	Agent string
	Lane  color.Color
}

// RailPlanItem is one open step of an agent's plan deck.
type RailPlanItem struct {
	Text   string
	Status string // pending · in_progress · done · blocked
}

// RailPlan is one agent's plan deck as the rail shows it: whose it is, the rank names the
// rank lane the deck falls back to, Role names the role the agent's turn currently runs
// under (drafter drafts / judge verdicts / analyst reads) — the card holds the role's
// lane when it's known, the rank's when it isn't.
type RailPlan struct {
	Agent string
	Rank  string
	Role  string
	Lane  color.Color
	Items []RailPlanItem
	Stale bool // penned before this session's provisioning: last session's intent shown as memory, never this session's outstanding work
	// Archived flags a deck that the boot sweep (LEC-2026-10-04-STALE-PLANS Order 1) moved under
	// <planDir>/archive/: no longer live, still browsable in the rail (alt+n snaps off it; ORDER 1's
	// app Plans() merge sets this — until that lands the field stays false and archived decks are
	// indistinguishable from stale ones, which alt+n already treats as a snap-away view).
	Archived bool
}

// RailSubagent is one roster row: the rank dot, the name, the state, the age.
type RailSubagent struct {
	Name   string
	State  string
	Aged   string
	Lane   color.Color
	Done   bool
	Failed bool
}

// RailView is the right rail's data: the workspace's mascot and name, the run's vitals (ctx gauge,
// tokens, cost), the file-touch ledger and the subagent roster — proper cards down the side, the
// board's information architecture carried to the terminal. Max caps shrink cards on short
// windows (0 asks for the default); Mascot nil omits the sprite.
type RailView struct {
	Mascot       []string // the workspace's sprite art, pre-coloured rows
	Workspace    string
	Model        string
	Dash         string // the board's URL, drawn as a clickable dash link in the vitals card
	CtxPct       int
	CtxKnown     bool
	Stress       bool
	Zone         string // the context zone the vitals card paints: OK quiet, NEAR warn, STRESS bad
	Tokens       int
	Cost         string
	CtxMax       string      // configured ctx cap ("128k" | "∞"), shown at the meter's end
	Layers       []RailLayer // per-race token totals of this session, session first
	Plan         *RailPlan   // nil when no agent has penned its deck yet
	Files        []RailFile
	Subagents    []RailSubagent
	MaxFiles     int
	MaxSubagents int
	Collapsed    map[string]bool // card key ("subagents" | "plan" | "files") → collapsed to its header
}

const railFilesMax = 10

// filesTree renders the touch ledger as a lipgloss tree: directories collapse to one dim
// root each, the touched files hang under them in their agent's rank lane. A bare filename
// (no dir) rides the root directly, unenclosed.
func filesTree(files []RailFile, t Theme) string {
	type dirGroup struct {
		dir   string
		items []RailFile
	}
	var groups []dirGroup
	seen := map[string]int{}
	for _, f := range files {
		dir, _ := splitPath(f.Path)
		if i, ok := seen[dir]; ok {
			groups[i].items = append(groups[i].items, f)
			continue
		}
		seen[dir] = len(groups)
		groups = append(groups, dirGroup{dir: dir, items: []RailFile{f}})
	}
	root := tree.New().
		Enumerator(tree.RoundedEnumerator).
		EnumeratorStyle(t.dim.PaddingRight(1)).
		IndenterStyle(t.dim.PaddingRight(1))
	for _, g := range groups {
		if g.dir == "" {
			for _, f := range g.items {
				root.Child(styledLeaf(f, t, f.Path))
			}
			continue
		}
		lanes := map[string]color.Color{}
		sub := tree.Root(t.dim.Render(g.dir + "/"))
		sub.Enumerator(tree.RoundedEnumerator).EnumeratorStyle(t.dim.PaddingRight(1)).IndenterStyle(t.dim.PaddingRight(1))
		for _, f := range g.items {
			_, base := splitPath(f.Path)
			lanes[base] = f.Lane
			sub.Child(base)
		}
		sub.ItemStyleFunc(func(children tree.Children, i int) lipgloss.Style {
			lane := lanes[children.At(i).Value()]
			if lane == nil {
				return t.dim
			}
			return lipgloss.NewStyle().Foreground(lane)
		})
		root.Child(sub)
	}
	return root.String()
}

// planLine flattens one deck line to plain terminal text: one line, markdown noise
// (backticks, stars, leading bullets/numbers) stripped — the card shows the step, not
// the formatting.
func planLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimLeft(s, "-*+ ")
	if idx := strings.Index(s, ". "); idx > 0 && idx <= 2 {
		allDigits := true
		for _, r := range s[:idx] {
			if r < '0' || r > '9' {
				allDigits = false
			}
		}
		if allDigits {
			s = s[idx+2:]
		}
	}
	repl := strings.NewReplacer("`", "", "**", "", "__", "", "~~", "")
	return strings.TrimSpace(repl.Replace(s))
}

// splitPath splits "tui/model.go" into "tui", "model.go"; a bare name yields "", name.
func splitPath(p string) (dir, base string) {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i], p[i+1:]
	}
	return "", p
}

// styledLeaf renders a root-level touched path in its rank lane (dim when the writing
// agent's lane is unknown).
func styledLeaf(f RailFile, t Theme, shown string) string {
	if f.Lane == nil {
		return t.dim.Render(shown)
	}
	return lipgloss.NewStyle().Foreground(f.Lane).Render(shown)
}

// centre pads s into the middle of w cells (right-biased on an odd cell), for the pane's
// mascot rows; callers wrap the result in the pane wipe.
func centre(s string, w int) string {
	pad := w - ansi.StringWidth(s)
	if pad <= 0 {
		return s
	}
	left := pad / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", pad-left)
}

// Rail renders the sidebar column on its own (the goldens' view): each content row padded one
// cell a side, each section rule as the frame draws it between the divider and the border.
func (t Theme) Rail(v RailView, widths ...int) string {
	rw := railFloor
	if len(widths) > 0 {
		rw = widths[0]
	}
	var out []string
	for _, r := range t.railRows(v, rw) {
		switch {
		case r.sep:
			out = append(out, fit(r.text, rw))
		case t.Depth:
			out = append(out, " "+fit(r.text, rw-1))
		default:
			out = append(out, " "+fit(r.text, rw-2)+" ")
		}
	}
	return strings.Join(out, "\n")
}

// railRows lays the sidebar out as rows for a column rw cells wide: the session's vitals (the
// workspace's sprite beside them where it fits), then a ruled section each for the subagents, the
// plan deck and the files touched. A section's rule carries its title, a fold glyph and its
// count; a folded section is its rule alone (alt+c/p/f, or a click on the rule).
func (t Theme) railRows(v RailView, rw int) []railRow {
	inner := rw - 2
	tileW := inner
	if t.Depth {
		// a tile spans the column but its left margin: its shadow falls on the last cell
		tileW = rw - 1
		inner = slabInner(t.surfTile(nil), tileW, slabOpt{band: 1})
	}
	// the sections as they are built: the vitals (no header), then one per section — flat they
	// read as rules between rows, under the depth as stacked raised tiles
	type section struct {
		key, head string
		hue       color.Color
		rows      []railRow
	}
	secs := []*section{{}}
	cur := secs[0]
	var out []railRow
	line := func(s string) { cur.rows = append(cur.rows, railRow{text: s}) }
	rule := func(key, title string, hue lipgloss.Style, count int) {
		fold := "▾"
		if v.Collapsed[key] {
			fold = "▸"
		}
		head := t.dim.Render(fold+" ") + hue.Bold(true).Render(title)
		if count >= 0 {
			head += t.dim.Render(fmt.Sprintf(" · %d", count))
		}
		cur = &section{key: key, head: head, hue: hue.GetForeground()}
		secs = append(secs, cur)
	}
	assemble := func() []railRow {
		if !t.Depth {
			for _, sc := range secs {
				if sc.key != "" {
					out = append(out, railRow{text: t.rule(rw, sc.head, ""), sep: true, key: sc.key})
				}
				out = append(out, sc.rows...)
			}
			return out
		}
		for i, sc := range secs {
			hue := sc.hue
			if hue == t.label.GetForeground() {
				hue = nil
			}
			if i == 0 {
				hue = t.accent.GetForeground()
			}
			ts, o := t.surfTile(hue), slabOpt{band: 1}
			var texts []string
			var meta []railRow
			if sc.key != "" {
				texts, meta = append(texts, sc.head), append(meta, railRow{key: sc.key})
			} else {
				o = slabOpt{top: true}
			}
			for _, r := range sc.rows {
				texts, meta = append(texts, r.text), append(meta, r)
			}
			lines := strings.Split(t.slab(ts, texts, tileW, o), "\n")
			k := 0
			if o.top {
				out = append(out, railRow{text: lines[0]})
				k = 1
			}
			for j, m := range meta {
				m.text = lines[k+j]
				out = append(out, m)
			}
			out = append(out, railRow{text: lines[len(lines)-1]})
		}
		return out
	}

	// vitals: who and where, the context, the spend
	var id []string
	if v.Workspace != "" {
		id = append(id, t.label.Bold(true).Render(v.Workspace))
	}
	if v.Model != "" {
		id = append(id, t.dim.Render(v.Model))
	}
	ctx := t.dim.Render("ctx —")
	if v.CtxKnown {
		cellsN := 8
		if t.Depth {
			// the tile's padding narrows the facts' column: the meter yields before the cap does
			factsW := inner
			if len(v.Mascot) > 0 {
				factsW = inner - ansi.StringWidth(v.Mascot[0]) - 2
			}
			cellsN = min(max(factsW-12, 4), 8)
		}
		ctx = t.Meter(v.CtxPct, cellsN) + " " + t.text.Render(fmt.Sprintf("%d%%", v.CtxPct))
		if v.Stress {
			ctx += " " + t.bad.Render("stress")
		}
	}
	if v.CtxMax != "" {
		ctx += t.dim.Render(" / " + ansi.Truncate(v.CtxMax, 5, ""))
	}
	id = append(id, ctx)
	usage := t.text.Render(humanTokens(v.Tokens)) + t.dim.Render(" tok")
	if strings.HasPrefix(v.Cost, "$") {
		usage += t.dim.Render(" · ") + t.text.Render(v.Cost)
	}
	id = append(id, usage)
	art := v.Mascot
	artW := 0
	for _, l := range art {
		artW = max(artW, ansi.StringWidth(l))
	}
	switch {
	case len(art) > 0 && (inner >= artW+2+18 || t.Depth && inner >= artW+2+14):
		// the sprite beside the facts: a portrait and its caption
		for i := 0; i < max(len(art), len(id)); i++ {
			a := strings.Repeat(" ", artW)
			if i < len(art) {
				a = art[i] + padRight(art[i], artW)
			}
			r := ""
			if i < len(id) {
				r = ansi.Truncate(id[i], inner-artW-2, "…")
			}
			line(a + "  " + r)
		}
	case len(art) > 0 && inner >= artW:
		for _, a := range art {
			line(centre(a, inner))
		}
		line("")
		for _, r := range id {
			line(r)
		}
	default:
		for _, r := range id {
			line(r)
		}
	}
	if len(v.Layers) > 0 {
		// each rank's spend as a chip in its lane; chips wrap whole, never cut
		cur := ""
		for _, l := range v.Layers {
			c := t.chip(l.Lane, l.Rank+" "+humanTokens(l.Tokens))
			if cur != "" && ansi.StringWidth(cur)+1+ansi.StringWidth(c) > inner {
				line(cur)
				cur = ""
			}
			if cur != "" {
				cur += " "
			}
			cur += c
		}
		if cur != "" {
			line(cur)
		}
	}
	switch v.Zone {
	case "NEAR":
		line(t.warn.Render("▲ zone near"))
	case "STRESS":
		line(t.bad.Render("▲ zone stress — write to the log"))
	}
	if v.Dash != "" {
		// the board rides as a terminal hyperlink (OSC 8): the terminal's link modifier opens it
		shown := ansi.Truncate(strings.TrimPrefix(v.Dash, "http://"), max(inner-7, 8), "…")
		line(t.dim.Render("dash ↗ ") + lipgloss.NewStyle().Hyperlink(v.Dash).Underline(true).Foreground(t.accent.GetForeground()).Render(shown))
	}

	// subagents: a roster, the state a chip on the right edge
	subagents := v.Subagents
	rule("subagents", "subagents", t.label, len(subagents))
	if !v.Collapsed["subagents"] {
		maxSubagents := v.MaxSubagents
		if maxSubagents <= 0 {
			maxSubagents = 8
		}
		if len(subagents) == 0 {
			line(t.dim.Render("· none yet"))
		}
		if len(subagents) > maxSubagents {
			subagents = subagents[:maxSubagents]
		}
		for _, c := range subagents {
			hue := lipgloss.NewStyle().Foreground(c.Lane)
			if c.Lane == nil {
				hue = t.dim
			}
			word := strings.ToUpper(c.State)
			if word == "" && c.Done {
				word = "DONE"
			}
			var state string
			switch {
			case c.Done && c.Failed:
				state = t.bad.Render("✗ " + word)
			case c.Done:
				state = t.ok.Render("✓ " + word)
			default:
				state = hue.Render(word)
			}
			if c.Aged != "" && inner-ansi.StringWidth(c.Name)-ansi.StringWidth(state) > 10 {
				state += t.dim.Render(" " + c.Aged)
			}
			dot := hue.Render("●")
			if c.Done {
				dot = t.dim.Render("●")
			}
			nameW := max(inner-2-ansi.StringWidth(state)-1, 4)
			name := ansi.Truncate(c.Name, nameW, "…")
			row := dot + " " + t.text.Render(name)
			row += strings.Repeat(" ", max(inner-ansi.StringWidth(row)-ansi.StringWidth(state), 1)) + state
			cur.rows = append(cur.rows, railRow{text: row, subagent: c.Name})
		}
	}

	// the plan deck, titled in its role's lane — the card's one hue
	if v.Plan != nil {
		lane := v.Plan.Lane
		if lane == nil {
			lane = t.lane("drafter")
		}
		hue := lipgloss.NewStyle().Foreground(lane)
		rule("plan", "plan ▸ "+v.Plan.Agent, hue, len(v.Plan.Items))
		if !v.Collapsed["plan"] {
			switch {
			case v.Plan.Archived:
				line(t.dim.Render("archived · alt+n for the current deck"))
			case v.Plan.Stale:
				line(t.dim.Render("stale · alt+n for the current deck"))
			}
			if len(v.Plan.Items) == 0 {
				line(t.dim.Render("· deck empty"))
			}
			for _, it := range v.Plan.Items {
				var mark string
				text := t.text
				switch it.Status {
				case "in_progress":
					mark = hue.Render("▸")
				case "done":
					mark, text = t.ok.Render("✓"), t.dim
				case "blocked":
					mark = t.bad.Render("✗")
				default:
					mark = t.dim.Render("·")
				}
				line(mark + " " + text.Render(ansi.Truncate(planLine(it.Text), inner-2, "…")))
			}
		}
	}

	// files: the touch ledger as a tree, latest last
	rule("files", "files", t.label, len(v.Files))
	if v.Collapsed["files"] {
		return assemble()
	}
	files := append([]RailFile(nil), v.Files...)
	maxFiles := v.MaxFiles
	if maxFiles <= 0 {
		maxFiles = railFilesMax
	}
	hidden := 0
	if len(files) > maxFiles {
		hidden = len(files) - maxFiles
		files = files[len(files)-maxFiles:]
	}
	if len(files) == 0 {
		line(t.dim.Render("· nothing written yet"))
	} else {
		for _, l := range strings.Split(filesTree(files, t), "\n") {
			line(l)
		}
	}
	if hidden > 0 {
		line(t.dim.Render(fmt.Sprintf("… +%d more", hidden)))
	}
	return assemble()
}

// Gate is the one quiet line after a turn that wrote.
// Gate is the turn's verdict as a banner card: the verdict's colour as the block's background,
// foreground picked for contrast on each background. On a colourless profile it degrades to
// the same words, plain.
func (t Theme) Gate(verdict, logRel string, width int, word string) string {
	if word == "" {
		word = "gate"
	}
	mark, hue := "✓", t.ok
	if strings.HasPrefix(strings.ToLower(verdict), "fail") {
		mark, hue = "✗", t.bad
	}
	line := t.pillChip(hue.GetForeground(), mark+" "+word+" · "+verdict)
	if logRel != "" {
		line += t.dim.Render(" · " + logRel)
	}
	return ansi.Truncate(line, width, "…")
}

// Holes are what the engine could not settle, one line each.
func (t Theme) Holes(holes []string, width int) string {
	var out []string
	for _, h := range holes {
		lines := strings.Split(wrap(h, width-2), "\n")
		for i, l := range lines {
			p := "  "
			if i == 0 {
				p = t.warn.Render("? ")
			}
			out = append(out, p+t.dim.Render(l))
		}
	}
	return strings.Join(out, "\n")
}

// Notice is one dim line (a subagent started, a line queued, config reloaded).
func (t Theme) Notice(text string, width int) string {
	return t.dim.Render(ansi.Truncate(text, width, "…"))
}

// Error is the error colour, wrapped to the width: a long error reads whole, never clipped.
func (t Theme) Error(text string, width int) string {
	lines := strings.Split(wrap(expandTabs(strings.TrimRight(text, "\n")), width), "\n")
	for i, l := range lines {
		lines[i] = t.bad.Render(l)
	}
	return strings.Join(lines, "\n")
}

// ChoiceView is an inline choice: the operator gate's approval, or a question the model asks.
type ChoiceView struct {
	Title   string // "Approval" | "Question"
	Class   string // the class tag for an approval ("" for a question)
	Tool    string
	Summary string
	Why     string
	Agent   string // the agent that asks, when not the session
	Options []string
	Notes   []string // one per option, shown under it ("" for none); when compact, inline-suffixed
	Reasons []string // one per option: a non-empty prompt asks for a line of text on pick
	// Recommended: which option the asker lawfully advises (opencode's choice convention
	// brought in-workspace, Operator 2026-10-03) — "" = the asker stays neutral (approvals over
	// outward/destructive acts advise NOTHING: the safe pick is the human's reflex, never ours).
	Recommended string
	Cursor      int
	Free        bool   // free text accepted (typed below)
	Typed       string // the free text so far
	Prompt      string // the free text prompt ("tell the model why")
	Typing      bool   // the human is typing the free text
	Answered    bool   // printed after the answer: the picked option only, no hints
}

// Choice renders the choice block.
func (t Theme) Choice(v ChoiceView, width int) string {
	inner := min(width-4, 96)
	if t.Depth {
		inner = min(width-5, 96) // the float's shadow takes a column
	}
	var rows []string
	title := t.accent.Bold(true).Render(v.Title)
	if v.Agent != "" {
		title += t.dim.Render(" · " + v.Agent)
	}
	rows = append(rows, title)
	if v.Tool != "" {
		head := ""
		if v.Class != "" {
			head = t.class(v.Class).Render(v.Class) + "  "
		}
		head += t.text.Bold(true).Render(v.Tool)
		if s := oneLine(v.Summary); s != "" {
			head += "  " + ansi.Truncate(s, inner-ansi.StringWidth(head)-2, "…")
		}
		rows = append(rows, head)
	}
	if v.Why != "" {
		rows = append(rows, t.Markdown(v.Why, inner))
	}
	rows = append(rows, "")
	for i, o := range v.Options {
		if v.Answered {
			rows = append(rows, t.ok.Render("✓ ")+ansi.Truncate(o, inner-3, "…"))
			continue
		}
		// one row per option — label first, its note inlined dim, the recommendation tagged.
		// No sub-rows: the box's height never varies with the cursor (over/under the float pin).
		row := fmt.Sprintf("%d · %s", i+1, o)
		note := ""
		if i < len(v.Notes) {
			note = v.Notes[i]
		}
		if note != "" {
			row += " · " + note
		}
		if v.Recommended != "" && o == v.Recommended {
			row += " — ◆ recommended"
		}
		label := ansi.Truncate(row, inner-4, "…")
		if i == v.Cursor && !v.Typing {
			// the picked pill: accent fill, dark face, full row width so rows stay symmetrical
			rows = append(rows, lipgloss.NewStyle().Background(t.accent.GetForeground()).Foreground(t.onAccent()).Bold(true).Padding(0, 1).Width(inner).Render(label))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(t.dim.GetForeground()).Padding(0, 1).Width(inner).Render(label))
		}
	}
	if v.Free && !v.Answered {
		if v.Cursor >= len(v.Options) && !v.Typing {
			rows = append(rows, lipgloss.NewStyle().Background(t.accent.GetForeground()).Foreground(t.onAccent()).Bold(true).Padding(0, 1).Render("… · type an answer"))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(t.dim.GetForeground()).Padding(0, 1).Render("…   type an answer"))
		}
	}
	switch {
	case v.Answered:
	case v.Typing:
		rows = append(rows, "", t.dim.Render(v.Prompt+" ")+v.Typed+t.accent.Render("▏"), t.dim.Render("enter sends · esc goes back"))
	default:
		rows = append(rows, "", t.dim.Render("↑↓ or a digit picks · enter confirms"))
	}
	if t.Depth {
		// a float: the title on its lit band, the primary's spine, the deepest shadow
		s := t.surfFloat()
		s.spine = t.accent.GetForeground()
		var out []string
		for _, r := range rows {
			out = append(out, strings.Split(r, "\n")...)
		}
		return t.slab(s, out, inner+5, slabOpt{top: true, band: 1})
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.accent.GetForeground()).Padding(0, 1).Width(inner + 4)
	return box.Render(strings.Join(rows, "\n"))
}

// Spinner is the live line while a turn runs.
func (t Theme) Spinner(frame, verb string, elapsed time.Duration, tokens int, width int) string {
	parts := []string{elapsed.Round(time.Second).String()}
	if tokens > 0 {
		parts = append(parts, humanTokens(tokens)+" tokens")
	}
	parts = append(parts, "esc to interrupt")
	s := t.accent.Render(frame) + " " + verb + " " + t.dim.Render("("+strings.Join(parts, " · ")+")")
	return ansi.Truncate(s, width, "…")
}

// FooterView is what the footer says.
// RailLayer is one layer's spend as the vitals card shows it — the rank, its tokens so far
// this session, and its lane colour.
type RailLayer struct {
	Rank   string
	Tokens int
	Lane   color.Color
}

type FooterView struct {
	Model     string
	Ctx       string // "ctx 12%" or "ctx —"
	Cost      string // "$0.0123" | "no calls yet" | "unpriced"
	Live      int    // live subagents
	Workspace string
	Subagents string // the plural word
	Hint      string // "? for shortcuts"
	Queued    int
	Badge     string // a mode the session is in ("mouse: terminal"), lit in the accent
	Message   string // a transient message shown in place of the hint
	// Zone carries the instrument zone of the session's context reading ("OK", "NEAR",
	// "STRESS", "UNREAD"; "" when the app doesn't read zones). The footer holds it; the
	// sidebar paints it.
	Zone     string
	Tokens   int // the session's tokens so far (the spinner shows them)
	CtxPct   int // the context window's fill in percent, drawn as a meter when CtxKnown
	CtxKnown bool
	// Dash is the board's URL ("" when the board didn't start); the sidebar's vitals card
	// holds it as a terminal hyperlink — a click opens the dash, no paste needed.
	Dash string
	// CtxMax is the configured context cap, top-right of the vitals card ("128k" or "∞").
	CtxMax string
	// Layers are the per-race token totals of this session — session (orchestrator's lane) at
	// index 0, then coordinator, domain owner, zone worker… in dispatch order. The vitals card shows each as a
	// lane-coloured pill.
	Layers []RailLayer
	// CtxTokens/CtxLimit carry the session's spend against its context window (raw counts).
	// When both > 0 and width ≥ 88 the footer draws `ctx ▰▰▰▰ 42% · 84k/200k`; zero values
	// render exactly as before.
	CtxTokens int
	CtxLimit  int
}

// Footer renders the line under the input.
func (t Theme) Footer(f FooterView, width int) string {
	parts := []string{f.Model, f.Ctx, f.Cost}
	parts = parts[:0:0]
	ctx := f.Ctx
	if f.CtxKnown && width >= 72 {
		ctx = "ctx " + t.Meter(f.CtxPct, 8) + fmt.Sprintf(" %d%%", f.CtxPct)
		// raw counts ride only on wide screens — a narrow terminal keeps the meter
		if f.CtxTokens > 0 && f.CtxLimit > 0 && width >= 88 {
			ctx += fmt.Sprintf(" · %s/%s", humanTokens(f.CtxTokens), humanTokens(f.CtxLimit))
		}
		if strings.Contains(f.Ctx, "stress") {
			ctx += " " + t.bad.Render("stress")
		}
	}
	parts = append(parts, t.accent.Render(f.Model), ctx, f.Cost)
	if f.Live > 0 {
		word := f.Subagents
		if word == "" {
			word = "subagents"
		}
		if f.Live == 1 {
			word = strings.TrimSuffix(word, "s")
		}
		parts = append(parts, fmt.Sprintf("%d live %s", f.Live, word))
	}
	if f.Queued > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", f.Queued))
	}
	if f.Badge != "" {
		parts = append(parts, t.accent.Render(f.Badge))
	}
	right := f.Hint
	if f.Message != "" {
		right = f.Message
	}
	join := func(workspace string) string {
		ps := append([]string(nil), parts...)
		if workspace != "" {
			ps = append(ps, workspace)
		}
		return strings.Join(ps, " · ")
	}
	fits := func(left string) bool { return ansi.StringWidth(left)+ansi.StringWidth(right)+3 <= width }
	left := join(f.Workspace)
	if !fits(left) && f.Workspace != "" {
		// the workspace's last two path components, then none
		segs := strings.Split(strings.Trim(f.Workspace, "/"), "/")
		if len(segs) > 2 {
			left = join("…/" + strings.Join(segs[len(segs)-2:], "/"))
		}
		if !fits(left) {
			left = join("")
		}
	}
	if right != "" && fits(left) {
		gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
		return t.dim.Render(left) + strings.Repeat(" ", gap) + t.dim.Render(right)
	}
	return t.dim.Render(ansi.Truncate(left, width, "…"))
}

// MenuItem is one slash command in the menu.
type MenuItem struct {
	Name        string // without the slash
	Description string
}

// Menu renders the slash menu under the input, filtered and with a cursor.
func (t Theme) Menu(items []MenuItem, cursor int, width int) string {
	if len(items) == 0 {
		return t.dim.Render("  no matching command")
	}
	namew := 0
	for _, it := range items {
		if l := len(it.Name) + 1; l > namew {
			namew = l
		}
	}
	var rows []string
	for i, it := range items {
		p := "  "
		name := t.text.Render(fmt.Sprintf("/%-*s", namew, it.Name))
		if i == cursor {
			p = t.accent.Render("❯ ")
			name = t.accent.Bold(true).Render(fmt.Sprintf("/%-*s", namew, it.Name))
		}
		rows = append(rows, ansi.Truncate(p+name+" "+t.dim.Render(it.Description), width, "…"))
	}
	return strings.Join(rows, "\n")
}

// Completions renders @path candidates in a row.
func (t Theme) Completions(paths []string, cursor int, width int) string {
	if len(paths) == 0 {
		return t.dim.Render("  no matching path")
	}
	var parts []string
	for i, p := range paths {
		if i == cursor {
			parts = append(parts, t.accent.Bold(true).Render(p))
		} else {
			parts = append(parts, t.dim.Render(p))
		}
	}
	return ansi.Truncate("  "+strings.Join(parts, "  "), width, "…")
}

// Shortcut is one key and what it does.
type Shortcut struct{ Key, Does string }

// Shortcuts renders the ? panel.
func (t Theme) Shortcuts(list []Shortcut, width int) string {
	kw := 14
	for _, s := range list {
		kw = max(kw, ansi.StringWidth(s.Key)+2)
	}
	var rows []string
	for _, s := range list {
		does := ansi.Truncate(s.Does, max(width-4-kw, 8), "…")
		rows = append(rows, t.accent.Render(s.Key+strings.Repeat(" ", kw-ansi.StringWidth(s.Key)))+t.dim.Render(does))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.border.GetForeground()).Padding(0, 1)
	return box.Render(strings.Join(rows, "\n"))
}

// keysSheet is the ? sheet under the frame: the shortcuts in one column, or two when one would
// stand taller than room rows — keys in the label tone, their action dim.
func (t Theme) keysSheet(list []Shortcut, w, room int) string {
	kw := 0
	for _, s := range list {
		kw = max(kw, ansi.StringWidth(s.Key))
	}
	row := func(s Shortcut, cw int) string {
		return t.label.Render(s.Key+strings.Repeat(" ", kw-ansi.StringWidth(s.Key))) + "  " + t.dim.Render(ansi.Truncate(s.Does, max(cw-kw-2, 4), "…"))
	}
	inner := w - 4
	var rows []string
	if len(list)+2 <= room || inner < 2*(kw+20)+3 {
		for _, s := range list {
			rows = append(rows, row(s, inner))
		}
	} else {
		half := (len(list) + 1) / 2
		colW := (inner - 3) / 2
		for i := 0; i < half; i++ {
			l := fit(row(list[i], colW), colW)
			r := ""
			if i+half < len(list) {
				r = row(list[i+half], colW)
			}
			rows = append(rows, l+"   "+r)
		}
	}
	return t.panel(t.title("keys"), rows, w)
}

// DefaultShortcuts is the list ? shows.
func DefaultShortcuts() []Shortcut {
	return []Shortcut{
		{"ctrl+p", "walk plan decks"},
		{"alt+n", "snap to the current plan (off a stale deck)"},
		{"enter", "send"},
		{"shift+enter", "newline (also \\ enter, alt+enter)"},
		{"↑ ↓", "history (on the first / last line)"},
		{"/", "commands"},
		{"ctrl+k", "command palette (fuzzy)"},
		{"@path", "complete a file in the workspace (tab)"},
		{"esc", "interrupt · clear the input (↑ brings it back)"},
		{"ctrl+c", "clear · interrupt · twice when idle: exit"},
		{"pgup / pgdn", "scroll the chat (shift+↑ ↓ a line, the wheel)"},
		{"ctrl+t", "the agents: every agent's log"},
		{"drag", "select in the chat, copied on release"},
		{"shift+drag", "selects with the terminal (/mouse: always)"},
		{"ctrl+y", "copy the last answer (/copy)"},
		{"ctrl+o", "expand the last collapsed block"},
		{"ctrl+l", "redraw"},
		{"?", "this list"},
		{"alt+p/c/f", "fold the plan / subagents / files sections"},
		{"alt+s", "hide or show the sidebar (/sidebar)"},
	}
}

// Queued is a line typed mid-turn, shown as queued.
func (t Theme) Queued(text string, width int) string {
	return t.dim.Render(ansi.Truncate("  ⏎ queued: "+oneLine(text), width, "…"))
}

// InputBox frames the textarea's view (the inline layout's input): a rounded box in the
// border tone, "ask" set into its top edge.
func (t Theme) InputBox(view string, width int, busy bool) string {
	title := t.title("ask")
	if busy {
		title = t.dim.Render(smallCap("answer the card above"))
	}
	return t.panel(title, []string{view}, width)
}

// helpers

// textSanitize drops U+FE0F (the emoji-presentation selector) before any width-touched text:
// x/ansi honestly counts ⚙️ as 2 cells, but terminals that fall back to the text-presentation
// glyph draw it 1 cell — every padded line then lies by a width and the pane's right edge
// drifts (the "**Maintenance:** ⚙️" reports Operator saw, 2026-10-03). Stripping forces the
// deterministic narrow glyph everywhere the TUI measures.
func textSanitize(s string) string {
	return strings.ReplaceAll(s, "️", "")
}

func wrap(s string, width int) string {
	if width < 10 {
		width = 10
	}
	return ansi.Wrap(textSanitize(s), width, "")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func reason(why string) string {
	if why == "" {
		return ""
	}
	return " · " + oneLine(why)
}

func elapsed(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", ms)
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

func countLines(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" || s == "(no output)" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

func humanTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return itoa(n)
}
