package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
	"github.com/Kaginari/agent-one/tool"
)

// files lays down the files a test's reads find, so every read is a clean act.
func files(t *testing.T, e *Engine, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(e.Root, n), []byte(n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readOf(id, path string) provider.Response {
	return mock.Call(id, "read", map[string]string{"path": path})
}

func writeOf(id, path string) provider.Response {
	return mock.Call(id, "write", map[string]string{"path": path, "content": "x"})
}

func TestSignatureFoldsSpacingAndKeyOrder(t *testing.T) {
	a := signature("read", json.RawMessage(`{"path":"a","limit":5}`))
	b := signature("read", json.RawMessage(`{ "limit": 5,  "path": "a" }`))
	if a != b {
		t.Fatal("one read in two spellings reads as two")
	}
	if a == signature("grep", json.RawMessage(`{"path":"a","limit":5}`)) || a == signature("read", json.RawMessage(`{"path":"b","limit":5}`)) {
		t.Fatal("different reads share a signature")
	}
}

func TestRepinGuardCheckpoints(t *testing.T) {
	m := mock.New(read("1"), read("2"), read("3"), mock.Text("never reached"))
	e := engine(t, m)
	e.Guts = &Guts{}
	r, _ := e.Run(context.Background(), "find the bug")
	if r.Status != Checkpoint || len(r.Steps) != 3 {
		t.Fatalf("want a checkpoint at the third identical read: %+v", r)
	}
	if !strings.Contains(strings.Join(r.Holes, "\n"), "re-pin loop") {
		t.Fatalf("the hole does not name the re-pin: %v", r.Holes)
	}
	second := m.Requests[2].Messages[len(m.Requests[2].Messages)-1].ToolResults[0].Content
	if !strings.Contains(second, "[re-pin] the same read as s1, the same answer") || !strings.Contains(second, "1 more time ") {
		t.Fatalf("the second read carries no warning: %q", second)
	}
}

func TestRepinGuardResetsOnWrite(t *testing.T) {
	m := mock.New(read("1"), read("2"), writeOf("3", "a.txt"), read("4"), read("5"), mock.Text("done"))
	e := engine(t, m)
	e.Guts = &Guts{}
	r, _ := e.Run(context.Background(), "fix it")
	if r.Status != Done {
		t.Fatalf("a write between re-reads is new evidence: %+v", r)
	}
}

func TestRepinGuardLeavesTheShellAlone(t *testing.T) {
	poll := mock.Call("p", "bash", map[string]string{"command": "cat README.md"})
	m := mock.New(poll, poll, poll, poll, mock.Text("done"))
	e := engine(t, m)
	e.Guts = &Guts{}
	if r, _ := e.Run(context.Background(), "watch the log"); r.Status != Done {
		t.Fatalf("a shell poll is the gut's to price, not a pin: %+v", r)
	}
}

func TestRepinGuardOff(t *testing.T) {
	m := mock.New(read("1"), read("2"), read("3"), read("4"), mock.Text("done"))
	e := engine(t, m)
	e.Guts = &Guts{Repin: -1}
	if r, _ := e.Run(context.Background(), "x"); r.Status != Done {
		t.Fatalf("repin -1 is off: %+v", r)
	}
}

func TestActBudgetCheckpointsReadOnlyRun(t *testing.T) {
	m := mock.New(readOf("1", "a"), readOf("2", "b"), readOf("3", "c"), readOf("4", "d"), mock.Text("never reached"))
	e := engine(t, m)
	e.Guts = &Guts{ActBudget: 4}
	files(t, e, "a", "b", "c", "d")
	r, _ := e.Run(context.Background(), "survey the tree")
	if r.Status != Checkpoint || len(m.Requests) != 4 {
		t.Fatalf("want a checkpoint before the fifth model call: status %s, %d calls", r.Status, len(m.Requests))
	}
	if !strings.Contains(strings.Join(r.Holes, "\n"), "act budget: 4 turns") {
		t.Fatalf("holes %v", r.Holes)
	}
	warned := 0
	for _, q := range m.Requests {
		if last := q.Messages[len(q.Messages)-1]; strings.Contains(last.Text, "[act budget]") {
			warned++
		}
	}
	if warned != 1 {
		t.Fatalf("the three-quarter warning rode %d times, want once", warned)
	}
}

func TestActBudgetDefusedByWrite(t *testing.T) {
	m := mock.New(readOf("1", "a"), readOf("2", "b"), writeOf("3", "w.txt"), readOf("4", "c"), readOf("5", "d"), mock.Text("done"))
	e := engine(t, m)
	e.Guts = &Guts{ActBudget: 3}
	files(t, e, "a", "b", "c", "d")
	if r, _ := e.Run(context.Background(), "x"); r.Status != Done {
		t.Fatalf("a landing restarts the budget: %+v", r)
	}
}

func TestActBudgetResetsOnANewAsk(t *testing.T) {
	m := mock.New(readOf("1", "a"), readOf("2", "b"), mock.Text("first answer"), readOf("3", "c"), readOf("4", "d"), mock.Text("second answer"))
	e := engine(t, m)
	e.Guts = &Guts{ActBudget: 3}
	files(t, e, "a", "b", "c", "d")
	s := e.NewSession()
	for _, ask := range []string{"what is a", "what is c"} {
		if r, _ := s.Turn(context.Background(), ask); r.Status != Done {
			t.Fatalf("%q: a question answered by reading is no loop across asks: %+v", ask, r)
		}
	}
}

func TestBookkeepingIsNoLanding(t *testing.T) {
	s := (&Engine{Guts: &Guts{}}).NewSession()
	s.stall.idle = 7
	s.landed("workingNotes", tool.Write)
	s.landed("plan", tool.Write)
	s.landed("queue", tool.Write)
	s.landed("grep", tool.Read)
	if s.stall.idle != 7 {
		t.Fatal("working memory, queue notes and reads reset the act budget")
	}
	s.landed("dispatch", tool.Write)
	if s.stall.idle != 0 {
		t.Fatal("a dispatch is a landing")
	}
}

func TestRecallDedupe(t *testing.T) {
	rc := Recall{Anchors: []string{"a.md#1", "b.md", "a.md#1"}, Tools: []string{"@T x", "@T x"}, Holes: []string{"h", "h"}}.dedupe()
	if strings.Join(rc.Anchors, " ") != "a.md#1 b.md" || len(rc.Tools) != 1 || len(rc.Holes) != 1 {
		t.Fatalf("%+v", rc)
	}
}

// The answer is the witness: a file changed by another hand (a parallel subagent, the human's editor)
// reads anew, however often the same read is asked.
func TestRepinGuardReadsTheAnswer(t *testing.T) {
	m := mock.New(read("1"), read("2"), read("3"), read("4"), mock.Text("done"))
	e := engine(t, m)
	e.Guts = &Guts{}
	n := 0
	e.Hooks.PostTool = func(ctx context.Context, s *Session, st *StepRecord) {
		n++
		_ = os.WriteFile(filepath.Join(e.Root, "README.md"), []byte(fmt.Sprintf("edited elsewhere %d\n", n)), 0o644)
	}
	if r, _ := e.Run(context.Background(), "watch the file"); r.Status != Done {
		t.Fatalf("a file that moved outside the session is new evidence: %+v", r)
	}
}
