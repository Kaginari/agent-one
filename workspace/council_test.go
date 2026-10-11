package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
)

// The council (policy.stress): an agent whose runs never land accrues strain, and past escalateAt a
// warrant rides the report back to its dispatcher. The warrants an agent carries are bounded by
// maxConsults — past that the report hardens to @S ESCALATE and rises. A clean landing clears
// the ledger. The policy is counted and demanded in code; the manager's answer stays his own.
func TestSubagentCouncilWarrant(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)

	failText := "@S FAIL\n@F the landing broke\n@U team wire-policy holds — FAIL reports owe the line too\n@E 3"
	cleanText := "@S DONE\n@F src/auth/login.go:1 landed\n@U team c\n@E 0"
	// the script: the domain owner dispatches the zone worker eleven times; the zone worker fails ten, lands the
	// tenth. Each dispatch rides two mock entries: the parent's call, then the child's answer.
	childFails := []string{
		failText, failText, failText, failText, // ledger 1..4 — silent
		failText,  // 5  → warrant, consult 1 of 3
		failText,  // 6  → warrant, consult 2
		failText,  // 7  → warrant, consult 3
		failText,  // 8  → @S ESCALATE, council exhausted
		failText,  // 9  → still escalated
		cleanText, // 10 → clean: ledger cleared
		failText,  // 11 → ledger 1 again: silent
	}
	var script []provider.Response
	for i, child := range childFails {
		script = append(script,
			mock.Call(fmt.Sprintf("d%d", i+1), "dispatch", map[string]interface{}{"agent": "zone-auth", "ask": "land the token change"}),
			mock.Text(child))
	}
	script = append(script, mock.Text("@S DONE\n@U team the ledger was watched\n@E 0"))
	m := mock.New(script...)
	b := build(m)
	b.Budget.Steps = -1
	b.Subagent.StressEscalateAt, b.Subagent.StressMaxConsults = 5, 3
	e := w.Engine("domain-security", b)
	r, err := e.Run(context.Background(), "land the token change through the zone worker")
	if err != nil {
		t.Fatal(err)
	}

	// dispatch i (0-based) answers into the parent's request 2+2i: its last message is the
	// tool result of that dispatch (req0 the domain owner's first call, req1 the zone worker's, req2 the
	// domain owner's request carrying dispatch 0's result, …)
	stepResult := func(i int) string {
		req := m.Requests[2+2*i]
		mrs := req.Messages[len(req.Messages)-1].ToolResults
		if len(mrs) == 0 {
			t.Fatalf("dispatch %d: no tool result in request %d (%d messages)", i+1, 2+2*i, len(req.Messages))
		}
		return mrs[0].Content
	}
	for i := 0; i < 4; i++ {
		if out := stepResult(i); strings.Contains(out, "council warrant") {
			t.Fatalf("dispatch %d: the ledger must stay silent below escalate-at:\n%s", i+1, out)
		}
	}
	if out := stepResult(4); !strings.Contains(out, "@? council warrant") || !strings.Contains(out, "strain 5 (escalate-at 5)") || !strings.Contains(out, "consult 1 of 3") || !strings.Contains(out, "@INSTRUCT") || !strings.Contains(out, "@SPLIT") || !strings.Contains(out, "@ESCALATE") {
		t.Fatalf("dispatch 5: the first warrant must name strain, consult 1/3 and the verbs:\n%s", out)
	}
	if out := stepResult(5); !strings.Contains(out, "consult 2 of 3") {
		t.Fatalf("dispatch 6: consults count across re-dispatches:\n%s", out)
	}
	if out := stepResult(6); !strings.Contains(out, "consult 3 of 3") {
		t.Fatalf("dispatch 7:\n%s", out)
	}
	out8 := stepResult(7)
	if first, _, _ := strings.Cut(out8, "\n"); first != "@S ESCALATE" {
		t.Fatalf("dispatch 8: past maxConsults the report ends @S ESCALATE:\n%s", out8)
	}
	if !strings.Contains(out8, "council exhausted") {
		t.Fatalf("dispatch 8: the exhausted warrant names itself:\n%s", out8)
	}
	if out := stepResult(10); strings.Contains(out, "council warrant") {
		t.Fatalf("a clean landing clears the ledger; the next failure starts it at one:\n%s", out)
	}
	if r.Status != "DONE" {
		t.Fatalf("the parent's own run still lands: %s", r.Status)
	}
}

// Forging drafts: a Subagent's @FORGE lines ride the report (DispatchReport carries them) and land
// as workspace notes of kind "forge" — filed, never registered; the working notes registers.
func TestForgingDraftsRideAndLand(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	m := mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "zone-auth", "ask": "x"}),
		mock.Text("@S DONE\n@F src/auth/login.go:1 landed\n@FORGE tool:token-wrangler | rewrite tokens safe | ./wrangle {{path}}\n@FORGE skill:ttl-craft | one hour is the shelf life\n@U team forgings verified\n@E 12"),
		mock.Text("@S DONE\n@U team watched\n@E 0"),
	)
	b := build(m)
	b.Budget.Steps = -1
	e := w.Engine("domain-security", b)
	if _, err := e.Run(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	res := m.Requests[2].Messages[2].ToolResults[0]
	if !strings.Contains(res.Content, "@FORGE tool:token-wrangler | rewrite tokens safe | ./wrangle {{path}}") {
		t.Fatalf("the draft rides the report up:\n%s", res.Content)
	}
	// landed home: the forge ledger holds both drafts
	notes, err := os.ReadFile(filepath.Join(dir, ".agent-one", "memory", "shared", "notes.jsonl"))
	if err != nil {
		t.Fatalf("the ledger file: %v", err)
	}
	got := string(notes)
	if !strings.Contains(got, `"tag":"forge"`) || !strings.Contains(got, `"kind":"team"`) || !strings.Contains(got, "tool:token-wrangler | rewrite tokens safe | ./wrangle {{path}}") || !strings.Contains(got, "skill:ttl-craft") {
		t.Fatalf("the ledger holds both drafts:\n%s", got)
	}
}
