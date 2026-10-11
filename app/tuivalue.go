package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Kaginari/agent-one/tui"
	"github.com/Kaginari/agent-one/workspace"
)

// footerValue fills the terminal's value readings from the value policy (policy.value), folded at most
// once a second (valueView): the session's meters as config resolves them, each rank's standing,
// each agent's meter for its subagent row, the orchestrator's own score, and the nearest promotion.
func (a *App) footerValue(fv *tui.FooterView) {
	cfg := a.liveConfig()
	fv.Meters, _ = cfg.Meters()
	if a.Workspace == nil {
		return
	}
	v := a.valueView()
	if v == nil || !v.Lit {
		return
	}
	rep, ok := v.Report.(workspace.ValueReport)
	if !ok {
		return
	}
	for _, r := range rep.Ranks {
		fv.RankNames = append(fv.RankNames, tui.RailRank{Rank: r.Rank, Family: r.Family, Meter: r.Reading, Agents: r.Agents, Confident: r.Confident, Mean: r.Mean})
	}
	fv.Values = map[string]tui.RailValue{}
	for _, b := range rep.Agents {
		rv := tui.RailValue{Meter: b.Meter, Low: b.Confidence != "ok"}
		fv.Values[b.Agent] = rv
		if b.Agent == workspace.Orchestrator {
			s := rv
			fv.Score = &s
		}
	}
	fv.Promote = nearestPromotion(rep.Agents)
}

// nearestPromotion is the agent closest to its promotion, said in a line: one condition left
// ("domain-engine · reports 17/20 to promote"), or how many are left and the nearest of them
// ("domain-engine · 2 to promote · passRate 84/90"); "" when no agent is on its way.
func nearestPromotion(agents []workspace.AgentValue) string {
	best, bestN, bestD := "", 0, 0.0
	var bestLeaf string
	for _, b := range agents {
		if b.Promotion == nil || b.Promotion.Met {
			continue
		}
		n, near, d := 0, "", -1.0
		for _, l := range b.Promotion.Leaves {
			if l.Met {
				continue
			}
			n++
			if d < 0 || l.Distance < d {
				d = l.Distance
				near = leafName(l.Text) + " " + leafNum(l.Got) + "/" + leafNum(l.Want)
			}
		}
		if n == 0 {
			continue
		}
		if best == "" || n < bestN || n == bestN && d < bestD {
			best, bestN, bestD, bestLeaf = b.Agent, n, d, near
		}
	}
	switch {
	case best == "":
		return ""
	case bestN == 1:
		return best + " · " + bestLeaf + " to promote"
	}
	return fmt.Sprintf("%s · %d to promote · %s", best, bestN, bestLeaf)
}

// leafName is a condition's left side ("reports" of "reports >= 20").
func leafName(text string) string {
	f := strings.Fields(text)
	if len(f) == 0 {
		return text
	}
	return f[0]
}

func leafNum(f float64) string { return strconv.FormatFloat(math.Round(f*10)/10, 'f', -1, 64) }
