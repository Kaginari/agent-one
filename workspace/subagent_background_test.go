package workspace

import (
	"strings"
	"testing"
)

// backgroundWake depth semantics (Operator order 2026-10-03: "make background dispatch
// possible — each agent has its own ownership"): the session keeps the session queue; a
// deeper agent's background report wakes its OWN dispatcher's inbox via Deliver; without
// Deliver wired, background stays refused at depth rather than orphaning the report on the
// wrong member.
func TestBackgroundWakeDepthSemantics(t *testing.T) {
	sessionWoke := []string{}
	b := Build{Depth: 0, Subagent: SubagentOptions{
		Background: true,
		Wake:       func(agent, report string, failed bool) { sessionWoke = append(sessionWoke, agent) },
		Noted:      func(agent, report string, failed bool) {},
		Deliver:    func(agent, line string) {},
	}}

	// depth 0: session queue as always
	bg, wake := b.backgroundWake("orchestrator")
	if !bg || wake == nil {
		t.Fatal("depth 0 must keep background with the session wake")
	}
	wake("coord-agent-one", "@S DONE", false)
	if len(sessionWoke) != 1 || sessionWoke[0] != "coord-agent-one" {
		t.Fatalf("session wake = %v", sessionWoke)
	}

	// depth 1 + Deliver: report rides the dispatcher's inbox, noted first
	noted, delivered := []string{}, map[string]string{}
	b.Depth = 1
	b.Subagent.Noted = func(agent, report string, failed bool) { noted = append(noted, agent) }
	b.Subagent.Deliver = func(agent, line string) { delivered[agent] = line }
	bg, wake = b.backgroundWake("coord-agent-one")
	if !bg || wake == nil {
		t.Fatal("depth ≥1 with Deliver must offer background")
	}
	wake("domain-workspace", "@S DONE\n@F landed", false)
	if len(noted) != 1 || noted[0] != "domain-workspace" {
		t.Fatalf("noted = %v", noted)
	}
	line, ok := delivered["coord-agent-one"]
	if !ok || !strings.HasPrefix(line, "[subagent domain-workspace reported]\n@S DONE") {
		t.Fatalf("delivered to dispatcher = %v", delivered)
	}
	if len(sessionWoke) != 1 {
		t.Fatalf("session queue must stay untouched at depth: %v", sessionWoke)
	}

	// depth 1 without Deliver: refused rather than orphaned
	b.Subagent.Deliver = nil
	bg, wake = b.backgroundWake("coord-agent-one")
	if bg || wake != nil {
		t.Fatal("depth ≥1 without Deliver must refuse background (bg=false, wake=nil)")
	}

	// background off by config: refused at the session too
	b.Depth, b.Subagent.Background, b.Subagent.Deliver = 0, false, func(agent, line string) {}
	bg, _ = b.backgroundWake("orchestrator")
	if bg {
		t.Fatal("tools.dispatch.background: false must refuse at every depth")
	}
}
