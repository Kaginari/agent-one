package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/wire"
	"github.com/Kaginari/agent-one/workspace"
)

// forgeWorkspace is the agent-one test workspace where coord-core holds ttl-craft (its doc names the Skill),
// dispatch reaches three hops, and the provider is a recording mock the test scripts.
func forgeWorkspace(t *testing.T) (*testWorkspace, *App) {
	t.Helper()
	files := agentOneMembers()
	files[".agent-one/coord/core/README.md"] = "# coord-core\n\n- **Rank:** Coordinator\n- **Owns:** `src/`\n- **Skills:** ttl-craft\n- **Purpose:** the shared skill\n"
	w := newTestWorkspace(t, "agent-one", files)
	w.script(".agent-one/tmp/s.json", text("nothing"))
	w.write(".agent-one/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true, script: .agent-one/tmp/s.json}}\ntools: {bash: {sandbox: none}, dispatch: {maxDepth: 3}}\nui: {board: {autostart: false}}\npolicy: {forge: {demandAtSteps: 0}}\n")
	return w, w.open()
}

// orchestrator runs one ask on the orchestrator with every agent answering from the same recording mock.
func orchestrator(t *testing.T, a *App, ask string, script ...provider.Response) *mock.Provider {
	t.Helper()
	m := mock.New(script...)
	b := a.Build()
	b.Provider, b.Models = m, nil
	e := a.Workspace.Engine(workspace.Orchestrator, b)
	defer a.Workspace.Close(e)
	if _, err := e.Run(context.Background(), ask); err != nil {
		t.Fatal(err)
	}
	return m
}

func dispatchCall(id, agent, ask string) provider.Response {
	return mock.Call(id, "dispatch", map[string]interface{}{"agent": agent, "ask": ask})
}

func forgeNotes(t *testing.T, w *testWorkspace) []workspace.ForgeNote {
	t.Helper()
	return workspace.ReadForgeLedger(filepath.Join(w.root, ".agent-one", "memory", "shared", "notes.jsonl"))
}

func workingNotes(a *App, args ...string) (int, string, string) {
	var out, errw bytes.Buffer
	code := a.forgeRun(&out, &errw, args, nil, "/forge")
	return code, out.String(), errw.String()
}

var workingNotesID = regexp.MustCompile(`(?m)^([0-9a-f]{7})  (\S+)`)

// idOf finds a listed draft's id by name.
func idOf(t *testing.T, a *App, name string) string {
	t.Helper()
	_, out, _ := workingNotes(a, "list")
	for _, m := range workingNotesID.FindAllStringSubmatch(out, -1) {
		if m[2] == name {
			return m[1]
		}
	}
	t.Fatalf("%s not on the working notes:\n%s", name, out)
	return ""
}

// The forge end to end: a Subagent's draft is filed from its report, the working notes lists it by a
// stable id, the register writes a real SKILL.md in Claude Code's format, and a later Subagent that
// holds the Skill carries `@T skill ttl-craft` in its prompt while skill{name} returns the agent.
func TestForgeDraftRegisterHold(t *testing.T) {
	w, a := forgeWorkspace(t)
	orchestrator(t, a, "work the token ttl",
		dispatchCall("d1", "coord-core", "findings on the token ttl"),
		mock.Text("@S DONE\n@F src/auth/login.go:1 ttl read\n@FORGE skill:ttl-craft | Use when a token TTL is set, read or questioned | 1. read src/auth/login.go 2. keep the ttl at one hour 3. say where it is set\n@U team the ttl is an hour\n@E 0"),
		mock.Text("done"))
	notes := forgeNotes(t, w)
	if len(notes) != 1 || notes[0].By != "coord-core" || notes[0].Run == "" || notes[0].Hash == "" {
		t.Fatalf("one draft filed by its agent with its run and hash: %+v", notes)
	}
	id := idOf(t, a, "ttl-craft")
	if !strings.HasPrefix(notes[0].Hash, id) {
		t.Fatalf("the working notes's id is the draft's hash: %s vs %s", id, notes[0].Hash)
	}
	if code, out, errs := workingNotes(a, "register", id); code != 0 || !strings.Contains(out, "registered skill:ttl-craft v1") {
		t.Fatalf("register: %d\n%s\n%s", code, out, errs)
	}
	skill := w.read(".agent-one/crafted-skills/ttl-craft/SKILL.md")
	for _, want := range []string{"---\nname: ttl-craft\n", "description: \"Use when a token TTL is set, read or questioned\"", "version: 1\n", "status: active\n", "by: \"coord-core\"", "provisioned: \"" + notes[0].Run + "\"", "registered: {by: \"operator\"", "# ttl-craft\n\n1. read src/auth/login.go"} {
		if !strings.Contains(skill, want) {
			t.Fatalf("SKILL.md lacks %q:\n%s", want, skill)
		}
	}
	if _, out, _ := workingNotes(a, "list"); strings.Contains(out, "ttl-craft") {
		t.Fatalf("a registered draft leaves the working notes:\n%s", out)
	}
	// a later Subagent that holds it: its prompt carries the Skill
	m := orchestrator(t, a, "work the token ttl again",
		dispatchCall("d1", "coord-core", "findings on the token ttl, again"),
		mock.Text("@S DONE\n@F src/auth/login.go:1 still an hour\n@U team nothing new\n@E 0"),
		mock.Text("done"))
	subagent := m.Requests[1]
	all := subagent.System + subagent.SystemTail
	for _, msg := range subagent.Messages {
		all += "\n" + msg.Text
	}
	if !strings.Contains(all, "@T skill ttl-craft") {
		t.Fatalf("the holding Subagent's prompt must carry @T skill ttl-craft:\n%s", all)
	}
	res := a.skillTool("coord-core").Run(context.Background(), tool.Env{Root: a.Root, WorkspaceDir: ".agent-one"}, json.RawMessage(`{"name":"ttl-craft"}`))
	if res.Err || !strings.Contains(res.Output, "keep the ttl at one hour") {
		t.Fatalf("skill{name} returns the agent: %+v", res)
	}
}

// a report over the 600-byte envelope (and over @CAP) still keeps its draft: the harness files
// from the report it holds, never from the digest the parent reads.
func TestForgeDraftSurvivesTheEnvelope(t *testing.T) {
	w, a := forgeWorkspace(t)
	long := "@S DONE\n"
	for len(long) < 2600 {
		long += "@F src/auth/login.go:1 a long finding that fills the report past the envelope and the cap\n"
	}
	long += "@FORGE skill:auth-tour | Use when touring src/auth | read login.go first\n@U team long\n@E 0"
	m := orchestrator(t, a, "tour auth",
		dispatchCall("d1", "coord-core", "findings on auth"),
		mock.Text(long),
		mock.Text("done"))
	if seen := m.Requests[2].Messages[len(m.Requests[2].Messages)-1].ToolResults[0].Content; strings.Contains(seen, "@FORGE") {
		t.Fatalf("the test needs the draft out of the parent's view:\n%s", seen)
	}
	notes := forgeNotes(t, w)
	if len(notes) != 1 || notes[0].By != "coord-core" || !strings.HasPrefix(notes[0].Text, "skill:auth-tour") {
		t.Fatalf("the draft is filed although the parent saw only the envelope: %+v", notes)
	}
}

// a draft relayed up three hops is filed once, as the zone worker's: the first filer wins.
func TestForgeRelayFilesOnce(t *testing.T) {
	w, a := forgeWorkspace(t)
	line := "@FORGE tool:token-grep | find token ttl lines | grep -rn ttl {{dir}}"
	orchestrator(t, a, "the ttl ladder",
		dispatchCall("d1", "coord-core", "draft on the ttl"),
		dispatchCall("d2", "domain-security", "verdict on the ttl"),
		dispatchCall("d3", "zone-auth", "findings on the ttl"),
		mock.Text("@S DONE\n@F src/auth/login.go:1 ttl\n"+line+"\n@U team zone worker\n@E 0"),
		mock.Text("@S DONE\n@F relayed\n"+line+"\n@U team domain owner\n@E 0"),
		mock.Text("@S DONE\n@F relayed\n"+line+"\n@U team coordinator\n@E 0"),
		mock.Text("done\n"+line))
	notes := forgeNotes(t, w)
	if len(notes) != 1 || notes[0].By != "zone-auth" {
		t.Fatalf("one filing, by the zone worker: %+v", notes)
	}
}

// the orchestrator's own final answer files its drafts (prose, not the wire).
func TestForgeOrchestratorAnswerFiles(t *testing.T) {
	w, a := forgeWorkspace(t)
	orchestrator(t, a, "say it", mock.Text("Here is the pattern.\n@FORGE skill:say-it | Use when asked to say it | say it once"))
	notes := forgeNotes(t, w)
	if len(notes) != 1 || notes[0].By != "orchestrator" || notes[0].Text != "skill:say-it | Use when asked to say it | say it once" {
		t.Fatalf("the orchestrator's draft: %+v", notes)
	}
}

func fileDraft(t *testing.T, a *App, by string, line string) {
	t.Helper()
	fs := wire.ForgesOf("@FORGE " + line)
	if len(fs) != 1 {
		t.Fatalf("draft line %q", line)
	}
	if holes := a.Workspace.FileForges(by, "run-1", fs); len(holes) > 0 {
		t.Fatal(holes)
	}
}

// A tool register is safe: shell quoting, the name checked first, the class from the classifier,
// interpreter-eval refused unless forced, outward needs --yes, the YAML printed, provenance kept.
func TestForgeToolRegisterIsSafe(t *testing.T) {
	w, a := forgeWorkspace(t)
	fileDraft(t, a, "zone-auth", "tool:todo-grep | find the TODOs | grep -rn 'TODO: fix' {{dir}}")
	fileDraft(t, a, "zone-auth", "tool:../evil | escape | ls")
	fileDraft(t, a, "zone-auth", "tool:runner | run anything | bash -c {{cmd}}")
	fileDraft(t, a, "zone-auth", "tool:pusher | push a branch | git push origin {{branch}}")
	fileDraft(t, a, "zone-auth", "tool:piped | count | grep x | wc -l")
	cfgBefore := w.read(".agent-one/config.yaml")

	code, out, errs := workingNotes(a, "register", idOf(t, a, "todo-grep"))
	if code != 0 || !strings.Contains(out, "tools.custom.todo-grep: {") || !strings.Contains(out, `"TODO: fix"`) {
		t.Fatalf("the YAML is printed before it is written: %d\n%s\n%s", code, out, errs)
	}
	y := w.read(".agent-one/config.yaml")
	if !strings.Contains(y, "TODO: fix") || !strings.Contains(y, "class: read") || !strings.Contains(y, "crafted:") || !strings.Contains(y, "draft:") || !strings.Contains(y, "dir:") {
		t.Fatalf("argv quoted as a shell would, class read from the classifier, provenance kept:\n%s", y)
	}
	if tl, ok := a.Cfg.Tools.Custom["todo-grep"]; !ok || len(tl.Run) != 4 || tl.Run[2] != "TODO: fix" || tl.Crafted == nil || tl.Crafted.By != "zone-auth" || tl.Crafted.Run != "run-1" {
		t.Fatalf("reloaded tools.custom.todo-grep: %+v", tl)
	}

	// the name is checked before any path is built
	code, _, errs = workingNotes(a, "register", idOf(t, a, "../evil"))
	if code == 0 || !strings.Contains(errs, "tools.custom's rule") {
		t.Fatalf("a bad name is refused: %d %s", code, errs)
	}
	// a shell pipeline is not an argv
	code, _, errs = workingNotes(a, "register", idOf(t, a, "piped"))
	if code == 0 || !strings.Contains(errs, "shell operator") {
		t.Fatalf("a pipeline is refused: %d %s", code, errs)
	}
	// interpreter-eval: refused, then forced
	runner := idOf(t, a, "runner")
	if code, _, errs = workingNotes(a, "register", runner); code == 0 || !strings.Contains(errs, "interpreter (bash -c)") {
		t.Fatalf("bash -c is refused unless forced: %d %s", code, errs)
	}
	if code, out, errs = workingNotes(a, "register", runner, "--force"); code != 0 {
		t.Fatalf("--force registers it: %d %s %s", code, out, errs)
	}
	// outward: headless needs --yes, and lands outward
	pusher := idOf(t, a, "pusher")
	if code, _, errs = workingNotes(a, "register", pusher); code == 0 || !strings.Contains(errs, "outward") || !strings.Contains(errs, "--yes") {
		t.Fatalf("an outward register asks the human gate: %d %s", code, errs)
	}
	if strings.Contains(w.read(".agent-one/config.yaml"), "pusher") {
		t.Fatal("a refused register wrote nothing")
	}
	if code, out, errs = workingNotes(a, "register", pusher, "--yes"); code != 0 || a.Cfg.Tools.Custom["pusher"] == nil || a.Cfg.Tools.Custom["pusher"].Class != "outward" {
		t.Fatalf("--yes lands it with its class: %d %s %s %+v", code, out, errs, a.Cfg.Tools.Custom["pusher"])
	}
	if !strings.Contains(cfgBefore, "models:") {
		t.Fatal("fixture")
	}
}

// The working notes: stable ids, reject with a why, a newer draft of a registered name shows as v2 and
// registers only with --version (the old file kept), never an overwrite.
func TestForgeWorkingNotesRejectAndVersions(t *testing.T) {
	w, a := forgeWorkspace(t)
	fileDraft(t, a, "zone-auth", "skill:ttl-craft | Use when the ttl moves | keep it at an hour")
	fileDraft(t, a, "zone-api", "skill:api-tour | Use when touring the api | read server.go")
	tour := idOf(t, a, "api-tour")
	if code, out, errs := workingNotes(a, "reject", tour, "--why", "too narrow"); code != 0 || !strings.Contains(out, "rejected") {
		t.Fatalf("reject: %d %s %s", code, out, errs)
	}
	var out, errw bytes.Buffer
	a.forgeCmd(&out, &errw, "list")
	if strings.Contains(out.String(), "api-tour") || !strings.Contains(out.String(), "ttl-craft") || !strings.Contains(out.String(), "zone-auth · run-1") {
		t.Fatalf("the rejected draft leaves; the list shows by · run · age · kind:\n%s", out.String())
	}
	marks := forgeNotes(t, w)
	if last := marks[len(marks)-1]; last.Tag != "forge-rejected" || !strings.HasPrefix(last.Hash, tour) || !strings.Contains(last.Text, "too narrow") {
		t.Fatalf("the reject mark names the hash and the why: %+v", last)
	}
	if code, _, _ := workingNotes(a, "register", idOf(t, a, "ttl-craft")); code != 0 {
		t.Fatal("v1 register")
	}
	v1 := w.read(".agent-one/crafted-skills/ttl-craft/SKILL.md")
	// a newer draft of the registered name: v2 on the working notes, refused with the diff, then --version
	fileDraft(t, a, "zone-auth", "skill:ttl-craft | Use when the ttl moves or is audited | keep it at an hour; log the change")
	_, list, _ := workingNotes(a, "list")
	if !regexp.MustCompile(`ttl-craft\s+v2`).MatchString(list) {
		t.Fatalf("a newer draft of a registered name shows as v2:\n%s", list)
	}
	id2 := idOf(t, a, "ttl-craft")
	code, out2, errs := workingNotes(a, "register", id2)
	if code == 0 || !strings.Contains(errs, "nothing overwritten") || !strings.Contains(out2, "  - description:") || !strings.Contains(out2, "  + description:") {
		t.Fatalf("an existing crafted skill is refused with the diff: %d\n%s\n%s", code, out2, errs)
	}
	if w.read(".agent-one/crafted-skills/ttl-craft/SKILL.md") != v1 {
		t.Fatal("the refusal overwrote")
	}
	if code, _, errs = workingNotes(a, "register", id2, "--version"); code != 0 {
		t.Fatalf("--version registers v2: %s", errs)
	}
	if s := w.read(".agent-one/crafted-skills/ttl-craft/SKILL.md"); !strings.Contains(s, "version: 2") || !strings.Contains(s, "log the change") {
		t.Fatalf("v2:\n%s", s)
	}
	if w.read(".agent-one/crafted-skills/ttl-craft/versions/v1.md") != v1 {
		t.Fatal("v1 is kept beside it")
	}
}

// a name another home already holds is refused: a crafted skill would shadow it.
func TestForgeSkillNeverShadows(t *testing.T) {
	w, a := forgeWorkspace(t)
	w.write(".claude/skills/ttl-craft/SKILL.md", "---\nname: ttl-craft\ndescription: the host's own\n---\nagent\n")
	fileDraft(t, a, "zone-auth", "skill:ttl-craft | Use when | how")
	fileDraft(t, a, "zone-auth", "skill:Bad_Name | Use when | how")
	code, _, errs := workingNotes(a, "register", idOf(t, a, "ttl-craft"))
	if code == 0 || !strings.Contains(errs, ".claude/skills/ttl-craft/SKILL.md") {
		t.Fatalf("shadowing refused: %d %s", code, errs)
	}
	if code, _, errs = workingNotes(a, "register", idOf(t, a, "Bad_Name")); code == 0 || !strings.Contains(errs, "[a-z0-9-]{1,64}") {
		t.Fatalf("Claude Code's name rule: %d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(w.root, ".agent-one/crafted-skills")); err == nil {
		if ents, _ := os.ReadDir(filepath.Join(w.root, ".agent-one/crafted-skills")); len(ents) > 0 {
			t.Fatalf("nothing registered: %v", ents)
		}
	}
}

// drafts filed before the hash rode the note still list, and a legacy "kind:name" mark still
// closes them.
func TestForgeLegacyLedger(t *testing.T) {
	w, a := forgeWorkspace(t)
	w.write(".agent-one/memory/shared/notes.jsonl", `{"at":"2026-09-28T21:00:00Z","by":"zone-auth","workspace":"x","tag":"forge","kind":"team","text":"tool:ui-wrangler | rewrite bare /v1 strings | ./wrangle {{path}}"}
{"at":"2026-09-28T21:01:00Z","by":"zone-auth","workspace":"x","tag":"forge","kind":"team","text":"skill:ttl-craft | one hour is the shelf life"}
{"at":"2026-09-28T21:02:00Z","by":"zone-auth","workspace":"x","tag":"forge-registered","kind":"team","text":"skill:ttl-craft"}
`)
	_, out, _ := workingNotes(a, "list")
	if !strings.Contains(out, "ui-wrangler") || strings.Contains(out, "ttl-craft") || !strings.Contains(out, "write") {
		t.Fatalf("legacy ledger:\n%s", out)
	}
}

// remember refuses the harness's own tags.
func TestRememberRefusesForgeTags(t *testing.T) {
	w, a := forgeWorkspace(t)
	for _, tag := range []string{"forge", "forge-registered", "skill", "Forge"} {
		res := a.rememberTool("zone-auth").Run(context.Background(), tool.Env{Root: a.Root, WorkspaceDir: ".agent-one"}, json.RawMessage(`{"text":"skill:x | y","tag":"`+tag+`"}`))
		if !res.Err || !strings.Contains(res.Output, "harness's own") {
			t.Fatalf("tag %s: %+v", tag, res)
		}
	}
	if strings.Contains(w.read(".agent-one/memory/shared/notes.jsonl"), "skill:x") {
		t.Fatal("nothing filed")
	}
	if res := a.rememberTool("zone-auth").Run(context.Background(), tool.Env{Root: a.Root, WorkspaceDir: ".agent-one"}, json.RawMessage(`{"text":"plain","tag":"mine"}`)); res.Err {
		t.Fatalf("other tags stand: %+v", res)
	}
}

// One registry: status's skills and the skill tool's known list include crafted and base skills;
// a name with two homes is a hole naming both.
func TestSkillRegistryIncludesCrafted(t *testing.T) {
	files := agentOneMembers()
	files[".agent-one/crafted-skills/ttl-craft/SKILL.md"] = "---\nname: ttl-craft\ndescription: crafted\n---\nthe crafted agent\n"
	files[".agent-one/base-skills/auth-tour/SKILL.md"] = "---\nname: auth-tour\ndescription: base\n---\nagent\n"
	files[".claude/skills/ttl-craft/SKILL.md"] = "---\nname: ttl-craft\ndescription: host copy\n---\nthe host agent\n"
	w := newTestWorkspace(t, "agent-one", files)
	w.script(".agent-one/tmp/s.json", text("nothing"))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	a := w.open()
	var out bytes.Buffer
	a.cmdStatus(IO{Out: &out, Err: &out})
	s := out.String()
	n := len(a.Found.Skills)
	if !strings.Contains(s, " "+strconv.Itoa(n)+" skills") || n < 3 {
		t.Fatalf("status counts %d skills:\n%s", n, s)
	}
	if !strings.Contains(s, `@? skill "ttl-craft" has two homes`) || !strings.Contains(s, ".agent-one/crafted-skills/ttl-craft/SKILL.md wins over") {
		t.Fatalf("two homes are a hole naming both:\n%s", s)
	}
	sk, ok := a.Found.Skill("ttl-craft")
	if !ok || sk.Source != "crafted" {
		t.Fatalf("crafted wins: %+v", sk)
	}
	res := a.skillTool("orchestrator").Run(context.Background(), tool.Env{Root: a.Root, WorkspaceDir: ".agent-one"}, json.RawMessage(`{"name":"nope"}`))
	if !strings.Contains(res.Output, "ttl-craft") || !strings.Contains(res.Output, "auth-tour") {
		t.Fatalf("the skill tool's known list: %s", res.Output)
	}
}

// the CLI working notes, headless: list, show, register --yes.
func TestForgeCLI(t *testing.T) {
	w, a := forgeWorkspace(t)
	fileDraft(t, a, "zone-auth", "skill:ttl-craft | Use when the ttl moves | keep it at an hour")
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
	code, out, errs := run("forge", "list")
	m := workingNotesID.FindStringSubmatch(out)
	if code != 0 || m == nil || m[2] != "ttl-craft" {
		t.Fatalf("forge list: %d\n%s\n%s", code, out, errs)
	}
	if code, out, errs = run("forge", "show", m[1]); code != 0 || !strings.Contains(out, "crafted-skills/ttl-craft/SKILL.md") {
		t.Fatalf("forge show: %d\n%s\n%s", code, out, errs)
	}
	if code, out, errs = run("forge", "register", m[1], "--version"); code != 0 || !strings.Contains(out, "registered skill:ttl-craft v1") {
		t.Fatalf("forge register (--version is the working notes's flag, not the binary's): %d\n%s\n%s", code, out, errs)
	}
	if code, out, _ = run("--help"); !strings.Contains(out, "forge") {
		t.Fatalf("help lists forge: %s", out)
	}
}

// Under Claude Code the Stop hook files the turn's final answer's drafts: the transcript's last
// assistant text (after the last user entry), by the session — once, however often stop runs.
func TestClaudeStopFilesTheFinalAnswersDrafts(t *testing.T) {
	w := claudeWorkspace(t)
	tp := filepath.Join(w.home, "transcript.jsonl")
	w.write("~/transcript.jsonl", `{"type":"user","message":{"role":"user","content":"tour the auth zone"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"@FORGE skill:early-idea | not final | x"},{"type":"tool_use","name":"Read"}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Toured.\n@FORGE skill:auth-tour | Use when touring src/auth | read login.go first"}]}}
`)
	w.hook("prompt", map[string]interface{}{"hook_event_name": "UserPromptSubmit", "prompt": "tour the auth zone"})
	for i := 0; i < 2; i++ {
		w.hook("stop", map[string]interface{}{"hook_event_name": "Stop", "transcript_path": tp})
	}
	notes := forgeNotes(t, w)
	if len(notes) != 1 || notes[0].By != "orchestrator" || notes[0].Run != "cc-1" || !strings.HasPrefix(notes[0].Text, "skill:auth-tour") {
		t.Fatalf("the final answer's draft, once: %+v", notes)
	}
	// the host's own field wins when it sends one
	w.hook("stop", map[string]interface{}{"hook_event_name": "Stop", "last_assistant_message": "@FORGE tool:auth-grep | grep auth | grep -rn auth src"})
	if notes = forgeNotes(t, w); len(notes) != 2 || !strings.HasPrefix(notes[1].Text, "tool:auth-grep") {
		t.Fatalf("last_assistant_message: %+v", notes)
	}
}

// policy.forge.demandAtSteps: 0 is reported as switched off, with where, like every policy.
func TestForgeDemandOffIsReported(t *testing.T) {
	_, a := forgeWorkspace(t) // its config sets policy.forge.demandAtSteps: 0
	var out bytes.Buffer
	a.cmdStatus(IO{Out: &out, Err: &out})
	if !strings.Contains(out.String(), "policy.forge.demandAtSteps") || !strings.Contains(out.String(), "config.yaml") {
		t.Fatalf("status names the switched-off policy and where:\n%s", out.String())
	}
	if a.Build().ForgeDemandAt != 0 {
		t.Fatal("the build carries the policy's value")
	}
}
