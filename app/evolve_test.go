package app

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/tui"
	"github.com/Kaginari/agent-one/workspace"
)

// policy.evolution through the app: config arms the workspace (mode, working notes limit, the founding sketch's
// survey); the run-start sweep weighs a stressed agent; propose mode queues; the working notes lists,
// shows and applies; status and the dashboard's stream carry it.
func TestEvolutionThroughTheApp(t *testing.T) {
	files := agentOneMembers()
	files[".agent-one/zone/auth/README.md"] = "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- **Verify:** `test -d src/auth`\n\n## Invariants\n- tokens expire after one hour\n\n## Working notes\n" +
		"- 2026-10-01 refresh: always run the token test first\n- 2026-10-02 refresh ranks the logout\n- 2026-10-03 refresh: use the clock helper\n- 2026-10-04 refresh window is 5m\n"
	files["go.mod"] = "module example.com/w\n\ngo 1.21\n"
	files["src/auth/login_test.go"] = "package auth\n\nimport \"testing\"\n\nfunc TestLogin(t *testing.T) {}\n"
	w := newTestWorkspace(t, "agent-one", files)
	w.write(".agent-one/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true}}\ntools: {bash: {sandbox: none}}\nui: {board: {autostart: false}}\npolicy: {evolution: {mode: propose, notesMax: 3, failStreak: 2}}\n")
	a := w.open()
	o := a.Workspace.Evolution()
	if !o.Enabled || o.Mode != "propose" || o.NotesMax != 3 || o.StressAt != 5 || o.Survey == nil || o.On == nil {
		t.Fatalf("config arms the workspace: %+v", o)
	}
	sv := a.surveyOwnership([]string{"src/auth/"})
	if sv.Files != 2 || len(sv.Verify) != 1 || !strings.Contains(sv.Verify[0], "go test ./src/auth/") || len(sv.Invariants) == 0 || !strings.HasPrefix(sv.Invariants[0], "sketch: Go package auth") {
		t.Fatalf("the founding sketch reads the ownership: %+v", sv)
	}
	var heard []tui.EvEvolution
	a.evoSink = func(e tui.EvEvolution) { heard = append(heard, e) }
	for i := 0; i < 2; i++ {
		a.Workspace.FoldRun(workspace.RunFold{Agent: "zone-auth", Loop: "DONE", Word: "FAIL", Gate: "fail"})
	}
	a.evolveSweep()
	if len(heard) != 1 || heard[0].Status != "proposed" || heard[0].Arm != "mutate" || heard[0].Agent != "zone-auth" {
		t.Fatalf("the sweep proposes and the surfaces hear it: %+v", heard)
	}
	id := heard[0].ID
	var out, errw bytes.Buffer
	io := IO{Out: &out, Err: &errw}
	if code := a.cmdEvolve([]string{"list"}, io); code != 0 || !strings.Contains(out.String(), id) || !strings.Contains(out.String(), "1 open proposal") {
		t.Fatalf("evolve list: %d %s %s", code, out.String(), errw.String())
	}
	out.Reset()
	if code := a.cmdEvolve([]string{"show", id[:6]}, io); code != 0 || !strings.Contains(out.String(), "+- refresh — ") {
		t.Fatalf("evolve show carries the diff: %d\n%s", code, out.String())
	}
	out.Reset()
	if code := a.cmdEvolve([]string{"apply", id}, io); code != 0 || !strings.Contains(out.String(), "@S OK zone-auth mutated its configuration") {
		t.Fatalf("evolve apply: %d %s %s", code, out.String(), errw.String())
	}
	doc := w.read(".agent-one/zone/auth/README.md")
	if !strings.Contains(doc, "- refresh — ") || !strings.Contains(doc, "zone-auth-refresh") {
		t.Fatalf("applied — the invariant and the registered how:\n%s", doc)
	}
	out.Reset()
	if code := a.cmdStatus(io); code != 0 || !strings.Contains(out.String(), "evolution: on · mode propose") || !strings.Contains(out.String(), "zone-auth applied: mutate") {
		t.Fatalf("status shows the evolution:\n%s", out.String())
	}
	ev := webEvent(heard[len(heard)-1])
	if ev["type"] != "evolution" || ev["agent"] != "zone-auth" || ev["status"] != "applied" {
		t.Fatalf("the dashboard's stream carries it: %v", ev)
	}
	if code := a.cmdEvolve([]string{"reject", id, "late"}, io); code == 0 {
		t.Fatal("an applied proposal is not rejected")
	}
}

// The subagent seam end to end: the orchestrator dispatches a zone worker, the subagent reports FAIL, the strain
// crosses stressAt, its working notes is past the limit — it evolves inside the dispatch, the report says
// so, and the orchestrator's own gate never takes the harness's writes for its own.
func TestEvolutionAtTheSubagentSeam(t *testing.T) {
	files := agentOneMembers()
	var workingNotes []string
	for i := 1; i <= 8; i++ {
		workingNotes = append(workingNotes, fmt.Sprintf("- 2026-10-%02d session: run the token test before the refresh (%d)", i, i))
	}
	files[".agent-one/zone/auth/README.md"] = "# zone-auth\n\n- **Rank:** Zone worker\n- **Owns:** `src/auth/`\n- **Reports to:** domain-security\n- **Verify:** `test -d src/auth`\n\n## Working notes\n" + strings.Join(workingNotes, "\n") + "\n"
	w := newTestWorkspace(t, "agent-one", files)
	w.script(".agent-one/tmp/s.json",
		when("go", map[string]interface{}{"calls": []interface{}{map[string]interface{}{"name": "dispatch", "input": map[string]string{"agent": "zone-auth", "ask": "settle the login"}}}}),
		when("settle the login", text("@S FAIL\n@? the login would not settle\n@U team nothing new\n@E 10")),
		text("noted"),
	)
	w.write(".agent-one/config.yaml", "models: {default: mock/m}\nproviders: {mock: {enabled: true, script: .agent-one/tmp/s.json}}\ntools: {bash: {sandbox: none}, dispatch: {strictFlow: false, lineage: false}}\nui: {board: {autostart: false}}\npolicy: {evolution: {stressAt: 1}}\n")
	a := w.open()
	e, err := a.Engine()
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Steps[0].Result.Output, "[evolution] zone-auth mutated its configuration") {
		t.Fatalf("the subagent's report carries the evolution:\n%s", r.Steps[0].Result.Output)
	}
	for _, h := range r.Holes {
		if strings.Contains(h, "gate") {
			t.Fatalf("the orchestrator's gate took the harness's writes for its own: %v", r.Holes)
		}
	}
	if doc := w.read(".agent-one/zone/auth/README.md"); !strings.Contains(doc, "- refresh — ") || !strings.Contains(doc, "zone-auth-refresh") {
		t.Fatalf("the configuration mutated:\n%s", doc)
	}
	if !strings.Contains(w.read(".agent-one/log.md"), "evolution — zone-auth mutated its configuration") {
		t.Fatal("logged")
	}
}
