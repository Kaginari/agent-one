// Package value is the value policy, coded: one definition of a landed run, one append-only
// ledger of every run every agent made, the signals a window of that ledger folds to, the small
// arithmetic a rank's score is written in, and the meter a surface draws. It reads no prompt and
// calls no model — every number is a fold over rows the harness wrote.
package value

import (
	"fmt"
	"strconv"
	"strings"
)

// Expr is one parsed value expression: arithmetic over signals and numbers (+ - * /, unary -,
// parens), comparisons (>= <= > < == !=) and booleans (&& || !). A score is a numeric Expr; an
// promotesWhen condition is a boolean one. Evaluation reads a signal map and nothing else.
//
//	or      := and ("||" and)*
//	and     := not ("&&" not)*
//	not     := "!" not | cmp
//	cmp     := sum (op sum)?
//	sum     := prod (("+"|"-") prod)*
//	prod    := unary (("*"|"/") unary)*
//	unary   := "-" unary | primary
//	primary := number ["%"] | signal | "(" or ")"
//
// A percent literal reads as its number (rates are whole percents, 0..100); a division by zero
// reads zero, never a NaN.
type Expr struct {
	root *node
	Src  string
}

type node struct {
	op       string // "num" "sig" "neg" "+" "-" "*" "/" "cmp" "&&" "||" "!"
	num      float64
	pct      bool
	sig      string
	cmp      string
	kids     []*node
	boolType bool
}

// Parse reads an expression. known reports whether an identifier names a signal (nil accepts any).
func Parse(src string, known func(string) bool) (*Expr, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("an empty expression")
	}
	p := &parser{toks: toks, known: known}
	n, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.i != len(p.toks) {
		return nil, fmt.Errorf("unexpected %q", p.peek())
	}
	return &Expr{root: n, Src: strings.TrimSpace(src)}, nil
}

// ParseNumber reads a numeric expression (a score); a comparison at the top is refused.
func ParseNumber(src string, known func(string) bool) (*Expr, error) {
	e, err := Parse(src, known)
	if err != nil {
		return nil, err
	}
	if e.root.boolType {
		return nil, fmt.Errorf("%q is a condition, not a number", src)
	}
	return e, nil
}

// ParseCondition reads a boolean expression (an promotesWhen); a bare number is refused.
func ParseCondition(src string, known func(string) bool) (*Expr, error) {
	e, err := Parse(src, known)
	if err != nil {
		return nil, err
	}
	if !e.root.boolType {
		return nil, fmt.Errorf("%q is a number, not a condition (compare it: %s >= …)", src, strings.TrimSpace(src))
	}
	return e, nil
}

// IsCondition reports whether the expression is boolean.
func (e *Expr) IsCondition() bool { return e != nil && e.root.boolType }

func lex(s string) ([]string, error) {
	var toks []string
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case strings.ContainsRune("()+-*/%", rune(c)):
			toks = append(toks, string(c))
			i++
		case c == '&' || c == '|':
			if i+1 < len(s) && s[i+1] == c {
				toks = append(toks, string([]byte{c, c}))
				i += 2
				continue
			}
			return nil, fmt.Errorf("a lone %c — the boolean is %c%c", c, c, c)
		case c == '<' || c == '>' || c == '=' || c == '!':
			if i+1 < len(s) && s[i+1] == '=' {
				toks = append(toks, s[i:i+2])
				i += 2
				continue
			}
			if c == '=' {
				return nil, fmt.Errorf("a lone = — equality is ==")
			}
			toks = append(toks, string(c))
			i++
		case c >= '0' && c <= '9' || c == '.':
			j := i
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i
			for j < len(s) && (s[j] == '_' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9') {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		default:
			return nil, fmt.Errorf("unexpected %q", string(c))
		}
	}
	return toks, nil
}

type parser struct {
	toks  []string
	i     int
	known func(string) bool
}

func (p *parser) peek() string {
	if p.i < len(p.toks) {
		return p.toks[p.i]
	}
	return ""
}

func (p *parser) next() string { t := p.peek(); p.i++; return t }

func (p *parser) or() (*node, error) {
	return p.chain([]string{"||"}, p.and, true)
}

func (p *parser) and() (*node, error) {
	return p.chain([]string{"&&"}, p.not, true)
}

// chain folds a left-associative run of one precedence level. Booleans join booleans; arithmetic
// joins numbers.
func (p *parser) chain(ops []string, sub func() (*node, error), boolean bool) (*node, error) {
	left, err := sub()
	if err != nil {
		return nil, err
	}
	for contains(ops, p.peek()) {
		op := p.next()
		right, err := sub()
		if err != nil {
			return nil, err
		}
		if boolean && (!left.boolType || !right.boolType) {
			return nil, fmt.Errorf("%s joins conditions — compare a number first (signal >= value)", op)
		}
		if !boolean && (left.boolType || right.boolType) {
			return nil, fmt.Errorf("%s is arithmetic — a condition is not a number", op)
		}
		left = &node{op: op, kids: []*node{left, right}, boolType: boolean}
	}
	return left, nil
}

func (p *parser) not() (*node, error) {
	if p.peek() == "!" {
		p.next()
		n, err := p.not()
		if err != nil {
			return nil, err
		}
		if !n.boolType {
			return nil, fmt.Errorf("! negates a condition, not a number")
		}
		return &node{op: "!", kids: []*node{n}, boolType: true}, nil
	}
	return p.cmp()
}

var cmpOps = []string{">=", "<=", ">", "<", "==", "!="}

func (p *parser) cmp() (*node, error) {
	left, err := p.sum()
	if err != nil {
		return nil, err
	}
	if !contains(cmpOps, p.peek()) {
		return left, nil
	}
	op := p.next()
	if p.peek() == "" {
		return nil, fmt.Errorf("no value after %s", op)
	}
	right, err := p.sum()
	if err != nil {
		return nil, err
	}
	if left.boolType || right.boolType {
		return nil, fmt.Errorf("%s compares numbers", op)
	}
	return &node{op: "cmp", cmp: op, kids: []*node{left, right}, boolType: true}, nil
}

func (p *parser) sum() (*node, error) { return p.chain([]string{"+", "-"}, p.prod, false) }

func (p *parser) prod() (*node, error) { return p.chain([]string{"*", "/"}, p.unary, false) }

func (p *parser) unary() (*node, error) {
	if p.peek() == "-" {
		p.next()
		n, err := p.unary()
		if err != nil {
			return nil, err
		}
		if n.boolType {
			return nil, fmt.Errorf("- negates a number, not a condition")
		}
		return &node{op: "neg", kids: []*node{n}}, nil
	}
	return p.primary()
}

func (p *parser) primary() (*node, error) {
	t := p.next()
	switch {
	case t == "":
		return nil, fmt.Errorf("the expression ends early")
	case t == "(":
		n, err := p.or()
		if err != nil {
			return nil, err
		}
		if p.next() != ")" {
			return nil, fmt.Errorf("an unbalanced paren")
		}
		return n, nil
	case t[0] >= '0' && t[0] <= '9' || t[0] == '.':
		v, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil, fmt.Errorf("value %q is not a number", t)
		}
		n := &node{op: "num", num: v}
		if p.peek() == "%" {
			p.next()
			n.pct = true
		}
		return n, nil
	case t[0] == '_' || t[0] >= 'a' && t[0] <= 'z' || t[0] >= 'A' && t[0] <= 'Z':
		if p.known != nil && !p.known(t) {
			return nil, fmt.Errorf("unknown signal %q (one of %s)", t, strings.Join(SignalNames(), ", "))
		}
		return &node{op: "sig", sig: t}, nil
	}
	return nil, fmt.Errorf("unexpected %q", t)
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Eval computes a numeric expression over a signal map (a condition reads 1 or 0). A signal the
// map lacks reads zero.
func (e *Expr) Eval(m map[string]float64) float64 {
	if e == nil || e.root == nil {
		return 0
	}
	return e.root.eval(m)
}

// True evaluates a condition.
func (e *Expr) True(m map[string]float64) bool { return e.Eval(m) != 0 }

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (n *node) eval(m map[string]float64) float64 {
	switch n.op {
	case "num":
		return n.num
	case "sig":
		return m[n.sig]
	case "neg":
		return -n.kids[0].eval(m)
	case "+":
		return n.kids[0].eval(m) + n.kids[1].eval(m)
	case "-":
		return n.kids[0].eval(m) - n.kids[1].eval(m)
	case "*":
		return n.kids[0].eval(m) * n.kids[1].eval(m)
	case "/":
		d := n.kids[1].eval(m)
		if d == 0 {
			return 0
		}
		return n.kids[0].eval(m) / d
	case "cmp":
		return b2f(compare(n.cmp, n.kids[0].eval(m), n.kids[1].eval(m)))
	case "&&":
		return b2f(n.kids[0].eval(m) != 0 && n.kids[1].eval(m) != 0)
	case "||":
		return b2f(n.kids[0].eval(m) != 0 || n.kids[1].eval(m) != 0)
	case "!":
		return b2f(n.kids[0].eval(m) == 0)
	}
	return 0
}

func compare(op string, got, want float64) bool {
	switch op {
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

// Signals lists the signal names the expression reads, in first-use order.
func (e *Expr) Signals() []string {
	var out []string
	var walk func(n *node)
	walk = func(n *node) {
		if n.op == "sig" && !contains(out, n.sig) {
			out = append(out, n.sig)
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	if e != nil && e.root != nil {
		walk(e.root)
	}
	return out
}

// String renders the expression back in the spec's words; it parses to the same tree.
func (e *Expr) String() string {
	if e == nil || e.root == nil {
		return ""
	}
	return e.root.str(0)
}

// precedence for parenthesising on render
func prec(op string) int {
	switch op {
	case "||":
		return 1
	case "&&":
		return 2
	case "!":
		return 3
	case "cmp":
		return 4
	case "+", "-":
		return 5
	case "*", "/":
		return 6
	case "neg":
		return 7
	}
	return 8
}

func (n *node) str(parent int) string {
	var s string
	switch n.op {
	case "num":
		s = trimNum(n.num)
		if n.pct {
			s += "%"
		}
	case "sig":
		s = n.sig
	case "neg":
		s = "-" + n.kids[0].str(prec("neg"))
	case "!":
		s = "!" + n.kids[0].str(prec("!")+1)
		if n.kids[0].op != "sig" && n.kids[0].op != "num" {
			s = "!(" + n.kids[0].str(0) + ")"
		}
	case "cmp":
		s = n.kids[0].str(prec("cmp")+1) + " " + n.cmp + " " + n.kids[1].str(prec("cmp")+1)
	default:
		p := prec(n.op)
		s = n.kids[0].str(p) + " " + n.op + " " + n.kids[1].str(p+1)
	}
	if prec(n.op) < parent {
		return "(" + s + ")"
	}
	return s
}

func trimNum(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Leaf is one comparison of a condition, read against a signal map: the promotion progress a
// surface shows ("passRate 82 of 90 — 8 to go").
type Leaf struct {
	Text     string  `json:"text"`     // the comparison as written
	Got      float64 `json:"got"`      // the left side's reading
	Want     float64 `json:"want"`     // the right side's reading
	Cmp      string  `json:"cmp"`      // the operator
	Met      bool    `json:"met"`      // whether it holds now
	Distance float64 `json:"distance"` // how far the left side is from holding (0 when met)
}

// Leaves lists every comparison of a condition with its reading.
func (e *Expr) Leaves(m map[string]float64) []Leaf {
	var out []Leaf
	var walk func(n *node)
	walk = func(n *node) {
		if n.op == "cmp" {
			got, want := n.kids[0].eval(m), n.kids[1].eval(m)
			l := Leaf{Text: n.str(0), Got: got, Want: want, Cmp: n.cmp, Met: compare(n.cmp, got, want)}
			if !l.Met {
				switch n.cmp {
				case ">=", ">":
					l.Distance = want - got
				case "<=", "<":
					l.Distance = got - want
				default:
					l.Distance = abs(got - want)
				}
			}
			out = append(out, l)
			return
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	if e != nil && e.root != nil {
		walk(e.root)
	}
	return out
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
