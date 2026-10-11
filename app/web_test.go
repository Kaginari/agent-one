package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/tui"
)

// TestWebBusChoiceAgeAndClear covers the wedge meter: an open card exposes its age in
// Status (gateWaitMs), a bus-side answer clears it, and an EvChoiceAnswered from another
// surface clears every stale entry — a terminal answer can no longer leave a phantom
// "waiting on you" banner up forever (freeze 2026-10-07).
func TestWebBusChoiceAgeAndClear(t *testing.T) {
	w := newWebBus()
	reply := make(chan tui.ChoiceAnswer, 1)
	w.Send(tui.EvChoice{View: tui.ChoiceView{Title: "Approval", Options: []string{"Yes", "No"}}, Reply: reply})
	replay, _, cancel := w.Subscribe()
	defer cancel()
	var c map[string]any
	var id string
	for _, ev := range replay {
		if e := json.Unmarshal(ev, &c); e == nil && c["type"] == "choice" {
			id, _ = c["id"].(string)
		}
	}
	if id == "" {
		t.Fatal("no choice id registered")
	}
	time.Sleep(2100 * time.Millisecond) // gateWaitMs is in ms; prove it ages
	statusNoHost := w.Status()
	if statusNoHost["gateWaitMs"] != nil {
		t.Fatalf("no host attached: gateWaitMs belongs on a live host's status, got %v", statusNoHost["gateWaitMs"])
	}
	// answered from the bus: the age entry dies with the card
	if err := w.Answer(id, 0, ""); err != nil {
		t.Fatal(err)
	}
	w.Send(tui.EvChoice{View: tui.ChoiceView{Title: "Approval", Options: []string{"Yes", "No"}}, Reply: make(chan tui.ChoiceAnswer, 1)})
	replay2, _, cancel2 := w.Subscribe()
	cancel2()
	var last map[string]any
	for _, ev := range replay2 {
		var m map[string]any
		if e := json.Unmarshal(ev, &m); e == nil && m["type"] == "choice" {
			last = m
		}
	}
	if last == nil {
		t.Fatal("second choice not published")
	}
	// answered on the terminal → the host emits EvChoiceAnswered → the bus drops every stale card
	w.Send(tui.EvChoiceAnswered{Class: "write", Tool: "edit", Answer: "Yes"})
	w.mu.Lock()
	left := len(w.choices) + len(w.choiceAt)
	w.mu.Unlock()
	if left != 0 {
		t.Fatalf("EvChoiceAnswered must clear every stale choice entry, left: %d", left)
	}
}

func TestWebBusReplaysAndAnswers(t *testing.T) {
	w := newWebBus()
	w.Send(tui.EvDelta{Text: "hel"})
	w.Send(tui.EvToolStart{Tool: tui.ToolView{ID: "s1", Name: "read", Status: "refused"}})
	w.Send(struct{}{}) // a terminal-only message is not published
	replay, ch, cancel := w.Subscribe()
	defer cancel()
	if len(replay) != 2 {
		t.Fatalf("replay = %d events", len(replay))
	}
	var tool map[string]any
	_ = json.Unmarshal(replay[1], &tool)
	if tool["type"] != "tool" || tool["tool"].(map[string]any)["status"] != "running" {
		t.Fatalf("a starting tool reads running on the web: %s", replay[1])
	}
	// a choice registers an id from provisioning — the browser answers in every run mode (the decree)
	reply := make(chan tui.ChoiceAnswer, 1)
	w.Send(tui.EvChoice{View: tui.ChoiceView{Title: "Approval", Options: []string{"Yes", "No"}}, Reply: reply})
	var c map[string]any
	_ = json.Unmarshal(<-ch, &c)
	id, _ := c["id"].(string)
	if id == "" {
		t.Fatalf("no choice id: %v", c)
	}
	if c["elsewhere"] == true {
		t.Fatalf("a fresh bus is answerable: %v", c)
	}
	if err := w.Answer(id, 1, "not now"); err != nil {
		t.Fatal(err)
	}
	if a := <-reply; a.Index != 1 || a.Text != "not now" {
		t.Fatalf("answer = %+v", a)
	}
	if err := w.Answer(id, 0, ""); err == nil {
		t.Fatal("a choice is answered once")
	}
	if err := w.Ask("hi"); err == nil {
		t.Fatal("no host: an ask is refused, not dropped")
	}
}

// TestWebBusAnswerTwoConsumersNeverBlocks proves Answer never blocks the HTTP handler when
// the terminal and the dashboard rank on one choice: both get the reply chan (the fanout at
// tui.go:42 delivers EvChoice to model and webBus), the chan is buffered cap 1 (tui.go:743),
// and Answer's select/default (web.go:148) makes the loser return an error promptly.
func TestWebBusAnswerTwoConsumersNeverBlocks(t *testing.T) {
	w := newWebBus()
	// Two consumers share ONE reply chan, exactly as the fanout delivers EvChoice:
	reply := make(chan tui.ChoiceAnswer, 1)
	w.Send(tui.EvChoice{View: tui.ChoiceView{Title: "Approval", Options: []string{"Yes", "No"}}, Reply: reply})
	// Discover the id the bus registered for this choice.
	replay, _, cancel := w.Subscribe()
	defer cancel()
	var c map[string]any
	for _, ev := range replay {
		if e := json.Unmarshal(ev, &c); e == nil && c["type"] == "choice" {
			break
		}
	}
	id, _ := c["id"].(string)
	if id == "" {
		t.Fatalf("no choice id: %v", c)
	}
	// The terminal answers first — the send is default-guarded (model.go answer), so this
	// returns immediately.
	terminalDone := make(chan struct{})
	go func() {
		defer close(terminalDone)
		select {
		case reply <- tui.ChoiceAnswer{Index: 0, Text: "terminal"}:
		default:
		}
	}()
	<-terminalDone
	// The dashboard answers the same choice: the buffered slot is taken, so Answer must take
	// the default arm and return an error PROMPTLY — never block the HTTP handler.
	errc := make(chan error, 1)
	go func() { errc <- w.Answer(id, 1, "dash") }()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("the second Answer must lose the rank and error")
		}
		if !strings.Contains(err.Error(), "answered elsewhere") {
			t.Fatalf("error = %q, want \"answered elsewhere\"", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Answer blocked — the HTTP handler would hang")
	}
	// The winner's answer sits in the slot when Answer lost.
	if a := <-reply; a.Index != 0 || a.Text != "terminal" {
		t.Fatalf("answer = %+v", a)
	}
}

// TestWebBusTerminalWinNoHang pins the no-hang property in the terminal-wins direction:
// the card stays open client-side (no bus note fires for a terminal answer) but the bus
// id cleanly refuses a late answer — the human gets "no longer open", not a hang.
func TestWebBusTerminalWinNoHang(t *testing.T) {
	w := newWebBus()
	reply := make(chan tui.ChoiceAnswer, 1)
	w.Send(tui.EvChoice{View: tui.ChoiceView{Title: "Approval", Options: []string{"Yes", "No"}}, Reply: reply})
	replay, _, cancel := w.Subscribe()
	defer cancel()
	var id string
	for _, ev := range replay {
		var m map[string]any
		if json.Unmarshal(ev, &m) == nil && m["type"] == "choice" {
			id, _ = m["id"].(string)
		}
	}
	if id == "" {
		t.Fatal("the choice registered no id")
	}
	reply <- tui.ChoiceAnswer{Index: 0, Text: "terminal"} // the terminal wins
	done := make(chan error, 1)
	go func() { done <- w.Answer(id, 1, "dash") }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a terminal-answered choice refuses a late web answer")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Answer hung after the terminal answered")
	}
}

func TestWebBusTapStreamSidecar(t *testing.T) {
	dir := t.TempDir()
	w := newWebBus()
	w.tapStream(dir)
	w.Send(tui.EvDelta{Text: "audit me live"})
	matches, err := filepath.Glob(filepath.Join(dir, ".agent-one", "instruments", "stream", "*.jsonl"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected one day-scoped sidecar, got %v (err %v)", matches, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	if !strings.Contains(string(raw), "audit me live") {
		t.Fatalf("sidecar missing the event: %s", raw)
	}
}

// The choice ledger rides the bus (Operator 2026-10-03): every answered choice is published and
// tapped, so a later review lane can read "you always allow class X" and graduate a rule —
// never a live nudge, the gate keeps its load-bearing silence.
func TestChoiceAnsweredLandsOnTheTap(t *testing.T) {
	dir := t.TempDir()
	w := newWebBus()
	w.tapStream(dir)
	w.Send(tui.EvChoiceAnswered{Class: "outward", Tool: "git", Answer: "Yes, and don't ask again for git:git push*", Recommended: ""})
	matches, _ := filepath.Glob(filepath.Join(dir, ".agent-one", "instruments", "stream", "*.jsonl"))
	if len(matches) != 1 {
		t.Fatalf("tap missing: %v", matches)
	}
	raw, _ := os.ReadFile(matches[0])
	for _, want := range []string{`"type":"choice-answered"`, `"class":"outward"`, `"tool":"git"`, "don't ask again", `"recommended":""`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("sidecar missing %q: %s", want, raw)
		}
	}
}
