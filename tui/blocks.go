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

// Welcome is the framed box at start.
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
}

// Render the welcome box.
func (t Theme) Welcome(w Welcome, width int) string {
	if w.WorkspaceWord == "" {
		w.WorkspaceWord = "workspace"
	}
	inner := min(width-4, 76)
	if inner < 30 {
		inner = width - 4
	}
	var rows []string
	title := Title(w.Dist) + " " + t.dim.Render(w.Version)
	_, tag := Mascot(w.Dist)
	rows = append(rows, title)
	if tag != "" {
		rows = append(rows, t.dim.Italic(true).Render(ansi.Truncate(tag, inner, "…")))
	}
	rows = append(rows, "")
	rows = append(rows, t.dim.Render("the subagent holds its lane · ")+t.laneLegend(w.Dist), "")
	labels := []string{w.WorkspaceWord, "model", "roles", "session", "handoff", "off"}
	lw := 0
	for _, l := range labels {
		if len(l)+1 > lw {
			lw = len(l) + 1
		}
	}
	row := func(k, v string) {
		if v == "" {
			return
		}
		for i, l := range strings.Split(wrap(v, inner-lw-1), "\n") {
			k2 := k
			if i > 0 {
				k2 = ""
			}
			rows = append(rows, t.dim.Render(fmt.Sprintf("%-*s", lw, k2))+ansi.Truncate(l, inner-lw-1, "…"))
		}
	}
	row(w.WorkspaceWord, w.Workspace)
	row("model", w.Model)
	if len(w.Roles) > 0 {
		row("roles", strings.Join(w.Roles, " · "))
	}
	row("session", w.Session)
	row("handoff", w.Handoff)
	if w.Off > 0 {
		row("off", fmt.Sprintf("%d feature%s switched off by config (/status)", w.Off, plural(w.Off)))
	}
	for _, h := range w.Holes {
		for i, l := range strings.Split(wrap(h, inner-2), "\n") {
			p := "  "
			if i == 0 {
				p = t.warn.Render("? ")
			}
			rows = append(rows, p+ansi.Truncate(l, inner-2, "…"))
		}
	}
	rows = append(rows, "", t.dim.Render("/help for commands · ? for shortcuts · esc interrupts · ctrl+c twice exits"))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(inner + 4) // lipgloss v2: the width includes padding and border
	box = box.BorderForegroundBlend(t.reflecting()...)
	return box.Render(strings.Join(rows, "\n"))
}

// reflecting is the border's reflecting light: the accent, sunk into the pane's dim and
// back again, wrapped so the blend closes around the box (lipgloss blends stop-stop along
// the border path — a wrapped set lands on its own start).
func (t Theme) reflecting() []color.Color {
	a, b := t.accent.GetForeground(), t.border.GetForeground()
	return []color.Color{a, b, a}
}

// User is the human's message as sent.
func (t Theme) User(text string, width int) string {
	text = strings.TrimSpace(text)
	lines := strings.Split(wrap(text, width-2), "\n")
	for i, l := range lines {
		p := "  "
		if i == 0 {
			p = "> "
		}
		lines[i] = t.user.Render(p + l)
	}
	return strings.Join(lines, "\n")
}

// Assistant is the model's answer, rendered as markdown.
func (t Theme) Assistant(md string, width int) string {
	return t.Markdown(md, width)
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
	gap := cw - ansi.StringWidth(head) - ansi.StringWidth(stS) - 2 - bob
	if gap < 2 {
		b.WriteString(ansi.Truncate(head, cw-ansi.StringWidth(stS)-3, "…") + "  " + stS)
	} else {
		b.WriteString(head + strings.Repeat(" ", gap) + stS + strings.Repeat(" ", bob))
	}
	if a := oneLine(v.Ask); a != "" {
		if v.Exit != "" && v.Exit != v.State && v.State != "" {
			a += " · " + v.State // lifecycle, demoted
		}
		b.WriteString("\n" + indent + t.dim.Render("└ ") + ansi.Truncate(t.dim.Render(a), cw-4, "…"))
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
		b.WriteString("\n" + indent + indent + mark.Render(sym) + " " + ansi.Truncate(text, cw-6, "…"))
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
	// <planDir>/archive/: no longer live, still browsable in the rail ('c' snaps off it; ORDER 1's
	// app Plans() merge sets this — until that lands the field stays false and archived decks are
	// indistinguishable from stale ones, which the 'c' gate already treats as snap-away view).
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

// railCard frames one section: a rounded border, its title one small-cap line tinted by the
// card's own lane (vitals golden-the session's, subagents green like their role, files blue,
// plan holds the role lane of the agent that owns the deck). Cards ride the pane's background
// (lipgloss paints a style's background into its padded cells — every line lands full-width).
func (t Theme) railCard(title string, lane color.Color, agent []string, rw int) []string {
	inner := rw - 4 // border 2 + padding 2
	if lane == nil {
		lane = t.accent.GetForeground()
	}
	var rows []string
	rows = append(rows, lipgloss.NewStyle().Foreground(lane).Bold(true).Render(smallCap(title)))
	for _, l := range agent {
		rows = append(rows, ansi.Truncate(l, inner, "…"))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForegroundBlend(t.reflecting()...).Background(t.paneBg()).Padding(0, 1).Width(rw)
	return strings.Split(box.Render(strings.Join(rows, "\n")), "\n")
}

// railCollapsedHeader renders a card's collapsed stub: `+<glyph> <title> (<count>)` inside a
// one-row rounded box, adorning it the way the card's header would.
func (t Theme) railCollapsedHeader(glyph, title string, count int, rw int) []string {
	stub := fmt.Sprintf("+%s %s (%d)", glyph, title, count)
	return t.railCard(stub, nil, nil, rw)
}

// Rail renders the sidebar's card stack at its proportioned width; every returned line is
// exactly rw cells, the pane's background filling what content doesn't touch (mascot rows,
// spacer rows). The pane's height padding lives with the model.
func (t Theme) Rail(v RailView, widths ...int) string {
	rw := railFloor
	if len(widths) > 0 {
		rw = widths[0]
	}
	inner := rw - 4
	bg := lipgloss.NewStyle().Background(t.paneBg())
	wipe := func(l string) string {
		if pad := rw - ansi.StringWidth(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		return bg.Render(l)
	}
	var out []string
	if len(v.Mascot) > 0 {
		for _, l := range v.Mascot {
			out = append(out, wipe(centre(ansi.Truncate(t.paneArt(l), inner, ""), inner)))
		}
		out = append(out, wipe(""))
	}
	var vitals []string
	if v.Workspace != "" {
		vitals = append(vitals, t.tag.Render(ansi.Truncate(v.Workspace, inner, "…")))
	}
	vitals = append(vitals, t.dim.Render(ansi.Truncate(v.Model, inner, "…")))
	ctx := t.dim.Render("ctx —")
	if v.CtxKnown {
		ctx = "ctx " + t.Meter(v.CtxPct, 8) + fmt.Sprintf(" %d%%", v.CtxPct)
		if v.Stress {
			ctx += " " + t.bad.Render("stress")
		}
	}
	if v.CtxMax != "" {
		ctx += t.dim.Render(" / " + ansi.Truncate(v.CtxMax, 5, ""))
	}
	vitals = append(vitals, ctx)
	if len(v.Layers) > 0 {
		// each rank's spend as a lane pill: line-wrap instead of truncating — the badge's
		// name stays whole, the layer's own card budget holds
		var lines []string
		var cur string
		for _, l := range v.Layers {
			lane := l.Lane
			if lane == nil {
				lane = t.dim.GetForeground()
			}
			fg := lipgloss.Color("#0b1424")
			if !t.Dark {
				fg = lipgloss.Color("#f8fafc")
			}
			pill := lipgloss.NewStyle().Background(lane).Foreground(fg).Padding(0, 1).Render(l.Rank + " " + humanTokens(l.Tokens))
			if cur != "" && ansi.StringWidth(cur)+1+ansi.StringWidth(pill) > inner {
				lines = append(lines, cur)
				cur = ""
			}
			if cur != "" {
				cur += " "
			}
			cur += pill
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		vitals = append(vitals, lines...)
	}
	switch v.Zone {
	case "NEAR":
		vitals = append(vitals, t.warn.Render("zone NEAR"))
	case "STRESS":
		vitals = append(vitals, t.bad.Render("zone STRESS"))
	}
	usage := humanTokens(v.Tokens) + " tokens"
	if v.Cost != "" {
		usage += "  " + t.accent.Render(v.Cost)
	}
	vitals = append(vitals, t.dim.Render(usage))
	if v.Dash != "" {
		// the board rides as a terminal-hyperlink (OSC 8): ⌘-click (or the terminal's own
		// link modifier) opens it in the browser — no paste, no copy
		shown := v.Dash
		if room := inner - 6; room > 12 {
			shown = ansi.Truncate(shown, room, "…")
		}
		dash := lipgloss.NewStyle().Hyperlink(v.Dash).Underline(true).Foreground(t.accent.GetForeground()).Render(shown)
		vitals = append(vitals, t.dim.Render("dash ↗ ")+dash)
	}
	out = append(out, t.railCard("vitals", t.lane("orchestrator"), vitals, rw)...)
	out = append(out, wipe(""))

	subagents := v.Subagents
	maxSubagents := v.MaxSubagents
	if v.Collapsed["subagents"] {
		out = append(out, t.railCollapsedHeader("⊕", "subagents", len(subagents), rw)...)
		out = append(out, wipe(""))
	} else {
		if maxSubagents <= 0 {
			maxSubagents = 8
		}
		var subagentRows []string
		if len(subagents) == 0 {
			subagentRows = append(subagentRows, t.dim.Render("· none"))
		}
		if len(subagents) > maxSubagents {
			subagents = subagents[:maxSubagents]
		}
		for _, c := range subagents {
			dot, st := t.dim, t.dim
			if !c.Done {
				dot = lipgloss.NewStyle().Foreground(c.Lane)
				st = dot
			} else if c.Failed {
				dot = t.bad
			} else {
				dot = t.ok
			}
			word := strings.ToUpper(c.State)
			if word == "" && c.Done {
				word = "DONE"
			}
			state := word
			if c.Aged != "" {
				state += " · " + c.Aged
			}
			// keep name+state whole; the age yields first, mid-token chops never
			nameW := inner - ansi.StringWidth("● "+state) - 1
			if nameW < 4 && c.Aged != "" {
				state = word
				nameW = inner - ansi.StringWidth("● "+state) - 1
			}
			row := dot.Render("● ") + t.text.Render(ansi.Truncate(c.Name, max(1, nameW), "")) + " " + st.Render(state)
			subagentRows = append(subagentRows, row)
		}
		out = append(out, t.railCard("subagents", t.lane("domain"), subagentRows, rw)...)
		out = append(out, wipe(""))
	}

	if v.Plan != nil {
		// the whole card holds the role lane of the agent running the deck — the plan is
		// that role's hat work (drafter drafts orange, judge verdicts green, analyst
		// reads blue); the tint reads as "which role is thinking", never the agent's rank
		lane := v.Plan.Lane
		if lane == nil {
			lane = t.lane("drafter")
		}
		cardBg := lipgloss.NewStyle().Background(lipgloss.Blend1D(6, t.paneBg(), lane)[1])
		header := "plan ▸ " + v.Plan.Agent
		switch {
		case v.Plan.Archived:
			header += " · archived (boot-swept under tmp/plans/archive) — c — current"
		case v.Plan.Stale:
			header += " · stale (penned in an earlier session) — c — current"
		}
		if v.Collapsed["plan"] {
			out = append(out, t.railCollapsedHeader("◇", "plan — "+v.Plan.Agent, len(v.Plan.Items), rw)...)
			out = append(out, wipe(""))
		} else {
			var rows []string
			if len(v.Plan.Items) == 0 {
				rows = append(rows, cardBg.Render(ansi.Truncate("  · deck empty", inner, "…")))
			}
			for _, it := range v.Plan.Items {
				var mark string
				switch it.Status {
				case "in_progress":
					mark = t.accent.Render("▸")
				case "done":
					mark = t.ok.Render("✓")
				case "blocked":
					mark = t.bad.Render("✗")
				default:
					mark = t.dim.Render("·")
				}
				text := ansi.Truncate(mark+" "+planLine(it.Text), inner, "…")
				if pad := inner - ansi.StringWidth(text); pad > 0 {
					text += strings.Repeat(" ", pad)
				}
				rows = append(rows, cardBg.Render(text))
			}
			out = append(out, t.railCard(header, lane, rows, rw)...)
			out = append(out, wipe(""))
		}
	}

	if v.Collapsed["files"] {
		n := len(v.Files)
		out = append(out, t.railCollapsedHeader("▪", "files", n, rw)...)
		return strings.Join(out, "\n")
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
	var fileRows []string
	if len(files) == 0 {
		fileRows = append(fileRows, t.dim.Render("· nothing written yet"))
	}
	if len(files) > 0 {
		fileRows = append(fileRows, strings.Split(filesTree(files, t), "\n")...)
	}
	if hidden > 0 {
		fileRows = append(fileRows, t.dim.Render(fmt.Sprintf("· … +%d more", hidden)))
	}
	out = append(out, t.railCard("files", t.lane("analyst"), fileRows, rw)...)
	return strings.Join(out, "\n")
}

// Gate is the one quiet line after a turn that wrote.
// Gate is the turn's verdict as a banner card: the verdict's colour as the block's background,
// foreground picked for contrast on each background. On a colourless profile it degrades to
// the same words, plain.
func (t Theme) Gate(verdict, logRel string, width int, word string) string {
	if word == "" {
		word = "gate"
	}
	mark := "✓"
	bg := t.ok.GetForeground()
	if strings.HasPrefix(strings.ToLower(verdict), "fail") {
		mark = "✗"
		bg = t.bad.GetForeground()
	}
	text := " " + mark + " " + word + " · " + verdict + " "
	if logRel != "" {
		text += "· " + logRel + " "
	}
	fg := lipgloss.Color("#0d1117")
	if !t.Dark {
		fg = lipgloss.Color("#f6f8fa") // the dark-ink band is only light-theme'd once
	}
	card := lipgloss.NewStyle().Background(bg).Foreground(fg).Bold(true)
	return card.Render(ansi.Truncate(text, width-2, "…"))
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

// Error is one line in the error colour.
func (t Theme) Error(text string, width int) string {
	return t.bad.Render(ansi.Truncate(text, width, "…"))
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
			rows = append(rows, lipgloss.NewStyle().Background(t.accent.GetForeground()).Foreground(lipgloss.Color("#0b1424")).Bold(true).Padding(0, 1).Width(inner).Render(label))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(t.dim.GetForeground()).Padding(0, 1).Width(inner).Render(label))
		}
	}
	if v.Free && !v.Answered {
		if v.Cursor >= len(v.Options) && !v.Typing {
			rows = append(rows, lipgloss.NewStyle().Background(t.accent.GetForeground()).Foreground(lipgloss.Color("#0b1424")).Bold(true).Padding(0, 1).Render("… · type an answer"))
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
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForegroundBlend(t.reflecting()...).Padding(0, 1).Width(inner + 4)
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
	var rows []string
	for _, s := range list {
		rows = append(rows, t.accent.Render(fmt.Sprintf("%-14s", s.Key))+t.dim.Render(s.Does))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.border.GetForeground()).Padding(0, 1)
	return box.Render(strings.Join(rows, "\n"))
}

// DefaultShortcuts is the list ? shows.
func DefaultShortcuts() []Shortcut {
	return []Shortcut{
		{"ctrl+p", "walk plan decks"},
		{"c", "snap to current plan (pinned stale/archived view)"},
		{"C", "snap to current plan (any view)"},
		{"enter", "send"},
		{"shift+enter", "newline (also \\ enter, alt+enter)"},
		{"↑ ↓", "history (on the first / last line)"},
		{"/", "commands"},
		{"ctrl+k", "command palette (fuzzy)"},
		{"@path", "complete a file in the workspace (tab)"},
		{"esc", "interrupt the turn · clear the input"},
		{"ctrl+c ×2", "exit"},
		{"ctrl+o", "expand the last collapsed block"},
		{"ctrl+l", "redraw"},
		{"?", "this list"},
		{"alt+p / alt+c / alt+f", "collapse the plan / subagents / files rail cards"},
	}
}

// Queued is a line typed mid-turn, shown as queued.
func (t Theme) Queued(text string, width int) string {
	return t.dim.Render(ansi.Truncate("  ⏎ queued: "+oneLine(text), width, "…"))
}

// InputBox frames the textarea's view with the theme's border.
func (t Theme) InputBox(view string, width int, busy bool) string {
	col := t.border.GetForeground()
	if busy {
		col = t.dim.GetForeground()
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(col).Width(width).Render(view)
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
