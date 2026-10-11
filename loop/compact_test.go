package loop

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider"
)

func TestCompactReplayFoldsOldToolOutputs(t *testing.T) {
	big := strings.Repeat("x", 500) + "\n" + strings.Repeat("x", 500)
	msgs := []provider.Message{
		{Role: provider.User, Text: "q"},
		// five tool groups; the default keepTools=4 means the first must fold
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "t1", Name: "read"}}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "t1", Content: big}}},
		{Role: provider.Assistant, Text: "ok", Opaque: []json.RawMessage{json.RawMessage(`{"type":"thinking","thinking":"hmm"}`)}},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "t2", Name: "read"}}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "t2", Content: big}}},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "t3", Name: "read"}}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "t3", Content: big}}},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "t4", Name: "read"}}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "t4", Content: big}}},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "t5", Name: "read"}}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "t5", Content: big}}},
		{Role: provider.Assistant, Text: "final", Opaque: []json.RawMessage{json.RawMessage(`{"type":"thinking","thinking":"latest"}`)}},
		{Role: provider.User, Text: "follow-up"},
	}
	out := compactReplay(msgs)
	// the old long result folded (t1 was folded, newest 4 stay raw)
	if !strings.Contains(out[2].ToolResults[0].Content, "replay-folded") {
		t.Fatalf("fold caption missing on the oldest tool group: %q", out[2].ToolResults[0].Content[:120])
	}
	if strings.Contains(out[11].ToolResults[0].Content, "replay-folded") {
		t.Fatalf("newest tool output must survive unfolded, got fold caption: %q", out[11].ToolResults[0].Content[:120])
	}
	if out[3].Opaque != nil {
		t.Fatal("past assistant message carries thinking onto the wire")
	}
	if len(out[12].Opaque) == 0 {
		t.Fatal("last assistant keeps its thinking")
	}
}

func TestCompactReplaySmallConversationIsExact(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.User, Text: "hi"},
		{Role: provider.Assistant, Text: "hello"},
	}
	out := compactReplay(msgs)
	if len(out) != 2 || out[0].Text != "hi" || out[1].Text != "hello" {
		t.Fatalf("small conversations pass through exact: %+v", out)
	}
}

// The A/B the user asked for: same ask, same 12 tool runs, same follow-up — wire bytes with
// and without the diet. This stands in for "opencode vs agent-one" (opencode isn't a Go
// module we can embed); it pins the shape the user saw in the audit: a subagent's later calls
// stop paying full replay price.
func TestCompactReplayBeforeAfterReport(t *testing.T) {
	toolFat := strings.Repeat("read the file\n", 400)
	var msgs []provider.Message
	msgs = append(msgs, provider.Message{Role: provider.User, Text: "fix the thing"})
	for i := 0; i < 12; i++ {
		id := "t" + string(rune('0'+i))
		if i >= 10 {
			id = "t1" + string(rune('0'+i-10))
		}
		msgs = append(msgs, provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: id, Name: "read"}}})
		msgs = append(msgs, provider.Message{Role: provider.User, ToolResults: []provider.ToolResult{{ID: id, Content: toolFat}}})
	}
	msgs = append(msgs, provider.Message{Role: provider.User, Text: "now what says it"})

	wire := compactReplay(msgs)
	raw, folded := 0, 0
	for i := range msgs {
		if len(msgs[i].ToolResults) == 0 {
			continue
		}
		raw += len(msgs[i].ToolResults[0].Content)
		folded += len(wire[i].ToolResults[0].Content)
	}
	cut := 100 - folded*100/max(raw, 1)
	t.Logf("wire diet: raw=%d compacted=%d cut=%d%%", raw, folded, cut)
	if cut < 60 {
		t.Fatalf("12-tool run should cut ≥60%% of replayed bytes (4 kept raw, 8 folded): cut=%d%%", cut)
	}
}

// The wire-accounting shape the audit asked for: the compactor's floor on short runs — the
// four newest groups stay raw, exactly the older ones fold, and the wire shrinks the whole
// way a little more per added round-trip.
func TestCompactReplayBudgetCut(t *testing.T) {
	toolFat := strings.Repeat("read the file\n", 400) // ~5.6kB
	var msgs []provider.Message
	msgs = append(msgs, provider.Message{Role: provider.User, Text: "read the thing"})
	for i := 0; i < 6; i++ {
		id := "t" + string(rune('1'+i))
		msgs = append(msgs, provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: id, Name: "read"}}})
		msgs = append(msgs, provider.Message{Role: provider.User, ToolResults: []provider.ToolResult{{ID: id, Content: toolFat}}})
	}
	msgs = append(msgs, provider.Message{Role: provider.User, Text: "what does it say?"})

	wire := compactReplay(msgs)
	raw, folded := 0, 0
	for i, m := range msgs {
		if len(m.ToolResults) == 0 {
			continue
		}
		raw += len(m.ToolResults[0].Content)
		folded += len(wire[i].ToolResults[0].Content)
	}
	if folded >= raw*4/5 {
		t.Fatalf("the diet must cut >20%% of replayed tool bytes on a short run (grows with length): raw=%d folded=%d", raw, folded)
	}
	// and specifically: the four newest groups stay raw, exactly two folded
	folds := 0
	for i := range msgs {
		if len(msgs[i].ToolResults) > 0 && strings.HasPrefix(wire[i].ToolResults[0].Content, "[replay-folded") {
			folds++
		}
	}
	if folds != 2 {
		t.Fatalf("exactly the two oldest groups fold under keepTools=4: fold hits=%d", folds)
	}
}
