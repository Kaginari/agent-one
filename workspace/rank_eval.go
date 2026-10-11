package workspace

import (
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/value"
)

// ASL is the promotion spec language — a small deterministic condition a config declares on an
// promoted rank (`promotesWhen`). It is parsed once and evaluated against an agent's signals, the
// fold of the value ledger over the window (value.Fold); evaluation never calls a tool, reads a
// file, or touches the network. Grammar:
//
//	promote <rank> when <condition>
//
// The condition is a value expression (value.Expr): comparisons of arithmetic over signals and
// numbers, joined by && || and !, grouped by parens —
//
//	reports >= 20 && passRate >= 90% && holeRate <= 10%
//	score >= 75 && reports >= 25
//	0.5*gateRight + 0.5*landRate >= 85
//
// A signal is any of value.SignalNames — reports, passes, fails, holes, passRate, failRate,
// holeRate, landRate, intelligence (passRate less holeRate, floored at zero), the rank's own
// (gateRate, gateRight, zoneHealth, planLandRate, …) and score, the rank's value score. A bare
// number is an absolute count; a percent reads a rate (rates are whole percents, 0..100). Reports
// of zero make every rate zero — nothing promotes on no record.
type ASL struct {
	Target string // the base rank promoted
	Cond   *Condition
}

// Metrics is one agent's signals: the value ledger folded over the window, its score included.
type Metrics = value.Signals

// Tally builds the signals of a bare report record — reports, passes, fails and harness holes —
// with their rates, as value.Fold would read them. Tests and the promotion doc's reading use it.
func Tally(reports, passes, fails, holes int) Metrics {
	m := Metrics{"reports": float64(reports), "passes": float64(passes), "fails": float64(fails), "holes": float64(holes)}
	rate := func(n int) float64 {
		if reports <= 0 {
			return 0
		}
		v := 100 * float64(n) / float64(reports)
		if v > 100 {
			v = 100
		}
		return v
	}
	m["passRate"], m["failRate"], m["holeRate"] = rate(passes), rate(fails), rate(holes)
	if m["passRate"] > m["holeRate"] {
		m["intelligence"] = m["passRate"] - m["holeRate"]
	} else {
		m["intelligence"] = 0
	}
	return m
}

// MetricNames is the ASL operand set, in the docs order.
func MetricNames() []string { return value.SignalNames() }

// Condition is a parsed promotesWhen condition.
type Condition struct{ expr *value.Expr }

// Eval folds the condition against an agent's signals.
func (c *Condition) Eval(m Metrics) bool { return c != nil && c.expr.True(m) }

// String renders the condition back in the spec's words.
func (c *Condition) String() string {
	if c == nil {
		return ""
	}
	return c.expr.String()
}

// Leaves reads each comparison against an agent's signals: the promotion progress a surface shows.
func (c *Condition) Leaves(m Metrics) []value.Leaf {
	if c == nil {
		return nil
	}
	return c.expr.Leaves(m)
}

// Signals lists the signals the condition reads.
func (c *Condition) Signals() []string {
	if c == nil {
		return nil
	}
	return c.expr.Signals()
}

// ParseASL reads one promotesWhen string into a condition; the string must open with the
// promotion subject so the config's intent is pinned to the rank it decorates.
func ParseASL(text string) (ASL, error) {
	fields := strings.Fields(text)
	if len(fields) < 3 || fields[0] != "promote" {
		return ASL{}, fmt.Errorf("opens with `promote <rank> when …`")
	}
	target := fields[1]
	if fields[2] != "when" {
		return ASL{}, fmt.Errorf("missing `when` after the promoted rank")
	}
	cond, err := ParseCondition(strings.Join(fields[3:], " "))
	if err != nil {
		return ASL{}, err
	}
	return ASL{Target: target, Cond: cond}, nil
}

// ParseCondition parses the boolean expression half (after `when`).
func ParseCondition(expr string) (*Condition, error) {
	e, err := value.ParseCondition(expr, value.IsSignal)
	if err != nil {
		if strings.TrimSpace(expr) == "" {
			return nil, fmt.Errorf("an empty condition")
		}
		return nil, err
	}
	return &Condition{expr: e}, nil
}

// String is the parsed promotesWhen, half-written for a hole.
func (a ASL) String() string { return "promote " + a.Target + " when " + a.Cond.String() }
