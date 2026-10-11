package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/loop"
)

// TestTmpGuardBashRewrite: absolute off-workspace words land inside the workspace's scratch.
func TestTmpGuardBashRewrite(t *testing.T) {
	a := testTmpApp(t, "/Users/x/ws/agent-one")
	a.Cfg.Tools.TmpRedirect.Enabled = true
	a.Cfg.Tools.TmpRedirect.Mode = "rewrite"
	g := a.tmpGuardHook()
	if g == nil {
		t.Fatal("tmpRedirect enabled")
	}
	call := func(cmd string) string {
		in, _ := json.Marshal(map[string]string{"command": cmd})
		return g(nil, "bash", in)
	}
	out := call("ls /tmp/foo.txt")
	if !strings.Contains(out, "rewrite:") || !strings.Contains(out, ".agent-one/tmp/foo.txt") {
		t.Fatalf("/tmp rewrite: %q", out)
	}
	if !strings.Contains(out, "run this instead:") || !strings.Contains(out, "ls /Users/x/ws/agent-one/.agent-one/tmp/foo.txt") {
		t.Fatalf("rewritten command: %q", out)
	}
	// a system path becomes scratch/system
	out = call("cat /var/log/syslog")
	if !strings.Contains(out, ".agent-one/tmp/system/var/log/syslog") {
		t.Fatalf("/var rewrite: %q", out)
	}
	// a root-level file becomes scratch/root
	out = call("tar czf /backup.tgz src")
	if !strings.Contains(out, ".agent-one/tmp/root/backup.tgz") {
		t.Fatalf("root rewrite: %q", out)
	}
	// relative paths and workspace-inside paths pass through
	if out := call("ls sub/dir"); out != "" {
		t.Fatalf("a relative path is untouched: %q", out)
	}
	if out := call("ls /Users/x/ws/agent-one/.agent-one/tmp/keep.txt"); out != "" {
		t.Fatalf("already inside the workspace: %q", out)
	}
	// VAR=/tmp/x assignments rewrite too
	out = call("OUT=/tmp/x.txt make")
	if !strings.Contains(out, "OUT=/Users/x/ws/agent-one/.agent-one/tmp/x.txt") {
		t.Fatalf("assignment rewrite: %q", out)
	}
	// the hint reaches the denylist: a non-bash tool name never trips
	in, _ := json.Marshal(map[string]string{"command": "ls /tmp/x"})
	if out := g(nil, "grep", in); out != "" {
		t.Fatalf("non-bash passes: %q", out)
	}
}

// TestTmpGuardRefuse: mode refuse names the scratch dir instead of rewriting.
func TestTmpGuardRefuse(t *testing.T) {
	a := testTmpApp(t, "/w/agent-one")
	a.Cfg.Tools.TmpRedirect.Enabled = true
	a.Cfg.Tools.TmpRedirect.Mode = "refuse"
	g := a.tmpGuardHook()
	in, _ := json.Marshal(map[string]string{"command": "ls /tmp/x"})
	out := g(nil, "bash", in)
	if !strings.HasPrefix(out, "refused:") || !strings.Contains(out, ".agent-one/tmp") {
		t.Fatalf("refuse: %q", out)
	}
}

// TestTmpGuardEditRefused: a write/edit outside the workspace is refused and told where to land.
func TestTmpGuardEditRefused(t *testing.T) {
	a := testTmpApp(t, "/w/agent-one")
	a.Cfg.Tools.TmpRedirect.Enabled = true
	a.Cfg.Tools.TmpRedirect.Mode = "rewrite"
	g := a.tmpGuardHook()
	in, _ := json.Marshal(map[string]string{"file_path": "/tmp/out.md"})
	for _, tool := range []string{"write", "edit", "multiedit", "patch", "str_replace_based_edit_tool"} {
		out := g(nil, tool, in)
		if !strings.Contains(out, "refused:") || !strings.Contains(out, ".agent-one/tmp/out.md") {
			t.Fatalf("%s: %q", tool, out)
		}
	}
	// a path already inside the workspace is untouched
	ok, _ := json.Marshal(map[string]string{"file_path": "/w/agent-one/notes.md"})
	if out := g(nil, "write", ok); out != "" {
		t.Fatalf("inside the workspace is untouched: %q", out)
	}
	// the str_replace tool's field name is 'path'
	sr, _ := json.Marshal(map[string]string{"path": "/elsewhere/x.txt"})
	if out := g(nil, "str_replace_based_edit_tool", sr); !strings.Contains(out, "refused:") {
		t.Fatalf("str_replace by path: %q", out)
	}
}

// TestTmpGuardCwd: with refuseCwd, a command run from under an off-workspace root is refused.
func TestTmpGuardCwd(t *testing.T) {
	a := testTmpApp(t, "/w/agent-one")
	a.Cfg.Tools.TmpRedirect.Enabled = true
	a.Cfg.Tools.TmpRedirect.RefuseCwd = true
	g := a.tmpGuardHook()
	in, _ := json.Marshal(map[string]string{"command": "ls", "cwd": "/"})
	if out := g(nil, "bash", in); !strings.Contains(out, "cwd") {
		t.Fatalf("off-workspace cwd refused: %q", out)
	}
	in, _ = json.Marshal(map[string]string{"command": "ls", "cwd": "/tmp"})
	if out := g(nil, "bash", in); !strings.Contains(out, "cwd") {
		t.Fatalf("/tmp cwd refused: %q", out)
	}
	// workspace root and workspace-relative cwd pass
	in, _ = json.Marshal(map[string]string{"command": "ls", "cwd": "/w/agent-one/sub"})
	if out := g(nil, "bash", in); out != "" {
		t.Fatalf("inside-workspace cwd passes: %q", out)
	}
	// a cd into /tmp inside the command is refused outright
	in, _ = json.Marshal(map[string]string{"command": "cd /tmp && make"})
	if out := g(nil, "bash", in); !strings.Contains(out, "cd") {
		t.Fatalf("cd refusal: %q", out)
	}
}

// TestTmpGuardOff: disabled → nil, never called.
func TestTmpGuardOff(t *testing.T) {
	a := testTmpApp(t, "")
	a.Cfg.Tools.TmpRedirect.Enabled = false
	if g := a.tmpGuardHook(); g != nil {
		t.Fatalf("disabled hook")
	}
}

func testTmpApp(t *testing.T, root string) *App {
	t.Helper()
	c, err := config.Defaults("agent-one")
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	return &App{Root: root, Cfg: c}
}

// TestTmpGuardPerAgentScratch: with a session known, fresh redirects namespace under the agent's
// own scratch/<runid>/; nil session keeps the shared .agent-one/tmp shape (pre-D behavior).
func TestTmpGuardPerAgentScratch(t *testing.T) {
	a := testTmpApp(t, "/Users/x/ws/agent-one")
	a.Cfg.Tools.TmpRedirect.Enabled = true
	a.Cfg.Tools.TmpRedirect.Mode = "rewrite"
	g := a.tmpGuardHook()
	if g == nil {
		t.Fatal("tmpRedirect enabled")
	}
	s := &loop.Session{RunID: "zone-tui-board-20261002181951-6a6b"}
	in, _ := json.Marshal(map[string]string{"command": "probe > /tmp/probe.out"})
	want := ".agent-one/tmp/scratch/zone-tui-board-20261002181951-6a6b/probe.out"
	out := g(s, "bash", in)
	if !strings.Contains(out, want) {
		t.Fatalf("per-agent bash redirect: %q (want %s)", out, want)
	}
	// the write family lands per-agent too
	w, _ := json.Marshal(map[string]string{"file_path": "/scratchy/note.md"})
	out = g(s, "write", w)
	if !strings.Contains(out, ".agent-one/tmp/scratch/zone-tui-board-20261002181951-6a6b") {
		t.Fatalf("per-agent write redirect: %q", out)
	}
	// nil session → shared root (old behavior, tested above)
	in2, _ := json.Marshal(map[string]string{"command": "probe > /tmp/probe.out"})
	out = g(nil, "bash", in2)
	if strings.Contains(out, "/scratch/") || !strings.Contains(out, ".agent-one/tmp/probe.out") {
		t.Fatalf("shared-when-unknown: %q", out)
	}
}

// TestTmpGuardProbeMarker: read-only probes get the honesty line; write-shaped commands do not.
func TestTmpGuardProbeMarker(t *testing.T) {
	a := testTmpApp(t, "/Users/x/ws/agent-one")
	a.Cfg.Tools.TmpRedirect.Enabled = true
	a.Cfg.Tools.TmpRedirect.Mode = "rewrite"
	g := a.tmpGuardHook()
	in, _ := json.Marshal(map[string]string{"command": "ls /Users/ap00000/sdk/go1.26.0"})
	out := g(nil, "bash", in)
	if !strings.Contains(out, "probe-redirect") {
		t.Fatalf("read probe must carry the marker: %q", out)
	}
	in2, _ := json.Marshal(map[string]string{"command": "tar czf /backup.tgz src"})
	out2 := g(nil, "bash", in2)
	if strings.Contains(out2, "probe-redirect") {
		t.Fatalf("a write-shaped command must not be flagged as probe: %q", out2)
	}
	// my own audit-flail verb list: 'go version / which go' stay unmarked (not a probe verb,
	// and not redirected anyway since in-workspace CWD short-circuits before this)
	if got := probeMarker(`go version`); got != "" {
		t.Fatalf("go version: %q", got)
	}
	if got := probeMarker(`ls -la /Users/x/sdk`); got == "" {
		t.Fatal("ls probe must mark")
	}
}
