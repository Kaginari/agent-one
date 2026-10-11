package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
)

// hungProvider streams nothing and never returns until its context is cancelled —
// the exact shape of Operator's stuck subagents (`thinking · 22m`): a live SSE stream
// whose bytes trickle under every per-read timeout, so only a hard context clock
// can ever end it.
type hungProvider struct{}

func (hungProvider) Name() string { return "hung" }
func (hungProvider) Complete(ctx context.Context, req provider.Request) (provider.Response, error) {
	<-ctx.Done()
	return provider.Response{}, ctx.Err()
}

// The loop is supposed to protect exactly this: an agent whose provider goes silent
// (empty completes, no tools, no wire) can never spin forever. The wire guard is the
// protection as the policy is written today — an answer with no @S goes back wireRetries
// times, then the turn ends Done and undisciplined, the slip recorded as a hole
// (TestWireAnswerRetryBounded pins that contract). This test pins the termination
// guarantee: it never spins, no human has to kill it.
func TestEmptyCompleteStalls(t *testing.T) {
	for _, wire := range []bool{true, false} {
		name := "agent-on-the-wire"
		if !wire {
			name = "plain-turn-no-wire-policy"
		}
		t.Run(name, func(t *testing.T) {
			dir := fixture(t)
			w := open(t, dir)
			m := mock.New()
			m.Fallback = &provider.Response{} // silent forever: no text, no calls, no @S
			b := build(m)
			e := w.Engine("zone-auth", b)
			e.Wire = wire // a subagent sets this; the session does not
			done := make(chan *loop.Result, 1)
			go func() {
				r, err := e.Run(context.Background(), "list the auth files")
				if err != nil {
					t.Errorf("turn error: %v", err)
				}
				done <- r
			}()
			select {
			case r := <-done:
				if r == nil {
					t.Fatalf("no result")
				}
				if got := len(m.Requests); got > 3 {
					t.Fatalf("empty completes kept being accepted: %d requests, status %s", got, r.Status)
				}
				joined := strings.Join(r.Holes, "\n")
				if wire {
					// the policy as written (TestWireAnswerRetryBounded): the re-ask is spent,
					// then the agent stands undisciplined — Done off the wire, the hole the
					// only record. Whether that should be !Done is a policy decision.
					if r.Status != loop.Done || r.IsWire {
						t.Fatalf("silent agent, wire policy: %s isWire=%v\n%s", r.Status, r.IsWire, joined)
					}
					if !strings.Contains(joined, "wire") {
						t.Fatalf("the holes must record the wire slip: %s", joined)
					}
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("turn never returned after %d silent completes (wire=%v) — the loop's protection is dead", len(m.Requests), wire)
			}
		})
	}
}

// The clock-killed subagent dies WITH its knowledge: the grace window runs one journalise turn
// (resumed from its own transcript, MaxTokens capped at salvageTokens, Steps=1) whose wire
// report's @U lands in the shared notes. What the agent knew at T-grace is on disk before
// the parent even sees the FAIL.
func TestSubagentHangSalvagesKnowledge(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	salvage := mock.New(mock.Text(fmt.Sprintf("@S HOLE\n@F src/auth/login.go:1 token check lives here\n@U domain the whole auth zone is one file\n@E 0")))
	m := mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "zone-auth", "ask": "list the auth files", "scope": "src/auth"}),
		mock.Text("@S DONE\n@U team answered\n@E 0"),
	)
	b := build(m)
	b.Journal = ""          // journaling on: the worker must leave a transcript for the grace turn to resume from
	b.Budget.Minutes = 0.05 // 3s work clock → ~0.3s grace: prompt effect, not minutes realism
	b.Subagent.SalvageTokens = 512
	b.Models = func(r Route) provider.Provider {
		if r.Task == "salvage" {
			return salvage // the grace turn
		}
		if r.Task == "dispatch" {
			return hungProvider{} // the worker: streams open forever until its context dies
		}
		return m
	}
	e := w.Engine("domain-security", b)
	done := make(chan *loop.Result, 1)
	go func() {
		r, _ := e.Run(context.Background(), "@ASK findings\nwhat is in auth?")
		done <- r
	}()
	select {
	case r := <-done:
		if r == nil {
			t.Fatal("no result")
		}
		// the salvage provider saw the journalise ask on a resumed transcript
		found := false
		for _, req := range salvage.Requests {
			for _, msg := range req.Messages {
				if strings.Contains(msg.Text, "spend") || strings.Contains(msg.Text, "@U") || strings.Contains(msg.Text, "out of time") {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("no journalise turn reached the provider; requests: %d", len(salvage.Requests))
		}
		// the unsaid must have landed in the shared notes, not died with the subagent
		data, err := os.ReadFile(filepath.Join(dir, ".agent-one", "memory", "shared", "notes.jsonl"))
		if err != nil || !strings.Contains(string(data), "whole auth zone is one file") {
			t.Fatalf("salvaged @U never landed: %v\n%s", err, data)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the subagent never died: hang + grace never terminated")
	}
}

// the session's wall-clock, but a Subagent still carries a hard clock: when the model's stream
// hangs open forever, the deadline cancels mid-Complete — the parent is not left watching a
// spinner for 22 minutes. The provider here never returns except by context, exactly the SSE
// shape those stuck subagents had.
func TestSubagentHangDiesByClock(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)
	m := mock.New(
		mock.Call("d1", "dispatch", map[string]interface{}{"agent": "zone-auth", "ask": "list the auth files", "scope": "src/auth"}),
		mock.Text("@S DONE\n@U team x\n@E 0"),
	)
	b := build(m)
	b.Budget.Minutes = 0.05       // 3 seconds: the parent's ceiling, inherited by the Subagent — the loop's own convention (loop_test.go) for forcing a clock
	b.Subagent.SalvageTokens = -1 // pin the clock death itself: no salvage turn inside the grace window
	b.Models = func(r Route) provider.Provider {
		if r.Task == "dispatch" {
			return hungProvider{} // the agent's model: streams open forever, like the stuck gateway calls
		}
		return m // orchestrator's own route stays on the script
	}
	e := w.Engine("domain-security", b)
	done := make(chan *loop.Result, 1)
	go func() {
		r, _ := e.Run(context.Background(), "@ASK findings\nwhat is in auth?")
		done <- r
	}()
	select {
	case r := <-done:
		// the honest pin: the run ENDED through a hung Complete that nothing else in the chain
		// can cut — provider http.Timeout, keepalive and stream reads all stay blind while SSE
		// bytes trickle; only this clock reaches inside Complete. Under budget.minutes=0.05 the
		// parent's own wall-clock fires too, so we assert termination + never-Done, not who spoke.
		if r == nil {
			t.Fatal("no result")
		}
		t.Logf("run ended through the hung subagent: status=%s turns=%d", r.Status, r.Turns)
		if r.Status == loop.Done {
			t.Fatalf("a run through a hung subagent must not end Done: %+v", r)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("a hung subagent never dies: the dispatcher is still blocked")
	}
}
