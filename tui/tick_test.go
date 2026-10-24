package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// An idle session's tick sleeps: it wakes for the zone worker's blink and nothing else, and the blink
// keeps the exact rhythm the frame-rate tick drew (closed one frame in twelve).
func TestIdleTickSleepsUntilTheBlink(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(false), DefaultWords())
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 36})
	frames, wakes := 0, 0
	var closed []int
	steps := 1
	for frames < 48 {
		m.Update(evTick{gen: m.tickGen, steps: steps})
		frames += steps
		wakes++
		if m.blinkOn {
			closed = append(closed, frames)
		}
		if !m.tickIdle {
			t.Fatalf("an idle session kept the frame rate at frame %d", frames)
		}
		steps = int(m.nextTick / tickStep)
	}
	if len(closed) != 4 || closed[0] != 11 || closed[1] != 23 || closed[2] != 35 || closed[3] != 47 {
		t.Fatalf("the blink lost its rhythm: closed at frames %v", closed)
	}
	if wakes > 9 {
		t.Fatalf("an idle session woke %d times in 48 frames, want at most two a blink", wakes)
	}
}

// Something starting to move wakes the sleeping chain at once, at the frame rate; the sleeping
// tick, when it lands, is an orphan and moves nothing.
func TestAMovingSessionWakesTheTick(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(false), DefaultWords())
	m.Init()
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 36})
	m.Update(evTick{gen: m.tickGen, steps: 1})
	if !m.tickIdle {
		t.Fatal("the idle session's tick did not sleep")
	}
	old := m.tickGen
	_, cmd := m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "x"}})
	if cmd == nil || m.tickIdle || m.tickGen == old || m.nextTick != tickStep {
		t.Fatalf("a Subagent's row did not wake the tick: idle %v gen %d→%d next %v", m.tickIdle, old, m.tickGen, m.nextTick)
	}
	before := m.railTick
	if _, cmd := m.Update(evTick{gen: old, steps: 10}); cmd != nil || m.railTick != before {
		t.Fatal("the orphaned sleeping tick still moved the session")
	}
	m.Update(evTick{gen: m.tickGen, steps: 1})
	if m.tickIdle || m.nextTick != tickStep {
		t.Fatal("a live Subagent's row must keep the frame rate")
	}
}

// The frame's rows are painted by content: a redraw with nothing changed paints no row again,
// and a theme swap repaints.
func TestIdleFramePaintsNoRowTwice(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 36})
	m.Update(EvNotice{Text: "a block in the pane"})
	first := m.Render()
	m.rowPaint.hits, m.rowPaint.miss = 0, 0
	if again := m.Render(); again != first {
		t.Fatal("the memo changed the frame")
	}
	if m.rowPaint.miss != 0 || m.rowPaint.hits == 0 {
		t.Fatalf("an idle redraw painted %d rows again (%d from the memo)", m.rowPaint.miss, m.rowPaint.hits)
	}
	m.setTheme(NewTheme(false))
	m.rowPaint.miss = 0
	m.Render()
	if m.rowPaint.miss == 0 {
		t.Fatal("a theme swap kept the old theme's rows")
	}
}
