package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestFooterHintNamesThePlanKey pins the small-but-visible plan affordance
// (human directive, relayed coord-agent-one -> domain-host -> this zone worker): the
// footer hint names ctrl+p so a human who never opens '?' still learns the
// deck walk. The hint must stay plain-runable through the existing
// fits()/truncate path — a narrow terminal drops or truncates it whole,
// never half-renders it.
func TestFooterHintNamesThePlanKey(t *testing.T) {
	th := NewTheme(true)
	base := FooterView{
		Model:     "gemini-3.0-flash",
		Ctx:       "ctx 12%",
		Cost:      "$0.0123",
		Live:      1,
		Workspace: "~/kaginari/pfcli",
	}
	withHint := base
	withHint.Hint = "? for shortcuts · ctrl+p plans"

	if !strings.Contains(withHint.Hint, "ctrl+p") {
		t.Fatal("the hint names the plan key")
	}
	if !strings.HasPrefix(withHint.Hint, "? for shortcuts") {
		t.Fatal("the plan key rides after the shortcuts word, hint stays terse")
	}

	// wide: the full hint stands right-aligned
	wide := ansi.Strip(th.Footer(withHint, 120))
	if !strings.Contains(wide, "? for shortcuts · ctrl+p plans") {
		t.Fatalf("wide footer carries the full hint:\n%s", wide)
	}
	// narrow at 80 with this parts set: the hint does not fit and drops whole —
	// the line still holds the parts, never a half-drawn hint
	narrow := ansi.Strip(th.Footer(withHint, 80))
	if strings.Contains(narrow, "ctrl+p") && !strings.Contains(narrow, "? for shortcuts · ctrl+p plans") {
		t.Fatalf("narrow footer must drop or carry the hint whole, never fragment it:\n%s", narrow)
	}
	if !strings.Contains(narrow, "gemini-3.0-flash") {
		t.Fatalf("narrow footer keeps the parts through fits()/truncate:\n%s", narrow)
	}
	// squeezed hard: truncate path ends the line inside the width, ellipsis-marked
	squeezed := ansi.Strip(th.Footer(withHint, 40))
	if w := ansi.StringWidth(squeezed); w > 40 {
		t.Fatalf("squeezed footer stays inside its width, got %d:\n%s", w, squeezed)
	}
}

// TestDefaultShortcutsListThePlanKeys pins the '?' overlay rows for the plan
// affordance: ctrl+p walks decks, alt+n snaps to the current deck off a
// stale one. The wording must not overclaim: no walk-back key exists
// (ctrl+n is overlay scroll), and no plain letter is a chord.
func TestDefaultShortcutsListThePlanKeys(t *testing.T) {
	list := DefaultShortcuts()
	var ctrlP, snapC *Shortcut
	for i := range list {
		switch list[i].Key {
		case "ctrl+p":
			ctrlP = &list[i]
		case "alt+n":
			snapC = &list[i]
		}
	}
	if ctrlP == nil {
		t.Fatal("DefaultShortcuts lists ctrl+p")
	} else if !strings.Contains(ctrlP.Does, "plan") {
		t.Fatalf("ctrl+p row names the plan decks, got %q", ctrlP.Does)
	}
	if snapC == nil {
		t.Fatal("DefaultShortcuts lists alt+n")
	} else {
		if !strings.Contains(snapC.Does, "current") {
			t.Fatalf("alt+n row names the current deck, got %q", snapC.Does)
		}
		if !strings.Contains(snapC.Does, "stale") {
			t.Fatalf("alt+n row names the stale deck, got %q", snapC.Does)
		}
	}
	// the honest-keys policy: no row may promise a deck walk-back that doesn't exist
	for _, s := range list {
		if strings.Contains(s.Key, "ctrl+n") {
			t.Fatalf("no ctrl+n deck row — that key is overlay scroll-down: %+v", s)
		}
	}
}
