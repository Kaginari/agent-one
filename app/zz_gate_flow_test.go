package app

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Repro (bug commission via coord-agent-one): the dash session (answerable bus, host attached) —
// a gate ask must reach the bus with an id the browser can answer, and the answered reply
// must resolve approve().
func TestDashGateAnswerFlow(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		text("push attempted"),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	a := w.open()
	// the bus answers from provisioning now (newWebBus): the dash wiring needs no flag flip — the
	// browser resolves the gate in every run mode, first answer wins
	bus := a.webBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := a.tuiHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h.attach(bus)
	bus.attach(h)

	// run a turn that asks at the gate
	done := make(chan struct{})
	go func() {
		if why := h.Submit("push it"); why != "" {
			t.Logf("submit why: %s", why)
		}
		close(done)
	}()
	var gotID string
	deadline := time.After(10 * time.Second)
	for gotID == "" {
		bus.mu.Lock()
		for id := range bus.choices {
			gotID = id
		}
		bus.mu.Unlock()
		if gotID != "" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no choice id landed on the bus — the dashboard could never answer")
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Logf("choice id landed: %s", gotID)
	// answer it as the browser would (board /api/answer → webBus.Answer)
	if err := bus.Answer(gotID, 0, ""); err != nil {
		t.Fatal(err)
	}
	j := w.journalText(".agent-one")
	dl := time.After(10 * time.Second)
	for !strings.Contains(j, `"by":"tty"`) {
		select {
		case <-dl:
			t.Fatalf("the approval never reached the journal; journal:\n%s", j)
		case <-time.After(20 * time.Millisecond):
			j = w.journalText(".agent-one")
		}
	}
	// the turn must be fully over before teardown — the running loop still writes
	// (.agent-one/instruments/...) while TempDir RemoveAll sweeps; that rank is the
	// "directory not empty" TempDir flake the commission predicted
	<-done
	waitTurns(t, h)
}

// waitTurns holds a test until every turn the host started has finished, finish included —
// a turn still writing while TempDir's RemoveAll sweeps recreates the workspace after it, and
// the directory leaks.
func waitTurns(t *testing.T, h *tuiHost) {
	t.Helper()
	idle := make(chan struct{})
	go func() { h.turns.Wait(); close(idle) }()
	select {
	case <-idle:
	case <-time.After(30 * time.Second):
		t.Error("a turn did not finish within 30s — its writes outlive the test's temp dir")
	}
}
