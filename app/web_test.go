package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/tui"
)

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
	// the terminal answers when it is open
	reply := make(chan tui.ChoiceAnswer, 1)
	w.Send(tui.EvChoice{View: tui.ChoiceView{Title: "Approval", Options: []string{"Yes", "No"}}, Reply: reply})
	if e := <-ch; !strings.Contains(string(e), `"elsewhere":true`) {
		t.Fatalf("with a terminal the choice is answered there: %s", e)
	}
	// the dashboard answers when it is the only ui
	w.answerable = true
	w.Send(tui.EvChoice{View: tui.ChoiceView{Title: "Approval", Options: []string{"Yes", "No"}}, Reply: reply})
	var c map[string]any
	_ = json.Unmarshal(<-ch, &c)
	id, _ := c["id"].(string)
	if id == "" {
		t.Fatalf("no choice id: %v", c)
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
