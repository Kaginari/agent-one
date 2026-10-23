package workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider/mock"
	"github.com/Kaginari/agent-one/tool"
)

func TestBindSubagentRefusesDuplicate(t *testing.T) {
	w := open(t, fixture(t))
	first := w.Engine("zone-auth", build(mock.New()))
	if held := w.seat("zone-auth", first); held != "" {
		t.Fatalf("an empty seat is refused: %s", held)
	}
	if held := w.seat("zone-auth", first); held != "" {
		t.Fatalf("a subagent re-seating itself is refused: %s", held)
	}
	pb := build(mock.New())
	parent := w.Engine("domain-security", pb)
	_, err := w.Dispatcher(parent, pb)(context.Background(), tool.Env{Root: w.Root}, tool.Commission{Agent: "zone-auth", Ask: "check the tokens"})
	if err == nil || !strings.Contains(err.Error(), "already sits a live subagent") {
		t.Fatalf("a second subagent on a live seat: %v", err)
	}
	if held := w.seat("zone-api", w.Engine("zone-api", build(mock.New()))); held != "" {
		t.Fatalf("one agent's seat holds another's: %s", held)
	}
	w.Close(first)
	if held := w.seat("zone-auth", w.Engine("zone-auth", build(mock.New()))); held != "" {
		t.Fatalf("Close did not give the seat up: %s", held)
	}
}

func TestSubagentDeathShape(t *testing.T) {
	steps := func(n int) []*loop.StepRecord {
		var out []*loop.StepRecord
		for i := 0; i < n; i++ {
			out = append(out, &loop.StepRecord{Status: "done"})
		}
		return out
	}
	for _, c := range []struct {
		r    *loop.Result
		want string
	}{
		{nil, "provider hang"},
		{&loop.Result{Turns: 1, Steps: []*loop.StepRecord{{Status: "pending"}}}, "provider hang"},
		{&loop.Result{Turns: 60, Steps: steps(80)}, "a recon loop"},
		{&loop.Result{Turns: 30, Steps: steps(40), Wrote: []string{"a.go", "b.go"}}, "40 acts, 2 writes"},
	} {
		if got := subagentDeathShape(c.r, "openai/m", "openai", 12); !strings.Contains(got, c.want) {
			t.Errorf("%+v: %q, want %q", c.r, got, c.want)
		}
	}
}

func TestReportNamedForItsRun(t *testing.T) {
	w := open(t, fixture(t))
	a, _, _ := w.fileReport("zone-auth-20261023150405-ab12", "first")
	b, _, _ := w.fileReport("zone-auth-20261023030405-cd34", "second")
	if a == b || !strings.HasSuffix(a, "zone-auth-20261023150405-ab12.report.md") {
		t.Fatalf("15:04 and 03:04 share a name, or the run is lost: %s %s", a, b)
	}
}
