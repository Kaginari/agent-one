package loop

import (
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
					name := toolName(msgs, tr2.ID)
					lines := strings.Count(tr2.Content, "\n") + 1
					tr2.Content = fmt.Sprintf("[replay-folded %s: %d lines were here — ask again if you need the raw text]", name, lines)
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

// toolName resolves a tool_use id from the preceding assistant message's call list (the
// "what ran" the fold caption reads).
func toolName(msgs []provider.Message, id string) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		for _, tc := range msgs[i].ToolCalls {
			if tc.ID == id {
				return tc.Name
			}
		}
	}
	return "tool"
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
