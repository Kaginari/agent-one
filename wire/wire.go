// Package wire parses and emits the envelope of Absolute Rule II: a commission (@ROOT @SCOPE
// @ASK @CAP @DUMP @SIZE) going down, a report (@S @F @V @? @U @$ @E) coming back. @CAP is
// enforced on emit: a report over its ceiling is cut, and the cut is named as a hole. @$ is
// work the agent orders one layer down; the Subagent drains it after the turn, re-checks it
// against the hierarchy, behind anything the human queued.
package wire

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DefaultCap is the answer ceiling when a commission names none.
const DefaultCap = 2048

// Commission is what a dispatcher sends down.
type Commission struct {
	Root   string
	Scope  string
	Ask    string   // findings | verdict | draft | free text
	Unsaid bool     // +unsaid: surface the unsaid
	Raw    bool     // wire:raw: the pipe-dense form is welcome
	Cap    int      // bytes; 0 means DefaultCap
	Dump   string   // overflow path
	Size   int      // a draft's payload budget
	Agent  []string // lines that are not tags — the ask in prose
}

// UnsaidKinds are the three homes of the unsaid.
var UnsaidKinds = []string{"policy", "team", "domain"}

// Unsaid is one piece of knowledge that was in a head and nowhere on disk.
type Unsaid struct {
	Kind string
	Text string
}

// Enqueue is one @$ line: work the reporting agent orders down one layer, text after
// `@$ <body>: <ask>`. The Subagent drains it after the turn; an agent never registers a Subagent
// itself mid-turn, the wire carries the order and the harness enforces the chain.
type Enqueue struct {
	Agent string
	Ask   string
}

// Forge is one drafted thing the run proved: `@FORGE <kind>:<name> | <text>`, kind tool or
// skill. A draft is never registered by the wire — it rides the report up, the harness files it
// from the report it holds (never re-parsed from a relayed digest), and the working notes (the human,
// through /forge or `agent-one forge`) registers. The agent's truth, the layer's design.
type Forge struct {
	Kind string // tool | skill
	Name string
	Text string
}

// Line is the draft as its wire line's payload: `<kind>:<name> | <text>`.
func (f Forge) Line() string {
	if f.Text == "" {
		return f.Kind + ":" + f.Name
	}
	return f.Kind + ":" + f.Name + " | " + f.Text
}

// Hash is the draft's content address: a short fnv64 over kind, name and text with runs of
// whitespace folded — a relayed line hashes the same at every hop, so the first filer wins.
func (f Forge) Hash() string {
	h := fnv.New64a()
	h.Write([]byte(strings.ToLower(f.Kind) + ":" + f.Name + "|" + strings.Join(strings.Fields(f.Text), " ")))
	return fmt.Sprintf("%016x", h.Sum64())[:10]
}

// ForgesOf reads the @FORGE lines out of any text — a wire report or a prose answer (the
// orchestrator speaks prose; its drafts still count).
func ForgesOf(text string) []Forge {
	var out []Forge
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		rest, ok := strings.CutPrefix(line, "@FORGE ")
		if !ok {
			continue
		}
		if f, ok := parseForge(rest); ok {
			out = append(out, f)
		}
	}
	return out
}

// ParseForge reads one `<kind>:<name> | <text>` payload (a filed note's text).
func ParseForge(payload string) (Forge, bool) { return parseForge(payload) }

// Report is what an agent sends back.
type Report struct {
	Status   string
	Findings []string
	Verdicts []string
	Holes    []string
	Unsaid   []Unsaid
	Tools    []string  // @T lines (dialect: the toolbox manifest)
	Other    []string  // lines the parser did not recognise, kept in order
	Enqueues []Enqueue // @$ lines — work ordered down one layer; the Subagent drains them after the turn
	Forges   []Forge   // @FORGE lines — drafted skills/tools riding up for curation
	Bytes    int       // @E as read, or as emitted
}

var tagLine = regexp.MustCompile(`^@([A-Z]+|[?$])(?:\s+(.*))?$`)

// ParseCommission reads the tags of a commission; unrecognised lines become Agent.
func ParseCommission(text string) Commission {
	c := Commission{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		m := tagLine.FindStringSubmatch(line)
		if m == nil {
			if line != "" {
				c.Agent = append(c.Agent, line)
			}
			continue
		}
		rest := strings.TrimSpace(m[2])
		switch m[1] {
		case "ROOT":
			c.Root = rest
		case "SCOPE":
			c.Scope = rest
		case "ASK":
			words := strings.Fields(rest)
			var ask []string
			for _, w := range words {
				switch w {
				case "+unsaid":
					c.Unsaid = true
				case "wire:raw":
					c.Raw = true
				default:
					ask = append(ask, w)
				}
			}
			c.Ask = strings.Join(ask, " ")
		case "CAP":
			c.Cap, _ = strconv.Atoi(rest)
		case "DUMP":
			c.Dump = rest
		case "SIZE":
			c.Size, _ = strconv.Atoi(rest)
		default:
			c.Agent = append(c.Agent, line)
		}
	}
	return c
}

// String emits the commission.
func (c Commission) String() string {
	var b strings.Builder
	if c.Root != "" {
		fmt.Fprintf(&b, "@ROOT %s\n", c.Root)
	}
	if c.Scope != "" {
		fmt.Fprintf(&b, "@SCOPE %s\n", c.Scope)
	}
	ask := c.Ask
	if c.Unsaid {
		ask += " +unsaid"
	}
	if c.Raw {
		ask += " wire:raw"
	}
	if strings.TrimSpace(ask) != "" {
		fmt.Fprintf(&b, "@ASK %s\n", strings.TrimSpace(ask))
	}
	if c.Cap > 0 {
		fmt.Fprintf(&b, "@CAP %d\n", c.Cap)
	}
	if c.Dump != "" {
		fmt.Fprintf(&b, "@DUMP %s\n", c.Dump)
	}
	if c.Size > 0 {
		fmt.Fprintf(&b, "@SIZE %d\n", c.Size)
	}
	for _, l := range c.Agent {
		b.WriteString(l + "\n")
	}
	return b.String()
}

// CapOrDefault is the ceiling in force.
func (c Commission) CapOrDefault() int {
	if c.Cap > 0 {
		return c.Cap
	}
	return DefaultCap
}

// ParseReport reads a report. It tolerates prose around the envelope (Other keeps it) and
// the raw pipe form (S|PASS, F|path:ln|claim). ok is false when no @S opened the answer.
func ParseReport(text string) (Report, bool) {
	r := Report{}
	seen := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		tag, rest, ok := splitTag(line)
		if !ok {
			r.Other = append(r.Other, line)
			continue
		}
		switch tag {
		case "S":
			r.Status, seen = rest, true
		case "F":
			r.Findings = append(r.Findings, rest)
		case "V":
			r.Verdicts = append(r.Verdicts, rest)
		case "?":
			r.Holes = append(r.Holes, rest)
		case "U":
			kind, text := "", rest
			if f := strings.Fields(rest); len(f) > 0 && isKind(f[0]) {
				kind, text = f[0], strings.TrimSpace(strings.TrimPrefix(rest, f[0]))
			}
			r.Unsaid = append(r.Unsaid, Unsaid{Kind: kind, Text: text})
		case "T":
			r.Tools = append(r.Tools, rest)
		case "FORGE":
			if f, ok := parseForge(rest); ok {
				r.Forges = append(r.Forges, f)
			} else {
				r.Other = append(r.Other, line) // a @FORGE with no `kind:name | text` is prose, kept
			}
		case "$":
			if i := strings.Index(rest, ":"); i > 0 {
				r.Enqueues = append(r.Enqueues, Enqueue{Agent: strings.ToLower(strings.TrimSpace(rest[:i])), Ask: strings.TrimSpace(rest[i+1:])})
			} else {
				r.Other = append(r.Other, line) // a @$ with no `<body>:` is prose, kept
			}
		case "E":
			r.Bytes, _ = strconv.Atoi(strings.Fields(rest + " 0")[0])
		default:
			r.Other = append(r.Other, line)
		}
	}
	return r, seen
}

// EnqueuesOf scans any text for `@$ <body>: <ask>` order lines — the same grammar reports
// carry, usable mid-commission so a relay expectation is machine-readable before the agent's
// report exists. A commission carrying `@$ zone-x: …` states the relay it expects; the subagent
// holds that expectation and names the relay unbooked if the report comes back without it
// (the Arm A stall of 2026-10-03: relay narrated in the working notes, no order on the wire, the chain
// stopped cold).
func EnqueuesOf(text string) []Enqueue {
	var out []Enqueue
	for _, line := range strings.Split(text, "\n") {
		tag, rest, ok := splitTag(line)
		if !ok || tag != "$" {
			continue
		}
		if i := strings.Index(rest, ":"); i > 0 {
			out = append(out, Enqueue{Agent: strings.ToLower(strings.TrimSpace(rest[:i])), Ask: strings.TrimSpace(rest[i+1:])})
		}
	}
	return out
}

func splitTag(line string) (tag, rest string, ok bool) {
	if m := tagLine.FindStringSubmatch(line); m != nil {
		return m[1], strings.TrimSpace(m[2]), true
	}
	if i := strings.Index(line, "|"); i > 0 && i <= 2 && strings.ToUpper(line[:i]) == line[:i] {
		t := line[:i]
		if t == "S" || t == "F" || t == "V" || t == "?" || t == "U" || t == "E" || t == "T" || t == "$" {
			return t, strings.ReplaceAll(strings.TrimSpace(line[i+1:]), "|", " — "), true
		}
	}
	return "", "", false
}

// parseForge reads `<kind>:<name> | <text>`; the pipe part may be absent (Name-only Text then
// carries the whole payload). kind is folded lower and must be tool or skill.
func parseForge(rest string) (Forge, bool) {
	i := strings.Index(rest, ":")
	if i <= 0 {
		return Forge{}, false
	}
	kind := strings.ToLower(strings.TrimSpace(rest[:i]))
	if kind != "tool" && kind != "skill" {
		return Forge{}, false
	}
	payload := strings.TrimSpace(rest[i+1:])
	name, text := payload, ""
	if j := strings.Index(payload, "|"); j >= 0 {
		name, text = strings.TrimSpace(payload[:j]), strings.TrimSpace(payload[j+1:])
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Forge{}, false
	}
	return Forge{Kind: kind, Name: name, Text: text}, true
}

func isKind(s string) bool {
	for _, k := range UnsaidKinds {
		if k == s {
			return true
		}
	}
	return false
}

// Lines renders the report as envelope lines without @E: @S, @?, @U, @V, @F, @T, then Other.
// Holes and the unsaid come first because a cut must never lose them.
func (r Report) Lines() []string {
	var out []string
	out = append(out, "@S "+strings.TrimSpace(r.Status))
	for _, h := range r.Holes {
		out = append(out, "@? "+h)
	}
	for _, u := range r.Unsaid {
		if u.Kind != "" {
			out = append(out, "@U "+u.Kind+" "+u.Text)
		} else {
			out = append(out, "@U "+u.Text)
		}
	}
	for _, v := range r.Verdicts {
		out = append(out, "@V "+v)
	}
	for _, f := range r.Findings {
		out = append(out, "@F "+f)
	}
	for _, t := range r.Tools {
		out = append(out, "@T "+t)
	}
	for _, q := range r.Enqueues {
		out = append(out, "@$ "+q.Agent+": "+q.Ask)
	}
	for _, f := range r.Forges {
		if f.Text != "" {
			out = append(out, "@FORGE "+f.Kind+":"+f.Name+" | "+f.Text)
		} else {
			out = append(out, "@FORGE "+f.Kind+":"+f.Name)
		}
	}
	out = append(out, r.Other...)
	return out
}

// Emit renders the report under a byte ceiling and closes it with an exact @E. A cap of 0
// means no ceiling. When the report does not fit, trailing lines are dropped (Other, then
// @T, then @F, then @V — never @S, @? or @U) and a hole names the cut; if the dump path is set,
// the hole points there.
func (r Report) Emit(cap int, dump string) string {
	lines := r.Lines()
	render := func(ls []string) string {
		agent := strings.Join(ls, "\n")
		// @E counts the whole answer including its own line; fixed-point on the digit count.
		n := len(agent) + len("\n@E ")
		for d := 1; ; d++ {
			if len(strconv.Itoa(n+d)) == d {
				n += d
				break
			}
		}
		return agent + "\n@E " + strconv.Itoa(n)
	}
	if cap <= 0 {
		return render(lines)
	}
	if len(render(lines)) <= cap {
		return render(lines)
	}
	keep := r
	dropped := 0
	var cutHole string
	for {
		if len(keep.Other) > 0 {
			keep.Other = keep.Other[:len(keep.Other)-1]
		} else if len(keep.Tools) > 0 {
			keep.Tools = keep.Tools[:len(keep.Tools)-1]
		} else if len(keep.Forges) > 0 {
			keep.Forges = keep.Forges[:len(keep.Forges)-1]
		} else if len(keep.Findings) > 0 {
			keep.Findings = keep.Findings[:len(keep.Findings)-1]
		} else if len(keep.Verdicts) > 0 {
			keep.Verdicts = keep.Verdicts[:len(keep.Verdicts)-1]
		} else {
			break
		}
		dropped++
		cutHole = fmt.Sprintf("@CAP %d: %d line%s cut", cap, dropped, plural(dropped))
		if dump != "" {
			cutHole += " — full report at " + dump
		}
		trial := keep
		trial.Holes = append(append([]string(nil), r.Holes...), cutHole)
		if len(render(trial.Lines())) <= cap {
			return render(trial.Lines())
		}
	}
	keep.Holes = append(append([]string(nil), r.Holes...), cutHole)
	return render(keep.Lines())
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// String emits with no ceiling.
func (r Report) String() string { return r.Emit(0, "") }

// SortedKinds lists the kinds of the unsaid present, for a dispatcher's summary.
func (r Report) SortedKinds() []string {
	set := map[string]bool{}
	for _, u := range r.Unsaid {
		set[u.Kind] = true
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
