package loop

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/tool"
)

// Guts is the loop's per-step value sense: every act is scored as it lands, the windowed momentum
// tracks the run's direction, and past a slide it speaks once inside the tool result of the act
// that caused it — an agent's gut tells mid-run ("what you just did fucked it up"); the judge of the
// end arrives later. It speaks rarely — from the floor, with a window of silence after — so a
// smooth run never hears it. Every reading journals, whether it spoke or not: the per-step
// training signal the old way never had. Learning mid-run, not only after it.
//
// The signal is deliberately cheap — outcome + lexemes — a course-corrector and training data,
// not a judge of worth. Negative lexemes catch tests that fail through pipes ("ok" exit, FAILED
// text), the class that burned fj tonight (2026-09-29).
type Guts struct {
	Window int // momentum window in steps (default 5)
	// Floor: speak when the window's sum ≤ Floor. Any value ≤ 0 is honoured when FloorExact is
	// set (config's policy.guts.floor always is); a bare Guts{} reads -2.
	Floor      int
	FloorExact bool
	// BenchThrashVisits: from this many visits to one bench (keyed by gutsBenchKey — first
	// command fields, so every `go test -run X` shares one bench), each further hit costs. The
	// 2026-10-09 audit shape: 63 red benches on one seat-fix, edits interleaved — the red pin
	// scored every visit neutral and the echo penalty's no-write gate never fired, so the thrash
	// was invisible to every guard. The red pin keeps the first visits neutral (TDD's signal);
	// past the threshold the visits are a check-loop, not development. Default 6; negative = off.
	BenchThrashVisits int
	// DigestEvery: every N steps the next tool result carries a one-line run-so-far digest
	// (the pulse line the prompt promises). Informational, never a verdict. Default 10;
	// zero or negative = off.
	DigestEvery int
	// Benches are the test workbenches the gut knows, language-neutral (policy.guts.benches); nil =
	// DefaultBenches. A bench is what the red pin and the thrash arm read.
	Benches []Bench
	// Negative and Positive are the output lexemes (policy.guts.lexemes); nil = the defaults.
	Negative, Positive []string
	// Weights price each reading (policy.guts.weights); nil = DefaultWeights.
	Weights *GutsWeights
	// ClampMin and ClampMax bound one act's lexeme reading (policy.guts.clamp); both zero = -2..+1.
	ClampMin, ClampMax int
	// Ring is how many recent workbenches the echo penalty remembers (policy.guts.ring); 0 = 12.
	Ring int
}

// Bench is one test workbench: a command that carries any Match substring, run through a shell
// tool, reads red when its output carries a Red lexeme (or the act failed) and green on a Green
// lexeme; neither is muted.
type Bench struct {
	Name  string   `json:"name"`
	Match []string `json:"match"`
	Red   []string `json:"red"`
	Green []string `json:"green"`
}

// DefaultBenches are the shipped workbenches — Go's as the gut always read it, then pytest,
// the JS runners, cargo and make.
func DefaultBenches() []Bench {
	return []Bench{
		{Name: "go", Match: []string{"go test", "go vet"}, Red: []string{"FAIL"}, Green: []string{"ok  ", " PASSED", "tests passed"}},
		{Name: "pytest", Match: []string{"pytest"}, Red: []string{"FAILED", " failed", "ERROR"}, Green: []string{" passed"}},
		{Name: "js", Match: []string{"npm test", "npm run test", "yarn test", "pnpm test", "npx jest", "npx vitest"}, Red: []string{"FAIL", "failing", "✗"}, Green: []string{"passing", "PASS"}},
		{Name: "cargo", Match: []string{"cargo test"}, Red: []string{"FAILED", "panicked"}, Green: []string{"test result: ok"}},
		{Name: "make", Match: []string{"make test", "make check"}, Red: []string{"FAIL", "Error "}, Green: []string{"PASS", "ok  "}},
	}
}

// GutsWeights price one reading of each class; a cost is negative.
type GutsWeights struct {
	Fail     int `json:"fail"`     // the act failed
	Negative int `json:"negative"` // the output reads like failure (a negative lexeme)
	Positive int `json:"positive"` // a clean positive sighting
	Repeat   int `json:"repeat"`   // the same act twice
	Echo     int `json:"echo"`     // the same bench re-run with no write since
	Thrash   int `json:"thrash"`   // a bench past its visit threshold
}

// DefaultWeights is the gut as it always priced.
func DefaultWeights() GutsWeights {
	return GutsWeights{Fail: -1, Negative: -1, Positive: 1, Repeat: -1, Echo: -1, Thrash: -1}
}

// DefaultNegative and DefaultPositive are the shipped output lexemes.
func DefaultNegative() []string {
	return []string{"FAILED", "AssertionError", "Traceback (most recent", "panic:", "No such file or directory", "command not found"}
}
func DefaultPositive() []string { return []string{" PASSED", "tests passed", "passed,"} }

// The guts' class codes: a stable name per reading kind, so lessons group by class, never by a
// why that embeds its counts.
const (
	ClassFail     = "fail"
	ClassNegative = "neg-lexeme"
	ClassRepeat   = "repeat"
	ClassEcho     = "echo"
	ClassThrash   = "thrash"
	ClassRedPin   = "red-pin"
	ClassPositive = "positive"
)

func (i *Guts) window() int {
	if i != nil && i.Window > 0 {
		return i.Window
	}
	return 5
}
func (i *Guts) floor() int {
	if i == nil {
		return -2
	}
	if i.Floor < 0 || i.FloorExact && i.Floor == 0 {
		return i.Floor
	}
	return -2
}

// thrash is the bench-visit threshold; < 0 = the arm is off.
func (i *Guts) thrash() int {
	if i == nil {
		return 6
	}
	if i.BenchThrashVisits > 0 {
		return i.BenchThrashVisits
	}
	if i.BenchThrashVisits < 0 {
		return -1
	}
	return 6
}

// digestEvery is the digest cadence; zero takes 10, negative = off.
func (i *Guts) digestEvery() int {
	if i == nil {
		return 10
	}
	if i.DigestEvery > 0 {
		return i.DigestEvery
	}
	if i.DigestEvery < 0 {
		return 0
	}
	return 10
}

func (i *Guts) benches() []Bench {
	if i != nil && i.Benches != nil {
		return i.Benches
	}
	return DefaultBenches()
}

func (i *Guts) weights() GutsWeights {
	if i != nil && i.Weights != nil {
		return *i.Weights
	}
	return DefaultWeights()
}

func (i *Guts) negative() []string {
	if i != nil && i.Negative != nil {
		return i.Negative
	}
	return DefaultNegative()
}

func (i *Guts) positive() []string {
	if i != nil && i.Positive != nil {
		return i.Positive
	}
	return DefaultPositive()
}

func (i *Guts) clamp() (int, int) {
	if i != nil && (i.ClampMin != 0 || i.ClampMax != 0) {
		return i.ClampMin, i.ClampMax
	}
	return -2, 1
}

func (i *Guts) ring() int {
	if i != nil && i.Ring > 0 {
		return i.Ring
	}
	return 12
}

// benchHit is one bench's visit tally: how many times the agent struck this workbench and how
// the visits read. Muted is the pipe class — `go test … | grep …` loses its exit status and
// its FAIL text, so the outcome is unknowable; it counts as a visit, never as good news.
type benchHit struct {
	visits, red, green, muted int
}

// benchOf reports which workbench the act strikes (nil when none — a bench rides a shell tool)
// and whether the command pipes its output away (exit status and red text lost).
func (i *Guts) benchOf(call provider.ToolCall) (*Bench, bool) {
	benchTool := call.Name == "bash" || call.Name == "shell" || call.Name == "exec" || call.Name == "shell.exec"
	if !benchTool {
		return nil, false
	}
	in := string(call.Input)
	for _, b := range i.benches() {
		for _, m := range b.Match {
			if m != "" && commandAt(in, m) {
				bb := b
				return &bb, strings.Contains(strings.ReplaceAll(in, "||", "\x00"), `|`)
			}
		}
	}
	return nil, false
}

// commandAt reports whether m opens a command word in s: at the start, or after a space, a quote,
// a paren or a shell separator — `cargo test` never reads as `go test`.
func commandAt(s, m string) bool {
	for i := 0; i+len(m) <= len(s); {
		j := strings.Index(s[i:], m)
		if j < 0 {
			return false
		}
		at := i + j
		if at == 0 || strings.ContainsRune(" \t\n\"'(;&|`", rune(s[at-1])) {
			return true
		}
		i = at + 1
	}
	return false
}

// isBench reports whether the act is a bench and whether it is piped.
func (i *Guts) isBench(call provider.ToolCall) (bench, piped bool) {
	b, p := i.benchOf(call)
	return b != nil, p
}

// benchOutcome reads one bench visit: red beats green beats muted — a bench that failed (error
// result or a red lexeme) is red even when piped; a piped bench that cannot show its face is
// muted, never green. A nil bench reads as the first configured one.
func (i *Guts) benchOutcome(b *Bench, res tool.Result) (red, green bool) {
	if b == nil {
		bs := i.benches()
		if len(bs) == 0 {
			return res.Err, false
		}
		b = &bs[0]
	}
	if res.Err || containsAny(res.Output, b.Red) {
		return true, false
	}
	if containsAny(res.Output, b.Green) {
		return false, true
	}
	return false, false
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if w != "" && strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// value scores one act: a failed result, a negative lexeme in the output even when the exit read
// ok, a clean positive sighting, the same act rerun unchanged — each priced by the weights, the
// sum clamped. It answers the reading, its why and its class codes.
func (i *Guts) value(st *StepRecord, call provider.ToolCall, res tool.Result, prevKey string) (int, string, []string) {
	w := i.weights()
	v := 0
	var why, classes []string
	if res.Err {
		v += w.Fail
		why = append(why, "the act failed")
		classes = append(classes, ClassFail)
	} else {
		// Red pin: a test bench whose output carries a red lexeme is TDD's signal, not thrash —
		// score NEUTRAL with the red-pin why, never the lexicon's «output reads like failure».
		// A non-zero-exit bench keeps «the act failed» above (res.Err untouched). The early
		// visits stay neutral; the thrash arm in gutsNote prices the visits past the threshold.
		if b, _ := i.benchOf(call); b != nil && containsAny(res.Output, b.Red) {
			return 0, "red pin — failure is the signal", []string{ClassRedPin}
		}
		if containsAny(res.Output, i.negative()) {
			v += w.Negative
			why = append(why, "the output reads like failure")
			classes = append(classes, ClassNegative)
		} else if containsAny(res.Output, i.positive()) {
			v += w.Positive
			classes = append(classes, ClassPositive)
		}
	}
	key := string(call.Input)
	if prevKey != "" && prevKey == st.Tool+"\x00"+key {
		v += w.Repeat
		why = append(why, "the same act twice")
		classes = append(classes, ClassRepeat)
	}
	lo, hi := i.clamp()
	if v < lo {
		v = lo
	}
	if v > hi {
		v = hi
	}
	return v, strings.Join(why, "; "), classes
}

// gutsBenchKey is the act's workbench identity: the tool plus the command's head — two bash calls
// render1.py / render2.py share one bench. A windowed read (offset/limit) keys on the window, not
// the path: a lawful page-walk through one file (range 1-200, 201-400, …) is not an echo — the
// third-plus rerun of one window is.
func gutsBenchKey(call provider.ToolCall) string {
	head := string(call.Input)
	if call.Name == "read" {
		return call.Name + "\x00" + head // reads keep their full window: distinct pages, distinct benches
	}
	if i := strings.Index(head, `"command"`); i >= 0 {
		if j := strings.IndexByte(head[i:], ':'); j >= 0 {
			head = strings.Trim(strings.TrimSpace(head[i+j+1:]), `"} `)
		}
	}
	fields := strings.Fields(head)
	if len(fields) > 2 {
		fields = fields[:2]
	}
	return call.Name + "\x00" + strings.Join(fields, " ")
}

// gutsNote scores the act, journals the reading, and — past the floor, with a once-window silence —
// returns the line that rides the tool result back to the agent.
func (s *Session) gutsNote(st *StepRecord, call provider.ToolCall, res tool.Result) string {
	i := s.Engine.Guts
	if i == nil {
		return ""
	}
	key := st.Tool + "\x00" + string(call.Input)
	v, why, classes := i.value(st, call, res, s.gutsPrevKey)
	s.gutsPrevKey = key
	w := i.weights()
	// bench tally: the visit record the thrash arm and the pulse digest read. Every bench
	// visit counts, writes or not — the debug rhythm's edit-test loop is lawful a few
	// times; a dozen hits on one bench with no green is a loop no matter what sits
	// between the hits (the 2026-10-09 audit: the echo penalty's no-write gate never saw
	// the thrash because every test run had an edit before it).
	var hit *benchHit
	if bench, piped := i.benchOf(call); bench != nil {
		if s.gutsBenchHits == nil {
			s.gutsBenchHits = map[string]*benchHit{}
		}
		k := gutsBenchKey(call)
		hit = s.gutsBenchHits[k]
		if hit == nil {
			hit = &benchHit{}
			s.gutsBenchHits[k] = hit
		}
		hit.visits++
		if red, green := i.benchOutcome(bench, res); red {
			hit.red++
		} else if green {
			hit.green++
		} else {
			if piped {
				hit.muted++
			}
		}
	}
	// echo penalty: third-plus clean rerun of the same read/bash bench with no write since the last
	// visit — the gut's clean-sight lesson: rereading the same window harder never sharpens the
	// first read. Iterating-with-edits (the debug rhythm) is not an echo.
	bench := gutsBenchKey(call)
	visited := 0
	for _, b := range s.gutsBench {
		if b == bench {
			visited++
		}
	}
	lastWrote := -1
	if s.gutsBenchWrot != nil {
		lastWrote = s.gutsBenchWrot[bench]
	}
	if visited >= 2 && !res.Err && lastWrote >= 0 && lastWrote == s.wroteTot && (st.Tool == "bash" || st.Tool == "read") && !strings.Contains(why, "the same act twice") {
		v += w.Echo
		if why != "" {
			why += "; "
		}
		why += "the same bench re-run — a closer look is not new evidence"
		classes = append(classes, ClassEcho)
	}
	if s.gutsBenchWrot == nil {
		s.gutsBenchWrot = map[string]int{}
	}
	s.gutsBenchWrot[bench] = s.wroteTot
	s.gutsBench = append(s.gutsBench, bench)
	if ring := i.ring(); len(s.gutsBench) > ring {
		s.gutsBench = s.gutsBench[len(s.gutsBench)-ring:]
	}
	// bench thrash: past the visit threshold every further hit on the same bench costs —
	// the red pin keeps the early visits neutral (TDD's signal); a bench struck this many
	// times is a check-loop, not development. Independent of the echo penalty's write gate.
	if thr := i.thrash(); hit != nil && thr > 0 && hit.visits >= thr {
		v += w.Thrash
		s.gutsThrash++
		classes = append(classes, ClassThrash)
		if why != "" {
			why += "; "
		}
		why += fmt.Sprintf("bench thrash — %d hits on one bench (%d red / %d green / %d muted) — a closer look is not new evidence", hit.visits, hit.red, hit.green, hit.muted)
	}
	s.gutsVals = append(s.gutsVals, v)
	vals := s.gutsVals
	if len(vals) > i.window() {
		vals = vals[len(vals)-i.window():]
	}
	m := 0
	for _, x := range vals {
		m += x
	}
	spoke := m <= i.floor() && s.Steps-s.gutsLastSpoke >= i.window()
	s.gutsMomentum = m
	s.gutsReads++
	ev := Event{"t": "guts", "id": st.ID, "v": v, "momentum": m, "why": why, "spoke": spoke}
	if len(classes) > 0 {
		ev["class"], ev["classes"] = classes[0], classes
	}
	s.Journal.Log(ev)
	if m > i.floor() {
		s.gutsSpoke = 0 // a window above the floor counts as recovery — streak resets
	}
	if why != "" {
		s.gutsLastWhy = why
	}
	if !spoke {
		return s.digest(i)
	}
	s.gutsLastSpoke = s.Steps
	s.gutsSpoke++
	whyNote := why
	if whyNote == "" {
		// the floor was crossed by an act with no why of its own (a clean write between
		// thrash hits): the note still names the run's losing ground, never an empty pair
		whyNote = s.gutsLastWhy
	}
	return "\n[guts: the run is losing ground (" + whyNote + ") — reassess the approach before running the same act again]"
}

// digest is the one-line run-so-far self-state that rides a tool result every DigestEvery
// steps — the prompt's pulse promise made ambient. Informational, never a verdict.
func (s *Session) digest(i *Guts) string {
	if every := i.digestEvery(); every == 0 || s.Steps == 0 || s.Steps%every != 0 {
		return ""
	}
	return "\n[pulse: " + s.Pulse() + "]"
}

// Pulse is the acting agent's own run state: what the `pulse` tool returns and the digest
// line quotes. One screen, no judgment — the bench table is the loop-blindspot cure: an agent
// that can see "23 hits, 19 red, 0 green" breaks its own check-loop without a subagent.
func (s *Session) Pulse() string {
	c := s.Reading()
	var b strings.Builder
	fmt.Fprintf(&b, "steps %d · turns %d · wall %s · ctx %s/%s zone %s",
		s.Steps, s.Turns, time.Since(s.Started).Round(time.Second), tok(c.Tokens), tok(c.Limit), c.Zone)
	if i := s.Engine.Guts; i != nil {
		vals := s.gutsVals
		if len(vals) > i.window() {
			vals = vals[len(vals)-i.window():]
		}
		m := 0
		for _, x := range vals {
			m += x
		}
		last := "never"
		if s.gutsLastSpoke > 0 {
			last = fmt.Sprintf("step %d", s.gutsLastSpoke)
		}
		fmt.Fprintf(&b, "\nmomentum %d (window %d) · last spoke %s", m, i.window(), last)
	}
	if len(s.gutsBenchHits) > 0 {
		keys := make([]string, 0, len(s.gutsBenchHits))
		for k := range s.gutsBenchHits {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(a, b int) bool { return s.gutsBenchHits[keys[a]].visits > s.gutsBenchHits[keys[b]].visits })
		if len(keys) > 3 {
			keys = keys[:3]
		}
		b.WriteString("\nbenches:")
		for _, k := range keys {
			h := s.gutsBenchHits[k]
			fmt.Fprintf(&b, " `%s` ×%d (%d red / %d green / %d muted)", strings.ReplaceAll(k, "\x00", " "), h.visits, h.red, h.green, h.muted)
		}
	}
	fmt.Fprintf(&b, "\nwrites %d · failed acts %d", s.wroteTot, s.failures)
	return b.String()
}

// tok renders a token count for the digest line.
func tok(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%d", n)
}
