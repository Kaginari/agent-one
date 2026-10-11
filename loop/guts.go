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
	Floor  int // speak when the window's sum ≤ Floor (default -2)
	// BenchThrashVisits: from this many visits to one bench (the go test / go vet family,
	// keyed by gutsBenchKey — first command fields, so every `go test -run X` shares one
	// bench), each further hit costs. The 2026-10-09 audit shape: 63 red benches on one
	// seat-fix, edits interleaved — the red pin scored every visit neutral and the echo
	// penalty's no-write gate never fired, so the thrash was invisible to every guard.
	// The red pin keeps the first visits neutral (TDD's signal); past the threshold the
	// visits are a check-loop, not development. Default 6; negative = off.
	BenchThrashVisits int
	// DigestEvery: every N steps the next tool result carries a one-line run-so-far digest
	// (the pulse line the prompt promises). Informational, never a verdict. Default 10;
	// zero or negative = off.
	DigestEvery int
}

func (i Guts) window() int {
	if i.Window > 0 {
		return i.Window
	}
	return 5
}
func (i Guts) floor() int {
	if i.Floor < 0 {
		return i.Floor
	}
	return -2
}

// thrash is the bench-visit threshold; < 0 = the arm is off.
func (i Guts) thrash() int {
	if i.BenchThrashVisits > 0 {
		return i.BenchThrashVisits
	}
	if i.BenchThrashVisits < 0 {
		return -1
	}
	return 6
}

// digestEvery is the digest cadence; zero takes 10, negative = off.
func (i Guts) digestEvery() int {
	if i.DigestEvery > 0 {
		return i.DigestEvery
	}
	if i.DigestEvery < 0 {
		return 0
	}
	return 10
}

// benchHit is one bench's visit tally: how many times the agent struck this workbench and how
// the visits read. Muted is the pipe class — `go test … | grep …` loses its exit status and
// its FAIL text, so the outcome is unknowable; it counts as a visit, never as good news.
type benchHit struct {
	visits, red, green, muted int
}

// gutsBenchCall reports whether the act is a test bench (go test / go vet through a shell
// tool) and whether the command pipes its output away (exit status + FAIL text lost).
func gutsBenchCall(call provider.ToolCall) (bench, piped bool) {
	benchTool := call.Name == "bash" || call.Name == "shell" || call.Name == "exec" || call.Name == "shell.exec"
	in := string(call.Input)
	if !benchTool || !(strings.Contains(in, `go test`) || strings.Contains(in, `go vet`)) {
		return false, false
	}
	return true, strings.Contains(strings.ReplaceAll(in, "||", "\x00"), `|`)
}

// gutsBenchOutcome reads one bench visit: red beats green beats muted — a bench that failed
// (error result or FAIL text) is red even when piped; a piped bench that cannot show its
// face is muted, never green.
func gutsBenchOutcome(res tool.Result) (red, green bool) {
	if res.Err || strings.Contains(res.Output, `FAIL`) {
		return true, false
	}
	if strings.Contains(res.Output, "ok  ") || strings.Contains(res.Output, " PASSED") || strings.Contains(res.Output, "tests passed") {
		return false, true
	}
	return false, false
}

var gutsNegative = []string{"FAILED", "AssertionError", "Traceback (most recent", "panic:", "No such file or directory", "command not found"}
var gutsPositive = []string{" PASSED", "tests passed", "passed,"}

// gutsValue scores one act: failed result −1; negative lexemes in the output −1 even when the
// exit read ok; clean positive sighting +1; the same act rerun unchanged −1. Clamped to [-2, +1].
func gutsValue(st *StepRecord, call provider.ToolCall, res tool.Result, prevKey string) (int, string) {
	v := 0
	var why []string
	if res.Err {
		v--
		why = append(why, "the act failed")
	} else {
		// Red pin: a test bench (go test / go vet) whose output carries FAIL is TDD's
		// signal, not thrash — score NEUTRAL with the red-pin why, never the lexicon's
		// «output reads like failure». Ok-exit/FAILED-text red run; a non-zero-exit
		// bench keeps «the act failed» above (res.Err untouched). The early visits stay
		// neutral; the thrash arm in gutsNote prices the visits past the threshold.
		if bench, _ := gutsBenchCall(call); bench && strings.Contains(res.Output, `FAIL`) {
			return 0, "red pin — failure is the signal"
		}
		neg := false
		for _, w := range gutsNegative {
			if strings.Contains(res.Output, w) {
				neg = true
				break
			}
		}
		if neg {
			v--
			why = append(why, "the output reads like failure")
		} else {
			for _, w := range gutsPositive {
				if strings.Contains(res.Output, w) {
					v++
					break
				}
			}
		}
	}
	key := string(call.Input)
	if prevKey != "" && prevKey == st.Tool+"\x00"+key {
		v--
		why = append(why, "the same act twice")
	}
	if v < -2 {
		v = -2
	}
	if v > 1 {
		v = 1
	}
	return v, strings.Join(why, "; ")
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
	v, why := gutsValue(st, call, res, s.gutsPrevKey)
	s.gutsPrevKey = key
	// bench tally: the visit record the thrash arm and the pulse digest read. Every bench
	// visit counts, writes or not — the debug rhythm's edit-test loop is lawful a few
	// times; a dozen hits on one bench with no green is a loop no matter what sits
	// between the hits (the 2026-10-09 audit: the echo penalty's no-write gate never saw
	// the thrash because every test run had an edit before it).
	var hit *benchHit
	if bench, piped := gutsBenchCall(call); bench {
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
		if red, green := gutsBenchOutcome(res); red {
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
		v--
		if why != "" {
			why += "; "
		}
		why += "the same bench re-run — a closer look is not new evidence"
	}
	if s.gutsBenchWrot == nil {
		s.gutsBenchWrot = map[string]int{}
	}
	s.gutsBenchWrot[bench] = s.wroteTot
	s.gutsBench = append(s.gutsBench, bench)
	if len(s.gutsBench) > 12 {
		s.gutsBench = s.gutsBench[len(s.gutsBench)-12:]
	}
	// bench thrash: past the visit threshold every further hit on the same bench costs —
	// the red pin keeps the early visits neutral (TDD's signal); a bench struck this many
	// times is a check-loop, not development. Independent of the echo penalty's write gate.
	if thr := i.thrash(); hit != nil && thr > 0 && hit.visits >= thr {
		v--
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
	s.Journal.Log(Event{"t": "guts", "id": st.ID, "v": v, "momentum": m, "why": why, "spoke": spoke})
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
