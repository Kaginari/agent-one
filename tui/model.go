package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Host is what the program asks of the app. Every call is made from the program's goroutine;
// a slow one (a slash command) runs as a tea.Cmd.
type Host interface {
	Welcome() Welcome
	Footer() FooterView
	// Submit starts a turn; a non-empty string refuses it (a userPrompt hook) and is shown.
	Submit(text string) string
	// Queue hands a line typed mid-turn to the running agent; the string is the notice shown.
	Queue(text string) string
	// Interrupt cancels the running turn.
	Interrupt()
	// Slash runs a command the UI does not handle itself; lines are printed, quit ends the session.
	Slash(line string) (lines []string, quit bool)
	// Pipe hands a line typed at an agent in the agents pane to that agent's inbox; the string
	// is the notice shown. A done agent refuses: its context died with its task.
	Pipe(agent, text string) string
	// Commands lists the slash commands for the menu (built-in and discovered).
	Commands() []MenuItem
	// Complete lists workspace paths for an @prefix.
	Complete(prefix string) []string
	// Board is what /board draws; rng is the usage range (BoardRanges).
	Board(rng string) BoardView
	// Plans lists the plan decks agents have penned, in name order; the rail shows
	// the first one by default ("coordinator" first when present) and ctrl+p cycles.
	Plans() []RailPlan
}

// Model is the Bubble Tea model of the session: the live area at the bottom (stream, running
// tools, subagents, a choice, the spinner, the input, the menu, the footer); everything finished
// is printed above it with tea.Println and stays in the terminal's scrollback.
type Model struct {
	host  Host
	theme Theme
	words Words

	width, height int
	ready         bool

	// the turn
	busy          bool
	turnStart     time.Time
	verb          string
	stream        strings.Builder
	streamed      bool
	tools         []*ToolView // running tools, in order
	subagents     map[string]*SubagentView
	subagentSeq   []string
	pulses        map[string]*subagentPulse // a live Subagent row's motion, keyed by its name (motion.go)
	files         []RailFile                // the touch ledger: who wrote what, in order, latest last (slice-2 rail)
	plans         []RailPlan                // plan decks the rail shows; planIdx picks the visible one (ctrl+p)
	planIdx       int
	planSticky    bool            // the human ctrl+p'd off the coordinator default: keep their pick
	subagentRows  map[int]string  // rail row → subagent name; a click there switches the plan card
	railCardRows  map[string]int  // rail card key ("subagents" | "plan" | "files") → its header row; a click there toggles its collapse
	railScroll    int             // the rail's scroll offset in rows: the wheel over the rail column walks it
	railCollapsed map[string]bool // rail card key ("subagents" | "plan" | "files") → collapsed to its header
	lastBlock     *collapsed      // the last collapsed block, for ctrl+o

	// the choice
	choice   *EvChoice
	view     ChoiceView
	choiceAt time.Time // when the card opened: the enter-drop's clock
	choiceW  int       // the modal's pinned width — the box may only GROW while open, never reflow the frame as the cursor moves
	choiceH  int       // the modal's pinned height, same policy

	// the input
	input     textarea.Model
	history   []string
	histIdx   int
	draft     string
	pasted    map[string]string // placeholder → text
	menu      *menuState
	comp      *compState
	shortcuts bool
	queued    int
	lastCtrlC time.Time
	message   string
	msgUntil  time.Time

	spin   spinner.Model
	footer FooterView
	quit   bool
	err    error

	// the slice-4 verdict burst: armed by a turn-end gate verdict after Subagents ran,
	// self-expiring after burstTicks
	burstAt   time.Time
	burstGood bool
	burstOn   bool

	// prints is the FIFO of finished blocks; one goroutine hands them to the program in order
	// (a tea.Println per Update would race the next Update's).
	prints chan printItem
	// blocks is every finished block as a render at a width: a width change clears the terminal
	// and prints them again at the new one, as the terminal's own rewrap cannot be trusted.
	blocks    []func(w int) string
	reflowSeq int
	lastWidth int // the width the transcript was last printed at
	bgKnown   bool
	welcomed  bool
	intro     *introState // the startup animation; nil when off or over (retired at launch: the zone worker lives in the sidebar now)
	shimmer   int         // the spinner verb's highlight, advanced with the spinner
	railTick  int         // the sidebar's animation counter: the zone worker blinks on it (closed one tick in twelve)
	blinkOn   bool
	palette   *paletteState
	// the cast (cast.go): the session's thinking as it arrives, every agent's log, the agent view
	think      strings.Builder
	thinkStart time.Time
	state      string // the session's state, for its verb and icon
	verbSeed   int
	logs       map[string]*bodyLog
	logOrder   []string
	bview      *bodyView
	toasts     []toast

	// the pane-mode chat cache: blocks rendered at cw, rebuilt when dirtied (a new block, a
	// width change, a theme swap) — never re-rendered per frame
	paneDirty bool
	paneCache []string
	paneW     int
	paneRef   int
	// the chat pane's viewport: transcript scrollback lives here while the panes own the
	// window (pane_vp.go)
	vp   viewport.Model
	vpOK bool
	vpCW int
	vpH  int
	// printedIdx: blocks[:printedIdx] own the terminal scrollback already; under panes nothing
	// prints live, so when the window narrows the panes hand the scrollback what it missed —
	// every block exactly once, never twice
	printedIdx int
	// the board: full screen while open; blocks that finish meanwhile wait in held
	board    *boardState
	held     []string
	sender   func(tea.Msg)
	attached chan struct{}
}

type collapsed struct {
	tool     *ToolView
	subagent *SubagentView
	thought  *thought
}

// evSnapshot is the tests' frame reader: rendered on the program goroutine (see the handler).
type evSnapshot struct{ c chan string }

type menuState struct {
	items  []MenuItem
	cursor int
}

type compState struct {
	prefix string // the @token being completed, without the @
	items  []string
	cursor int
}

// New builds the model.
func New(host Host, theme Theme, words Words) *Model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetPromptFunc(2, func(p textarea.PromptInfo) string {
		if p.LineNumber == 0 {
			return "> "
		}
		return "  "
	})
	ta.Placeholder = "ask, or / for commands"
	ta.CharLimit = 0
	ta.MaxHeight = 8
	ta.SetHeight(1)
	ta.KeyMap.InsertNewline.SetEnabled(false)
	st := ta.Styles()
	st.Focused.CursorLine = lipgloss.NewStyle()
	st.Focused.Base = lipgloss.NewStyle()
	st.Focused.Prompt = theme.dim
	st.Focused.Placeholder = theme.dim
	st.Blurred = st.Focused
	ta.SetStyles(st)
	ta.Focus()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.accent))
	m := &Model{host: host, theme: theme, words: words, input: ta, spin: sp, subagents: map[string]*SubagentView{}, pulses: map[string]*subagentPulse{}, pasted: map[string]string{}, width: 80, height: 24,
		prints: make(chan printItem, 4096), attached: make(chan struct{})}
	m.footer = host.Footer()
	m.histIdx = -1
	return m
}

// Attach gives the model the program's Send, which the printer needs; Start and the test
// harness call it once the program exists.
func (m *Model) Attach(send func(tea.Msg)) {
	m.sender = send
	close(m.attached)
}

// Init starts the ticks and the printer and asks the terminal for its background; the welcome
// waits for the terminal's width, and briefly for that answer, so it is drawn in the right theme.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, tick(), m.printer}
	if m.intro != nil {
		cmds = append(cmds, introTick())
	}
	if !m.theme.Forced {
		cmds = append(cmds, tea.RequestBackgroundColor, tea.Tick(bgWait, func(time.Time) tea.Msg { return evBgWait{} }))
	} else {
		m.bgKnown = true
	}
	return tea.Batch(cmds...)
}

// bgWait bounds how long the welcome waits for the terminal's background answer; a terminal
// that never answers keeps the dark theme.
const bgWait = 150 * time.Millisecond

type evBgWait struct{}

// welcome prints the welcome once the width is known and the theme settled.
func (m *Model) welcome() tea.Cmd {
	if m.welcomed || !m.ready || !m.bgKnown || m.intro != nil {
		return nil
	}
	m.welcomed = true
	wel := m.host.Welcome()
	return m.print(func(w int) string { return m.theme.Welcome(wel, w) })
}

// setTheme switches the palette, the textarea's and the spinner's styles with it.
func (m *Model) setTheme(t Theme) {
	m.theme = t
	m.paneDirty = true
	st := m.input.Styles()
	st.Focused.Prompt = t.dim
	st.Focused.Placeholder = t.dim
	st.Blurred = st.Focused
	m.input.SetStyles(st)
	m.spin.Style = t.accent
}

// printItem is one FIFO entry: a block, or (clear) the whole transcript that replaces the
// terminal's contents.
type printItem struct {
	text  string
	clear bool
}

type evReflow struct{ seq int }

// reflowDelay lets a window drag settle before the transcript is printed again.
const reflowDelay = 120 * time.Millisecond

// fixed is a block that does not depend on the width.
func fixed(s string) func(int) string { return func(int) string { return s } }

// printer is a long-lived Cmd: it takes blocks off the FIFO and prints each above the live
// area, in order. It ends with the program.
func (m *Model) printer() tea.Msg {
	<-m.attached
	for it := range m.prints {
		if it.clear {
			// the screen, then the scrollback (\x1b[3J), so the old width leaves no copy behind
			m.sender(tea.ClearScreen())
			it.text = "\x1b[3J" + it.text
		}
		m.sender(tea.Printf("%s\n", it.text)())
	}
	return nil
}

func tick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return evTick{at: t} })
}

// Update is the event loop.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var boardCmd tea.Cmd
	if m.board != nil {
		cmd, done := m.boardUpdate(msg)
		if done {
			return m, cmd
		}
		boardCmd = cmd
	}
	if boardCmd != nil {
		mm, cmd := m.update(msg)
		return mm, tea.Batch(boardCmd, cmd)
	}
	return m.update(msg)
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && m.bview != nil {
		return m, m.bodiesKey(k)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width <= 0 || msg.Height <= 0 {
			return m, nil // a terminal that reports no size keeps the last one (80×24 at first)
		}
		m.width, m.height = msg.Width, msg.Height
		if m.width < 20 {
			m.width = 20
		}
		m.input.SetWidth(m.width - 4)
		if !m.ready {
			m.ready = true
			m.lastWidth = m.width
			return m, m.welcome()
		}
		if m.width == m.lastWidth {
			return m, nil
		}
		m.reflowSeq++
		seq := m.reflowSeq
		return m, tea.Tick(reflowDelay, func(time.Time) tea.Msg { return evReflow{seq} })
	case evReflow:
		if m.board != nil || m.bview != nil || msg.seq != m.reflowSeq || m.width == m.lastWidth {
			return m, nil
		}
		return m, m.reflow()
	case tea.BackgroundColorMsg:
		if !m.theme.Forced && msg.IsDark() != m.theme.Dark {
			m.setTheme(NewTheme(msg.IsDark()))
		}
		m.bgKnown = true
		return m, m.welcome()
	case evBgWait:
		m.bgKnown = true
		return m, m.welcome()
	case evIntro:
		if m.intro == nil {
			return m, nil
		}
		if !m.ready || m.stepIntro() {
			return m, introTick()
		}
		m.intro = nil
		return m, m.welcome()
	case evToastFrame:
		if m.stepToasts() {
			return m, toastTick()
		}
		return m, nil
	case evTick:
		if len(m.toasts) > 0 {
			m.stepToasts()
		}
		m.stepPulses(time.Now())
		// the zone worker's blink: closed one tick in twelve
		m.railTick++
		m.blinkOn = m.railTick%12 == 11
		// the flash sweep: a landed Subagent's row flashes in the live area for flashHold, then owes
		// the scrollback exactly one tombstone — the tick that sees the flash spent prints it
		var lands []tea.Cmd
		for _, n := range m.subagentSeq {
			c := m.subagents[n]
			if c == nil || c.State != "done" || c.Report == nil {
				continue
			}
			p := m.pulses[n]
			if p == nil || p.doneAt.IsZero() || time.Since(p.doneAt) >= flashHold {
				lands = append(lands, m.landSubagent(c))
			}
		}
		if m.busy || m.choice != nil {
			m.footer = m.host.Footer()
		}
		if m.railVisible() > 0 {
			plans := m.host.Plans()
			m.plans = plans
			if m.planSticky {
				if m.planIdx >= len(plans) {
					m.planIdx = 0
				}
			} else if len(plans) > 0 {
				// the steady state: the orchestrator's own deck rides the rail; ctrl+p walks the others.
				// A live orchestrator deck is preferred over a stale one (2026-10-13 relay): booting
				// facing a stale deck armed the 'c' chord from the first keystroke.
				m.planIdx = 0
				for i, p := range plans {
					if (p.Rank == "orchestrator") && !p.Stale && !p.Archived {
						m.planIdx = i
						break
					}
				}
			}
		}
		if m.message != "" && time.Now().After(m.msgUntil) {
			m.message = ""
		}
		if len(lands) > 0 {
			return m, tea.Batch(append([]tea.Cmd{tick()}, lands...)...)
		}
		return m, tick()
	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		m.shimmer++
		return m, cmd
	case tea.MouseWheelMsg:
		if cmd, ok := m.vpWheel(msg); ok {
			return m, cmd
		}
		return m, nil
	case tea.MouseClickMsg:
		// a click on a subagents-card row switches the plan card to that agent's deck — the agent's
		// role lane tints the card (that's the role that turn is running under)
		if msg.Button == tea.MouseLeft && m.railVisible() > 0 && m.choice == nil {
			cw := m.width - m.railVisible()
			if msg.X > cw && len(m.subagentRows) > 0 {
				if name, ok := m.subagentRows[msg.Y]; ok {
					for i, p := range m.plans {
						if p.Agent == name {
							m.planIdx = i
							m.planSticky = true
							m.say("plan · " + p.Agent)
							break
						}
					}
				}
			}
			// a click on a card's top border collapses/expands it
			if msg.X > cw && len(m.railCardRows) > 0 {
				for key, y := range m.railCardRows {
					if y == msg.Y {
						if m.railCollapsed == nil {
							m.railCollapsed = map[string]bool{}
						}
						m.railCollapsed[key] = !m.railCollapsed[key]
						break
					}
				}
			}
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.PasteMsg:
		if m.bview != nil && m.bview.pipe {
			// the pane's pipe swallows pastes the same as keys: they land in the pipe's line
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.choice != nil {
			if m.view.Typing {
				m.view.Typed += msg.Content
			}
			return m, nil
		}
		return m, m.paste(msg.Content)
	case EvStream:
		if m.isSession(msg.Agent) {
			if msg.Kind == "thinking" {
				if m.think.Len() == 0 {
					m.thinkStart = time.Now()
				}
				m.think.WriteString(msg.Text)
			}
			return m, nil
		}
		m.logOf(msg.Agent).appendStream(msg.Kind, msg.Text)
		if p := m.pulses[msg.Agent]; p != nil {
			p.active = time.Now()
		}
		return m, nil
	case EvAgentStep:
		m.logOf(msg.Agent).step(msg.Tool, msg.End)
		for _, p := range msg.Tool.Wrote {
			m.noteWrite(p, msg.Agent)
		}
		if p := m.pulses[msg.Agent]; p != nil {
			p.active = time.Now()
		}
		return m, nil
	case EvDelta:
		think := m.flushThinking()
		m.stream.WriteString(msg.Text)
		m.streamed = true
		return m, tea.Batch(think, m.flushStream(false))
	case EvState:
		if msg.Agent == "" || m.isSession(msg.Agent) {
			if msg.State == "tool" && msg.Tool == "" && strings.HasPrefix(m.verb, "Running ") {
				return m, nil
			}
			if msg.State != "done" {
				if msg.State != m.state {
					m.verbSeed++
				}
				m.state = msg.State
				m.verb = Verb(m.words.Dist, m.words.Session(), msg.State, msg.Tool, m.verbSeed)
			}
			return m, nil
		}
		if c := m.subagents[msg.Agent]; c != nil && c.State != "done" {
			c.State = msg.State
			m.notePulse(msg.Agent, c.State, msg.Tool)
		}
		m.logOf(msg.Agent).state = msg.State
		return m, nil
	// evSnapShot (tests): hand the frame back over the channel, rendered on the program's own
	// goroutine — a direct Render from a test goroutine ranks the tick loop.
	case evSnapshot:
		msg.c <- m.Render()
		return m, nil
	case EvToolStart:
		cmd := tea.Batch(m.flushThinking(), m.flushStream(true))
		t := msg.Tool
		t.Status = "running"
		m.tools = append(m.tools, &t)
		m.state = "tool"
		m.verb = Verb(m.words.Dist, m.words.Session(), "tool", t.Name, m.verbSeed)
		return m, cmd
	case EvToolEnd:
		t := msg.Tool
		for i, r := range m.tools {
			if r.ID == t.ID {
				m.tools = append(m.tools[:i], m.tools[i+1:]...)
				break
			}
		}
		m.verb = "Thinking…"
		if m.collapses(t) {
			m.lastBlock = &collapsed{tool: &t}
		}
		for _, p := range t.Wrote {
			m.noteWrite(p, m.words.Session())
		}
		return m, m.print(func(w int) string { return m.theme.Tool(t, w) })
	case EvSubagent:
		c := msg.Subagent
		if c.Word == "" {
			c.Word = m.words.Subagent
		}
		cur := m.subagents[c.Name]
		if cur == nil {
			m.subagentSeq = append(m.subagentSeq, c.Name)
			cur = &SubagentView{}
			m.subagents[c.Name] = cur
			m.logOf(c.Name).rank = c.Rank
			m.logOf(c.Name).state = c.State // the agent pane reads this; a done event must close it, never stay "thinking"
			// the model's text before the dispatch stays above the block
			cmd := m.flushStream(true)
			*cur = c
			m.notePulse(c.Name, c.State, "")
			if c.Report == nil && c.State != "done" {
				return m, tea.Sequence(cmd, m.print(func(w int) string { return m.theme.Notice("→ "+c.Word+" "+c.Name+" started", w) }))
			}
			// a Subagent arriving already done flashes in the live area for flashHold; the tick
			// sweep prints its tombstone
			return m, cmd
		}
		if c.Rank != "" {
			cur.Rank, cur.Role, cur.Model = c.Rank, c.Role, c.Model
			m.logOf(c.Name).rank = c.Rank
		}
		if c.Ask != "" {
			cur.Ask = c.Ask
		}
		cur.State, cur.Elapsed, cur.Failed = c.State, c.Elapsed, c.Failed
		m.logOf(c.Name).state = c.State // same close-out as the open branch: the agent pane never outlives its card
		if c.Exit != "" {
			cur.Exit = c.Exit
		}
		if c.Report != nil {
			cur.Report = c.Report
		}
		m.notePulse(c.Name, cur.State, "")
		if cur.State == "done" && cur.Report != nil {
			// same flash hold: the row settles in the live area first, the sweep owes the
			// tombstone once; the turn's end may print it sooner (its sweep keeps the print
			// order the wire implied)
			return m, nil
		}
		return m, nil
	case EvTurnStart:
		m.begin()
		if msg.Auto {
			return m, m.print(func(w int) string { return m.theme.Notice("↻ continuing with what arrived: "+oneLine(msg.Text), w) })
		}
		return m, m.print(func(w int) string { return m.theme.User(msg.Text, w) })
	case EvTurnDone:
		return m, m.finish(msg)
	case EvNotice:
		return m, m.print(func(w int) string { return m.theme.Notice(msg.Text, w) })
	case EvError:
		return m, m.print(func(w int) string { return m.theme.Error(msg.Text, w) })
	case EvLines:
		return m, m.print(fixed(strings.Join(msg.Lines, "\n")))
	case EvChoice:
		m.choice = &msg
		m.view = msg.View
		m.choiceW, m.choiceH = 0, 0 // a fresh card reopens its growth-only pin
		if msg.View.Recommended != "" {
			for i, o := range msg.View.Options {
				if o == msg.View.Recommended {
					m.view.Cursor = i // the asker's lawful advice pre-seats the cursor; the human still picks
					break
				}
			}
		}
		m.choiceAt = time.Now()
		m.verb = "Waiting for you…"
		return m, m.flushStream(true)
	case EvChoiceAnswered:
		// an out-of-band resolution — the dashboard answered, the watchdog interrupted or
		// suspended, the session ended beneath the card — must collapse the modal here too.
		// Without this the card stayed on screen eating keystrokes after its gate resolved
		// (found by the suspended-card flow, freeze review 2026-10-07).
		if m.choice != nil {
			m.choice = nil
			m.choiceW, m.choiceH = 0, 0
			v := chosen(m.view, ChoiceAnswer{Index: -1, Text: msg.Answer})
			m.verb = "Thinking…"
			return m, m.print(func(w int) string { return m.theme.Choice(v, w) })
		}
		return m, nil
	case evSlashDone:
		var cmds []tea.Cmd
		if len(msg.lines) > 0 {
			cmds = append(cmds, m.print(fixed(strings.Join(msg.lines, "\n"))))
		}
		if msg.quit {
			m.quit = true
			cmds = append(cmds, tea.Quit)
		}
		return m, tea.Sequence(cmds...)
	case EvQuit:
		m.quit = true
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// noteWrite records a path an agent touched; the rail's ledger shows latest-last, one line per
// path, held in the writing agent's rank lane.
func (m *Model) noteWrite(path, agent string) {
	if path == "" {
		return
	}
	for i, f := range m.files {
		if f.Path == path {
			m.files = append(m.files[:i], m.files[i+1:]...)
			break
		}
	}
	rank, _ := m.rankOf(agent)
	m.files = append(m.files, RailFile{Path: path, Agent: agent, Lane: m.theme.lane(rank)})
}

// railVisible: the sidebar's width for this window (railSize) — 0 keeps the session one
// column. A choice no longer dismisses it: the modal's dim veil covers both panes evenly.
func (m *Model) railVisible() int {
	return railSize(m.width, m.height)
}

// choiceFloat: on windows with room the ask card is a true overlay (N16 slice 3) — the frame
// darkens, the card centres; below the tripwire it keeps flowing inline, as always.
func (m *Model) choiceFloat() bool {
	return m.choice != nil && m.width >= choiceMinW && m.height >= choiceMinH
}

// paneChat is the pane mode's scrollback: every finished block rendered at the chat pane's
// width, cached and rebuilt on change (a block landed, the width moved, the theme swapped).
// Clipping happens in splitPanes.
func (m *Model) paneChat(cw int) []string {
	if !m.paneDirty && m.paneCache != nil && m.paneW == cw && m.paneRef == m.reflowSeq {
		return m.paneCache
	}
	var out []string
	for _, r := range m.blocks {
		if b := r(cw); b != "" {
			out = append(out, strings.Split(b, "\n")...)
			out = append(out, "")
		}
	}
	m.paneCache, m.paneW, m.paneRef, m.paneDirty = out, cw, m.reflowSeq, false
	return out
}

// splitPanes is the window's frame: chat left (cw) — the transcript viewport (paneChat) plus
// the live rows — one │ divider, the sidebar right (rail) — both padded to the pane height, so
// the split reads as two regions, not floating text. The sidebar's cells carry its background
// all the way down; the chat pads with terminal background. The newest chat lines win the
// clip: the pane never scrolls the old away first.
func (m *Model) splitPanes(live []string, rail, cw, paneH int) string {
	cl := m.feedVP(live, cw, paneH) // the viewport's own rows: exact-height, scroll-carried
	if len(cl) > paneH {
		cl = cl[len(cl)-paneH:]
	}
	for len(cl) < paneH {
		cl = append([]string{""}, cl...)
	}
	for i, l := range cl {
		l = ansi.Truncate(l, cw, "")
		cl[i] = l + padRight(l, cw)
	}
	rv := m.railView(rail - 3)
	rl := strings.Split(m.theme.Rail(rv, rail-3), "\n")
	// the rail's scroll: the wheel over its column walks the rows. Never scroll below the
	// content's end — the last row may be a trailing pad we drop so the content always
	// fills the pane; ▲/▼ on the rail's edge rows say what lies beyond.
	content := len(rl)
	if content > 0 && strings.TrimSpace(ansi.Strip(rl[content-1])) == "" {
		content--
	}
	maxScroll := max(content-paneH, 0)
	m.railScroll = min(max(m.railScroll, 0), maxScroll)
	if off := m.railScroll; off > 0 && off < len(rl) {
		rl = rl[off:]
	}
	// subagent hit-rows for clicks: each subagents-card row (`◆ <name>`) answers which plan to show
	m.subagentRows = map[int]string{}
	m.railCardRows = map[string]int{}
	floor := 0
	if m.bview == nil && m.board == nil {
		for i, rl0 := range rl {
			raw := ansi.Strip(rl0)
			if !strings.Contains(raw, "◆ ") {
				continue
			}
			for _, c := range rv.Subagents {
				if strings.Contains(raw, "◆ "+c.Name) {
					m.subagentRows[i+floor] = c.Name
					break
				}
			}
		}
		// each collapsible rail card answers its top border row — a click there collapses it
		if rv.Plan != nil || len(rv.Subagents) > 0 || len(rv.Files) > 0 {
			// the rail card order is fixed: vitals (always), subagents (when any skill), plan (when Penned), files
			var cardKeys []string
			cardKeys = append(cardKeys, "vitals") // exempt from collapse, skipped
			if len(rv.Subagents) > 0 {
				cardKeys = append(cardKeys, "subagents")
			}
			if rv.Plan != nil {
				cardKeys = append(cardKeys, "plan")
			}
			cardKeys = append(cardKeys, "files")
			y := 0
			for i, rl0 := range rl {
				raw := ansi.Strip(rl0)
				if !strings.HasPrefix(strings.TrimSpace(raw), "╭") {
					continue
				}
				if y < len(cardKeys) {
					key := cardKeys[y]
					if key != "vitals" {
						m.railCardRows[key] = i + floor
					}
				}
				y++
			}
		}
	}
	if len(rl) > paneH {
		rl = rl[:paneH]
	}
	bg := lipgloss.NewStyle().Background(m.theme.paneBg())
	for len(rl) < paneH {
		rl = append(rl, bg.Render(strings.Repeat(" ", rail-3)))
	}
	mut := m.theme.dim
	if !m.theme.Color {
		mut = m.theme.dim
	}
	// the rail's scroll markers: ▲/▼ on the edge rows say what lies beyond
	if m.railScroll > 0 && paneH > 0 {
		rl[0] = bg.Render(strings.Repeat(" ", rail-3-1)) + mut.Render("▲")
	}
	if m.railScroll < maxScroll && paneH > 0 {
		rl[paneH-1] = bg.Render(strings.Repeat(" ", rail-3-1)) + mut.Render("▼")
	}
	// the pane edge: a two-column soft shadow the rail casts on the chat (density-gradient
	// runes tinted like the pane's sunk light), then the divider itself riding the pane's bg
	shadow := m.theme.paneShadow()
	sep := lipgloss.NewStyle().Background(m.theme.paneBg()).Foreground(m.theme.border.GetForeground()).Render("│")
	rows := make([]string, paneH)
	for i := 0; i < paneH; i++ {
		rows[i] = cl[i] + shadow + sep + rl[i]
	}
	return strings.Join(rows, "\n")
}

// padRight returns the plain spaces that take a (possibly styled) line to exactly w cells;
// truncate first, then pad.
func padRight(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return strings.Repeat(" ", d)
	}
	return ""
}

// railView is the rail's frame data: the workspace's mascot, the run's vitals, the touch ledger,
// the subagent roster — live subagents first in dispatch order, then the landed agents the roster
// still remembers. rw is the rail's width; short windows shrink the cards' caps and drop the
// mascot in favour of the facts.
func (m *Model) railView(rw int) RailView {
	v := RailView{
		Workspace: filepath.Base(strings.TrimRight(m.footer.Workspace, "/")),
		Model:     m.footer.Model,
		Dash:      m.footer.Dash,
		CtxPct:    m.footer.CtxPct,
		CtxKnown:  m.footer.CtxKnown,
		Stress:    strings.Contains(m.footer.Ctx, "stress"),
		Zone:      m.footer.Zone,
		Tokens:    m.footer.Tokens,
		Cost:      m.footer.Cost,
		Files:     m.files,
		CtxMax:    m.footer.CtxMax,
		Layers:    m.footer.Layers,
	}
	if m.railCollapsed != nil {
		v.Collapsed = map[string]bool{
			"subagents": m.railCollapsed["subagents"],
			"plan":      m.railCollapsed["plan"],
			"files":     m.railCollapsed["files"],
		}
	}
	if len(m.plans) > 0 && m.planIdx < len(m.plans) {
		p := m.plans[m.planIdx]
		// the plan card holds the ROLE's lane (the hat the named agent is running under),
		// not the agent's rank: running subagents know their role; fallback daisy-chains the
		// agent's rank's role lane (drafter for coordinators, judge for domain owners, analyst for zone workers)
		p.Lane = m.theme.lane(p.Rank)
		if p.Role != "" {
			if l := m.theme.lane(p.Role); l != nil {
				p.Lane = l
			}
		}
		v.Plan = &p
	}
	if m.height < 40 {
		v.MaxFiles = 5
		v.MaxSubagents = 4
	}
	if m.height >= 30 {
		if art, _ := mascotArt(m.words.Dist, m.blinkOn); len(art) > 0 {
			w := 0
			for _, l := range art {
				w = max(w, ansi.StringWidth(l))
			}
			if w+6 <= rw { // the sprite only rides where it fits inside a card's inner width
				v.Mascot = art
			}
		}
	}
	for _, n := range m.subagentSeq {
		c := m.subagents[n]
		if c == nil {
			continue
		}
		st := c.State
		if st == "" {
			st = "queued"
		}
		aged := ""
		if c.Elapsed > 0 {
			aged = c.Elapsed.Round(time.Second).String()
		}
		v.Subagents = append(v.Subagents, RailSubagent{Name: n, State: st, Aged: aged, Lane: m.theme.lane(c.Rank), Done: c.State == "done", Failed: c.Failed})
	}
	for i := len(m.logOrder) - 1; i >= 0 && len(v.Subagents) < 8; i-- {
		b := m.logOrder[i]
		if b == m.words.Session() || m.subagents[b] != nil {
			continue
		}
		l := m.logs[b]
		if l == nil || l.state != "done" {
			continue
		}
		v.Subagents = append(v.Subagents, RailSubagent{Name: b, State: "done", Lane: m.theme.lane(l.rank), Done: true})
	}
	if len(v.Subagents) > 8 {
		v.Subagents = v.Subagents[:8]
	}
	return v
}

func (m *Model) isSession(agent string) bool {
	return agent == "" || agent == m.words.Session()
}

// Session is the session agent's name (the one that is not a Subagent).
func (w Words) Session() string {
	if w.SessionName != "" {
		return w.SessionName
	}
	return "orchestrator"
}

func (m *Model) begin() {
	m.busy = true
	m.turnStart = time.Now()
	m.verbSeed++
	m.state = "thinking"
	m.think.Reset()
	m.verb = Verb(m.words.Dist, m.words.Session(), "thinking", "", m.verbSeed)
	m.stream.Reset()
	m.streamed = false
	m.tools = nil
	m.queued = 0
}

func (m *Model) finish(ev EvTurnDone) tea.Cmd {
	cmds := []tea.Cmd{m.flushThinking()}
	m.state = ""
	if ev.Streamed || m.streamed {
		cmds = append(cmds, m.flushStream(true))
	} else if t := strings.TrimSpace(ev.Text); t != "" {
		cmds = append(cmds, m.print(func(w int) string { return m.theme.Assistant(t, w) }))
	}
	for _, t := range m.tools {
		t.Status = "failed"
		t.Why = "interrupted"
		cmds = append(cmds, m.print(func(w int) string { return m.theme.Tool(*t, w) }))
	}
	m.tools = nil
	// every subagent that ended during the turn owes one tombstone: a report-carrying card
	// landed at its dispatch step's end — this sweeps the rest (a failed or unparseable
	// report, an interrupted run) so no done subagent waits out the session in the live area
	for _, n := range append([]string(nil), m.subagentSeq...) {
		if c := m.subagents[n]; c != nil && c.State == "done" {
			cmds = append(cmds, m.landSubagent(c))
		}
	}
	if ev.Interrupted {
		cmds = append(cmds, m.print(func(w int) string { return m.theme.Notice("■ interrupted", w) }))
	} else if len(ev.Holes) > 0 {
		cmds = append(cmds, m.print(func(w int) string { return m.theme.Holes(ev.Holes, w) }))
	}
	if ev.Verdict != "" {
		cmds = append(cmds, m.print(func(w int) string { return m.theme.Gate(ev.Verdict, ev.LogRel, w, m.words.Gate) }))
		if strings.HasPrefix(ev.Verdict, "pass") {
			cmds = append(cmds, m.Toast("◆ "+m.words.Gate+" pass", m.theme.pill("ok")))
		} else if strings.HasPrefix(ev.Verdict, "fail") {
			cmds = append(cmds, m.Toast("◆ "+m.words.Gate+" fail", m.theme.pill("bad")))
		}
		// the burst rains when a Subagent ran this turn — a courtless verdict keeps its quiet
		if len(m.logOrder) > 0 {
			m.burstAt, m.burstGood, m.burstOn = time.Now(), strings.HasPrefix(ev.Verdict, "pass"), true
		}
	}
	if ev.Hint != "" {
		cmds = append(cmds, m.print(func(w int) string { return m.theme.Notice(ev.Hint, w) }))
	}
	m.busy = false
	m.verb = ""
	m.footer = m.host.Footer()
	return tea.Sequence(cmds...)
}

// print queues a finished block for the scrollback, followed by a blank line. In pane mode the
// conversation lives in the chat pane's own viewport instead (bubbletea's inline renderer and
// interleaved prints both claim the same rows when the panes own the window — the paste dumps
// upstream of this comment are the scars); the cache invalidates so the pane picks it up on
// the next frame.
func (m *Model) print(render func(w int) string) tea.Cmd {
	block := render(m.width)
	if block == "" {
		return nil
	}
	m.blocks = append(m.blocks, render)
	if m.railVisible() > 0 {
		m.paneDirty = true
		return nil
	}
	if m.board != nil || m.bview != nil {
		m.held = append(m.held, block)
		return nil
	}
	m.enqueue(printItem{text: block})
	m.printedIdx = len(m.blocks)
	return nil
}

// reflow clears the terminal and prints the whole transcript again at the current width. Under
// panes the transcript never left — this just dirties the viewport cache and the frame follows.
func (m *Model) reflow() tea.Cmd {
	m.lastWidth = m.width
	if m.railVisible() > 0 {
		m.paneDirty = true
		m.reflowSeq++
		return nil
	}
	var parts []string
	for _, r := range m.blocks {
		if b := r(m.width); b != "" {
			parts = append(parts, b)
		}
	}
	m.enqueue(printItem{text: strings.Join(parts, "\n\n"), clear: true})
	m.printedIdx = len(m.blocks)
	return nil
}

func (m *Model) enqueue(it printItem) {
	select {
	case m.prints <- it:
	default:
		// a full FIFO (the program is gone or stuck): the block is dropped rather than the loop
	}
}

// collapses is the shrink-policy: any done tool with an output or diff rides collapsed —
// status + elapsed only; the agent's logs are ctrl+o ownership, never ambient.
func (m *Model) collapses(t ToolView) bool {
	if t.Status != "done" {
		return false
	}
	return t.Diff != nil || countLines(t.Output) > 0
}

// landSubagent prints a Subagent's finished block and drops it from the live area.
func (m *Model) landSubagent(c *SubagentView) tea.Cmd {
	delete(m.subagents, c.Name)
	delete(m.pulses, c.Name)
	for i, n := range m.subagentSeq {
		if n == c.Name {
			m.subagentSeq = append(m.subagentSeq[:i], m.subagentSeq[i+1:]...)
			break
		}
	}
	if c.Report != nil && len(c.Report.Findings)+len(c.Report.Holes)+len(c.Report.Unsaid)+len(c.Report.Verdicts)+len(c.Report.Other) > collapseLines {
		cp := *c
		m.lastBlock = &collapsed{subagent: &cp}
	}
	cv := *c
	pill, word := m.theme.pill("ok"), "✓ "+cv.Name+" done"
	if cv.Failed {
		pill, word = m.theme.pill("bad"), "✗ "+cv.Name+" failed"
	}
	return tea.Batch(m.print(func(w int) string { return m.theme.Subagent(cv, w) }), m.Toast(word, pill))
}

// flushStream moves complete paragraphs of the streamed answer into the scrollback (all of it
// when final), keeping only the paragraph still being written live. A code fence is never
// split.
func (m *Model) flushStream(final bool) tea.Cmd {
	s := m.stream.String()
	if strings.TrimSpace(s) == "" {
		if final {
			m.stream.Reset()
		}
		return nil
	}
	if final {
		m.stream.Reset()
		return m.print(func(w int) string { return m.theme.Assistant(s, w) })
	}
	cut := safeCut(s)
	if cut <= 0 {
		return nil
	}
	head, tail := s[:cut], s[cut:]
	m.stream.Reset()
	m.stream.WriteString(tail)
	return m.print(func(w int) string { return m.theme.Assistant(head, w) })
}

// safeCut finds the last blank line outside a code fence such that the paragraph after it is
// not a continuation of a list or an indented block; 0 when there is none.
func safeCut(s string) int {
	lines := strings.Split(s, "\n")
	fence := false
	best := 0
	off := 0
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
		}
		if !fence && l == "" && i > 0 && i+1 < len(lines) {
			next := lines[i+1]
			if next == "" || isListLine(next) || strings.HasPrefix(next, "  ") {
				off += len(l) + 1
				continue
			}
			// the previous paragraph must not be a list that the next line continues
			best = off + 1
		}
		off += len(l) + 1
	}
	return best
}

func isListLine(l string) bool {
	t := strings.TrimLeft(l, " ")
	if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ ") {
		return true
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(t) && (t[i] == '.' || t[i] == ')') && t[i+1] == ' '
}

// View draws the live area.
// View is the frame: the board takes the alternate screen while it is open.
func (m *Model) View() tea.View {
	v := tea.NewView(m.Render())
	v.AltScreen = m.board != nil || m.bview != nil
	// the window title and the terminal tab's progress follow the session
	state := "idle"
	switch {
	case m.choice != nil:
		state = "waiting for you"
		v.ProgressBar = &tea.ProgressBar{State: tea.ProgressBarWarning, Value: 100}
	case m.busy:
		state = strings.ToLower(strings.TrimSuffix(m.verb, "…"))
		v.ProgressBar = &tea.ProgressBar{State: tea.ProgressBarIndeterminate}
	}
	v.WindowTitle = m.words.Dist + " · " + filepath.Base(m.footer.Workspace) + " · " + state
	if m.board != nil {
		v.MouseMode = tea.MouseModeCellMotion
	} else if m.railVisible() > 0 {
		// the chat pane scrolls its transcript with the wheel while the panes own the window
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// Render is the frame's text: the live area, or the board. With the sidebar on, the frame is
// an invariant: chat pane │ divider │ sidebar rows, then the home rows — the total is always
// m.height (crush's own lesson: its fullscreen layout is rectangle-exact; our inline mode pays
// the same discipline or the terminal ghosts). Without the sidebar the session flows as
// before, one column into the scrollback.
func (m *Model) Render() string {
	if m.quit {
		return ""
	}
	if m.board != nil {
		return m.boardView()
	}
	if m.bview != nil {
		return m.bodiesView()
	}
	if m.intro != nil && m.ready {
		return m.introView()
	}
	cw := m.width
	rail := m.railVisible()
	if rail == 0 && m.printedIdx < len(m.blocks) {
		// pane exit (or an always-narrow run): the scrollback gets the blocks it never saw
		for _, r := range m.blocks[m.printedIdx:] {
			if b := r(m.width); b != "" {
				m.enqueue(printItem{text: b})
			}
		}
		m.printedIdx = len(m.blocks)
	}
	if rail > 0 {
		cw = m.width - rail // the rail's width carries its divider cell
	}
	var live []string
	if s := m.stream.String(); m.busy && strings.TrimSpace(s) != "" {
		live = append(live, m.theme.Assistant(s, cw), "")
	}
	for _, t := range m.tools {
		live = append(live, m.theme.Tool(*t, cw), "")
	}
	for _, n := range m.subagentSeq {
		if c := m.subagents[n]; c != nil {
			// a done subagent's clock is over: its card holds the elapsed its terminal
			// event carried — recomputing it every frame kept dead subagents "running"
			if c.State != "done" {
				c.Elapsed = time.Since(m.subagentStart(c))
			}
			live = append(live, m.theme.Subagent(*c, cw, m.pulseFor(c)), "")
		}
	}
	if m.choice == nil && m.busy {
		tokens := m.footer.Tokens
		if m.think.Len() > 0 && strings.TrimSpace(m.stream.String()) == "" {
			live = append(live, m.theme.liveThinking(m.think.String(), cw, 3), "")
		}
		live = append(live, m.statusLines(m.theme.Spinner(m.spin.View(), m.theme.Shimmer(m.verb, shimmerPhase(m.shimmer, len([]rune(m.verb)))), time.Since(m.turnStart), tokens, cw)))
	}
	// the home rows: shortcuts (when open), the input (its textarea can grow), menu or
	// completions under it, the footer — the panes own everything above
	var home []string
	if m.shortcuts {
		home = append(home, m.theme.Shortcuts(DefaultShortcuts(), m.width))
	}
	home = append(home, m.theme.InputBox(m.input.View(), m.width, m.choice != nil))
	switch {
	case m.menu != nil:
		home = append(home, m.theme.Menu(m.menu.items, m.menu.cursor, m.width))
	case m.comp != nil:
		home = append(home, m.theme.Completions(m.comp.items, m.comp.cursor, m.width))
	}
	f := m.footer
	f.Queued = m.queued
	f.Hint = "? for shortcuts · ctrl+p plans"
	f.Message = m.message
	if f.Subagents == "" {
		f.Subagents = m.words.Subagents
	}
	home = append(home, m.theme.Footer(f, m.width))

	var parts []string
	if rail > 0 {
		// an inline ask (window under the modal tripwire) belongs to the chat pane — inside
		// paneH, never below it, or the frame's height would lie again
		if m.choice != nil && !m.choiceFloat() {
			live = append(live, m.theme.Choice(m.view, cw), "")
		}
		paneH := max(m.height-lines(home), 4)
		parts = append(parts, m.splitPanes(live, rail, cw, paneH), strings.Join(home, "\n"))
	} else {
		parts = live
		if m.choice != nil && !m.choiceFloat() {
			parts = append(parts, m.theme.Choice(m.view, m.width), "")
		}
		parts = append(parts, home...)
	}
	return m.overlay(strings.Join(parts, "\n"))
}

// lines is the row count of a joined multi-line string list.
func lines(parts []string) int {
	n := 0
	for _, p := range parts {
		n += strings.Count(p, "\n") + 1
	}
	return n
}

func (m *Model) subagentStart(c *SubagentView) time.Time {
	if c.started.IsZero() {
		c.started = time.Now()
	}
	return c.started
}

// say shows a transient message in the footer.
func (m *Model) say(s string) {
	m.message = s
	m.msgUntil = time.Now().Add(4 * time.Second)
}

// answer replies to the pending choice.
func (m *Model) answer(a ChoiceAnswer) tea.Cmd {
	if m.choice == nil {
		return nil
	}
	ch := m.choice
	m.choice = nil
	m.choiceW, m.choiceH = 0, 0
	v := chosen(m.view, a)
	select {
	case ch.Reply <- a:
	default:
	}
	m.verb = "Thinking…"
	return m.print(func(w int) string { return m.theme.Choice(v, w) })
}

// chosen is the choice as printed once answered: only the picked option, no cursor hints.
func chosen(v ChoiceView, a ChoiceAnswer) ChoiceView {
	out := v
	out.Typing = false
	switch {
	case a.Aborted:
		out.Options = []string{"(no answer — the session ended)"}
	case a.Index >= 0 && a.Index < len(v.Options):
		out.Options = []string{v.Options[a.Index]}
		if a.Text != "" {
			out.Options = []string{v.Options[a.Index] + ": " + a.Text}
		}
	default:
		out.Options = []string{a.Text}
	}
	out.Notes = nil
	out.Cursor = 0
	out.Answered = true
	return out
}

// Abort answers a pending choice when the program goes away.
func (m *Model) Abort() {
	if m.choice != nil {
		ch := m.choice
		m.choice = nil
		m.choiceW, m.choiceH = 0, 0
		select {
		case ch.Reply <- ChoiceAnswer{Index: -1, Aborted: true}:
		default:
		}
	}
}

func (m *Model) statusText() string {
	return fmt.Sprintf("busy=%v tools=%d subagents=%d", m.busy, len(m.tools), len(m.subagents))
}

// flushThinking folds the thinking so far into a block in the scrollback.
func (m *Model) flushThinking() tea.Cmd {
	text := strings.TrimSpace(m.think.String())
	m.think.Reset()
	if text == "" {
		return nil
	}
	th := thought{text: text, took: time.Since(m.thinkStart)}
	m.lastBlock = &collapsed{thought: &th}
	return m.print(func(w int) string { return m.theme.Thought(th, w) })
}
