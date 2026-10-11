package workspace

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ASL is the promotion spec language — a small deterministic condition a config declares on an
// promoted rank (`promotesWhen`). It is parsed once and evaluated against a metrics snapshot;
// evaluation never calls a tool, reads a file, or touches the network. Grammar:
//
// promote <rank> when <expr>
// expr       := or
// or         := and ("||" and)*
// and        := unary ("&&" unary)*
// unary      := "!" unary | "(" expr ")" | comparison
// comparison := metric op value
//
// metric := "reports" | "passes" | "fails" | "holes" | "passRate" | "failRate" | "holeRate"
// op     := ">=" | "<=" | ">" | "<" | "==" | "!="
// value  := number | number "%"
//
// A bare number is an absolute count (reports >= 25); a percent reads a rate (passRate >= 90%)
// against reports made. Reports of zero make every rate zero — nothing promotes on no record.
type ASL struct {
	Target string // the base rank promoted
	Cond   *Condition
}

// Metrics is one agent's snapshot: the reports it filed and how they landed.
type Metrics struct {
	Reports int // wire reports answered
	Passes  int // reports whose status was a pass
	Fails   int // reports whose status was a fail
	Holes   int // lines the report named as holes
	// Reverted is a held metric — a gate verdict undone on appeal. Not yet an ASL operand until
	// the harness reads reversals off the ledger; reserved, never read silently.
	Reverted int
}

// passRate and kin are the quotient of their count over reports, a whole percent [0,100]; a
// agent with no reports scores zero on every rate.
func (m Metrics) rate(n int) float64 {
	if m.Reports <= 0 {
		return 0
	}
	return 100 * float64(n) / float64(m.Reports)
}

// Intelligence is the agent's deterministic coded score, the fold the High promotion reads:
// the passRate discounted by the holeRate, so one number answers "did this agent think well
// while it routed and gated?" — the High promotion consumes this, never a prompted verdict.
// Reports of zero mean no basis; the score stays at zero until a record stands, and it floors
// at zero (never negative) when holes outnumber passes.
func (m Metrics) Intelligence() float64 {
	if m.Reports <= 0 {
		return 0
	}
	p := m.rate(m.Passes)
	h := m.rate(m.Holes)
	if p <= h {
		return 0
	}
	return p - h
}

// metric resolves an operand to its reading.
func (m Metrics) metric(name string) (float64, bool) {
	switch name {
	case "reports":
		return float64(m.Reports), true
	case "passes":
		return float64(m.Passes), true
	case "fails":
		return float64(m.Fails), true
	case "holes":
		return float64(m.Holes), true
	case "passRate":
		return m.rate(m.Passes), true
	case "failRate":
		return m.rate(m.Fails), true
	case "holeRate":
		return m.rate(m.Holes), true
	case "intelligence":
		return m.Intelligence(), true
	}
	return 0, false
}

// MetricNames is the ASL operand set, in the docs order.
func MetricNames() []string {
	return []string{"reports", "passes", "fails", "holes", "passRate", "failRate", "holeRate", "intelligence"}
}

// Condition is a boolean tree over metric comparisons.
type Condition struct {
	Metric string  // when set, a leaf: Metric Cmp Value
	Cmp    string  // one of the six ops
	Value  float64 // the threshold (a percent with `%`)
	And    []*Condition
	Or     []*Condition
	Not    *Condition
}

// lex splits on the boolean groupers and parens, keeping them as tokens.
func lexASL(s string) ([]string, error) {
	var toks []string
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '(' || c == ')':
			toks = append(toks, string(c))
			i++
		case c == '&' || c == '|':
			if i+1 < len(s) && s[i+1] == c {
				toks = append(toks, string([]byte{c, c}))
				i += 2
			} else {
				return nil, fmt.Errorf("a lone %c — the boolean is %c%c", c, c, c)
			}
		default:
			j := i
			for j < len(s) && !strings.ContainsRune(" \t()&|!", rune(s[j])) {
				j++
			}
			if j == i { // a lone ! is its own token (unary not); != splits below
				if strings.HasPrefix(s[i:], "!=") {
					toks = append(toks, "!=")
					i += 2
					continue
				}
				toks = append(toks, "!")
				i++
				continue
			}
			// comparators cut a glued word into metric, op, value: `reports>=3` lexes as
			// `reports >= 3`. Metrics and values never hold <>=, and a comparison is always
			// metric-left, so the split is safe.
			word := s[i:j]
			for k, r := range word {
				if strings.ContainsRune("<>=", r) {
					op, rest := word[k:k+1], word[k+1:]
					if strings.HasPrefix(word[k:], "<=") || strings.HasPrefix(word[k:], ">=") || strings.HasPrefix(word[k:], "==") {
						op, rest = word[k:k+2], word[k+2:]
					}
					if word[:k] != "" {
						toks = append(toks, word[:k])
					}
					toks = append(toks, op)
					if rest != "" {
						toks = append(toks, rest)
					}
					word = ""
					break
				}
			}
			if word != "" {
				toks = append(toks, word)
			}
			i = j
		}
	}
	return toks, nil
}

type aslParser struct {
	toks []string
	i    int
}

func (p *aslParser) peek() string {
	if p.i < len(p.toks) {
		return p.toks[p.i]
	}
	return ""
}
func (p *aslParser) next() string {
	t := p.peek()
	p.i++
	return t
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
	toks, err := lexASL(expr)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("an empty condition")
	}
	p := &aslParser{toks: toks}
	c, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.i != len(p.toks) {
		return nil, fmt.Errorf("unexpected %q", p.peek())
	}
	return c, nil
}

func (p *aslParser) parseOr() (*Condition, error) {
	first, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	var rest []*Condition
	for p.peek() == "||" {
		p.next()
		n, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		rest = append(rest, n)
	}
	if len(rest) == 0 {
		return first, nil
	}
	return &Condition{Or: append([]*Condition{first}, rest...)}, nil
}

func (p *aslParser) parseAnd() (*Condition, error) {
	first, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	var rest []*Condition
	for p.peek() == "&&" {
		p.next()
		n, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		rest = append(rest, n)
	}
	if len(rest) == 0 {
		return first, nil
	}
	return &Condition{And: append([]*Condition{first}, rest...)}, nil
}

func (p *aslParser) parseUnary() (*Condition, error) {
	switch t := p.peek(); {
	case t == "!":
		p.next()
		c, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &Condition{Not: c}, nil
	case t == "(":
		p.next()
		c, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.next() != ")" {
			return nil, fmt.Errorf("an unbalanced paren")
		}
		return c, nil
	default:
		return p.parseCmp()
	}
}

func (p *aslParser) parseCmp() (*Condition, error) {
	metric := p.next()
	if _, ok := (Metrics{}).metric(metric); !ok {
		return nil, fmt.Errorf("unknown metric %q (one of %s)", metric, strings.Join(MetricNames(), ", "))
	}
	op := p.next()
	switch op {
	case ">=", "<=", ">", "<", "==", "!=":
	default:
		return nil, fmt.Errorf("a comparison %q — one of >= <= > < == !=", op)
	}
	vtok := p.next()
	if vtok == "" {
		return nil, fmt.Errorf("no value after %s %s", metric, op)
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(vtok, "%"), 64)
	if err != nil {
		return nil, fmt.Errorf("value %q is not a number", vtok)
	}
	return &Condition{Metric: metric, Cmp: op, Value: v}, nil
}

// String renders the condition back in the spec's words. A rate metric's value is a percent —
// the token re-takes its % so the rendered condition parses back to the same tree.
func (c *Condition) String() string {
	switch {
	case c.Not != nil:
		return "!(" + c.Not.String() + ")"
	case len(c.And) > 0:
		var parts []string
		for _, p := range c.And {
			parts = append(parts, p.String())
		}
		return strings.Join(parts, " && ")
	case len(c.Or) > 0:
		var parts []string
		for _, p := range c.Or {
			parts = append(parts, p.String())
		}
		return strings.Join(parts, " || ")
	default:
		value := trimNum(c.Value)
		if strings.HasSuffix(c.Metric, "Rate") {
			value += "%"
		}
		return fmt.Sprintf("%s %s %s", c.Metric, c.Cmp, value)
	}
}

func trimNum(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// String is the parsed promotesWhen, half-written for a hole.
func (a ASL) String() string { return "promote " + a.Target + " when " + a.Cond.String() }

// cmp runs one comparison.
func cmp(cmp string, got, want float64) bool {
	switch cmp {
	case ">=":
		return got >= want
	case "<=":
		return got <= want
	case ">":
		return got > want
	case "<":
		return got < want
	case "==":
		return got == want
	case "!=":
		return got != want
	}
	return false
}

// Eval folds the condition against a snapshot. A leaf reads its metric through the snapshot so
// an unknown name cannot slip past the parse.
func (c *Condition) Eval(m Metrics) bool {
	switch {
	case c.Not != nil:
		return !c.Not.Eval(m)
	case len(c.And) > 0:
		for _, p := range c.And {
			if !p.Eval(m) {
				return false
			}
		}
		return true
	case len(c.Or) > 0:
		for _, p := range c.Or {
			if p.Eval(m) {
				return true
			}
		}
		return false
	default:
		v, ok := m.metric(c.Metric)
		return ok && cmp(c.Cmp, v, c.Value)
	}
}

// MetricsStore is the harness's per-agent report tally; empty on a cold start, filled as Subagents
// report in. Namespace: the workspace, one per open session's life (the Subagent that records the
// verdict folds it here).
type MetricsStore struct {
	byAgent map[string]Metrics
}

// NewMetricsStore opens an empty tally.
func NewMetricsStore() *MetricsStore { return &MetricsStore{byAgent: map[string]Metrics{}} }

// Fold counts one report under its agent. `status` is the report's @S word; `holes` the count of
// lines it named as holes.
func (s *MetricsStore) Fold(agent, status string, holes int) {
	if s == nil {
		return
	}
	m := s.byAgent[agent]
	m.Reports++
	switch strings.ToUpper(strings.Fields(status + " ")[0]) {
	case "PASS", "DONE":
		m.Passes++
	case "FAIL":
		m.Fails++
	}
	m.Holes += holes
	s.byAgent[agent] = m
}

// Metrics reads one agent's snapshot (zero-value for an agent with no record).
func (s *MetricsStore) Metrics(agent string) Metrics {
	if s == nil {
		return Metrics{}
	}
	return s.byAgent[agent]
}

// Agents lists the agents with a record (stable order for the ledger).
func (s *MetricsStore) Agents() []string {
	var out []string
	for b := range s.byAgent {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}
