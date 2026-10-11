package tui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Kaginari/agent-one/theme"
	"github.com/Kaginari/agent-one/value"
)

// The sidebar as a cockpit column: a header tile (the workspace's sprite, its name, the model and
// the session's state), the vitals as a label/value grid of meters, the ranks' share of the
// work, the subagents (live first, each state a chip), the plan (its progress and the current
// step) and the files written (a tree). Each tile holds its own hue on its title; under the
// depth they are raised slabs stacked down the column, flat they are ruled sections. The
// ranks, subagents, plan and files fold to their title (alt+c/p/f, or a click on the title).

// railTileHues are the tiles' title hues, by key: each region of the column its own colour.
func (t Theme) railTileHue(key string) color.Color {
	switch key {
	case "vitals":
		return t.accent.GetForeground()
	case "ranks":
		return t.lane("orchestrator")
	case "subagents":
		return t.magenta.GetForeground()
	case "plan":
		return t.plan.GetForeground()
	case "files":
		return t.class("read").GetForeground()
	}
	return nil
}

// railRows lays the sidebar out as rows for a column rw cells wide (see the file's comment).
func (t Theme) railRows(v RailView, rw int) []railRow {
	inner := rw - 2
	tileW := inner
	if t.Depth {
		// a tile spans the column but its left margin: its shadow falls on the last cells
		tileW = rw - 1
		inner = slabInner(t.surfTile(nil), tileW, slabOpt{band: 1})
	}
	type section struct {
		key, head string
		hue       color.Color
		rows      []railRow
	}
	var secs []*section
	cur := &section{}
	secs = append(secs, cur)
	line := func(s string) { cur.rows = append(cur.rows, railRow{text: s}) }
	open := func(key, title string, foldable bool, count string) {
		hue := t.railTileHue(key)
		h := ""
		if foldable {
			fold := "▾"
			if v.Collapsed[key] {
				fold = "▸"
			}
			h = t.dim.Render(fold + " ")
		}
		h += lipgloss.NewStyle().Foreground(hue).Bold(true).Render(title)
		if count != "" {
			h += t.dim.Render(" · " + count)
		}
		cur = &section{key: key, head: h, hue: hue}
		secs = append(secs, cur)
	}
	// a label/value row of the vitals grid: the label in small caps, dim, in a fixed column
	const lw = 5
	kv := func(k, val string) string {
		return t.dim.Render(smallCap(k)+strings.Repeat(" ", max(lw-ansi.StringWidth(k), 0))) + " " + val
	}

	// ── the header tile: who, where, on what, doing what
	var id []string
	if v.Workspace != "" {
		id = append(id, Gradient(v.Workspace, true, t.accent.GetForeground(), t.plan.GetForeground()))
	}
	if v.Model != "" {
		id = append(id, t.dim.Render(v.Model))
	}
	id = append(id, t.railState(v))
	art := v.Mascot
	artW := 0
	for _, l := range art {
		artW = max(artW, ansi.StringWidth(l))
	}
	switch {
	case len(art) > 0 && inner >= artW+2+14:
		// the sprite beside the facts, the facts centred on its height
		off := max((len(art)-len(id))/2, 0)
		for i := 0; i < max(len(art), len(id)+off); i++ {
			a := strings.Repeat(" ", artW)
			if i < len(art) {
				a = art[i] + padRight(art[i], artW)
			}
			r := ""
			if j := i - off; j >= 0 && j < len(id) {
				r = ansi.Truncate(id[j], inner-artW-2, "…")
			}
			line(a + "  " + r)
		}
	case len(art) > 0 && inner >= artW:
		for _, a := range art {
			line(centre(a, inner))
		}
		for _, r := range id {
			line(centre(ansi.Truncate(r, inner, "…"), inner))
		}
	default:
		for _, r := range id {
			line(ansi.Truncate(r, inner, "…"))
		}
	}

	// ── vitals: the context meter, the spend, the board
	open("vitals", "vitals", false, "")
	valW := inner - lw - 1
	ctx := t.dim.Render("—")
	if v.CtxKnown {
		pct := t.text.Bold(true).Render(fmt.Sprintf("%d%%", v.CtxPct))
		capS := ""
		if v.CtxMax != "" {
			capS = t.dim.Render(" / " + ansi.Truncate(v.CtxMax, 5, ""))
		}
		cells := valW - ansi.StringWidth(pct) - 1 - ansi.StringWidth(capS)
		if cells < 6 {
			capS = ""
			cells = valW - ansi.StringWidth(pct) - 1
		}
		cells = min(max(cells, 3), 18)
		ctx = t.gauge(float64(v.CtxPct)/100, cells, nil, t.meterHue(meterDef(v.Meters, "ctx"))) + " " + pct + capS
	}
	line(kv("ctx", ctx))
	switch {
	case v.Stress || v.Zone == "STRESS":
		line(kv("", t.bad.Render("▲ stress — write to the log")))
	case v.Zone == "NEAR":
		line(kv("", t.warn.Render("▲ near the edge")))
	}
	line(kv("tok", t.text.Render(humanTokens(v.Tokens))))
	if v.Cost != "" {
		cost := t.dim.Render(v.Cost)
		if strings.HasPrefix(v.Cost, "$") {
			cost = t.text.Render(v.Cost)
		}
		line(kv("cost", cost))
	}
	if v.Score != nil && v.Score.Meter.Label != "" {
		line(kv("val", t.valueBar(*v.Score, valW)))
	}
	if v.Promote != "" {
		line(kv("next", t.label.Render(ansi.Truncate(v.Promote, max(valW, 8), "…"))))
	}
	if v.Evolved != "" {
		line(kv("evo", t.plan.Render(ansi.Truncate("✦ "+v.Evolved, max(valW, 8), "…"))))
	}
	if v.Dash != "" {
		// the board rides as a terminal hyperlink (OSC 8): the terminal's link modifier opens it
		shown := ansi.Truncate(strings.TrimPrefix(v.Dash, "http://"), max(valW-2, 8), "…")
		line(kv("dash", t.dim.Render("↗ ")+lipgloss.NewStyle().Hyperlink(v.Dash).Underline(true).Foreground(t.accent.GetForeground()).Render(shown)))
	}

	// ── ranks: each rank's value meter (policy.value) — its label, its reading and a bar in its
	// band's colour; confident ranks first by their mean score, a rank with no confident record
	// dimmed and unranked; the session's token spend per rank at the row's end when it fits
	if rows := t.rankRows(v); len(rows) > 0 {
		open("ranks", "ranks", true, fmt.Sprint(len(rows)))
		if !v.Collapsed["ranks"] {
			for _, r := range t.rankLines(rows, inner) {
				line(r)
			}
		}
	}

	// ── subagents: the roster, live first, each state a chip on the right edge
	subagents := append([]RailSubagent(nil), v.Subagents...)
	sort.SliceStable(subagents, func(i, j int) bool { return !subagents[i].Done && subagents[j].Done })
	live := 0
	for _, c := range subagents {
		if !c.Done {
			live++
		}
	}
	count := fmt.Sprint(len(subagents))
	if live > 0 {
		count = fmt.Sprintf("%d · %d live", len(subagents), live)
	}
	open("subagents", "subagents", true, count)
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
			lane := c.Lane
			if lane == nil {
				lane = t.dim.GetForeground()
			}
			word := strings.ToUpper(c.State)
			if word == "" && c.Done {
				word = "DONE"
			}
			var chip string
			switch {
			case c.Done && c.Failed:
				chip = t.pillChip(t.bad.GetForeground(), "✗ "+word)
			case c.Done:
				chip = t.pillChip(t.ok.GetForeground(), "✓ "+word)
			default:
				w := word
				if c.Aged != "" && inner-ansi.StringWidth(c.Name)-ansi.StringWidth(w) > 16 {
					w += " " + c.Aged
				}
				chip = t.pillChip(lane, w)
			}
			// the agent's value meter beside its state, where the row has room: its reading in its
			// band's colour, dimmed when the record is too short to rank
			if c.Value != nil && c.Value.Meter.Label != "" && inner-ansi.StringWidth(c.Name)-ansi.StringWidth(chip) >= 12 {
				chip = t.valueChip(*c.Value) + " " + chip
			}
			dot := lipgloss.NewStyle().Foreground(lane).Render("●")
			name := t.text.Render
			if c.Done {
				dot, name = t.dim.Render("●"), t.label.Render
			}
			nameW := max(inner-2-ansi.StringWidth(chip)-1, 4)
			row := dot + " " + name(ansi.Truncate(c.Name, nameW, "…"))
			row += strings.Repeat(" ", max(inner-ansi.StringWidth(row)-ansi.StringWidth(chip), 1)) + chip
			cur.rows = append(cur.rows, railRow{text: row, subagent: c.Name})
		}
	}

	// ── the plan: its progress, the current step lit, the deck under it
	if v.Plan != nil {
		hue := t.plan
		open("plan", "plan ▸ "+v.Plan.Agent, true, fmt.Sprint(len(v.Plan.Items)))
		if !v.Collapsed["plan"] {
			switch {
			case v.Plan.Archived:
				line(t.dim.Render("archived · alt+n for the current deck"))
			case v.Plan.Stale:
				line(t.dim.Render("stale · alt+n for the current deck"))
			}
			if len(v.Plan.Items) == 0 {
				line(t.dim.Render("· deck empty"))
			} else {
				done := 0
				for _, it := range v.Plan.Items {
					if it.Status == "done" {
						done++
					}
				}
				frac := fmt.Sprintf(" %d/%d", done, len(v.Plan.Items))
				cells := min(inner-ansi.StringWidth(frac), 24)
				// the bar in the plan's own violet; the count in the plan meter's band (config's
				// ui.meters.plan) once a slice is done — a deck with nothing done has no basis yet
				pct := 100 * float64(done) / float64(len(v.Plan.Items))
				fracS := t.label.Render(frac)
				if done > 0 {
					fracS = lipgloss.NewStyle().Foreground(t.bandHue(value.Render(meterDef(v.Meters, "plan"), pct, true).Band)).Render(frac)
				}
				line(t.gauge(pct/100, cells, hue.GetForeground(), nil) + fracS)
			}
			for _, it := range v.Plan.Items {
				var mark string
				text := t.text
				switch it.Status {
				case "in_progress":
					mark, text = hue.Bold(true).Render("▸"), hue.Bold(true)
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

	// ── files: the touch ledger as a tree, latest last
	open("files", "files", true, fmt.Sprint(len(v.Files)))
	if !v.Collapsed["files"] {
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
				line(ansi.Truncate(l, inner, "…"))
			}
		}
		if hidden > 0 {
			line(t.dim.Render(fmt.Sprintf("… +%d more", hidden)))
		}
	}

	// assemble: flat, a rule per section between the rows; under the depth, a raised tile each
	var out []railRow
	if !t.Depth {
		for _, sc := range secs {
			if sc.head != "" {
				out = append(out, railRow{text: t.rule(rw, sc.head, ""), sep: true, key: sc.key})
			}
			out = append(out, sc.rows...)
		}
		return out
	}
	for i, sc := range secs {
		hue := sc.hue
		if i == 0 {
			hue = t.accent.GetForeground()
		}
		ts, o := t.surfTile(hue), slabOpt{band: 1}
		var texts []string
		var meta []railRow
		if sc.head != "" {
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

// railState is the header tile's state chip: idle quiet, a running turn in the primary, a
// pending ask in the human's accent.
func (t Theme) railState(v RailView) string {
	switch v.State {
	case "wait":
		return t.pillChip(t.magenta.GetForeground(), "◆ waiting for you")
	case "busy":
		verb := v.Verb
		if verb == "" {
			verb = "working"
		}
		return t.pillChip(t.accent.GetForeground(), "● "+verb)
	}
	return t.dim.Render("○ idle")
}

// rankHue is a rank's colour: its own lane, else its family's (a custom rank), else the quiet
// tone.
func (t Theme) rankHue(rank, family string) color.Color {
	if c := t.lane(rank); c != nil {
		return c
	}
	if l, ok := theme.FamilyLane[family]; ok {
		return t.lane(l)
	}
	return t.dim.GetForeground()
}

// rankRow is one row of the ranks tile: a rank's standing and its session spend.
type rankRow struct {
	rank, family string
	meter        *value.Meter
	low          bool
	mean         float64
	tokens       int
}

// rankRows merges the value policy's ranks with the session's spend per rank: every rank with a
// agent or a token, confident ranks first by their mean score, then the rest in spend order.
func (t Theme) rankRows(v RailView) []rankRow {
	var out []rankRow
	at := map[string]int{}
	for _, l := range v.Layers {
		at[l.Rank] = len(out)
		out = append(out, rankRow{rank: l.Rank, tokens: l.Tokens, low: true})
	}
	for _, r := range v.RankNames {
		if r.Agents == 0 {
			if _, ok := at[r.Rank]; !ok {
				continue
			}
		}
		i, ok := at[r.Rank]
		if !ok {
			at[r.Rank] = len(out)
			i = len(out)
			out = append(out, rankRow{rank: r.Rank})
		}
		m := r.Meter
		out[i].family, out[i].meter, out[i].low, out[i].mean = r.Family, &m, r.Confident == 0, r.Mean
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].low != out[j].low {
			return !out[i].low
		}
		return !out[i].low && out[i].mean > out[j].mean
	})
	return out
}

// rankLines draws the ranks tile's rows inner cells wide: the rank's dot and name in its hue,
// its meter's label, a bar in its band and the reading; a rank with no confident record dimmed
// (its reading "low"), one with no record at all a dash; its tokens at the end on a wide column.
func (t Theme) rankLines(rows []rankRow, inner int) []string {
	valued := false
	for _, r := range rows {
		valued = valued || r.meter != nil
	}
	if !valued {
		return t.spendLines(rows, inner)
	}
	nw := 0
	for _, r := range rows {
		nw = max(nw, ansi.StringWidth(r.rank))
	}
	nw = min(nw, max(inner/4+1, 6)) // "orchestrator" whole in a 48-cell sidebar
	labW := 0
	if inner >= 34 {
		labW = min(12, inner/4)
	}
	tokW := 0
	if inner >= 46 {
		tokW = 5
	}
	var out []string
	for _, r := range rows {
		hue := t.rankHue(r.rank, r.family)
		name := ansi.Truncate(r.rank, nw, "…")
		nameS := t.text.Render(name)
		if r.low {
			nameS = t.label.Render(name)
		}
		row := lipgloss.NewStyle().Foreground(hue).Render("●") + " " + nameS + strings.Repeat(" ", nw-ansi.StringWidth(name)) + " "
		reading, label := "—", ""
		frac, fill := 0.0, t.dim.GetForeground()
		readS := t.dim.Render
		if r.meter != nil {
			label = strings.ToLower(r.meter.Label)
			if r.low {
				reading = "low"
			} else {
				reading, frac = r.meter.Text, r.meter.Pct/100
				fill = t.bandHue(r.meter.Band)
				readS = lipgloss.NewStyle().Foreground(fill).Bold(true).Render
			}
		}
		if labW > 0 {
			l := ansi.Truncate(label, labW, "…")
			row += t.dim.Render(l) + strings.Repeat(" ", labW-ansi.StringWidth(l)) + " "
		}
		cells := inner - ansi.StringWidth(row) - 5 - tokW
		if tokW > 0 {
			cells--
		}
		if cells >= 3 {
			row += t.gauge(frac, cells, fill, nil) + " "
		}
		row += strings.Repeat(" ", max(4-ansi.StringWidth(reading), 0)) + readS(reading)
		if tokW > 0 {
			tok := ""
			if r.tokens > 0 {
				tok = humanTokens(r.tokens)
			}
			row += " " + strings.Repeat(" ", max(tokW-ansi.StringWidth(tok), 0)) + t.label.Render(tok)
		}
		out = append(out, ansi.Truncate(row, inner, "…"))
	}
	return out
}

// spendLines is the ranks tile without a value record (the policy off, a young workspace): each rank's
// share of the session's tokens, a bar in its lane.
func (t Theme) spendLines(rows []rankRow, inner int) []string {
	total, nw := 0, 0
	for _, r := range rows {
		total += r.tokens
		nw = max(nw, ansi.StringWidth(r.rank))
	}
	nw = min(nw, max(inner/3, 6))
	var out []string
	for _, r := range rows {
		hue := t.rankHue(r.rank, r.family)
		name := ansi.Truncate(r.rank, nw, "…")
		tok := humanTokens(r.tokens)
		cells := inner - 2 - nw - 1 - 1 - 5
		share := 0.0
		if total > 0 {
			share = float64(r.tokens) / float64(total)
		}
		row := lipgloss.NewStyle().Foreground(hue).Render("●") + " " + t.text.Render(name) + strings.Repeat(" ", nw-ansi.StringWidth(name)) + " "
		if cells >= 3 {
			row += t.gauge(share, cells, hue, nil) + " "
		}
		row += strings.Repeat(" ", max(5-ansi.StringWidth(tok), 0)) + t.label.Render(tok)
		out = append(out, row)
	}
	return out
}

// valueBar is a value reading as a vitals row: a bar in its band, the reading, its label.
func (t Theme) valueBar(v RailValue, w int) string {
	m := v.Meter
	hue := t.bandHue(m.Band)
	text := lipgloss.NewStyle().Foreground(hue).Bold(true).Render(m.Text)
	if v.Low {
		hue, text = t.dim.GetForeground(), t.dim.Render("low")
	}
	label := t.dim.Render(" " + strings.ToLower(m.Label))
	cells := min(w-ansi.StringWidth(text)-ansi.StringWidth(label)-1, 12)
	if cells < 3 {
		return text + label
	}
	frac := m.Pct / 100
	if v.Low {
		frac = 0
	}
	return ansi.Truncate(t.gauge(frac, cells, hue, nil)+" "+text+label, w, "…")
}

// valueChip is an agent's reading as a small chip in its band's hue (dimmed when the record is
// too short to rank).
func (t Theme) valueChip(v RailValue) string {
	if v.Low {
		return t.dim.Render("·" + v.Meter.Text)
	}
	return lipgloss.NewStyle().Foreground(t.bandHue(v.Meter.Band)).Render(v.Meter.Text)
}
