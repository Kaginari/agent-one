package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Subagent-card exit-verdict policy (harness leg 2b): the dispatch tool stamps the run journal's
// exit verdict (t:end, carried on DispatchReport.Status) as the payload's first @S line, so a
// renderer that reads only the returned string can never show a bare lifecycle "done" over a
// FAILed journal (the 2026-09-28 Policy-9 break). The agent's own self-judged @S stays as context.
func TestDispatchReportStampsExitVerdict(t *testing.T) {
	fired := false
	tool := DispatchTool(DispatchOptions{Enabled: true, Agents: []string{"domain-x"}},
		func(_ context.Context, _ Env, _ Commission) (DispatchReport, error) {
			fired = true
			return DispatchReport{Text: "@S PASSED\n@F src/x.go:1 ok\n@E 30", Status: "FAIL", Unsaid: 1, Failed: true}, nil
		})
	res := tool.Run(context.Background(), Env{}, json.RawMessage(`{"agent":"domain-x","ask":"findings: x"}`))
	if !fired || !res.Err {
		t.Fatalf("the dispatch must run and surface the failure: fired=%v err=%v", fired, res.Err)
	}
	first, _, _ := strings.Cut(res.Output, "\n")
	if first != "@S FAIL" {
		t.Fatalf("the payload's first line must be the journal's exit verdict, got %q\nfull:\n%s", first, res.Output)
	}
	if !strings.Contains(res.Output, "@S PASSED") {
		t.Fatalf("the agent's self-judged line stays as context:\n%s", res.Output)
	}

	clean := DispatchTool(DispatchOptions{Enabled: true}, func(_ context.Context, _ Env, _ Commission) (DispatchReport, error) {
		return DispatchReport{Text: "@S PASSED\n@E 3", Status: "DONE", Unsaid: 1}, nil
	}).Run(context.Background(), Env{}, json.RawMessage(`{"agent":"domain-x","ask":"findings: x"}`))
	if first, _, _ := strings.Cut(clean.Output, "\n"); first != "@S DONE" {
		t.Fatalf("a green journal is stamped `@S DONE` — the agent's own @S (self-judged) is not the verdict:\n%s", clean.Output)
	}
	if strings.Contains(clean.Output, "@? dispatch") {
		t.Fatalf("Unsaid=1 must silence the no-@U nudge:\n%s", clean.Output)
	}
}

// tools.dispatch.sessionBackground: at the session's own depth a commission with no background
// word rides in the background — the orchestrator stays free and the report wakes it. The commission
// may still say background:false; one layer down (a Subagent's own dispatch), the default is dead.
func TestSessionBackgroundDefaults(t *testing.T) {
	woke := make(chan string, 4)
	runner := func(_ context.Context, _ Env, _ Commission) (DispatchReport, error) {
		return DispatchReport{Text: "@S DONE\n@E 3", Status: "DONE", Unsaid: 1}, nil
	}
	opt := DispatchOptions{Enabled: true, Background: true, SessionBackground: true, Wake: func(agent, report string, _ bool) { woke <- agent }}
	tool := DispatchTool(opt, runner)

	bg := tool.Run(context.Background(), Env{}, json.RawMessage(`{"agent":"domain-x","ask":"x"}`))
	if !strings.Contains(bg.Output, "in the background") {
		t.Fatalf("a session commission with no background word rides background:\n%s", bg.Output)
	}
	select {
	case <-woke:
	case <-time.After(2 * time.Second):
		t.Fatal("the background Subagent's report never woke the session")
	}

	fg := tool.Run(context.Background(), Env{}, json.RawMessage(`{"agent":"domain-x","ask":"x","background":false}`))
	if strings.Contains(fg.Output, "in the background") || !strings.HasPrefix(fg.Output, "@S DONE") {
		t.Fatalf("background:false must ride the foreground:\n%s", fg.Output)
	}

	deep := DispatchTool(DispatchOptions{Enabled: true, Depth: 1, MaxDepth: 3, Background: true, SessionBackground: true, Wake: opt.Wake}, runner)
	out := deep.Run(context.Background(), Env{}, json.RawMessage(`{"agent":"domain-x","ask":"x"}`))
	if !strings.HasPrefix(out.Output, "@S DONE") {
		t.Fatalf("one layer down, a Subagent's own dispatch stays foreground by design:\n%s", out.Output)
	}

	// background off → the default is dead too (a background ride would refuse, not surprise)
	off := DispatchTool(DispatchOptions{Enabled: true, Background: false, SessionBackground: true}, runner)
	out = off.Run(context.Background(), Env{}, json.RawMessage(`{"agent":"domain-x","ask":"x"}`))
	if !strings.HasPrefix(out.Output, "@S DONE") {
		t.Fatalf("background allowance off must keep the foreground default:\n%s", out.Output)
	}
}
