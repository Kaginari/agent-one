package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPipeDbg2(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking"}})
	m.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	t.Logf("bview sel=%d", m.bview.sel)
	m.Update(tea.KeyPressMsg{Code: -1, Text: "i"})
	t.Logf("pipe=%v", m.bview.pipe)
	m.Update(tea.PasteMsg{Content: "hold the ttl"})
	t.Logf("input after paste: %q", m.input.Value())
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	h.mu.Lock()
	t.Logf("piped=%v", h.piped)
	h.mu.Unlock()
}
