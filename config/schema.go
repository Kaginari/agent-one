package config

import (
	"fmt"
	"github.com/Kaginari/agent-one/value"
	"strconv"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/config/yaml"
)

// Config is the effective configuration: the typed schema, plus what the loader learned.
type Config struct {
	Schema     string `json:"$schema"`
	SmallModel string `json:"smallModel"`
	Mode       string `json:"mode"` // build | plan

	Models    Models               `json:"models"`
	Providers map[string]*Provider `json:"providers"`
	Tools     Tools                `json:"tools"`
	Policy    Policy               `json:"policy"`
	Guard     Guard                `json:"guard"`
	Registry  Registry             `json:"registry"`
	Memory    Memory               `json:"memory"`
	Toolbox   Toolbox              `json:"toolbox"`
	Ontology  Ontology             `json:"ontology"`

	Instruments Instruments         `json:"instruments"`
	Permissions Permissions         `json:"permissions"`
	Rules       []*Rule             `json:"rules"`
	Hooks       Hooks               `json:"hooks"`
	MCP         MCP                 `json:"mcp"`
	Discovery   Discovery           `json:"discovery"`
	Sessions    Sessions            `json:"sessions"`
	Compaction  Compaction          `json:"compaction"`
	Plan        Plan                `json:"plan"`
	Undo        Undo                `json:"undo"`
	Output      Output              `json:"output"`
	LogLevel    string              `json:"logLevel"`
	Budgets     Budgets             `json:"budgets"`
	UI          UI                  `json:"ui"`
	RankSet     string              `json:"rankSet"` // extend | replace
	RankDefs    map[string]*RankDef `json:"ranks"`

	// Learned by the loader.
	tree    *yaml.Node
	ranks   []Rank
	Dist    Dist              `json:"-"`
	Root    string            `json:"-"` // the workspace root (dir holding <dist-dir>)
	Layers  []Layer           `json:"-"`
	Origins map[string]Origin `json:"-"` // dotted key → where its live value came from
	Holes   []string          `json:"-"` // unknown keys and other @? findings
	opts    Options
}

// Origin is where a live value came from.
type Origin struct {
	Layer string // default | global | project | local | env-file | env | flag
	File  string // path, "env:VAR", "flag:--x" or "default"
	Line  int
}

func (o Origin) String() string {
	if o.Line > 0 {
		return fmt.Sprintf("%s:%d", o.File, o.Line)
	}
	return o.File
}

// Provider is one named model endpoint. Keys come from env only.
type Provider struct {
	Enabled   bool   `json:"enabled"`
	Type      string `json:"type"` // anthropic | openai | mock
	BaseURL   string `json:"baseURL"`
	APIKeyEnv string `json:"apiKeyEnv"`
	// APIKey: the key literal, when no env var is possible (the estate's container runs). Env stays
	// the preferred route: `apiKeyEnv` wins when it resolves. Never commit this value anywhere.
	APIKey          string                 `json:"apiKey"`
	Headers         map[string]string      `json:"headers"`
	Models          map[string]*ModelEntry `json:"models"`
	Timeout         Duration               `json:"timeout"`
	TLS             TLS                    `json:"tls"`
	MaxOutputTokens int                    `json:"maxOutputTokens"`
	ToolCalls       string                 `json:"toolCalls"` // native | text
	GuidedDecoding  bool                   `json:"guidedDecoding"`
	ContextWindow   AutoInt                `json:"contextWindow"` // auto | <n>
	Thinking        string                 `json:"thinking"`      // anthropic: adaptive | off
	Fallbacks       string                 `json:"fallbacks"`     // anthropic: default | off (refusal fallbacks)
	Script          string                 `json:"script"`        // mock only
}

// Models routes calls by member, role, rank and task; the session runs on Default.
// Role keys are canonical (analyst, judge, drafter); agent-one's analyst/judge/drafter
// and its rank words are accepted on input and folded to the canonical keys.
type Models struct {
	Default ModelRef            `json:"default"`
	Roles   map[string]ModelRef `json:"roles"`
	Ranks   map[string]ModelRef `json:"ranks"`
	Members map[string]ModelRef `json:"members"`
	Tasks   map[string]ModelRef `json:"tasks"`
}

// ModelRef is "provider/model" or {model, effort, fallback}.
type ModelRef struct {
	Model    string `json:"model"`
	Effort   string `json:"effort"`
	Fallback string `json:"fallback"`
}

// ModelEntry is what a provider does not report about one model; Tier orders the triad,
// Price prices its usage (absent = unpriced: tokens are shown, a cost is never guessed).
type ModelEntry struct {
	ID            string `json:"id"`
	Tier          int    `json:"tier"`
	ContextWindow int    `json:"contextWindow"`
	MaxOutput     int    `json:"maxOutput"`
	Reasoning     bool   `json:"reasoning"`
	Price         *Price `json:"price"`
}

// Price is USD per million tokens.
type Price struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// Cost prices a usage; the four counts are tokens.
func (p *Price) Cost(input, output, cacheRead, cacheWrite int) float64 {
	if p == nil {
		return 0
	}
	return (float64(input)*p.Input + float64(output)*p.Output + float64(cacheRead)*p.CacheRead + float64(cacheWrite)*p.CacheWrite) / 1e6
}

// Budgets are the honest stop lines per session and per Subagent; 0 is off.
type Budgets struct {
	Session  Cap `json:"session"`
	Subagent Cap `json:"subagent"`
}

type Cap struct {
	Tokens int     `json:"tokens"`
	USD    float64 `json:"usd"`
}

func (c Cap) Off() bool { return c.Tokens == 0 && c.USD == 0 }

// UI is the live session's chrome.
type UI struct {
	StatusLine        bool    `json:"statusLine"`
	AnnounceSubagents bool    `json:"announceSubagents"`
	Board             BoardUI `json:"board"`
	// Theme is the palette both faces hold — the terminal and the dashboard (package theme:
	// cyberpunk, neon-dystopia, corporate-rebellion, rain-city, synthwave, glitch, chromecore,
	// realm); Mode its variant: dark, light, or system (the terminal's background, the browser's
	// prefers-color-scheme). Switched live by /theme, /settings and the dashboard's menu, which
	// write the choice to the workspace's config.local.yaml.
	Theme string `json:"theme"`
	Mode  string `json:"mode"`
	// Meters are the session's own meters (ctx, plan); ctx unset derives its bands from
	// policy.budget (value.CtxMeter), plan unset is value.PlanMeter.
	Meters map[string]*value.MeterDef `json:"meters"`
}

// BoardUI is ui.board: the board the binary serves on 127.0.0.1.
type BoardUI struct {
	Autostart bool `json:"autostart"`
	Port      int  `json:"port"`
}

type TLS struct {
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	CAFile             string `json:"caFile"`
}

// Tools is the toolset: the builtins, custom tools, the profile and the missing-tool policy.
type Tools struct {
	Profile      string                 `json:"profile"` // max | anthropic | openai | minimal
	Read         BuiltinTool            `json:"read"`
	Ls           BuiltinTool            `json:"ls"`
	Glob         BuiltinTool            `json:"glob"`
	Grep         BuiltinTool            `json:"grep"`
	Write        BuiltinTool            `json:"write"`
	Edit         BuiltinTool            `json:"edit"`
	Delete       BuiltinTool            `json:"delete"`
	Multiedit    BuiltinTool            `json:"multiedit"`
	Patch        BuiltinTool            `json:"patch"`
	Bash         BuiltinTool            `json:"bash"`
	Git          BuiltinTool            `json:"git"`
	Webfetch     BuiltinTool            `json:"webfetch"`
	Websearch    BuiltinTool            `json:"websearch"`
	Ask          BuiltinTool            `json:"ask"`
	Dispatch     BuiltinTool            `json:"dispatch"`
	Recall       BuiltinTool            `json:"recall"`
	Remember     BuiltinTool            `json:"remember"`
	Toolbox      BuiltinTool            `json:"toolbox"`
	Onto         BuiltinTool            `json:"onto"`
	Skill        BuiltinTool            `json:"skill"`
	WorkingNotes BuiltinTool            `json:"workingNotes"`
	Plan         BuiltinTool            `json:"plan"`
	Pulse        BuiltinTool            `json:"pulse"`
	Custom       map[string]*CustomTool `json:"custom"`
	Output       ToolOutput             `json:"output"`
	Missing      Missing                `json:"missing"`
	TmpRedirect  TmpRedirect            `json:"tmpRedirect"`
}

// TmpRedirect is tools.tmpRedirect: absolute off-workspace targets are herded into
// `.agent-one/tmp` — a bash command's words rewritten (mode rewrite) or refused (mode refuse),
// and write/edit calls refused with the scratch target named. Policy: scratch lives under the
// workspace, never at /tmp or the filesystem root.
type TmpRedirect struct {
	Enabled   bool   `json:"enabled"`
	Mode      string `json:"mode"`      // rewrite | refuse
	RefuseCwd bool   `json:"refuseCwd"` // also refuse a bash `cwd` that leaves the workspace
}

// BuiltinTool is the union of every builtin's knobs; a knob a tool does not use is ignored.
type BuiltinTool struct {
	Enabled     bool     `json:"enabled"`
	Description string   `json:"description"` // override; "" keeps the builtin text
	Class       string   `json:"class"`       // tighten-only against the builtin floor
	Timeout     Duration `json:"timeout"`
	MaxTimeout  Duration `json:"maxTimeout"`
	MaxLines    int      `json:"maxLines"`
	MaxBytes    int      `json:"maxBytes"`
	Limit       int      `json:"limit"`
	Shell       string   `json:"shell"`
	Background  bool     `json:"background"`
	JobsDir     string   `json:"jobsDir"`
	Sandbox     string   `json:"sandbox"`  // bash: bwrap | none
	EnvAllow    []string `json:"envAllow"` // bash: env vars that survive scrubbing
	MaxDepth    int      `json:"maxDepth"` // dispatch
	// BackgroundTimeout bounds a background dispatch (tools.dispatch.backgroundTimeout;
	// 0 keeps the built-in 30m) — without it a background run had no clock at all
	BackgroundTimeout Duration `json:"backgroundTimeout"`
	StrictFlow        bool     `json:"strictFlow"` // dispatch: one hop per layer — no skip-level dispatches, neighbors per the rank's Sideways
	// Lineage (tools.dispatch.lineage, default true): an agent commissions only members whose
	// Reports-to chain reaches it — a domain owner its own zone workers, a coordinator its own domain owners. false leaves the
	// rank policy alone in force (any domain owner may order any zone worker).
	Lineage bool `json:"lineage"`
	// SessionBackground (tools.dispatch.sessionBackground): the session's own dispatches
	// default to background — the orchestrator stays free, the Subagent rides on and wakes it with the
	// report. Only the session (depth 0): a Subagent's own dispatches stay foreground by design.
	SessionBackground bool   `json:"sessionBackground"`
	Backend           string `json:"backend"` // websearch: a URL with {query}, answering SearXNG-style JSON
}

// CustomTool is a tool declared in config: argv with {{param}} placeholders, or a shell
// template with params as $P_<name> env vars — never shell-interpolated params.
type CustomTool struct {
	Enabled     bool              `json:"enabled"`
	Description string            `json:"description"`
	Params      map[string]*Param `json:"params"`
	Run         []string          `json:"run"`
	Shell       string            `json:"shell"`
	Class       string            `json:"class"`
	Timeout     Duration          `json:"timeout"`
	Cwd         string            `json:"cwd"`
	Sandbox     string            `json:"sandbox"` // inherit | bwrap | none
	Override    bool              `json:"override"`
	Profiles    []string          `json:"profiles"`
	// Crafted is a registered tool's provenance (the forge writes it; a hand-declared tool has none).
	Crafted *Crafted `json:"crafted"`
}

// Crafted names where a registered tool came from: the drafting agent, its run, the register's time and
// the draft's content hash in the forge ledger.
type Crafted struct {
	By    string `json:"by"`
	Run   string `json:"run"`
	At    string `json:"at"`
	Draft string `json:"draft"`
}

// Param is one flat JSON-schema property of a custom tool.
type Param struct {
	Type        string   `json:"type"` // string | integer | number | boolean
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     string   `json:"default"`
	Enum        []string `json:"enum"`
}

type ToolOutput struct {
	MaxLines int    `json:"maxLines"`
	MaxBytes int    `json:"maxBytes"`
	DumpDir  string `json:"dumpDir"`
	KeepDays int    `json:"keepDays"`
}

// Missing is the policy when the model calls a tool that is off or absent.
type Missing struct {
	DoomLoopRepeats       int  `json:"doomLoopRepeats"`
	ProposeOnSecondNaming bool `json:"proposeOnSecondNaming"`
	LiveReload            bool `json:"liveReload"`
}

// Policy is the convention as code paths.
type Policy struct {
	Principles Feature      `json:"principles"`
	HumanGate  HumanGate    `json:"humanGate"`
	Gate       Gate         `json:"gate"`
	DocsAsCode Feature      `json:"docsAsCode"`
	Ownership  OwnershipCfg `json:"ownership"`
	Wire       Wire         `json:"wire"`
	Log        LogFile      `json:"log"`
	Escalation Escalation   `json:"escalation"`
	Budget     Budget       `json:"budget"`
	DoomLoop   DoomLoop     `json:"doomLoop"`
	Stress     Stress       `json:"stress"`
	// ClosingCheck: the loop re-asks once, at closing time, when enabled — "is the daemon's ask
	// applied end-to-end?" — for the headless/bench path where nobody sits at the keyboard.
	ClosingCheck Feature `json:"closingCheck"`
	// Forge: the crystallization question. A run of at least DemandAtSteps steps is asked once,
	// at its close, to name a @FORGE draft or say the path was one-off — whether or not the
	// closing check is on. 0 switches it off.
	Forge ForgePolicy `json:"forge"`
	// Guts: the loop's per-step value sense (default on): scored acts, windowed momentum, a nudge
	// inside the tool result when the run slides past floor. The journal trains on every reading.
	Guts GutsCfg `json:"guts"`
	// Value: the one definition of success every surface reads (the ASL fold, the council, the
	// leaderboard, the guts' end-of-run), the ledger's window, and the families' value policies.
	Value ValuePolicy `json:"value"`
	// Evolution: a stressed member whose Notes working notes overflows heals itself — split, mutate
	// or rebirth (ranks.md §Evolution).
	Evolution EvolutionPolicy `json:"evolution"`
}

// EvolutionPolicy is policy.evolution (ranks.md §Evolution): a member that is stressed (strain at
// stressAt, its last failStreak gate verdicts failed, or a doom checkpoint) AND whose Notes
// working notes holds more than notesMax entries evolves by one arm, chosen from evidence: split (a
// large ownership whose failures cluster in two sub-directories), mutate (the Notes repeat a
// lesson: consolidated into invariants, a crafted skill when the lesson is a how) or rebirth (its invariants
// are stale: a fresh doc from the code). mode auto applies; propose queues the diff for the
// human (`evolve list|show|apply|reject`). A member evolves at most once per cooldownRuns of
// its runs. stressAt 0 reads policy.stress.escalateAt; notesMax 0 switches evolution off.
type EvolutionPolicy struct {
	Enabled      bool     `json:"enabled"`
	Mode         string   `json:"mode"`
	NotesMax     int      `json:"notesMax"`
	StressAt     int      `json:"stressAt"`
	FailStreak   int      `json:"failStreak"`
	CooldownRuns int      `json:"cooldownRuns"`
	Arms         []string `json:"arms"`
	// SplitLines and SplitFiles are what "large" means for the split arm: an ownership holding at
	// least this many source lines, or this many files (0 = that measure never reads large).
	SplitLines int `json:"splitLines"`
	SplitFiles int `json:"splitFiles"`
}

// ValuePolicy is policy.value (ranks.md §Value): which run words pass, fail or stay neutral; the
// window the value ledger folds over; how many orchestrator turns a rewrite of the same paths reopens
// a pass; the record below which a score is low-confidence; each family's default value policy
// (unset = the binary's, value.FamilyDefault).
type ValuePolicy struct {
	Enabled     bool                  `json:"enabled"`
	Window      string                `json:"window"`
	Pass        []string              `json:"pass"`
	Fail        []string              `json:"fail"`
	Neutral     []string              `json:"neutral"`
	ReopenTurns int                   `json:"reopenTurns"`
	MinRecord   int                   `json:"minRecord"`
	Families    map[string]*RankValue `json:"families"`
}

// RankValue is a rank's (or a family's) value policy as config writes it: a score expression over
// signals (value.Expr, clamped 0..100), the meter a surface draws, and the minimum record.
type RankValue struct {
	Score     string          `json:"score"`
	Meter     *value.MeterDef `json:"meter"`
	MinRecord int             `json:"minRecord"`
}

// GutsCfg tunes the loop's guts: what window the momentum reads, the floor past which they
// speak, the bench-visit count past which one more hit is thrash (not TDD), and the digest
// cadence for the run-so-far pulse line. Zero values take the loop's defaults (window 5,
// floor -2, benchThrashVisits 6, digestEvery 10).
type GutsCfg struct {
	Enabled           bool `json:"enabled"`
	Window            int  `json:"window"`
	Floor             int  `json:"floor"`             // ≤ 0; a positive floor is refused at load
	BenchThrashVisits int  `json:"benchThrashVisits"` // < 0 = the thrash arm is off
	DigestEvery       int  `json:"digestEvery"`       // 0 or < 0 = no digest line
	// Benches are the test workbenches, language-neutral: a command carrying any match string
	// is a bench; red lexemes pin it red, green ones green. Unset = the loop's (go, pytest, the
	// JS runners, cargo, make).
	Benches []GutsBench `json:"benches"`
	// Lexemes are the output words read as failure (negative) or a clean sighting (positive).
	Lexemes GutsLexemes `json:"lexemes"`
	// Weights price each reading class; unset = fail -1, negative -1, positive +1, repeat -1,
	// echo -1, thrash -1.
	Weights *GutsWeights `json:"weights"`
	// Clamp bounds one act's lexeme reading; unset = -2..+1.
	Clamp *GutsClamp `json:"clamp"`
	// Ring is how many recent workbenches the echo penalty remembers; 0 = 12.
	Ring int `json:"ring"`
}

// GutsBench is one test workbench.
type GutsBench struct {
	Name  string   `json:"name"`
	Match []string `json:"match"`
	Red   []string `json:"red"`
	Green []string `json:"green"`
}

// GutsLexemes are the output lexemes; an unset list keeps the loop's.
type GutsLexemes struct {
	Negative []string `json:"negative"`
	Positive []string `json:"positive"`
}

// GutsWeights price one reading of each class.
type GutsWeights struct {
	Fail     int `json:"fail"`
	Negative int `json:"negative"`
	Positive int `json:"positive"`
	Repeat   int `json:"repeat"`
	Echo     int `json:"echo"`
	Thrash   int `json:"thrash"`
}

// GutsClamp bounds one act's reading.
type GutsClamp struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Stress is the council: at what strain-ledger reading a dispatched agent draws a warrant that
// its dispatcher must answer (escalateAt; 0 or negative = the council is off), and how many
// warrants one agent may carry before the run hardens to ESCALATE and rises to the human
// (maxConsults; 0 reads as 3). Strain accumulates per agent from the runs that never landed —
// their governance sent-backs plus one — and clears on the agent's first clean landing.
// RelieveAt (0/negative = relief off) is the council's ACT threshold: strain past it means the
// zone itself is wrong, so the council relieves — splits a ≥2-prefix zone into two same-race
// young ones, retires a narrower one (the seat keeps its graph memory but claims no soil).
// Configuration shifts are not on this path: the doomLoop ladder names those arms on the wire, and
// a lane's DNA moves on the human's or a manager's word — never silently here.
type ForgePolicy struct {
	DemandAtSteps int `json:"demandAtSteps"`
	// EvolvedStatus is the status a skill the harness registers on its own (an evolution's mutate
	// arm) lands at: trial (default — the human promotes it) or active.
	EvolvedStatus string `json:"evolvedStatus"`
}

type Stress struct {
	EscalateAt  int `json:"escalateAt"`
	MaxConsults int `json:"maxConsults"`
	RelieveAt   int `json:"relieveAt"`
}

type Feature struct {
	Enabled bool `json:"enabled"`
}

type OwnershipCfg struct {
	Enabled                  bool `json:"enabled"`
	OrchestratorNeverAuthors bool `json:"orchestratorNeverAuthors"`
}

type HumanGate struct {
	Enabled bool     `json:"enabled"`
	Strict  bool     `json:"strict"`
	Approve []string `json:"approve"`
	DryRun  bool     `json:"dryRun"`
	// TrustContainer: true = when the harness lives inside its own runtime container
	// (<PREFIX>DOCKER_AGENT=1 — set only by container.go's argv, never by an agent), every action
	// class reads as pre-approved: the sandbox is the membrane, so outward/destructive no
	// longer ask. The guard denylist still refuses first (the workspace dir is mounted read-write,
	// inside the container or not).
	TrustContainer bool `json:"trustContainer"`
}

type Gate struct {
	Enabled        bool `json:"enabled"`
	RightAuthor    bool `json:"rightAuthor"`
	InvariantsHold bool `json:"invariantsHold"`
	DutiesDone     bool `json:"dutiesDone"`
	DocTruthful    bool `json:"docTruthful"`
	Retries        int  `json:"retries"`     // a failed gate goes back to the model this many times per turn
	TestsIntact    bool `json:"testsIntact"` // a turn may not pass by deleting, skipping or narrowing tests
	UI             bool `json:"ui"`          // a turn that touched the ui dir passes `ui check`'s lints
	// VerifyTimeout caps each member verify command's wall clock; 0 = the engine's
	// 120s. The old hardcoded 120s let one slow verify hold a write turn's tail for minutes.
	VerifyTimeout Duration `json:"verifyTimeout"`
}

type Wire struct {
	Enabled       bool `json:"enabled"`
	Cap           int  `json:"cap"`
	RequireUnsaid bool `json:"requireUnsaid"`
	Raw           bool `json:"raw"`
}

type LogFile struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

type Escalation struct {
	Enabled    bool `json:"enabled"`
	MaxRetries int  `json:"maxRetries"`
}

type Budget struct {
	ContextTokens int     `json:"contextTokens"`
	StressTokens  int     `json:"stressTokens"`
	Steps         int     `json:"steps"`
	Minutes       float64 `json:"minutes"`
	// SalvageTokens caps a subagent's pre-timeout journalise turn (tokens, output side).
	// At T-grace the subagent is resumed from its own transcript with one instruction — spend
	// this budget on `@U`/`@F` only — so a clock-killed agent dies with its knowledge landed.
	// 0 = the engine default; negative = off, the agent dies silent.
	SalvageTokens int `json:"salvageTokens"`
}

type DoomLoop struct {
	Enabled   bool `json:"enabled"`
	Threshold int  `json:"threshold"`
}

type Memory struct {
	Short  MemoryShort  `json:"short"`
	Long   MemoryLong   `json:"long"`
	Shared MemoryShared `json:"shared"`
	Recall MemoryRecall `json:"recall"`
}

type MemoryShort struct {
	Enabled  bool   `json:"enabled"`
	CacheDir string `json:"cacheDir"`
}

type MemoryLong struct {
	Enabled        bool   `json:"enabled"`
	Index          string `json:"index"`
	RebuildOnStale bool   `json:"rebuildOnStale"`
	// IndexOnBoot: heal a stale/missing long index once at boot, before any agent
	// runs — rebuildOnStale keeps rebuilds off the recall beat, this gate pays the
	// cost outside it (audit 2026-10-05 F1: a 24 h-stale index taxed every recall
	// of every agent with two nag holes for a whole night).
	IndexOnBoot bool `json:"indexOnBoot"`
}

type MemoryShared struct {
	Workspace PathFeature `json:"workspace"`
	Machine   PathFeature `json:"machine"`
}

type PathFeature struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

type MemoryRecall struct {
	K             int  `json:"k"`
	RelationBoost bool `json:"relationBoost"`
}

type Toolbox struct {
	Enabled      bool   `json:"enabled"`
	BudgetTokens int    `json:"budgetTokens"`
	Registry     string `json:"registry"`
	Extra        string `json:"extra"`
}

type Ontology struct {
	Enabled             bool   `json:"enabled"`
	Schema              string `json:"schema"`
	GraphDir            string `json:"graphDir"`
	ProjectBudgetTokens int    `json:"projectBudgetTokens"`
	Validate            bool   `json:"validate"`
}

type Instruments struct {
	Context Feature     `json:"context"`
	Loop    DirFeature  `json:"loop"`
	Toolbox DirFeature  `json:"toolbox"`
	Status  StatusKnobs `json:"status"`
}

type DirFeature struct {
	Enabled    bool   `json:"enabled"`
	JournalDir string `json:"journalDir"`
}

type StatusKnobs struct {
	ShowOff bool `json:"showOff"`
}

// Permissions overlays the class gate with ordered, most-specific-wins rules.
type Permissions struct {
	Enabled bool        `json:"enabled"`
	Rules   []*PermRule `json:"rules"`
	Import  Imports     `json:"import"`
}

// PermRule is `{match: "<tool>:<glob>", action: allow|ask|deny}`.
type PermRule struct {
	Match  string `json:"match"`
	Action string `json:"action"`

	Tool    string `json:"-"` // split from Match
	Pattern string `json:"-"`
	Origin  Origin `json:"-"`
}

type Imports struct {
	ClaudeCode Feature `json:"claudeCode"`
	Opencode   Feature `json:"opencode"`
}

// Rule is an injected policy: prose for the prompt and the ontology, or a check the gate runs.
type Rule struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	File    string   `json:"file"`
	Scope   string   `json:"scope"` // all | rank:<rank> | member:<name>
	Check   string   `json:"check"`
	Timeout Duration `json:"timeout"`

	Origin   Origin `json:"-"`
	Resolved string `json:"-"` // absolute path of File, when set
}

type Hooks struct {
	Enabled      bool     `json:"enabled"`
	Timeout      Duration `json:"timeout"`
	PreTool      []*Hook  `json:"preTool"`
	PostTool     []*Hook  `json:"postTool"`
	SessionStart []*Hook  `json:"sessionStart"`
	PreCompact   []*Hook  `json:"preCompact"`
	Stop         []*Hook  `json:"stop"`
	UserPrompt   []*Hook  `json:"userPrompt"`
}

type Hook struct {
	Match   string `json:"match"`
	Command string `json:"command"`
}

// MCP is the client: servers by name, imports from the other harnesses.
type MCP struct {
	Enabled bool                  `json:"enabled"`
	Timeout Duration              `json:"timeout"`
	Import  MCPImports            `json:"import"`
	Servers map[string]*MCPServer `json:"servers"`
}

type MCPImports struct {
	ClaudeCode PathFeature `json:"claudeCode"`
	Opencode   Feature     `json:"opencode"`
}

// MCPServer is one server: stdio (a command under the sandbox) or streamable http.
type MCPServer struct {
	Enabled    bool                `json:"enabled"`
	Type       string              `json:"type"` // stdio | http
	Command    []string            `json:"command"`
	Args       []string            `json:"args"`
	Env        map[string]string   `json:"env"`
	EnvAllow   []string            `json:"envAllow"`
	Cwd        string              `json:"cwd"`
	Network    bool                `json:"network"`
	Sandbox    string              `json:"sandbox"` // inherit | bwrap | none
	URL        string              `json:"url"`
	Headers    map[string]string   `json:"headers"`
	HeadersEnv map[string]string   `json:"headersEnv"` // header → env var holding its value
	Inward     bool                `json:"inward"`
	Tools      map[string]*MCPTool `json:"tools"`
	Timeout    Duration            `json:"timeout"`
}

type MCPTool struct {
	Enabled     bool   `json:"enabled"`
	Class       string `json:"class"`
	Description string `json:"description"`
}

type Discovery struct {
	Instructions Instructions `json:"instructions"`
	Skills       PathsFeature `json:"skills"`
	Commands     PathsFeature `json:"commands"`
	Agents       PathsFeature `json:"agents"`
}

type Instructions struct {
	Enabled bool     `json:"enabled"`
	Files   []string `json:"files"`
	Global  []string `json:"global"`
	WalkUp  bool     `json:"walkUp"`
}

type PathsFeature struct {
	Enabled bool     `json:"enabled"`
	Paths   []string `json:"paths"`
}

type Sessions struct {
	Enabled  bool   `json:"enabled"`
	Dir      string `json:"dir"`
	KeepDays int    `json:"keepDays"`
	Title    bool   `json:"title"`
}

type Compaction struct {
	Enabled         bool             `json:"enabled"`
	Strategy        string           `json:"strategy"` // drain | summary
	Trigger         Trigger          `json:"trigger"`
	KeepRecentTurns int              `json:"keepRecentTurns"`
	Passes          CompactionPasses `json:"passes"`
	Journal         bool             `json:"journal"`
}

type Trigger struct {
	Tokens   int     `json:"tokens"`
	Fraction float64 `json:"fraction"`
}

type CompactionPasses struct {
	Pointerize   Feature `json:"pointerize"`
	TrimSpent    Feature `json:"trimSpent"`
	Unsaid       Feature `json:"unsaid"`
	WorkingNotes Feature `json:"workingNotes"`
	Episode      Feature `json:"episode"`
	Verify       Feature `json:"verify"`
}

type Plan struct {
	Enabled bool   `json:"enabled"`
	Dir     string `json:"dir"`
}

type Undo struct {
	Enabled  bool   `json:"enabled"`
	Dir      string `json:"dir"`
	KeepDays int    `json:"keepDays"`
}

type Output struct {
	Format   string `json:"format"` // text | json | wire
	Stream   bool   `json:"stream"`
	Thinking bool   `json:"thinking"`
	Color    string `json:"color"`
}

// Duration reads "10m" / "30s" or a bare number of seconds.
type Duration time.Duration

func (d *Duration) decode(text string, isNumber bool) error {
	if isNumber {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return err
		}
		*d = Duration(time.Duration(f * float64(time.Second)))
		return nil
	}
	v, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("%q is not a duration (10m, 30s) or a number of seconds", text)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) String() string { return time.Duration(d).String() }

// D is the plain time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// AutoInt is "auto" or a number.
type AutoInt struct {
	Auto bool
	N    int
}

func (a *AutoInt) decode(text string, isNumber bool) error {
	if !isNumber {
		if strings.EqualFold(text, "auto") {
			*a = AutoInt{Auto: true}
			return nil
		}
		return fmt.Errorf("%q is not \"auto\" or a number", text)
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return err
	}
	*a = AutoInt{N: n}
	return nil
}

func (a AutoInt) String() string {
	if a.Auto {
		return "auto"
	}
	return strconv.Itoa(a.N)
}

// Class order: a declared class may only tighten (move right).
var classOrder = map[string]int{"read": 0, "write": 1, "outward": 2, "destructive": 3}

// Builtins maps each builtin tool to its class floor; "" means classified per call.
var Builtins = map[string]string{
	"bash": "", "git": "",
	"read": "read", "ls": "read", "glob": "read", "grep": "read",
	"write": "write", "edit": "write", "multiedit": "write", "patch": "write",
	"delete":   "destructive",
	"webfetch": "outward", "websearch": "outward",
	"ask": "read", "dispatch": "write", "recall": "read", "remember": "write",
	"toolbox": "read", "onto": "read", "skill": "read", "workingNotes": "write", "plan": "write",
	"pulse": "read",
}

// Profiles lists the tools each profile offers; "max" is every builtin.
var Profiles = map[string][]string{
	"max":       nil,
	"anthropic": nil,
	"openai":    nil,
	"minimal":   {"bash", "read", "write", "edit", "ask", "dispatch", "recall", "remember", "skill", "workingNotes", "skim"},
}

// OutwardCapable names the tools whose acts may leave the workspace; an `allow` on them is a
// loosening, and a bare wildcard `allow` on them is refused.
var OutwardCapable = map[string]bool{"bash": true, "git": true, "webfetch": true, "websearch": true, "*": true}

// Guard is the global dangerous-command guard: the built-in denylist, the machine-wide file
// (~/.agents/hooks/dangerous-patterns.txt) and these files, refused before any gate.
type Guard struct {
	Enabled  bool     `json:"enabled"`
	Files    []string `json:"files"`
	Patterns []string `json:"patterns"` // POSIX-ERE (RE2) lines, added to the built-in list
}

// Registry is where things come from: the models a gateway serves, the external tools the toolbox
// indexes, the package registries (a private Artifactory) the shell and the container use, and the
// container registry --docker-agent pulls from. Keys and tokens are named, never written.
type Registry struct {
	// Models is the model catalogue, keyed "<provider>/<model-id>"; each entry joins
	// providers.<provider>.models unless the provider already declares it.
	Models map[string]*ModelEntry `json:"models"`
	// Tools are external tools for the toolbox, as extra.jsonl lines are.
	Tools      []*RegistryTool `json:"tools"`
	Packages   Packages        `json:"packages"`
	Containers Containers      `json:"containers"`
}

// RegistryTool is one external tool the toolbox indexes (the extra.jsonl fields).
type RegistryTool struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Description string   `json:"description"`
	Usage       string   `json:"usage"`
	Serves      string   `json:"serves"`
	Triggers    []string `json:"triggers"`
}

// Packages points the package managers at a registry: set in the shell and in the container.
type Packages struct {
	Npm string `json:"npm"` // NPM_CONFIG_REGISTRY
	Pip string `json:"pip"` // PIP_INDEX_URL, UV_INDEX_URL
	Go  string `json:"go"`  // GOPROXY
	// TokenEnv names the variable holding the registry's token; it is let through to the shell
	// (the agent can read it — give it a read-only token). The value is never written.
	TokenEnv string            `json:"tokenEnv"`
	Env      map[string]string `json:"env"` // anything else: PIP_TRUSTED_HOST, GONOSUMDB, CARGO_REGISTRIES_…
}

// Containers is --docker-agent's registry: a prebuilt runtime image, or the base and apt mirror to
// build one from.
// Containers is --docker-agent's registry: a prebuilt runtime image, or the base and apt mirror to
// build one from. Login fields resolve env-first (usernameEnv/tokenEnv) with literal fallback
// (username/token); the values never print, never log.
type Containers struct {
	Image string `json:"image"` // pull this runtime image instead of building one
	Base  string `json:"base"`  // the build's base image (default debian:bookworm-slim)
	Apt   string `json:"apt"`   // an apt mirror for the build (e.g. an Artifactory debian remote)
	// private registry login for pulling Base/Image
	Username    string `json:"username"`
	UsernameEnv string `json:"usernameEnv"`
	Token       string `json:"token"`
	TokenEnv    string `json:"tokenEnv"`
}

// PackageEnv is the environment the package registries set, sorted by name.
func (p Packages) PackageEnv() []string {
	var out []string
	add := func(k, v string) {
		if v != "" {
			out = append(out, k+"="+v)
		}
	}
	add("NPM_CONFIG_REGISTRY", p.Npm)
	add("PIP_INDEX_URL", p.Pip)
	add("UV_INDEX_URL", p.Pip)
	add("GOPROXY", p.Go)
	for _, k := range sortedKeys(p.Env) {
		add(k, p.Env[k])
	}
	return out
}
