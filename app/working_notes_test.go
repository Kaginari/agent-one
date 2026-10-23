package app

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/workspace"
)

// A subagent's working notes are its own run's file — the one the drainer and a resume read — and
// the orchestrator keeps the session's: no agent writes over another's working memory.
func TestWorkingNotesIsTheRunsOwn(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/config.yaml", "ui: {board: {autostart: false}}\n")
	a := w.open()
	add := func(agent, run, note string) {
		t.Helper()
		in, _ := json.Marshal(map[string]string{"add": note})
		if r := a.workingNotesTool(agent).Run(context.Background(), tool.Env{Root: a.Root, RunID: run}, in); r.Err {
			t.Fatal(r.Output)
		}
	}
	add(workspace.Orchestrator, "orchestrator-run", "the orchestrator's plan")
	add("zone-auth", "zone-auth-r1", "the subagent's finding")
	add("zone-api", "zone-api-r2", "another subagent's finding")
	for path, want := range map[string]string{
		a.workingNotesPath(a.SessionID):    "the orchestrator's plan",
		a.workingNotesPath("zone-auth-r1"): "the subagent's finding",
		a.workingNotesPath("zone-api-r2"):  "another subagent's finding",
	} {
		b, err := os.ReadFile(path)
		if err != nil || strings.Count(string(b), "\n- ") != 1 || !strings.Contains(string(b), want) {
			t.Errorf("%s: %q %v, want only %q", path, b, err, want)
		}
	}
}
