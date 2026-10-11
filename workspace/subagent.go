package workspace

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/gate"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/provider"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/wire"
)

// reportEnvelopeFloor is the envelope threshold: a report under it rides the parent's context
// whole (courier overhead would exceed relay savings); over it the subagent files it on the
// instruments shelf and sends the digest. Small relay answers stay inline on purpose.
const reportEnvelopeFloor = 600

// fileReport lands a Subagent's report text beside its journal on the instruments shelf
// (engine soil — ownership policy refuses member writes under .agent-one/instruments/, so the
// file lands from the harness, never through the agent's hands) and answers the envelope fields.
func (w *Workspace) fileReport(agent, text string) (rel string, bytes int, hash string) {
	dir := filepath.Join(w.Root, w.Lex.WorkspaceDir) + "/instruments/loop"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, ""
	}
	name := fmt.Sprintf("%s-%s.report.md", agent, time.Now().UTC().Format("20060102030405"))
	path := filepath.Join(dir, name)
	h := fnv.New64a()
	h.Write([]byte(text))
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", 0, ""
	}
	return filepath.ToSlash(filepath.Join(w.Lex.WorkspaceDir, "instruments", "loop", name)), len(text), fmt.Sprintf("%x", h.Sum64())
}

// SubagentOptions is tools.dispatch as the workspace applies it: who may dispatch, how deep, what a
// Subagent's report is capped at. Which ranks may dispatch, and whom, is the rank table's: an agent
// dispatches ranks below its own (or its own when the rank is Sideways); the shelf is the
// rank's Tools.
type SubagentOptions struct {
	Enabled    bool
	MaxDepth   int       // 0 = 1
	Cap        int       // 0 = the wire's default
	Unsaid     bool      // commission +unsaid by default (a report with no @U is flagged either way)
	Agents     []string  // names a session may dispatch; nil = any member on the roster
	Trace      io.Writer // a Subagent's step trace; nil is quiet
	Background bool      // background Subagents allowed (tools.dispatch.background)
	// BackgroundTimeout bounds a background run; 0 falls back to the dispatch tool's default
	BackgroundTimeout time.Duration
	Wake              func(agent, report string, failed bool) // a background Subagent's report lands here (depth 0: the session's queue)
	// Deliver drops a line into a RUNNING agent's mid-turn inbox (the app subagent's Send path —
	// the same channel /send rides). Wired at depth ≥1, a background Subagent's report wakes its
	// own dispatcher there instead of the session's queue; nil there keeps background refused
	// at that depth (an orphaned report would wake the wrong agent). Ownership policy already
	// fences parallel Subagents' writes to their own ground, so async children never collide.
	Deliver func(agent, line string)
	// Noted is the session's bookkeeping for a finished background Subagent: its @U seats land as
	// workspace holes and its subagent card closes. Every depth calls it; the session queue (Wake) is
	// stacked on top of it at depth 0, Deliver at depth ≥1.
	Noted      func(agent, report string, failed bool)
	StrictFlow bool // one hop per layer: an agent dispatches only the rank directly below it (no skip-level); neighbors allowed per the rank's Sideways
	// SalvageTokens caps the journalise turn a Clock-subagent runs in its grace window — the agent is
	// resumed from its own transcript with "spend this on @U/@F only" so its knowledge lands before
	// the axe. 0 = the loop default; negative = off, the agent dies silent.
	SalvageTokens int
	// StressEscalateAt is the agent's strain ledger reading that draws a council warrant into
	// the Subagent's report (0 = the council is off). StressMaxConsults bounds the warrants one
	// agent may carry before the run hardens @S ESCALATE (0 reads as 3). StressRelieveAt is
	// the council's act threshold: strain past it calls relieve() — the zone itself changes
	// (split a wide one, retire a narrow one), and the ledger shelf keeps the birth/death.
	StressEscalateAt, StressMaxConsults, StressRelieveAt int
	// SessionBackground (tools.dispatch.sessionBackground): the session's own dispatches default
	// to background — orchestrator stays free; the Subagent rides on and its report wakes the session.
	SessionBackground bool
}

// Route is what the integrator's model router sees for one engine: the role the ask names
// (findings → analyst, verdict → judge, draft → drafter), the rank and the member.
type Route struct {
	Role   string
	Rank   string
	Member string
	Task   string // dispatch | session
}

// Build is what an engine for an agent is made of: the provider and gate it shares with its
// dispatcher, the base shelf, budgets, and every switch.
type Build struct {
	Provider  provider.Provider
	Models    func(Route) provider.Provider // nil = Provider for every route; the integrator routes models.roles/ranks/members here
	Gate      *gate.Gate
	Tools     *tool.Registry                                      // the base shelf; nil = tool.Builtins()
	Shelf     func(as string, depth int) (*tool.Registry, func()) // per-agent shelf (its own shell); nil = Tools; the closer runs when the agent's engine is closed
	Budget    loop.Budget
	Hooks     HookOptions
	Ownership OwnershipOptions
	Subagent  SubagentOptions
	Journal   string
	Trace     io.Writer
	Depth     int // 0 for the session; a dispatched agent is Depth+1
	Extra     loop.Hooks
	Role      string     // the role this engine serves (set by the dispatcher from @ASK; "" = the session's)
	Guts      *loop.Guts // the per-step value sense the orchestrator already carries — children inherit it here
	Doom      int        // notes-stress auto-relief threshold (policy.budget doomLoop); 0 = off
}

// providerFor resolves the provider for a route.
func (b Build) providerFor(r Route) provider.Provider {
	if b.Models != nil {
		if p := b.Models(r); p != nil {
			return p
		}
	}
	return b.Provider
}

// Engine builds a full engine for an agent: tools cut to its rank and ownership (plus dispatch
// when its rank and depth allow), the ownership policy, the workspace's hooks, the lexicon.
func (w *Workspace) Engine(as string, b Build) *loop.Engine {
	base := b.Tools
	var closer func()
	if b.Shelf != nil {
		base, closer = b.Shelf(as, b.Depth)
	}
	if base == nil {
		base = tool.Builtins()
	}
	shelf := tool.NewRegistry()
	for _, n := range base.Names() {
		if t, ok := base.Get(n); ok {
			shelf.Add(t)
		}
	}
	shelf.Add(w.PolicyTool())
	rank := w.RankOf(as)
	names := ExpandTools(w.Ranks.Tools(rank.Name), append(shelf.Names(), "dispatch"))
	role := b.Role
	if role == "" {
		role = rank.Role
	}
	task := "session"
	if b.Depth > 0 {
		task = "dispatch"
	}
	e := &loop.Engine{
		Provider: b.providerFor(Route{Role: role, Rank: rank.Name, Member: as, Task: task}), Gate: b.Gate, Root: w.Root, As: as, Budget: b.Budget, Journal: b.Journal, Trace: b.Trace,
		Lexicon: w.Lex.Loop(), Policy: w.ToolPolicy(as, b.Ownership), Cap: b.Subagent.Cap,
		Guts: b.Guts, Doom: b.Doom,
		// every write/edit/delete answers naming its zone's persistent agent — the ownership truth
		// quoted at the moment of the landing, out of the same w.Owner the gate reads
		ZoneOwner: func(rel string) string {
			if c := w.Owner(rel); c != nil {
				return c.Name
			}
			return ""
		},
	}
	if e.Cap == 0 {
		e.Cap = wire.DefaultCap
	}
	max := b.Subagent.MaxDepth
	if max <= 0 {
		max = 1
	}
	// the depth ceiling never hides the layer an agent still owes: the longest rank chain is the
	// configured floor — a coordinator always reaches its domain owner unless the human set a lower MaxDepth
	if cd := w.Ranks.ChainDepth(); max < cd {
		max = cd
	}
	if b.Subagent.Enabled && contains(names, "dispatch") && b.Depth < max {
		bg, wake := b.backgroundWake(as)
		shelf.Add(tool.DispatchTool(tool.DispatchOptions{Enabled: true, Depth: b.Depth, MaxDepth: max, Cap: b.Subagent.Cap, Unsaid: b.Subagent.Unsaid, Agents: b.Subagent.Agents, Background: bg, BackgroundTimeout: b.Subagent.BackgroundTimeout, Wake: wake, SessionBackground: b.Subagent.SessionBackground}, w.Dispatcher(e, b)))
	}
	e.Tools = shelf.Only(names...)
	e.Hooks = MergeHooks(w.Hooks(b.Hooks), b.Extra)
	if closer != nil {
		w.mu.Lock()
		w.closers[e] = closer
		w.mu.Unlock()
	}
	return e
}

// backgroundWake resolves the dispatch tool's background switch and wake channel for one
// engine's depth. The session (depth 0) keeps the session queue as always. Deeper, an
// explicit `background: true` was dead design — the wake default-queued into the SESSION,
// orphaning the report from its real dispatcher (the agent that asked). With Deliver wired
// (the app subagent's Send inbox channel), a background Subagent's report wakes its own dispatcher
// mid-turn at any depth; without Deliver, background stays refused at that depth rather than
// landing on the wrong member.
func (b Build) backgroundWake(as string) (bool, func(agent, report string, failed bool)) {
	if b.Depth == 0 {
		return b.Subagent.Background, b.Subagent.Wake
	}
	if b.Subagent.Deliver == nil {
		return false, nil
	}
	parent, deliver, noted := as, b.Subagent.Deliver, b.Subagent.Noted
	return b.Subagent.Background, func(child, report string, failed bool) {
		if noted != nil {
			noted(child, report, failed)
		}
		deliver(parent, fmt.Sprintf("[subagent %s reported]\n%s", child, report))
	}
}

// Close ends what an engine's agent held (its shell, its jobs). A dispatcher closes a Subagent's
// engine when the report is taken; the session closes its own at exit.
func (w *Workspace) Close(e *loop.Engine) {
	w.mu.Lock()
	c := w.closers[e]
	delete(w.closers, e)
	delete(w.sessions, e)
	// subagentWrites is arm 2's cross-session memory — Close must not clear it:
	// gate.go subagentPairedDoc (OUT-2) consults lane B past the engine's Close.
	delete(w.subagentWrote, e)
	w.mu.Unlock()
	if c != nil {
		c()
	}
}

// Dispatcher runs a commission as a Ephemeral subagent under a parent engine: a fresh loop with its
// own context, the parent's provider and gate, budgets cut to what the parent has left, one
// wire report back. The Subagent's spend is added to the parent's.
// stratTargets lists the members this engine may lawfully dispatch right now — kept short,
// one name per member that passes rank policy + strict-flow (caller is either just above it in
// the tree, or sideways for ranks that ride side-to-side). The policy's refusal prints it instead
// of a dead "Policy 2" wall: refusal + known-good list in the same breath (07 of the 2026-10-02
// audit — the "orchestrator flails 3 refused hops" shape).
func (w *Workspace) stratTargets(parent *loop.Engine) string {
	me := w.RankOf(parent.As)
	var ok []string
	for _, c := range append(append([]Member{}, w.Members...), w.Foreign...) {
		if c.Name == parent.As {
			continue
		}
		if me.Name == Orchestrator {
			if w.Ranks.DirectChild(Orchestrator, c.Rank) {
				ok = append(ok, c.Name)
			}
			continue
		}
		down := w.Ranks.Below(c.Rank, me.Name) && w.Ranks.DirectChild(me.Name, c.Rank)
		side := me.Sideways && c.Rank == me.Name
		if down || side {
			ok = append(ok, c.Name)
		}
	}
	if len(ok) == 0 {
		return "(none — try @? to escalate)"
	}
	return strings.Join(ok, ", ")
}

func (w *Workspace) Dispatcher(parent *loop.Engine, b Build) tool.Dispatcher {
	return func(ctx context.Context, env tool.Env, c tool.Commission) (tool.DispatchReport, error) {
		agent := strings.ToLower(strings.TrimSpace(c.Agent))
		cr := w.Member(agent)
		if cr == nil {
			return tool.DispatchReport{}, fmt.Errorf("no agent named %q on the roster (%s/{%s}/<name>/) — a Subagent is registered for a rank, never generic. Dispatchable from here: %s", agent, w.Lex.WorkspaceDir, strings.Join(rankDirs(w.Ranks), ","), w.stratTargets(parent))
		}
		me := w.RankOf(parent.As)
		if me.Name != Orchestrator && !w.Ranks.Below(cr.Rank, me.Name) && !(me.Sideways && cr.Rank == me.Name) {
			return tool.DispatchReport{}, fmt.Errorf("%s (%s) may not dispatch %s (%s): work goes down the ranks, never up or sideways (Policy 2) — escalate with @? instead. Dispatchable from you: %s", parent.As, me.Name, cr.Name, cr.Rank, w.stratTargets(parent))
		}
		// strictLayer: the ladder is climbed one layer at a time — an agent hands work to the rank
		// directly (one hop) below it, never skipping a layer; sideways per the rank's Sideways.
		// orchestrator is exempt above (it dispatches the coordinator; with strictFlow it may reach only its
		// direct children — enforced here too, never skipped into a domain).
		if b.Subagent.StrictFlow && !w.Ranks.DirectChild(me.Name, cr.Rank) {
			return tool.DispatchReport{}, fmt.Errorf("%s (%s) may not dispatch %s (%s) under strict layer flow: work crosses one layer at a time — hand it to the rank directly below (%s), and the neighbor knows its own", parent.As, me.Name, cr.Name, cr.Rank, w.firstChildLine(me.Name))
		}
		// Domain owner TDD as code (Policy 3 + Operator decree 2026-09-30): a gate holder dispatching an
		// authoring zone worker must first have coded that zone worker's verify lines on its README — the
		// verdict the gate will run exists BEFORE the work is ordered, never written in the same
		// breath. Fires when the dispatcher's rank HoldsGate and the target's rank Authors.
		{
			targetRank, _ := w.Ranks.Get(cr.Rank)
			meRank, _ := w.Ranks.Get(me.Name)
			if targetRank.Authors && meRank.HoldsGate && !cr.VerifyCoded() {
				return tool.DispatchReport{}, fmt.Errorf("%s (%s) has no Verify line coded — the domain owner writes the zone worker's verdict BEFORE the order (TDD per Policy 3, Operator decree 2026-09-30). Add a non-trivial line in %s (front-matter verify:, or a `- `<cmd>`` bullet under ## Verify), then dispatch again", cr.Name, cr.Rank, cr.Doc)
			}
		}
		child := b
		child.Depth = b.Depth + 1
		child.Role = RoleOf(c.Ask)
		if c.Role != "" {
			child.Role = c.Role
		}
		child.Trace = b.Subagent.Trace
		child.Budget = b.Budget
		if ps := w.SessionOf(parent); ps != nil {
			if steps := parent.Budget.Steps; steps >= 0 {
				if steps == 0 {
					steps = loop.DefaultSteps
				}
				left := steps - ps.Steps
				if left <= 0 {
					return tool.DispatchReport{}, fmt.Errorf("parent step budget spent (%d of %d) — nothing left for a Subagent", ps.Steps, steps)
				}
				child.Budget.Steps = left
			}
			if m := parent.Budget.Minutes; m >= 0 {
				if m == 0 {
					m = loop.DefaultMinutes
				}
				left := m - time.Since(ps.Started).Minutes()
				if left <= 0 {
					return tool.DispatchReport{}, fmt.Errorf("parent wall-clock budget spent (%g min) — nothing left for a Subagent", m)
				}
				child.Budget.Minutes = left
			}
		}
		e := w.Engine(cr.Name, child)
		defer w.Close(e) // the Subagent's context dies with the task
		e.Unsaid = c.Unsaid
		e.UnsaidExplicit = c.UnsaidExplicit
		e.Wire = true
		if c.Cap > 0 {
			e.Cap = c.Cap
		}
		c.Cap = e.Cap // the ceiling in force rides the commission
		// an agent that never reports holds its own clock: a hung model call can be cancelled
		// mid-turn, not just between turns — the parent waits forever otherwise (the stuck
		// subagent Operator saw: `thinking · 17m`, then `thinking · 22m` on a streamed Completion).
		// The uncapped-order (policy.budget.minutes: -1) outlaws the wall-clock for the SESSION's
		// own turns — a dispatched agent is not the session: it always carries a hard clock, the
		// parent's ceiling when one is armed, loop.DefaultMinutes when the session runs uncapped.
		subagentMinutes := child.Budget.Minutes
		if subagentMinutes <= 0 {
			subagentMinutes = loop.DefaultMinutes
		}
		// two clocks: a grace deadline, then the axe. When the grace fires the worker journalizes
		// its own transcript (the resume pack) so a clock-killed subagent dies WITH its knowledge
		// on disk — a next run can `agent-one resume <runID>` and inherit it, instead of starting cold.
		total := time.Duration(subagentMinutes * float64(time.Minute))
		grace := total / 10
		if grace < 2*time.Second {
			grace = 2 * time.Second
		}
		if grace > time.Minute {
			grace = time.Minute
		}
		// the worker keeps the full ceiling unless the grace is spendable: with salvage off (negative)
		// there is nothing to do in the window, so the subagent runs to the axe exactly as it always has —
		// a parent whose own budget.minutes matches the subagent's then dies by its own clock first.
		salvageArmed := b.Subagent.SalvageTokens >= 0
		workerTotal := total
		if salvageArmed {
			workerTotal = total - grace
		}
		inner, cancelWorker := context.WithTimeout(ctx, workerTotal)
		ctx2, cancel := context.WithTimeout(ctx, total)
		if e.JournalDir() != "" {
			go func() {
				<-inner.Done()
				if ctx2.Err() != nil {
					return // the axe fell first, or the parent died — nothing to prepare
				}
				s := w.SessionOf(e)
				if s == nil {
					return
				}
				tp := strings.TrimSuffix(s.Journal.Path, ".jsonl") + ".transcript.json"
				if b := s.Transcript(); b != nil {
					_ = os.WriteFile(tp, b, 0o644)
				}
			}()
		}
		defer cancel()
		defer cancelWorker()
		r, err := e.Run(inner, c.String())
		// the clock fired inside the run when the worker context died by deadline. A hung child comes
		// back err == nil — the loop folds the cancelled Complete into a Fail status via end(), which
		// returns a nil Go error — so the trigger is the worker deadline with a non-Done result, never
		// err != nil. That is the shape of the stuck subagents Operator saw (`thinking · 17m`).
		clockFired := inner.Err() == context.DeadlineExceeded || ctx2.Err() == context.DeadlineExceeded
		diedByClock := clockFired && (r == nil || r.Status != loop.Done)
		memoLine := ""
		if diedByClock {
			// Name it: the lane, the provider, the budget — never a mule "the model never answered"
			// (018 of the 2026-10-02 audit: the user reads this line on the wire).
			lane := "unknown-model"
			if e.Provider != nil {
				lane = e.Provider.Name() // "openai/<model>"
			}
			provider, _, _ := strings.Cut(lane, "/")
			timeoutWhy := fmt.Sprintf("model %s on provider %q did not answer within the %g-minute subagent budget", lane, provider, subagentMinutes)
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				err = fmt.Errorf("%s: subagent ran out of its %g-minute budget — %s (escalate with @? or re-dispatch): %w", cr.Name, subagentMinutes, timeoutWhy, err)
			} else if err == nil {
				err = fmt.Errorf("%s: subagent ran out of its %g-minute budget — %s (escalate with @? or re-dispatch)", cr.Name, subagentMinutes, timeoutWhy)
			}
			// configuration memo (workspace/subagent_vita.go): the stress ledger keeps the lane of the burn; the next-fit
			// ladder rides the wire as advice — never a silent mutation (Operator policy 2026-10-02).
			w.mu.Lock()
			w.stressDeaths[agent] = stressEnd{Lane: lane, At: time.Now().UTC()}
			w.ledgerNote(ledgerRow{Kind: "death", Agent: agent, Lane: lane, At: w.stressDeaths[agent].At})
			w.mu.Unlock()
			if up := laneUp(lane); up != "" {
				memoLine = fmt.Sprintf("\n[subagent configuration memo] %s burned its subagent clock on lane %s — next-fit ladder says %s (or split the ask). Advisory only: nobody's DNA moves until the parent or human signs off.", cr.Name, lane, up)
			}
			// then spend the grace on a journalise turn: the agent resumes from its own transcript,
			// hard-capped at salvageTokens, under the axe's remaining window — told to land @U/@F only,
			// so a clock-killed subagent dies with its knowledge on the wire and the shared notes, not in
			// a transcript nobody reads.
			if st := b.Subagent.SalvageTokens; st >= 0 {
				if st == 0 {
					st = loop.DefaultSalvageTokens
				}
				se := w.Engine(cr.Name, child)
				se.Unsaid, se.Wire = c.Unsaid, true
				se.Budget.MaxTokens, se.Budget.Steps = st, 1
				se.Provider = b.providerFor(Route{Role: RoleOf(c.Ask), Rank: w.RankOf(cr.Name).Name, Member: cr.Name, Task: "salvage"})
				if sr, serr := se.Resume(ctx2, e.LastRunID(), "You are out of time — the grace window is all there is. Do NOT continue the task. Answer on the wire now: `@S HOLE`, one `@F` per finding already certain, every `@U` you owe (@U policy/team/ownership), `@?` for what you could not settle, `@E`. Plain notes next."); serr == nil && sr != nil && sr.IsWire {
					if rep, ok := wire.ParseReport(sr.Text); ok && len(rep.Unsaid) > 0 {
						holes := w.Land(cr.Name, rep)
						_ = holes
					}
				}
				w.Close(se)
			}
		}
		if err != nil {
			return tool.DispatchReport{}, err
		}
		if ps := w.SessionOf(parent); ps != nil {
			ps.Spend = ps.Spend.Add(r.Usage)
		}
		rep := tool.DispatchReport{Text: r.Emit(e.Cap), Status: r.Status, Unsaid: len(r.Report.Unsaid), Failed: r.Status == loop.Fail, Holes: r.Holes, Wrote: wroteOf(r), Journal: r.Journal, Forges: r.Report.Forges}
		rep.Text += memoLine
		// the courier policy: substantive reports are filed on the instruments shelf, and what
		// rides the parent's context is the envelope — the path, the size, the checksum. The
		// parent that needs the flesh buys it with one read; relaying prose up five layers is
		// the relay tax this closes.
		if strings.TrimSpace(rep.Text) != "" && len(rep.Text) > reportEnvelopeFloor {
			rep.ReportPath, rep.ReportBytes, rep.ReportHash = w.fileReport(agent, rep.Text)
		}
		if stamp, escalate := w.councilLedger(agent, verdictOf(r), r, b.Subagent); stamp != "" {
			rep.Text += stamp
			if escalate {
				rep.Status, rep.Failed = "ESCALATE", true
			}
		}
		// the relief arm (policy.stress.relieveAt): the warrant talked, now the zone changes —
		// outside the council's lock; relieve rereads safely, and both the act and the ledger
		// shall witness it
		if rel := b.Subagent.StressRelieveAt; rel > 0 {
			w.mu.Lock()
			strainNow := w.stress[agent]
			w.mu.Unlock()
			if strainNow >= rel {
				if act, ok := w.relieve(agent); ok {
					rep.Text += "\n@? council relief — " + agent + " crosses relieveAt: " + act
				}
			}
		}
		w.subagentGated(parent, agent, rep.Wrote)
		// the tally grows before the return: the ranking agent's report is its record, and an
		// promotion may register off it
		w.FoldMetrics(agent, string(r.Status), len(r.Holes))
		return rep, nil
	}
}

// firstChildLine names the layer directly below a rank for the strict-flow refusal text.
func (w *Workspace) firstChildLine(rank string) string {
	if rank == Orchestrator {
		var kids []string
		for _, r := range w.Ranks {
			if r.ReportsTo == Orchestrator {
				kids = append(kids, r.Name)
			}
		}
		if len(kids) > 0 {
			return "orchestrator's children: " + strings.Join(kids, ", ")
		}
		return "orchestrator has no children"
	}
	r, ok := w.Ranks.Get(rank)
	if !ok {
		return "the layer below"
	}
	var kids []string
	for _, o := range w.Ranks {
		if o.ReportsTo == r.Name {
			kids = append(kids, o.Name)
		}
	}
	if len(kids) > 0 {
		return "the one beneath: " + strings.Join(kids, ", ")
	}
	if r.Sideways {
		return "sideways within the layer: " + strings.Join(w.Ranks.NeighborRanks(r.Name), ", ")
	}
	return "no child and no neighbor — escalate with @?"
}

// verdictOf is the agent's own word from its wire: the first token of its @S line. The loop's
// lifecycle status is not the verdict — a subagent that reports "@S FAIL" landed its run cleanly
// (the Policy-9 lesson); the journal's status says DONE where the agent says FAIL.
func verdictOf(r *loop.Result) string {
	if s := r.Report.Status; s != "" {
		tok := strings.ToUpper(strings.Fields(s)[0])
		switch tok {
		case "DONE", "FAIL", "ESCALATE", "DENIED", "INTERRUPTED":
			return tok
		}
	}
	return ""
}

// councilLedger is the workspace's stress policy, coded. A Subagent whose work did not land — the loop
// ended Fail or Checkpoint, or the agent's own verdict is FAIL/ESCALATE — adds strain to its
// agent's ledger: the run's governance sent-backs (failed acts standing at the end, gate
// re-asks, wire re-asks) plus one for the landing itself. Past policy.stress.escalateAt a warrant
// rides the report back to the dispatcher — an agent under strain needs its MANAGER, and the
// manager answers in kind: rewrite the spec (@INSTRUCT), narrow it (@SPLIT), take counsel
// sideways, or carry the hole up (@ESCALATE). The warrants an agent may carry are bounded
// (policy.stress.maxConsults); past the bound the warrant hardens: the report ends @S ESCALATE
// and rises to the human. A clean landing clears the ledger.
//
// The loop's DENIED and INTERRUPTED never consult: the human's word and the clock's policy
// already answered.
func (w *Workspace) councilLedger(agent string, verdict string, r *loop.Result, subagent SubagentOptions) (stamp string, escalate bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	landed := r.Status == loop.Done && verdict == "DONE"
	if landed { // a clean landing clears the ledger — management by exception
		delete(w.stress, agent)
		delete(w.consults, agent)
		w.ledgerNote(ledgerRow{Kind: "stressClear", Agent: agent})
		return "", false
	}
	struggled := r.Status == loop.Fail || r.Status == loop.Checkpoint || verdict == "FAIL" || verdict == "ESCALATE"
	if !struggled {
		return "", false // self-DENIED, INTERRUPTED, off-verdict: another policy already spoke
	}
	at := subagent.StressEscalateAt
	if at <= 0 {
		return "", false // the council is off
	}
	w.stress[agent] += max(1, r.Strains+1)
	w.ledgerNote(ledgerRow{Kind: "stress", Agent: agent, Value: w.stress[agent]})
	if w.stress[agent] < at {
		return "", false
	}
	w.consults[agent]++
	w.ledgerNote(ledgerRow{Kind: "consults", Agent: agent, Value: w.consults[agent]})
	maxConsults := subagent.StressMaxConsults
	if maxConsults <= 0 {
		maxConsults = 3
	}
	head := fmt.Sprintf("\n@? council warrant — %s carries strain %d (escalate-at %d): its governance note-line broke %d time%s this run and never landed.", agent, w.stress[agent], at, r.Strains+1, plural(r.Strains+1))
	if w.consults[agent] > maxConsults {
		return head + fmt.Sprintf("\n@? council exhausted: %d consults carried and the strain holds. This Subagent ends @S ESCALATE — the warrant rises to the human.", maxConsults), true
	}
	return head + fmt.Sprintf(`
@? Answer it on your next step, as its manager — a strain the worker carries is a spec the manager rewrites:
@?     @INSTRUCT <text>    re-commission this agent with the instruction folded into the ask (context rides the spec, never the worker)
@?     @SPLIT <asks>       narrow it: one commission per slice, same agent
@?     @ESCALATE           carry @? up — your layer cannot settle it (or take counsel sideways first: layer-mates know the domain)
@? This warrant is consult %d of %d; a clean landing clears the ledger.`, w.consults[agent], maxConsults), false
}

// subagentWrite is the subagent-write ledger's value: which agent wrote the path, and when its report
// was taken (the path is the key). The ledger is an attribution record, not a mere set — the
// orchestrator's second pairing arm (workspace/gate.go, decree 2026-10-01) reads it to prove a doc on
// disk is this session's own subagent work, merely a turn apart from the code that lands under it.
type subagentWrite struct {
	Writer string    // the Subagent's own agent name
	At     time.Time // when its report was taken
}

// subagentWritesBound caps arm 2's cross-session memory per member.
const subagentWritesBound = 8

// subagentGated records what a Subagent of the parent engine wrote (and gated itself): path, writer,
// moment. The record rides the PARENT's key at subagent-report time, and the parent's key survives
// the Subagent's own Close (Close deletes the subagent's key only), so the attribution holds across
// the dispatcher's later turns and resumes within the session.
func (w *Workspace) subagentGated(parent *loop.Engine, agent string, wrote []string) {
	if len(wrote) == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	m := w.subagentWrote[parent]
	if m == nil {
		m = map[string]subagentWrite{}
		w.subagentWrote[parent] = m
	}
	now := time.Now()
	for _, p := range wrote {
		m[normRel(p)] = subagentWrite{Writer: agent, At: now}
	}
	// Lane B: double-write into the workspace-resident record keyed by the member name,
	// so an orchestrator-side consult through a fresh engine still pairs cross-session.
	bm := w.subagentWrites[agent]
	if bm == nil {
		bm = map[string]subagentWrite{}
		w.subagentWrites[agent] = bm
	}
	for _, p := range wrote {
		bm[normRel(p)] = subagentWrite{Writer: agent, At: now}
		w.ledgerNote(ledgerRow{Kind: "subagentWrite", Agent: agent, Path: normRel(p), Writer: agent, At: now})
	}
	// Evict oldest-bound arm 2's cross-session memory per member.
	for len(bm) > subagentWritesBound {
		oldest := ""
		for k, cw := range bm {
			if oldest == "" || cw.At.Before(bm[oldest].At) {
				oldest = k
			}
		}
		delete(bm, oldest)
	}
}

// ownWrites is a session's writes less what its Subagents wrote and gated on their own account.
func (w *Workspace) ownWrites(s *loop.Session) []string {
	w.mu.Lock()
	m := w.subagentWrote[s.Engine]
	w.mu.Unlock()
	if len(m) == 0 {
		return s.Wrote
	}
	var out []string
	for _, p := range s.Wrote {
		if _, subagent := m[normRel(p)]; !subagent {
			out = append(out, p)
		}
	}
	return out
}

// subagentGatedTask reports whether doc was written by a Subagent of s's engine —
// lane A (this session's per-engine ledger) or, lane B, the workspace-resident
// cross-session ledger keyed by the member name carried in w.sessions, so a
// orchestrator re-consult through a fresh engine still pairs end-gate. Lane B is live
// at the gate: gate.go subagentPairedDoc (OUT-2/arm 2) consults it directly since
// 2026-10-03; this helper predates that wiring and now serves the test suite.
func (w *Workspace) subagentGatedTask(s *loop.Session, doc string) bool {
	if doc == "" {
		return false
	}
	rel := normRel(doc)
	w.mu.Lock()
	defer w.mu.Unlock()
	if m := w.subagentWrote[s.Engine]; m != nil && m[rel].Writer != "" {
		return true
	}
	if name := s.Engine.As; name != "" {
		if m := w.subagentWrites[name]; m != nil && m[rel].Writer != "" {
			return true
		}
	}
	return false
}

func rankDirs(rs Ranks) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range rs {
		if r.Dir != "" && !seen[r.Dir] {
			seen[r.Dir] = true
			out = append(out, r.Dir)
		}
	}
	return out
}

func wroteOf(r *loop.Result) []string {
	if len(r.Wrote) > 0 {
		return r.Wrote // every write the Subagent's gate saw, whatever tool made it
	}
	var out []string
	for _, st := range r.Steps {
		out = append(out, st.Wrote...)
	}
	return out
}
