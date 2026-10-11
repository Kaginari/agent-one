package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Recommendations: the asked advice tags its option, the tag rides the picked pill too, and
// nothing renders without one (silence is load-bearing — an approval asks nothing).
func TestChoiceRecommendedTag(t *testing.T) {
	th := NewTheme(true)
	v := ChoiceView{Title: "Question", Why: "Which tier should the index live on?", Options: []string{"jsonl files, rebuildable at will", "sqlite, one binary store", "postgresql, shared across workspaces"}, Recommended: "jsonl files, rebuildable at will"}
	out := ansi.Strip(th.Choice(v, 90))
	if !strings.Contains(out, "◆ recommended") {
		t.Fatalf("the advised option holds the tag:\n%s", out)
	}
	if n := strings.Count(out, "recommended"); n != 1 {
		t.Fatalf("exactly the advised option tags (got %d):\n%s", n, out)
	}
	// no recommendations asked (approvals stay neutral by policy): no tag rows at all
	out = ansi.Strip(th.Choice(ChoiceView{Title: "Approval", Class: "outward", Options: []string{"Yes", "No"}}, 80))
	if strings.Contains(out, "recommended") {
		t.Fatalf("an approval must not hold a recommendation the asker never gave:\n%s", out)
	}
}

// The float pin: once the card opens, cursor moves never change its rendered height — the
// frame under it does not reflow (the "text floats when moving from choice to choice" cure).
func TestChoicePinStabilisesCard(t *testing.T) {
	th := NewTheme(true)
	var m Model
	v := ChoiceView{Title: "Question", Why: "pick", Options: []string{"a", "b", "with a long trailing note to widen"}, Notes: []string{"", "", "x"}, Cursor: 0}
	first := m.pinChoice(th.Choice(v, 96))
	v.Cursor = 2
	second := m.pinChoice(th.Choice(v, 96))
	if h1, h2 := strings.Count(first, "\n"), strings.Count(second, "\n"); h1 != h2 {
		t.Fatalf("cursor moves reflow the card: %d → %d rows", h1+1, h2+1)
	}
	// growth-only: a bigger state grows the pin but never shrinks back a state returned
	v.Cursor = 0
	third := m.pinChoice(th.Choice(v, 96))
	if strings.Count(third, "\n") < strings.Count(first, "\n") {
		t.Fatal("the pin may only grow while the card is open")
	}
}
