package loop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
	"github.com/Kaginari/agent-one/tool"
)

// The 2026-10-09 audit shape (zone-onto 53ca): one bench (every `go test -run X` shares the
// gut bench key), varied filters so no two commands are identical, red every time, writes
// interleaved between the hits — the red pin scored every visit neutral, the echo penalty's
// no-write gate never fired, and 63 visits later the run died on the gateway with every
// health guard silent. These tests pin the cure: the visit tally sees the loop.
func TestGutsBenchThrashSpeaks(t *testing.T) {
	// the strict 53ca shape: a write between EVERY bench pair — the echo penalty's
	// no-write gate never fires (the audit's exact finding), only the visit tally sees
	// the loop. Eight visits: five stay neutral red pins, the sixth on costs.
	bench := func(run string) provider.Response {
		return mock.Call(run, "bash", map[string]string{"command": "echo 'go test -run " + run + " FAIL'"})
	}
	pipe := func(run string) provider.Response {
		return mock.Call(run, "bash", map[string]string{"command": "echo 'go test -run " + run + " 2>&1 | tee /dev/null'"})
	}
	write := func(n string) provider.Response {
		return mock.Call(n, "write", map[string]string{"path": n + ".go", "content": "v"})
	}
	thrash := mock.New(
		pipe("A"), write("f1"), pipe("B"), write("f2"),
		bench("C"), write("f3"), bench("D"), write("f4"), bench("E"), write("f5"),
		bench("F"), write("f6"), bench("G"), write("f7"), bench("H"), write("f8"),
		mock.Text("still going"), mock.Text("done"),
	)
	e := engine(t, thrash)
	e.Guts = &Guts{Window: 5, Floor: -2, BenchThrashVisits: 6}
	e.Budget.Retries = 20 // the thrash must be scored and spoken, not escalated
	r, _ := e.Run(context.Background(), "fix the seat")
	if r.Status != Done {
		t.Fatalf("%+v", r)
	}
	spokeAny, thrashFired, echoFired, tallyRead := false, false, false, false
	earlyNeutral := 0
	for _, x := range events(t, e, r.RunID) {
		if x["t"] != "guts" {
			continue
		}
		why, _ := x["why"].(string)
		v, _ := x["v"].(int)
		if strings.Contains(why, "red pin") && v == 0 && !strings.Contains(why, "bench thrash") {
			earlyNeutral++ // the first reds stay TDD's signal
		}
		if strings.Contains(why, "same bench re-run") {
			echoFired = true // must NOT: a write sits between every pair
		}
		if x["spoke"] == true {
			spokeAny = true
		}
		if strings.Contains(why, "bench thrash") {
			thrashFired = true
			if strings.Contains(why, "2 muted") && strings.Contains(why, "6 red") {
				tallyRead = true // the two piped visits read in the tally
			}
		}
	}
	if echoFired {
		t.Fatal("with a write between every bench pair the echo penalty must stay silent — this is the 53ca blindspot")
	}
	if earlyNeutral < 3 {
		t.Fatalf("the first bench visits must stay neutral red pins, got %d", earlyNeutral)
	}
	if !thrashFired {
		t.Fatal("the visit tally must price the bench past the threshold — the 53ca blindspot")
	}
	if !spokeAny {
		t.Fatal("the thrash must drive the run to speak")
	}
	if !tallyRead {
		t.Fatal("the thrash why must carry the tally: 6 red / 0 green / 2 muted")
	}
	heard := false
	for _, req := range thrash.Requests {
		for _, msg := range req.Messages {
			for _, res := range msg.ToolResults {
				if strings.Contains(res.Content, "[guts:") && strings.Contains(res.Content, "bench thrash") {
					heard = true
				}
			}
		}
	}
	if !heard {
		t.Fatal("the agent must hear the thrash inside the tool result that caused it")
	}
}

func TestGutsPipedBenchCounted(t *testing.T) {
	// the 18-of-63 unclassifiable class: a bench piped through a filter loses its exit
	// status and its FAIL text — muted, never green; FAIL text that survives the pipe
	// still reads red.
	bench, piped := (*Guts)(nil).isBench(provider.ToolCall{Name: "bash", Input: json.RawMessage(`{"command":"go test -count=1 ./onto 2>&1 | grep -E \"FAIL|ok\""}`)})
	if !bench || !piped {
		t.Fatalf("a piped go test must read as a piped bench: bench=%v piped=%v", bench, piped)
	}
	if red, green := (*Guts)(nil).benchOutcome(nil, tool.Result{Output: "workspace_test.go:571: class (f)"}); red || green {
		t.Fatalf("a signal-less bench outcome is muted, never green: red=%v green=%v", red, green)
	}
	if red, _ := (*Guts)(nil).benchOutcome(nil, tool.Result{Output: "--- FAIL: TestX\nFAIL\t./onto\t0.1s"}); !red {
		t.Fatal("FAIL text surviving the pipe still reads red")
	}
	if _, green := (*Guts)(nil).benchOutcome(nil, tool.Result{Output: "ok  \tgithub.com/Kaginari/agent-one/onto\t0.2s"}); !green {
		t.Fatal("a clean ok bench reads green")
	}
	if bench, _ := (*Guts)(nil).isBench(provider.ToolCall{Name: "bash", Input: json.RawMessage(`{"command":"echo hello"}`)}); bench {
		t.Fatal("a command with neither go test nor go vet is not a bench")
	}
	if bench, _ := (*Guts)(nil).isBench(provider.ToolCall{Name: "read", Input: json.RawMessage(`{"path":"go test.go"}`)}); bench {
		t.Fatal("a read tool is never a bench — bench calls ride shell tools")
	}
}

func TestGutsDigestRidesEveryNSteps(t *testing.T) {
	// the prompt promises the pulse line; the digest makes it ambient — one line every
	// DigestEvery steps on the next tool result, silent between.
	calmm := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "echo one"}),
		mock.Call("2", "bash", map[string]string{"command": "echo two"}),
		mock.Call("3", "bash", map[string]string{"command": "echo three"}),
		mock.Call("4", "bash", map[string]string{"command": "echo four"}),
		mock.Call("5", "bash", map[string]string{"command": "echo five"}),
		mock.Call("6", "bash", map[string]string{"command": "echo six"}),
		mock.Call("7", "bash", map[string]string{"command": "echo seven"}),
		mock.Call("8", "bash", map[string]string{"command": "echo eight"}),
		mock.Call("9", "bash", map[string]string{"command": "echo nine"}),
		mock.Call("10", "bash", map[string]string{"command": "echo ten"}),
		mock.Text("closed"), mock.Text("closed again"),
	)
	e := engine(t, calmm)
	e.Guts = &Guts{Window: 5, Floor: -2} // digestEvery defaults to 10
	e.Budget.Retries = 5
	r, _ := e.Run(context.Background(), "walk")
	if r.Status != Done {
		t.Fatalf("%+v", r)
	}
	digests, others := 0, 0
	for _, req := range calmm.Requests {
		for _, msg := range req.Messages {
			for _, res := range msg.ToolResults {
				if strings.Contains(res.Content, "[pulse:") {
					digests++
					if !strings.Contains(res.Content, "steps 10") || !strings.Contains(res.Content, "momentum 0") {
						t.Fatalf("the digest must carry the run state: %q", res.Content)
					}
				} else if strings.Contains(res.Content, "[guts:") {
					others++
				}
			}
		}
	}
	if digests != 1 || others != 0 {
		t.Fatalf("exactly one digest at step 10, no guts note on a smooth run: digests=%d guts=%d", digests, others)
	}
}

// The benches are language-neutral: a pytest bench reads red on its own lexemes and pins red
// (TDD's signal, neutral), a configured bench replaces the shipped list, and a journal reading
// carries a stable class code.
func TestGutsBenchesLanguageNeutral(t *testing.T) {
	var g *Guts
	pytest := provider.ToolCall{Name: "bash", Input: json.RawMessage(`{"command":"python -m pytest tests/test_auth.py"}`)}
	b, _ := g.benchOf(pytest)
	if b == nil || b.Name != "pytest" {
		t.Fatalf("pytest is a bench: %+v", b)
	}
	out := "tests/test_auth.py::test_login FAILED\n=== 1 failed, 3 passed in 0.12s ==="
	if red, green := g.benchOutcome(b, tool.Result{Output: out}); !red || green {
		t.Fatal("a failing pytest reads red, red beats green")
	}
	v, why, classes := g.value(&StepRecord{Tool: "bash"}, pytest, tool.Result{Output: out}, "")
	if v != 0 || !strings.Contains(why, "red pin") || len(classes) != 1 || classes[0] != ClassRedPin {
		t.Fatalf("a red pytest bench pins red: v=%d why=%q classes=%v", v, why, classes)
	}
	cargo := provider.ToolCall{Name: "bash", Input: json.RawMessage(`{"command":"cargo test"}`)}
	if b, _ := g.benchOf(cargo); b == nil || b.Name != "cargo" {
		t.Fatal("cargo test is a bench")
	}
	// a configured bench list replaces the shipped one; weights and the clamp are config's
	own := &Guts{Benches: []Bench{{Name: "mix", Match: []string{"mix test"}, Red: []string{"failure"}}}, Weights: &GutsWeights{Fail: -3}, ClampMin: -3, ClampMax: 1}
	if b, _ := own.benchOf(pytest); b != nil {
		t.Fatal("pytest is not on a list that does not name it")
	}
	if v, _, cls := own.value(&StepRecord{Tool: "bash"}, cargo, tool.Result{Err: true}, ""); v != -3 || cls[0] != ClassFail {
		t.Fatalf("the fail weight is config's: v=%d %v", v, cls)
	}
	// a non-bench act reading like failure is the neg-lexeme class
	if _, _, cls := g.value(&StepRecord{Tool: "read"}, provider.ToolCall{Name: "read", Input: json.RawMessage(`{"path":"x"}`)}, tool.Result{Output: "Traceback (most recent call last)"}, ""); len(cls) != 1 || cls[0] != ClassNegative {
		t.Fatalf("neg-lexeme class: %v", cls)
	}
}

// A guts floor of zero is honoured when config set it (FloorExact); a bare Guts{} keeps -2.
func TestGutsFloorExact(t *testing.T) {
	if (&Guts{}).floor() != -2 || (&Guts{Floor: 0, FloorExact: true}).floor() != 0 || (&Guts{Floor: -4}).floor() != -4 {
		t.Fatal("floor")
	}
}

// Holes are split by kind: the harness's catches of the agent's discipline cost it, the workspace's
// faults do not, the agent's own @? are counted apart.
func TestHoleKinds(t *testing.T) {
	for h, want := range map[string]string{
		"answer off the wire — sent back (1 of 1)":                      HoleHarness,
		"report carries no @U line — sent back (1 of 1)":                HoleHarness,
		"gate failed and was sent back (1 of 1): fail":                  HoleHarness,
		"closed with an open plan: 2 of 3 open":                         HoleHarness,
		"the provider refused the request":                              HoleInfra,
		"the model's answer was cut by the output ceiling (max_tokens)": HoleInfra,
		"drain failed: boom":                                            HoleInfra,
		"recall: memory: index stale":                                   HoleInfra,
		"policy.gate is off — the turn's writes were not checked":       HoleInfra,
		"forge note: disk full":                                         HoleInfra,
	} {
		if got := HoleKind(h); got != want {
			t.Errorf("%q → %s, want %s", h, got, want)
		}
	}
	r := &Result{IsWire: true, Holes: []string{"the provider refused the request", "answer off the wire — sent back (1 of 1)"}}
	r.Report.Holes = []string{"unknown a", "unknown b"}
	if i, h, s := r.HoleCount(); i != 1 || h != 1 || s != 2 {
		t.Fatalf("HoleCount %d %d %d", i, h, s)
	}
}
