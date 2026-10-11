package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/tui"
)

// TestDashGateAnswerOverHTTP proves the gate's approval survives a real HTTP round-trip:
// the choice id is discovered on the bus, POSTed to /api/answer over the wire (as a browser
// would against the board serveBoard mounts), and the approval must resolve the gate
// (journal `"by":"tty"`). A second POST to the same id must return "answered elsewhere"
// promptly — first-wins over HTTP, no deadlock.
//
// This closes the gap TestDashGateAnswerFlow leaves: that test calls bus.Answer in-process
// and never touches the /api/answer handler that board/live.go serves.
func TestDashGateAnswerOverHTTP(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		text("push attempted"),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	a := w.open()
	bus := a.webBus()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := a.tuiHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h.attach(bus)
	bus.attach(h)

	go func() {
		if why := h.Submit("push it"); why != "" {
			t.Logf("submit why: %s", why)
		}
	}()

	// Discover the choice id on the bus (the browser would see it via the dash event stream).
	var gotID string
	deadline := time.After(15 * time.Second)
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
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Logf("choice id landed: %s", gotID)

	// Answer over real HTTP as a browser would: POST /api/answer {id, index, text}.
	// serveBoard mounts /api/answer on the board's mux; the test exercises the full wire,
	// not the in-process Answer entry point.
	addr := a.boardAddr
	if addr == "" {
		a.serveBoard(ctx)
		// serveBoard sets a.boardAddr under mu.
		for i := 0; i < 100 && addr == ""; i++ {
			a.mu.Lock()
			addr = a.boardAddr
			a.mu.Unlock()
			if addr == "" {
				time.Sleep(10 * time.Millisecond)
			}
		}
		if addr == "" {
			t.Fatal("board never came up")
		}
	}

	// The board's acts are token-guarded (live.go act): carry X-Board-Token like the
	// dashboard page does. The token comes from the LIVE board serveBoard mounted
	// (retained on the App), never a second board.New — a fresh board would register a
	// fresh token and the guard would (correctly) refuse it.
	tok := a.boardToken(ctx)

	post := func(id string, index int, text string) (int, string) {
		body, _ := json.Marshal(map[string]any{"id": id, "index": index, "text": text})
		req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/api/answer", bytes.NewReader(body))
		if err != nil {
			return 0, err.Error()
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Board-Token", tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err.Error()
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}

	code, _ := post(gotID, 0, "")
	if code != http.StatusOK && code != http.StatusNoContent {
		t.Fatalf("first answer POST not accepted: %d", code)
	}

	// Second POST to the same id must refuse PROMPTLY (first-wins over HTTP, never block).
	// The first Answer consumed the id (web.go deletes it under mu), so the late POST gets
	// the bus's pinned late-answer refusal "no longer open" (web.go:149, same wording
	// TestWebBusTerminalWinNoHang pins); "answered elsewhere" is the terminal-wins arm.
	var code2 int
	var agent2 string
	errc := make(chan struct{})
	go func() { code2, agent2 = post(gotID, 0, ""); close(errc) }()
	select {
	case <-errc:
	case <-time.After(2 * time.Second):
		t.Fatal("second answer POST blocked — the HTTP handler hung")
	}
	if code2 != http.StatusConflict {
		t.Fatalf("second answer POST should refuse 409; code=%d", code2)
	}
	if agent2 == "" || !strings.Contains(agent2, "no longer open") {
		t.Fatalf("second answer should report the late-answer refusal; code=%d agent=%q", code2, agent2)
	}

	// The approval must reach the journal (the gate resolves).
	j := w.journalText(".agent-one")
	dl := time.After(15 * time.Second)
	for !strings.Contains(j, `"by":"tty"`) {
		select {
		case <-dl:
			t.Fatalf("the HTTP-routed approval never reached the journal; journal:\n%s", j)
		case <-time.After(50 * time.Millisecond):
			j = w.journalText(".agent-one")
		}
	}
	t.Log("gate approved over HTTP — journal carries the approval")
}

// Compile-check: the ChoiceAnswer route this test drives.
var _ = tui.ChoiceAnswer{}
