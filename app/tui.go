package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/config/yaml"
	"github.com/Kaginari/agent-one/gate"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/tui"
	"github.com/Kaginari/agent-one/wire"
	"github.com/Kaginari/agent-one/workspace"
)

// TUI runs the live session as the terminal UI (canon/tui.md): the same engine, sessions,
// Subagents and gate as the REPL, drawn as blocks. It is chosen on a terminal; the REPL stays for
// pipes and --plain.
func (a *App) TUI(ctx context.Context) int {
	h, err := a.tuiHost(ctx)
	if err != nil {
		fmt.Fprintf(a.Opt.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	ui := tui.Start(ctx, h, tui.DetectThemeFor(a.Opt.Env, a.Cfg.Dist.EnvPrefix, a.Cfg.UI.Theme, a.Cfg.UI.Mode), h.words, nil, nil)
	// the zone worker lives in the sidebar now; the launch drop is retired
	h.attach(fanout{ui, a.webBus()})
	a.webBus().attach(h)
	defer a.webBus().close()
	// evolutions reach the terminal and the dashboard through the host; the run-start sweep
	// runs once both are up, so its toasts are seen
	// the run-start sweep lands before the first turn (its writes are never the turn's); what
	// it says is held until the program runs, then sent
	var held []tui.EvEvolution
	a.mu.Lock()
	a.evoSink = func(ev tui.EvEvolution) { held = append(held, ev) }
	a.mu.Unlock()
	a.evolveSweep()
	a.mu.Lock()
	a.evoSink = func(ev tui.EvEvolution) { h.send(ev) }
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.evoSink = nil; a.mu.Unlock() }()
	go func() {
		for _, ev := range held {
			h.send(ev)
		}
	}()
	code := h.run(ui.Run)
	// veil-exit: the human's stop is its own chronicle row — an end with no cause could read
	// earned DONE either way; this one names the hand (Operator 2026-10-03)
	a.logExit(h.session())
	return code
}

// logExit drops the exit row into the session journal: the human stopped the workspace between
// turns. Midnight audits count them separately from instrument-checkpointed turns. The cause
// names the lexicon's word for the human ("operator"); a workspace without a lexicon falls back to
// "the human" so the row is never anonymous.
func (a *App) logExit(s *loop.Session) {
	if s == nil || s.Journal == nil {
		return
	}
	cause := a.Lex.Human
	if cause == "" {
		cause = "the human"
	}
	s.Journal.Log(loop.Event{"t": "exit", "cause": cause, "ms": time.Since(s.Started).Milliseconds()})
}

// sender is where the host's events go: the program, or a test harness.
type sender interface{ Send(msg tea.Msg) }

// wantsTUI says whether the session should be the terminal UI: a terminal on both ends, not
// dumb, and neither --plain nor <PREFIX>PLAIN said otherwise.
func (a *App) wantsTUI(plain bool) bool {
	if plain || a.Opt.Plain {
		return false
	}
	if v := a.Opt.Env(a.Cfg.Dist.EnvPrefix + "PLAIN"); v != "" && v != "0" && v != "false" {
		return false
	}
	if a.Opt.Env("TERM") == "dumb" {
		return false
	}
	in, ok := a.Opt.In.(*os.File)
	if !ok || in != os.Stdin || !isTerminal(in) {
		return false
	}
	out, ok := a.Opt.Out.(*os.File)
	return ok && isTerminal(out)
}

// tuiHost is the app behind the program: it runs turns, routes the loop's events to blocks and
// answers the program's asks. in/out nil means the process terminal.
type tuiHost struct {
	a   *App
	ctx context.Context
	e   *loop.Engine
	s   *loop.Session
	ui  sender
	out *bytes.Buffer // what slash commands and the engine's writers print
	mu  sync.Mutex
	// turns counts the turn goroutines still running, finish included: waiting on it is the
	// only proof a turn's writes (journal, session sync) are over
	turns sync.WaitGroup

	cancel         context.CancelFunc
	turnCtx        context.Context // the running turn's ctx; gate/ask waits select on it so esc can free them
	running        bool
	streamed       bool
	turnStart      time.Time // when the running turn opened — the plan-outcome sweep's freshness window (decks updated inside it rode this turn)
	denyReason     string
	choiceOpen     int               // open gate/ask cards; finish never re-registers h.ctx under an open card
	pulse          time.Time         // last heartbeat (any send); the per-turn watchdog reads it
	suspending     bool              // watchdog's suspend layer is mid-flight: Interrupt resolves the card as suspend, not deny
	pendingSuspend *pendingSuspend   // the parked card waiting on the human's word to resume its run
	quiet          bool              // notices held back (a reload the UI already announces)
	before         map[string]string // step id → file text before a write-class act
	subagentAsk    map[string]string
	subagentSeen   map[string]bool
	subagentStart  map[string]time.Time
	files          []string
	filesAt        time.Time
	words          tui.Words
	exit           int
	plans          plansMemo // the rail's decks, read again only when they moved
}

func (a *App) tuiWords() tui.Words {
	w := tui.DefaultWords()
	w.Dist = a.Cfg.Dist.Name
	w.ConfigRel = filepath.ToSlash(filepath.Join(a.Cfg.Dist.WorkspaceDir, "config.local.yaml"))
	w.SessionName = workspace.Orchestrator
	return w
}

func (a *App) tuiHost(ctx context.Context) (*tuiHost, error) {
	// the engine writes no trace on the terminal the program owns; what it prints is drained
	// into notices
	a.Opt.Quiet = true
	buf := &bytes.Buffer{}
	h := &tuiHost{a: a, ctx: ctx, out: buf, before: map[string]string{}, subagentAsk: map[string]string{}, subagentSeen: map[string]bool{}, subagentStart: map[string]time.Time{}, words: a.tuiWords()}
	out, errw, quiet := a.Opt.Out, a.Opt.Err, a.Opt.Quiet
	a.Opt.Out, a.Opt.Err = buf, buf
	a.Notify = func(s string) {
		h.mu.Lock()
		quiet := h.quiet
		h.mu.Unlock()
		if !quiet {
			h.send(tui.EvNotice{Text: s})
		}
	}
	// the gate's choice block, the ask tool's question
	a.Gate.Answer = h.approve
	a.Shelf.Asker = h.ask
	a.Missing.Ask = h.ask
	// the loop's events
	a.OnDelta = func(t string) {
		h.mu.Lock()
		h.streamed = true
		h.mu.Unlock()
		h.send(tui.EvDelta{Text: t})
	}
	a.OnStep = h.observe
	a.OnStream = func(agent, kind, text string) {
		// the session's text arrives as EvDelta; its thinking, and every Subagent's stream, as EvStream
		if agent == workspace.Orchestrator && kind == "text" {
			return
		}
		h.send(tui.EvStream{Agent: agent, Kind: kind, Text: text})
	}
	prevState := a.OnState
	a.OnState = func(agent, state string) {
		if prevState != nil {
			prevState(agent, state)
		}
		h.state(agent, state)
	}
	a.Subagent.onFinish = h.subagentFinished
	a.planBoot = time.Now() // plan decks penned before now are last session's memory, never gate-holders
	e, err := a.Engine()
	if err != nil {
		// no program will own the terminal: why it did not start is said there
		a.Opt.Out, a.Opt.Err, a.Opt.Quiet = out, errw, quiet
		return nil, err
	}
	h.e = e
	h.s = e.NewSession()
	a.Inbox = func(ls *loop.Session) []string { return a.Subagent.Inbox(agentName(ls)) }
	a.Subagent.onWake = h.woke
	return h, nil
}

func (h *tuiHost) attach(ui sender) {
	h.mu.Lock()
	h.ui = ui
	h.mu.Unlock()
}

func (h *tuiHost) run(loop func() error) int {
	a := h.a
	a.Hooks.SessionStart(h.ctx)
	err := loop()
	h.mu.Lock()
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	// the turn goroutine ends on its own; give it a moment to sync the session
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		running := h.running
		h.mu.Unlock()
		if !running || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil && err != context.Canceled {
		fmt.Fprintln(os.Stderr, "tui: "+err.Error())
		return 2
	}
	return h.exit
}

func (h *tuiHost) send(ev tea.Msg) {
	h.mu.Lock()
	ui := h.ui
	h.pulse = time.Now() // the workspace's last heartbeat the watchdog reads — every event out is a breath
	h.mu.Unlock()
	if ui != nil {
		ui.Send(ev)
	}
}

// sendQuiet speaks without a breath: the watchdog's own cries must not re-arm the silence
// meter they report on (a notice that pulses would notice itself forever).
func (h *tuiHost) sendQuiet(ev tea.Msg) {
	h.mu.Lock()
	ui := h.ui
	h.mu.Unlock()
	if ui != nil {
		ui.Send(ev)
	}
}

// --- tui.Host ---

func (h *tuiHost) Welcome() tui.Welcome {
	a := h.a
	w := tui.Welcome{Dist: a.Cfg.Dist.Name, Version: orStr(a.Opt.Version.Version, "dev"), Workspace: tilde(a.Root, a.Opt.Home), WorkspaceWord: h.words.Workspace, Model: a.mountModel.Ref.Model, Session: a.SessionID}
	if p, age := a.latestHandoff(); p != "" && age < 14*24*time.Hour {
		w.Handoff = relOrAbs(a.Root, p) + " (" + age.Round(time.Minute).String() + " ago) — /handoff read"
	}
	var roles []string
	for _, o := range sortedKeys(a.Cfg.Models.Roles) {
		roles = append(roles, o+" → "+a.Cfg.Models.Roles[o].Model)
	}
	w.Roles = roles
	w.Off = len(a.Cfg.Off())
	w.Holes = a.Holes()
	a.mu.Lock()
	if a.boardAddr != "" {
		w.Dash = "http://" + a.boardAddr + "/"
	}
	a.mu.Unlock()
	return w
}

func (h *tuiHost) Footer() tui.FooterView {
	a := h.a
	tot := a.Journal.Total()
	cost := "unpriced"
	switch {
	case tot.Calls == 0:
		cost = "no calls yet"
	case tot.Unpriced == 0:
		cost = fmt.Sprintf("$%.4f", tot.USD)
	}
	ctx, pct, known, zone := "ctx —", 0, false, ""
	ctxMax := "∞"
	var ctxTokens, ctxLimit int
	if r := readingOf(h.session()); r.Available {
		pct, known = r.Percent(), true
		ctx = fmt.Sprintf("ctx %d%%", r.Percent())
		if r.Stressed() {
			ctx += " (stress)"
		}
		zone = string(r.Zone)
		if r.Limit > 0 {
			ctxMax = humanish(r.Limit)
		}
		ctxTokens, ctxLimit = r.Tokens, r.Limit
	}
	fv := tui.FooterView{Model: a.mountModel.Ref.Model, Ctx: ctx, Cost: cost, Live: a.Subagent.Live(), Workspace: tilde(a.Root, a.Opt.Home), Subagents: h.words.Subagents, Tokens: tot.Tokens(), CtxPct: pct, CtxKnown: known, Zone: zone, CtxMax: ctxMax, CtxTokens: ctxTokens, CtxLimit: ctxLimit}
	a.mu.Lock()
	if a.boardAddr != "" {
		fv.Dash = "http://" + a.boardAddr + "/"
	}
	a.mu.Unlock()
	ranks := a.Journal.Ranks()
	order := []string{"orchestrator", "coord", "domain", "zone", "service", "auditor", "principal-domain-owner", "principal-coordinator"}
	seen := map[string]bool{}
	name, mode := a.look()
	th := tui.DetectThemeFor(func(k string) string { return a.Opt.Env(k) }, a.Cfg.Dist.EnvPrefix, name, mode)
	mk := func(rank string) {
		if seen[rank] || rank == "" {
			return
		}
		seen[rank] = true
		s, ok := ranks[rank]
		if !ok {
			return
		}
		fv.Layers = append(fv.Layers, tui.RailLayer{Rank: rank, Tokens: s.Tokens(), Lane: th.Lane(rank)})
	}
	for _, r := range order {
		mk(r)
	}
	for r := range ranks {
		mk(r)
	}
	a.footerValue(&fv)
	return fv
}

// humanish is the ctx cap in k/M ("128k", "1M"): what the panel shows before "∞" — the
// sign it ships when nothing's configured.
func humanish(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%dM", tokens/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%dk", tokens/1_000)
	}
	return fmt.Sprintf("%d", tokens)
}

func (h *tuiHost) Submit(text string) string {
	if why := h.a.Hooks.UserPrompt(h.ctx, text); why != "" {
		return "refused by a userPrompt hook: " + why
	}
	h.start(text, false)
	return ""
}

func (h *tuiHost) Queue(text string) string {
	h.a.webBus().note(map[string]any{"type": "ask", "text": text, "queued": true})
	// the Subagent's next-turn lane, unified (freeze policy): never a host-side buffer that a
	// dead turn can own forever; finish's DrainAll reaches it on every exit path.
	if err := h.a.Subagent.Enqueue(workspace.Orchestrator, text); err != nil {
		return err.Error()
	}
	return "queued — runs after the current turn (esc interrupts and takes it next)"
}

// Interrupt aborts the running turn. Any queued line is left in the inbox, so the
// turn-done drain immediately starts it as the next turn — esc pivots, never resumes
// the aborted task.
func (h *tuiHost) Interrupt() {
	h.mu.Lock()
	cancel := h.cancel
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Pipe hands a line typed at an agent in the agents pane to that agent's inbox: it lands at its
// next turn boundary (the FIFO contract — never mid-turn). A done agent refuses: its context
// died with its task.
func (h *tuiHost) Pipe(agent, text string) string {
	name := strings.ToLower(agent)
	for _, b := range h.a.Subagent.Agents() {
		if b.Name == name {
			if b.State == "done" {
				return name + " is done — its context died with its task; dispatch it again"
			}
			break
		}
	}
	if err := h.a.Subagent.Enqueue(name, text); err != nil {
		return err.Error()
	}
	return "piped into " + name + "'s inbox — it lands at its next turn boundary"
}

func (h *tuiHost) Slash(line string) ([]string, bool) {
	h.mu.Lock()
	busy := h.running
	h.mu.Unlock()
	quit := h.a.slash(h.ctx, line, h.session(), busy)
	return h.drain(), quit
}

// drain takes what the engine printed on the app's writers since the last drain.
func (h *tuiHost) drain() []string {
	h.mu.Lock()
	s := h.out.String()
	h.out.Reset()
	h.mu.Unlock()
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

var builtinCommands = []tui.MenuItem{
	{Name: "help", Description: "commands and shortcuts"},
	{Name: "dash", Description: "open the dashboard in the browser — the same session, live: console, neural net, metrics"},
	{Name: "board", Description: "the board, full screen: agents, the ontology graph, roles, usage"},
	{Name: "agents", Description: "the live subagent: every agent, its state, context and spend"},
	{Name: "send", Description: "/send <agent> <text> — a line for a running agent"},
	{Name: "usage", Description: "the usage journal of this session"},
	{Name: "status", Description: "the honesty rule and the instrument board"},
	{Name: "config", Description: "config [explain] — the switchboard and where each value came from"},
	{Name: "compact", Description: "drain the context now"},
	{Name: "review", Description: "/review [range] — two reviewers on two models, one merged shortlist; nothing fixed before you approve"},
	{Name: "handoff", Description: "/handoff [focus] writes one for a fresh session · /handoff read picks the last up"},
	{Name: "sessions", Description: "the sessions of this workspace"},
	{Name: "resume", Description: "/resume <id> — reopen a session"},
	{Name: "theme", Description: "/theme <name> [dark|light|system] — the palette the terminal and the dashboard hold; alone, the picker"},
	{Name: "copy", Description: "copy the last answer's markdown to the clipboard"},
	{Name: "mouse", Description: "hand the mouse to the terminal (its own selection) or back to the panes"},
	{Name: "clear", Description: "clear the screen"},
	{Name: "quit", Description: "end the session"},
}

func (h *tuiHost) Commands() []tui.MenuItem {
	a := h.a
	items := append([]tui.MenuItem(nil), builtinCommands...)
	for _, c := range a.Found.Commands {
		d := c.Description
		if d == "" {
			d = firstLine(strings.TrimSpace(c.Template))
		}
		if c.Source != "" {
			d += " (" + c.Source + ")"
		}
		items = append(items, tui.MenuItem{Name: c.Name, Description: strings.TrimSpace(d)})
	}
	if a.MCP != nil {
		for _, p := range a.MCP.Prompts {
			items = append(items, tui.MenuItem{Name: p.Server + ":" + p.Name, Description: orStr(p.Description, "an MCP prompt")})
		}
	}
	return items
}

// Complete lists workspace paths starting with prefix (the listing is cached for a few seconds).
func (h *tuiHost) Complete(prefix string) []string {
	h.mu.Lock()
	if h.files == nil || time.Since(h.filesAt) > 5*time.Second {
		h.files = h.listFiles()
		h.filesAt = time.Now()
	}
	files := h.files
	h.mu.Unlock()
	var out []string
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			out = append(out, f)
			if len(out) >= 50 {
				break
			}
		}
	}
	if len(out) == 0 {
		lp := strings.ToLower(prefix)
		for _, f := range files {
			if strings.Contains(strings.ToLower(f), lp) {
				out = append(out, f)
				if len(out) >= 50 {
					break
				}
			}
		}
	}
	return out
}

func (h *tuiHost) listFiles() []string {
	root := h.a.Root
	var out []string
	n := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if p != root && (name == ".git" || name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".cache")) {
				return filepath.SkipDir
			}
			return nil
		}
		n++
		if n > 20000 {
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out
}

// --- turns ---

func (h *tuiHost) start(text string, auto bool) {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		// busy: hold the line in the inbox. The running turn finishes and its drain
		// starts this as the next turn — never injected mid-turn (FIFO, B).
		h.a.Subagent.Enqueue(workspace.Orchestrator, text)
		return
	}
	// the resume route (freeze review, card spine): the human's typed word after a
	// suspension revives the parked run's own transcript, at the parked step — never a
	// fresh turn that forgot the question; only the live-thread route clears the park
	ps := h.pendingSuspend
	if !auto && ps != nil {
		h.pendingSuspend = nil
	}
	tctx, cancel := context.WithCancel(h.ctx)
	h.turnCtx, h.cancel, h.running, h.streamed, h.turnStart = tctx, cancel, true, false, time.Now()
	h.pulse = h.turnStart // a fresh turn is a fresh breath — the watchdog ages from here
	h.mu.Unlock()
	h.watchdog(tctx)
	h.a.webBus().note(map[string]any{"type": "ask", "text": text})
	h.turns.Add(1)
	go func() {
		defer h.turns.Done()
		// a panic anywhere in the turn becomes a failed turn, never a dead process:
		// running clears, the harness lives (Session.Turn recovers first and journals
		// the stack; this is the outermost backstop)
		defer func() {
			if rec := recover(); rec != nil {
				hole := fmt.Sprintf("panic: %v — the harness survived; the turn did not", rec)
				h.finish(&loop.Result{Status: loop.Fail, Holes: []string{hole}}, false)
			}
		}()
		var r *loop.Result
		var err error
		if ps != nil {
			ask := "the human answered your parked card " + ps.CardID + ": " + text + " — continue from where the run stopped"
			var s2 *loop.Session
			r, s2, err = h.session().Engine.ResumeWithSession(tctx, ps.RunID, ask)
			if err == nil && s2 != nil {
				h.mu.Lock()
				h.s = s2 // the resumed session owns the thread now
				h.mu.Unlock()
			}
		} else {
			if auto {
				h.send(tui.EvTurnStart{Text: text, Auto: true})
			}
			r, err = h.session().Turn(tctx, text)
		}
		if err != nil {
			r = &loop.Result{Status: loop.Fail, Holes: []string{err.Error()}}
		}
		h.finish(r, tctx.Err() != nil && h.ctx.Err() == nil)
	}()
}

func (h *tuiHost) finish(r *loop.Result, interrupted bool) {
	a := h.a
	h.mu.Lock()
	cancel := h.cancel
	streamed := h.streamed
	reason := h.denyReason
	h.denyReason = ""
	h.cancel, h.turnCtx, h.running = nil, nil, false
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	sess := h.session()
	a.Sessions.Sync(a.SessionID, sess, a.mountModel.Ref.Model)
	if r.Verdict != "" {
		a.record(sess, "verdict", map[string]interface{}{"verdict": r.Verdict, "wrote": sess.Wrote})
	}
	ev := tui.EvTurnDone{Status: r.Status, Text: r.Text, Streamed: streamed, Verdict: r.Verdict, Interrupted: interrupted}
	if r.Verdict != "" {
		ev.LogRel = filepath.ToSlash(filepath.Join(a.Cfg.Dist.WorkspaceDir, "log.md"))
	}
	for _, hole := range r.Holes {
		if interrupted && strings.Contains(hole, "context canceled") {
			continue
		}
		ev.Holes = append(ev.Holes, hole)
	}
	if r.Status == loop.Checkpoint {
		ev.Hint = fmt.Sprintf("the session hit a budget — a good point to stop; resume with: %s resume %s", a.Cfg.Dist.Name, a.SessionID)
	}
	if r.Status == loop.Suspended {
		h.mu.Lock()
		ps := h.pendingSuspend
		h.mu.Unlock()
		if ps != nil {
			ev.Hint = "the card " + ps.CardID + " is parked on the instruments shelf — type your answer and this run resumes at its parked step (or /resume " + ps.RunID + " by hand)"
		}
	}
	if lines := h.drain(); len(lines) > 0 {
		h.send(tui.EvLines{Lines: lines})
	}
	h.planOutcomeSweep(string(r.Status))
	h.send(ev)
	if h.ctx.Err() != nil {
		return
	}
	// a suspended turn owns the next act: the parked card is the open question — nothing
	// typed meanwhile chains behind it; the human's word resumes the parked run itself
	h.mu.Lock()
	suspended := h.pendingSuspend != nil || h.suspending
	h.suspending = false
	h.mu.Unlock()
	if r.Status == loop.Suspended || suspended {
		return
	}
	// the queue, first: both lanes (mid-turn leftovers, next-turn FIFO) plus wake reports
	// fold into whatever turn opens next — a denial with a reason no longer strands voice
	pending := append(a.Subagent.DrainAll(workspace.Orchestrator), a.Subagent.TakeWake()...)
	// the relay hold: the held @$ orders are this wake's to claim, their prompts first
	pending = append(h.claimHeld(), pending...)
	// a denial with a reason opens the next turn with the human's words (the policy: a denial
	// ends the turn; the reason reaches the model as the human speaking)
	if r.Status == loop.Denied && reason != "" {
		text := reason
		if len(pending) > 0 {
			text = strings.Join(append([]string{reason}, pending...), "\n\n")
		}
		h.send(tui.EvTurnStart{Text: reason})
		h.start(text, false)
		return
	}
	if len(pending) > 0 {
		text := strings.Join(pending, "\n\n")
		h.send(tui.EvTurnStart{Text: text})
		// Fix B (R2 ctx-ancestry): the old turn's ctx is cancelled and finish nulled h.cancel;
		// if left alone the next start() would WithCancel a dead parent. Re-register h.ctx from a
		// live root so context.WithCancel(h.ctx) in start() builds the next turn on a live branch.
		// Never under an open card (freeze-2026-10-07 policy): a swap mid-wait would orphan its select.
		h.mu.Lock()
		h.cancel = nil
		if h.choiceOpen == 0 {
			h.ctx = context.Background()
		}
		h.mu.Unlock()
		h.start(text, true)
	}
}

// planOutcomeSweep closes the scenario funnel's loop at a turn's end (scenario-3-2-1 policy):
// every plan deck updated within the running turn rides that turn's verdict out as one
// plan-outcome row on the bus tap — agent, cards count, the chosen slice, its evidence.
func (h *tuiHost) planOutcomeSweep(verdict string) {
	dir := h.a.planDir()
	h.mu.Lock()
	turnStart := h.turnStart
	h.mu.Unlock()
	if turnStart.IsZero() {
		return
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, p := range files {
		d := planRead(dir, strings.TrimSuffix(filepath.Base(p), ".json"))
		if d.Agent == "" {
			continue
		}
		if at, ok := planUpdatedAt(d); !ok || at.Before(turnStart.Truncate(time.Second)) {
			continue
		}
		chosen, why := "", ""
		for _, it := range d.Items {
			if it.Status == "in_progress" {
				chosen = it.ID
				why = it.Why
			}
		}
		h.send(tui.EvPlanOutcome{Agent: d.Agent, Verdict: verdict, Cards: len(d.Items), Chosen: chosen, Why: why})
	}
}

func (h *tuiHost) woke() {
	h.mu.Lock()
	running := h.running
	h.mu.Unlock()
	if running {
		for _, rep := range h.a.Subagent.TakeWake() {
			_ = h.a.Subagent.Send(workspace.Orchestrator, rep)
		}
		return
	}
	if reports := h.a.Subagent.TakeWake(); len(reports) > 0 {
		h.start(strings.Join(append(h.claimHeld(), reports...), "\n\n"), true)
	}
}

// claimHeld is the wake's claim on the held queue rows; the footer's held count follows it.
func (h *tuiHost) claimHeld() []string {
	lines := h.a.heldWakeLines()
	h.send(tui.EvHeld{N: h.a.heldCount()})
	return lines
}

// --- the loop's events ---

var diffTools = map[string]bool{"write": true, "edit": true, "multiedit": true, "patch": true, "str_replace_based_edit_tool": true}

func (h *tuiHost) observe(s *loop.Session, st *loop.StepRecord, phase string) {
	agent := agentName(s)
	// a dispatch step seeds and lands its Subagent's card at EVERY depth: the start names the
	// Subagent's ask; the end carries its wire report. A Subagent dispatched by a Subagent never gets
	// a card close anywhere else — its own session's steps reach this host only as its
	// parent's steps — so without this the card sat in the live area forever (the zombie
	// card Operator saw: DONE on its face, the clock still ticking under it).
	if st.Tool == "dispatch" {
		switch phase {
		case "start":
			var in struct{ Agent, Ask string }
			_ = json.Unmarshal(st.Input, &in)
			h.mu.Lock()
			h.subagentAsk[strings.ToLower(in.Agent)] = in.Ask
			h.mu.Unlock()
		case "end":
			if st.Status == "done" || st.Status == "failed" {
				if rep, ok := wire.ParseReport(st.Result.Output); ok {
					var in struct{ Agent string }
					_ = json.Unmarshal(st.Input, &in)
					h.subagentReport(strings.ToLower(in.Agent), agent, rep, st.Result.Err, st.Result.Output)
					if agent == workspace.Orchestrator {
						// the session's dispatch step has no tool block of its own: the Subagent's card is it
						return
					}
				}
			}
		}
	}
	if agent != workspace.Orchestrator {
		if phase == "start" {
			h.send(tui.EvState{Agent: agent, State: "tool", Tool: st.Tool})
		}
		// the Subagent's own log, for the agent view
		h.send(tui.EvAgentStep{Agent: agent, Tool: h.toolView(st), End: phase == "end"})
		return
	}
	switch phase {
	case "start":
		if diffTools[st.Tool] {
			if p := stepPath(st.Input); p != "" {
				h.mu.Lock()
				h.before[st.ID] = readCapped(filepath.Join(h.a.Root, p))
				h.mu.Unlock()
			}
		}
		if st.Tool == "dispatch" || st.Tool == "ask" {
			// a Subagent has its own block; a question is its own block
			return
		}
		h.send(tui.EvToolStart{Tool: h.toolView(st)})
	case "end":
		if st.Tool == "ask" && st.Status == "done" {
			return
		}
		v := h.toolView(st)
		if diffTools[st.Tool] && st.Status == "done" {
			h.mu.Lock()
			before, had := h.before[st.ID]
			delete(h.before, st.ID)
			h.mu.Unlock()
			if p := stepPath(st.Input); p != "" && had {
				d := tui.DiffText(before, readCapped(filepath.Join(h.a.Root, p)), 2)
				d.Path = p
				v.Diff = &d
			}
		}
		h.send(tui.EvToolEnd{Tool: v})
	}
}

func (h *tuiHost) toolView(st *loop.StepRecord) tui.ToolView {
	v := tui.ToolView{ID: st.ID, Name: st.Tool, Summary: stepSummary(st.Input), Class: st.Effective, Status: st.Status, Ms: st.Ms, Output: st.Result.Output, Wrote: st.Wrote, Link: fileLink(h.a.Root, st.Input)}
	if st.Tool == "str_replace_based_edit_tool" {
		v.Name = "edit"
	}
	switch st.Status {
	case "denied":
		v.Why = st.Gate.Why
	case "refused", "would-ask":
		v.Why = st.Result.Output
	case "pending":
		v.Status, v.Why = "refused", st.Result.Output
	case "failed":
		if st.Tool == "" || st.Effective == "" {
			v.Class = ""
		}
	}
	return v
}

func (h *tuiHost) state(agent, state string) {
	if agent == workspace.Orchestrator {
		h.send(tui.EvState{Agent: agent, State: state})
		return
	}
	h.mu.Lock()
	seen := h.subagentSeen[agent]
	ask := h.subagentAsk[agent]
	if !seen && state == "done" {
		// terminal state for an agent whose banner already closed (subagentReport landed, or it was
		// never opened): no zombie banner, no second tombstone.
		h.mu.Unlock()
		return
	}
	if !seen {
		h.subagentSeen[agent] = true
		h.subagentStart[agent] = time.Now()
	}
	started, startedOK := h.subagentStart[agent]
	if state == "done" {
		delete(h.subagentSeen, agent)
		delete(h.subagentStart, agent)
	}
	h.mu.Unlock()
	if !seen {
		v := tui.SubagentView{Name: agent, State: state, Ask: ask, Word: h.words.Subagent}
		for _, b := range h.a.Subagent.Agents() {
			if b.Name == agent {
				v.Rank, v.Role, v.Model = b.Rank, b.Role, b.Model
			}
		}
		h.send(tui.EvSubagent{Subagent: v})
		return
	}
	if state == "done" {
		// a @$-drained agent's report never rides the dispatcher's step end, so no subagentReport
		// lands for it: its banner must close here, at the loop's terminal state, once.
		v := tui.SubagentView{Name: agent, State: "done", Ask: ask, Word: h.words.Subagent}
		if startedOK && !started.IsZero() {
			v.Elapsed = time.Since(started)
		}
		for _, b := range h.a.Subagent.Agents() {
			if b.Name == agent {
				v.Rank, v.Role, v.Model = b.Rank, b.Role, b.Model
				// leg 2a: the card face carries the exit verdict the Subagent recorded at Finish
				// (the run journal's t:end word), never the lifecycle alone.
				v.Exit = b.Exit
				v.Failed = b.Failed
			}
		}
		h.send(tui.EvSubagent{Subagent: v})
		return
	}
	h.send(tui.EvState{Agent: agent, State: state})
}

// subagentReport lands a Subagent's report as its block (foreground dispatch); up is the agent that
// dispatched it, the one hop up a loud verdict is addressed to.
func (h *tuiHost) subagentReport(agent, up string, rep wire.Report, failed bool, raw string) {
	h.mu.Lock()
	started := h.subagentStart[agent]
	delete(h.subagentSeen, agent)
	delete(h.subagentStart, agent)
	h.mu.Unlock()
	v := tui.SubagentView{Name: agent, State: "done", Exit: exitVerdict(rep), Failed: failed, Word: h.words.Subagent, Report: reportView(rep), Up: up}
	if !started.IsZero() {
		v.Elapsed = time.Since(started)
	}
	for _, b := range h.a.Subagent.Agents() {
		if b.Name == agent {
			v.Rank, v.Role, v.Model = b.Rank, b.Role, b.Model
		}
	}
	h.send(tui.EvSubagent{Subagent: v})
}

// subagentFinished is a background Subagent's report landing.
func (h *tuiHost) subagentFinished(name, report string) {
	if report == "" {
		return
	}
	h.send(tui.EvHeld{N: h.a.heldCount()}) // its @$ orders are held now, until the wake
	rep, ok := wire.ParseReport(report)
	if !ok {
		rep = wire.Report{Status: "DONE", Other: strings.Split(strings.TrimSpace(report), "\n")}
	}
	h.subagentReport(name, "", rep, strings.HasPrefix(rep.Status, "FAIL"), report)
}

func reportView(rep wire.Report) *tui.ReportView {
	v := &tui.ReportView{Status: rep.Status, Findings: rep.Findings, Verdicts: rep.Verdicts, Holes: rep.Holes, Other: rep.Other}
	for _, u := range rep.Unsaid {
		v.Unsaid = append(v.Unsaid, u.Kind+": "+u.Text)
	}
	return v
}

// exitVerdict is the run journal's exit word riding a wire report's @S line: the loop stamps
// a non-DONE exit as "FAIL — PASSED" (the agent's own self-judged status after the dash), so
// the first token is the verdict. "" when the report carried no @S line (a working-notes-level DONE
// fallback wrote it) or it names no known verdict.
func exitVerdict(rep wire.Report) string {
	if rep.Status == "" {
		return ""
	}
	tok := strings.ToUpper(strings.Fields(rep.Status)[0])
	switch tok {
	case "DONE":
		return "PASSED" // a green journal face reads PASSED — DONE is the lifecycle word, never a verdict
	case "FAIL", "ESCALATE", "DENIED", "INTERRUPTED":
		return tok
	}
	return ""
}

// --- the gate and the ask tool ---

func (h *tuiHost) approve(r gate.Request) (gate.Answer, bool) {
	a := h.a
	match := ruleMatch(r, tool.Env{Root: a.Root, WorkspaceDir: a.Cfg.Dist.WorkspaceDir})
	why := r.Why
	switch r.Class {
	case tool.Outward:
		why = "reaches beyond the " + h.words.Workspace + " (" + r.Why + ") — nothing leaves without your word"
	case tool.Destructive:
		why = "irreversible (" + r.Why + ") — no destruction without your consent"
	}
	view := tui.ChoiceView{Title: "Approval", Class: r.Class.String(), Tool: r.Tool, Summary: r.Summary, Why: why,
		Options: []string{"Yes", "Yes, and don't ask again for " + match, "No, and tell the model why"},
		Notes:   []string{"", "writes a rule to " + h.words.ConfigRel, ""},
		Reasons: []string{"", "", "why not?"}}
	if r.Agent != "" && r.Agent != workspace.Orchestrator {
		view.Agent = r.Agent
	}
	reply := make(chan tui.ChoiceAnswer, 1)
	h.mu.Lock()
	h.choiceOpen++
	turnCtx := h.turnCtx // the running turn's ctx: esc/interrupt must be able to free this wait
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.choiceOpen--
		h.mu.Unlock()
	}()
	h.send(tui.EvChoice{View: view, Reply: reply})
	var ans tui.ChoiceAnswer
	select {
	case ans = <-reply:
	case <-turnCtxDone(turnCtx):
		// the turn ctx fired while the card waited. Two voices can lower the hand: the
		// human's (interrupt — a lawful denial) or the watchdog's (suspend layer — the workspace
		// parks the card durably and the thread waits for the word to resume it).
		h.mu.Lock()
		suspending := h.suspending
		h.mu.Unlock()
		if suspending {
			cardPath := h.parkCard(r, view)
			h.send(tui.EvChoiceAnswered{Class: r.Class.String(), Tool: r.Tool, Answer: "parked: suspended at the watchdog layer"})
			return gate.Answer{Decision: gate.Suspended, By: "watchdog", Why: "card parked (" + cardPath + ") — the human's word revives this run"}, true
		}
		h.send(tui.EvChoiceAnswered{Class: r.Class.String(), Tool: r.Tool, Answer: "aborted: turn interrupted"})
		return gate.Answer{Decision: gate.Denied, By: "tui", Why: "interrupted while waiting for " + h.words.Human + "'s word"}, true
	case <-h.ctx.Done():
		return gate.Answer{Decision: gate.Denied, By: "tui", Why: "the session ended before an answer"}, true
	}
	// the choice ledger: what was asked, class-wise and tool-wise, rides the session store — the
	// evidence a review lane graduates rules from; never a live nudge (the gate stays silent).
	h.send(tui.EvChoiceAnswered{Class: r.Class.String(), Tool: r.Tool, Answer: choiceAnswerText(view.Options, ans), Recommended: view.Recommended})
	switch {
	case ans.Aborted:
		return gate.Answer{Decision: gate.Denied, By: "tui", Why: "the session ended before an answer"}, true
	case ans.Index == 0:
		return gate.Answer{Decision: gate.Approved, By: "tty", Why: r.Why}, true
	case ans.Index == 1:
		where, err := h.addRule(match)
		if err != nil {
			h.send(tui.EvError{Text: "the rule was not written: " + err.Error()})
			return gate.Answer{Decision: gate.Approved, By: "tty", Why: r.Why}, true
		}
		h.send(tui.EvNotice{Text: "rule " + match + " → allow · in " + where})
		return gate.Answer{Decision: gate.Approved, By: "tty", Why: r.Why + "; rule " + match + " written"}, true
	}
	reason := strings.TrimSpace(ans.Text)
	h.mu.Lock()
	h.denyReason = reason
	h.mu.Unlock()
	if reason == "" {
		return gate.Answer{Decision: gate.Denied, By: "tty", Why: h.words.Human + " said no"}, true
	}
	return gate.Answer{Decision: gate.Denied, By: "tty", Why: h.words.Human + " said: " + reason}, true
}

// ruleMatch is the permission rule proposed for "don't ask again": the tool and a glob on
// the act's subject — the command's first two words for bash and git (`bash:git push*`), the
// host for a URL, the subject itself otherwise.
func ruleMatch(r gate.Request, env tool.Env) string {
	subject := subjectOf(r.Tool, r.Input, env)
	if subject == "" {
		subject = strings.TrimSpace(r.Summary)
	}
	switch r.Tool {
	case "bash", "git":
		words := strings.Fields(subject)
		if len(words) == 0 {
			return r.Tool + ":*"
		}
		n := 1
		if len(words) > 1 && !strings.HasPrefix(words[1], "-") {
			n = 2
		}
		return r.Tool + ":" + strings.Join(words[:n], " ") + "*"
	case "webfetch":
		if i := strings.Index(subject, "://"); i > 0 {
			rest := subject[i+3:]
			host, _, _ := strings.Cut(rest, "/")
			return r.Tool + ":" + subject[:i+3] + host + "/*"
		}
	}
	return r.Tool + ":" + subject
}

// addRule appends an allow rule to the local config layer and reloads the config live, so the
// next step already reads it. The local file is the operator's machine-local word: gitignored,
// never the project's.
func (h *tuiHost) addRule(match string) (string, error) {
	a := h.a
	file := filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, "config.local.yaml")
	for _, l := range a.Cfg.Layers {
		if l.Name == "local" && l.Present && l.Path != "" {
			file = l.Path
		}
	}
	var items []string
	if b, err := os.ReadFile(file); err == nil {
		if n, err := yaml.Parse(b, file); err == nil && n != nil && n.Kind == yaml.Map {
			if p := n.Get("permissions"); p != nil {
				if rs := p.Get("rules"); rs != nil && rs.Kind == yaml.List {
					for _, it := range rs.Items {
						if it.Kind != yaml.Map {
							continue
						}
						m, act := it.Get("match"), it.Get("action")
						if m == nil && it.Get("tool") != nil {
							m = &yaml.Node{Kind: yaml.String, Str: it.Get("tool").Text() + ":" + it.Get("pattern").Text()}
						}
						if m == nil || act == nil {
							continue
						}
						if m.Text() == match {
							items = nil
							break
						}
						items = append(items, fmt.Sprintf("{match: %s, action: %s}", yamlQuote(m.Text()), act.Text()))
					}
				}
			}
		}
	}
	items = append(items, fmt.Sprintf("{match: %s, action: allow}", yamlQuote(match)))
	op := config.Op{Path: "permissions.rules", Value: "[" + strings.Join(items, ", ") + "]"}
	if err := a.Cfg.ApplyPatch(file, []config.Op{op}); err != nil {
		return "", err
	}
	next, changes, err := a.Cfg.Reload()
	if err != nil {
		return "", fmt.Errorf("written, but the reload failed: %w", err)
	}
	h.mu.Lock()
	h.quiet = true
	h.mu.Unlock()
	a.reload(next, changes)
	h.mu.Lock()
	h.quiet = false
	h.mu.Unlock()
	a.Missing.SetConfig(next)
	// The pairing hook (Docs-as-code / pairing-is-a-class): any auto-growing file under a
	// member's ownership moves its tending doc in the same write. config.local.yaml is
	// instance one — growth by hand or by loop from a permissions rule is the same event,
	// and the owner's doc carries a dated Notes line so the gate and the human's memory
	// stand together. Failure to pair is a hole, never a refusal: the rule stands.
	if werr := a.Workspace.Reload(); werr != nil {
		a.hole("pairing: workspace reload after rule " + match + ": " + werr.Error())
	} else if own := a.Workspace.Owner(relOrAbs(a.Root, file)); own != nil && own.Doc != "" {
		pairIfDoc(a.Root, own.Doc, match)
	}
	return relOrAbs(a.Root, file), nil
}

// pairIfDoc appends a dated pairing line to the owning member's doc under ## Working notes
// in the same write wave as the tend target, so Docs-as-code sees the paired move.
func pairIfDoc(root, docRel, match string) {
	docPath := filepath.Join(root, filepath.FromSlash(docRel))
	b, err := os.ReadFile(docPath)
	if err != nil {
		return
	}
	dt := time.Now().UTC().Format("2006-01-02")
	line := "- " + dt + " — paired: `" + match + "` grew config.local.yaml; this doc moved in the same write (Docs-as-code, pairing-is-a-class)."
	t := string(b)
	if !strings.HasSuffix(t, "\n") {
		t += "\n"
	}
	// append under the "## Working notes" heading if present, else append after the doc's last line
	if idx := strings.Index(t, "\n## Working notes\n"); idx >= 0 {
		rest := t[idx+len("\n## Working notes\n"):]
		head := t[:idx+len("\n## Working notes\n")]
		lines := strings.Split(strings.TrimSuffix(rest, "\n"), "\n")
		out := head + line + "\n" + strings.Join(lines, "\n") + "\n"
		_ = os.WriteFile(docPath, []byte(out), 0o644)
	}
}

func yamlQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// turnCtxDone is the Done channel of the running turn's ctx; nil ctx (a gate asked outside
// a turn) yields a never-firing channel — the old wait semantics, unchanged.
func turnCtxDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

// choiceAnswerText reduces the human's pick to one reviewable token for the ledger.
func choiceAnswerText(opts []string, a tui.ChoiceAnswer) string {
	switch {
	case a.Aborted:
		return "aborted"
	case a.Index >= 0 && a.Index < len(opts):
		return opts[a.Index]
	case strings.TrimSpace(a.Text) != "":
		return "text: " + oneLineOf(a.Text)
	default:
		return "none"
	}
}

// ask answers the ask tool (and a Provisioning proposal) with a question block.
func (h *tuiHost) ask(ctx context.Context, q tool.Question) (string, error) {
	view := tui.ChoiceView{Title: "Question", Why: q.Text, Options: q.Options, Free: q.Free || len(q.Options) == 0}
	if q.Recommended >= 1 && q.Recommended <= len(q.Options) {
		view.Recommended = q.Options[q.Recommended-1] // the asker's lawful advice, rendered and pre-seated — never auto-picked
	}
	reply := make(chan tui.ChoiceAnswer, 1)
	h.mu.Lock()
	h.choiceOpen++
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.choiceOpen--
		h.mu.Unlock()
	}()
	h.send(tui.EvChoice{View: view, Reply: reply})
	select {
	case ans := <-reply:
		switch {
		case ans.Aborted:
			return "", fmt.Errorf("no answer: the session ended")
		}
		h.send(tui.EvChoiceAnswered{Tool: "ask", Answer: choiceAnswerText(q.Options, ans), Recommended: view.Recommended})
		if ans.Index >= 0 && ans.Index < len(q.Options) {
			return q.Options[ans.Index], nil
		}
		return ans.Text, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-h.ctx.Done():
		return "", fmt.Errorf("no answer: the session ended")
	}
}

// --- helpers ---

func stepSummary(in json.RawMessage) string {
	var m map[string]interface{}
	if json.Unmarshal(in, &m) != nil {
		return oneLineText(string(in))
	}
	for _, k := range []string{"command", "path", "pattern", "url", "query", "agent", "question", "name"} {
		if v, ok := m[k].(string); ok && v != "" {
			if k == "path" {
				if p, ok := m["pattern"].(string); ok {
					v = p + " in " + v
				}
			}
			if k == "agent" {
				if ask, ok := m["ask"].(string); ok {
					v += " — " + ask
				}
			}
			return oneLineText(v)
		}
	}
	if args, ok := m["args"].([]interface{}); ok {
		var parts []string
		for _, a := range args {
			parts = append(parts, fmt.Sprint(a))
		}
		return oneLineText(strings.Join(parts, " "))
	}
	return oneLineText(string(in))
}

func stepPath(in json.RawMessage) string {
	var m struct{ Path string }
	_ = json.Unmarshal(in, &m)
	return strings.TrimSpace(m.Path)
}

func oneLineText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

const diffCap = 512 << 10

func readCapped(p string) string {
	b, err := os.ReadFile(p)
	if err != nil || len(b) > diffCap || bytes.IndexByte(b, 0) >= 0 {
		return ""
	}
	return string(b)
}

func tilde(p, home string) string {
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// fileLink is a file:// URL for a step whose input names a path inside the workspace; "" otherwise.
func fileLink(root string, raw json.RawMessage) string {
	var input struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(raw, &input)
	p := input.Path
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	p = filepath.Clean(p)
	if rel, err := filepath.Rel(root, p); err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// session is the session the host drives, read under mu: a resumed run swaps it from the turn's
// goroutine while the terminal's goroutine draws the footer from it.
func (h *tuiHost) session() *loop.Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.s
}
