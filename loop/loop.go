// Package loop is the turn engine: a model turn is a plan; each tool call it asks for is one
// step worked to six beats — perceive → recall → plan → act → verify → record — journaled in
// the shape loop.js writes. Budgets are readings; past one the run checkpoints and stops
// honestly. Hooks let the workspace, memory, toolbox, onto and compaction packages wire in without
// this package importing them.
package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/gate"
	"github.com/Kaginari/agent-one/instrument"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/value"
	"github.com/Kaginari/agent-one/wire"
)

// Statuses and exit codes, loop.js's.
const (
	Done       = "DONE"
	Dry        = "DRY"
	Fail       = "FAIL"
	Escalate   = "ESCALATE"
	Denied     = "DENIED"
	Checkpoint = "CHECKPOINT"
	// Suspended: the turn parked its unanswered human question durably (the card is on the
	// instruments shelf) and ended cleanly — the human's word, whenever it comes, revives the
	// thread through Resume at the exact step it stopped (freeze review 2026-10-07).
	Suspended = "SUSPENDED"
)

// Exit maps a status to loop.js's exit code.
func Exit(status string) int {
	switch status {
	case Fail:
		return 2
	case Escalate:
		return 3
	case Denied:
		return 4
	case Checkpoint:
		return 5
	case Suspended:
		return 6
	}
	return 0
}

const (
	DefaultSteps   = 50
	DefaultMinutes = 30
	// DefaultSalvageTokens caps a subagent's journalise turn when the workspace's config leaves
	// policy.budget.salvageTokens at 0 — enough for @S/@F/@U/@E and nothing more.
	DefaultSalvageTokens = 4096
	DefaultRetries       = 2
	tailBytes            = 400
)

// Budget is every ceiling the engine reads before it acts. Zero takes the default; a negative
// value switches that budget off.
type Budget struct {
	Steps     int     // tool steps per turn
	Minutes   float64 // wall clock per session
	Retries   int     // consecutive failed acts before the run escalates
	MaxTokens int     // provider output ceiling per call (0: provider default)
	Context   instrument.Budget
}

func (b Budget) steps() int {
	if b.Steps == 0 {
		return DefaultSteps
	}
	return b.Steps
}
func (b Budget) minutes() float64 {
	if b.Minutes == 0 {
		return DefaultMinutes
	}
	return b.Minutes
}
func (b Budget) retries() int {
	if b.Retries == 0 {
		return DefaultRetries
	}
	if b.Retries < 0 {
		return 1 << 30
	}
	return b.Retries
}

// Recall is what the recall beat brings into a step: anchors and manifest lines, never agents.
type Recall struct {
	Anchors []string // path#section
	Tools   []string // @T lines
	Holes   []string
	Rebuilt bool
}

// dedupe drops repeats, first sighting kept: two recall sources (MergeHooks) or two hits on one
// section must not spend the prompt twice on the same anchor.
func (rc Recall) dedupe() Recall {
	rc.Anchors, rc.Tools, rc.Holes = unique(rc.Anchors), unique(rc.Tools), unique(rc.Holes)
	return rc
}

func unique(xs []string) []string {
	seen := make(map[string]bool, len(xs))
	out := xs[:0:0]
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// StepRecord is one tool step as the journal and the report see it.
type StepRecord struct {
	ID        string              `json:"id"`
	N         int                 `json:"n"`
	Tool      string              `json:"tool"`
	Input     json.RawMessage     `json:"input"`
	Class     tool.Classification `json:"-"`
	Effective string              `json:"class"`
	Gate      gate.Answer         `json:"-"`
	Status    string              `json:"status"` // done | failed | denied | refused | would-ask | would-run
	Attempts  int                 `json:"attempts"`
	Ms        int64               `json:"ms"`
	Result    tool.Result         `json:"-"`
	Wrote     []string            `json:"wrote,omitempty"`
	Recall    *Recall             `json:"-"`
}

// Hooks are the seams other packages set. Every one is optional.
type Hooks struct {
	// System builds the system prompt for a provider call; nil uses the engine's default.
	System func(s *Session) string
	// Perceive overrides the context reading (memory status's method); nil reads the last
	// provider usage.
	Perceive func(s *Session) *instrument.Context
	// Recall runs before each provider call with the current ask; anchors, not payloads.
	Recall func(ctx context.Context, s *Session, ask string) Recall
	// Record runs after each done step (memory flows up); holes it returns are reported.
	Record func(ctx context.Context, s *Session, st *StepRecord) []string
	// Answer sees a turn's final answer once it closes DONE (the forge files the orchestrator's
	// @FORGE drafts from it); holes it returns ride the result.
	Answer func(s *Session, r *Result) []string
	// EndGate runs when the model finishes a turn that wrote files: the Domain owner's gate. It returns
	// the verdict word for the report and holes; an error fails the turn.
	EndGate func(ctx context.Context, s *Session, r *Result) (verdict string, holes []string, err error)
	// GateRetries is how many times a failed end gate goes back to the model within the same
	// turn, its reasons as the message, before the turn fails (law.gate.retries).
	GateRetries int
	// Drain is the compaction seam: called when the context reading crosses the stress line,
	// before the engine checkpoints. Returning true means the context was drained and the run
	// may continue; false (or nil hook) means checkpoint.
	Drain func(ctx context.Context, s *Session, c instrument.Context) (bool, error)
	// Missing answers a call to a tool the registry does not hold (binary.md §When a tool is
	// missing): the error the model reads and whether the turn stops here (the doom-loop
	// guard). nil: "unknown tool" and never a stop.
	Missing func(ctx context.Context, s *Session, call provider.ToolCall) (content string, stop bool)
	// Guard refuses a catastrophic command before the policy and the gate: a non-empty answer is
	// the refusal, and no approval can run it. The session rides along so agent-scoped guards
	// (per-agent scratch dirs) know who is acting.
	Guard func(s *Session, tool string, input json.RawMessage) string
	// Stream sees every agent's streamed text ("text") and reasoning ("thinking") as it arrives.
	Stream func(s *Session, kind, text string)
	// Decide is the permission rule before the gate (config.Decide): allow silences the gate
	// for this act (logged as pre-approved by rule), deny refuses it, ask forces the gate even
	// for a read or a write. An empty Action leaves the class default.
	Decide func(s *Session, st *StepRecord, cls tool.Classification) Decision
	// PreTool runs before an act; a non-empty string blocks it with that reason (a hook's
	// stderr). PostTool runs after a done act.
	PreTool  func(ctx context.Context, s *Session, st *StepRecord) string
	PostTool func(ctx context.Context, s *Session, st *StepRecord)
	// Note runs after a done act; a non-empty return rides the tool result back to the agent as
	// advisory text. Advisory only — refusal is PreTool's seat. (The onto census hooks land
	// here: ledger row always, one unowned-soil line per run.)
	Note func(ctx context.Context, s *Session, st *StepRecord) string
	// State reports the agent's state for the live subagent: thinking · tool · waiting on gate ·
	// done.
	State func(s *Session, state string)
	// Observe sees every tool step twice: "start" once its class is settled (before the gate)
	// and "end" once its record is final (done, failed, denied, refused, would-ask, would-run,
	// or a missing tool). The terminal UI draws its blocks from it.
	Observe func(s *Session, st *StepRecord, phase string)
	// Inbox hands lines typed by the human mid-turn; they ride the next tool-result message.
	Inbox func(s *Session) []string
	// Budget reads the session and subagent budgets before a model call; a non-empty reason
	// checkpoints the run honestly.
	Budget func(s *Session) string
	// Ended sees every turn's result once it closes, whatever its status (the value ledger
	// folds the orchestrator's turns here).
	Ended func(s *Session, r *Result)
}

// Decision is a permission rule's word on an act.
type Decision struct {
	Action string // allow | ask | deny | ""
	Why    string // the rule and its origin
}

// Lexicon holds every user-facing word a distribution may rename.
type Lexicon struct {
	Agent        string // default agent name: "ephemeral-subagent"
	Human        string // who the gate asks: "the human"
	WorkspaceDir string // ".agent-one"
	Policy       string // one line the default system prompt opens with
}

func (l Lexicon) agent() string {
	if l.Agent == "" {
		return "ephemeral-subagent"
	}
	return l.Agent
}
func (l Lexicon) human() string {
	if l.Human == "" {
		return "the human"
	}
	return l.Human
}
func (l Lexicon) workspaceDir() string {
	if l.WorkspaceDir == "" {
		return tool.DefaultWorkspaceDir
	}
	return l.WorkspaceDir
}

// Engine is one configured loop. Every field is a switch.
type Engine struct {
	Provider       provider.Provider
	Tools          *tool.Registry
	Gate           *gate.Gate
	Root           string
	As             string
	Budget         Budget
	Hooks          Hooks
	Policy         tool.Policy
	Lexicon        Lexicon
	Journal        string    // directory; "" means <Root>/<WorkspaceDir>/instruments/loop; "-" disables
	Trace          io.Writer // step-by-step trace for the human; nil is quiet
	Unsaid         bool      // the commission asked +unsaid: a report with no @U is a hole
	UnsaidExplicit bool      // the caller named +unsaid (dispatch args or text), not the ambient policy's default — only explicit demands bounce for a missing @U
	// Guts: the per-step value sense (guts.go) — nil keeps the loop dumb. When set, every act is
	// scored, the windowed momentum tracks direction, and a slide past the floor speaks inside the
	// tool result of the act that caused it.
	Guts *Guts
	// ClosingCheck: on the first turn the agent closes with no tool call, send the ask back once
	// ("is it applied end-to-end?") — the bench-class failure this catches is an answer that
	// closes mid-plan ("let me merge…") with the merge never run. Live sessions keep it off.
	ClosingCheck bool
	// ForgeDemandAt is the crystallization question's step floor (policy.forge.demandAtSteps): a
	// turn that ran at least this many steps is asked once, at its close, to name a @FORGE draft
	// or say the path was one-off — with or without the closing check. 0 = off.
	ForgeDemandAt int
	// PlanOpen returns "" when the agent may close clean, else one line naming the open plan
	// item. Wired by the workspace to the agent's deck: a run never closes clean on an open plan —
	// the closing check's re-ask carries why, and a stubborn close is named a hole on the report.
	PlanOpen func() string
	Cap      int // @CAP on the final report; 0 means no ceiling
	IsRecord func(rel string) bool
	// ZoneOwner names the member authoring a root-relative path's zone — the file kit quotes
	// it in write/edit/delete results so every landing names its soil's persistent agent (nil-safe).
	ZoneOwner func(rel string) string
	// Receipts is the write book shared by every engine on this root (the workspace keeps one): the
	// tree watch leaves a change another run's tool reported writing to that run. nil = no book.
	Receipts *Receipts
	// Seals reads the harness's own files against their seals after every act (the workspace's
	// shared shelf): each answer names bytes the harness did not write, already set aside, and
	// the act that left them fails. nil = no check.
	Seals   func() []string
	Wire    bool              // the answer is expected on the wire (a Subagent): providers with guided decoding constrain it
	OnDelta func(text string) // streamed text, as it arrives
	// OnThinking is the model's reasoning, as it arrives (shown, never replayed from here).
	OnThinking func(text string)
	// Doom: notes-stress auto-relief (policy.budget doomLoop). > 0: when the guts' spoke streak
	// reaches it (sustained momentum-collapse readings without recovery), the run ends in a
	// CHECKPOINT carrying the relief arms (retire-with-handoff / split / configuration-shift) for the
	// dispatcher to choose. 0 = off. Guts alone nudges; Doom ends the drift.
	Doom      int
	lastRunID string // the run id of the most recent Run (a subagent circuit reads it to resume the dying session)
}

// Session is a running conversation on an Engine: the messages, the readings, the journal.
type Session struct {
	Engine        *Engine
	RunID         string
	Journal       *Journal
	Messages      []provider.Message
	Started       time.Time
	Turns         int
	Steps         int
	Spend         provider.Usage
	spendMu       sync.Mutex // Spend is also added to from a background subagent's goroutine (AddSpend)
	Last          provider.Usage
	Context       instrument.Context // the session's own goroutine reads it directly; others use Reading
	ctxMu         sync.RWMutex       // guards Context against readers on other goroutines (status line, board)
	Wrote         []string           // the writes the gate has yet to see; cleared once a turn's gate ran
	Ask           string             // the first ask of the session (the commission)
	failures      int
	base          snapshot  // the workspace tree as last stamped (snapshot.go)
	stampedAt     time.Time // when base was taken: a receipt noted after it explains a change since
	nowatch       bool      // the tree could not be stamped: shell writes go unseen, the hole named
	gated         bool      // the last turn's writes passed the end gate
	unsaidBounces int       // count of @U bounces this session — separate from wireTries (which resets per wire-correct turn)
	gateTries     int       // failed gates sent back to the model this turn
	wireTries     int       // off-the-wire answers sent back to the model this turn
	closingAsked  bool      // the closing check has been asked this session (it asks once, never twice)
	forgeAsked    bool      // the forge question has been asked this turn
	turnStepBase  int       // s.Steps as this turn opened: the forge question counts the turn's own steps
	stall         stall     // the turn's spin ledger: re-pins and turns since the last landing (stall.go)
	// the guts' readings (nil Engine.Guts: never appended, never read)
	gutsVals      []int
	gutsLastSpoke int
	gutsSpoke     int // consecutive spoke windows without recovery — feeds Engine.Doom's relief ladder
	gutsPrevKey   string
	gutsLastWhy   string               // the most recent non-empty reading — the spoke note names the run's losing ground, not the crossing act's
	gutsBench     []string             // recent act workbench keys, echo-penalty window
	gutsBenchWrot map[string]int       // bench → cumulative writes as of its last visit
	gutsBenchHits map[string]*benchHit // bench → visit tally (red/green/muted) — the thrash arm's and pulse's record
	gutsThrash    int                  // readings that priced a bench thrash this session
	gutsMomentum  int                  // the momentum at the last reading
	gutsReads     int                  // readings this session
	wroteTot      int                  // every write the workspace has landed (echo penalty's divider)
	// TestsBefore is the test files' text as the turn opened (the gate's "tests intact" check).
	TestsBefore map[string]string
}

// Result is one turn's outcome.
type Result struct {
	RunID   string             `json:"runId"`
	Status  string             `json:"@S"`
	Text    string             `json:"text"`
	Report  wire.Report        `json:"-"`
	IsWire  bool               `json:"wire"`
	Verdict string             `json:"verdict,omitempty"`
	Wrote   []string           `json:"wrote,omitempty"` // every write the end gate saw, whatever tool made it
	Steps   []*StepRecord      `json:"steps"`
	Holes   []string           `json:"@?"`
	Usage   provider.Usage     `json:"usage"`
	Context instrument.Context `json:"context"`
	Journal string             `json:"journal,omitempty"`
	Turns   int                `json:"turns"`
	// Strains is the turn's governance sent-back count: consecutive failed acts standing at
	// the end + failed-gate re-asks + off-wire re-asks. A clean turn reads 0; a struggling one
	// reads how often its note-line broke. The dispatch ledger weighs it per agent.
	Strains int `json:"strains,omitempty"`
	// GateTries is how often the end gate sent this turn back before it closed.
	GateTries int `json:"gateTries,omitempty"`
	// Interrupted: the hand was lowered mid-drive (the context ended) — the human's word, never
	// the agent's failure.
	Interrupted bool `json:"interrupted,omitempty"`
	// HoleKinds names each of Holes' kind at the close, index for index: infra (the workspace's
	// fault), harness (the discipline the harness caught). The agent's own @? are Report.Holes.
	HoleKinds []string `json:"holeKinds,omitempty"`
	// Guts is the turn's closing gut reading.
	Guts GutsSum `json:"guts,omitempty"`
}

// GutsSum is a run's gut at its close: whether it read at all, the momentum at the last reading,
// and how many readings priced a bench thrash.
type GutsSum struct {
	Read     bool `json:"read,omitempty"`
	Momentum int  `json:"momentum,omitempty"`
	Thrash   int  `json:"thrash,omitempty"`
}

// The hole kinds. Only a harness hole is the agent's: the workspace's faults and the agent's honest @?
// never cost it.
const (
	HoleInfra   = "infra"   // the provider, the drain, recall, the output ceiling, a config switch, a filing error
	HoleHarness = "harness" // the harness caught the agent's discipline: off the wire, no @U, a gate sent back
	HoleSelf    = "self"    // the agent's own @? on the wire
)

// harnessHoles are the openings of every hole the loop raises against the agent's own discipline.
var harnessHoles = []string{
	"answer off the wire", "report carries no @U line", "answer is not on the wire",
	"gate failed and was sent back", "end-of-turn gate:", "closed with an open plan",
	"+unsaid was asked and no @U came back", "consecutive failed acts", "doom loop",
}

// HoleKind classifies one hole the harness raised: harness when it names the agent's discipline,
// infra for everything else (the provider refused, a drain or recall failed, the output ceiling
// cut the answer, the gate is switched off, a note could not be filed).
func HoleKind(h string) string {
	for _, p := range harnessHoles {
		if strings.Contains(h, p) {
			return HoleHarness
		}
	}
	return HoleInfra
}

// HoleCount splits the run's holes by kind; self counts the agent's own @? lines.
func (r *Result) HoleCount() (infra, harness, self int) {
	for _, h := range r.Holes {
		if HoleKind(h) == HoleHarness {
			harness++
		} else {
			infra++
		}
	}
	if r.IsWire {
		self = len(r.Report.Holes)
	}
	return infra, harness, self
}

func (r *Result) hole(h string) {
	for _, x := range r.Holes {
		if x == h {
			return
		}
	}
	r.Holes = append(r.Holes, h)
}

// Exit is the process exit code for this result.
func (r *Result) Exit() int { return Exit(r.Status) }

func (e *Engine) as() string {
	if e.As != "" {
		return e.As
	}
	return e.Lexicon.agent()
}

func (e *Engine) env() tool.Env {
	return tool.Env{Root: e.Root, WorkspaceDir: e.Lexicon.workspaceDir(), Policy: e.Policy, IsRecord: e.IsRecord, ZoneOwner: e.ZoneOwner}
}

// JournalDir is where this engine journals; "" when journaling is off.
func (e *Engine) JournalDir() string {
	if e.Journal == "-" {
		return ""
	}
	if e.Journal != "" {
		return e.Journal
	}
	return filepath.Join(e.Root, e.Lexicon.workspaceDir(), "instruments", "loop")
}

func newRunID(as string) string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, as)
	return fmt.Sprintf("%s-%s-%s", safe, time.Now().UTC().Format("20060102150405"), hex.EncodeToString(b))
}

// NewSession opens a session: a run id, a journal (unless off or dry), an empty conversation.
func (e *Engine) NewSession() *Session {
	s := &Session{Engine: e, RunID: newRunID(e.as()), Started: time.Now()}
	s.setContext(e.Budget.Context.Unread("no provider call yet"))
	if dir := e.JournalDir(); dir != "" && !(e.Gate != nil && e.Gate.DryRun) {
		s.Journal = &Journal{Path: filepath.Join(dir, s.RunID+".jsonl")}
	}
	return s
}

// Run is the one-shot form: a fresh session, one turn.
func (e *Engine) Run(ctx context.Context, ask string) (*Result, error) {
	s := e.NewSession()
	e.lastRunID = s.RunID
	return s.Turn(ctx, ask)
}

// LastRunID is the run id of the most recent Run — a subagent-side circuit needs it to resume
// the session it is about to lose (the journalise turn before the clock's grace ends).
func (e *Engine) LastRunID() string { return e.lastRunID }

func (s *Session) trace(format string, a ...interface{}) {
	if s.Engine.Trace != nil {
		fmt.Fprintf(s.Engine.Trace, format+"\n", a...)
	}
}

func tail(str string) string {
	if len(str) > tailBytes {
		return "…" + str[len(str)-tailBytes:]
	}
	return str
}

func (s *Session) perceive() instrument.Context {
	var hooked *instrument.Context
	if s.Engine.Hooks.Perceive != nil {
		hooked = s.Engine.Hooks.Perceive(s)
	}
	var c instrument.Context
	switch {
	case hooked != nil:
		c = *hooked
	case s.Turns == 0:
		c = s.Engine.Budget.Context.Unread("no provider call yet")
	default:
		c = s.Engine.Budget.Context.Reading(s.Last)
	}
	s.setContext(c)
	return c
}

func (s *Session) setContext(c instrument.Context) {
	s.ctxMu.Lock()
	s.Context = c
	s.ctxMu.Unlock()
}

// Reading is the session's context occupancy, safe from any goroutine.
func (s *Session) Reading() instrument.Context {
	s.ctxMu.RLock()
	defer s.ctxMu.RUnlock()
	return s.Context
}

func (s *Session) overWallClock() (bool, string) {
	m := s.Engine.Budget.minutes()
	if m < 0 {
		return false, ""
	}
	if el := time.Since(s.Started); el >= time.Duration(m*float64(time.Minute)) {
		return true, fmt.Sprintf("wall-clock budget reached (%g min)", m)
	}
	return false, ""
}

// DefaultSystem is the system prompt when no hook builds one: who the agent is, where it
// stands, the four classes and the gate, the wire it answers in.
func (e *Engine) DefaultSystem(s *Session) string {
	var b strings.Builder
	if e.Lexicon.Policy != "" {
		b.WriteString(e.Lexicon.Policy + "\n\n")
	}
	fmt.Fprintf(&b, "You are %s, an agent working inside the workspace rooted at %s. Ownership is inward: every path is relative to that root.\n", e.as(), e.Root)
	b.WriteString("Every tool call is one step with a class — read, write (inside the workspace), outward (beyond it), destructive (irreversible). Outward and destructive steps are put to " + e.Lexicon.human() + " at a gate before they run; a denial ends the turn. Declare `class` on bash when you know a command reaches further than it looks; a declaration can only tighten.\n")
	b.WriteString("Sweeping a package: `skim <dir-or-file>` draws its outline before any reads (types, funcs, line anchors); `read`'s `paths` batches windows into one call. Reach for them before a read sweep — a grep-then-read dance through one package is a five-commission waste tonight's ghosts already paid for once.\n")
	b.WriteString("Budgets are readings: steps, wall clock and the context window are checked before every step; past one, the run checkpoints and stops.\n")
	if e.Guts != nil {
		b.WriteString("The guts are a reading too: every act is scored, and when the run slides they say so inside a tool result — when they speak, change course; when they are silent, proceed.\n")
	}
	if e.Wire {
		b.WriteString("Answer on the wire: `@S <status>` opens, one `@F <file:line> <fact>` per finding, `@? <hole>` for anything you could not settle (never guess), `@U <policy|team|ownership> <text>` for each thing you know that nothing on disk says, `@E <bytes>` closes. No greetings, nothing long — a long thing lives on disk and crosses as a path.\n")
		// a run that proved a repeating need names its draft: @FORGE rides the report up, the
		// harness files it, the working notes (the human's `/forge register` or `agent-one forge register`) lands it —
		// a skill becomes <workspace>/crafted-skills/<name>/SKILL.md, a tool a tools.custom patch;
		// drafts never register themselves mid-turn
		b.WriteString("When the run proved a repeating need, name its draft once: `@FORGE skill:<name> | <when: the triggers> | <how: the steps>` (name in [a-z0-9-]), or `@FORGE tool:<name> | <summary> | <argv template>` (holes as `{{param}}`; an argv, never a shell pipeline).\n")
		// an agent that may dispatch may also order: @$ names work for the layer below, drained
		// by the Subagent after the turn (never registered by the agent mid-turn — the wire carries
		// the order, the harness enforces the chain); a report in hand travels as plain prose.
		if e.Tools != nil {
			for _, n := range e.Tools.Names() {
				if n == "dispatch" {
					b.WriteString("Order work one layer down with `@$ <body>: <ask>` — one line per order; it is held on the workspace's shared queue until the wake after your report claims it, re-checked against the hierarchy.\n")
					break
				}
			}
		}
	} else {
		b.WriteString("You speak to " + e.Lexicon.human() + " directly: answer in plain language, in full sentences — what you did, what you found, and anything you could not settle. The wire (`@S … @E`) is for the reports of bodies you dispatch, never for the human.\n")
	}
	if e.Tools != nil {
		fmt.Fprintf(&b, "Tools: %s.\n", strings.Join(e.Tools.Names(), ", "))
		named := false
		for _, n := range e.Tools.Names() {
			if n == "dispatch" {
				named = true
				break
			}
		}
		if named {
			b.WriteString("When the ask names a Subagent agent (a member on the roster) and hands it work — \"dispatch <body> …\", \"have <body> …\", \"<body>, look at …\" — call the `dispatch` tool for it; never do that work in the agent's place.\n")
		}
	}
	return b.String()
}

func (s *Session) system() string {
	if s.Engine.Hooks.System != nil {
		return s.Engine.Hooks.System(s)
	}
	return s.Engine.DefaultSystem(s)
}

func (s *Session) defs() []provider.ToolDef {
	if s.Engine.Tools == nil {
		return nil
	}
	var out []provider.ToolDef
	for _, d := range s.Engine.Tools.Defs() {
		out = append(out, provider.ToolDef{Name: d.Name, Description: d.Description, Schema: d.Schema, Declare: d.Declare})
	}
	return out
}

// Turn runs one ask to its end: the model plans, each tool call is a step, the model answers.
func (s *Session) Turn(ctx context.Context, ask string) (r *Result, err error) {
	// a panic in a hook, a stream callback or a tool shell down-stack becomes a failed turn,
	// never a dead process: the caller's recover (tui.go, dispatch.go) still runs and the
	// session's journal carries the stack
	defer func() {
		if rec := recover(); rec != nil {
			s.Journal.Log(Event{"t": "panic", "n": s.Turns, "panic": fmt.Sprint(rec), "stack": string(debug.Stack())})
			s.saveTranscript()
			s.state("panic")
			r = &Result{RunID: s.RunID, Journal: journalRel(s), Status: Fail,
				Holes: []string{fmt.Sprintf("panic: %v", rec)}}
			err = nil
		}
	}()
	if strings.TrimSpace(ask) == "" {
		return nil, fmt.Errorf("loop: empty ask")
	}
	s.open(ask)
	s.forgeAsked, s.turnStepBase = false, s.Steps
	if s.gated {
		// the last turn's writes met the gate; this turn gates only its own
		s.Wrote, s.gated = nil, false
	}
	if n := len(s.Messages); n > 0 && s.Messages[n-1].Role == provider.User {
		// the last turn stopped on its tool results (denied, escalated, checkpointed): the ask
		// rides that message — two user messages in a row is a shape no provider takes
		s.Messages[n-1].Text = strings.TrimSpace(s.Messages[n-1].Text + "\n" + ask)
	} else {
		s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: ask})
	}
	return s.drive(ctx)
}

func (s *Session) open(ask string) {
	e := s.Engine
	if s.Ask != "" {
		return
	}
	s.Ask = ask
	g := s.gate()
	s.Journal.Log(Event{"t": "run", "id": s.RunID, "as": e.as(), "ask": ask, "provider": e.Provider.Name(), "root": e.Root,
		"budget":  map[string]interface{}{"steps": e.Budget.steps(), "minutes": e.Budget.minutes(), "retries": e.Budget.retries(), "context": e.Budget.Context.Unread("").Limit, "stress": e.Budget.Context.Unread("").Stress},
		"approve": approveList(g), "strict": g.Strict, "dryRun": g.DryRun, "tools": e.toolNames()})
}

func (s *Session) gate() *gate.Gate {
	if s.Engine.Gate == nil {
		s.Engine.Gate = gate.New()
	}
	return s.Engine.Gate
}

// drive runs the conversation from its current state (ending on a user message) to the end
// of the turn.
func (s *Session) drive(ctx context.Context) (*Result, error) {
	e := s.Engine
	if e.Provider == nil {
		return nil, fmt.Errorf("loop: no provider")
	}
	g := s.gate()
	r := &Result{RunID: s.RunID, Journal: journalRel(s), Holes: []string{}, Steps: []*StepRecord{}}
	s.watch(r) // the turn's baseline: every write from here on is seen, whatever tool makes it
	defer s.Engine.Receipts.unwatch(s)
	stepsThisTurn := 0
	s.gateTries = 0
	s.wireTries = 0
	s.gutsThrash, s.gutsReads = 0, 0 // the turn's gut summary is the turn's own
	s.stall = stall{}                // a new ask is new ground: spin is counted from here
	end := func(status string, why string) (*Result, error) {
		r.Status = status
		if why != "" {
			r.hole(why)
		}
		r.Usage, r.Context, r.Turns = s.SpendNow(), s.Context, s.Turns
		r.Strains = s.failures + s.gateTries + s.wireTries
		r.GateTries = s.gateTries
		r.Interrupted = ctx.Err() != nil
		if e.Guts != nil {
			r.Guts = GutsSum{Read: s.gutsReads > 0, Momentum: s.gutsMomentum, Thrash: s.gutsThrash}
		}
		if status == Done && e.Hooks.Answer != nil {
			for _, h := range e.Hooks.Answer(s, r) {
				r.hole(h)
			}
		}
		if status != Done && status != Dry {
			// a turn stopped short of finish still owns what it changed on disk
			for _, w := range s.seen() {
				s.Wrote = appendUnique(s.Wrote, w)
			}
			s.saveTranscript()
		}
		ev := Event{"t": "end", "status": status, "turns": s.Turns, "steps": s.Steps, "ms": time.Since(s.Started).Milliseconds(), "holes": r.Holes}
		// a turn that ended because the hand was lowered mid-drive is not a DONE the work earned:
		// the cause names the one who lowered it — the lexicon's word for the human (Operator 2026-10-03)
		if ctx.Err() != nil {
			ev["cause"] = e.Lexicon.human()
		}
		r.HoleKinds = r.HoleKinds[:0]
		for _, h := range r.Holes {
			r.HoleKinds = append(r.HoleKinds, HoleKind(h))
		}
		ev["holeKinds"] = r.HoleKinds
		// the run's one word (policy.value): what the leaderboard and the activity count by
		word := ""
		if r.IsWire {
			word = r.Report.Status
		}
		ev["word"] = value.RunWord(status, word)
		if ctx.Err() != nil {
			ev["word"] = "INTERRUPTED"
		}
		s.Journal.Log(ev)
		if e.Hooks.Ended != nil {
			e.Hooks.Ended(s, r)
		}
		s.state("done")
		return r, nil
	}
	resumeHint := func() string { return "resume: agent-one resume " + s.RunID }

	pauses := 0
	for {
		// PERCEIVE (the turn): budgets before the provider call
		if over, why := s.overWallClock(); over {
			return end(Checkpoint, why+" before the next model call — "+resumeHint())
		}
		if e.Hooks.Budget != nil {
			if why := e.Hooks.Budget(s); why != "" {
				s.Journal.Log(Event{"t": "budget", "why": why, "before": "turn"})
				return end(Checkpoint, why+" — "+resumeHint())
			}
		}
		if why := s.budgetSpent(); why != "" {
			s.Journal.Log(Event{"t": "budget", "why": "actBudget", "idle": s.stall.idle, "before": "turn"})
			return end(Checkpoint, why)
		}
		c := s.perceive()
		if c.Stressed() {
			s.Journal.Log(Event{"t": "budget", "context": c, "before": "turn"})
			drained := false
			if e.Hooks.Drain != nil {
				ok, err := e.Hooks.Drain(ctx, s, c)
				if err != nil {
					r.hole("drain failed: " + err.Error())
				}
				drained = ok
				s.Journal.Log(Event{"t": "drain", "ok": ok, "before": c.Tokens, "after": s.perceive().Tokens})
			}
			if !drained {
				return end(Checkpoint, fmt.Sprintf("context in the stress zone (%d ≥ %d tok) — write what is not yet durable (/handoff writes one), then resume in a fresh session: %s", c.Tokens, c.Stress, resumeHint()))
			}
		}
		// RECALL (the turn): anchors for the current ask, never payloads
		var rc Recall
		if e.Hooks.Recall != nil {
			rc = e.Hooks.Recall(ctx, s, currentAsk(s)).dedupe()
			s.Journal.Log(Event{"t": "recall", "id": fmt.Sprintf("t%d", s.Turns+1), "q": currentAsk(s), "rebuilt": rc.Rebuilt, "hits": rc.Anchors, "tools": rc.Tools, "holes": rc.Holes})
			for _, h := range rc.Holes {
				r.hole("recall: " + h)
			}
		}
		onDelta, onThink := e.OnDelta, e.OnThinking
		if h := e.Hooks.Stream; h != nil {
			od, ot := onDelta, onThink
			onDelta = func(t string) {
				if od != nil {
					od(t)
				}
				h(s, "text", t)
			}
			onThink = func(t string) {
				if ot != nil {
					ot(t)
				}
				h(s, "thinking", t)
			}
		}
		// the wire's copy is the compacted one; the session keeps the whole conversation.
		// a subagent's fan-out grows input-token cost against context replay — this fold +
		// thinking-drop keeps the growth sub-linear in turn count (see compact.go)
		wire := compactReplay(s.Messages)
		req := provider.Request{System: s.system(), Messages: wire, Tools: s.defs(), MaxTokens: e.Budget.MaxTokens, Wire: e.Wire, OnDelta: onDelta, OnThinking: onThink}
		if len(rc.Anchors)+len(rc.Tools) > 0 {
			req.SystemTail = recallBlock(rc)
		}
		// PLAN: the model's turn
		s.state("thinking")
		resp, err := e.Provider.Complete(ctx, req)
		s.Turns++
		if err != nil {
			s.Journal.Log(Event{"t": "turn", "n": s.Turns, "error": err.Error()})
			return end(Fail, "provider: "+err.Error())
		}
		s.Last = resp.Usage
		s.AddSpend(resp.Usage)
		s.perceive() // the reading the loop acts on (a Perceive hook may scale the stress line) is the one reported
		s.Journal.Log(Event{"t": "turn", "n": s.Turns, "model": resp.Model, "stop": string(resp.Stop), "usage": resp.Usage, "context": s.Context, "calls": len(resp.Message.ToolCalls)})
		s.Messages = append(s.Messages, resp.Message)
		if resp.Message.Text != "" && len(resp.Message.ToolCalls) > 0 {
			s.trace("  %s", strings.TrimSpace(resp.Message.Text))
		}
		if len(resp.Message.ToolCalls) == 0 {
			if resp.Stop == provider.StopPause && pauses < 3 {
				// the server paused a long turn: resend to continue, a bounded number of times
				pauses++
				s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: "continue"})
				continue
			}
			res, err := s.finish(ctx, r, resp, end)
			if err == errGateRetry {
				continue // the gate's reasons are the model's next message
			}
			if err == errWireRetry {
				continue // the wire violation's wording is the model's next message
			}
			return res, err
		}
		// each tool call is a step
		s.spin()
		var results []provider.ToolResult
		stop, stopWhy := "", ""
		for _, call := range resp.Message.ToolCalls {
			if stop != "" {
				results = append(results, provider.ToolResult{ID: call.ID, Content: "not run: the turn stopped (" + stop + ")", IsError: true})
				continue
			}
			st, res, status, why := s.step(ctx, g, call, r, stepsThisTurn)
			results = append(results, res)
			r.Steps = append(r.Steps, st)
			if st.Status == "done" || st.Status == "failed" || st.Status == "refused" {
				stepsThisTurn++
			}
			if status != "" {
				stop, stopWhy = status, why
			}
		}
		next := provider.Message{Role: provider.User, ToolResults: results}
		var lines []string
		if e.Hooks.Inbox != nil {
			if lines = e.Hooks.Inbox(s); len(lines) > 0 {
				s.Journal.Log(Event{"t": "inbox", "n": len(lines)})
			}
		}
		if warn := s.idleWarning(); warn != "" {
			lines = append(lines, warn)
		}
		next.Text = strings.Join(lines, "\n")
		s.Messages = append(s.Messages, next)
		if stop != "" {
			return end(stop, stopWhy)
		}
	}
}

func (s *Session) state(st string) {
	if s.Engine.Hooks.State != nil {
		s.Engine.Hooks.State(s, st)
	}
}

func (s *Session) observe(st *StepRecord, phase string) {
	if s.Engine.Hooks.Observe != nil {
		s.Engine.Hooks.Observe(s, st, phase)
	}
}

// step works one tool call through the six beats. It returns the record, the result for the
// model, and a stop status when the turn must end here.
func (s *Session) step(ctx context.Context, g *gate.Gate, call provider.ToolCall, r *Result, stepsThisTurn int) (*StepRecord, provider.ToolResult, string, string) {
	e := s.Engine
	s.Steps++
	st := &StepRecord{ID: fmt.Sprintf("s%d", s.Steps), N: s.Steps, Tool: call.Name, Input: call.Input, Status: "pending"}
	deny := func(msg string) provider.ToolResult {
		st.Result = tool.Result{Output: msg, Err: true} // the record keeps what the model read
		s.observe(st, "end")
		return provider.ToolResult{ID: call.ID, Content: msg, IsError: true}
	}
	// PERCEIVE: budgets before the step
	if max := e.Budget.steps(); max > 0 && stepsThisTurn >= max {
		st.Status = "pending"
		return st, deny("not run: step budget reached"), Checkpoint, fmt.Sprintf("step budget reached (%d) before %s — resume: agent-one resume %s", max, st.ID, s.RunID)
	}
	if over, why := s.overWallClock(); over {
		return st, deny("not run: " + why), Checkpoint, why + " before " + st.ID
	}
	s.Journal.Log(Event{"t": "perceive", "id": st.ID, "steps": stepsThisTurn, "ms": time.Since(s.Started).Milliseconds(), "context": s.Context})
	t, ok := e.Tools.Get(call.Name)
	raw, native := malformedInput(call.Input)
	if !ok || native {
		st.Status = "failed"
		msg, stop := fmt.Sprintf("unknown tool %q", call.Name), false
		switch {
		case native:
			msg = fmt.Sprintf("malformed tool call: the arguments to %s are not valid JSON (%s) — call it again with a whole JSON object", call.Name, clipRaw(raw))
		case call.Name == "_malformed":
			var f struct{ Fault, Raw string }
			_ = json.Unmarshal(call.Input, &f)
			msg = "malformed tool call: " + f.Fault + " — write one <tool_call>{\"name\", \"input\"}</tool_call> per call"
		case e.Hooks.Missing != nil:
			msg, stop = e.Hooks.Missing(ctx, s, call)
		}
		why := "missing tool"
		if native || call.Name == "_malformed" {
			why = "malformed"
		}
		s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "failed", "attempts": 0, "why": why, "msg": msg})
		s.trace("✗ %s %s %s: %s", st.ID, call.Name, why, lastLine(msg))
		if stop {
			return st, deny(msg), Escalate, msg
		}
		return st, deny(msg), "", ""
	}
	// PLAN: the class, then the gate
	env := e.env()
	env.Self = s.Pulse // the pulse tool reads the acting session's own state through the env
	env.RunID = s.RunID
	cls, holes := t.Settle(env, call.Input)
	st.Class, st.Effective = cls, cls.Class.String()
	for _, h := range holes {
		r.hole(st.ID + ": " + h)
	}
	s.observe(st, "start")
	if e.Hooks.Guard != nil {
		if why := e.Hooks.Guard(s, t.Name, call.Input); why != "" {
			st.Status = "refused"
			s.Journal.Log(Event{"t": "gate", "id": st.ID, "tool": t.Name, "effective": st.Effective, "why": why, "needed": true, "decision": "refused", "by": "guard"})
			s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "refused", "attempts": 0})
			s.trace("✗ %s %s refused by the guard", st.ID, t.Name)
			return st, deny(why), "", ""
		}
	}
	if env.Policy != nil {
		if err := env.Policy(tool.Access{Tool: t.Name, Class: cls.Class, Paths: cls.Paths, Targets: cls.Targets, Input: call.Input}); err != nil {
			st.Status = "refused"
			s.Journal.Log(Event{"t": "gate", "id": st.ID, "tool": t.Name, "effective": st.Effective, "why": cls.Why, "needed": true, "decision": "refused", "by": "policy"})
			s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "refused", "attempts": 0})
			s.trace("✗ %s %s [%s] refused: %v", st.ID, t.Name, st.Effective, err)
			return st, deny("refused: " + err.Error()), "", ""
		}
	}
	dry := g.DryRun
	greq := gate.Request{ID: st.ID, Tool: t.Name, Class: cls.Class, Why: cls.Why, Summary: summary(call), Agent: e.as(), Input: call.Input}
	var ans gate.Answer
	rule := Decision{}
	if e.Hooks.Decide != nil {
		rule = e.Hooks.Decide(s, st, cls)
	}
	switch rule.Action {
	case "deny":
		st.Status = "refused"
		s.Journal.Log(Event{"t": "gate", "id": st.ID, "tool": t.Name, "effective": st.Effective, "why": rule.Why, "needed": true, "decision": "refused", "by": "rule"})
		s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "refused", "attempts": 0})
		s.trace("✗ %s %s [%s] denied by rule (%s)", st.ID, t.Name, st.Effective, rule.Why)
		return st, deny("denied by permission rule: " + rule.Why), "", ""
	case "allow":
		ans = gate.Answer{Needed: g.Needs(cls.Class), Decision: gate.Approved, By: "rule", Why: rule.Why}
		if !ans.Needed {
			ans.Decision = gate.NotNeeded
		}
		if dry && cls.Class > tool.Read {
			ans.Decision, ans.By = gate.WouldAsk, "dry-run"
		}
	case "ask":
		greq.Force = true
		fallthrough
	default:
		if g.Needs(cls.Class) || greq.Force {
			s.state("waiting on gate")
		}
		ans = g.Ask(greq)
	}
	st.Gate = ans
	s.Journal.Log(Event{"t": "gate", "id": st.ID, "tool": t.Name, "heuristic": st.Effective, "effective": st.Effective, "why": cls.Why, "needed": ans.Needed, "decision": string(ans.Decision), "by": ans.By, "rule": rule.Why})
	switch ans.Decision {
	case gate.Suspended:
		// the human's word didn't come: the card goes durable and the turn parks — Resume
		// carries it back (this is not a denial: nothing was refused, the question waits)
		st.Status = "denied"
		s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "denied", "attempts": 0, "suspend": true})
		s.trace("○ %s %s [%s] suspended — %s", st.ID, t.Name, st.Effective, ans.Why)
		return st, deny("suspended pending the human's word: " + ans.Why + " — resume: agent-one resume " + s.RunID), Suspended, ans.Why
	case gate.Denied:
		st.Status = "denied"
		s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "denied", "attempts": 0})
		s.trace("✗ %s %s [%s] denied at the gate (%s)", st.ID, t.Name, st.Effective, ans.Why)
		return st, deny("denied at the gate: " + ans.Why), Denied, fmt.Sprintf("%s denied at the gate (%s: %s) — %s decides", st.ID, st.Effective, ans.Why, e.Lexicon.human())
	case gate.WouldAsk:
		st.Status = "would-ask"
		s.trace("? %s %s [%s] would ask (%s)", st.ID, t.Name, st.Effective, cls.Why)
		return st, deny(fmt.Sprintf("not run (dry run): would ask %s — %s: %s", e.Lexicon.human(), st.Effective, cls.Why)), "", ""
	}
	if dry && cls.Class > tool.Read {
		st.Status = "would-run"
		s.trace("· %s %s [%s] would run", st.ID, t.Name, st.Effective)
		return st, deny(fmt.Sprintf("not run (dry run): would run — %s: %s", st.Effective, cls.Why)), "", ""
	}
	// ACT
	if e.Hooks.PreTool != nil {
		if why := e.Hooks.PreTool(ctx, s, st); why != "" {
			st.Status = "refused"
			s.Journal.Log(Event{"t": "hook", "id": st.ID, "kind": "preTool", "blocked": true, "why": why})
			s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "refused", "attempts": 0})
			s.trace("✗ %s %s [%s] blocked by a preTool hook: %s", st.ID, t.Name, st.Effective, lastLine(why))
			return st, deny("blocked by a preTool hook: " + why), "", ""
		}
	}
	s.trace("→ %s %s [%s] %s", st.ID, t.Name, st.Effective, summary(call))
	st.Attempts = 1
	s.Journal.Log(Event{"t": "act", "id": st.ID, "attempt": 1, "start": true, "tool": t.Name, "summary": summary(call)})
	s.state("tool")
	t0 := time.Now()
	res := t.Run(ctx, env, call.Input)
	st.Ms = time.Since(t0).Milliseconds()
	if e.Seals != nil {
		// whatever tool it was, an act that left bytes on harness soil the harness did not
		// write (a script, an interpreter, a path the classifier could not read) fails
		if found := e.Seals(); len(found) > 0 {
			res.Err = true
			res.Output = strings.TrimRight(res.Output, "\n") + "\nrefused after the act: " + strings.Join(found, "; ")
			r.hole(st.ID + ": " + strings.Join(found, "; "))
		}
	}
	st.Result = res
	if e.Hooks.PostTool != nil {
		e.Hooks.PostTool(ctx, s, st)
	}
	s.Journal.Log(Event{"t": "act", "id": st.ID, "attempt": 1, "ok": !res.Err, "ms": st.Ms, "out": tail(res.Output), "wrote": res.Wrote})
	// VERIFY: the tool's own reading — an error result is a failed act
	s.Journal.Log(Event{"t": "verify", "id": st.ID, "attempt": 1, "code": map[bool]int{false: 0, true: 1}[res.Err]})
	pinNote, pinWhy := "", ""
	if !res.Err {
		pinNote, pinWhy = s.pinned(t.Name, cls.Class, call.Input, res, st.ID)
	}
	gutsNote := pinNote + s.gutsNote(st, call, res)
	if (len(res.Wrote) > 0 || cls.Class >= tool.Write) && !res.Err {
		s.gutsSpoke = 0 // a landed write is the opposite of thrash — streak resets
		s.landed(t.Name, max(cls.Class, tool.Write))
	}
	if e.Hooks.Note != nil {
		gutsNote += e.Hooks.Note(ctx, s, st)
	}
	if e.Doom > 0 && s.gutsSpoke >= e.Doom {
		why := fmt.Sprintf("notes-stress relief (doomLoop, streak %d ≥ %d): the guts kept speaking through %d windows with no recovery — auto-checkpoint, not another burn. Arms for the dispatcher: retire (handoff the durables, re-authorize the remnant), split (re-divide the slice into two fresh agents), configuration-shift (re-spawn on the lane's fallback model). journal %s",
			s.gutsSpoke, e.Doom, s.gutsSpoke, journalRel(s))
		s.Journal.Log(Event{"t": "budget", "why": "doomLoop:guts", "streak": s.gutsSpoke, "threshold": e.Doom})
		s.trace("☠ doomLoop: %s", why)
		// the relief must relieve: the streak and window that fired this doom are spent — without
		// the reset the session carries an armed doom into the next turn, and any later guts at
		// floor instant-checkpoints it (the pre-armed doom caught live 2026-10-03: a wake-run
		// died on its first act because the streak of the checkpointed turn before it stood at 3)
		s.gutsSpoke, s.gutsVals = 0, nil
		content := res.Output
		if content == "" {
			content = "(no output)"
		}
		return st, provider.ToolResult{ID: call.ID, Content: content + gutsNote + "\n[doomLoop: relief ladder — this run is being checkpointed; the parent receives the split/retire/configuration-shift arms]", IsError: res.Err}, Checkpoint, why
	}
	for _, w := range res.Wrote {
		st.Wrote = append(st.Wrote, env.Rel(w))
		s.Wrote = appendUnique(s.Wrote, env.Rel(w))
		s.wroteTot++
		e.Receipts.note(s.RunID, e.Root, env.Rel(w))
	}
	if cls.Class >= tool.Write {
		// what the act changed on disk, whether or not the tool named it (the shell never does)
		for _, w := range s.seen() {
			st.Wrote = appendUnique(st.Wrote, w)
			s.Wrote = appendUnique(s.Wrote, w)
			s.wroteTot++
		}
	}
	// RECORD
	if res.Err {
		st.Status = "failed"
		s.failures++
		s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "failed", "attempts": 1, "ms": st.Ms, "last": tail(res.Output)})
		s.trace("  failed (%dms): %s", st.Ms, lastLine(res.Output))
		s.observe(st, "end")
		if s.failures > e.Budget.retries() {
			return st, provider.ToolResult{ID: call.ID, Content: res.Output + gutsNote, IsError: true}, Escalate,
				fmt.Sprintf("%d consecutive failed acts, last %s (%s) — one hop up: the dispatcher decides; journal %s", s.failures, st.ID, lastLine(res.Output), journalRel(s))
		}
		return st, provider.ToolResult{ID: call.ID, Content: res.Output + gutsNote, IsError: true}, "", ""
	}
	s.failures = 0
	st.Status = "done"
	if e.Hooks.Record != nil {
		for _, h := range e.Hooks.Record(ctx, s, st) {
			r.hole(st.ID + " record: " + h)
		}
	}
	s.Journal.Log(Event{"t": "record", "id": st.ID, "status": "done", "attempts": 1, "ms": st.Ms, "wrote": st.Wrote})
	s.trace("  ok (%dms)%s", st.Ms, map[bool]string{true: " wrote " + strings.Join(st.Wrote, " "), false: ""}[len(st.Wrote) > 0])
	s.observe(st, "end")
	out := res.Output
	if out == "" {
		out = "(no output)"
	}
	if pinWhy != "" {
		s.Journal.Log(Event{"t": "budget", "id": st.ID, "why": "repin", "tool": t.Name})
		s.trace("○ %s %s re-pinned — checkpoint", st.ID, t.Name)
		return st, provider.ToolResult{ID: call.ID, Content: out + gutsNote}, Checkpoint, pinWhy
	}
	return st, provider.ToolResult{ID: call.ID, Content: out + gutsNote}, "", ""
}

// errGateRetry tells drive that finish sent a failed gate back to the model.
var errGateRetry = errors.New("gate retry")

// errWireRetry tells drive that finish sent an off-the-wire answer back to the model. Bounded
// to one re-ask — the policy is pulled, not begged in a loop.
var errWireRetry = errors.New("wire retry")

const wireRetries = 1

func (s *Session) finish(ctx context.Context, r *Result, resp provider.Response, end func(string, string) (*Result, error)) (*Result, error) {
	e := s.Engine
	r.Text = resp.Message.Text
	if resp.Stop == provider.StopMaxTokens {
		r.hole("the model's answer was cut by the output ceiling (max_tokens)")
	}
	if resp.Stop == provider.StopRefusal {
		r.hole("the provider refused the request")
	}
	rep, isWire := wire.ParseReport(r.Text)
	r.Report, r.IsWire = rep, isWire
	// The wire is policy for a dispatched agent, coded not begged: an answer that never opened @S is
	// sent back once, exactly as a failed gate is. The violation's wording is the model's next
	// message; one re-ask, then the agent stands undisciplined and the hole is the record.
	if !isWire && e.Wire && s.wireTries < wireRetries {
		s.wireTries++
		r.Holes = append(r.Holes, fmt.Sprintf("answer off the wire — sent back (%d of %d)", s.wireTries, wireRetries))
		s.Journal.Log(Event{"t": "wire", "id": "turn", "kind": "retry", "try": s.wireTries})
		s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: fmt.Sprintf(
			"Your answer was not on the wire (%d of %d sent back): it never opened `@S <status>`. A dispatched agent answers only on the wire — `@S <status>` opens, one `@F <file:line> <fact>` per finding, `@? <hole>` for what you could not settle, `@U <policy|team|ownership> <text>` for what nothing on disk says, `@E <bytes>` closes. Answer the same ask again, on the wire.", s.wireTries, wireRetries)})
		return nil, errWireRetry
	}
	if isWire {
		s.wireTries = 0 // the agent answered on the wire; a later turn's slip is its own
	}
	if isWire && e.Unsaid && len(rep.Unsaid) == 0 {
		// +unsaid owes the @U line (requireUnsaid:true cables it into every commission). One bounce to
		// land it — same medicine as the missing-@S retry, bound to ephemeral subagents (e.Wire). OWN counter
		// (unsaidBounces): wireTries is zeroed by the isWire reset above.
		if e.Wire && s.unsaidBounces < wireRetries {
			s.unsaidBounces++
			r.Holes = append(r.Holes, fmt.Sprintf("report carries no @U line — sent back (%d of %d)", s.unsaidBounces, wireRetries))
			s.Journal.Log(Event{"t": "wire", "id": "turn", "kind": "unsaid-retry", "try": s.unsaidBounces})
			s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: fmt.Sprintf(
				"Your report answered on the wire but the `+unsaid` ask you carried forces an `@U` line, and yours has none (%d of %d sent back). Re-answer the same ask with the full wire shape — `@S` opens, `@F` findings, `@?` holes, `@U <policy|team|ownership> <text>` for each thing you know that nothing on disk says (answer `@U nothing unsaid` when there genuinely is none), `@E <bytes>` closes.", s.unsaidBounces, wireRetries)})
			return nil, errWireRetry
		}
		r.hole("report carries no @U line — nothing unsaid, or the duty failed (the dispatcher may ask)")
	}
	if !isWire && e.Unsaid {
		r.hole("answer is not on the wire (no @S)")
	}
	// a write a read-classified command slipped in (`xargs touch`) is still a write
	if late := s.seen(); len(late) > 0 {
		s.Journal.Log(Event{"t": "watch", "id": "turn", "wrote": late})
		for _, w := range late {
			s.Wrote = appendUnique(s.Wrote, w)
		}
	}
	if e.Hooks.EndGate != nil && len(s.Wrote) > 0 {
		s.state("gating")
		before := len(r.Holes)
		verdict, holes, err := e.Hooks.EndGate(ctx, s, r)
		r.Verdict = verdict
		r.Wrote = append([]string(nil), s.Wrote...) // what the gate saw (the hook may have narrowed it)
		// a gate cut short by the run's end judged nothing: its writes carry into the next
		// turn's gate instead of being dropped unjudged
		s.gated = err == nil || ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
		for _, h := range holes {
			r.hole(h)
		}
		s.Journal.Log(Event{"t": "gate", "id": "turn", "kind": "end", "verdict": verdict, "wrote": s.Wrote, "holes": holes, "error": errString(err)})
		if err != nil && s.gateTries < e.Hooks.GateRetries && ctx.Err() == nil {
			// the gate's reasons go back to the model once more: it fixes what they name, and
			// the whole turn's writes are gated again — never after the run's context ended
			s.gateTries++
			r.Holes = append(r.Holes[:before], fmt.Sprintf("gate failed and was sent back (%d of %d): %s", s.gateTries, e.Hooks.GateRetries, verdict))
			s.Journal.Log(Event{"t": "gate", "id": "turn", "kind": "retry", "try": s.gateTries, "verdict": verdict})
			s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: fmt.Sprintf(
				"The gate failed this turn's writes (%d of %d sent back):\n%s\nFix what it names in the same turn — the writes stand as they are — then answer again.", s.gateTries, e.Hooks.GateRetries, verdict)})
			return nil, errGateRetry
		}
		if err != nil {
			return end(Fail, "end-of-turn gate: "+err.Error())
		}
	} else {
		r.Wrote = append([]string(nil), s.Wrote...)
	}
	if e.Gate != nil && e.Gate.DryRun {
		return end(Dry, "")
	}
	open := ""
	if e.PlanOpen != nil {
		open = e.PlanOpen()
	}
	forge := ""
	if !s.forgeAsked {
		forge = forgeDemand(s.Steps-s.turnStepBase, e.ForgeDemandAt, e.Lexicon.WorkspaceDir)
	}
	if e.ClosingCheck && !s.closingAsked {
		s.closingAsked = true
		s.forgeAsked = s.forgeAsked || forge != ""
		s.Journal.Log(Event{"t": "closingcheck", "wrote": s.Wrote, "steps": s.Steps, "plan": open})
		ask := "closing check: the agent's run is about to close. Read the ask once more — if anything it requests is " +
			"not yet applied end-to-end, apply it now; your tools stand. Otherwise answer again and that answer is the report."
		if open != "" {
			ask += "\nplan open — " + open + ": tick it done (the plan tool walks one step), or finish it first."
		}
		ask += witnessDemand(s.Ask, s.Wrote)
		ask += forge
		s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: ask})
		return nil, errGateRetry
	}
	if forge != "" {
		// policy.forge stands on its own: the closing check is off (or already spent), the run was
		// long, and the question is asked once
		s.forgeAsked = true
		s.Journal.Log(Event{"t": "forgedemand", "steps": s.Steps - s.turnStepBase})
		s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: "forge question:" + forge + "\nAnswer again with your report; that answer closes the run."})
		return nil, errGateRetry
	}
	if open != "" {
		r.hole("closed with an open plan: " + open)
	}
	return end(Done, "")
}

// witnessDemand is the closing check's falsification demand: an agent that wrote earns its close
// by naming each behavior it requires of the output, the check it ran that could have proven it
// wrong, and what that check observed — a close on belief ships nobody's transform verified. When
// the ask carries invariance words ("unchanged", "identical", "preserve", "equivalent", …), only
// a byte-level compare on representative untouched inputs admits; anything softer is the parse-
// round-trip failure the fj cell taught (2026-09-29). Read-only runs are not asked for witnesses.
func witnessDemand(ask string, wrote []string) string {
	if len(wrote) == 0 {
		return ""
	}
	w := wrote
	if len(w) > 8 {
		w = w[len(w)-8:]
	}
	t := "\nThis run wrote files (" + strings.Join(w, ", ") + "). Before you close: name every behavior you require of the output, " +
		"the check you ran that could have proven it wrong, and what it observed. No witness named means you have not looked — look now." +
		"\nAdmissible witnesses: a machine-check that could fail (a test, a diff, an assertion on the output), or an independent " +
		"second method that shares no codepath with the first. Reading the same output again at higher fidelity is not a witness — " +
		"it cannot disprove the first read. Name the forest before you name the leaf: enumerate what the artifact contains before " +
		"answering what it says."
	if invariantAsk(ask) {
		t += "\nThe ask requires some inputs to survive unchanged/identical: the only admissible witness for that is a byte-level " +
			"compare (cmp/sha) of your outputs against their inputs on a representative untouched sample (entities, self-closing " +
			"voids, attribute ordering, mixed quoting — whatever the input class offers). Run it now if you have not."
	}
	return t
}

// forgeDemand is the crystallization question for long runs (policy.forge.demandAtSteps): an agent
// that burned many steps carved a path with its feet, and paths walked twice should become
// skills. Under the floor the question stays silent (short runs teach nothing worth registering);
// over it the close must either name a @FORGE draft or say why the path is one-off. A floor of
// 0 is the policy switched off.
func forgeDemand(steps, floor int, workspaceDir string) string {
	if floor <= 0 || steps < floor {
		return ""
	}
	if workspaceDir == "" {
		workspaceDir = ".agent-one"
	}
	return "\nThis run cost " + fmt.Sprintf("%d", steps) + " steps. If the path it carved repeats — a tour, a probe, a ritual — " +
		"name its draft once: @FORGE skill:<name> | <when: the triggers that call for it> | <how: the steps>. The harness files it in the forge ledger; " +
		"when the human registers it at the working notes it becomes " + workspaceDir + "/crafted-skills/<name>/SKILL.md, which a later agent holds or loads with the skill tool. " +
		"If the path is one-off, say so in a word and close."
}

// invariantAsk reads the commission for invariance language. Substrings, lowercase — cheap and
// generic; a false positive only buys the agent one extra verification step.
func invariantAsk(ask string) bool {
	q := strings.ToLower(ask)
	for _, w := range []string{"unchanged", "identical", "preserve", "equivalent", "untouched", "unaltered", "byte-identical", "byte-for-byte", "as-is"} {
		if strings.Contains(q, w) {
			return true
		}
	}
	return false
}

// Emit renders the turn's answer for the human or the dispatcher: the model's report under
// @CAP when it spoke on the wire, else its text; the engine's own holes are appended.
func (r *Result) Emit(cap int) string {
	if !r.IsWire {
		parts := []string{}
		if t := strings.TrimSpace(r.Text); t != "" {
			parts = append(parts, t)
		}
		parts = append(parts, prefix("@? ", r.Holes)...)
		return strings.Join(parts, "\n")
	}
	rep := r.Report
	if r.Status != Done && r.Status != Dry {
		rep.Status = r.Status + " — " + rep.Status
	}
	for _, h := range r.Holes {
		rep.Holes = append(rep.Holes, h)
	}
	return rep.Emit(cap, "")
}

func (s *Session) saveTranscript() {
	if s.Journal == nil {
		return
	}
	p := strings.TrimSuffix(s.Journal.Path, ".jsonl") + ".transcript.json"
	if b := s.Transcript(); b != nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}

// Transcript marshals the resume pack for this run: what Resume reads back. Written by
// saveTranscript at the end of every failed/checkpointed/denied turn, and readable early
// by a circuit that needs the pack before the run has finished dying (a subagent about to be
// axed by its own clock journalizes to prepare for the time-out).
func (s *Session) Transcript() []byte {
	b, err := json.Marshal(map[string]interface{}{"runId": s.RunID, "ask": s.Ask, "messages": s.Messages, "wrote": s.Wrote, "steps": s.Steps, "spend": s.SpendNow()})
	if err != nil {
		return nil
	}
	return b
}

// ResumeWithSession is Resume plus the resumed session handle: hosts that rebind their live
// thread to the run (the suspended-card flow — the human's word revives the question at the
// step it died) need the session, not just the result.
func (e *Engine) ResumeWithSession(ctx context.Context, runID string, ask string) (*Result, *Session, error) {
	dir := e.JournalDir()
	if dir == "" {
		return nil, nil, fmt.Errorf("loop: journaling is off; nothing to resume from")
	}
	p := filepath.Join(dir, runID+".transcript.json")
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, nil, fmt.Errorf("loop: no transcript for %s (%v) — only CHECKPOINT, DENIED, ESCALATE, SUSPENDED and FAIL runs keep one", runID, err)
	}
	var t struct {
		Ask      string             `json:"ask"`
		Messages []provider.Message `json:"messages"`
		Wrote    []string           `json:"wrote"`
		Steps    int                `json:"steps"`
		Spend    provider.Usage     `json:"spend"`
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, nil, err
	}
	s := e.NewSession()
	s.RunID = runID
	if s.Journal != nil {
		s.Journal.Path = filepath.Join(dir, runID+".jsonl")
	}
	s.Messages, s.Wrote, s.Steps, s.Spend, s.Ask = t.Messages, t.Wrote, t.Steps, t.Spend, t.Ask
	s.Journal.Log(Event{"t": "resume", "id": runID, "messages": len(s.Messages), "ask": ask})
	if ask == "" {
		ask = "resumed — continue from where the run stopped"
	}
	n := len(s.Messages)
	switch {
	case n > 0 && s.Messages[n-1].Role == provider.Assistant && len(s.Messages[n-1].ToolCalls) > 0:
		var rs []provider.ToolResult
		for _, c := range s.Messages[n-1].ToolCalls {
			rs = append(rs, provider.ToolResult{ID: c.ID, Content: "interrupted before this ran — re-issue it if still needed", IsError: true})
		}
		s.Messages = append(s.Messages, provider.Message{Role: provider.User, ToolResults: rs, Text: ask})
	case n > 0 && s.Messages[n-1].Role == provider.User:
		s.Messages[n-1].Text = strings.TrimSpace(s.Messages[n-1].Text + "\n" + ask)
	default:
		s.Messages = append(s.Messages, provider.Message{Role: provider.User, Text: ask})
	}
	r, err := s.drive(ctx)
	return r, s, err
}

// Resume reopens a checkpointed, denied or escalated run from its transcript. Budgets restart
// with this process; a tool call cut mid-flight is answered "interrupted", never replayed —
// the model re-issues it and the gate asks again. Hosts needing the resumed session (the
// suspended-card flow) use ResumeWithSession.
func (e *Engine) Resume(ctx context.Context, runID string, ask string) (*Result, error) {
	r, _, err := e.ResumeWithSession(ctx, runID, ask)
	return r, err
}

func journalRel(s *Session) string {
	if s.Journal == nil {
		return ""
	}
	if rel, err := filepath.Rel(s.Engine.Root, s.Journal.Path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return s.Journal.Path
}

func currentAsk(s *Session) string {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == provider.User && s.Messages[i].Text != "" {
			return s.Messages[i].Text
		}
	}
	return s.Ask
}

func recallBlock(rc Recall) string {
	var b strings.Builder
	if len(rc.Anchors) > 0 {
		b.WriteString("@RECALL " + strings.Join(rc.Anchors, " ") + "\n")
	}
	if len(rc.Tools) > 0 {
		b.WriteString("@TOOLS\n" + strings.Join(rc.Tools, "\n") + "\n")
	}
	return b.String()
}

func summary(c provider.ToolCall) string {
	var m map[string]interface{}
	if json.Unmarshal(c.Input, &m) != nil {
		return string(c.Input)
	}
	for _, k := range []string{"command", "path", "pattern"} {
		if v, ok := m[k].(string); ok {
			if k == "path" {
				if p, ok := m["pattern"].(string); ok {
					v = p + " in " + v
				}
			}
			return oneLine(v)
		}
	}
	return oneLine(string(c.Input))
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	l := lines[len(lines)-1]
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" && !strings.HasPrefix(t, "[") {
			l = t
			break
		}
	}
	if len(l) > 120 {
		l = l[:120] + "…"
	}
	return l
}

func appendUnique(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

func prefix(p string, xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = p + x
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func approveList(g *gate.Gate) []string {
	var out []string
	for c, ok := range g.Approve {
		if ok {
			out = append(out, c.String())
		}
	}
	return out
}

func (e *Engine) toolNames() []string {
	if e.Tools == nil {
		return nil
	}
	return e.Tools.Names()
}

// malformedInput reads a native call's {"_malformed": "<raw>"} input — arguments the provider
// could not parse, kept under the tool's own name.
func malformedInput(in json.RawMessage) (string, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(in, &m) != nil || len(m) != 1 {
		return "", false
	}
	v, ok := m["_malformed"]
	if !ok {
		return "", false
	}
	var raw string
	_ = json.Unmarshal(v, &raw)
	return raw, true
}

func clipRaw(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	if s == "" {
		return "empty"
	}
	return s
}

// AddSpend adds a call's usage to the session's spend. A background subagent's report lands on
// its parent's session from the subagent's own goroutine while the parent's turn runs, so every
// read and write of Spend goes through spendMu (AddSpend, SpendNow).
func (s *Session) AddSpend(u provider.Usage) {
	s.spendMu.Lock()
	s.Spend = s.Spend.Add(u)
	s.spendMu.Unlock()
}

// SpendNow is the session's spend so far, read under the same lock.
func (s *Session) SpendNow() provider.Usage {
	s.spendMu.Lock()
	defer s.spendMu.Unlock()
	return s.Spend
}
