package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/wire"
)

// Commission is what the dispatch tool hands its runner: an agent to register and the wire
// commission it carries (@ROOT @SCOPE @ASK @CAP).
type Commission struct {
	Agent  string
	Ask    string
	Scope  string
	Root   string
	Unsaid bool
	// UnsaidExplicit marks a demand made on purpose (the caller passed unsaid:true), vs the
	// policy's ambient default. Only explicit demands make the close-out bounce for a missing @U.
	UnsaidExplicit bool
	Cap            int
	Depth          int    // the Subagent's nesting depth (1 for an agent dispatched by the session)
	Role           string // the role the ask names: findings → analyst, verdict → judge, draft → drafter
}

// RoleOf reads the role off an @ASK word (the triad: perceive, judge, speak); "" if none.
func RoleOf(ask string) string {
	for _, w := range strings.Fields(strings.ToLower(ask)) {
		switch w {
		case "findings":
			return "analyst"
		case "verdict":
			return "judge"
		case "draft":
			return "drafter"
		}
	}
	return ""
}

// String renders the commission on the wire.
func (c Commission) String() string {
	var b strings.Builder
	if c.Root != "" {
		fmt.Fprintf(&b, "@ROOT %s\n", c.Root)
	}
	if c.Scope != "" {
		fmt.Fprintf(&b, "@SCOPE %s\n", c.Scope)
	}
	ask := "findings"
	switch c.Role {
	case "judge":
		ask = "verdict"
	case "drafter":
		ask = "draft"
	}
	if c.Unsaid {
		ask += " +unsaid"
	}
	fmt.Fprintf(&b, "@ASK %s\n", ask)
	if c.Cap > 0 {
		fmt.Fprintf(&b, "@CAP %d\n", c.Cap)
	}
	b.WriteString(strings.TrimSpace(c.Ask) + "\n")
	return b.String()
}

// DispatchReport is what a runner brings back: the Subagent's wire report (under @CAP) and the
// readings the dispatcher needs.
type DispatchReport struct {
	Text    string   // the report, ready for the model
	Status  string   // the run's status word
	Unsaid  int      // @U lines carried
	Failed  bool     // the run failed (the act failed)
	Holes   []string // the engine's own holes
	Wrote   []string // paths the Subagent wrote (gated on its own account)
	Journal string
	// ReportPath/ReportBytes/ReportHash are the courier envelope: the harness filed the
	// report on the instruments shelf, and the dispatcher's context carries the digest
	// (@R path · size · @H fnv64) with the wrote list (@W) inline. The parent reads the
	// file when the verdict needs the flesh — relaying prose up the layers is the tax this
	// closes.
	ReportPath  string
	ReportBytes int
	ReportHash  string
	// Forges are the run's drafts: skills or tools the agent proved on the wire (@FORGE). They
	// ride up unminted — the layer curates, the working notes registers.
	Forges []wire.Forge
}

// Dispatcher runs one commission as a Ephemeral subagent: a fresh loop with its own context, tools cut
// to rank and ownership, one wire report back. The workspace package implements it; this package
// cannot import the loop.
type Dispatcher func(ctx context.Context, env Env, c Commission) (DispatchReport, error)

// DispatchOptions is tools.dispatch.
type DispatchOptions struct {
	Enabled    bool
	Depth      int      // the depth of the agent holding this tool (0 for the session)
	MaxDepth   int      // 0 = 1: a Subagent may not dispatch
	Cap        int      // default @CAP for a Subagent's report (0 = the wire's default)
	Unsaid     bool     // commission +unsaid by default
	Agents     []string // allowed agent names; nil = any
	Background bool     // `background: true` may run the Subagent asynchronously (tools.dispatch.background)
	// BackgroundTimeout bounds a background Subagent's run (0: 30m). The provider's own
	// timeout bounds each model call; nothing else stopped a background run from
	// outliving everything around it.
	BackgroundTimeout time.Duration
	// SessionBackground: at the session's own depth a commission defaults to background — the
	// orchestrator stays free; the Subagent rides on and its report wakes the session. A commission may
	// still name background:false to ride in the foreground.
	SessionBackground bool
	// Wake receives a background Subagent's report when it lands; nil: background is refused.
	Wake func(agent, report string, failed bool)
	// Preflight refuses a commission before anything runs or spawns — an agent off the roster,
	// an order the dispatch policy forbids — so a background dispatch is never acked as started
	// for a Subagent that cannot be registered. nil: the runner alone decides.
	Preflight func(c Commission) error
}

// DispatchTool is `dispatch`: register a named agent for one commission and return its report. A
// report with no @U line is flagged to the dispatcher (the unsaid).
func DispatchTool(opt DispatchOptions, run Dispatcher) *Tool {
	return &Tool{
		Name:        "dispatch",
		Description: "Dispatch a Ephemeral subagent: a named agent (zone-<zone>, domain-<domain>, coord-<name>) works one commission in a fresh context, tools cut to its rank and ownership, and answers one wire report. Its context dies with the task.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"agent":{"type":"string"},"ask":{"type":"string"},"scope":{"type":"string"},"role":{"type":"string","enum":["analyst","judge","drafter"],"description":"findings (analyst), verdict (judge) or draft (drafter); default by the ask"},"cap":{"type":"integer"},"unsaid":{"type":"boolean"},"background":{"type":"boolean","description":"run the Subagent in the background; its report wakes you when it lands"}},"required":["agent","ask"]}`),
		Class:       Write,
		Classify: func(env Env, in json.RawMessage) Classification {
			var a struct{ Agent string }
			_ = decode(in, &a)
			return Classification{Class: Write, Why: "dispatches " + a.Agent}
		},
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			var a struct {
				Agent, Ask, Scope, Role string
				Cap                     int
				Unsaid                  *bool
				Background              *bool
			}
			if err := decode(in, &a); err != nil {
				return fail("dispatch: %v", err)
			}
			if strings.TrimSpace(a.Agent) == "" || strings.TrimSpace(a.Ask) == "" {
				return fail("dispatch: agent and ask are required")
			}
			if !opt.Enabled {
				return fail("dispatch is off (tools.dispatch.enabled: false)")
			}
			max := opt.MaxDepth
			if max <= 0 {
				max = 1
			}
			if opt.Depth >= max {
				// the workspace floors its own ceiling at the rank chain's length (workspace.Engine), so this
				// refusal only answers at the floor rank or under a human-set MaxDepth below it
				return fail("dispatch: nesting limit reached (depth %d of %d) — a Subagent at this depth may not dispatch; answer with @? one hop up instead", opt.Depth, max)
			}
			if opt.Agents != nil && !inList(opt.Agents, a.Agent) {
				return fail("dispatch: %q is not an agent this session may dispatch (%s)", a.Agent, strings.Join(opt.Agents, ", "))
			}
			if run == nil {
				return fail("dispatch: no runner wired")
			}
			c := Commission{Agent: strings.TrimSpace(a.Agent), Ask: a.Ask, Scope: a.Scope, Root: env.Root, Cap: a.Cap, Depth: opt.Depth + 1, Unsaid: opt.Unsaid, Role: a.Role}
			if c.Role == "" {
				c.Role = RoleOf(a.Ask)
			}
			if c.Role == "" {
				c.Role = "analyst"
			}
			if c.Cap == 0 {
				c.Cap = opt.Cap
			}
			if a.Unsaid != nil {
				c.Unsaid = *a.Unsaid
				c.UnsaidExplicit = true // the caller named +unsaid in words — not the ambient policy
			}
			if opt.Preflight != nil {
				if err := opt.Preflight(c); err != nil {
					return fail("dispatch %s: %v", c.Agent, err)
				}
			}
			background := opt.Background && opt.SessionBackground && opt.Depth == 0
			if a.Background != nil {
				background = *a.Background
			}
			render := func(rep DispatchReport) string {
				// subagent-card-exit-verdict policy (harness leg 2b): the exit verdict is the run
				// journal's t:end status (rep.Status), stamped as the payload's first line so
				// any renderer that reads only the returned string still shows DONE/FAIL/
				// ESCALATE — never a collapsed lifecycle "done". The agent's own @S follows.
				var b strings.Builder
				if rep.Status != "" {
					b.WriteString("@S " + rep.Status + "\n")
				}
				text := strings.TrimSpace(rep.Text)
				if rep.ReportPath != "" {
					// the courier envelope: the report is filed on the instruments shelf; the
					// parent's context buys only the digest — @R where it lives, @H its
					// integrity, @W what the subagent wrote, a head-digest of the first lines.
					fmt.Fprintf(&b, "@R %s — the report is filed there (%d bytes); read it when the verdict needs the flesh\n", rep.ReportPath, rep.ReportBytes)
					fmt.Fprintf(&b, "@H fnv64:%s\n", rep.ReportHash)
					for _, p := range rep.Wrote {
						b.WriteString("@W " + p + "\n")
					}
					if text != "" {
						head := text
						if ls := strings.Split(text, "\n"); len(ls) > 8 {
							head = strings.Join(ls[:8], "\n") + "\n…"
						}
						b.WriteString("digest:\n" + head + "\n")
					}
				} else {
					b.WriteString(text)
				}
				out := b.String()
				if rep.Unsaid == 0 {
					out += fmt.Sprintf("\n@? dispatch %s: report carries no @U line — nothing unsaid, or the duty failed; ask again with +unsaid", c.Agent)
				}
				return out
			}
			if background {
				if !opt.Background || opt.Wake == nil {
					return fail("dispatch: background Subagents are off here (tools.dispatch.background: false, or no inbox delivery wired at depth %d) — dispatch %s in the foreground", opt.Depth, c.Agent)
				}
				dispatchedAt := time.Now().UnixNano()
				go spawnWatch(env, c.Agent, dispatchedAt, opt.Wake)
				go func() {
					defer func() {
						// a background runner panicking must never take the process down
						if rec := recover(); rec != nil {
							debug.PrintStack()
							opt.Wake(c.Agent, fmt.Sprintf("@S FAIL\n@? dispatch %s: panic: %v\n@E 0", c.Agent, rec), true)
						}
					}()
					t := opt.BackgroundTimeout
					if t <= 0 {
						t = 30 * time.Minute
					}
					bctx, cancel := context.WithTimeout(context.Background(), t)
					defer cancel()
					rep, err := run(bctx, env, c)
					if err != nil {
						opt.Wake(c.Agent, fmt.Sprintf("@S FAIL\n@? dispatch %s: %v\n@E 0", c.Agent, err), true)
						return
					}
					opt.Wake(c.Agent, render(rep), rep.Failed)
				}()
				return Result{Output: fmt.Sprintf("subagent %s started in the background on `%s` — keep working; its report wakes you when it lands (or /send %s <text> to reach it)", c.Agent, oneLineAsk(c.Ask), c.Agent)}
			}
			rep, err := run(ctx, env, c)
			if err != nil {
				return fail("dispatch %s: %v", c.Agent, err)
			}
			// the Subagent's writes are named so the dispatcher's record is honest; they were gated
			// on the Subagent's own account and its dispatcher's gate leaves them alone
			return Result{Output: render(rep), Err: rep.Failed, Wrote: rep.Wrote}
		},
	}
}

func oneLineAsk(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}

// spawnWatch is the phantom-dispatch guard (audit N5, 2026-10-02 evening): an acked background
// dispatch once returned ok while the child never registered its journal — the parent closed DONE
// believing the slice was staffed and the work was nobody's. Within spawnGrace of the ack, the
// child's journal must exist under <root>/<workspaceDir>/instruments/loop/<body>-<ts>-<id>.jsonl
// and have a first event. No journal, no first event → the dispatcher is woken with a
// spawn-failed FAIL on the wire, naming what to trust instead.
const spawnGrace = 10 * time.Second

func spawnWatch(env Env, agent string, dispatchedAt int64, wake func(agent, report string, failed bool)) {
	if wake == nil || agent == "" {
		return
	}
	dir := env.Root + "/" + env.workspaceDir() + "/instruments/loop"
	deadline := time.Now().Add(spawnGrace)
	for time.Now().Before(deadline) {
		ents, err := os.ReadDir(dir)
		if err == nil {
			for _, ent := range ents {
				if ent.IsDir() || !strings.HasPrefix(ent.Name(), agent+"-") || !strings.HasSuffix(ent.Name(), ".jsonl") {
					continue
				}
				info, err := ent.Info()
				if err != nil {
					continue
				}
				if info.ModTime().UnixNano() < dispatchedAt {
					continue // a stale journal from an earlier run of the same agent
				}
				if info.Size() == 0 {
					continue // created but no first event yet
				}
				return // alive and writing
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	wake(agent, fmt.Sprintf("@S FAIL\n@? dispatch %s: spawn failed — %s passed and no journal with a first event appeared in %s. The slice was never staffed; re-dispatch or escalate with @?, do not assume the child is alive.\n@E 0", agent, dir, spawnGrace), true)
}

func inList(xs []string, x string) bool {
	for _, y := range xs {
		if strings.EqualFold(y, x) {
			return true
		}
	}
	return false
}
