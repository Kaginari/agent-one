package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/gate"
	"github.com/Kaginari/agent-one/instrument"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
	"github.com/Kaginari/agent-one/tool"
)

func engine(t *testing.T, m *mock.Provider) *Engine {
	t.Helper()
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".agent-one"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("hello workspace\n"), 0o644)
	g := gate.New()
	g.IsTTY = func() bool { return false }
	return &Engine{Provider: m, Tools: tool.Builtins(), Gate: g, Root: root, As: "zone-test", Unsaid: true}
}

func TestClosingCheckAsksOnce(t *testing.T) {
	// closingCheck on: the first close with no tool call is sent back once; the agent resumes into
	// a real step; the second close passes with no second ask.
	m := mock.New(mock.Text("let me merge it"), mock.Call("1", "write", map[string]string{"path": "a.txt", "content": "x"}), mock.Text("merged"))
	e := engine(t, m)
	e.ClosingCheck = true
	r, _ := e.Run(context.Background(), "merge the commits")
	if r.Status != Done || len(r.Steps) != 1 || len(m.Requests) != 3 {
		t.Fatalf("%+v requests=%d", r, len(m.Requests))
	}
	ask := m.Requests[1].Messages[len(m.Requests[1].Messages)-1].Text
	if !strings.Contains(ask, "closing check") {
		t.Fatalf("the re-ask must carry the closing check, got %q", ask)
	}
	// still closing with prose: asked once, then let through — never twice.
	m2 := mock.New(mock.Text("nothing to do"), mock.Text("really nothing"))
	e.Provider = m2
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Done || r.Text != "really nothing" || len(m2.Requests) != 2 {
		t.Fatalf("%+v requests=%d", r, len(m2.Requests))
	}
	// off: closed at the first no-tool-call message, as before.
	e.ClosingCheck = false
	e.Provider = mock.New(mock.Text("short"))
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Done || r.Text != "short" {
		t.Fatalf("%+v", r)
	}
}

func TestClosingCheckDemandsWitness(t *testing.T) {
	reask := func(m *mock.Provider) string {
		for _, req := range m.Requests {
			t2 := req.Messages[len(req.Messages)-1].Text
			if strings.Contains(t2, "closing check") {
				return t2
			}
		}
		return ""
	}
	// writes + close: the re-ask demands falsification witnesses, named output files included
	m := mock.New(mock.Call("1", "write", map[string]string{"path": "out.txt", "content": "x"}), mock.Text("done"), mock.Text("really done"))
	e := engine(t, m)
	e.ClosingCheck = true
	r, _ := e.Run(context.Background(), "build the report")
	if r.Status != Done || len(m.Requests) != 3 {
		t.Fatalf("%+v requests=%d", r, len(m.Requests))
	}
	ask := reask(m)
	if !strings.Contains(ask, "witness") || !strings.Contains(ask, "out.txt") {
		t.Fatalf("the mutating close must demand witnesses and name the files, got %q", ask)
	}
	if strings.Contains(ask, "byte-level compare") {
		t.Fatalf("no invariance words in the ask — the byte-compare demand must not fire: %q", ask)
	}
	// an ask carrying invariance words gets the byte-compare demand on top
	m2 := mock.New(mock.Call("1", "write", map[string]string{"path": "a.html", "content": "x"}), mock.Text("ok"), mock.Text("closed"))
	e.Provider = m2
	_, _ = e.Run(context.Background(), "strip the scripts but keep the file byte-identical otherwise")
	ask = reask(m2)
	if !strings.Contains(ask, "byte-level compare") {
		t.Fatalf("an invariance ask must trigger the byte-compare witness, got %q", ask)
	}
	// read-only close: no witness demand at all
	m3 := mock.New(mock.Text("looked"), mock.Text("looked twice"))
	e.Provider = m3
	_, _ = e.Run(context.Background(), "count the lines, identical output please")
	ask = reask(m3)
	if strings.Contains(ask, "witness") || strings.Contains(ask, "byte-level") {
		t.Fatalf("read-only closes carry no witness demand, got %q", ask)
	}
}

func TestClosingCheckDemandsForgeOnLongRuns(t *testing.T) {
	reask := func(m *mock.Provider) string {
		for _, req := range m.Requests {
			t2 := req.Messages[len(req.Messages)-1].Text
			if strings.Contains(t2, "closing check") {
				return t2
			}
		}
		return ""
	}
	// 12 tool steps then a close: the re-ask carries the forge question — the long run must
	// either name a @FORGE draft or answer why the path is one-off
	var seq []provider.Response
	for i := 0; i < 12; i++ {
		seq = append(seq, mock.Call(fmt.Sprintf("%d", i), "read", map[string]string{"path": fmt.Sprintf("f%d.md", i)}))
	}
	seq = append(seq, mock.Text("finished"), mock.Text("really finished"))
	m := mock.New(seq...)
	e := engine(t, m)
	for i := 0; i < 12; i++ { // the tour's files must exist — failed reads escalate before the close
		if err := os.WriteFile(filepath.Join(e.Root, fmt.Sprintf("f%d.md", i)), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e.ClosingCheck = true
	e.ForgeDemandAt = 12
	if r, _ := e.Run(context.Background(), "tour the package"); r.Status == Fail {
		t.Fatalf("%+v", r)
	}
	if ask := reask(m); !strings.Contains(ask, "@FORGE") {
		t.Fatalf("the long close must ask the forge question, got %q", ask)
	}
	// a short run teaches nothing: no forge question under the floor
	m2 := mock.New(mock.Call("1", "read", map[string]string{"path": "x"}), mock.Text("done"), mock.Text("really done"))
	e.Provider = m2
	if r, _ := e.Run(context.Background(), "peek"); r.Status == Fail {
		t.Fatalf("%+v", r)
	}
	if ask := reask(m2); strings.Contains(ask, "@FORGE") {
		t.Fatalf("short runs carry no forge question, got %q", ask)
	}
}

// policy.forge stands on its own: with the closing check off, a run past demandAtSteps is still
// asked the forge question once; 0 switches it off; the promise names what really happens.
func TestForgeDemandIndependentOfClosingCheck(t *testing.T) {
	run := func(floor int) (*mock.Provider, *Result) {
		var seq []provider.Response
		for i := 0; i < 12; i++ {
			seq = append(seq, mock.Call(fmt.Sprintf("%d", i), "read", map[string]string{"path": fmt.Sprintf("f%d.md", i)}))
		}
		seq = append(seq, mock.Text("finished"), mock.Text("finished, one-off"))
		m := mock.New(seq...)
		e := engine(t, m)
		for i := 0; i < 12; i++ {
			if err := os.WriteFile(filepath.Join(e.Root, fmt.Sprintf("f%d.md", i)), []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		e.ForgeDemandAt = floor
		r, err := e.Run(context.Background(), "tour the package")
		if err != nil {
			t.Fatal(err)
		}
		return m, r
	}
	m, r := run(12)
	last := m.Requests[len(m.Requests)-1].Messages
	ask := last[len(last)-1].Text
	if !strings.Contains(ask, "forge question") || !strings.Contains(ask, "@FORGE skill:<name>") || !strings.Contains(ask, "crafted-skills/<name>/SKILL.md") || strings.Contains(ask, "three steps") {
		t.Fatalf("the forge question must fire without the closing check and say what a register lands:\n%s", ask)
	}
	if r.Text != "finished, one-off" || r.Status != Done {
		t.Fatalf("the answer after the question closes the run: %q %s", r.Text, r.Status)
	}
	m0, r0 := run(0)
	for _, req := range m0.Requests {
		if strings.Contains(req.Messages[len(req.Messages)-1].Text, "forge question") {
			t.Fatal("demandAtSteps 0 is the policy switched off")
		}
	}
	if r0.Text != "finished" {
		t.Fatalf("off: the first close stands, got %q", r0.Text)
	}
}

func TestGutsSpeakingOnASlideAndQuietOnSmooth(t *testing.T) {
	// three bad acts in a row: the guts speak inside a tool result, and the journal carries every
	// reading (per-step training signal), most recent one marked spoke
	fail := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "false"}),
		mock.Call("2", "bash", map[string]string{"command": "exit 2"}),
		mock.Call("3", "bash", map[string]string{"command": "false yet again"}),
		mock.Text("not going anywhere"), mock.Text("done"),
	)
	e := engine(t, fail)
	e.Guts = &Guts{Window: 3, Floor: -2}
	e.Budget.Retries = 5 // headroom: the slide must be scored, not escalated, for this test
	r, _ := e.Run(context.Background(), "x")
	if r.Status != Done {
		t.Fatalf("%+v", r)
	}
	spoke := false
	for _, req := range fail.Requests {
		for _, msg := range req.Messages {
			for _, res := range msg.ToolResults {
				if strings.Contains(res.Content, "[guts:") {
					spoke = true
				}
			}
		}
	}
	if !spoke {
		t.Fatal("three failed acts must make the guts speak")
	}
	ev := events(t, e, r.RunID)
	readings, lastSpoke := 0, false
	for _, x := range ev {
		if x["t"] == "guts" {
			readings++
			lastSpoke = x["spoke"] == true
		}
	}
	if readings != 3 || !lastSpoke {
		t.Fatalf("the journal must carry one reading per act, last one spoken: %d events, lastSpoke=%v", readings, lastSpoke)
	}
	// a smooth run: silent
	ok := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "echo hi"}),
		mock.Call("2", "bash", map[string]string{"command": "echo there"}),
		mock.Text("passed all"),
	)
	e.Provider = ok
	r, _ = e.Run(context.Background(), "y")
	for _, x := range events(t, e, r.RunID) {
		if x["t"] == "guts" && x["spoke"] == true {
			t.Fatalf("a smooth run stays silent: %v", x)
		}
	}
	// off by default: no readings at all
	e2 := engine(t, mock.New(mock.Call("1", "bash", map[string]string{"command": "false"}), mock.Text("x")))
	r, _ = e2.Run(context.Background(), "z")
	for _, x := range events(t, e2, r.RunID) {
		if x["t"] == "guts" {
			t.Fatalf("nil guts must not read: %v", x)
		}
	}
}

func TestGutsBenchKeyReadWindowsAreDistinctBenches(t *testing.T) {
	// tonight's shape: a zone worker sweeping one file's five lawful windows is a page-walk, not an echo;
	// re-reading the SAME window thrice is the echo. The bench key for reads rides offset+limit.
	w1 := gutsBenchKey(provider.ToolCall{Name: "read", Input: json.RawMessage(`{"path":"loop.go","offset":1,"limit":100}`)})
	w2 := gutsBenchKey(provider.ToolCall{Name: "read", Input: json.RawMessage(`{"path":"loop.go","offset":101,"limit":100}`)})
	if w1 == w2 {
		t.Fatal("windowed reads of one file must not share a bench — that punishes lawful page-walks")
	}
	w3 := gutsBenchKey(provider.ToolCall{Name: "read", Input: json.RawMessage(`{"path":"loop.go","offset":1,"limit":100}`)})
	if w1 != w3 {
		t.Fatal("the same window twice must share a bench — that is the echo the penalty guards")
	}
}

func TestGutsRedPin(t *testing.T) {
	// A test-bench (go test / go vet) whose output carries FAIL is TDD's signal,
	// not thrash: gutsValue must score NEUTRAL (0) with why «red pin — …», not
	// −1 «output reads like failure». Genuine thrash (echo arm) stays untouched.
	benchCall := provider.ToolCall{Name: "bash", Input: json.RawMessage(`{"command":"go test ./loop -count=1 -run TestRedX"}`)}
	benchOut := "=== RUN TestGutsRedPinX\n--- FAIL: TestGutsRedPinX (0.00s)\n    loop_test.go:000: boom\nFAIL\tgithub.com/Kaginari/agent-one/loop\t0.123s\nFAIL\n"
	st := &StepRecord{}
	// bench FAIL → red pin (0). res.Err=false so we exercise the output-lexicon path.
	v, why, _ := (*Guts)(nil).value(st, benchCall, tool.Result{Output: benchOut}, "")
	if v != 0 || !strings.Contains(why, "red pin") {
		t.Fatalf("bench FAIL must score 0 'red pin — failure is the signal', got v=%d why=%q", v, why)
	}
	// go vet bench failing must also pin.
	vetCall := provider.ToolCall{Name: "shell.exec", Input: json.RawMessage(`{"command":"go vet ./..."}`)}
	v2, _, _ := (*Guts)(nil).value(st, vetCall, tool.Result{Output: "# x\nvet.go:1: FAIL\n"}, "")
	if v2 != 0 {
		t.Fatalf("go vet FAIL must score 0 red pin, got v=%d", v2)
	}
	// echo arm unchanged: same bench repeated still penalized by the bench echo
	// (gutsNote same-bench rerun penalty). We assert the key identity collapses the
	// same bash command — a second identical gutsValue call must not be lifted by red-pin.
	if gutsBenchKey(benchCall) != gutsBenchKey(benchCall) {
		t.Fatal("same bench must key identically")
	}
	// non-test tool with FAIL output still eats the negative lexicon (−1).
	catCall := provider.ToolCall{Name: "read", Input: json.RawMessage(`{"path":"loop.go"}`)}
	v3, _, _ := (*Guts)(nil).value(st, catCall, tool.Result{Output: "FAILED\n"}, "")
	if v3 == 0 {
		t.Fatalf("non-test tool FAIL output must still score −1 via lexicon, got v=%d", v3)
	}
}

func TestGutsEchoAndWitnessAdmissibility(t *testing.T) {
	e := engine(t, nil)
	e.Guts = &Guts{Window: 3, Floor: -2}
	e.Budget.Retries = 5
	e.ClosingCheck = true // the witness admissibility check lives on the close
	// echo penalty: three clean re-runs of the same read bench (look v1/v2/v3) speak up
	echo := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "echo render v1"}),
		mock.Call("2", "bash", map[string]string{"command": "echo render v2"}),
		mock.Call("3", "bash", map[string]string{"command": "echo render v3"}),
		mock.Text("four renders agree"), mock.Text("five agree"),
	)
	e.Provider = echo
	r, _ := e.Run(context.Background(), "just look")
	echoWhy := false
	for _, x := range events(t, e, r.RunID) {
		if x["t"] == "guts" {
			if s, ok := x["why"].(string); ok && strings.Contains(s, "same bench re-run") {
				echoWhy = true
			}
			if s, ok := x["why"].(string); ok && x["spoke"] == true && strings.Contains(s, "same bench re-run") {
				return // penalized AND spoken
			}
		}
	}
	if !echoWhy {
		t.Fatal("the echo class must journal its why even below the speak floor")
	}
	// iterations that WRITE between looks are not echoes
	work := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "echo render"}),
		mock.Call("2", "write", map[string]string{"path": "r.py", "content": "x"}),
		mock.Call("3", "bash", map[string]string{"command": "echo render"}),
		mock.Call("4", "write", map[string]string{"path": "r.py", "content": "y"}),
		mock.Call("5", "bash", map[string]string{"command": "echo render"}),
		mock.Text("fixed"), mock.Text("closed"),
	)
	e.Provider = work
	r, _ = e.Run(context.Background(), "fix the renderer")
	for _, x := range events(t, e, r.RunID) {
		if x["t"] == "guts" {
			if s, ok := x["why"].(string); ok && strings.Contains(s, "same bench re-run") {
				t.Fatalf("debug iterations that write between looks are not echoes: %v", x)
			}
		}
	}
	// the witness close carries the independence demand
	a3 := mock.New(mock.Call("1", "write", map[string]string{"path": "n.txt", "content": "x"}), mock.Text("done"), mock.Text("closed"))
	e.Provider = a3
	_, _ = e.Run(context.Background(), "write a note")
	a3ask := ""
	for _, req := range a3.Requests {
		if m := req.Messages[len(req.Messages)-1]; strings.Contains(m.Text, "closing check") {
			a3ask = m.Text
		}
	}
	if !strings.Contains(a3ask, "independent second method") || !strings.Contains(strings.ToLower(a3ask), "name the forest") {
		t.Fatalf("the close must demand independence and object-first reading, got %q", a3ask)
	}
}

func TestClosingCheckCarriesOpenPlan(t *testing.T) {
	// an open deck item rides the re-ask, and a stubborn close names the hole on the report.
	m := mock.New(mock.Text("wrapping up"), mock.Text("really wrapping up"))
	e := engine(t, m)
	e.ClosingCheck = true
	e.PlanOpen = func() string { return "b3 is pending — merge the commits" }
	r, _ := e.Run(context.Background(), "x")
	if r.Status != Done || len(m.Requests) != 2 {
		t.Fatalf("%+v requests=%d", r, len(m.Requests))
	}
	ask := m.Requests[1].Messages[len(m.Requests[1].Messages)-1].Text
	if !strings.Contains(ask, "merge the commits") {
		t.Fatalf("the re-ask must name the open item, got %q", ask)
	}
	holes := strings.Join(r.Holes, "|")
	if !strings.Contains(holes, "closed with an open plan: b3") {
		t.Fatalf("the stubborn close must be named, got %q", holes)
	}
	// the hole is named even with the check off (the agent may still close, but the report says so)
	e.ClosingCheck = false
	e.Provider = mock.New(mock.Text("closed"))
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Done || !strings.Contains(strings.Join(r.Holes, "|"), "closed with an open plan") {
		t.Fatalf("%+v %v", r, r.Holes)
	}
}

func read(id string) provider.Response {
	return mock.Call(id, "read", map[string]string{"path": "README.md"})
}

func events(t *testing.T, e *Engine, id string) []Event {
	t.Helper()
	ev, err := ReadJournal(filepath.Join(e.JournalDir(), id+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestTurnAndJournal(t *testing.T) {
	m := mock.New(read("1"), mock.Call("2", "write", map[string]string{"path": "out.txt", "content": "x"}), mock.Text("@S DONE\n@F out.txt:1 x\n@U team c\n@E 0"))
	e := engine(t, m)
	var trace strings.Builder
	e.Trace = &trace
	r, err := e.Run(context.Background(), "do it")
	if err != nil || r.Status != Done || r.Exit() != 0 {
		t.Fatalf("%v %+v", err, r)
	}
	if len(r.Steps) != 2 || r.Steps[0].ID != "s1" || r.Steps[1].ID != "s2" || r.Steps[1].Wrote[0] != "out.txt" || r.Turns != 3 {
		t.Fatalf("steps %+v", r.Steps)
	}
	if !r.IsWire || r.Report.Unsaid[0].Text != "c" || len(r.Holes) != 0 {
		t.Fatalf("report %+v %v", r.Report, r.Holes)
	}
	if r.Context.Zone != instrument.OK || r.Usage.Output == 0 {
		t.Fatalf("readings %+v %+v", r.Context, r.Usage)
	}
	if !strings.Contains(trace.String(), "→ s1 read [read] README.md") || !strings.Contains(trace.String(), "→ s2 write [write] out.txt") {
		t.Fatalf("trace %q", trace.String())
	}
	ev := events(t, e, r.RunID)
	var ts []string
	for _, x := range ev {
		ts = append(ts, x["t"].(string))
	}
	seq := strings.Join(ts, ",")
	if !strings.HasPrefix(seq, "run,turn,perceive,gate,act,act,verify,record,turn,perceive,gate,act,act,verify,record,turn,end") {
		t.Fatalf("journal beats: %s", seq)
	}
	if ev[0]["as"] != "zone-test" || ev[0]["ask"] != "do it" || ev[0]["provider"] != "mock" {
		t.Fatalf("run line %v", ev[0])
	}
	st := State("", ev)
	if st.Status != "DONE" || st.Turns != 3 || len(st.Steps) != 2 || st.Steps[1].Tool != "write" || st.Steps[1].Class != "write" {
		t.Fatalf("state %+v", st)
	}
	runs, _ := Runs(e.JournalDir())
	if len(runs) != 1 || runs[0].ID != r.RunID {
		t.Fatalf("runs %+v", runs)
	}
	// conversation shape sent to the provider
	last := m.Requests[2].Messages
	if len(last) != 5 || last[2].ToolResults[0].ID != "1" || last[4].ToolResults[0].ID != "2" || !strings.Contains(last[2].ToolResults[0].Content, "hello workspace") {
		t.Fatalf("messages %+v", last)
	}
	// the session speaks plain language to the human; only a dispatched body answers on the wire
	if sys := m.Requests[0].System; !strings.Contains(sys, "zone-test") || !strings.Contains(sys, "plain language") || strings.Contains(sys, "@U <") {
		t.Fatalf("system %q", sys)
	}
	if court := (&Engine{Wire: true}).DefaultSystem(&Session{}); !strings.Contains(court, "@U") || strings.Contains(court, "plain language") {
		t.Fatalf("a Court's system %q", court)
	}
}

func TestDenialStopsTheTurn(t *testing.T) {
	m := mock.New(mock.Calls(provider.ToolCall{ID: "1", Name: "bash", Input: json.RawMessage(`{"command":"git push"}`)}, provider.ToolCall{ID: "2", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}), mock.Text("never"))
	e := engine(t, m)
	r, _ := e.Run(context.Background(), "publish")
	if r.Status != Denied || r.Exit() != 4 || len(r.Steps) != 1 || r.Steps[0].Status != "denied" || len(m.Requests) != 1 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.Holes[0], "s1 denied at the gate (outward: git ↔ remote") {
		t.Fatalf("hole %v", r.Holes)
	}
	if _, err := os.Stat(filepath.Join(e.JournalDir(), r.RunID+".transcript.json")); err != nil {
		t.Fatal("transcript kept for resume")
	}
	// resume: the open calls are answered as interrupted, the model goes on
	m2 := mock.New(mock.Text("@S DONE\n@U policy x\n@E 0"))
	e.Provider = m2
	r2, err := e.Resume(context.Background(), r.RunID, "skip it")
	if err != nil || r2.Status != Done || r2.RunID != r.RunID {
		t.Fatalf("%v %+v", err, r2)
	}
	msgs := m2.Requests[0].Messages
	if len(msgs) != 3 || len(msgs[2].ToolResults) != 2 || !strings.Contains(msgs[2].Text, "skip it") {
		t.Fatalf("resumed conversation %+v", msgs)
	}
	if _, err := e.Resume(context.Background(), "no-such-run", ""); err == nil {
		t.Fatal("missing transcript is an error")
	}
}

func TestPreApprovalAndDryRun(t *testing.T) {
	m := mock.New(mock.Call("1", "bash", map[string]string{"command": "echo hi", "class": "outward"}), mock.Text("@S DONE\n@U policy x\n@E 0"))
	e := engine(t, m)
	e.Gate.Approve, _ = gate.ParseApprove("outward")
	r, _ := e.Run(context.Background(), "x")
	if r.Status != Done || r.Steps[0].Gate.By != "pre-approved" || r.Steps[0].Effective != "outward" {
		t.Fatalf("%+v", r.Steps[0])
	}
	e.Gate.Approve = nil
	e.Gate.DryRun = true
	m = mock.New(mock.Calls(
		provider.ToolCall{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)},
		provider.ToolCall{ID: "2", Name: "write", Input: json.RawMessage(`{"path":"w.txt","content":"x"}`)},
		provider.ToolCall{ID: "3", Name: "bash", Input: json.RawMessage(`{"command":"rm -rf x"}`)},
	), mock.Text("@S DRY\n@U policy x\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Dry || r.Steps[0].Status != "done" || r.Steps[1].Status != "would-run" || r.Steps[2].Status != "would-ask" || r.Journal != "" {
		t.Fatalf("%+v journal=%q", r.Steps, r.Journal)
	}
	if _, err := os.Stat(filepath.Join(e.Root, "w.txt")); err == nil {
		t.Fatal("dry run must not write")
	}
	if runs, _ := Runs(e.JournalDir()); len(runs) != 1 {
		t.Fatalf("dry run must not journal: %d journals", len(runs))
	}
}

func TestBudgetsAreReadings(t *testing.T) {
	m := mock.New(mock.Calls(provider.ToolCall{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}, provider.ToolCall{ID: "2", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}), mock.Text("x"))
	e := engine(t, m)
	e.Budget.Steps = 1
	r, _ := e.Run(context.Background(), "x")
	if r.Status != Checkpoint || r.Exit() != 5 || !strings.Contains(r.Holes[0], "step budget reached (1) before s2") || !strings.Contains(r.Holes[0], "agent-one resume "+r.RunID) {
		t.Fatalf("%+v", r)
	}
	e.Budget.Steps = 0
	e.Budget.Minutes = 0.000001
	e.Provider = mock.New(read("1"), mock.Text("x"))
	time.Sleep(time.Millisecond)
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Checkpoint || !strings.Contains(r.Holes[0], "wall-clock budget") {
		t.Fatalf("%+v", r)
	}
	e.Budget.Minutes = -1
	e.Budget.Retries = 1
	e.Provider = mock.New(mock.Call("1", "bash", map[string]string{"command": "false"}), mock.Call("2", "bash", map[string]string{"command": "false"}), mock.Text("x"))
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Escalate || r.Exit() != 3 || len(r.Steps) != 2 || !strings.Contains(r.Holes[0], "2 consecutive failed acts") {
		t.Fatalf("%+v", r)
	}
	// a success resets the failure count
	e.Provider = mock.New(mock.Call("1", "bash", map[string]string{"command": "false"}), read("2"), mock.Call("3", "bash", map[string]string{"command": "false"}), mock.Text("@S DONE\n@U policy x\n@E 0"))
	if r, _ = e.Run(context.Background(), "x"); r.Status != Done {
		t.Fatalf("%+v", r)
	}
	// context stress
	e.Provider = mock.New(provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}}}, Usage: provider.Usage{Input: 200000}}, mock.Text("x"))
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Checkpoint || !strings.Contains(r.Holes[0], "stress zone (200000 ≥ 180000") {
		t.Fatalf("%+v", r)
	}
	ev := events(t, e, r.RunID)
	found := false
	for _, x := range ev {
		if x["t"] == "budget" {
			found = true
		}
	}
	if !found {
		t.Fatal("budget line journaled")
	}
	// a Perceive hook overrides the reading
	e.Hooks.Perceive = func(s *Session) *instrument.Context {
		c := instrument.Budget{}.Reading(provider.Usage{Input: 10})
		return &c
	}
	e.Provider = mock.New(provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}}}, Usage: provider.Usage{Input: 200000}}, mock.Text("@S DONE\n@U policy x\n@E 0"))
	if r, _ = e.Run(context.Background(), "x"); r.Status != Done {
		t.Fatalf("perceive hook: %+v", r)
	}
	// a Drain hook may continue past the stress line
	e.Hooks.Perceive = nil
	drained := 0
	e.Hooks.Drain = func(ctx context.Context, s *Session, c instrument.Context) (bool, error) {
		drained++
		s.Last = provider.Usage{Input: 1}
		return true, nil
	}
	e.Provider = mock.New(provider.Response{Message: provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read", Input: json.RawMessage(`{"path":"README.md"}`)}}}, Usage: provider.Usage{Input: 200000}}, mock.Text("@S DONE\n@U policy x\n@E 0"))
	if r, _ = e.Run(context.Background(), "x"); r.Status != Done || drained != 1 {
		t.Fatalf("drain hook: %+v %d", r, drained)
	}
}

func TestHooks(t *testing.T) {
	m := mock.New(mock.Call("1", "write", map[string]string{"path": "a.txt", "content": "x"}), mock.Text("@S DONE\n@U policy x\n@E 0"))
	e := engine(t, m)
	var recorded []string
	e.Hooks.System = func(s *Session) string { return "CUSTOM SYSTEM" }
	e.Hooks.Recall = func(ctx context.Context, s *Session, ask string) Recall {
		return Recall{Anchors: []string{"README.md#top"}, Tools: []string{"@T skill x — p — load≈5tok"}, Holes: []string{"index stale"}, Rebuilt: true}
	}
	e.Hooks.Record = func(ctx context.Context, s *Session, st *StepRecord) []string {
		recorded = append(recorded, st.ID)
		return []string{"note failed"}
	}
	e.Hooks.EndGate = func(ctx context.Context, s *Session, r *Result) (string, []string, error) {
		return "pass", []string{"doc not touched"}, nil
	}
	r, _ := e.Run(context.Background(), "x")
	if r.Status != Done || r.Verdict != "pass" || len(recorded) != 1 {
		t.Fatalf("%+v %v", r, recorded)
	}
	holes := strings.Join(r.Holes, "|")
	for _, want := range []string{"recall: index stale", "s1 record: note failed", "doc not touched"} {
		if !strings.Contains(holes, want) {
			t.Fatalf("missing %q in %q", want, holes)
		}
	}
	sys := m.Requests[0].SystemText()
	if !strings.HasPrefix(sys, "CUSTOM SYSTEM") || !strings.Contains(sys, "@RECALL README.md#top") || !strings.Contains(sys, "@TOOLS\n@T skill x") {
		t.Fatalf("system %q", sys)
	}
	// policy refusal
	e.Hooks = Hooks{}
	e.Policy = func(a tool.Access) error {
		if a.Class >= tool.Write {
			return errorf("ownership")
		}
		return nil
	}
	m = mock.New(mock.Call("1", "write", map[string]string{"path": "b.txt", "content": "x"}), mock.Text("@S DONE\n@U policy x\n@E 0"))
	e.Provider = m
	r, _ = e.Run(context.Background(), "x")
	if r.Status != Done || r.Steps[0].Status != "refused" || !m.Requests[1].Messages[2].ToolResults[0].IsError {
		t.Fatalf("%+v", r.Steps[0])
	}
	// an EndGate error fails the turn
	e.Policy = nil
	e.Hooks.EndGate = func(ctx context.Context, s *Session, r *Result) (string, []string, error) {
		return "fail", nil, errorf("invariants broken")
	}
	e.Provider = mock.New(mock.Call("1", "write", map[string]string{"path": "c.txt", "content": "x"}), mock.Text("@S DONE\n@U policy x\n@E 0"))
	if r, _ = e.Run(context.Background(), "x"); r.Status != Fail || r.Exit() != 2 || !strings.Contains(r.Holes[0], "invariants broken") {
		t.Fatalf("%+v", r)
	}
}

func TestUnsaidAndEmit(t *testing.T) {
	e := engine(t, mock.New(mock.Text("@S DONE\n@F a:1 f\n@E 0")))
	r, _ := e.Run(context.Background(), "x")
	if len(r.Holes) != 1 || !strings.Contains(r.Holes[0], "no @U") {
		t.Fatalf("%v", r.Holes)
	}
	out := r.Emit(0)
	if !strings.HasPrefix(out, "@S DONE\n@? report carries no @U") || !strings.HasSuffix(out, "@E "+itoa(len(out))) {
		t.Fatalf("%q", out)
	}
	e.Unsaid = false
	e.Provider = mock.New(mock.Text("plain prose answer"))
	r, _ = e.Run(context.Background(), "x")
	if r.IsWire || len(r.Holes) != 0 || r.Emit(0) != "plain prose answer" {
		t.Fatalf("%+v", r)
	}
	e.Provider = mock.New(mock.Call("1", "bash", map[string]string{"command": "git push"}))
	r, _ = e.Run(context.Background(), "x")
	if !strings.HasPrefix(r.Emit(0), "@? s1 denied") {
		t.Fatalf("a denied turn with no wire text still names the hole: %q", r.Emit(0))
	}
	// provider error → FAIL
	e.Provider = mock.New()
	if r, _ = e.Run(context.Background(), "x"); r.Status != Fail || !strings.Contains(r.Holes[0], "exhausted") {
		t.Fatalf("%+v", r)
	}
	// journaling off and lexicon
	e.Journal = "-"
	e.Lexicon = Lexicon{Agent: "worker", Human: "the operator", WorkspaceDir: ".agent-one"}
	e.As = ""
	e.Provider = mock.New(mock.Text("ok"))
	m := e.Provider.(*mock.Provider)
	r, _ = e.Run(context.Background(), "x")
	if r.Journal != "" || e.JournalDir() != "" || !strings.Contains(m.Requests[0].System, "You are worker") || !strings.Contains(m.Requests[0].System, "the operator") {
		t.Fatalf("lexicon/journal: %q %q", r.Journal, m.Requests[0].System)
	}
	if _, err := e.NewSession().Turn(context.Background(), "  "); err == nil {
		t.Fatal("empty ask")
	}
}

func TestSessionAcrossTurns(t *testing.T) {
	m := mock.New(mock.Text("one"), read("1"), mock.Text("two"))
	e := engine(t, m)
	e.Unsaid = false
	s := e.NewSession()
	r1, _ := s.Turn(context.Background(), "first")
	r2, _ := s.Turn(context.Background(), "second")
	if r1.Text != "one" || r2.Text != "two" || r2.RunID != r1.RunID || len(s.Messages) != 6 || s.Turns != 3 || s.Steps != 1 {
		t.Fatalf("%+v %+v %d", r1, r2, len(s.Messages))
	}
	ev := events(t, e, s.RunID)
	runs, ends := 0, 0
	for _, x := range ev {
		switch x["t"] {
		case "run":
			runs++
		case "end":
			ends++
		}
	}
	if runs != 1 || ends != 2 {
		t.Fatalf("one run line, one end per turn: %d %d", runs, ends)
	}
}

type errorf string

func (e errorf) Error() string { return string(e) }

func itoa(n int) string { return fmt.Sprint(n) }

// A write that reaches disk through the shell — a redirect the classifier reads as write, and a
// `xargs touch` it reads as read-only — must reach the end-of-turn gate like a `write` call would;
// the workspace dir's own disposable paths (instruments, tmp) are not writes the gate sees.
func TestShellWritesReachTheGate(t *testing.T) {
	m := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "printf 'x\\n' > out.txt && mkdir -p .agent-one/instruments/x .agent-one/tmp && echo j > .agent-one/instruments/x/j.jsonl && echo t > .agent-one/tmp/t"}),
		mock.Call("2", "bash", map[string]string{"command": "echo two.txt | xargs touch"}),
		mock.Text("@S DONE\n@U team c\n@E 0"))
	e := engine(t, m)
	var seen []string
	calls := 0
	e.Hooks.EndGate = func(ctx context.Context, s *Session, r *Result) (string, []string, error) {
		calls++
		seen = append([]string(nil), s.Wrote...)
		return "pass", nil, nil
	}
	r, err := e.Run(context.Background(), "write through the shell")
	if err != nil || r.Status != Done {
		t.Fatalf("%v %+v", err, r)
	}
	if calls != 1 {
		t.Fatalf("the gate ran %d times; the turn wrote through the shell (wrote=%v)", calls, r.Steps[0].Wrote)
	}
	joined := strings.Join(seen, ",")
	if !strings.Contains(joined, "out.txt") || !strings.Contains(joined, "two.txt") {
		t.Fatalf("the gate did not see the shell's writes: %v", seen)
	}
	if strings.Contains(joined, "instruments") || strings.Contains(joined, ".agent-one/tmp") {
		t.Fatalf("disposable workspace-dir paths counted as writes: %v", seen)
	}
	if r.Steps[0].Class.Class != tool.Write || len(r.Steps[0].Wrote) != 1 || r.Steps[0].Wrote[0] != "out.txt" {
		t.Fatalf("step 1 should carry its own write: %+v", r.Steps[0])
	}
	if r.Verdict != "pass" || len(r.Wrote) != 2 {
		t.Fatalf("result: verdict %q wrote %v", r.Verdict, r.Wrote)
	}
	ev := events(t, e, r.RunID)
	found := false
	for _, x := range ev {
		if x["t"] == "gate" && x["id"] == "turn" {
			found = strings.Contains(fmt.Sprint(x["wrote"]), "two.txt")
		}
	}
	if !found {
		t.Fatalf("the journal's end gate line does not carry the shell's writes")
	}
}

// A turn's writes are gated once: the next turn of the same session gates only its own.
func TestGateOncePerTurn(t *testing.T) {
	m := mock.New(mock.Call("1", "write", map[string]string{"path": "out.txt", "content": "x"}), mock.Text("one"), mock.Text("two"),
		mock.Call("2", "write", map[string]string{"path": "again.txt", "content": "y"}), mock.Text("three"))
	e := engine(t, m)
	e.Unsaid = false
	var gated [][]string
	e.Hooks.EndGate = func(ctx context.Context, s *Session, r *Result) (string, []string, error) {
		gated = append(gated, append([]string(nil), s.Wrote...))
		return "pass", nil, nil
	}
	s := e.NewSession()
	if r, _ := s.Turn(context.Background(), "first"); r.Verdict != "pass" {
		t.Fatalf("first turn: %+v", r)
	}
	if r, _ := s.Turn(context.Background(), "second"); r.Verdict != "" {
		t.Fatalf("second turn wrote nothing yet was gated: %+v", r)
	}
	if r, _ := s.Turn(context.Background(), "third"); r.Verdict != "pass" {
		t.Fatalf("third turn: %+v", r)
	}
	if len(gated) != 2 || strings.Join(gated[0], ",") != "out.txt" || strings.Join(gated[1], ",") != "again.txt" {
		t.Fatalf("gated writes per turn: %v", gated)
	}
}

// A wire engine that answers off the wire is re-asked once, coded: the violation's text is the
// model's next message and the retry's answer is the report. The policy that a dispatched agent
// answers @S…@E is enforced by the loop, never begged by a prompt. Red until finish wires it.
func TestWireAnswerRetriedOnce(t *testing.T) {
	m := mock.New(mock.Text("yes"), mock.Text("@S DONE\n@F loop.go:1 retry applied\n@U team asked\n@E 1"))
	e := engine(t, m)
	e.Wire = true // a Subagent agent answers on the wire
	r, err := e.Run(context.Background(), "say yes on the wire")
	if err != nil || r.Status != Done {
		t.Fatalf("%v %+v", err, r)
	}
	if !r.IsWire {
		t.Fatalf("the retry's wire answer is the report: %+v", r.Report)
	}
	if len(m.Requests) != 2 {
		t.Fatalf("one retry, two requests, got %d", len(m.Requests))
	}
	last := m.Requests[1].Messages[len(m.Requests[1].Messages)-1]
	if last.Role != provider.User || !strings.Contains(last.Text, "wire") || !strings.Contains(last.Text, "@S") {
		t.Fatalf("the re-ask names the wire violation: %q", last.Text)
	}
	if r.Turns != 2 {
		t.Fatalf("retry is a second turn: %d", r.Turns)
	}
}

// The re-ask is bounded: one retry, then the agent stands undisciplined and the hole is the
// record. No loop of begging.
func TestWireAnswerRetryBounded(t *testing.T) {
	m := mock.New(mock.Text("yes"), mock.Text("still plain"))
	e := engine(t, m)
	e.Wire = true
	r, err := e.Run(context.Background(), "say yes on the wire")
	if err != nil || r.Status != Done {
		t.Fatalf("%v %+v", err, r)
	}
	if r.IsWire {
		t.Fatal("an agent that never speaks the wire ends off it")
	}
	if len(m.Requests) != 2 {
		t.Fatalf("exactly one retry: %d requests", len(m.Requests))
	}
	joined := strings.Join(r.Holes, "\n")
	if !strings.Contains(joined, "@S") {
		t.Fatalf("the hole records the wire violation: %v", r.Holes)
	}
}

// The context the run reports is the reading the loop acts on: a Perceive hook that lowers the
// stress line (the drain scaling to a small model's window) is what Result.Context shows.
func TestReportedContextIsThePerceivedReading(t *testing.T) {
	m := mock.New(read("1"), mock.Text("done"))
	e := engine(t, m)
	e.Unsaid = false
	e.Hooks.Perceive = func(s *Session) *instrument.Context {
		if s.Turns == 0 {
			return nil
		}
		c := s.Engine.Budget.Context.Reading(s.Last)
		c.Stress = 27852
		return &c
	}
	r, err := e.Run(context.Background(), "x")
	if err != nil || r.Status != Done {
		t.Fatalf("%v %+v", err, r)
	}
	if r.Context.Stress != 27852 {
		t.Fatalf("reported stress %d is the policy's line, not the perceived one", r.Context.Stress)
	}
}

// A native call whose arguments did not parse keeps its tool's name; it is named malformed to the
// model and never run with an input the tool would misread.
func TestMalformedNativeCallIsNamed(t *testing.T) {
	m := mock.New(mock.Calls(provider.ToolCall{ID: "1", Name: "read", Input: json.RawMessage(`{"_malformed":"{\"path\":\"READ"}`)}), mock.Text("@S DONE\n@U policy x\n@E 0"))
	e := engine(t, m)
	r, _ := e.Run(context.Background(), "read it")
	if len(r.Steps) != 1 || r.Steps[0].Status != "failed" {
		t.Fatalf("steps %+v", r.Steps)
	}
	res := m.Requests[1].Messages[2].ToolResults[0].Content
	if !strings.Contains(res, "malformed tool call") || !strings.Contains(res, "read") || strings.Contains(res, "path is required") {
		t.Fatalf("the model was told %q", res)
	}
}

// A turn that stops early still records what its read-classified commands wrote: the stopped
// turn's transcript carries it to the resume and the gate.
func TestStoppedTurnKeepsItsWrites(t *testing.T) {
	m := mock.New(mock.Calls(provider.ToolCall{ID: "1", Name: "bash", Input: json.RawMessage(`{"command":"echo f.txt | xargs touch"}`)}),
		mock.Calls(provider.ToolCall{ID: "2", Name: "bash", Input: json.RawMessage(`{"command":"git push"}`)}), mock.Text("never"))
	e := engine(t, m)
	r, _ := e.Run(context.Background(), "touch then publish")
	if r.Status != Denied {
		t.Fatalf("%+v", r)
	}
	b, err := os.ReadFile(filepath.Join(e.JournalDir(), r.RunID+".transcript.json"))
	if err != nil || !strings.Contains(string(b), `"wrote":["f.txt"]`) {
		t.Fatalf("the stopped turn lost its write: %v %s", err, b)
	}
}

func TestDependencyTreesAreNotWatched(t *testing.T) {
	e := &Engine{}
	for rel, want := range map[string]bool{"node_modules": true, "web/node_modules": true, ".venv": true, "src/__pycache__": true, "src/node_modules.go": false, ".git": true} {
		if e.unwatched(rel) != want {
			t.Errorf("%s: want unwatched=%v", rel, want)
		}
	}
}

func TestDoomLoopReliefLadder(t *testing.T) {
	// the guts speak every window; Doom=1 checkpoints the run on the first sustained slide,
	// carrying the three relief arms for the dispatcher (Operator 2026-10-02 evening order)
	fails := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "false"}),
		mock.Call("2", "bash", map[string]string{"command": "false"}),
		mock.Call("3", "bash", map[string]string{"command": "false"}),
		mock.Call("4", "bash", map[string]string{"command": "false"}),
		mock.Text("never reached"),
	)
	e := engine(t, fails)
	e.Guts = &Guts{Window: 3, Floor: -1}
	e.Budget.Retries = 9 // keep the slide scored, not escalated — the ladder, not fatigue, ends it
	e.Doom = 1
	r, _ := e.Run(context.Background(), "x")
	if r.Status != Checkpoint {
		t.Fatalf("doomLoop relief must checkpoint, got %+v", r)
	}
	hole := strings.Join(r.Holes, " ")
	if !strings.Contains(hole, "doomLoop") || !strings.Contains(hole, "split") || !strings.Contains(hole, "configuration-shift") {
		t.Fatalf("relief arms must name retire/split/configuration-shift in the holes: %v", r.Holes)
	}
	doomSeen := false
	for _, x := range events(t, e, r.RunID) {
		if x["t"] == "budget" && x["why"] == "doomLoop:guts" {
			doomSeen = true
		}
	}
	if !doomSeen {
		t.Fatal("the journal must carry the doomLoop budget event")
	}
	// the relieved session must be disarmed: Doom 1 on one session; after the checkpoint its
	// gutsSpoke streak and the window that carried it are spent. Pre-fix the turn reignited on
	// any later floor window — the workspace looked alive but could not run a full turn.
	e2 := engine(t, mock.New(
		mock.Call("1", "bash", map[string]string{"command": "false"}),
		mock.Call("2", "bash", map[string]string{"command": "false"}),
		mock.Call("3", "bash", map[string]string{"command": "false"}),
		mock.Call("4", "bash", map[string]string{"command": "false"}),
		mock.Call("5", "bash", map[string]string{"command": "false"}),
		mock.Call("6", "bash", map[string]string{"command": "false"}),
		mock.Text("end"),
	))
	e2.Guts = &Guts{Window: 3, Floor: -1}
	e2.Budget.Retries = 9
	e2.Doom = 1
	s2 := e2.NewSession()
	r1, _ := s2.Turn(context.Background(), "burn")
	if r1.Status != Checkpoint {
		t.Fatalf("first turn must doom-checkpoint on the slide, got %+v", r1)
	}
	if s2.gutsSpoke != 0 || len(s2.gutsVals) != 0 {
		t.Fatalf("the relief left the doom armed (spoke streak=%d window=%v)", s2.gutsSpoke, s2.gutsVals)
	}
	r2, _ := s2.Turn(context.Background(), "still sliding")
	if r2.Status != Checkpoint {
		t.Fatalf("an honestly still-sliding turn must earn its doom again, got %+v", r2)
	}
	// Doom 0: nothing relieves (existing behavior — slide scored, run walks on)
	fails2 := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "false"}),
		mock.Call("2", "bash", map[string]string{"command": "false"}),
		mock.Call("3", "bash", map[string]string{"command": "false"}),
		mock.Text("end"),
	)
	e.Provider = fails2
	e.Doom = 0
	r, _ = e.Run(context.Background(), "y")
	if r.Status == Checkpoint && strings.Contains(strings.Join(r.Holes, " "), "doomLoop") {
		t.Fatalf("Doom 0 must keep the loop walking: %+v", r)
	}
}

func TestUnsaidBouncedOnce(t *testing.T) {
	// wire-shaped but missing @U on a +unsaid run → one bounce; landing @U closes clean
	m := mock.New(mock.Text("@S DONE\n@F board/tree.go:1 landed\n@E 42"), mock.Text("@S DONE\n@F board/tree.go:1 landed\n@U team pairing verified\n@E 42"))
	e := engine(t, m)
	e.Wire = true
	r, err := e.Run(context.Background(), "witness (+unsaid)")
	if err != nil || r.Status != Done {
		t.Fatalf("%v %+v", err, r)
	}
	if len(m.Requests) != 2 {
		t.Fatalf("one @U bounce, two requests, got %d", len(m.Requests))
	}
	joined := strings.Join(r.Holes, "\n")
	if !strings.Contains(joined, "no @U") || !strings.Contains(joined, "sent back") {
		t.Fatalf("the bounce hole is the record of the strike — kept, not erased: %v", r.Holes)
	}
	last := m.Requests[1].Messages[len(m.Requests[1].Messages)-1]
	if !strings.Contains(last.Text, "@U") {
		t.Fatalf("the re-ask names the missing wire clause: %q", last.Text)
	}
}

func TestUnsaidBounceBounded(t *testing.T) {
	// never names @U twice → hole stands as the record
	m := mock.New(mock.Text("@S DONE\n@E 42"), mock.Text("@S DONE\n@E 42"))
	e := engine(t, m)
	e.Wire = true
	r, _ := e.Run(context.Background(), "witness (+unsaid)")
	joined := strings.Join(r.Holes, "\n")
	if !strings.Contains(joined, "no @U") {
		t.Fatalf("the hole records the missing @U: %v", r.Holes)
	}
	if len(m.Requests) != 2 {
		t.Fatalf("exactly one @U bounce: %d", len(m.Requests))
	}
}

// A gate cut short by its context judged nothing: the writes it was handed are not marked gated,
// so they carry into the next turn's gate instead of being dropped unjudged.
func TestCancelledGateCarriesTheWrites(t *testing.T) {
	m := mock.New(
		mock.Call("1", "write", map[string]string{"path": "a.txt", "content": "x"}),
		mock.Text("@S DONE\n@U team c\n@E 0"),
		mock.Text("@S DONE\n@U team c\n@E 0"),
	)
	e := engine(t, m)
	ctx, cancel := context.WithCancel(context.Background())
	var gated [][]string
	e.Hooks.EndGate = func(gctx context.Context, s *Session, r *Result) (string, []string, error) {
		gated = append(gated, append([]string(nil), s.Wrote...))
		if len(gated) == 1 {
			cancel() // the run is stopped while its gate reads
			return "cancelled", nil, fmt.Errorf("gate cancelled — %w", gctx.Err())
		}
		return "pass", nil, nil
	}
	s := e.NewSession()
	if r, _ := s.Turn(ctx, "write a"); r == nil || r.Status == Done {
		t.Fatalf("a cancelled gate does not close the turn done: %+v", r)
	}
	if r, err := s.Turn(context.Background(), "carry on"); err != nil || r.Status != Done {
		t.Fatalf("%v %+v", err, r)
	}
	if len(gated) != 2 || strings.Join(gated[1], ",") != "a.txt" {
		t.Fatalf("the unjudged write must reach the next turn's gate: %v", gated)
	}
}
