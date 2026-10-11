package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Operator 2026-10-03: the room's report markers ("**Maintenance:** ⚙️" / "**Evolution:** 🚀")
// reached in-workspace speech and warped panes — U+FE0F is counted 2 cells by x/ansi but drawn
// 1 cell by terminals that fall back to the text-presentation glyph, so every padded line on
// a marker row lies. textSanitize must pin all rendered text to unpadded, measurable cells:
// no VS16 survives Markdown or wrap, and every emitted line stays inside its width.
func TestMarkdownStripsVS16(t *testing.T) {
	th := NewTheme(true)
	in := "**Maintenance:** ⚙️ relayed the commission down and the report up; the pairing landed — **Evolution:** 🚀 new capability."
	out := th.Markdown(in, 40)
	if strings.ContainsRune(out, '️') {
		t.Fatalf("VS16 survived markdown: %q", out)
	}
	if !strings.Contains(out, "⚙") || !strings.Contains(out, "🚀") {
		t.Fatalf("base glyphs must survive: %q", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > 40 {
			t.Fatalf("line over width %d: %q", w, l)
		}
	}
}

func TestWrapStripsVS16(t *testing.T) {
	out := wrap("line with ⚙️ emoji and ordinary words wrapped tight", 20)
	if strings.ContainsRune(out, '️') {
		t.Fatalf("VS16 survived wrap: %q", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > 20 {
			t.Fatalf("line over width %d: %q", w, l)
		}
	}
}
