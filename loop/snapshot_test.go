package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
	"github.com/Kaginari/agent-one/tool"
)

func TestSnapshotSkipsHeavyDirs(t *testing.T) {
	e := engine(t, mock.New(mock.Text("x")))
	plant := func(rel string) {
		t.Helper()
		p := filepath.Join(e.Root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// dependency / build forests an agent never authors
	plant("app/node_modules/pkg/index.js")
	plant("svc/.venv/lib/python/os.py")
	plant("crate/target/debug/build.bin")
	plant("web/dist/bundle.js")
	// the authored surface that must stay watched
	plant("app/index.js")
	plant("svc/main.py")

	snap, why := e.stampTree()
	if why != "" {
		t.Fatal(why)
	}
	for rel := range snap {
		if strings.Contains(rel, "node_modules") || strings.Contains(rel, ".venv") || strings.Contains(rel, "target") || strings.Contains(rel, "dist") {
			t.Fatalf("heavy dir walked: %s", rel)
		}
	}
	for _, want := range []string{"app/index.js", "svc/main.py", "README.md"} {
		if _, ok := snap[want]; !ok {
			t.Fatalf("authored file missing from stamp: %s", want)
		}
	}
}

func TestUnwatchedNamesHeavyDirsAtAnyDepth(t *testing.T) {
	e := engine(t, mock.New(mock.Text("x")))
	for _, rel := range []string{"node_modules", "a/b/node_modules", ".venv", "x/__pycache__", "target", "y/dist", ".agent-one/memory"} {
		if !e.unwatched(rel) {
			t.Fatalf("%s must be unwatched", rel)
		}
	}
	for _, rel := range []string{"vendor", "app/index.js", "targets", "distributor", "src/memory"} {
		if e.unwatched(rel) {
			t.Fatalf("%s must stay watched", rel)
		}
	}
}

// steps answers each call with the next function's response: a test's provider that acts
// between the turns it scripts.
type steps []func() provider.Response

func (st *steps) Name() string { return "fake/steps" }

func (st *steps) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	next := (*st)[0]
	*st = (*st)[1:]
	return next(), nil
}

// A subagent working beside its dispatcher changes the same tree. The dispatcher's watch sees the
// subagent's write by its stamp, but the subagent's tool holds the receipt: the dispatcher's gate
// leaves it to the subagent. A receipt older than the watcher's baseline explains nothing — the
// dispatcher's own later shell write of that file is its own.
func TestWatchPaysTheReceiptNotTheStamp(t *testing.T) {
	book := NewReceipts()
	orchestrator := engine(t, nil)
	orchestrator.Receipts = book
	subagent := &Engine{Provider: mock.New(mock.Call("1", "write", map[string]string{"path": "subagent.txt", "content": "the subagent's\n"}), mock.Text("@S DONE\n@U team c\n@E 0")),
		Tools: tool.Builtins(), Gate: orchestrator.Gate, Root: orchestrator.Root, As: "zone-subagent", Unsaid: true, Receipts: book}
	var gated [][]string
	orchestrator.Hooks.EndGate = func(ctx context.Context, s *Session, r *Result) (string, []string, error) {
		gated = append(gated, append([]string(nil), s.Wrote...))
		return "pass", nil, nil
	}
	shell := func(id, cmd string) func() provider.Response {
		return func() provider.Response { return mock.Call(id, "bash", map[string]string{"command": cmd}) }
	}
	done := func() provider.Response { return mock.Text("@S DONE\n@U team c\n@E 0") }
	orchestrator.Provider = &steps{
		func() provider.Response {
			// the subagent runs while the orchestrator's turn is open (its baseline already taken)
			if r, err := subagent.Run(context.Background(), "write subagent.txt"); err != nil || r.Status != Done {
				t.Fatalf("subagent: %v %+v", err, r)
			}
			return mock.Call("1", "bash", map[string]string{"command": "echo mine > mine.txt"})
		},
		done,
		shell("2", "echo again >> subagent.txt"),
		done,
	}
	if r, err := orchestrator.Run(context.Background(), "work beside the subagent"); err != nil || r.Status != Done {
		t.Fatalf("%v %+v", err, r)
	}
	if len(gated) != 1 || strings.Join(gated[0], ",") != "mine.txt" {
		t.Fatalf("the orchestrator's gate must judge its own write only, not the subagent's receipt: %v", gated)
	}
	if r, err := orchestrator.Run(context.Background(), "now touch it yourself"); err != nil || r.Status != Done {
		t.Fatalf("%v %+v", err, r)
	}
	if len(gated) != 2 || strings.Join(gated[1], ",") != "subagent.txt" {
		t.Fatalf("a stale receipt hid the orchestrator's own write: %v", gated)
	}
}
