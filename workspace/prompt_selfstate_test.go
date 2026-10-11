package workspace

import (
	"strings"
	"testing"
)

func TestPromptSelfStateBlock(t *testing.T) {
	w := open(t, fixture(t))
	// the block names the pulse tool and the check-loop policy — the member enters the
	// run knowing its own instruments (the 2026-10-09 human order)
	on := w.Prompt("zone-auth", PromptOptions{Identity: true, SelfState: true}, nil)
	if !strings.Contains(on, "`pulse`") || !strings.Contains(on, "check-loop") || !strings.Contains(on, "exit status") {
		t.Fatalf("the self-state policy must name the pulse tool, the check-loop policy and the pipe trap:\n%s", on)
	}
	off := w.Prompt("zone-auth", PromptOptions{Identity: true}, nil)
	if strings.Contains(off, "check-loop") {
		t.Fatal("SelfState off must not carry the block")
	}
	// the default prompt carries it (DefaultPrompt is everything on)
	if !DefaultPrompt().SelfState {
		t.Fatal("DefaultPrompt must carry the self-state policy")
	}
}
