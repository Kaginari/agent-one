package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/workspace"
)

// TestQueueCommand: `q` reads and moves the workspace's shared queue as the human — add, claim
// for an agent, close; --json prints the items; a refusal exits 2.
func TestQueueCommand(t *testing.T) {
	w, a := forgeWorkspace(t)
	a.Close()
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := Main("agent-one", append([]string{"--root", w.root, "--quiet", "--no-board"}, args...), IO{In: strings.NewReader(""), Out: &out, Err: &errb, Env: func(k string) string {
			if k == "HOME" {
				return w.home
			}
			return ""
		}}, Version{Version: "test"})
		return code, out.String(), errb.String()
	}
	if code, out, errs := run("q", "add", "--for", "zone-auth", "--rank", "0", "tighten", "the", "ttl"); code != 0 || !strings.Contains(out, "queued q1") {
		t.Fatalf("q add: %d %q %q", code, out, errs)
	}
	if code, out, _ := run("q", "claim", "q1", "--as", "zone-auth"); code != 0 || !strings.Contains(out, "held by zone-auth") {
		t.Fatalf("q claim --as: %d %q", code, out)
	}
	if code, out, _ := run("--json", "q", "list"); code != 0 || !strings.HasPrefix(out, `[{"id":"q1"`) {
		t.Fatalf("q list --json: %d %q", code, out)
	}
	if code, out, _ := run("q", "close", "q1", "landed"); code != 0 || !strings.Contains(out, "done q1") {
		t.Fatalf("the human closes any work: %d %q", code, out)
	}
	if code, _, errs := run("q", "requeue", "q1"); code != 2 || !strings.Contains(errs, "only claimed work") {
		t.Fatalf("a refusal exits 2: %d %q", code, errs)
	}
	if b, err := os.ReadFile(filepath.Join(w.root, ".agent-one", "shared", "queue.jsonl")); err != nil || strings.Count(string(b), "\n") != 3 {
		t.Fatalf("three moves, three lines: %v %q", err, b)
	}
	if _, out, _ := run("--help"); !strings.Contains(out, "q list") {
		t.Fatalf("help lists q: %s", out)
	}
}

// TestQueueToolOnEveryShelf: the queue tool reaches a zone worker's cut shelf and the orchestrator's, and
// tools.queue.enabled: false takes it off with the origin named.
func TestQueueToolOnEveryShelf(t *testing.T) {
	_, a := forgeWorkspace(t)
	defer a.Close()
	for _, agent := range []string{"zone-auth", workspace.Orchestrator} {
		e := a.Workspace.Engine(agent, a.Build())
		if !strings.Contains(","+strings.Join(e.Tools.Names(), ",")+",", ",queue,") {
			t.Fatalf("%s's shelf lacks queue: %v", agent, e.Tools.Names())
		}
		a.Workspace.Close(e)
	}
	a.Cfg.Tools.Queue.Enabled = false
	if why := a.Shelf.Disabled()["queue"]; !strings.Contains(why, "tools.queue.enabled") {
		t.Fatalf("the switch names its origin: %q", why)
	}
}
