package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/sandbox"
	"github.com/Kaginari/agent-one/wire"
)

// Check is one injected rule check (config supplies them): a command whose exit code is the
// reading. It runs at the end of every turn that wrote files, after the four checks.
type Check struct {
	Name    string
	Command string
	Timeout time.Duration
	Scope   string // all | rank:<rank> | member:<name>; "" = all
}

// GateOptions is policy.gate: the Domain owner's four checks at turn end, each its own switch, plus the
// injected checks and whether the verdict is recorded.
type GateOptions struct {
	Enabled        bool
	RightAuthor    bool
	InvariantsHold bool
	DutiesDone     bool
	DocTruthful    bool
	Checks         []Check
	Log            bool // append the verdict to log.md
	Retries        int  // a failed gate goes back to the model this many times per turn
	TestsIntact    bool // a turn may not pass by deleting, skipping or narrowing tests
	// UI lints the ui dir when the turn wrote under it (app.uiGate); nil = off
	UI func(wrote []string) []string
	// TestsBefore is the test files' text as the turn opened; the EndGate hook fills it per call.
	TestsBefore map[string]string
	Timeout     time.Duration // per verify command; 0 = 120s
	// VerifyBefore is each member's verify lines as the turn opened, when the caller took them
	// itself (a host whose gate runs in a fresh process); nil reads the roster as it stands
	VerifyBefore map[string][]string
	// Sandbox contains every verify line and check: no network, secrets scrubbed; nil runs them
	// with the scrubbed environment alone
	Sandbox *sandbox.Sandbox
}

// Verdict is one gate run.
type Verdict struct {
	Word    string   // pass | fail | n/a (no domain owners)
	Reasons []string // why it failed
	Holes   []string
	Checked []string // what ran
	// VerifyRun and VerifyRed count the verify lines the gate ran and those that came back red —
	// the value ledger's verifyGreenRate reads them.
	VerifyRun, VerifyRed int
}

func (v Verdict) String() string {
	if len(v.Reasons) == 0 {
		return v.Word
	}
	return v.Word + " — " + strings.Join(v.Reasons, "; ")
}

// Gate runs the checks over a turn's writes for an agent and returns the verdict. It is the
// EndGate hook's agent, exposed so a dispatcher can run it on its own account.
//
// Hard policy (Right-author, human decree 2026-09-30): no agent — including the orchestrator — writes
// inside another member's ownership unless that member's own README rides the same turn.
// Orchestrator owns no ownership, so the owner's doc is the lawful vehicle for any cross-ownership
// write from above; without it, Right-author fails (the old silent exemption is closed).
//
// Own-pair reconciliation (human decree 2026-10-01): the owner's doc is paired when EITHER arm
// holds — (1) it rides this turn's writes (the 2026-09-30 arm, unchanged), or (2) it exists on
// disk, differs from the clean git baseline, and is attributable to this session's own subagents
// via the subagent-write ledger. Pairing was per-turn, so the only Policy-7-lawful flow — a subagent
// authors the README this turn, the orchestrator lands the code next turn — could never pass: the
// gate re-ran every turn on the accumulated writes while the subagent's doc write had been
// subtracted from the orchestrator's bucket (ownWrites), a subtraction nothing ever gave back.
func (w *Workspace) Gate(ctx context.Context, s *loop.Session, opt GateOptions, as string, wrote []string, rep *wire.Report, isWire bool, ask string) Verdict {
	v := Verdict{Word: "pass"}
	if !opt.Enabled {
		return Verdict{Word: "off", Holes: []string{"policy.gate is off — the turn's writes were not checked"}}
	}
	// the verify lines as they stood before the turn: an agent edits its own doc in the same turn
	// (Docs-as-code), so the post-turn lines alone would let it rewrite the check it is judged by
	before := opt.VerifyBefore
	if before == nil {
		before = w.verifyLines()
	}
	_ = w.Reload() // docs may have changed this turn; the roster is read fresh
	fail := func(why string) { v.Reasons = append(v.Reasons, why) }
	domainOwners := w.GateHolders()
	if !domainOwners {
		v.Word = "Gate: n/a (no gate holder)"
		if policyTable(w) {
			v.Word = or(w.Lex.GateNA, v.Word)
		}
	}
	self := w.Member(as)
	touched := map[string]*Member{}
	if self != nil {
		touched[self.Name] = self
	}
	for _, rel := range wrote {
		owner := w.Owner(rel)
		if owner != nil {
			touched[owner.Name] = owner
		}
		// A member's own doc (<dir>/README.md) sits at the member's seat in the workspace's
		// address space: the orchestrator authors the skel at founding, the member itself edits it
		// after. Right-author never fires on it.
		memberDoc := owner != nil && owner.Dir != "" && normRel(rel) == normRel(owner.Dir+"/README.md")
		orchestratorAuthored := as == "" || as == Orchestrator || w.RankOf(as).Name == Orchestrator
		// pairing, three vehicles: the doc rides THIS turn's writes (arm 1), or a cross-session
		// subagent record proves it (arm 2), or it moved in an earlier wave this workspace witnessed
		// (arm 3, session-pair mark — 2026-10-07). docPaired is any-writer evidence;
		// pairedEarlier keeps the orchestrator-scoped reading the 2026-10-01/02 arms were provisioned with.
		docPaired := owner != nil && (w.subagentPairedDoc(s, owner) || w.docMovedEarlier(owner))
		pairedEarlier := orchestratorAuthored && docPaired
		// The orchestrator's own seat files — the system manifest and the policy — are orchestrator's
		// work product, of one class with a member's own doc. Owner's gate-holder fallback
		// would otherwise silently make domain owner own every unowned path under .agent-one/; the orchestrator
		// seat files are orchestrator's regardless of which agent Owner resolves.
		orchestratorSeat := normRel(rel) == normRel(w.Lex.WorkspaceDir+"/system.yaml") ||
			normRel(rel) == normRel(w.Lex.WorkspaceDir+"/"+w.Lex.Policy)
		// arm-2 discrimination (auditor verdict 2026-10-02, third strike closed 2026-10-04):
		// an orchestrator-authored write of an EXISTING member doc is exempt only while it is
		// not conclusively drifted off the clean git baseline (founding skel, identical,
		// no-git soil) — or a lane A/B subagent record pairs the drift (pairedEarlier).
		driftedUnpaired := false
		if memberDoc && orchestratorAuthored {
			d, ok := w.docDriftedFromBaseline(owner.Doc)
			driftedUnpaired = ok && d && !pairedEarlier
		}
		// 1. right zone worker authored
		if opt.RightAuthor && domainOwners {
			switch {
			case orchestratorSeat && orchestratorAuthored:
				// the orchestrator's own work, on its own seat — no agent's authorship to check
			case memberDoc && as == owner.Name:
				// self-edit (member): the member's doc is not a work product to police
				// for authorship
			case memberDoc && orchestratorAuthored && !driftedUnpaired:
				// founding skel or subagent-paired re-authoring (orchestrator): clean, identical, no-git
				// soil, or a lane A/B subagent record stands behind the drift — the member's doc
				// is not a work product to police for authorship
			case memberDoc && orchestratorAuthored:
				// third strike closed (auditor verdict 2026-10-02): drifted off the clean
				// baseline with no subagent record in any lane — the orchestrator re-authored an
				// existing member doc without pairing evidence
				fail(fmt.Sprintf("%s: %s is %s's doc, re-authored by the orchestrator with no pairing evidence (drifted off the clean baseline, no subagent record in any lane)", w.Lex.Checks[0], rel, owner.Name))
			case orchestratorAuthored && owner != nil && (contains(wrote, owner.Doc) || pairedEarlier):
				// pure hard policy (human decree 2026-09-30, arm 1) — the orchestrator may cross a member's soil only carrying that member's doc in the same turn; orchestrator holds no ownership, so the owner README riding the same wave is the lawful vehicle; without it Right-author still fires.
				// own-pair reconciliation (decree 2026-10-01, arm 2; lane-B consult 2026-10-03) — or the doc landed a turn apart: the subagent-write ledger attributes it to a recorded subagent writer cross-session (lane B survives the subagent engine's Close), it is on disk and drifted off the clean baseline, so the pairing stands and this wave's cross-author is treated as paired
			case owner != nil && (self == nil || owner.Name != self.Name) && !(self != nil && self.InOwnership(rel)):
				// Right-author mortals: HARD — cross-ownership authoring is the Human's
				// 2026-09-30 decree and pair-marks don't soften it. The 2026-10-07 thrash class
				// (identical retries on identical fails) is answered by the circuit-breaker in
				// EndGate and the refusals naming the pairing doc, never by loosening this arm.
				fail(fmt.Sprintf("%s: %s belongs to %s, written by %s", w.Lex.Checks[0], rel, owner.Name, or(as, "orchestrator")))
			case owner == nil && self != nil && !self.InOwnership(rel) && len(self.Ownership) > 0:
				fail(fmt.Sprintf("%s: %s is outside %s's ownership", w.Lex.Checks[0], rel, self.Name))
			}
		}
		// 4. doc truthful — Docs-as-code. The same two pairing arms hold here: a turn's change under a
		// ownership is Docs-as-code-paired when the owner's doc changed in the same turn (arm 1) or
		// sits on disk drifted off the baseline with a cross-session subagent-write record behind
		// it (arm 2; lane B keyed by member name, kept at Close, bound 8 per member) —
		// otherwise the check double-fired on every retry for a doc the orchestrator cannot lawfully
		// write itself.
		if opt.DocTruthful && !orchestratorSeat && owner != nil && owner.Doc != "" && !contains(wrote, owner.Doc) && normRel(rel) != normRel(owner.Doc) && !docPaired {
			fail(fmt.Sprintf("%s (Docs-as-code): %s changed under %s's ownership but %s did not move — pair it: edit %s in this wave, or have %s's own subagent author its doc first (the doc names the pairing vehicle)", w.Lex.Checks[3], rel, owner.Name, owner.Doc, owner.Doc, owner.Name))
		}
	}
	// Spend the session pair-marks this wave rode (arm 3 is transactional): an ownership whose
	// code moved under a mark, WITHOUT moving the doc in this same wave, consumes the mark —
	// one doc wave buys exactly one later wave, never a permanent exemption.
	if len(w.pairMarks) > 0 {
		var spend []string
		for _, rel := range wrote {
			owner := w.Owner(rel)
			if owner == nil || owner.Dir == "" || contains(wrote, owner.Doc) {
				continue
			}
			doc := normRel(owner.Dir + "/README.md")
			if _, ok := w.pairMarks[doc]; ok && !contains(spend, doc) {
				spend = append(spend, doc)
			}
		}
		if len(spend) > 0 {
			w.mu.Lock()
			for _, d := range spend {
				delete(w.pairMarks, d)
			}
			w.mu.Unlock()
		}
	}
	v.Checked = append(v.Checked, w.Lex.Checks[0], w.Lex.Checks[3])
	// 2. invariants hold — verify commands are readings
	if opt.InvariantsHold {
		names := make([]string, 0, len(touched))
		for n := range touched {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			now := touched[n].Verify
			cmds := append([]string(nil), before[n]...)
			for _, c := range before[n] {
				if !contains(now, c) {
					v.Holes = append(v.Holes, fmt.Sprintf("%s: %s's verify `%s` was removed or changed this turn — the pre-turn line still ran; the gate holder confirms the change", w.Lex.Checks[1], n, c))
				}
			}
			// a line added this turn is the model's own command: it runs once the gate holder has
			// confirmed it, from the next turn on, never on the turn that wrote it
			for _, c := range now {
				if !contains(cmds, c) {
					v.Holes = append(v.Holes, fmt.Sprintf("%s: %s's verify `%s` was added this turn — not run; the gate holder confirms it and it runs from the next turn", w.Lex.Checks[1], n, c))
				}
			}
			for _, cmd := range cmds {
				v.VerifyRun++
				if code, tail := w.run(ctx, opt.Sandbox, cmd, opt.Timeout); code != 0 {
					v.VerifyRed++
					fail(fmt.Sprintf("%s: %s verify `%s` exit %d%s", w.Lex.Checks[1], n, cmd, code, tail))
				}
			}
		}
		v.Checked = append(v.Checked, w.Lex.Checks[1])
	}
	// tests intact — the turn did not pass by making the tests easier
	if opt.TestsIntact && opt.TestsBefore != nil {
		for _, why := range w.testsIntact(opt.TestsBefore, wrote) {
			fail("Tests intact: " + why + " — only the operator decides a test goes")
		}
		v.Checked = append(v.Checked, "Tests intact")
	}
	// ui holds — the component system's lints on a turn that touched it
	if opt.UI != nil {
		for _, why := range opt.UI(wrote) {
			fail("UI system: " + why)
		}
		v.Checked = append(v.Checked, "UI system")
	}
	// 3. duties done — the commission is answered on the wire
	if opt.DutiesDone {
		c := wire.ParseCommission(ask)
		commissioned := c.Ask != "" || c.Unsaid || c.Scope != ""
		switch {
		case commissioned && !isWire:
			fail(w.Lex.Checks[2] + ": the commission asked on the wire and the answer carries no @S")
		case isWire && rep != nil && strings.TrimSpace(rep.Status) == "":
			fail(w.Lex.Checks[2] + ": @S is empty")
		case commissioned && c.Unsaid && rep != nil && len(rep.Unsaid) == 0:
			v.Holes = append(v.Holes, w.Lex.Checks[2]+": +unsaid was asked and no @U came back")
		}
		v.Checked = append(v.Checked, w.Lex.Checks[2])
	}
	// injected checks, those whose scope reaches this agent
	rank := w.RankOf(as).Name
	if rank == Orchestrator {
		rank = ""
	}
	for _, ck := range opt.Checks {
		if strings.TrimSpace(ck.Command) == "" || !(Rule{Scope: ck.Scope}).Applies(as, rank) {
			continue
		}
		if code, tail := w.run(ctx, opt.Sandbox, ck.Command, ck.Timeout); code != 0 {
			fail(fmt.Sprintf("check %s: `%s` exit %d%s", or(ck.Name, "unnamed"), ck.Command, code, tail))
		}
		v.Checked = append(v.Checked, "check "+or(ck.Name, ck.Command))
	}
	if len(v.Reasons) > 0 {
		v.Word = "fail"
	}
	return v
}

// run is one gate reading: under the sandbox with the network off and secrets scrubbed, killed
// with everything it started when the timeout passes.
func (w *Workspace) run(ctx context.Context, sb *sandbox.Sandbox, command string, timeout time.Duration) (int, string) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := sb.Command(cctx, sandbox.Call{Cwd: w.Root}, nil, "bash", "-c", command)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	if cctx.Err() == context.DeadlineExceeded {
		code = 124
	}
	tail := strings.TrimSpace(out.String())
	if len(tail) > 160 {
		tail = "…" + tail[len(tail)-160:]
	}
	if tail != "" {
		tail = " — " + oneLine(tail)
	}
	return code, tail
}

// EndGate is the loop hook: the gate over the turn, the verdict appended to log.md, a fail
// returned as an error so the turn fails (Policy 3: nothing lands without the gate).
func (w *Workspace) EndGate(opt GateOptions) func(ctx context.Context, s *loop.Session, r *loop.Result) (string, []string, error) {
	return func(ctx context.Context, s *loop.Session, r *loop.Result) (string, []string, error) {
		as := s.Engine.As
		if as == "" {
			as = s.Engine.Lexicon.Agent
		}
		// a Subagent's writes were gated on its own account: the dispatcher gates only its own
		s.Wrote = w.ownWrites(s)
		if len(s.Wrote) == 0 {
			return "", nil, nil
		}
		o := opt
		o.TestsBefore = s.TestsBefore
		v := w.Gate(ctx, s, o, as, s.Wrote, &r.Report, r.IsWire, s.Ask)
		// the council circuit-breaker (freeze review 2026-10-07): the same gate failing on
		// the same file-set for the same agent is a spec defect, not an authoring skill —
		// after breakerAt repeats, the fail text hardens into the only lawful next acts.
		w.breaker(as, s.Wrote, &v)
		w.noteGate(as, v, s.Wrote) // the value ledger's fold takes it with the agent's report
		holes := append([]string(nil), v.Holes...)
		if opt.Log && opt.Enabled {
			result := "done"
			if v.Word == "fail" {
				result = "failed — gate"
			}
			title := "gate " + v.Word
			if !isVerdictWord(v.Word) {
				title = v.Word // already `Gate: n/a (no domain owners)`
			}
			e := Entry{Author: or(as, "orchestrator"), Title: title + " — " + oneLine(firstLine(s.Ask)), Task: firstLine(s.Ask), Files: s.Wrote, Gate: v.String(), Result: result}
			if r.IsWire {
				for _, u := range r.Report.Unsaid {
					e.Learned = append(e.Learned, "@U "+u.Kind+" "+u.Text)
				}
			}
			if err := w.AppendLog(e); err != nil {
				holes = append(holes, "log.md: "+err.Error())
			}
		}
		if v.Word == "fail" {
			return v.String(), holes, fmt.Errorf("gate fail — %s", strings.Join(v.Reasons, "; "))
		}
		if v.Word == "pass" {
			// pair policy (freeze review 2026-10-07): a doc that moved and PASSED policy marks the
			// pairing window — later waves that touch the ownership without re-moving the doc pair
			// against the mark, once (the gate spends marks after its wrote loop). Failures never
			// register marks: a lawful wave is the only lawful evidence.
			for _, rel := range s.Wrote {
				if owner := w.Owner(rel); owner != nil && owner.Dir != "" && normRel(rel) == normRel(owner.Dir+"/README.md") {
					w.mu.Lock()
					w.pairMarks[normRel(owner.Dir+"/README.md")] = time.Now()
					w.mu.Unlock()
				}
			}
		}
		return v.String(), holes, nil
	}
}

// gateStreak is one agent's circuit-breaker reading: how often the same signature
// (sorted file-set) failed its gate in a row. A pass or a differently-shaped wave resets it.
type gateStreak struct {
	Sig string
	N   int
}

// gateBreakerAt: the streak that says the spec is broken, not the agent.
const gateBreakerAt = 3

// breaker stamps the circuit-breaker directive into a streaked fail verdict. It refuses the
// thrash loop the way the policy affords: naming the only lawful next acts in the fail text the
// model reads (rewrite the commission with each member's doc duty, @SPLIT, or escalate);
// a dispatch-side hard veto is the next slice, flagged in the freeze wrap-up.
func (w *Workspace) breaker(as string, wrote []string, v *Verdict) {
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.gateStreaks[as]
	if st == nil {
		st = &gateStreak{}
		w.gateStreaks[as] = st
	}
	if v.Word == "pass" {
		st.N, st.Sig = 0, ""
		return
	}
	if v.Word != "fail" {
		return
	}
	sorted := append([]string(nil), wrote...)
	sort.Strings(sorted)
	sig := strings.Join(sorted, "|")
	if st.Sig == sig {
		st.N++
	} else {
		st.Sig, st.N = sig, 1
	}
	if st.N >= gateBreakerAt {
		v.Reasons = append(v.Reasons, fmt.Sprintf("council circuit-breaker — this same gate failed %d times in a row on this exact file-set for %s; the spec is broken, not the agent. Lawful next acts: rewrite the commission naming each member's doc duty, @SPLIT it narrower, or escalate to the human. Retrying identically burns the subagent clock.", st.N, as))
	}
}

// AppendLog appends one entry to the workspace's log (Policy 4).
func (w *Workspace) AppendLog(e Entry) error {
	l, err := OpenLog(w.LogPath())
	if err != nil {
		return err
	}
	return l.Append(e)
}

// LogPath is <root>/<workspaceDir>/log.md.
func (w *Workspace) LogPath() string { return w.Dir() + "/" + w.Lex.Log }

// domain owners is true for a verdict word from a workspace with a gate holder (pass | fail), false for the
// no-gate-holder line, which already reads `Gate: n/a (…)`.
func isVerdictWord(word string) bool { return word == "pass" || word == "fail" }

// docMovedEarlier is arm 3 of pairing (session-pair): the member's own doc moved in a
// witnessed earlier wave this process — the mark. The gate spends marks after the wrote loop
// (see Gate), so a rider re-marks only by moving the doc again.
func (w *Workspace) docMovedEarlier(owner *Member) bool {
	if owner == nil || owner.Dir == "" {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.pairMarks[normRel(owner.Dir+"/README.md")]
	return ok
}

// subagentPairedDoc is arm 2 of own-pair reconciliation (decree 2026-10-01): the owner's doc is
// attributable to a recorded subagent write of this workspace's agents AND sits on disk drifted off
// the clean git baseline. The subagent-write ledger (workspace/subagent.go subagentGated) records what a
// Subagent wrote and gated on its own account — lane A per-engine (dies at the engine's Close),
// lane B cross-session keyed by member name (Close keeps it; subagent.go subagentWrites). Arm 2
// first reads lane A on the session's own engine; when that lane has closed it consults lane
// B — the asked-for pairing narrows to the ledger record itself: the doc write the orchestrator is
// being asked to re-do is exactly the write the record attributes to its writer member. The baseline leg
// is git's clean tree (HEAD when tracked, staged copy when new): a doc identical to its clean
// state is a pre-existing doc nobody re-authored, never this session's pairing. The
// refnames leg ("refs/notes/..." HEAD) refuses a detached or in-flux work tree rather than
// guess. No work tree at all (no ledger hit can be proven clean-or-drifted against git) is
// conservative: no pairing, the same verdict as before the decree.
func (w *Workspace) subagentPairedDoc(s *loop.Session, owner *Member) bool {
	if s == nil {
		return false
	}
	w.mu.Lock()
	rec, subagent := w.subagentWrote[s.Engine][normRel(owner.Doc)]
	// Lane B (2026-10-03 reshoot-final): the per-engine lane dies at Close. No session
	// walks workspace (no w.Par/w.Sessions — the commission's pseudonyms unmet), so the
	// narrowest truthful consult is the workspace-resident name lane itself: the asking
	// session's member, and the owner whose doc the orchestrator re-authors.
	if !subagent {
		if m := w.subagentWrites[owner.Name]; m != nil && m[normRel(owner.Doc)].Writer != "" {
			rec, subagent = m[normRel(owner.Doc)], true
		}
	}
	if !subagent && s.Engine.As != "" && s.Engine.As != owner.Name {
		if m := w.subagentWrites[s.Engine.As]; m != nil && m[normRel(owner.Doc)].Writer != "" {
			rec, subagent = m[normRel(owner.Doc)], true
		}
	}
	w.mu.Unlock()
	if !subagent {
		return false // never touched by this session's subagents
	}
	if rec.Writer == "" {
		return false // recorded without a writer: no attribution to re-derive pairing from
	}
	drifted, ok := w.docDriftedFromBaseline(owner.Doc)
	return ok && drifted
}

// docDriftedFromBaseline reads whether the doc differs from its clean git baseline:
// (drifted, true) when the reading is conclusive, (false, false) when git cannot speak.
func (w *Workspace) docDriftedFromBaseline(rel string) (drifted bool, ok bool) {
	// the path as written: normRel folds case for comparing keys, and a case-sensitive
	// filesystem (Linux) would read the folded name as a missing file
	rel = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rel)), "./")
	// refs/heads-style refnames tell a clean work tree; a detached HEAD or an in-flux rebase
	// is not a baseline to judge drift against — refuse rather than guess.
	if out, err := w.gitOut("symbolic-ref -q --short HEAD"); err != nil || strings.TrimSpace(out) == "" {
		return false, false
	}
	cur, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
	if err != nil {
		return false, false // gone from disk: not on-disk, never a pairing
	}
	base, berr := w.gitOut("show HEAD:./" + rel)
	if berr != nil {
		// untracked (new file): the staged copy is the clean baseline when one exists, else
		// absent — an untracked, unstaged doc is drift by definition (its clean baseline is
		// nothing), which still only pairs when the ledger attributes it to a subagent.
		if staged, serr := w.gitOut("show :" + rel); serr == nil {
			return string(cur) != staged, true
		}
		return true, true
	}
	return string(cur) != base, true
}

// gitOut runs one git command in the workspace's root, returning stdout; an error means git
// could not speak (no work tree, unknown revision, in-flux state).
func (w *Workspace) gitOut(args string) (string, error) {
	c := exec.Command("git", strings.Fields(args)...)
	c.Dir = w.Root
	out, err := c.Output()
	return string(out), err
}

func policyTable(w *Workspace) bool {
	d := DefaultRanks(w.Lex)
	if len(d) != len(w.Ranks) {
		return false
	}
	for i := range d {
		if d[i].Name != w.Ranks[i].Name {
			return false
		}
	}
	return true
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}

// VerifyLines is each member's verify commands as the docs hold them now, read fresh.
func (w *Workspace) VerifyLines() map[string][]string {
	_ = w.Reload()
	return w.verifyLines()
}

// verifyLines is each member's verify commands as the roster holds them now.
func (w *Workspace) verifyLines() map[string][]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[string][]string{}
	for _, c := range w.Members {
		out[c.Name] = append([]string(nil), c.Verify...)
	}
	return out
}
