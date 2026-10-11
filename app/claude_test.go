package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hook runs `agent-one hook <event>` the way Claude Code does: the event's JSON on stdin.
func (w *testWorkspace) hook(event string, in map[string]interface{}) (int, map[string]interface{}, string) {
	w.t.Helper()
	if _, ok := in["cwd"]; !ok {
		in["cwd"] = w.root
	}
	if _, ok := in["session_id"]; !ok {
		in["session_id"] = "cc-1"
	}
	b, _ := json.Marshal(in)
	var out, errb bytes.Buffer
	env := func(k string) string {
		if k == "HOME" {
			return w.home
		}
		return ""
	}
	code := Main(w.dist, []string{"hook", event}, IO{In: bytes.NewReader(b), Out: &out, Err: &errb, Env: env}, Version{Version: "test"})
	var got map[string]interface{}
	if s := strings.TrimSpace(out.String()); s != "" {
		if err := json.Unmarshal([]byte(s), &got); err != nil {
			w.t.Fatalf("hook %s answered non-JSON: %q", event, s)
		}
	}
	return code, got, errb.String()
}

func claudeWorkspace(t *testing.T) *testWorkspace {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/config.yaml", "tools: {bash: {sandbox: none}}\nontology: {enabled: false}\n")
	return w
}

func decision(got map[string]interface{}) (string, string) {
	h, _ := got["hookSpecificOutput"].(map[string]interface{})
	d, _ := h["permissionDecision"].(string)
	r, _ := h["permissionDecisionReason"].(string)
	return d, r
}

// PreToolUse: the guard refuses, outward and destructive ask, everything else is left to Claude
// Code's own permissions — the hook only ever tightens.
func TestClaudePreToolTightensOnly(t *testing.T) {
	w := claudeWorkspace(t)
	bash := func(cmd string) map[string]interface{} {
		return map[string]interface{}{"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]string{"command": cmd}}
	}
	if code, got, _ := w.hook("pre-tool", bash("rm -rf /")); code != 0 {
		t.Fatalf("exit %d", code)
	} else if d, r := decision(got); d != "deny" || !strings.Contains(r, "guard") {
		t.Errorf("guard: %q %q", d, r)
	}
	if _, got, _ := w.hook("pre-tool", bash("git push origin main")); true {
		if d, r := decision(got); d != "ask" || !strings.Contains(r, "outward") {
			t.Errorf("outward: %q %q", d, r)
		}
	}
	if _, got, _ := w.hook("pre-tool", bash("git reset --hard")); true {
		if d, _ := decision(got); d != "ask" {
			t.Errorf("destructive: %q", d)
		}
	}
	if _, got, _ := w.hook("pre-tool", bash("ls src")); got != nil {
		t.Errorf("a read is Claude Code's to decide: %v", got)
	}
	write := map[string]interface{}{"hook_event_name": "PreToolUse", "tool_name": "Write", "tool_input": map[string]string{"file_path": "/etc/cron.d/x", "content": "x"}}
	if _, got, _ := w.hook("pre-tool", write); true {
		if d, _ := decision(got); d != "ask" {
			t.Errorf("a write outside the workspace: %q", d)
		}
	}
	inside := map[string]interface{}{"hook_event_name": "PreToolUse", "tool_name": "Edit", "tool_input": map[string]string{"file_path": filepath.Join(w.root, "src/auth/login.go")}}
	if _, got, _ := w.hook("pre-tool", inside); got != nil {
		t.Errorf("a write inside the workspace: %v", got)
	}
}

// Outside a workspace the hook is silent: installed globally, it costs nothing elsewhere.
func TestClaudeHookOutsideAWorkspaceIsSilent(t *testing.T) {
	w := claudeWorkspace(t)
	code, got, errs := w.hook("pre-tool", map[string]interface{}{"cwd": t.TempDir(), "tool_name": "Bash", "tool_input": map[string]string{"command": "git push"}})
	if code != 0 || got != nil || errs != "" {
		t.Fatalf("%d %v %q", code, got, errs)
	}
}

// SessionStart reconnects a fresh (or cleared, or compacted) session to what was written down.
func TestClaudeSessionStartInjectsThePrinciplesAndTheLog(t *testing.T) {
	w := claudeWorkspace(t)
	w.write(".agent-one/log.md", "# Chronicle\n\n---\n\n### [2026-10-01 10:00] the orchestrator — the auth zone founded\n- **Task:** x\n")
	_, got, _ := w.hook("session-start", map[string]interface{}{"hook_event_name": "SessionStart", "source": "clear"})
	h, _ := got["hookSpecificOutput"].(map[string]interface{})
	ctx, _ := h["additionalContext"].(string)
	for _, want := range []string{"Docs-as-code", "Egress control", "the auth zone founded", ".agent-one/AGENT-ONE.md", "gate"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("additionalContext lacks %q:\n%s", want, ctx)
		}
	}
	if h["hookEventName"] != "SessionStart" {
		t.Errorf("event name: %v", h)
	}
}

// Stop runs the gate over what the turn wrote: a Docs-as-code failure sends the turn back with the
// reason; once the retries are spent the turn may end, and the failed verdict is logged.
func TestClaudeStopGatesTheTurn(t *testing.T) {
	w := claudeWorkspace(t)
	w.hook("prompt", map[string]interface{}{"hook_event_name": "UserPromptSubmit", "prompt": "shorten the token ttl"})
	w.write("src/auth/login.go", "package auth // 15m\n")
	w.hook("post-tool", map[string]interface{}{"hook_event_name": "PostToolUse", "tool_name": "Edit", "tool_input": map[string]string{"file_path": filepath.Join(w.root, "src/auth/login.go")}})
	code, got, _ := w.hook("stop", map[string]interface{}{"hook_event_name": "Stop", "stop_hook_active": false})
	if code != 0 || got["decision"] != "block" || !strings.Contains(got["reason"].(string), ".agent-one/zone/auth/README.md") {
		t.Fatalf("vitality not enforced: %d %v", code, got)
	}
	// the model tries to stop again without fixing it: the retries are spent, the stop goes through
	_, got, _ = w.hook("stop", map[string]interface{}{"hook_event_name": "Stop", "stop_hook_active": true})
	if got["decision"] == "block" || !strings.Contains(got["systemMessage"].(string), "gate fail") {
		t.Fatalf("second stop: %v", got)
	}
	if lg := w.read(".agent-one/log.md"); !strings.Contains(lg, "gate fail — shorten the token ttl") || !strings.Contains(lg, "src/auth/login.go") {
		t.Fatalf("the verdict is not in the log:\n%s", lg)
	}
	// a new turn that changes the doc with the code passes, and is logged
	w.hook("prompt", map[string]interface{}{"hook_event_name": "UserPromptSubmit", "prompt": "fix the doc"})
	w.write("src/auth/login.go", "package auth // 15m, documented\n")
	w.write(".agent-one/zone/auth/README.md", "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n\n## Invariants\n- tokens expire after 15 minutes\n")
	_, got, _ = w.hook("stop", map[string]interface{}{"hook_event_name": "Stop"})
	if got["decision"] == "block" || !strings.Contains(got["systemMessage"].(string), "gate pass") {
		t.Fatalf("a truthful turn: %v", got)
	}
	if lg := w.read(".agent-one/log.md"); !strings.Contains(lg, "gate pass — fix the doc") {
		t.Fatalf("the pass is not in the log:\n%s", lg)
	}
	// a turn that wrote nothing is not gated
	w.hook("prompt", map[string]interface{}{"hook_event_name": "UserPromptSubmit", "prompt": "just look"})
	if _, got, _ = w.hook("stop", map[string]interface{}{"hook_event_name": "Stop"}); got != nil {
		t.Fatalf("a read-only turn: %v", got)
	}
}

// A verify line written in the same turn is the model's own command: under Claude Code too, the
// gate holds it as a hole and never runs it (each hook is a fresh process — the lines as they
// stood are taken at the prompt).
func TestClaudeStopNeverRunsAVerifyLineAddedThisTurn(t *testing.T) {
	w := claudeWorkspace(t)
	w.hook("prompt", map[string]interface{}{"hook_event_name": "UserPromptSubmit", "prompt": "x"})
	w.write("src/auth/x.go", "package auth\n")
	w.write(".agent-one/zone/auth/README.md", "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- **Verify:** `touch PWNED`\n")
	_, got, _ := w.hook("stop", map[string]interface{}{"hook_event_name": "Stop"})
	t.Logf("stop answered %v", got)
	if _, err := os.Stat(filepath.Join(w.root, "PWNED")); err == nil {
		t.Fatal("the gate ran a verify line the turn wrote")
	}
	if msg, _ := got["systemMessage"].(string); !strings.Contains(msg, "was added this turn") {
		t.Fatalf("no hole for the added line: %v", got)
	}
}

// UserPromptSubmit recalls: the anchors that answer the ask reach the model as context.
func TestClaudePromptRecalls(t *testing.T) {
	w := claudeWorkspace(t)
	_, got, _ := w.hook("prompt", map[string]interface{}{"hook_event_name": "UserPromptSubmit", "prompt": "how long do auth tokens live"})
	h, _ := got["hookSpecificOutput"].(map[string]interface{})
	if ctx, _ := h["additionalContext"].(string); !strings.Contains(ctx, ".agent-one/zone/auth/README.md") {
		t.Fatalf("recall: %v", got)
	}
}

// install --claude writes the hooks into the machine's own settings, keeping what is there, and
// shows the change before it writes it.
func TestClaudeInstall(t *testing.T) {
	w := claudeWorkspace(t)
	w.write(".claude/settings.local.json", `{"permissions": {"allow": ["Bash(git status:*)"]}}`)
	code, out, _ := w.main("agent-one", "", "install", "--claude")
	if code != 0 || !strings.Contains(out, "SessionStart") || !strings.Contains(out, "--yes") {
		t.Fatalf("dry run: %d %q", code, out)
	}
	if strings.Contains(w.read(".claude/settings.local.json"), "hook") {
		t.Fatal("written without --yes")
	}
	if code, _, _ = w.main("agent-one", "", "install", "--claude", "--yes"); code != 0 {
		t.Fatalf("install: %d", code)
	}
	var s map[string]interface{}
	if err := json.Unmarshal([]byte(w.read(".claude/settings.local.json")), &s); err != nil {
		t.Fatal(err)
	}
	hooks, _ := s["hooks"].(map[string]interface{})
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		if hooks[ev] == nil {
			t.Errorf("no %s hook: %v", ev, hooks)
		}
	}
	stop, _ := hooks["Stop"].([]interface{})
	if e, _ := stop[0].(map[string]interface{}); e != nil {
		hs, _ := e["hooks"].([]interface{})
		if h, _ := hs[0].(map[string]interface{}); h["timeout"] != float64(600) {
			t.Errorf("the gate needs time for its verify lines: %v", h)
		}
	}
	if s["permissions"] == nil {
		t.Error("the existing settings were dropped")
	}
	// idempotent
	before := w.read(".claude/settings.local.json")
	w.main("agent-one", "", "install", "--claude", "--yes")
	if w.read(".claude/settings.local.json") != before {
		t.Error("a second install changed the settings")
	}
}
