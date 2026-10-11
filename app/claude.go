package app

// The Claude Code integration. Claude Code is the loop — its model, its tools, its session — and
// this binary is the policy, run from Claude Code's hooks: code paths the model cannot skip.
// `<dist> hook <event>` reads the hook's JSON on stdin and answers on stdout. It never calls a
// model, so it needs no key and spends no tokens; outside a workspace it is silent, so a machine-wide
// install costs nothing elsewhere. `<dist> install --claude` wires it into Claude Code.
//
//	session-start  the principles, the log's last entries and how the policy runs here, as context
//	prompt         the turn's baseline (what is on disk), and recall for the ask, as context
//	pre-tool       the guard refuses; outward and destructive acts ask — the hook only tightens
//	post-tool      the paths a file tool wrote, for the gate
//	stop           the gate over what the turn changed: a failure sends the turn back with the
//	               reasons, up to policy.gate.retries; the verdict is appended to log.md

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/guard"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/wire"
	"github.com/Kaginari/agent-one/workspace"
)

// ccIn is the part of Claude Code's hook input the policy reads.
type ccIn struct {
	SessionID      string          `json:"session_id"`
	Cwd            string          `json:"cwd"`
	Event          string          `json:"hook_event_name"`
	Source         string          `json:"source"`
	Prompt         string          `json:"prompt"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	StopHookActive bool            `json:"stop_hook_active"`
	TranscriptPath string          `json:"transcript_path"`
	LastAssistant  string          `json:"last_assistant_message"`
}

// ccEvents maps the hook's argument to Claude Code's event name, in the order install wires them.
var ccEvents = []struct{ arg, event, matcher string }{
	{"session-start", "SessionStart", ""},
	{"prompt", "UserPromptSubmit", ""},
	{"pre-tool", "PreToolUse", "Bash|Write|Edit|MultiEdit|NotebookEdit|WebFetch|WebSearch"},
	{"post-tool", "PostToolUse", "Write|Edit|MultiEdit|NotebookEdit"},
	{"stop", "Stop", ""},
}

// policyHost is the workspace as a hook sees it: config, the workspace, and the turn's state file.
type policyHost struct {
	dist string
	cfg  *config.Config
	w    *workspace.Workspace
	root string
	io   IO
}

// ccTurn is what a turn carries between hooks: the ask, the tree as the turn opened, what the
// file tools wrote, and how often the gate has sent the turn back.
type ccTurn struct {
	Ask    string              `json:"ask"`
	Base   map[string]string   `json:"base,omitempty"`
	Wrote  []string            `json:"wrote,omitempty"`
	Verify map[string][]string `json:"verify,omitempty"` // the verify lines as the turn opened
	Blocks int                 `json:"blocks"`
}

func cmdHook(dist string, args []string, io IO) int {
	event := ""
	if len(args) > 0 {
		event = args[0]
	}
	raw, _ := readAllLimited(io.In, 8<<20)
	var in ccIn
	_ = json.Unmarshal([]byte(raw), &in)
	if event == "" {
		for _, e := range ccEvents {
			if e.event == in.Event {
				event = e.arg
			}
		}
	}
	h, err := openPolicy(dist, in, io)
	if err != nil {
		// a broken config in a workspace is the human's to see; Claude Code shows a non-blocking error
		fmt.Fprintf(io.Err, "%s hook: %v\n", dist, err)
		return 1
	}
	if h == nil {
		return 0 // not a workspace
	}
	switch event {
	case "session-start":
		return h.sessionStart(in)
	case "prompt":
		return h.prompt(in)
	case "pre-tool":
		return h.preTool(in)
	case "post-tool":
		return h.postTool(in)
	case "stop":
		return h.stop(in)
	}
	fmt.Fprintf(io.Err, "%s hook: unknown event %q — session-start, prompt, pre-tool, post-tool or stop\n", dist, event)
	return 1
}

// openPolicy loads the workspace around the hook's cwd; nil when there is none.
func openPolicy(dist string, in ccIn, io IO) (*policyHost, error) {
	cwd := in.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	home := io.Env("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	cfg, err := config.LoadWith(config.Options{Dist: dist, Cwd: cwd, Home: home, Env: io.Env})
	if err != nil {
		if strings.Contains(err.Error(), "no ") && !hasWorkspace(cwd, dist) {
			return nil, nil
		}
		return nil, err
	}
	if !hasWorkspace(cfg.Root, dist) {
		return nil, nil
	}
	w, err := openWorkspace(cfg, cfg.Root, lexiconFor())
	if err != nil {
		return nil, err
	}
	return &policyHost{dist: dist, cfg: cfg, w: w, root: cfg.Root, io: io}, nil
}

func hasWorkspace(root, dist string) bool {
	d, err := parseDist(dist)
	if err != nil || root == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(root, d.WorkspaceDir))
	return err == nil && st.IsDir()
}

func (h *policyHost) answer(v interface{}) int {
	b, _ := json.Marshal(v)
	fmt.Fprintln(h.io.Out, string(b))
	return 0
}

func (h *policyHost) context(event, text string) int {
	return h.answer(map[string]interface{}{"hookSpecificOutput": map[string]string{"hookEventName": event, "additionalContext": text}})
}

// ---- session-start

var logHeader = regexp.MustCompile(`(?m)^### \[([^\]]+)\] (.+)$`)

func (h *policyHost) sessionStart(in ccIn) int {
	wd := h.cfg.Dist.WorkspaceDir
	policyPath := filepath.Join(wd, h.w.Lex.Policy)
	var b strings.Builder
	fmt.Fprintf(&b, "This directory is an %s %s. Its policy is %s — read it before working; the principles first, always.\n", h.dist, "workspace", policyPath)
	if principles := section(readOr(filepath.Join(h.root, policyPath)), "## Core principles"); principles != "" {
		b.WriteString("\n" + principles + "\n")
	}
	if heads := logHeader.FindAllStringSubmatch(readOr(h.w.LogPath()), -1); len(heads) > 0 {
		if len(heads) > 7 {
			heads = heads[len(heads)-7:]
		}
		fmt.Fprintf(&b, "\nThe last entries of %s (what the workspace already learned — read the ones that touch this task):\n", filepath.Join(wd, h.w.Lex.Log))
		for _, m := range heads {
			fmt.Fprintf(&b, "- [%s] %s\n", m[1], m[2])
		}
	}
	fmt.Fprintf(&b, "\nIn this session the %s binary holds the policy through Claude Code's hooks: catastrophic commands are refused, outward and destructive acts ask the human, and when a turn changed files the gate runs as the turn ends — a failure (an owned path changed without its doc, a verify line that fails) sends the turn back with the reasons, and every verdict is appended to the log.\n", h.dist)
	return h.context("SessionStart", b.String())
}

// section is a markdown section's agent, from its heading to the next heading of the same level.
func section(doc, heading string) string {
	i := strings.Index(doc, heading+"\n")
	if i < 0 {
		return ""
	}
	rest := doc[i+len(heading)+1:]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

func readOr(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

// ---- the turn's state

func (h *policyHost) turnPath(session string) string {
	if session == "" {
		session = "default"
	}
	session = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == '.' {
			return '_'
		}
		return r
	}, session)
	return filepath.Join(h.w.Dir(), "tmp", "claude", session+".json")
}

func (h *policyHost) loadTurn(session string) ccTurn {
	var t ccTurn
	_ = json.Unmarshal([]byte(readOr(h.turnPath(session))), &t)
	return t
}

func (h *policyHost) saveTurn(session string, t ccTurn) {
	p := h.turnPath(session)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.Marshal(t)
	_ = os.WriteFile(p, b, 0o644)
}

func (h *policyHost) watcher() *loop.Engine {
	return &loop.Engine{Root: h.root, Lexicon: loop.Lexicon{WorkspaceDir: h.cfg.Dist.WorkspaceDir}}
}

// ---- prompt

func (h *policyHost) prompt(in ccIn) int {
	t := ccTurn{Ask: in.Prompt, Verify: h.w.VerifyLines()}
	t.Base, _ = h.watcher().Stamps()
	h.saveTurn(in.SessionID, t)
	ro := hookOptions(h.cfg, nil).Recall
	rc := h.w.Recall("", in.Prompt, ro)
	if len(rc.Anchors) == 0 && len(rc.Tools) == 0 {
		return 0
	}
	var b strings.Builder
	if len(rc.Anchors) > 0 {
		b.WriteString("Recalled for this ask — pointers, not content; read the ones that help:\n")
		seen := map[string]bool{}
		for _, a := range rc.Anchors {
			if !seen[a] {
				seen[a] = true
				b.WriteString("- " + a + "\n")
			}
		}
	}
	if len(rc.Tools) > 0 {
		b.WriteString("Tools that fit this ask (load one only when you use it):\n")
		for _, l := range rc.Tools {
			b.WriteString(l + "\n")
		}
	}
	return h.context("UserPromptSubmit", b.String())
}

// ---- pre-tool

func (h *policyHost) env() tool.Env {
	return tool.Env{Root: h.root, WorkspaceDir: h.cfg.Dist.WorkspaceDir}
}

// ccTool is a Claude Code tool as the policy's classifier and permission rules read it: the
// rule's tool name and the classification.
func (h *policyHost) ccTool(name string, in json.RawMessage) (string, json.RawMessage, tool.Classification, bool) {
	var a struct {
		Command      string `json:"command"`
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		URL          string `json:"url"`
		Query        string `json:"query"`
	}
	_ = json.Unmarshal(in, &a)
	env := h.env()
	switch name {
	case "Bash":
		return "bash", in, env.ClassifyCommand(a.Command), true
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		p := a.FilePath
		if p == "" {
			p = a.NotebookPath
		}
		rule := map[string]string{"Write": "write", "NotebookEdit": "write"}[name]
		if rule == "" {
			rule = "edit"
		}
		norm, _ := json.Marshal(map[string]string{"path": p})
		return rule, norm, env.ClassifyPath(env.Resolve(p)), true
	case "WebFetch":
		return "webfetch", in, tool.Classification{Class: tool.Outward, Why: "fetches " + a.URL}, true
	case "WebSearch":
		return "websearch", in, tool.Classification{Class: tool.Outward, Why: "searches the web for " + a.Query}, true
	}
	return "", nil, tool.Classification{}, false
}

func (h *policyHost) preTool(in ccIn) int {
	if in.ToolName == "Bash" && h.cfg.Guard.Enabled {
		var a struct{ Command string }
		_ = json.Unmarshal(in.ToolInput, &a)
		if r := h.guard().Match(a.Command); r != nil {
			return h.permission("deny", guard.Refusal(r))
		}
	}
	name, input, cls, ok := h.ccTool(in.ToolName, in.ToolInput)
	if !ok {
		return 0
	}
	live := func() *config.Config { return h.cfg }
	d := decideHook(live, h.env)(nil, &loop.StepRecord{Tool: name, Input: input}, cls)
	switch {
	case d.Action == "deny":
		return h.permission("deny", fmt.Sprintf("%s: refused by a permission rule — %s", h.dist, d.Why))
	case d.Action == "ask":
		return h.permission("ask", fmt.Sprintf("%s: %s — %s (a permission rule: %s)", h.dist, cls.Class, cls.Why, d.Why))
	case d.Action == "allow":
		return 0 // the workspace allows it; Claude Code's own permissions still decide
	}
	if !h.cfg.Policy.HumanGate.Enabled || h.approved(cls.Class) {
		return 0
	}
	if cls.Class >= tool.Outward || cls.Class == tool.Write && h.cfg.Policy.HumanGate.Strict {
		policy := "Principle 7, Egress control"
		if cls.Class == tool.Destructive {
			policy = "Policy 6, no destruction without consent"
		}
		return h.permission("ask", fmt.Sprintf("%s: %s — %s (%s)", h.dist, cls.Class, cls.Why, policy))
	}
	return 0
}

func (h *policyHost) approved(c tool.Class) bool {
	for _, a := range h.cfg.Policy.HumanGate.Approve {
		if a == c.String() {
			return true
		}
	}
	return false
}

func (h *policyHost) guard() *guard.Guard {
	home := h.io.Env("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	g := guard.ForHome(home, expandPaths(h.cfg.Guard.Files, h.root, home)...)
	if len(h.cfg.Guard.Patterns) > 0 {
		g.AddPatterns(h.cfg.Guard.Patterns, "config "+h.cfg.Where("guard.patterns"))
	}
	return g
}

func (h *policyHost) permission(decision, why string) int {
	return h.answer(map[string]interface{}{"hookSpecificOutput": map[string]string{
		"hookEventName": "PreToolUse", "permissionDecision": decision, "permissionDecisionReason": why}})
}

// ---- post-tool

func (h *policyHost) postTool(in ccIn) int {
	_, input, _, ok := h.ccTool(in.ToolName, in.ToolInput)
	if !ok {
		return 0
	}
	var a struct{ Path string }
	_ = json.Unmarshal(input, &a)
	if a.Path == "" {
		return 0
	}
	t := h.loadTurn(in.SessionID)
	t.Wrote = appendUniq(t.Wrote, h.env().Rel(h.env().Resolve(a.Path)))
	h.saveTurn(in.SessionID, t)
	return 0
}

func appendUniq(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

// ---- stop: the gate

func (h *policyHost) stop(in ccIn) int {
	// the turn's final answer files its @FORGE drafts, as the binary's orchestrator does
	forgeHoles := h.w.FileForges("orchestrator", in.SessionID, wire.ForgesOf(ccFinalText(in)))
	t := h.loadTurn(in.SessionID)
	wrote := append([]string(nil), t.Wrote...)
	if t.Base != nil {
		if now, why := h.watcher().Stamps(); why == "" {
			for _, p := range loop.Changed(t.Base, now) {
				wrote = appendUniq(wrote, p)
			}
		}
	}
	sort.Strings(wrote)
	if len(wrote) == 0 || !h.cfg.Policy.Gate.Enabled {
		if len(forgeHoles) > 0 {
			return h.answer(map[string]interface{}{"systemMessage": h.dist + " forge\n@? " + strings.Join(forgeHoles, "\n@? ")})
		}
		return 0
	}
	opt := hookOptions(h.cfg, nil).Gate
	opt.Sandbox = newSandbox(h.cfg, h.root)
	opt.DutiesDone = false  // the wire is the binary's; Claude Code answers in prose
	opt.RightAuthor = false // the session is The orchestrator; who should have authored is a hole below
	// the lines as the turn opened, taken at the prompt: this process opened the workspace after the
	// turn wrote, so its roster already holds any line the model added
	opt.VerifyBefore = t.Verify
	if opt.VerifyBefore == nil {
		opt.VerifyBefore = map[string][]string{} // no prompt seen: no line is trusted unconfirmed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	v := h.w.Gate(ctx, nil, opt, "", wrote, nil, false, t.Ask)
	holes := append(append([]string(nil), v.Holes...), forgeHoles...)
	if h.cfg.Policy.Gate.RightAuthor && h.w.GateHolders() {
		for _, rel := range wrote {
			if o := h.w.Owner(rel); o != nil {
				holes = append(holes, fmt.Sprintf("%s is %s's ground, written as The orchestrator — dispatch %s's agent to author there", rel, o.Name, o.Name))
				break
			}
		}
	}
	if v.Word == "fail" && t.Blocks < h.cfg.Policy.Gate.Retries {
		t.Blocks++
		h.saveTurn(in.SessionID, t)
		return h.answer(map[string]interface{}{"decision": "block", "reason": fmt.Sprintf(
			"The %s gate failed for this turn's changes (%s):\n- %s\nFix what it names — the doc that owns a changed ownership changes in the same change — then finish. (%d of %d sends back)",
			h.dist, strings.Join(wrote, ", "), strings.Join(v.Reasons, "\n- "), t.Blocks, h.cfg.Policy.Gate.Retries)})
	}
	if h.cfg.Policy.Log.Enabled {
		result := "done"
		if v.Word == "fail" {
			result = "failed — gate"
		}
		title := v.Word
		if domains := v.Word == "pass" || v.Word == "fail"; domains {
			title = "gate " + v.Word
		}
		ask := askTitle(t.Ask)
		e := workspace.Entry{Author: "the orchestrator", Title: title + " — " + ask, Task: ask, Files: wrote, Gate: v.String(), Result: result}
		if err := h.w.AppendLog(e); err != nil {
			holes = append(holes, "log.md: "+err.Error())
		}
	}
	// the turn is over: a next stop in the same turn sees only what changes after it
	t.Wrote, t.Blocks = nil, 0
	t.Base, _ = h.watcher().Stamps()
	t.Verify = h.w.VerifyLines()
	h.saveTurn(in.SessionID, t)
	msg := fmt.Sprintf("%s gate %s", h.dist, v.String())
	if !(v.Word == "pass" || v.Word == "fail") {
		msg = h.dist + " " + v.String()
	}
	if len(holes) > 0 {
		msg += "\n@? " + strings.Join(holes, "\n@? ")
	}
	return h.answer(map[string]interface{}{"systemMessage": msg})
}

// ccFinalText is the turn's final answer as Claude Code hands it to the Stop hook: the
// last_assistant_message field when the host sends it, else the assistant text after the
// transcript's last user entry (a prompt or a tool result) — the words that closed the turn.
func ccFinalText(in ccIn) string {
	if strings.TrimSpace(in.LastAssistant) != "" {
		return in.LastAssistant
	}
	if in.TranscriptPath == "" {
		return ""
	}
	b, err := os.ReadFile(in.TranscriptPath)
	if err != nil {
		return ""
	}
	var texts []string
	for _, line := range strings.Split(string(b), "\n") {
		var e struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		switch e.Type {
		case "user":
			texts = nil
		case "assistant":
			var blocks []struct{ Type, Text string }
			if json.Unmarshal(e.Message.Content, &blocks) == nil {
				for _, bl := range blocks {
					if bl.Type == "text" {
						texts = append(texts, bl.Text)
					}
				}
			}
		}
	}
	return strings.Join(texts, "\n")
}

func askTitle(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	if s == "" {
		s = "a Claude Code turn"
	}
	return s
}

// ---- install

// cmdInstall wires the hooks into Claude Code: the workspace's machine-local settings by default
// (.claude/settings.local.json — not tracked, like config.local), or the user's with --global.
// It shows the change; --yes writes it.
func cmdInstall(dist string, f cliFlags, args []string, io IO) int {
	var claude, global, yes bool
	for _, a := range args {
		switch a {
		case "--claude":
			claude = true
		case "--global":
			global = true
		case "--yes", "-y":
			yes = true
		}
	}
	if !claude {
		fmt.Fprintf(io.Err, "@S FAIL\n@? install: name the host — `%s install --claude [--global] [--yes]`\n", dist)
		return 2
	}
	home := io.Env("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	exe, err := os.Executable()
	if err != nil {
		exe = dist
	}
	if strings.ContainsAny(exe, " '\"") {
		exe = "'" + strings.ReplaceAll(exe, "'", `'\''`) + "'"
	}
	var target string
	if global {
		target = filepath.Join(home, ".claude", "settings.json")
	} else {
		root := f.root
		if root == "" {
			root, _ = os.Getwd()
		}
		cfg, err := config.LoadWith(config.Options{Dist: dist, Root: f.root, Cwd: root, Home: home, Env: io.Env})
		if err != nil || !hasWorkspace(cfg.Root, dist) {
			fmt.Fprintf(io.Err, "@S FAIL\n@? install: no workspace here — found one with `%s init`, or install for every workspace with --global\n", dist)
			return 2
		}
		target = filepath.Join(cfg.Root, ".claude", "settings.local.json")
	}
	before := readOr(target)
	after, changed, err := mergeClaudeHooks([]byte(before), exe)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? install: %s: %v\n", target, err)
		return 2
	}
	if !changed {
		fmt.Fprintf(io.Out, "%s already runs the %s hooks — nothing to change.\n", target, dist)
		return 0
	}
	fmt.Fprintf(io.Out, "%s — Claude Code runs the policy from these hooks:\n", target)
	for _, e := range ccEvents {
		m := ""
		if e.matcher != "" {
			m = " (" + e.matcher + ")"
		}
		fmt.Fprintf(io.Out, "  %-17s %s hook %s%s\n", e.event, exe, e.arg, m)
	}
	if !yes {
		fmt.Fprintf(io.Out, "\nnothing written: `%s install --claude%s --yes` writes it (then start a new Claude Code session)\n", dist, map[bool]string{true: " --global"}[global])
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? install: %v\n", err)
		return 2
	}
	if err := os.WriteFile(target, after, 0o644); err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? install: %v\n", err)
		return 2
	}
	fmt.Fprintf(io.Out, "\nwritten — start a new Claude Code session for the hooks to load\n")
	return 0
}

// mergeClaudeHooks adds the policy's hooks to a Claude Code settings document, keeping everything
// else; changed is false when every hook is already there.
func mergeClaudeHooks(doc []byte, exe string) ([]byte, bool, error) {
	m := map[string]interface{}{}
	if len(bytes.TrimSpace(doc)) > 0 {
		if err := json.Unmarshal(doc, &m); err != nil {
			return nil, false, fmt.Errorf("settings do not parse: %w", err)
		}
	}
	hooks, _ := m["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}
	changed := false
	for _, e := range ccEvents {
		command := exe + " hook " + e.arg
		list, _ := hooks[e.event].([]interface{})
		if hasCommand(list, command) {
			continue
		}
		hk := map[string]interface{}{"type": "command", "command": command}
		if e.event == "Stop" {
			hk["timeout"] = 600 // the gate runs verify lines and checks; Claude Code's default is shorter
		}
		entry := map[string]interface{}{"hooks": []interface{}{hk}}
		if e.matcher != "" {
			entry["matcher"] = e.matcher
		}
		hooks[e.event] = append(list, entry)
		changed = true
	}
	if !changed {
		return doc, false, nil
	}
	m["hooks"] = hooks
	out, err := json.MarshalIndent(m, "", "  ")
	return append(out, '\n'), true, err
}

func hasCommand(list []interface{}, command string) bool {
	for _, x := range list {
		e, _ := x.(map[string]interface{})
		hs, _ := e["hooks"].([]interface{})
		for _, y := range hs {
			if hk, _ := y.(map[string]interface{}); hk != nil && hk["command"] == command {
				return true
			}
		}
	}
	return false
}
