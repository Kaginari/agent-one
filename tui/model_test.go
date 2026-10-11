package tui

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// fakeHost records what the program asks of the app.
type fakeHost struct {
	mu          sync.Mutex
	submitted   []string
	queued      []string
	slashed     []string
	piped       []string
	interrupted int
	refuse      string
	plans       []RailPlan
}

func (h *fakeHost) Welcome() Welcome {
	return Welcome{Dist: "agent-one", Version: "vtest", Workspace: "/w", Model: "mock/m", Session: "s1"}
}
func (h *fakeHost) Footer() FooterView {
	return FooterView{Model: "mock/m", Ctx: "ctx 3%", Cost: "no calls yet", Workspace: "/w", Tokens: 1500}
}
func (h *fakeHost) Submit(text string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.refuse != "" {
		return h.refuse
	}
	h.submitted = append(h.submitted, text)
	return ""
}
func (h *fakeHost) Queue(text string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.queued = append(h.queued, text)
	return "queued for the next tool step"
}
func (h *fakeHost) Interrupt() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.interrupted++
}
func (h *fakeHost) Pipe(agent, text string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.piped = append(h.piped, agent+": "+text)
	return "piped into " + agent + "'s inbox"
}
func (h *fakeHost) Slash(line string) ([]string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.slashed = append(h.slashed, line)
	if line == "/quit-now" {
		return nil, true
	}
	return []string{"answer to " + line}, false
}
func (h *fakeHost) Commands() []MenuItem {
	return []MenuItem{{"agents", "the live subagent"}, {"status", "the board"}, {"send", "a line for a agent"}, {"review", "Review $ARGUMENTS"}}
}
func (h *fakeHost) Complete(prefix string) []string {
	var out []string
	for _, p := range []string{"src/auth/login.go", "src/auth/token.go", "src/api/server.go"} {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}

// fakeHost.Plans is a fixture the tests set per-scenario.
func (h *fakeHost) Plans() []RailPlan { return h.plans }

// tail accumulates the program's output so a test can wait for text across events.
type tail struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (t *tail) follow(r io.Reader) {
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				t.mu.Lock()
				t.buf.Write(b[:n])
				t.mu.Unlock()
			}
			if err == io.EOF {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			if err != nil {
				return
			}
		}
	}()
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return ansi.Strip(t.buf.String())
}

func (t *tail) wait(tb testing.TB, want string) {
	tb.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(t.String(), want) {
		if time.Now().After(deadline) {
			tb.Fatalf("waited for %q, output:\n%s", want, t.String())
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func start(t *testing.T, h *fakeHost) (*teatest.TestModel, *Model, *tail) {
	t.Helper()
	m := New(h, NewTheme(true), DefaultWords())
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	m.Attach(tm.Send)
	out := &tail{}
	out.follow(tm.Output())
	out.wait(t, "✦ agent-one")
	return tm, m, out
}

func key(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

func TestTurnStreamsAndLands(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Type("explain the gate")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "> explain the gate")
	out.wait(t, "esc to interrupt")
	tm.Send(EvState{Agent: "orchestrator", State: "thinking"})
	tm.Send(EvDelta{Text: "The gate has **four** checks.\n\nSecond paragraph "})
	out.wait(t, "four")
	tm.Send(EvDelta{Text: "done.\n"})
	tm.Send(EvToolStart{Tool: ToolView{ID: "s1", Name: "bash", Summary: "go test ./...", Class: "read"}})
	out.wait(t, "Running bash…")
	tm.Send(EvToolEnd{Tool: ToolView{ID: "s1", Name: "bash", Summary: "go test ./...", Class: "read", Status: "done", Ms: 40, Output: "ok\n"}})
	out.wait(t, "⎿ ok · 40ms · 1 line")
	tm.Send(EvTurnDone{Status: "DONE", Streamed: true, Verdict: "n/a (no domain owners)", LogRel: ".agent-one/log.md"})
	out.wait(t, "✓ gate · n/a (no domain owners) · .agent-one/log.md")
	s := out.String()
	if strings.Index(s, "Second paragraph") > strings.Index(s, "● bash") {
		t.Fatalf("the streamed text must land above the tool block:\n%s", s)
	}
	h.mu.Lock()
	sub := h.submitted
	h.mu.Unlock()
	if len(sub) != 1 || sub[0] != "explain the gate" {
		t.Fatalf("submitted %v", sub)
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestQueuedInterruptAndCtrlC(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Type("first")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "> first")
	tm.Type("also this")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "⏎ queued: also this")
	tm.Send(key(tea.KeyEsc))
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		n := h.interrupted
		h.mu.Unlock()
		if n == 1 || time.Now().After(deadline) {
			if n != 1 {
				t.Fatalf("esc did not interrupt (%d)", n)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	tm.Send(EvTurnDone{Status: "FAIL", Interrupted: true})
	out.wait(t, "■ interrupted")
	h.mu.Lock()
	q := h.queued
	h.mu.Unlock()
	if len(q) != 1 || q[0] != "also this" {
		t.Fatalf("queued %v", q)
	}
	tm.Send(ctrl('c'))
	out.wait(t, "ctrl+c again to exit")
	tm.Send(ctrl('c'))
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestSlashMenuAndCompletion(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Type("/")
	out.wait(t, "/agents")
	tm.Type("st")
	out.wait(t, "❯ /status")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "answer to /status")
	h.mu.Lock()
	sl := h.slashed
	h.mu.Unlock()
	if len(sl) != 1 || sl[0] != "/status" {
		t.Fatalf("slashed %v", sl)
	}
	// /help is the UI's own
	tm.Type("/help")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "ctrl+o        expand the last collapsed block")
	// @path completion: tab opens, tab accepts the first candidate
	tm.Type("look at @src/auth/l")
	tm.Send(key(tea.KeyTab))
	out.wait(t, "src/auth/login.go")
	tm.Send(key(tea.KeyTab))
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "> look at @src/auth/login.go")
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestHistoryNewlineAndPaste(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Type("one")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "> one")
	tm.Send(EvTurnDone{Status: "DONE", Text: "fine"})
	out.wait(t, "fine")
	tm.Send(key(tea.KeyUp))
	tm.Type(" more\\")
	tm.Send(key(tea.KeyEnter)) // "\" + enter is a newline
	tm.Type("second line")
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "second line")
	h.mu.Lock()
	sub := h.submitted
	h.mu.Unlock()
	if len(sub) != 2 || sub[1] != "one more\nsecond line" {
		t.Fatalf("history + newline: %q", sub)
	}
	tm.Send(EvTurnDone{Status: "DONE", Text: "ok"})
	out.wait(t, "ok")
	tm.Send(tea.PasteMsg{Content: "a\nb\nc\nd"})
	out.wait(t, "[pasted 4 lines #1]")
	tm.Send(key(tea.KeyEnter))
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		n := len(h.submitted)
		last := ""
		if n > 0 {
			last = h.submitted[n-1]
		}
		h.mu.Unlock()
		if n == 3 {
			if last != "a\nb\nc\nd" {
				t.Fatalf("paste expanded to %q", last)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the paste was not submitted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestCollapsedExpandsOnCtrlO(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	var lines []string
	for i := 1; i <= 12; i++ {
		lines = append(lines, "row "+itoa(i))
	}
	tm.Send(EvToolStart{Tool: ToolView{ID: "s1", Name: "read", Summary: "x", Class: "read"}})
	tm.Send(EvToolEnd{Tool: ToolView{ID: "s1", Name: "read", Summary: "x", Class: "read", Status: "done", Ms: 1, Output: strings.Join(lines, "\n")}})
	// the policy: status + elapsed + line count only — no ambient log lines, expand on ctrl+o
	out.wait(t, "⎿ ok · 1ms · 12 lines")
	if strings.Contains(out.String(), "row 12") {
		t.Fatal("collapsed output shows every line")
	}
	if strings.Contains(out.String(), "row 1") {
		t.Fatal("collapsed output shows any line")
	}
	tm.Send(ctrl('o'))
	out.wait(t, "row 12")
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestSubagentBlocks(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Send(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "mock/c", State: "queued", Ask: "findings: look"}})
	out.wait(t, "→ subagent zone-auth started")
	tm.Send(EvState{Agent: "zone-auth", State: "tool"})
	out.wait(t, "◆ subagent zone-auth · zone · analyst · mock/c")
	tm.Send(EvSubagent{Subagent: SubagentView{Name: "zone-auth", State: "done", Elapsed: time.Second, Report: &ReportView{Status: "PASS", Findings: []string{"src/auth/login.go:1 fine"}, Unsaid: []string{"team: subagents can run behind"}}}})
	out.wait(t, "∴ team: subagents can run behind")
	if !strings.Contains(out.String(), "⎿ PASS") {
		t.Fatalf("report status missing:\n%s", out.String())
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// The zombie subagent card (Operator's chain run, 2026-09-28): a subagent dispatched by a subagent — or
// any subagent whose step-end report never parsed — got its loop's terminal "done" without a wire
// report ever reaching the model, and its card then sat in the live area forever, the elapsed
// ticking past the agent's death (a domain owner card read "DONE · 15m0s" — the clock's own axe). The
// policy: a done subagent's clock freezes, and the turn's end owes its card one tombstone — land
// exactly once, leave the live area, and the agent pane reads the close, not a stale "thinking".
// The agents pane pipe: ctrl+t opens the pane, tab to an agent, i types a line for its inbox,
// enter hands it to the host — never mid-turn, the FIFO boundary contracts it (Operator: "push
// a message in his pipe"). esc cancels without piping.
func TestAgentsPanePipesToTheSelectedAgent(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	tm.Send(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", Model: "mock/c", State: "thinking", Ask: "findings: token ttl"}})
	out.wait(t, "→ subagent zone-auth started")
	tm.Send(ctrl('t'))
	out.wait(t, "i pipe a line")
	// the pane opens on the live Subagent already — zone-auth is selected; tab would move away
	tm.Type("i")
	out.wait(t, "lands in zone-auth")
	tm.Type("hold the ttl")
	tm.Send(key(tea.KeyEnter))
	// the host sees the truth — keys may ride in one paste under load; poll the inbox side
	piped := waitPiped(h, 2*time.Second)
	if len(piped) != 1 || piped[0] != "zone-auth: hold the ttl" {
		t.Fatalf("piped %v", piped)
	}
	// esc cancels a pipe in the making: nothing more reaches the host
	tm.Type("i")
	out.wait(t, "lands in zone-auth")
	tm.Type("never sent")
	tm.Send(key(tea.KeyEsc))
	time.Sleep(100 * time.Millisecond)
	h.mu.Lock()
	piped = h.piped
	h.mu.Unlock()
	if len(piped) != 1 {
		t.Fatalf("a cancelled pipe must not reach the host: %v", piped)
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestSubagentZombieCardLandsFrozenAtTurnEnd(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m.Update(EvSubagent{Subagent: SubagentView{Name: "domain-experience", Rank: "domain", Role: "judge", Model: "mock/r", State: "thinking", Ask: "verdict on the trials"}})
	if m.subagents["domain-experience"] == nil {
		t.Fatal("the running subagent's card never opened")
	}
	// the card's tombstones live in the block store — the transcript whatever the flow (prints
	// inline, viewport panes; both sites append here exactly once when the card lands)
	cards := func() int {
		n := 0
		for _, r := range m.blocks {
			if strings.Contains(ansi.Strip(r(100)), "◆ subagent domain-experience") {
				n++
			}
		}
		return n
	}
	_ = cards()

	// the loop's terminal state, no wire report riding it (the dispatch step belongs to the
	// parent subagent): the card closes its clock and waits for its report
	m.Update(EvSubagent{Subagent: SubagentView{Name: "domain-experience", State: "done", Elapsed: 211 * time.Second}})
	c := m.subagents["domain-experience"]
	if c == nil || c.State != "done" {
		t.Fatalf("the done subagent vanished before its tombstone: %+v", m.subagents)
	}
	r1 := m.Render()
	frozen := c.Elapsed
	time.Sleep(1100 * time.Millisecond)
	r2 := m.Render()
	if c.Elapsed != frozen {
		t.Fatalf("a done subagent's clock must be frozen: %v, then %v across renders", frozen, c.Elapsed)
	}
	if !strings.Contains(r1, "DONE · 3m31s") || !strings.Contains(r2, "DONE · 3m31s") {
		t.Fatalf("the closed card must keep holding its terminal elapsed, got:\n%s\n---\n%s", r1, r2)
	}
	// the agent pane reads the close too — never the last mid-run state
	if l := m.logs["domain-experience"]; l == nil || l.state != "done" {
		t.Fatalf("the agent's pane state must close with its card: %+v", m.logs["domain-experience"])
	}

	// the turn ends: the sweep owes the tombstone, exactly once, then the live area is clean
	m.Update(EvTurnDone{Status: "DONE"})
	if _, ok := m.subagents["domain-experience"]; ok {
		t.Fatal("a done subagent still rides the live area after the turn ended")
	}
	if tombs := cards(); tombs != 1 {
		t.Fatalf("tombstones for domain-experience: %d, want exactly 1", tombs)
	}
	for _, r := range m.blocks {
		if s := ansi.Strip(r(100)); strings.Contains(s, "◆ subagent domain-experience") && !strings.Contains(s, "DONE · 3m31s") {
			t.Fatalf("the tombstone lost the frozen clock: %q", s)
		}
	}
	m.Update(EvTurnDone{Status: "DONE"})
	if tombs := cards(); tombs != 1 {
		t.Fatalf("a second tombstone landed for a landed subagent: %d", tombs)
	}
}

// The slice-1 landing flash (N15/N16): a done Subagent carrying its report is not tombstoned at
// once — its row holds in the live area for flashHold, then the tick sweep prints exactly one
// tombstone. A live tick must exist; the turn's end still lands the card in print order first.
func TestSubagentFlashHoldsThenTickLandsTheTombstone(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	// the transcript under either flow: block renders naming the subagent
	cards := func() int {
		n := 0
		for _, r := range m.blocks {
			if strings.Contains(ansi.Strip(r(100)), "◆ subagent zone-auth") {
				n++
			}
		}
		return n
	}
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "findings: token ttl"}})

	// the pulse clock follows the agent's states incl. the tool's name
	m.Update(EvState{Agent: "zone-auth", State: "tool", Tool: "edit"})
	p := m.pulses["zone-auth"]
	if p == nil || p.state != "tool" || p.tool != "edit" {
		t.Fatalf("the pulse never dressed for the tool step: %+v", m.pulses)
	}
	if !strings.Contains(ansi.Strip(m.Render()), "TOOL edit") {
		t.Fatalf("a live tool step must name its tool:\n%s", ansi.Strip(m.Render()))
	}

	// done with a report: held for the flash, nothing lands in the transcript yet
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", State: "done", Elapsed: 3 * time.Second, Report: &ReportView{Status: "PASS", Findings: []string{"src/auth/token.go:1 done"}}}})
	if m.subagents["zone-auth"] == nil {
		t.Fatal("the landed subagent was evicted before its flash")
	}
	if cards() != 0 {
		t.Fatal("a tombstone landed during the flash hold")
	}
	if !strings.Contains(ansi.Strip(m.Render()), "⎿ PASS") {
		t.Fatalf("the flash window must show the landing card:\n%s", ansi.Strip(m.Render()))
	}
	if f := m.pulseFor(m.subagents["zone-auth"]).Flash; f <= 0 {
		t.Fatalf("fresh landing carries a flash, got %v", f)
	}

	// the flash spent, a tick owes exactly one tombstone
	m.pulses["zone-auth"].doneAt = time.Now().Add(-flashHold)
	m.Update(evTick{at: time.Now()})
	if tombs := cards(); tombs != 1 {
		t.Fatalf("tombstones after the flash: %d, want exactly 1", tombs)
	}
	if m.subagents["zone-auth"] != nil || m.pulses["zone-auth"] != nil {
		t.Fatal("the landed subagent still rides the live area or the pulse store")
	}
	m.Update(evTick{at: time.Now()})
	if tombs := cards(); tombs != 1 {
		t.Fatalf("a second tombstone landed: %d", tombs)
	}
}

func TestSubagentPulseCoolsPastIdle(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "watch"}})
	p := m.pulses["zone-auth"]
	// a hot agent: the phase is the age's full tick count
	p.since, p.active = time.Now().Add(-2*time.Second), time.Now()
	hot := m.pulseFor(m.subagents["zone-auth"]).Phase
	// an agent silent past the cool: the phase rides a quarter rate from the cool point on
	p.since, p.active = time.Now().Add(-12*time.Second), time.Now().Add(-pulseIdleCool-time.Second)
	cool := m.pulseFor(m.subagents["zone-auth"]).Phase
	full := int((12 * time.Second) / (time.Second / pulseFPS))
	if hot != int((2*time.Second)/(time.Second/pulseFPS)) {
		t.Fatalf("hot phase %d, want the tick count of 2s", hot)
	}
	if cool >= full {
		t.Fatalf("a cooled agent's phase must ride the quarter rate: %d >= %d", cool, full)
	}
	// the gate-wait spring only fires while the agent is warm
	p.enter("waiting on gate", "", time.Now())
	p.active = time.Now().Add(-pulseIdleCool - time.Second)
	m.stepPulses(time.Now())
	if p.bobPos != 0 || p.bobVel != 0 {
		t.Fatalf("a cooled bob must settle: pos %v vel %v", p.bobPos, p.bobVel)
	}
	p.active = time.Now()
	m.stepPulses(time.Now())
	if p.bobPos == 0 && p.bobVel == 0 {
		t.Fatal("a warm gate-wait bob never moved on the tick")
	}
}

// The sidebar claims a third of the window (floor 28, gone under 68×26): chat │ sidebar as two
// painted regions. The ledger dedupes per path and the roster follows the subagents; a choice
// veils both panes rather than collapsing the layout.
func TestRailRidesWideWindows(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(false), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 36})
	if rw := m.railVisible(); rw != 130/3 {
		t.Fatalf("130 wide claims a third for the rail, got width %d", rw)
	}
	if r := ansi.Strip(m.Render()); !strings.Contains(r, "nothing written yet") || !strings.Contains(r, "mock/m") {
		t.Fatalf("the rail is missing its vitals or ledger:\n%s", r)
	}
	m.Update(tea.WindowSizeMsg{Width: 104, Height: 30})
	if rw := m.railVisible(); rw != 104/3 {
		t.Fatalf("104 wide claims a third, got width %d", rw)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.railVisible() != 0 {
		t.Fatal("80×24 stays one column")
	}
	if strings.Contains(ansi.Strip(m.Render()), "nothing written yet") {
		t.Fatal("the rail leaked into the narrow frame")
	}

	// the ledger: one line per path, latest write last, held in the writer's lane
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 36})
	m.Update(EvToolEnd{Tool: ToolView{ID: "s1", Name: "edit", Status: "done", Wrote: []string{"tui/blocks.go"}}})
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "watch"}})
	m.Update(EvAgentStep{Agent: "zone-auth", Tool: ToolView{ID: "w1", Name: "edit", Status: "done", Wrote: []string{"tui/model.go", "tui/blocks.go"}}, End: true})
	if len(m.files) != 2 || m.files[1].Path != "tui/blocks.go" {
		t.Fatalf("the ledger must dedupe per path, latest last: %+v", m.files)
	}
	r := ansi.Strip(m.Render())
	// the ledger rides a lipgloss tree now: one dim dir root with lane-held leaves under it
	if !strings.Contains(r, "tui/") || !strings.Contains(r, "model.go") || !strings.Contains(r, "zone-auth") || !strings.Contains(r, "THINKING") {
		t.Fatalf("the rail lost the ledger or the roster:\n%s", r)
	}

	// an open choice veils both panes — the sidebar stays, darkened with the rest (crush's
	// modal policy is focus-theft of KEYS, not a layout collapse)
	m.choice = &EvChoice{}
	if m.railVisible() == 0 {
		t.Fatal("the sidebar must not collapse when a choice opens")
	}
	if r := ansi.Strip(m.Render()); !strings.Contains(r, "vitals") && !strings.Contains(r, "█") || !strings.Contains(r, "│") {
		t.Fatalf("the sidebar must stay under the veil (card title (plain or pixel-drawn) + divider):\n%s", r)
	}
	m.choice = nil
}

// Slice 3's modal: on roomy windows the ask card floats centred over the darkened frame (real
// dim text, no alpha), its top edge gradients, it drops in over the first ticks; below the
// tripwire it keeps flowing inline — the pre-slice golden path.
func TestChoiceFloatsOverDarkenedBackdrop(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(true), DefaultWords())
	m.theme.Color = true
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 36})
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "watch the rail"}})
	reply := make(chan ChoiceAnswer, 1)
	m.Update(EvChoice{View: ChoiceView{Title: "Approval", Class: "outward", Tool: "bash", Summary: "git push origin main", Options: []string{"Yes", "No"}}, Reply: reply})
	m.choiceAt = time.Now().Add(-time.Second) // past the drop

	if !m.choiceFloat() {
		t.Fatal("130×36 must float the choice")
	}
	full := m.Render()
	strip := ansi.Strip(full)
	// the backdrop is still there under the card…
	if !strings.Contains(strip, "zone-auth") || !strings.Contains(strip, "mock/m") {
		t.Fatalf("the modal lost the frame under it:\n%s", strip)
	}
	// …darkened: a live-area line rides the dim colour, not its own
	var backdrop string
	for _, l := range strings.Split(full, "\n") {
		if strings.Contains(ansi.Strip(l), "watch the rail") {
			backdrop = l
		}
	}
	if !strings.Contains(backdrop, "38;2;127;138;153") { // the dark theme's dim #7f8a99
		t.Fatalf("the backdrop must be darkened to dim, got: %q", backdrop)
	}
	// the card centres (geometry computed from the same inputs, not string archaeology — the
	// sidebar's cards own borders of their own in the frame now)
	box := m.theme.Choice(m.view, min(m.width*3/5, 100))
	bw, bh := lipgloss.Width(box), strings.Count(box, "\n")+1
	ex, ey := centerRect(130, 36, bw, bh)
	if got := cellAt(strip, ex+2, ey+1); got != 'A' { // border + one padding, then "Approval"
		runes := []rune(strings.Split(strip, "\n")[ey+1])
		t.Fatalf("the card must centre: want 'A' at (%d,%d), got %q (row %q)", ex+2, ey+1, got, string(runes))
	}
	// the card's top edge gradients accent → border (per-rune colours on ╭─╮)
	topLine := strings.Split(full, "\n")[ey]
	if stops := strings.Count(topLine, "38;2;"); stops < 4 {
		t.Fatalf("the card's top edge must gradient (want several fg stops, got %d)", stops)
	}
	// the enter drop: a fresh card rides 2 rows lower
	m.choiceAt = time.Now()
	freshRow := strings.Split(ansi.Strip(m.Render()), "\n")[ey+3]
	if !strings.Contains(freshRow, "Approval") {
		t.Fatalf("fresh card must drop in 2 rows below settled (row %d): %q", ey+3, freshRow)
	}
	// and below the tripwire the card flows inline again
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 18})
	if m.choiceFloat() {
		t.Fatal("70×18 must flow the choice inline")
	}
	if !strings.Contains(ansi.Strip(m.Render()), "Approval") {
		t.Fatal("the inline card went missing on the small window")
	}
}

// cellAt is the rune at (x, y) of a stripped render frame.
func cellAt(strip string, x, y int) rune {
	lines := strings.Split(strip, "\n")
	if y < 0 || y >= len(lines) {
		return 0
	}
	runes := []rune(lines[y])
	if x < 0 || x >= len(runes) {
		return 0
	}
	return runes[x]
}

// The slice-4 verdict burst: a gate verdict after Subagents ran rains the glyph for six ticks and
// never again; a turn without Subagents keeps its verdict quiet.
func TestVerdictBurstRainsOnce(t *testing.T) {
	h := &fakeHost{}
	m := New(h, NewTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 36})

	// a courtless verdict arms nothing
	m.begin()
	m.Update(EvTurnDone{Status: "DONE", Verdict: "pass · 2 files"})
	if m.burstOn {
		t.Fatal("a courtless verdict must not rain")
	}

	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", Rank: "zone", State: "thinking", Ask: "watch the burst"}})
	m.Update(EvSubagent{Subagent: SubagentView{Name: "zone-auth", State: "done", Elapsed: time.Second, Report: &ReportView{Status: "PASS", Findings: []string{"f"}}}})
	m.begin()
	m.Update(EvTurnDone{Status: "DONE", Verdict: "pass · 1 file"})
	if !m.burstOn || !m.burstGood {
		t.Fatal("a courted pass must arm the rain")
	}
	rain := ansi.Strip(m.Render())
	if n := strings.Count(rain, "◆"); n < 3 {
		t.Fatalf("the rain must carry several glyphs, got %d", n)
	}
	// spent: past the last tick the sky clears
	m.burstAt = time.Now().Add(-(burstTicks + 1) * time.Second / pulseFPS)
	m.Render()
	if m.burstOn {
		t.Fatal("the rain must expire")
	}
	if strings.Count(ansi.Strip(m.Render()), "◆") > strings.Count(rain, "◆") {
		t.Fatal("the rain kept falling after its ticks")
	}
	// a fail rains in the bad colour
	m.Update(EvSubagent{Subagent: SubagentView{Name: "domain-security", Rank: "domain", State: "thinking"}})
	m.begin()
	m.Update(EvTurnDone{Status: "FAIL", Verdict: "fail: outside ownership"})
	m.burstAt = time.Now().Add(-time.Second / pulseFPS)
	if out := m.Render(); !strings.Contains(out, "224;108;95") { // dark theme bad #e06c5f
		t.Fatalf("a failed verdict must rain bad-coloured: %q", out)
	}
}

func TestChoiceBlock(t *testing.T) {
	h := &fakeHost{}
	tm, _, out := start(t, h)
	reply := make(chan ChoiceAnswer, 1)
	tm.Send(EvChoice{View: ChoiceView{Title: "Approval", Class: "outward", Tool: "bash", Summary: "git push origin main", Why: "outward",
		Options: []string{"Yes", "Yes, and don't ask again for bash:git push*", "No, and tell the model why"}, Reasons: []string{"", "", "why not?"}}, Reply: reply})
	out.wait(t, "1 · Yes")
	tm.Send(key(tea.KeyDown))
	tm.Send(key(tea.KeyDown))
	tm.Send(key(tea.KeyEnter))
	out.wait(t, "why not?")
	tm.Type("tests first")
	tm.Send(key(tea.KeyEnter))
	select {
	case a := <-reply:
		if a.Index != 2 || a.Text != "tests first" {
			t.Fatalf("answer %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no answer")
	}
	out.wait(t, "✓ No, and tell the model why: tests first")
	// a digit answers at once
	reply2 := make(chan ChoiceAnswer, 1)
	tm.Send(EvChoice{View: ChoiceView{Title: "Question", Why: "which?", Options: []string{"a", "b"}, Free: true}, Reply: reply2})
	out.wait(t, "type an answer")
	tm.Type("2")
	select {
	case a := <-reply2:
		if a.Index != 1 {
			t.Fatalf("answer %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no answer")
	}
	// free text on a question
	reply3 := make(chan ChoiceAnswer, 1)
	tm.Send(EvChoice{View: ChoiceView{Title: "Question", Why: "name?", Options: []string{"a"}, Free: true}, Reply: reply3})
	out.wait(t, "name?")
	tm.Type("zed")
	tm.Send(key(tea.KeyEnter))
	select {
	case a := <-reply3:
		if a.Index != -1 || a.Text != "zed" {
			t.Fatalf("answer %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no answer")
	}
	tm.Send(EvQuit{})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestSafeCut(t *testing.T) {
	s := "para one\n\n```go\nx\n\ny\n```\n\npara three\n\n- a\n- b\n\nlast"
	cut := safeCut(s)
	head := s[:cut]
	if s[cut:] != "last" || !strings.HasSuffix(head, "\n\n") {
		t.Fatalf("cut at %d: %q", cut, head)
	}
	if strings.Count(head, "```")%2 != 0 {
		t.Fatal("cut inside a fence")
	}
	if safeCut("no blank line yet") != 0 {
		t.Fatal("cut without a paragraph")
	}
}

// waitPiped polls the fake host's piped inbox until it holds anything (keys may ride in one
// paste under load — the pane's visuals are not the assertable order; the host's inbox is).
func waitPiped(h *fakeHost, d time.Duration) []string {
	deadline := time.Now().Add(d)
	for {
		h.mu.Lock()
		p := h.piped
		h.mu.Unlock()
		if len(p) > 0 {
			return p
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (t *tail) raw() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}

// A width change clears the screen and the scrollback once the drag settles, then prints the
// transcript again at the new width: the terminal's own rewrap of the live area left ghosts.
func TestResizeReprintsAtTheNewWidth(t *testing.T) {
	h := &fakeHost{}
	tm, m, out := start(t, h)
	long := strings.Repeat("word ", 30)
	tm.Send(EvNotice{Text: long})
	out.wait(t, "word word")
	for _, w := range []int{70, 60, 50, 40} { // a drag: one reprint, not four
		tm.Send(tea.WindowSizeMsg{Width: w, Height: 24})
	}
	deadline := time.Now().Add(5 * time.Second)
	for strings.Count(out.raw(), "\x1b[3J") == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no reprint after the width changed")
		}
		time.Sleep(15 * time.Millisecond)
	}
	time.Sleep(3 * reflowDelay)
	raw := out.raw()
	if n := strings.Count(raw, "\x1b[3J"); n != 1 {
		t.Fatalf("%d reprints for one drag, want 1", n)
	}
	after := ansi.Strip(raw[strings.Index(raw, "\x1b[3J"):])
	if !strings.Contains(after, "word") {
		t.Fatalf("the reprint lost the transcript:\n%s", after)
	}
	for _, ln := range strings.Split(after, "\n") {
		if strings.Contains(ln, "word") && ansi.StringWidth(strings.TrimRight(ln, " \r")) > 40 {
			t.Fatalf("a reprinted line is wider than the new 40 columns: %q", ln)
		}
	}
	if m.lastWidth != 40 {
		t.Fatalf("lastWidth %d, want 40", m.lastWidth)
	}
	// the same width again is not a change
	tm.Send(tea.WindowSizeMsg{Width: 40, Height: 30})
	time.Sleep(3 * reflowDelay)
	if n := strings.Count(out.raw(), "\x1b[3J"); n != 1 {
		t.Fatalf("a height-only change reprinted (%d)", n)
	}
}
