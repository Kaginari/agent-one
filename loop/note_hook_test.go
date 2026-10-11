package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider/mock"
)

// The Note seam rides the tool result as advisory text — never blocking, never a separate wire.
func TestNoteHookRidesResult(t *testing.T) {
	m := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "true"}),
		mock.Text("end with @S and @E"),
	)
	e := engine(t, m)
	e.Hooks.Note = func(ctx context.Context, s *Session, st *StepRecord) string {
		if st.Tool == "bash" {
			return "\n[advisory: " + st.ID + " carried a note]"
		}
		return ""
	}
	r, err := e.Run(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Steps) == 0 {
		t.Fatalf("the run must carry the act's step, got %+v", r)
	}
	// the advisory is message-side: it rides the tool result the provider receives, never the
	// raw act output stored on the step (the step keeps ground truth; the wire learns the note)
	if len(m.Requests) < 2 {
		t.Fatalf("two provider calls recorded: %d", len(m.Requests))
	}
	last := m.Requests[len(m.Requests)-1]
	var joined string
	for _, msg := range last.Messages {
		for _, tr := range msg.ToolResults {
			joined += tr.Content
		}
		joined += msg.Text
	}
	if !strings.Contains(joined, "[advisory:") || !strings.Contains(joined, "carried a note]") {
		t.Fatalf("the advisory must ride the provider-bound tool result:\n%.300s", joined)
	}
	// nil Note must never rustle the result
	m2 := mock.New(
		mock.Call("1", "bash", map[string]string{"command": "true"}),
		mock.Text("end"),
	)
	e2 := engine(t, m2)
	if _, err := e2.Run(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	var joined2 string
	for _, msg := range m2.Requests[len(m2.Requests)-1].Messages {
		for _, tr := range msg.ToolResults {
			joined2 += tr.Content
		}
		joined2 += msg.Text
	}
	if strings.Contains(joined2, "advisory") {
		t.Error("no Note hook → no advisory")
	}
}
