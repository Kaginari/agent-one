package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRailCollapseToggleRender(t *testing.T) {
	m := railModel(t)
	// plan card is rendered expanded when not collapsed
	if len(m.plans) == 0 {
		t.Skip("no plan in fixture; nothing to collapse")
	}
	out := ansi.Strip(m.Render())
	if !strings.Contains(out, "▾ plan ▸") {
		t.Fatalf("expanded rail shows the plan header: %s", out)
	}

	// collapse it via alt+p
	if mm, _ := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt}); mm != nil {
		if got, ok := mm.(*Model); ok {
			m = got
		}
	}
	out = ansi.Strip(m.Render())
	if strings.Contains(out, "▾ plan ▸") || strings.Contains(out, "one") {
		t.Fatalf("collapsed rail drops the expanded plan header and its items")
	}
	if !strings.Contains(out, "▸ plan ▸ plan · 2") {
		t.Fatalf("collapsed rail shows the +stub: %s", out)
	}

	// expand it again
	if mm, _ := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModAlt}); mm != nil {
		if got, ok := mm.(*Model); ok {
			m = got
		}
	}
	out = ansi.Strip(m.Render())
	if !strings.Contains(out, "▾ plan ▸") {
		t.Fatalf("re-expanded rail shows the plan header again")
	}
}

func TestRailCollapseClickToggle(t *testing.T) {
	m := railModel(t)
	if len(m.plans) == 0 {
		t.Skip("no plan in fixture; nothing to collapse")
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40}) // re-render to populate railCardRows
	y, ok := m.railCardRows["plan"]
	if !ok {
		t.Fatalf("railCardRows must name the plan card's border row: %v", m.railCardRows)
	}
	railX := m.lay.rail.X + 2
	m.Update(tea.MouseClickMsg{X: railX, Y: y, Button: tea.MouseLeft}) // railCardRows holds window rows
	if !m.railCollapsed["plan"] {
		t.Fatalf("click on the plan card's border collapses it, got %v", m.railCollapsed)
	}
	m.Update(tea.MouseClickMsg{X: railX, Y: y, Button: tea.MouseLeft})
	if m.railCollapsed["plan"] {
		t.Fatalf("click again re-expands it, got %v", m.railCollapsed)
	}
}
