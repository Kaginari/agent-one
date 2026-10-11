package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// benchPane is a pane-mode session holding n finished blocks: answers through Glamour, tool
// cards, notices.
func benchPane(b *testing.B, n int) *Model {
	m := New(&fakeHost{}, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	for i := 0; i < n; i++ {
		switch i % 3 {
		case 0:
			m.printAnswer(fmt.Sprintf("## Step %d\n\nThe gate has **four** checks; `go test ./...` runs them.\n\n- one\n- two\n\n```go\nfunc f() int { return %d }\n```", i, i))
		case 1:
			m.Update(EvToolEnd{Tool: ToolView{ID: fmt.Sprint(i), Name: "bash", Summary: "go test ./...", Class: "read", Status: "done", Ms: 40, Output: "ok\nok\nok\n"}})
		default:
			m.Update(EvNotice{Text: fmt.Sprintf("notice %d", i)})
		}
	}
	m.Render()
	return m
}

// BenchmarkPaneIdleFrame is a frame with nothing new: the spinner-less idle redraw.
func BenchmarkPaneIdleFrame(b *testing.B) {
	m := benchPane(b, 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Render()
	}
}

// BenchmarkPaneNewBlock is a block landing on a 200-block transcript, and the frame after it.
func BenchmarkPaneNewBlock(b *testing.B) {
	m := benchPane(b, 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.printAnswer(fmt.Sprintf("a new paragraph %d with `code`", i))
		m.Render()
	}
}

// The rail's decks load in a tea.Cmd: the tick never calls the host itself, and one read is in
// flight at a time.
func TestPlansLoadOffTheUIGoroutine(t *testing.T) {
	h := &countingHost{fakeHost: fakeHost{plans: []RailPlan{{Agent: "orchestrator", Rank: "orchestrator"}}}}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	_, cmd := m.Update(evTick{})
	if h.calls != 0 {
		t.Fatal("the tick read the plans on the UI goroutine")
	}
	_, again := m.Update(evTick{})
	for _, msg := range run(again) {
		if _, ok := msg.(evPlans); ok {
			t.Fatal("a second read while one is in flight")
		}
	}
	var got []evPlans
	for _, msg := range run(cmd) {
		if p, ok := msg.(evPlans); ok {
			got = append(got, p)
		}
	}
	if len(got) != 1 || h.calls != 1 {
		t.Fatalf("one read in flight: %d messages, %d calls", len(got), h.calls)
	}
	m.Update(got[0])
	if len(m.plans) != 1 || m.plansBusy {
		t.Fatalf("the read landed: %d plans, busy %v", len(m.plans), m.plansBusy)
	}
}

type countingHost struct {
	fakeHost
	calls int
}

func (h *countingHost) Plans() []RailPlan { h.calls++; return h.fakeHost.plans }

// A landed block renders once per width and theme: frames, new blocks and reprints reuse it.
func TestBlocksRenderOncePerWidth(t *testing.T) {
	m := New(&fakeHost{}, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	n := 0
	m.print(func(w int) string { n++; return fmt.Sprintf("block at %d", w) })
	for i := 0; i < 5; i++ {
		m.Update(EvNotice{Text: fmt.Sprint("more ", i)})
		m.Render()
	}
	if n != 1 {
		t.Fatalf("rendered %d times across frames and new blocks", n)
	}
	m.setTheme(NewTheme(true))
	m.Render()
	m.Render()
	if n != 2 {
		t.Fatalf("a theme swap renders it again, once: %d", n)
	}
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 40})
	m.reflow()
	m.Render()
	if n != 3 {
		t.Fatalf("a new width renders it again, once: %d", n)
	}
}

// An idle frame feeds the viewport nothing: the content is set only when it changed.
func TestIdleFrameLeavesTheViewport(t *testing.T) {
	m := paneModel(t)
	m.Update(EvNotice{Text: "a block"})
	m.Render()
	ver := m.vpVer
	lines := m.vpLines
	m.Render()
	if m.vpVer != ver || &m.vpLines[0] != &lines[0] {
		t.Fatal("an idle frame fed the viewport again")
	}
	m.Update(EvNotice{Text: "another"})
	m.Render()
	if m.vpVer == ver {
		t.Fatal("a new block never reached the viewport")
	}
}
