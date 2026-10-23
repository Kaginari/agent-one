package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/provider"
)

// The token diet. A subagent's fan-out calls are priced by replaying the conversation as input
// every step; a parent asking a coordinator → domain owner → zone worker multiplies that. To the wire the
// conversation looks slimmer than in-memory truth: old tool outputs fold to one line, old
// thinking blocks drop (the model doesn't need to re-think its own past), while the session
// keeps every byte parked.
//
// compactReplay is a copy — the loop's session Messages stay whole.
func compactReplay(msgs []provider.Message, opts ...CompactOpt) []provider.Message {
	cfg := compactConfig{keepTools: 4, keepThinkLast: 1, foldLen: 320}
	for _, o := range opts {
		o(&cfg)
	}
	out := make([]provider.Message, len(msgs))
	calls := indexCalls(msgs)

	// which are the newest `keepTools` tool-result groups (walk from the tail)
	keepTool := map[int]bool{}
	cnt := 0
	for i := len(msgs) - 1; i >= 0 && cnt < cfg.keepTools; i-- {
		if len(msgs[i].ToolResults) > 0 {
			keepTool[i] = true
			cnt++
		}
	}
	// last assistant message index: thinking stays only there
	lastAssistant := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.Assistant {
			lastAssistant = i
			break
		}
	}

	for i, m := range msgs {
		m2 := m
		if len(m2.ToolResults) > 0 && !keepTool[i] {
			folded := make([]provider.ToolResult, len(m2.ToolResults))
			for j, tr := range m2.ToolResults {
				tr2 := tr
				if len(tr2.Content) > cfg.foldLen || strings.Count(tr2.Content, "\n") > 4 {
					tr2.Content = foldCaption(calls[tr2.ID], tr2.Content)
				}
				folded[j] = tr2
			}
			m2.ToolResults = folded
		}
		if m2.Role == provider.Assistant && i != lastAssistant && len(m2.ToolCalls) == 0 {
			// replayed thinking: the model already note it once — the past's opaque
			// thinking blocks stay out of the wire (they'd just cost tokens; they can't
			// change a settled turn). Messages that also carry tool_use keep theirs:
			// Anthropic's contract binds the thinking to its tool_use block in the wire.
			m2.Opaque = nil
		}
		out[i] = m2
	}
	return out
}

// indexCalls maps every tool_use id to its call, once per replay: a caption reads what ran and
// on what without a backwards walk per folded result.
func indexCalls(msgs []provider.Message) map[string]provider.ToolCall {
	idx := map[string]provider.ToolCall{}
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			idx[tc.ID] = tc
		}
	}
	return idx
}

// foldCaption is the one line a folded result leaves on the wire: the tool, what it ran on
// (the call's own gist), and how much was there. The same call and payload always caption the
// same, so a folded message stays byte-stable turn after turn.
func foldCaption(tc provider.ToolCall, content string) string {
	name := tc.Name
	if name == "" {
		name = "tool"
	}
	if g := callGist(tc.Input); g != "" {
		name += " " + g
	}
	return fmt.Sprintf("[replay-folded %s: %d lines, %s were here — ask again if you need the raw text]",
		name, strings.Count(content, "\n")+1, byteSize(len(content)))
}

// callGist is the argument a caption names: the first of the inputs that say what a call ran
// on, on one line, clamped.
func callGist(in json.RawMessage) string {
	var a map[string]json.RawMessage
	if json.Unmarshal(in, &a) != nil {
		return ""
	}
	for _, k := range []string{"path", "paths", "pattern", "command", "query", "question", "url", "name"} {
		raw, ok := a[k]
		if !ok {
			continue
		}
		var s string
		var v []string
		switch {
		case json.Unmarshal(raw, &s) == nil:
		case json.Unmarshal(raw, &v) == nil && len(v) > 0:
			s = strings.Join(v[:min(len(v), 3)], " ")
			if len(v) > 3 {
				s += fmt.Sprintf(" +%d", len(v)-3)
			}
		}
		if s = strings.Join(strings.Fields(s), " "); s != "" {
			if r := []rune(s); len(r) > 80 {
				s = string(r[:80]) + "…"
			}
			return s
		}
	}
	return ""
}

func byteSize(n int) string {
	if n < 1<<10 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%.1fkB", float64(n)/(1<<10))
}

type compactConfig struct {
	keepTools     int // the newest tool-result groups left raw
	keepThinkLast int // assistant messages (from the tail) whose Opaque blocks ride the wire
	foldLen       int // bytes of content beyond which a tool result folds
}

type CompactOpt func(*compactConfig)

// CompactKeepTools overrides how many of the newest tool-result groups survive unfolded.
func CompactKeepTools(n int) CompactOpt { return func(c *compactConfig) { c.keepTools = n } }

// CompactFold overrides the byte length at which an old tool result folds.
func CompactFold(n int) CompactOpt { return func(c *compactConfig) { c.foldLen = n } }
