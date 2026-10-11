package tui

import (
	"fmt"
	"image/color"
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
	turnSeq       int // counts turns: an answer's pieces share theirs
	verb          string
	stream        strings.Builder
	streamed      bool
	tools         []*ToolView // running tools, in order
	subagents     map[string]*SubagentView
	subagentSeq   []string
	pulses        map[string]*subagentPulse // a live Subagent row's motion, keyed by its name (motion.go)
	files         []RailFile                // the touch ledger: who wrote what, in order, latest last (slice-2 rail)
	plans         []RailPlan                // plan decks the rail shows; planIdx picks the visible one (ctrl+p)
	plansBusy     bool                      // a Plans read is in flight
	planIdx       int
	planSticky    bool            // the human ctrl+p'd off the coordinator default: keep their pick
	subagentRows  map[int]string  // window row → the subagent a roster row shows; a click there switches the plan card
	railCardRows  map[string]int  // sidebar section key ("subagents" | "plan" | "files") → its rule's window row; a click there folds it
	railScroll    int             // the rail's scroll offset in rows: the wheel over the rail column walks it
	railMore      bool            // rows lie below the sidebar's last visible one (the ▼ on its edge)
	railOff       bool            // the human hid the sidebar (alt+s, /sidebar): the frame keeps one column
	railCollapsed map[string]bool // rail card key ("subagents" | "plan" | "files") → collapsed to its header
	lay           layout          // the last frame's geometry: the mouse reads it back
	settings      *settingsState  // /settings: the huh form floating over the frame (settings.go)
	wel           Welcome         // the welcome's facts: the header and /roles read them
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
	blocks    []*block
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

	// the pane-mode chat cache: the blocks' lines at cw. A new block appends its own lines; a
	// width change, a theme swap or a reflow rebuilds it from the blocks' own caches
	paneDirty bool
	paneCache []string
	paneW     int
	paneRef   int
	paneN     int // blocks[:paneN] are in paneCache
	paneVer   int // bumps whenever paneCache changes: the viewport's feed key
	// renderGen bumps on a theme swap: every block's cached render is stale then
	renderGen int
	// paneTail is what the pane's transcript ends with: an answer piece of a turn joins it
	paneTail paneTail
	// ground is the terminal's own background, once it answered (nil before): every theme the
	// session switches to steps its depth from it
	ground   color.Color
	liveMemo struct {
		src    string
		w, gen int
		first  bool
		out    string
	}
	// the chat pane's viewport: transcript scrollback lives here while the panes own the
	// window (pane_vp.go)
	vp      viewport.Model
	vpOK    bool
	vpCW    int
	vpH     int
	vpLines []string // what the viewport holds: the transcript, then the live rows
	vpVer   int      // the paneVer and the live rows the viewport was last fed
	vpLive  []string
	vpTrim  int
	// liveTrim: the transcript rows the live answer takes back (its slab's seam) this frame
	liveTrim int
	inputW   int // the width the textarea was last sized to
	// the chat pane's drag selection (select.go); mouseTerm hands the mouse to the terminal
	sel       *selection
	clickAt   time.Time
	clickSpot spot
	clicks    int
	mouseTerm bool
	// printedIdx: blocks[:printedIdx] own the terminal scrollback already; under panes nothing
	// prints live, so when the window narrows the panes hand the scrollback what it missed —
	// every block exactly once, never twice
	printedIdx int
	// the board: full screen while open; blocks that finish meanwhile wait past printedIdx
	board    *boardState
	sender   func(tea.Msg)
	attached chan struct{}
}

// paneTail is the last block of the pane's transcript: an answer piece, and of which turn.
type paneTail struct {
	answer bool
	turn   int
}

// block is one finished block of the transcript: its render at a width, and the source it
// was drawn from — an answer's markdown — for the copy keys.
type block struct {
	render func(w int) string
	src    string
	answer bool // a piece of the session's answer
	turn   int  // the turn it belongs to (turnSeq)
	// the render cache: blocks are append-only and never change once landed (ctrl+o prints an
	// expanded copy), so one render per width and theme serves every frame and reprint
	w, gen int
	ok     bool
	text   string
	lines  []string
}

// rendered is a block at a width, from its cache when the width and theme still hold.
func (m *Model) rendered(b *block, w int) string {
	if !b.ok || b.w != w || b.gen != m.renderGen {
		b.text, b.lines = b.render(w), nil
		b.w, b.gen, b.ok = w, m.renderGen, true
	}
	return b.text
}

// renderedLines is rendered split into rows.
func (m *Model) renderedLines(b *block, w int) []string {
	t := m.rendered(b, w)
	if b.lines == nil && t != "" {
		b.lines = strings.Split(t, "\n")
	}
	return b.lines
}

// liveAnswer is the paragraph still being written, rendered once per change of text or width.
func (m *Model) liveAnswer(s string, w int) string {
	lm := &m.liveMemo
	first := m.firstPiece()
	if lm.out == "" || lm.src != s || lm.w != w || lm.gen != m.renderGen || lm.first != first {
		lm.src, lm.w, lm.gen, lm.first, lm.out = s, w, m.renderGen, first, m.theme.answer(s, w, first)
	}
	return lm.out
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
			return "› "
		}
		return "  "
	})
	ta.Placeholder = "ask, or / for commands"
	ta.CharLimit = 0
	// the box grows with its visual rows (soft wraps included) up to 8, then scrolls inside
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 8
	ta.SetHeight(1)
	ta.KeyMap.InsertNewline.SetEnabled(false)
	st := ta.Styles()
	st.Focused.CursorLine = lipgloss.NewStyle()
	st.Focused.Base = lipgloss.NewStyle()
	st.Focused.Prompt = theme.accent
	st.Focused.Placeholder = theme.dim
	st.Blurred = st.Focused
	st.Cursor.Color = theme.accent.GetForeground()
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
		// a forced theme still asks: the depth's surfaces step from the real background
		cmds = append(cmds, tea.RequestBackgroundColor)
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
	if wel.Dash == "" {
		wel.Dash = m.footer.Dash
	}
	m.wel = wel
	return m.print(func(w int) string { return m.theme.Welcome(wel, w) })
}

// setTheme switches the palette, the textarea's and the spinner's styles with it.
func (m *Model) setTheme(t Theme) {
	t = t.OnGround(m.ground)
	m.theme = t
	m.paneDirty = true
	m.renderGen++
	st := m.input.Styles()
	st.Focused.Prompt = t.accent
	st.Focused.Placeholder = t.dim
	st.Blurred = st.Focused
	// the cursor block in the primary: a palette white vanishes on a light well
	st.Cursor.Color = t.accent.GetForeground()
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
	if m.settings != nil && m.board == nil && m.bview == nil {
		cmd, handled := m.settingsUpdate(msg)
		if handled {
			return m, cmd
		}
		if cmd != nil {
			mm, next := m.update(msg)
			return mm, tea.Batch(cmd, next)
		}
	}
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
		m.sizeInput()
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
		// the depth steps its surfaces from the terminal's own background
		m.ground = msg.Color
		m.setTheme(m.theme.OnGround(msg.Color))
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
		// the rail's decks load off the UI goroutine, one read in flight at a time
		var load tea.Cmd
		if m.railVisible() > 0 && !m.plansBusy {
			m.plansBusy = true
			host := m.host
			load = func() tea.Msg { return evPlans{plans: host.Plans()} }
		}
		if m.message != "" && time.Now().After(m.msgUntil) {
			m.message = ""
		}
		return m, tea.Batch(append([]tea.Cmd{tick(), load}, lands...)...)
	case evPlans:
		m.plansBusy = false
		m.setPlans(msg.plans)
		return m, nil
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
	case tea.MouseMotionMsg, tea.MouseReleaseMsg:
		cmd, _ := m.mouseSelect(msg)
		return m, cmd
	case evSelScroll:
		return m, m.selScroll()
	case tea.MouseClickMsg:
		if cmd, ok := m.mouseSelect(msg); ok {
			return m, cmd
		}
		// the sidebar's hit regions come from its last frame (railRowsAt): a roster row
		// switches the plan card to that agent's deck, a section's rule folds the section
		if l := m.geom(); msg.Button == tea.MouseLeft && m.choice == nil && l.rc > 0 && msg.X >= l.divX {
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
			cmds = append(cmds, m.leave())
		}
		return m, tea.Sequence(cmds...)
	case EvQuit:
		return m, m.leave()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// evPlans carries the rail's decks, read by a tea.Cmd.
type evPlans struct{ plans []RailPlan }

// setPlans takes a fresh read of the decks: a pinned pick holds, else the orchestrator's own deck
// rides the rail (a live one over a stale one); ctrl+p walks the others.
func (m *Model) setPlans(plans []RailPlan) {
	m.plans = plans
	if m.planSticky {
		if m.planIdx >= len(plans) {
			m.planIdx = 0
		}
		return
	}
	if len(plans) > 0 {
		m.planIdx = 0
		for i, p := range plans {
			if (p.Rank == "orchestrator") && !p.Stale && !p.Archived {
				m.planIdx = i
				break
			}
		}
	}
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

// railVisible: the sidebar's claim on this window (railSize: its column, the divider and the
// right border) — 0 when the window is too small for the frame or the human hid it. A choice
// never dismisses it: the modal's dim veil covers both columns evenly.
func (m *Model) railVisible() int {
	if m.railOff {
		return 0
	}
	return railSize(m.width, m.height)
}

// sizeInput fits the textarea to where it lives: the main column's content width under the
// frame, the inline box's inner width otherwise.
func (m *Model) sizeInput() {
	w := m.width - 4
	if l := m.layout(); l.pane {
		w = l.input.W
	}
	if w != m.inputW {
		m.inputW = w
		m.input.SetWidth(w)
	}
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
	if m.paneDirty || m.paneW != cw || m.paneRef != m.reflowSeq || m.paneN > len(m.blocks) {
		m.paneCache, m.paneN, m.paneTail = nil, 0, paneTail{}
		m.paneW, m.paneRef, m.paneDirty = cw, m.reflowSeq, false
		m.paneVer++
	}
	if m.paneN == len(m.blocks) && m.paneCache != nil {
		return m.paneCache
	}
	for _, b := range m.blocks[m.paneN:] {
		ls := m.renderedLines(b, cw)
		if len(ls) == 0 {
			continue
		}
		if m.theme.Depth && b.answer && m.paneTail.answer && m.paneTail.turn == b.turn && len(m.paneCache) >= 2 && len(ls) > 1 {
			// a turn's next answer piece continues the same slab: the seam's bottom edge, the gap
			// and the new piece's top face go
			m.paneCache = append(append(m.paneCache[:len(m.paneCache)-2], m.theme.answerSeam(cw)), ls[1:]...)
		} else {
			if n := len(m.paneCache); m.theme.Depth && n >= 2 && m.paneCache[n-1] == "" && isDecorRow(m.paneCache[n-2]) && isDecorRow(ls[0]) {
				// two slabs meet: the one's shadow and the other's lit top face part them
				m.paneCache = m.paneCache[:n-1]
			}
			m.paneCache = append(m.paneCache, ls...)
		}
		m.paneCache = append(m.paneCache, "")
		m.paneTail.answer, m.paneTail.turn = b.answer, b.turn
	}
	if m.paneCache == nil {
		m.paneCache = []string{}
	}
	m.paneN = len(m.blocks)
	m.paneVer++
	return m.paneCache
}

// chatRows is the conversation pane's visible rows: the viewport over the transcript and the
// live rows, the selection painted, each row truncated to the pane's width.
func (m *Model) chatRows(live []string, l layout) []string {
	cl := m.feedVP(live, l.chat.W, l.chat.H)
	if len(cl) > l.chat.H {
		cl = cl[len(cl)-l.chat.H:]
	}
	for len(cl) < l.chat.H {
		cl = append([]string{""}, cl...)
	}
	for i, r := range cl {
		cl[i] = ansi.Truncate(r, l.chat.W, "")
	}
	m.highlight(cl, m.vp.YOffset())
	return cl
}

// railRowsAt is the sidebar's visible rows for the layout: scrolled (never past the content's
// end), and its hit regions recorded in window rows — the section rules a click folds, the
// roster rows a click opens the plan of.
func (m *Model) railRowsAt(l layout) []railRow {
	m.subagentRows = map[int]string{}
	m.railCardRows = map[string]int{}
	m.railMore = false
	if l.rc == 0 {
		return nil
	}
	rows := m.theme.railRows(m.railView(l.rc-2), l.rc)
	maxScroll := max(len(rows)-l.rail.H, 0)
	m.railScroll = min(max(m.railScroll, 0), maxScroll)
	rows = rows[m.railScroll:]
	if len(rows) > l.rail.H {
		rows = rows[:l.rail.H]
	}
	m.railMore = m.railScroll < maxScroll
	if m.bview == nil && m.board == nil {
		for i, r := range rows {
			y := l.rail.Y + i
			if r.key != "" {
				m.railCardRows[r.key] = y
			}
			if r.subagent != "" {
				m.subagentRows[y] = r.subagent
			}
		}
	}
	return rows
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
	if art, _ := mascotArt(m.words.Dist, m.blinkOn); len(art) > 0 {
		// the sprite rides beside the vitals where the column is wide enough (no rows spent),
		// above them on tall windows, not at all otherwise
		if rw >= mascotWidth+2+18 || m.height >= 30 && rw >= mascotWidth {
			v.Mascot = art
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
	m.turnSeq++
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
		cmds = append(cmds, m.printAnswer(t))
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
	return m.push(&block{render: render})
}

// printAnswer prints a piece of the session's answer, its markdown kept for ctrl+y.
func (m *Model) printAnswer(md string) tea.Cmd {
	first := m.firstPiece()
	return m.push(&block{render: func(w int) string { return m.theme.answer(md, w, first) }, src: md, answer: true, turn: m.turnSeq})
}

// firstPiece: no piece of this turn's answer has landed yet — the next one holds the gutter's ✦.
func (m *Model) firstPiece() bool {
	for i := len(m.blocks) - 1; i >= 0; i-- {
		b := m.blocks[i]
		if b.answer && b.turn == m.turnSeq {
			return false
		}
		if b.answer && b.turn < m.turnSeq {
			break
		}
	}
	return true
}

func (m *Model) push(b *block) tea.Cmd {
	m.syncDepth()
	if m.paneMode() {
		// the pane draws it at the chat's width: render there once, never at the full width too
		if m.rendered(b, m.chatW()) == "" {
			return nil
		}
		m.blocks = append(m.blocks, b)
		return nil
	}
	block := m.rendered(b, m.width)
	if block == "" {
		return nil
	}
	m.blocks = append(m.blocks, b)
	if m.board != nil || m.bview != nil {
		return nil // held: catchUp prints it once the screen comes back
	}
	m.enqueue(printItem{text: block})
	m.printedIdx = len(m.blocks)
	return nil
}

// reflow clears the terminal and prints the whole transcript again at the current width. Under
// panes the transcript never left — this just dirties the viewport cache and the frame follows.
func (m *Model) reflow() tea.Cmd {
	m.syncDepth()
	m.lastWidth = m.width
	m.sel = nil // the transcript rewraps: content coordinates move
	if m.paneMode() {
		m.paneDirty = true
		m.reflowSeq++
		return nil
	}
	var parts []string
	for _, r := range m.blocks {
		if b := m.rendered(r, m.width); b != "" {
			parts = append(parts, b)
		}
	}
	m.enqueue(printItem{text: strings.Join(parts, "\n\n"), clear: true})
	m.printedIdx = len(m.blocks)
	return nil
}

// leave ends the program. Under panes the transcript lived in the chat viewport and never
// reached the scrollback: the blocks it missed print above the erased frame first, at the full
// width, so the conversation stays in the terminal after exit.
func (m *Model) leave() tea.Cmd {
	m.quit = true
	m.board, m.bview = nil, nil // the alternate screen keeps no print: leave it first
	m.syncDepth()               // the scrollback is drawn flat
	var parts []string
	for _, r := range m.blocks[min(m.printedIdx, len(m.blocks)):] {
		if b := m.rendered(r, m.width); b != "" {
			parts = append(parts, b)
		}
	}
	m.printedIdx = len(m.blocks)
	if len(parts) == 0 {
		return tea.Quit
	}
	return tea.Sequence(tea.Printf("%s\n", strings.Join(parts, "\n\n")), tea.Quit)
}

// catchUp prints the blocks the scrollback has not seen (held under the board or the agents
// view, or kept by the panes) — every block exactly once.
func (m *Model) catchUp() {
	m.syncDepth()
	if m.printedIdx >= len(m.blocks) {
		return
	}
	for _, r := range m.blocks[m.printedIdx:] {
		if b := m.rendered(r, m.width); b != "" {
			m.enqueue(printItem{text: b})
		}
	}
	m.printedIdx = len(m.blocks)
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
		return m.printAnswer(s)
	}
	cut := safeCut(s)
	if cut <= 0 {
		return nil
	}
	head, tail := s[:cut], s[cut:]
	m.stream.Reset()
	m.stream.WriteString(tail)
	return m.printAnswer(head)
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
	} else if m.paneMode() && !m.mouseTerm {
		// the chat pane scrolls its transcript with the wheel and selects with a drag while the
		// panes own the window; /mouse gives the mouse back to the terminal
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// Render is the frame's text: the live area, or the board. With the sidebar on, the frame is
// an invariant: the headroom row (paneTop), chat pane │ divider │ sidebar rows, then the home
// rows — the total is always m.height (crush's own lesson: its fullscreen layout is rectangle-exact; our inline mode pays
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
	l := m.layout()
	m.lay = l
	if !l.pane {
		m.catchUp() // pane exit (or an always-narrow run): the scrollback gets the blocks it never saw
	}
	m.sizeInput()
	cw := m.width
	if l.pane {
		cw = l.chat.W
	}
	var tail []string
	if l.pane {
		tail = m.paneChat(cw) // the transcript's tail, current before the live rows join it
	}
	var live []string
	m.liveTrim = 0
	if s := m.stream.String(); m.busy && strings.TrimSpace(s) != "" {
		a := m.liveAnswer(s, cw)
		if m.theme.Depth && l.pane && !m.firstPiece() && m.paneTail.answer && m.paneTail.turn == m.turnSeq && len(m.paneCache) >= 2 {
			// the paragraph being written continues the slab its turn's pieces already raised
			if _, rest, ok := strings.Cut(a, "\n"); ok {
				a, m.liveTrim = m.theme.answerSeam(cw)+"\n"+rest, 2
			}
		}
		live = m.stack(live, a)
	}
	for _, t := range m.tools {
		live = m.stack(live, m.theme.Tool(*t, cw))
	}
	for _, n := range m.subagentSeq {
		if c := m.subagents[n]; c != nil {
			// a done subagent's clock is over: its card holds the elapsed its terminal
			// event carried — recomputing it every frame kept dead subagents "running"
			if c.State != "done" {
				c.Elapsed = time.Since(m.subagentStart(c))
			}
			live = m.stack(live, m.theme.Subagent(*c, cw, m.pulseFor(c)))
		}
	}
	if m.choice == nil && m.busy {
		tokens := m.footer.Tokens
		if m.think.Len() > 0 && strings.TrimSpace(m.stream.String()) == "" {
			live = append(live, m.theme.liveThinking(m.think.String(), cw, 3), "")
		}
		live = append(live, m.statusLines(m.theme.Spinner(m.spin.View(), m.theme.Shimmer(m.verb, shimmerPhase(m.shimmer, len([]rune(m.verb)))), time.Since(m.turnStart), tokens, cw)))
	}
	f := m.footer
	f.Queued = m.queued
	f.Hint = "? keys · ctrl+k palette · ctrl+t agents"
	if m.mouseTerm && l.pane {
		f.Badge = "mouse: terminal"
	}
	f.Message = m.message
	if f.Subagents == "" {
		f.Subagents = m.words.Subagents
	}

	if l.pane {
		// an inline ask (window under the modal tripwire) belongs to the conversation pane
		if m.choice != nil && !m.choiceFloat() {
			live = m.stack(live, m.theme.Choice(m.view, cw))
		}
		if m.liveTrim == 0 && m.theme.Depth && len(live) > 0 && len(tail) >= 2 && tail[len(tail)-1] == "" &&
			isDecorRow(tail[len(tail)-2]) && isDecorRow(firstLine(live[0])) {
			m.liveTrim = 1 // a raised live card stacks on the transcript's last slab
		}
		chat := m.chatRows(live, l)
		input := strings.Split(m.input.View(), "\n")
		rail := m.railRowsAt(l)
		aside := ""
		if m.vpOK && !m.vp.AtBottom() {
			below := max(m.vp.TotalLineCount()-m.vp.YOffset()-m.vp.Height(), 0)
			aside = m.theme.accent.Render(fmt.Sprintf("↓ %d more", below))
		}
		ask, askAside := m.theme.title("ask"), ""
		switch {
		case m.choice != nil:
			ask = m.theme.dim.Render(smallCap("ask") + " · answer the card first")
		case m.busy:
			askAside = m.theme.dim.Render("enter queues for " + m.words.Session())
		}
		rows := m.frame(l, m.header(l.W-4), chat, input, rail, aside, ask, askAside, m.theme.StatusBar(f, l.W-4))
		return strings.Repeat(strings.Repeat(" ", l.W)+"\n", paneTop) + m.overlay(strings.Join(rows, "\n"))
	}

	// the inline layout: the live rows, the input box, the menu or completions under it, the
	// footer — everything finished flows into the terminal's scrollback above
	parts := live
	if m.choice != nil && !m.choiceFloat() {
		parts = append(parts, m.theme.Choice(m.view, m.width), "")
	}
	if m.shortcuts {
		parts = append(parts, m.theme.Shortcuts(DefaultShortcuts(), m.width))
	}
	if m.settings != nil {
		parts = append(parts, m.settingsView())
	}
	parts = append(parts, m.theme.InputBox(m.input.View(), m.width, m.choice != nil))
	switch {
	case m.menu != nil:
		parts = append(parts, m.theme.Menu(m.menu.items, m.menu.cursor, m.width))
	case m.comp != nil:
		parts = append(parts, m.theme.Completions(m.comp.items, m.comp.cursor, m.width))
	}
	f.Hint = "? for shortcuts · ctrl+p plans"
	parts = append(parts, m.theme.Footer(f, m.width))
	return m.overlay(strings.Join(parts, "\n"))
}

// paneTop is the headroom the panes leave above themselves: the frame still fills the window
// (rectangle-exact), but its first row stays blank. The launching command line has scrolled
// above the window by then, and a terminal that pins the running command over the viewport's
// top row (VS Code's sticky scroll, on by default with shell integration) paints it there —
// over a blank row instead of the zone worker's head and the welcome's top border.
const paneTop = 1

// frameH is the height the live frame's own rows may use: the window, less the panes'
// headroom while they own it.
func (m *Model) frameH() int {
	if m.board == nil && m.bview == nil && m.paneMode() {
		return m.height - paneTop
	}
	return m.height
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

// syncDepth turns the depth on while the frame owns the window on a truecolor terminal (the
// scrollback a narrow window prints into stays flat); a change re-renders every cached block.
func (m *Model) syncDepth() {
	on := m.theme.TrueColor && m.theme.Color && !m.quit && m.paneMode()
	if on != m.theme.Depth {
		m.theme.Depth = on
		m.renderGen++
		m.paneDirty = true
	}
}

// stack adds a block to the live rows, a blank row after it — the blank between two raised
// slabs goes: the one's shadow and the other's lit top face already part them.
func (m *Model) stack(live []string, block string) []string {
	if m.theme.Depth && len(live) >= 2 && live[len(live)-1] == "" && isDecorRow(lastLine(live[len(live)-2])) && isDecorRow(firstLine(block)) {
		live = live[:len(live)-1]
	}
	return append(live, block, "")
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

func lastLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}
