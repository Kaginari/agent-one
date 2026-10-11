package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/tui"
	"github.com/Kaginari/agent-one/wire"
	"github.com/Kaginari/agent-one/workspace"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// The junction of the terminal UI with the real engine: the app's host under teatest, the mock
// provider scripted, real tools and the real classifier behind the gate.

type tmSender struct{ tm *teatest.TestModel }

func (s tmSender) Send(m tea.Msg) { s.tm.Send(m) }

// screen is what a junction test reads: the program's byte stream, and — asked of the program
// on its own goroutine — the conversation as it stands (tui.Model.Transcript). The stream alone
// is not enough: the renderer writes only the cells that changed, so a line that changed in part
// ("⎿ running…" → "⎿ ok · 4ms") never reaches the bytes whole, and the frame alone shows only the
// pane's window. A wait passes on either; nothing sleeps as synchronisation.
type screen struct {
	mu  sync.Mutex
	buf bytes.Buffer
	tm  *teatest.TestModel // set: reads ask the program for its transcript
}

// framed wraps the model so the test can ask for the transcript inside an Update.
type framed struct{ m *tui.Model }

type transcriptReq chan string

func (f framed) Init() tea.Cmd { return f.m.Init() }
func (f framed) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch req := msg.(type) {
	case transcriptReq:
		req <- f.m.Transcript()
		return f, nil
	case frameReq:
		req <- ansi.Strip(f.m.Render())
		return f, nil
	}
	_, cmd := f.m.Update(msg)
	return f, cmd
}
func (f framed) View() tea.View { return f.m.View() }

// transcript is the conversation's plain text with the live frame under it, or "" when the
// program does not answer in time (it has ended, or is busy past the bound).
func (s *screen) transcript() string {
	if s.tm == nil {
		return ""
	}
	req := make(transcriptReq, 1)
	go s.tm.Send(req) // never block the test on a program that has stopped reading
	select {
	case f := <-req:
		return f
	case <-time.After(time.Second):
		return ""
	}
}

type frameReq chan string

// frame is the live frame alone — what the window shows now — or "" when the program does not
// answer in time.
func (s *screen) frame() string {
	if s.tm == nil {
		return ""
	}
	req := make(frameReq, 1)
	go s.tm.Send(req)
	select {
	case f := <-req:
		return f
	case <-time.After(time.Second):
		return ""
	}
}

func (s *screen) follow(r io.Reader) {
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := r.Read(b)
			if n > 0 {
				s.mu.Lock()
				s.buf.Write(b[:n])
				s.mu.Unlock()
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

// text is the byte stream so far, stripped — order-faithful for whole lines only.
func (s *screen) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ansi.Strip(s.buf.String())
}

// shows: want is on screen — in the byte stream, or in the transcript and live frame.
func (s *screen) shows(want string) bool {
	return strings.Contains(s.text(), want) || strings.Contains(s.transcript(), want)
}

// waitFrame waits for want in the window itself — the pane's visible rows, not the scrollback.
func (s *screen) waitFrame(t testing.TB, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(s.frame(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("waited for %q in the window; the frame:\n%s", want, s.frame())
		}
		time.Sleep(15 * time.Millisecond) // a poll interval, not a synchronisation
	}
}

func (s *screen) wait(t testing.TB, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for tick := 0; ; tick++ {
		if strings.Contains(s.text(), want) || (tick%4 == 3 && strings.Contains(s.transcript(), want)) {
			return
		}
		if time.Now().After(deadline) {
			if strings.Contains(s.transcript(), want) {
				return
			}
			t.Fatalf("waited for %q; the transcript:\n%s\nthe stream so far:\n%s", want, s.transcript(), s.text())
		}
		time.Sleep(15 * time.Millisecond) // a poll interval, not a synchronisation
	}
}

// tuiSessionHost is tuiSession keeping the host — tests that drive the host's own seams
// (Queue, Interrupt) rather than only keys need it.
func tuiSessionHost(t *testing.T, w *testWorkspace) (*tuiHost, *teatest.TestModel, *screen) {
	t.Helper()
	a := w.open()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h, err := a.tuiHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m := tui.New(h, tui.NewTheme(true), h.words)
	tm := teatest.NewTestModel(t, framed{m}, teatest.WithInitialTermSize(100, 30))
	m.Attach(tm.Send)
	h.attach(tmSender{tm})
	sc := &screen{tm: tm}
	sc.follow(tm.Output())
	sc.wait(t, "⏎ ask") // the welcome card itself, not the header that is drawn before it
	t.Cleanup(func() {
		tm.Send(tui.EvQuit{})
		tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
		settle(h) // a turn still running writes into the workspace: let it end before the temp dir goes
	})
	return h, tm, sc
}

// tuiSession opens the app on a test workspace and drives its host under teatest.
func tuiSession(t *testing.T, w *testWorkspace) (*App, *teatest.TestModel, *screen) {
	t.Helper()
	a := w.open()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h, err := a.tuiHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m := tui.New(h, tui.NewTheme(true), h.words)
	tm := teatest.NewTestModel(t, framed{m}, teatest.WithInitialTermSize(100, 30))
	m.Attach(tm.Send)
	h.attach(tmSender{tm})
	sc := &screen{tm: tm}
	sc.follow(tm.Output())
	sc.wait(t, "⏎ ask") // the welcome card itself, not the header that is drawn before it
	t.Cleanup(func() {
		tm.Send(tui.EvQuit{})
		tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
		settle(h) // a turn still running writes into the workspace: let it end before the temp dir goes
	})
	return a, tm, sc
}

func enter(tm *teatest.TestModel) { tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter}) }

func TestTUIJunctionTurn(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("say hi", map[string]interface{}{"text": "Let me run it.\n\n", "calls": []interface{}{map[string]interface{}{"name": "bash", "input": map[string]string{"command": "echo hi; echo there"}}}}),
		when("there", call("edit", map[string]string{"path": "README.md", "old": "hello", "new": "hello\n\nthe login zone"})),
		when("edited", text("All **done**: `hi` was said and the file edited.\n\n```go\npackage auth\n```\n")),
	)
	// Policy 7 (orchestrator-never-authors) is default-on; this rendering test's mock orchestrator edit of
	// no-member soil (README.md) must stay lawful — the fixture opts the flag off.
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", "policy: {ownership: {orchestratorNeverAuthors: false}}\n"))
	_, tm, sc := tuiSession(t, w)
	tm.Type("say hi")
	enter(tm)
	sc.wait(t, "▌ say hi")
	sc.wait(t, "● bash  echo hi; echo there  read")
	sc.wait(t, "⎿ ok · ")
	sc.wait(t, "there")
	sc.wait(t, "● edit  README.md  write")
	sc.wait(t, "+2 −0") // the edit's own result: ctrl+o opens the last folded block, which must be it
	tm.Send(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	sc.wait(t, "+ the login zone")
	sc.wait(t, "hi was said and the file edited")
	sc.wait(t, "✓ gate · ")
	s := sc.transcript()
	if i, j := strings.Index(s, "Let me run it."), strings.Index(s, "● bash"); i < 0 || j < 0 || i > j {
		t.Fatalf("the streamed text lands before the tool block:\n%s", s)
	}
	if !strings.Contains(s, "+2 −0") {
		t.Fatalf("the diff stat is missing:\n%s", s)
	}
	if i, j := strings.Index(s, "⏎ ask"), strings.Index(s, "▌ say hi"); i < 0 || i > j {
		t.Fatalf("the welcome must head the conversation, before the first ask:\n%s", s)
	}
	if !strings.Contains(w.read(".agent-one/log.md"), "Gate") {
		t.Fatal("the gate verdict did not reach log.md")
	}
	// the footer reads the instruments
	sc.wait(t, "│ ctx ")
	// a slash command through the menu
	tm.Type("/stat")
	sc.wait(t, "❯ /status")
	enter(tm)
	sc.wait(t, "sandbox: none")
	// pane mode: the output is the conversation's last block and the pane follows it — in the
	// window, never printed above the frame, never left at the transcript's top
	sc.waitFrame(t, "sandbox: none")
	sc.waitFrame(t, "budget subagent: off")
	if s := sc.transcript(); strings.Index(s, "▌ /status") > strings.Index(s, "sandbox: none") {
		t.Fatalf("the status output is not under its command:\n%s", s)
	}
}

func TestTUIApprovalYes(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		text("push attempted"),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	_, tm, sc := tuiSession(t, w)
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "1 · Yes")
	if !sc.shows("outward  bash  git push origin main") || !sc.shows("don't ask again for bash:git push*") {
		t.Fatalf("approval block:\n%s", sc.transcript())
	}
	enter(tm)
	sc.wait(t, "✓ Yes")
	sc.wait(t, "push attempted")
	if j := w.journalText(".agent-one"); !strings.Contains(j, `"by":"tty"`) || !strings.Contains(j, `"decision":"approved"`) {
		t.Fatalf("the journal does not carry the operator's approval:\n%s", j)
	}
	if _, err := readFile(w, ".agent-one/config.local.yaml"); err == nil {
		t.Fatal("a plain yes must not write a rule")
	}
}

func TestTUIApprovalDontAskAgain(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		call("bash", map[string]string{"command": "git push origin main"}),
		text("pushed twice"),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	w.write(".agent-one/config.local.yaml", "permissions:\n  rules:\n    - {match: \"bash:make *\", action: allow}\n")
	_, tm, sc := tuiSession(t, w)
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "1 · Yes")
	tm.Type("2")
	sc.wait(t, "rule bash:git push* → allow · in .agent-one/config.local.yaml")
	sc.wait(t, "pushed twice")
	local, _ := readFile(w, ".agent-one/config.local.yaml")
	if !strings.Contains(local, `bash:git push*`) || !strings.Contains(local, `bash:make *`) {
		t.Fatalf("the local layer lost a rule or gained none:\n%s", local)
	}
	j := w.journalText(".agent-one")
	if strings.Count(j, `"by":"tty"`) != 1 || !strings.Contains(j, `"by":"rule"`) {
		t.Fatalf("the second push must pass by the rule, live:\n%s", j)
	}
}

func TestTUIApprovalNoWithReason(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		when("run the tests first", text("Understood: I will run the tests before any push.")),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	_, tm, sc := tuiSession(t, w)
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "1 · Yes")
	tm.Type("3")
	sc.wait(t, "why not?")
	tm.Type("run the tests first")
	enter(tm)
	sc.wait(t, "✓ No, and tell the model why: run the tests first")
	sc.wait(t, "denied at the gate")
	sc.wait(t, "▌ run the tests first")
	sc.wait(t, "I will run the tests before any push")
	j := w.journalText(".agent-one")
	if !strings.Contains(j, `"by":"tty"`) || !strings.Contains(j, `"decision":"denied"`) {
		t.Fatalf("the journal does not carry the denial:\n%s", j)
	}
	if !strings.Contains(w.read(".agent-one/instruments/loop/"+firstJournal(w)), "the operator said: run the tests first") {
		t.Fatal("the reason did not reach the model's tool result")
	}
}

func TestTUISubagentsAndQuestion(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/session.json",
		when("look", call("dispatch", map[string]interface{}{"agent": "zone-auth", "ask": "findings: look at the login zone"})),
		when("@F src/auth/login.go:1 fine", call("dispatch", map[string]interface{}{"agent": "zone-api", "ask": "findings: look at the api", "background": true})),
		when("started in the background", call("ask", map[string]interface{}{"question": "Which zone next?", "options": []string{"auth", "api"}})),
		when("api", text("The api zone it is.")),
		when("[subagent zone-api reported]", text("Both subagents reported.")),
	)
	w.script(".agent-one/tmp/subagent.json",
		when("login zone", text("@S PASS\n@F src/auth/login.go:1 fine\n@? no test for refresh\n@U team subagents can run behind\n@E 80")),
		when("the api", call("bash", map[string]string{"command": "sleep 1; echo slept"})),
		when("slept", text("@S PASS\n@F src/api/server.go:1 fine too\n@U domain the api zone serves json\n@E 70")),
	)
	w.write(".agent-one/config.yaml", `
models: {default: mock/m, roles: {analyst: subagent/c}}
providers:
  mock: {enabled: true, script: .agent-one/tmp/session.json}
  subagent: {type: mock, script: .agent-one/tmp/subagent.json}
tools: {bash: {sandbox: none}, dispatch: {background: true, strictFlow: false}}
ui: {board: {autostart: false}}
`)
	_, tm, sc := tuiSession(t, w)
	tm.Type("look")
	enter(tm)
	sc.wait(t, "→ subagent zone-auth started")
	sc.wait(t, "◆ subagent zone-auth · zone · analyst · subagent/c")
	sc.wait(t, "⎿ PASS")
	sc.wait(t, "● src/auth/login.go:1 fine")
	sc.wait(t, "∴ team: subagents can run behind")
	sc.wait(t, "1 · auth")
	tm.Type("2")
	sc.wait(t, "The api zone it is.")
	sc.wait(t, "◆ subagent zone-api")
	sc.wait(t, "● src/api/server.go:1 fine too")
	sc.wait(t, "↻ continuing with what arrived")
	sc.wait(t, "Both subagents reported.")
}

func TestTUIInterruptAndQueue(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	// "a note" must NEVER appear as a turn prompt in the running "slow" turn (no
	// mid-turn injection). It runs as its own turn after esc pivots to it.
	w.script(".agent-one/tmp/s.json",
		when("slow", call("bash", map[string]string{"command": "sleep 5; echo late"})),
		when("late", text("late came")),
		when("a note", text("the note ran as its own turn")),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	_, tm, sc := tuiSession(t, w)
	tm.Type("slow")
	enter(tm)
	sc.wait(t, "Running bash…")
	tm.Type("a note")
	enter(tm)
	sc.wait(t, "⏎ queued: a note")
	// esc pivots: the running turn aborts and the queued line fires as the next turn.
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	sc.wait(t, "■ interrupted")
	sc.wait(t, "the note ran as its own turn")
	// the FIFO contract: "a note" waited for "slow" to end, then ran as its OWN turn —
	// never a mid-turn injection into the running turn's messages. Assert via the queue's
	// observable ordering: the queued echo precedes the interrupt, and the note's turn
	// output follows it. A regression that injects mid-turn would show the note reaching
	// the model while "slow" was still running — the mock script has no step answering a
	// mid-turn "a note", so any injection yields no turn and this wait fails:
	if s := sc.transcript(); strings.Index(s, "■ interrupted") > strings.Index(s, "the note ran as its own turn") {
		t.Fatalf("the note's turn ran before the interrupt:\n%s", s)
	}
}

func TestTUIGateCardFreedByInterrupt(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	// Freeze-policy regression (2026-10-07): a gate card left unanswered wedged the whole
	// workspace in h.approve's session-ctx select — esc could not free it, queued asks never
	// drained. The card's wait now selects on the turn ctx too: esc resolves the card as
	// an interruption-denial AND the queued ask runs as the next turn.
	w.script(".agent-one/tmp/s.json",
		when("push it", call("bash", map[string]string{"command": "git push origin main"})),
		when("hello", text("hello back")),
	)
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	h, tm, sc := tuiSessionHost(t, w) // keeps the host: the queue entry point rides it directly
	tm.Type("push it")
	enter(tm)
	sc.wait(t, "1 · Yes") // the card is up; nothing answers it
	// the human's voice arrives the DASHBOARD way while the card holds the gate
	// (the 2026-10-07 freeze's real buffer: Queue → queuedBuf → drained only at finish)
	if why := h.Queue("hello"); !strings.Contains(why, "queued") {
		t.Fatalf("the queue word did not read as queued: %q", why)
	}
	if sc.shows("hello back") {
		t.Fatal("the workspace answered while its card was open — the queue must wait")
	}
	// the dashboard's interrupt endpoint (what a human hits while the modal holds the
	// workspace) calls exactly this: Interrupt → turn ctx fires → the card's wait resolves
	h.Interrupt()
	sc.wait(t, "■ interrupted") // freed — the workspace was never stuck on a modal
	sc.wait(t, "hello back")    // and the queued ask ran as the very next turn
	j := w.journalText(".agent-one")
	if !strings.Contains(j, `"decision":"denied"`) || !strings.Contains(j, "interrupted while waiting") {
		t.Fatalf("the freed card did not journal as an interruption-denial:\n%s", j)
	}
}

func TestWantsTUI(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json", text("nothing"))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	a := w.open()
	if a.wantsTUI(false) {
		t.Fatal("a pipe must keep the line REPL")
	}
	if a.wantsTUI(true) {
		t.Fatal("--plain must keep the line REPL")
	}
}

func readFile(w *testWorkspace, p string) (string, error) {
	b, err := os.ReadFile(filepath.Join(w.root, p))
	return string(b), err
}

func firstJournal(w *testWorkspace) string {
	ents, _ := os.ReadDir(filepath.Join(w.root, ".agent-one/instruments/loop"))
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			return e.Name()
		}
	}
	return ""
}

func TestTUIStartFailureReachesTheTerminal(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/config.yaml", "providers:\n  vllm: {type: openai, baseURL: http://127.0.0.1:1/v1, apiKeyEnv: VLLM_API_KEY}\nmodels: {default: vllm/fake/model}\n")
	a := w.open()
	errw := a.Opt.Err
	if code := a.TUI(context.Background()); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if got := errw.(*strings.Builder).String(); !strings.Contains(got, "VLLM_API_KEY is not set") {
		t.Fatalf("a TUI that cannot start must say why on stderr; got %q", got)
	}
}

// /board's view is the real workspace's: the reasoned ontology as a graph with its bonds and
// knowledge, the triad as config resolves it.
func TestTUIBoardReadsTheWorkspace(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json", text("nothing"))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	a := w.open()
	h, err := a.tuiHost(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v := h.Board("24h")
	byID := map[string]tui.GraphNode{}
	for _, n := range v.Graph.Nodes {
		byID[n.ID] = n
	}
	zone, ok := byID["zone-auth"]
	if !ok || len(byID) != 5 {
		t.Fatalf("graph nodes %v, want orchestrator, coord-core, domain-security, zone-auth, zone-api", v.Graph.Nodes)
	}
	if len(zone.Up) != 1 || zone.Up[0].To != "domain-security" || zone.Up[0].Bond != "truth" || zone.Rank != "zone" {
		t.Fatalf("zone-auth answers %v (rank %q), want domain-security by truth", zone.Up, zone.Rank)
	}
	if byID["orchestrator"].Level != 0 || byID["coord-core"].Level != 1 || byID["domain-security"].Level != 2 || zone.Level != 3 {
		t.Fatalf("levels: %+v", byID)
	}
	know := ""
	for _, kv := range zone.Knowledge {
		know += kv.Key + "=" + kv.Value + "\n"
	}
	if !strings.Contains(know, "chain=") || !strings.Contains(know, "orchestrator") || !strings.Contains(know, "owns=src/auth") {
		t.Fatalf("zone-auth's knowledge misses the inferred chain or its ownership:\n%s", know)
	}
	if !strings.Contains(v.Graph.Summary, "5 members") || !strings.Contains(v.Graph.Summary, "reasoned") {
		t.Fatalf("summary %q", v.Graph.Summary)
	}
	if len(v.Roles) != 3 || v.Roles[0].Name != "analyst" || v.Roles[0].Model != "mock/m" || len(v.Roles[0].Ranks) == 0 {
		t.Fatalf("roles %+v", v.Roles)
	}
}

// collector is a test sender that records the events a tuiHost emits.
type collector struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (c *collector) Send(m tea.Msg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
}

func (c *collector) events() []tea.Msg {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]tea.Msg(nil), c.msgs...)
}

func (c *collector) views() (subagents []tui.SubagentView, states []tui.EvState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.msgs {
		switch e := m.(type) {
		case tui.EvSubagent:
			subagents = append(subagents, e.Subagent)
		case tui.EvState:
			states = append(states, e)
		}
	}
	return
}

// An agent dispatched down the ladder by a drained @$ order never lands a subagentReport — its
// terminal state arrives only through the loop's State hook ("done"). The banner must close
// there, exactly once, instead of staying "thinking" with a running timer (the stale-subagent bug
// Operator saw: banners reading thinking·44m after the loop journal already said DONE).
func TestTUISubagentBannerClosesOnDoneState(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.write(".agent-one/config.yaml", `
models: {default: mock/m, roles: {analyst: subagent/c}}
providers:
  mock: {enabled: true}
  subagent: {type: mock}
ui: {board: {autostart: false}}
`)
	a := w.open()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := a.tuiHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c := &collector{}
	h.attach(c)

	// the Subagent knows the agent (as Track would have registered it at dispatch)
	h.a.Subagent.Track("zone-ledger", "zone", "analyst", "subagent/c", nil, 1)

	h.state("zone-ledger", "thinking") // opens the banner
	h.state("zone-ledger", "tool")     // mid-run updates ride EvState
	h.state("zone-ledger", "done")     // the drained agent's only terminal signal
	h.state("zone-ledger", "done")     // idempotent: no second banner, no panic

	subagents, states := c.views()
	var opens, closes int
	for _, v := range subagents {
		if v.Name != "zone-ledger" {
			continue
		}
		switch v.State {
		case "thinking":
			opens++
		case "done":
			closes++
			if v.Elapsed < 0 {
				t.Fatalf("closing banner carries a negative elapsed: %v", v.Elapsed)
			}
		}
	}
	if opens != 1 || closes != 1 {
		t.Fatalf("banners: %d opens, %d closes — want 1/1; all subagents: %+v", opens, closes, subagents)
	}
	for _, s := range states {
		if s.Agent == "zone-ledger" && s.State == "done" {
			t.Fatalf("a bare done EvState escaped — the banner would stay open: %+v", states)
		}
	}
	h.mu.Lock()
	seenLeft := h.subagentSeen["zone-ledger"]
	h.mu.Unlock()
	if seenLeft {
		t.Fatal("subagentSeen still holds the agent after done — the banner is still live")
	}
}

// A subagent's dispatch step seeds and lands its Subagent's card at EVERY depth (Operator's chain
// run, 2026-09-28): orchestrator → coord-team → domain-experience closed DOWN the loop, and the domain owner's
// card sat in the live area reading DONE long after the agent's clock axed it — the host saw
// the domain owner's steps only as coord-team's steps, so the report never reached the card. Now any
// agent's dispatch step end lands the child's card (its parent's log line rides along), and the
// session's own dispatch steps behave exactly as before.
func TestTUINestedDispatchStepsLandTheirSubagents(t *testing.T) {
	w := newTestWorkspace(t, "agent-one", agentOneMembers())
	w.script(".agent-one/tmp/s.json", text("nothing"))
	w.write(".agent-one/config.yaml", mockCfg(".agent-one/tmp/s.json", ""))
	a := w.open()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := a.tuiHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c := &collector{}
	h.attach(c)

	parent := &loop.Session{Engine: &loop.Engine{As: "coord-team"}}
	orchestrator := &loop.Session{Engine: &loop.Engine{As: workspace.Orchestrator}}
	input, _ := json.Marshal(map[string]string{"agent": "domain-x", "ask": "verdict on the trials"})

	// a subagent dispatches: the start seeds the child's ask; the parent's log still steps
	h.observe(parent, &loop.StepRecord{Tool: "dispatch", Input: input, Status: "running"}, "start")
	h.mu.Lock()
	seeded := h.subagentAsk["domain-x"]
	h.mu.Unlock()
	if seeded != "verdict on the trials" {
		t.Fatalf("the nested dispatch's start must seed the card's ask, got %q", seeded)
	}
	h.a.Subagent.Track("domain-x", "domain", "judge", "mock/r", nil, 2)
	h.state("domain-x", "thinking")
	h.state("domain-x", "done")
	// the step end carries the child's wire report: the card gets it
	h.observe(parent, &loop.StepRecord{Tool: "dispatch", Input: input, Status: "done",
		Result: tool.Result{Output: "@S DONE\n@F src/x.go:1 ok\n@U team nested subagents land their cards\n@E 20"}}, "end")
	// a failed dispatch's output is no wire report: no card report, the parent's log still closes
	h.observe(parent, &loop.StepRecord{Tool: "dispatch", Input: input, Status: "failed",
		Result: tool.Result{Output: "dispatch domain-x: subagent ran out of its 15.0-minute budget", Err: true}}, "end")

	// the session's own dispatch step: unchanged shape — no tool block, no agent-log lines
	h.observe(orchestrator, &loop.StepRecord{Tool: "dispatch", Input: input, Status: "running"}, "start")
	h.observe(orchestrator, &loop.StepRecord{Tool: "dispatch", Input: input, Status: "done",
		Result: tool.Result{Output: "@S DONE\n@F src/x.go:1 ok too\n@E 20"}}, "end")

	subagents, _ := c.views()
	var opens, reports int
	for _, v := range subagents {
		if v.Name != "domain-x" {
			continue
		}
		if v.State == "thinking" {
			opens++
			if v.Ask != "verdict on the trials" || v.Model != "mock/r" || v.Rank != "domain" || v.Role != "judge" {
				t.Fatalf("the nested card opened unseeded: %+v", v)
			}
		}
		if v.State == "done" && v.Report != nil {
			reports++
			if v.Exit != "PASSED" {
				t.Fatalf("a DONE journal verdict must read PASSED on the card face, got %q", v.Exit)
			}
		}
	}
	if opens != 1 || reports != 2 {
		t.Fatalf("opens %d, landed reports %d — want 1 open, 2 reports (parent's + session's): %+v", opens, reports, subagents)
	}
	var parentLogs, toolBlocks int
	for _, m := range c.events() {
		switch e := m.(type) {
		case tui.EvAgentStep:
			if e.Agent == "coord-team" && e.End {
				parentLogs++
			}
		case tui.EvToolStart, tui.EvToolEnd:
			toolBlocks++
		}
	}
	if parentLogs != 2 {
		t.Fatalf("the parent's own log lost steps to the child's card: %d step ends, want 2 (the reported one and the failed one)", parentLogs)
	}
	if toolBlocks != 0 {
		t.Fatalf("a dispatch step must never draw a generic tool block: %d", toolBlocks)
	}
}

// Subagent-card exit-verdict policy (harness leg 2c): a subagent whose run journal ended FAIL — the
// wire's @S carries the loop's status — must render "FAIL · Ns" on the card face, styled bad,
// with lifecycle "done" demoted to the dim ask line. Lifecycle returned ≠ verdict; the working notes
// must never print a bare "done · 2m35s" over a FAILED journal (the 2026-09-28 Policy-9 break).
func TestTUISubagentCardRendersExitVerdict(t *testing.T) {
	theme := tui.NewTheme(true)
	rep := wire.Report{Status: "FAIL — PASSED", Findings: []string{"src/x.go:1 broke"}}
	v := tui.SubagentView{
		Name: "domain-trials", Rank: "domain", Role: "judge", Model: "subagent/c",
		State: "done", Exit: exitVerdict(rep), Failed: true,
		Elapsed: 155 * time.Second, Ask: "rule on the trials",
		Report: reportView(rep), Word: "subagent",
	}
	if v.Exit != "FAIL" {
		t.Fatalf("exitVerdict(%q) = %q, want FAIL", rep.Status, v.Exit)
	}
	card := ansi.Strip(theme.Subagent(v, 100))
	if !strings.Contains(card, "FAIL · 2m35s") {
		t.Fatalf("the card face must read the journal verdict before the duration; got:\n%s", card)
	}
	if strings.Contains(card, "DONE · 2m35s") || strings.Contains(card, "done · 2m35s") {
		t.Fatalf("lifecycle done leaked onto the face over a FAIL verdict:\n%s", card)
	}
	// lifecycle survives demoted: the ask line is the tooltip tier
	if !strings.Contains(card, "rule on the trials · done") {
		t.Fatalf("lifecycle word must demote to the ask line; got:\n%s", card)
	}

	verdicts := map[string]string{
		"DONE": "PASSED", "ESCALATE": "ESCALATE", "DENIED": "DENIED", "INTERRUPTED": "INTERRUPTED", "FAIL": "FAIL",
		"": "", "PASSED": "", "SHINY": "", // legacy/junk: no verdict surface
	}
	for status, expect := range verdicts {
		if got := exitVerdict(wire.Report{Status: status}); got != expect {
			t.Fatalf("exitVerdict(%q) = %q, want %q", status, got, expect)
		}
	}
}

// settle interrupts the host's turn, if one runs, and waits for it to end (bounded).
func settle(h *tuiHost) {
	h.Interrupt()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		h.mu.Lock()
		running := h.running
		h.mu.Unlock()
		if !running {
			return
		}
	}
}
