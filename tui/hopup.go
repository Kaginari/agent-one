package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// hopVerdict is the verdict a landed Subagent carries one hop up loudly — ESCALATE, FAIL or
// DENIED, read off its exit (else its report's first @S word); "" for every other landing. A
// card in red among a column of cards is easy to scroll past: these three ask the one above
// to act, so they get their own line.
func hopVerdict(c SubagentView) string {
	v := c.Exit
	if v == "" && c.Report != nil {
		v = c.Report.Status
	}
	if f := strings.Fields(v); len(f) > 0 {
		v = strings.ToUpper(strings.TrimRight(f[0], ":—-·"))
	}
	switch v {
	case "ESCALATE", "FAIL", "DENIED":
		return v
	}
	return ""
}

// hopReason is the why the line carries: the report's first hole, else what the status says
// after its verdict word.
func hopReason(c SubagentView) string {
	if c.Report == nil {
		return ""
	}
	if len(c.Report.Holes) > 0 {
		return oneLine(c.Report.Holes[0])
	}
	if f := strings.Fields(c.Report.Status); len(f) > 1 {
		return strings.TrimLeft(strings.Join(f[1:], " "), "—-:· ")
	}
	return ""
}

// HopUp is the loud line under a Subagent that landed ESCALATE, FAIL or DENIED: the verdict on
// a filled chip (the warn hue for an escalation, the bad one for the other two), who sent it
// and to whom one hop up (Up, the agent that dispatched it, when known), and the why — one
// row, fitted to width.
func (t Theme) HopUp(c SubagentView, verdict string, width int) string {
	glyph, hue := "▲", t.warn.GetForeground()
	switch verdict {
	case "FAIL":
		glyph, hue = "✗", t.bad.GetForeground()
	case "DENIED":
		glyph, hue = "⊘", t.bad.GetForeground()
	}
	chip := lipgloss.NewStyle().Background(hue).Foreground(t.onAccent()).Bold(true).Render(" " + glyph + " " + verdict + " ")
	who := lipgloss.NewStyle().Foreground(hue).Bold(true).Render(c.Name)
	if c.Up != "" {
		who += t.dim.Render(" → ") + t.text.Bold(true).Render(c.Up)
	}
	line := chip + " " + who
	if why := hopReason(c); why != "" {
		room := width - ansi.StringWidth(line) - 3
		if room > 8 {
			line += t.dim.Render(" · ") + t.text.Render(ansi.Truncate(why, room, "…"))
		}
	}
	return ansi.Truncate(line, width, "…")
}
