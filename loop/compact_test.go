package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/provider/mock"
	"github.com/Kaginari/agent-one/tool"
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

// A folded result names what ran and on what — a caption that says only "read" leaves the
// agent guessing which of its forty reads left the wire.
func TestCompactFoldCaptionNamesTheCall(t *testing.T) {
	big := strings.Repeat("line\n", 200)
	msgs := []provider.Message{{Role: provider.User, Text: "q"}}
	calls := []provider.ToolCall{
		{ID: "a", Name: "read", Input: json.RawMessage(`{"path":"pkg/x.go","offset":40}`)},
		{ID: "b", Name: "read", Input: json.RawMessage(`{"paths":["a.go","b.go","c.go","d.go"]}`)},
		{ID: "c", Name: "bash", Input: json.RawMessage(`{"command":"go test\n  ./..."}`)},
		{ID: "d", Name: "glob", Input: json.RawMessage(`{"pattern":"**/*.go"}`)},
		{ID: "e", Name: "pulse", Input: json.RawMessage(`{}`)},
	}
	for _, c := range calls {
		msgs = append(msgs, provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{c}},
			provider.Message{Role: provider.User, ToolResults: []provider.ToolResult{{ID: c.ID, Content: big}}})
	}
	out := compactReplay(msgs, CompactKeepTools(0))
	want := []string{"read pkg/x.go: 201 lines, 1000B", "read a.go b.go c.go +1:", "bash go test ./...:", "glob **/*.go:", "[replay-folded pulse: 201 lines"}
	for i, w := range want {
		if got := out[2+2*i].ToolResults[0].Content; !strings.Contains(got, w) {
			t.Errorf("caption %d = %q, want it to name %q", i, got, w)
		}
	}
	again := compactReplay(msgs, CompactKeepTools(0))
	if again[2].ToolResults[0].Content != out[2].ToolResults[0].Content {
		t.Fatal("the same fold must caption byte-identically turn after turn (the wire's cache prefix)")
	}
}

// The long-session measure: forty read/bash round-trips of a few kB each, the wire's tool
// bytes raw against compacted.
func TestCompactReplayLongSessionBytes(t *testing.T) {
	var msgs []provider.Message
	msgs = append(msgs, provider.Message{Role: provider.User, Text: "tour the package"})
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("t%d", i)
		in := json.RawMessage(fmt.Sprintf(`{"path":"pkg/file%02d.go"}`, i))
		msgs = append(msgs, provider.Message{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: id, Name: "read", Input: in}}},
			provider.Message{Role: provider.User, ToolResults: []provider.ToolResult{{ID: id, Content: strings.Repeat("     1\tfunc f() { return }\n", 120+i)}}})
	}
	raw, wire := 0, 0
	for i, m := range compactReplay(msgs) {
		for j, tr := range m.ToolResults {
			raw += len(msgs[i].ToolResults[j].Content)
			wire += len(tr.Content)
		}
	}
	t.Logf("40-read session: raw=%d wire=%d cut=%d%%", raw, wire, 100-wire*100/raw)
	if wire*5 > raw {
		t.Fatalf("a 40-read session must replay under a fifth of its tool bytes: raw=%d wire=%d", raw, wire)
	}
}

// A folded payload keeps on the shelf under its digest, byte for byte, and the caption quotes
// where; a second session folding the same bytes keeps them once.
func TestCompactFoldKeepsThePayload(t *testing.T) {
	root := t.TempDir()
	payload := strings.Repeat("the exact bytes\n", 50) + "no trailing newline"
	msgs := []provider.Message{
		{Role: provider.User, Text: "q"},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "a", Name: "read", Input: json.RawMessage(`{"path":"x.go"}`)}}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "a", Content: payload}}},
	}
	kept := ".agent-one/tmp/fold/" + tool.FoldDigest(payload) + ".txt"
	for i := 0; i < 2; i++ {
		s := &Session{Engine: &Engine{Root: root}}
		out := compactReplay(msgs, CompactKeepTools(0), CompactShelf(s.keepFold))
		if got := out[2].ToolResults[0].Content; !strings.HasSuffix(got, "— kept at "+kept+"]") {
			t.Fatalf("caption %q does not quote the kept path %s", got, kept)
		}
		if again := compactReplay(msgs, CompactKeepTools(0), CompactShelf(s.keepFold)); again[2].ToolResults[0].Content != out[2].ToolResults[0].Content {
			t.Fatal("a kept fold must caption the same on every replay")
		}
	}
	got, err := os.ReadFile(filepath.Join(root, kept))
	if err != nil || string(got) != payload {
		t.Fatalf("the shelf must hold the payload byte for byte: %v", err)
	}
	if ents, _ := os.ReadDir(filepath.Dir(filepath.Join(root, kept))); len(ents) != 1 {
		t.Fatalf("one payload, one kept file: %d entries", len(ents))
	}
	// a rootless engine keeps nothing and asks for the re-run
	s := &Session{Engine: &Engine{}}
	if got := compactReplay(msgs, CompactKeepTools(0), CompactShelf(s.keepFold))[2].ToolResults[0].Content; !strings.Contains(got, "ask again") {
		t.Fatalf("no shelf, no pointer: %q", got)
	}
}

// The fold and the read cache agree: a window whose serve the replay folded off the wire is
// served again on a re-read, never withheld as "unchanged" — the agent no longer holds it.
func TestFoldedReadIsServedAgain(t *testing.T) {
	big := func(w string) string { return strings.Repeat(w+" line of the file\n", 40) }
	script := []provider.Response{mock.Call("1", "read", map[string]string{"path": "big.txt"})}
	for i := 1; i <= 4; i++ {
		script = append(script, mock.Call(fmt.Sprint(i+1), "read", map[string]string{"path": fmt.Sprintf("f%d.txt", i)}))
	}
	script = append(script, mock.Call("6", "read", map[string]string{"path": "big.txt"}), mock.Call("7", "read", map[string]string{"path": "big.txt"}),
		mock.Text("@S DONE\n@F big.txt:1 x\n@U team c\n@E 0"))
	m := mock.New(script...)
	e := engine(t, m)
	os.WriteFile(filepath.Join(e.Root, "big.txt"), []byte(big("big")), 0o644)
	for i := 1; i <= 4; i++ {
		os.WriteFile(filepath.Join(e.Root, fmt.Sprintf("f%d.txt", i)), []byte(big(fmt.Sprint("f", i))), 0o644)
	}
	if r, _ := e.Run(context.Background(), "tour"); r.Status != Done {
		t.Fatalf("%+v", r)
	}
	last := func(req int) string {
		msgs := m.Requests[req].Messages
		return msgs[len(msgs)-1].ToolResults[0].Content
	}
	if got := last(6); !strings.Contains(got, "big line of the file") {
		t.Fatalf("a re-read after the fold must ship the bytes again, got %q", got)
	}
	if got := last(7); !strings.Contains(got, "withheld") {
		t.Fatalf("a re-read while the serve is still on the wire stays withheld, got %q", got)
	}
}

// With the fold tool on the shelf, the caption says it restores the kept payload.
func TestFoldCaptionNamesTheFoldTool(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.User, Text: "q"},
		{Role: provider.Assistant, ToolCalls: []provider.ToolCall{{ID: "a", Name: "grep", Input: json.RawMessage(`{"pattern":"x"}`)}}},
		{Role: provider.User, ToolResults: []provider.ToolResult{{ID: "a", Content: strings.Repeat("hit\n", 20)}}},
	}
	s := &Session{Engine: &Engine{Root: t.TempDir(), Tools: tool.Builtins()}}
	got := compactReplay(msgs, CompactKeepTools(0), CompactShelf(s.keepFold))[2].ToolResults[0].Content
	if !strings.HasSuffix(got, ".txt (fold restores it)]") {
		t.Fatalf("caption %q does not name the fold tool", got)
	}
}
