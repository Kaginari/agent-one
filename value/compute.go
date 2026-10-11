package value

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Def is the one definition of success: which run words pass, which fail, which are neutral.
// The ASL fold, the council, the leaderboard and the guts' end-of-run all read it. A word on no
// list is neutral — an unknown word is never the agent's failure.
type Def struct {
	Pass    []string
	Fail    []string
	Neutral []string
}

// DefaultDef is policy.value's shipped definition.
func DefaultDef() Def {
	return Def{
		Pass:    []string{"DONE", "PASS"},
		Fail:    []string{"FAIL", "ESCALATE", "CHECKPOINT"},
		Neutral: []string{"DENIED", "INTERRUPTED", "DRY", "SUSPENDED"},
	}
}

// Outcomes.
const (
	Pass    = "pass"
	Fail    = "fail"
	Neutral = "neutral"
)

// Word is the first token of a status line, upper-cased ("@S FAIL — tests red" → FAIL).
func Word(status string) string {
	f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(status), "@S"))
	if len(f) == 0 {
		return ""
	}
	return strings.ToUpper(strings.Trim(f[0], ":,.;—-"))
}

// Outcome classifies one word.
func (d Def) Outcome(word string) string {
	w := Word(word)
	for _, x := range d.Fail {
		if strings.EqualFold(x, w) {
			return Fail
		}
	}
	for _, x := range d.Pass {
		if strings.EqualFold(x, w) {
			return Pass
		}
	}
	return Neutral
}

// RunWord is a run's one word: the loop's status when the loop did not end DONE (the clock, the
// human or the gate spoke), else the agent's own @S word, else the loop's.
func RunWord(loopStatus, agentWord string) string {
	l := Word(loopStatus)
	if l != "" && l != "DONE" {
		return l
	}
	if b := Word(agentWord); b != "" {
		return b
	}
	return l
}

// Landed reports whether a run landed: the loop ended DONE, its word is not a fail, and no gate
// failed it.
func (d Def) Landed(r Row) bool {
	return Word(r.Loop) == "DONE" && d.Outcome(r.Status) != Fail && r.Gate != "fail"
}

// ParseWindow reads a window ("28d", "7d", "24h", a Go duration); "", "all" and "0" mean the
// whole ledger (zero).
func ParseWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "all" || s == "0" {
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("window %q: a whole number of days (28d)", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("window %q: 28d, 7d, 24h or all", s)
	}
	return d, nil
}

// Opts tunes a fold.
type Opts struct {
	Def         Def
	Window      time.Duration // 0 = the whole ledger
	Now         time.Time
	EscalateAt  int // policy.stress.escalateAt: the strain that reads 100%
	ReopenTurns int // orchestrator turns within which a rewrite of the same paths reopens a pass
}

// Signals is one agent's readings.
type Signals map[string]float64

// Fold reads a ledger into every agent's signals over the window. A rate on no basis reads zero:
// nothing scores on no record.
func Fold(rows []Row, o Opts) map[string]Signals {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if len(o.Def.Pass)+len(o.Def.Fail) == 0 {
		o.Def = DefaultDef()
	}
	in := func(r Row) bool { return o.Window <= 0 || !r.At.Before(o.Now.Add(-o.Window)) }
	type acc struct {
		runs, reports, passes, fails, neutral, holes, holed, landed int
		gated, gateFirst, verifyRun, verifyRed                      int
		verdicts, overturned                                        int
		findingsReports, upheld, findings                           int
		thrashRuns, gutsRuns, momentum, tokens                      int
		strain                                                      int
		strainAt                                                    time.Time
		dispatched, dispatchLanded, reasks                          int
		children                                                    map[string]bool
		turns, turnsDone                                            int
	}
	accs := map[string]*acc{}
	get := func(b string) *acc {
		a := accs[b]
		if a == nil {
			a = &acc{children: map[string]bool{}}
			accs[b] = a
		}
		return a
	}
	// a rebirth starts an agent's record again: its own rows before the latest rebirth row are not
	// read (a dispatcher's counts of it still are — they are the dispatcher's record)
	reborn := map[string]time.Time{}
	for _, r := range rows {
		if r.Kind == KindRebirth && r.At.After(reborn[r.Agent]) {
			reborn[r.Agent] = r.At
		}
	}
	own := func(r Row) bool { return in(r) && !r.At.Before(reborn[r.Agent]) }
	// the latest strain per agent (any time: strain is a standing reading, not a windowed count)
	lastWarrant := map[string]bool{} // dispatcher\x00child → the child's previous run under it carried a warrant
	var turns []Row
	for _, r := range rows {
		switch r.Kind {
		case KindRun:
			a := get(r.Agent)
			if !r.At.Before(a.strainAt) && !r.At.Before(reborn[r.Agent]) {
				a.strain, a.strainAt = r.Strain, r.At
			}
			if r.By != "" {
				key := r.By + "\x00" + r.Agent
				if in(r) {
					d := get(r.By)
					d.dispatched++
					d.children[r.Agent] = true
					if o.Def.Landed(r) {
						d.dispatchLanded++
					}
					if lastWarrant[key] {
						d.reasks++
					}
				}
				lastWarrant[key] = r.Warrant
			}
			if r.Turn {
				turns = append(turns, r)
			}
			if !own(r) {
				continue
			}
			a.runs++
			a.tokens += r.Tokens
			if r.Guts {
				a.gutsRuns++
				a.momentum += r.Momentum
			}
			if r.Thrash > 0 {
				a.thrashRuns++
			}
			a.findings += r.Findings
			if r.Gate == "pass" || r.Gate == "fail" {
				a.gated++
				if r.Gate == "pass" && r.GateTries == 0 {
					a.gateFirst++
				}
			}
			a.verifyRun += r.VerifyRun
			a.verifyRed += r.VerifyRed
			switch o.Def.Outcome(r.Status) {
			case Pass:
				a.passes++
			case Fail:
				a.fails++
			default:
				a.neutral++
				continue // a neutral run is never judged: no rate reads it
			}
			a.reports++
			a.holes += r.Holes.Harness
			if r.Holes.Harness > 0 {
				a.holed++
			}
			landed := o.Def.Landed(r)
			if landed {
				a.landed++
			}
			if r.Findings > 0 {
				a.findingsReports++
				if landed {
					a.upheld++
				}
			}
		case KindVerdict:
			if own(r) {
				get(r.Agent).verdicts++
			}
		case KindOverturn:
			if own(r) {
				get(r.Agent).overturned++
			}
		}
	}
	// the orchestrator's turns: a pass not reopened within reopenTurns turns (a later turn rewrote one of
	// its paths) is an ask done
	reopen := o.ReopenTurns
	if reopen <= 0 {
		reopen = 3
	}
	for i, t := range turns {
		if !own(t) {
			continue
		}
		a := get(t.Agent)
		if o.Def.Outcome(t.Status) == Neutral {
			continue
		}
		a.turns++
		if o.Def.Outcome(t.Status) != Pass || Word(t.Word) == "FAIL" {
			continue
		}
		reopened := false
		for j := i + 1; j < len(turns) && j <= i+reopen && !reopened; j++ {
			if turns[j].Agent == t.Agent && overlap(t.Paths, turns[j].Paths) {
				reopened = true
			}
		}
		if !reopened {
			a.turnsDone++
		}
	}
	pct := func(n, d int) float64 {
		if d <= 0 {
			return 0
		}
		v := 100 * float64(n) / float64(d)
		if v > 100 {
			v = 100
		}
		return v
	}
	strainPct := func(s int) float64 {
		at := o.EscalateAt
		if at <= 0 {
			at = 5
		}
		return pct(s, at)
	}
	out := map[string]Signals{}
	for b, a := range accs {
		s := Signals{
			"runs": float64(a.runs), "reports": float64(a.reports), "passes": float64(a.passes),
			"fails": float64(a.fails), "neutral": float64(a.neutral), "holes": float64(a.holes),
			"passRate": pct(a.passes, a.reports), "failRate": pct(a.fails, a.reports),
			"holeRate": pct(a.holed, a.reports), "landRate": pct(a.landed, a.reports),
			"gateRate": pct(a.gateFirst, a.gated), "verifyGreenRate": pct(a.verifyRun-a.verifyRed, a.verifyRun),
			"verdicts": float64(a.verdicts), "overturned": float64(a.overturned),
			"planLandRate": pct(a.dispatchLanded, a.dispatched), "reaskRate": pct(a.reasks, a.dispatched),
			"askDoneRate": pct(a.turnsDone, a.turns), "upheldRate": pct(a.upheld, a.findingsReports),
			"findings": float64(a.findings), "thrashRate": pct(a.thrashRuns, a.runs),
			"strain": float64(a.strain), "strainPct": strainPct(a.strain), "tokens": float64(a.tokens),
		}
		if a.verdicts > 0 {
			s["gateRight"] = pct(a.verdicts-a.overturned, a.verdicts)
		} else {
			s["gateRight"] = 0
		}
		if a.gutsRuns > 0 {
			s["momentum"] = float64(a.momentum) / float64(a.gutsRuns)
		} else {
			s["momentum"] = 0
		}
		if p, h := s["passRate"], s["holeRate"]; p > h {
			s["intelligence"] = p - h
		} else {
			s["intelligence"] = 0
		}
		out[b] = s
	}
	// zoneHealth: 100 less the mean strain of the agents it dispatched in the window
	for b, a := range accs {
		if len(a.children) == 0 {
			out[b]["zoneHealth"] = 0
			continue
		}
		sum := 0.0
		for c := range a.children {
			sum += out[c]["strainPct"]
		}
		out[b]["zoneHealth"] = 100 - sum/float64(len(a.children))
	}
	return out
}

func overlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		if set[x] {
			return true
		}
	}
	return false
}

// Agents lists the agents a fold read, sorted.
func Agents(m map[string]Signals) []string {
	out := make([]string, 0, len(m))
	for b := range m {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

// Clamp holds a score in 0..100.
func Clamp(v float64) float64 {
	switch {
	case v != v: // NaN
		return 0
	case v < 0:
		return 0
	case v > 100:
		return 100
	}
	return v
}
