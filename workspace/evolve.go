package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/onto"
	"github.com/Kaginari/agent-one/sandbox"
	"github.com/Kaginari/agent-one/value"
)

// Evolution, coded (ranks.md §Evolution; AGENT-ONE.md Principles 2–5 and the adaptation loop): a
// member that is stressed — its council strain at stressAt, its last failStreak gate verdicts
// failed, or its last run ended in a doom checkpoint — AND whose Notes working notes (its own doc's
// plus its held skills') holds more than notesMax entries heals itself in the same wave, by one
// of three arms chosen from evidence, deterministically:
//
// split   — its ownership is large and its failures and Notes cluster in two or more
//           sub-directories: two young each take a half with the Notes that belong to it, the
//           parent retires (zone workers and domain owners; a domain owner's zone workers are divided with it);
// rebirth — its own configuration is stale (its verify lines went red at its gate, or its doc names
//           soil that is gone): a fresh doc from the code as it is now, plus the consolidated
//           lessons; the old doc rests in the attic and its value record starts again;
// mutate  — the Notes repeat a lesson: each family is consolidated into an invariant (and a verify
//           line when tests cover the paths it names); a recurring "how" is registered as a crafted
//           skill the member holds. Same name, same ownership.
//
// Every arm is a plan of file writes, staged under <workspace>/tmp/evolution/<id>/, proven before it
// lands — the new verify lines run green, and the ontology read over an overlay of the planned
// docs carries no finding the workspace did not already carry and gives every agent the ownership the
// plan meant — and only then swapped in. A plan that fails its proof keeps the old docs and lands
// a hole. Every evolution is logged in log.md (Policy 4), noted in the value ledger (`evolution`,
// and `rebirth` for a reborn record), and announced to the surfaces. In propose mode the plan is
// queued under <workspace>/evolution/ for the human (`evolve list|show|apply|reject`).

// EvolutionOptions is policy.evolution as the workspace reads it.
type EvolutionOptions struct {
	Enabled      bool
	Mode         string // auto | propose ("" = auto)
	NotesMax     int    // the working notes past which a stressed member evolves; 0 = off
	StressAt     int    // the strain that reads stressed (the caller resolves policy.stress.escalateAt)
	FailStreak   int    // or this many gated runs failed in a row (0 = that reading is off)
	CooldownRuns int    // runs of its own between two evolutions of one agent
	Arms         []string
	SplitLines   int // an ownership this many source lines reads large (0 = never by lines)
	SplitFiles   int // or this many files (0 = never by files)
	SkillStatus  string
	// Survey reads an ownership from the code (the founding sketch's per-ownership reading, wired
	// by the integrator); nil counts files and lines alone.
	Survey func(prefixes []string) OwnershipSurvey
	// Sandbox and VerifyTimeout contain the verify lines a plan runs, the gate's way.
	Sandbox       *sandbox.Sandbox
	VerifyTimeout time.Duration
	// On hears every evolution (applied, proposed or a hole) — the stream, the terminal, the board.
	On func(EvolveEvent)
}

// OwnershipSurvey is one ownership read from the code.
type OwnershipSurvey struct {
	Files      int
	Lines      int
	Purpose    string
	Invariants []string // the reading, each marked "sketch:"
	Verify     []string // the test command, only when tests exist there and a runner is known
}

// EvolveEvent is one evolution as the surfaces see it.
type EvolveEvent struct {
	ID       string    `json:"id"`
	Agent    string    `json:"agent"`
	Arm      string    `json:"arm"` // split | mutate | rebirth
	Into     []string  `json:"into,omitempty"`
	Reason   string    `json:"reason"`
	Evidence []string  `json:"evidence,omitempty"`
	Mode     string    `json:"mode"`   // auto | propose
	Status   string    `json:"status"` // applied | proposed | hole | rejected
	Hole     string    `json:"hole,omitempty"`
	Files    []string  `json:"files,omitempty"`
	Skill    string    `json:"skill,omitempty"`
	At       time.Time `json:"at"`
}

// Line is the event in one line, the way the report and the terminal say it.
func (e EvolveEvent) Line() string {
	switch e.Status {
	case "proposed":
		return fmt.Sprintf("%s proposes to evolve: %s — %s (evolve show %s)", e.Agent, e.Arm, e.Reason, e.ID)
	case "hole":
		return fmt.Sprintf("%s could not evolve (%s): %s — the old doc stands", e.Agent, e.Arm, e.Hole)
	case "rejected":
		return fmt.Sprintf("%s's %s proposal %s was rejected", e.Agent, e.Arm, e.ID)
	}
	switch e.Arm {
	case "split":
		return fmt.Sprintf("%s evolved: split → %s", e.Agent, strings.Join(e.Into, ", "))
	case "rebirth":
		return fmt.Sprintf("%s evolved: reborn from the code as it is now", e.Agent)
	}
	if e.Skill != "" {
		return fmt.Sprintf("%s mutated its configuration (and holds %s)", e.Agent, e.Skill)
	}
	return e.Agent + " mutated its configuration"
}

// evoState is the evolution machinery, under its own lock: one plan proves and lands at a time.
type evoState struct {
	run    sync.Mutex // serialises a plan's proof and swap
	mu     sync.Mutex
	opt    EvolutionOptions
	events []EvolveEvent
	news   []EvolveEvent // not yet announced to a dispatcher
}

// SetEvolution arms (or re-arms) policy.evolution.
func (w *Workspace) SetEvolution(o EvolutionOptions) {
	if o.Mode == "" {
		o.Mode = "auto"
	}
	w.evo.mu.Lock()
	w.evo.opt = o
	w.evo.mu.Unlock()
}

// Evolution is policy.evolution as armed.
func (w *Workspace) Evolution() EvolutionOptions {
	w.evo.mu.Lock()
	defer w.evo.mu.Unlock()
	return w.evo.opt
}

func (o EvolutionOptions) on() bool { return o.Enabled && o.NotesMax > 0 && len(o.Arms) > 0 }

func (o EvolutionOptions) arm(a string) bool { return contains(o.Arms, a) }

// Evolutions is this session's evolutions, oldest first.
func (w *Workspace) Evolutions() []EvolveEvent {
	w.evo.mu.Lock()
	defer w.evo.mu.Unlock()
	return append([]EvolveEvent(nil), w.evo.events...)
}

// evolveLines drains the evolutions since the last call — a dispatcher's report carries them.
func (w *Workspace) evolveLines() []string {
	w.evo.mu.Lock()
	news := w.evo.news
	w.evo.news = nil
	w.evo.mu.Unlock()
	var out []string
	for _, e := range news {
		if e.Status == "hole" {
			out = append(out, "@? evolution hole — "+e.Line())
			continue
		}
		out = append(out, "[evolution] "+e.Line()+" — "+e.Reason+" (log.md)")
	}
	return out
}

func (w *Workspace) announce(e EvolveEvent) {
	w.evo.mu.Lock()
	w.evo.events = append(w.evo.events, e)
	w.evo.news = append(w.evo.news, e)
	on := w.evo.opt.On
	w.evo.mu.Unlock()
	if on != nil {
		on(e)
	}
}

// --- the trigger ---

// Stress is an agent's stress reading: whether it reads stressed and on what evidence.
type Stress struct {
	Stressed bool
	Strain   int
	Streak   int  // gated runs failed in a row, newest first
	Doom     bool // its last run ended in a doom checkpoint
	Why      []string
}

// StressOf reads one agent's stress from the instruments: the council's strain, its gate streak,
// the doom checkpoint.
func (w *Workspace) StressOf(agent string) Stress {
	o := w.Evolution()
	w.mu.Lock()
	st := Stress{Strain: w.stress[agent]}
	w.mu.Unlock()
	if o.StressAt > 0 && st.Strain >= o.StressAt {
		st.Why = append(st.Why, fmt.Sprintf("strain %d ≥ stressAt %d", st.Strain, o.StressAt))
	}
	runs := w.ownRuns(agent)
	for i := len(runs) - 1; i >= 0; i-- {
		g := runs[i].Gate
		if g != "pass" && g != "fail" {
			continue
		}
		if g == "pass" {
			break
		}
		st.Streak++
	}
	if o.FailStreak > 0 && st.Streak >= o.FailStreak {
		st.Why = append(st.Why, fmt.Sprintf("its last %d gate verdicts failed", st.Streak))
	}
	if n := len(runs); n > 0 && value.Word(runs[n-1].Loop) == loop.Checkpoint {
		st.Doom = true
		st.Why = append(st.Why, "its last run ended in a doom checkpoint")
	}
	st.Stressed = len(st.Why) > 0
	return st
}

// ownRuns is an agent's run rows since its last rebirth, oldest first.
func (w *Workspace) ownRuns(agent string) []value.Row {
	rows := w.Ledger()
	var since time.Time
	for _, r := range rows {
		if r.Kind == value.KindRebirth && r.Agent == agent && r.At.After(since) {
			since = r.At
		}
	}
	var out []value.Row
	for _, r := range rows {
		if r.Kind == value.KindRun && r.Agent == agent && !r.At.Before(since) {
			out = append(out, r)
		}
	}
	return out
}

// Cooldown is where an agent stands against policy.evolution.cooldownRuns: its last evolution row
// and the runs of its own since.
type Cooldown struct {
	Last      *value.Row
	RunsSince int
	Left      int // runs still to go; 0 = it may evolve
}

// CooldownOf reads an agent's cooldown from the value ledger (persisted, replayed at Open).
func (w *Workspace) CooldownOf(agent string) Cooldown {
	o := w.Evolution()
	rows := w.Ledger()
	var cd Cooldown
	for i := range rows {
		if rows[i].Kind == value.KindEvolve && rows[i].Agent == agent {
			r := rows[i]
			cd.Last = &r
		}
	}
	if cd.Last == nil {
		return cd
	}
	for _, r := range rows {
		if r.Kind == value.KindRun && r.Agent == agent && r.At.After(cd.Last.At) {
			cd.RunsSince++
		}
	}
	if left := o.CooldownRuns - cd.RunsSince; left > 0 {
		cd.Left = left
	}
	return cd
}

// workingNotesEntry is one Note on one of a member's working notes.
type workingNotesEntry struct {
	Src   string // root-relative doc the working notes sits in
	Skill string // the held skill's name; "" for the member's own doc
	memory.WorkingNote
}

// WorkingNotesOf counts a member's Notes the creed's way: its own doc's working notes plus the working notes of every
// Skill it holds that lives in this workspace (the hat that did the work holds what the work taught).
func (w *Workspace) WorkingNotesOf(name string) int {
	c := w.Member(name)
	if c == nil {
		return 0
	}
	return len(w.workingNotesOf(c))
}

func (w *Workspace) workingNotesOf(c *Member) []workingNotesEntry {
	var out []workingNotesEntry
	read := func(rel, skill string) {
		b, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
		if err != nil {
			return
		}
		for _, t := range memory.NotesOf(string(b)) {
			out = append(out, workingNotesEntry{Src: rel, Skill: skill, WorkingNote: t})
		}
	}
	if c.Doc != "" {
		read(c.Doc, "")
	}
	for _, m := range c.Skills {
		if p := w.skillDoc(m); p != "" {
			read(p, m)
		}
	}
	return out
}

// skillDoc is a Skill's SKILL.md in the workspace's own registry (crafted, base, the hosts' project
// dirs), root-relative; "" when it lives nowhere under the root.
func (w *Workspace) skillDoc(name string) string {
	for _, base := range skillBases(w.Lex.WorkspaceDir) {
		rel := filepath.ToSlash(filepath.Join(base, name, "SKILL.md"))
		if _, err := os.Stat(filepath.Join(w.Root, filepath.FromSlash(rel))); err == nil {
			return rel
		}
	}
	return ""
}

// EvolveSweep is the run-start pass: every agent on the roster is weighed once.
func (w *Workspace) EvolveSweep() []EvolveEvent {
	if !w.Evolution().on() {
		return nil
	}
	var names []string
	for _, c := range w.Roster() {
		if c.Doc != "" {
			names = append(names, c.Name)
		}
	}
	sort.Strings(names)
	var out []EvolveEvent
	for _, n := range names {
		if ev, ok := w.EvolveMaybe(n); ok {
			out = append(out, ev)
		}
	}
	return out
}

// EvolveMaybe weighs one agent: stressed AND its working notes past notesMax AND out of cooldown AND no
// proposal of it waiting → an arm is chosen and applied (auto) or queued (propose).
func (w *Workspace) EvolveMaybe(agent string) (EvolveEvent, bool) {
	o := w.Evolution()
	if !o.on() {
		return EvolveEvent{}, false
	}
	w.evo.run.Lock()
	defer w.evo.run.Unlock()
	c := w.Member(agent)
	if c == nil || c.Doc == "" || c.Name == Orchestrator {
		return EvolveEvent{}, false
	}
	st := w.StressOf(c.Name)
	if !st.Stressed {
		return EvolveEvent{}, false
	}
	workingNotes := w.workingNotesOf(c)
	if len(workingNotes) <= o.NotesMax {
		return EvolveEvent{}, false
	}
	if cd := w.CooldownOf(c.Name); cd.Left > 0 {
		return EvolveEvent{}, false
	}
	for _, p := range w.Proposals() {
		if p.Agent == c.Name && p.Status == "open" {
			return EvolveEvent{}, false
		}
	}
	evidence := append(append([]string(nil), st.Why...), fmt.Sprintf("its working notes holds %d Notes > notesMax %d", len(workingNotes), o.NotesMax))
	plan, err := w.planEvolution(c, workingNotes, evidence)
	if err != nil {
		ev := EvolveEvent{Agent: c.Name, Arm: "none", Mode: o.Mode, Status: "hole", Hole: err.Error(), Evidence: evidence, At: time.Now().UTC()}
		w.evolutionHole(ev, nil)
		return ev, true
	}
	if o.Mode == "propose" {
		plan.Status = "open"
		if err := w.saveProposal(plan); err != nil {
			ev := plan.event("hole")
			ev.Hole = "proposal not written: " + err.Error()
			w.evolutionHole(ev, plan)
			return ev, true
		}
		w.noteEvolution(plan, "proposed")
		ev := plan.event("proposed")
		w.announce(ev)
		return ev, true
	}
	ev := w.applyPlan(context.Background(), plan)
	return ev, true
}

// --- the plan ---

// evoFile is one write a plan makes: the content it expects to find (Existed false = the path
// must not exist yet) and the content it lands.
type evoFile struct {
	Path    string `json:"path"`
	Existed bool   `json:"existed"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after"`
}

// EvolvePlan is one evolution before it lands: what it writes, what it must prove.
type EvolvePlan struct {
	ID       string    `json:"id"`
	Agent    string    `json:"agent"`
	Arm      string    `json:"arm"`
	Reason   string    `json:"reason"`
	Evidence []string  `json:"evidence"`
	Into     []string  `json:"into"`
	Files    []evoFile `json:"files"`
	// Verify is each agent's verify lines in the planned docs: they run green before the swap.
	Verify map[string][]string `json:"verify"`
	// Ownership is the soil each planned agent must derive in the overlay (the ontology's reading).
	Ownership map[string][]string `json:"ownership"`
	Skill     string              `json:"skill,omitempty"`
	WrapUp    []string            `json:"wrapUp,omitempty"` // Notes with no family: the log keeps them
	Rebirth   bool                `json:"rebirth,omitempty"`
	At        time.Time           `json:"at"`
	Mode      string              `json:"mode"`
	Status    string              `json:"status"` // open | applied | rejected | hole
	Why       string              `json:"why,omitempty"`
}

func (p *EvolvePlan) event(status string) EvolveEvent {
	var files []string
	for _, f := range p.Files {
		files = append(files, f.Path)
	}
	return EvolveEvent{ID: p.ID, Agent: p.Agent, Arm: p.Arm, Into: p.Into, Reason: p.Reason, Evidence: p.Evidence, Mode: p.Mode, Status: status, Files: files, Skill: p.Skill, At: time.Now().UTC()}
}

// write adds a file to the plan, reading what stands there now.
func (w *Workspace) planWrite(p *EvolvePlan, rel, after string) {
	rel = filepath.ToSlash(rel)
	for i := range p.Files {
		if p.Files[i].Path == rel {
			p.Files[i].After = after
			return
		}
	}
	f := evoFile{Path: rel, After: after}
	if b, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel))); err == nil {
		f.Existed, f.Before = true, string(b)
	}
	p.Files = append(p.Files, f)
}

// planned is the content the plan lands at rel, else what stands on disk.
func (w *Workspace) planned(p *EvolvePlan, rel string) string {
	for _, f := range p.Files {
		if f.Path == rel {
			return f.After
		}
	}
	b, _ := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
	return string(b)
}

// attic is where an old doc rests: <workspace>/attic/<name>@<date>.md (a second the same day -2, …).
func (w *Workspace) atticPath(p *EvolvePlan, name string) string {
	date := time.Now().UTC().Format("2006-01-02")
	for i := 1; ; i++ {
		base := name + "@" + date
		if i > 1 {
			base += fmt.Sprintf("-%d", i)
		}
		rel := filepath.ToSlash(filepath.Join(w.Lex.WorkspaceDir, "attic", base+".md"))
		taken := false
		for _, f := range p.Files {
			if f.Path == rel {
				taken = true
			}
		}
		if _, err := os.Stat(filepath.Join(w.Root, filepath.FromSlash(rel))); err == nil {
			taken = true
		}
		if !taken {
			return rel
		}
	}
}

// planEvolution chooses the arm from evidence and drafts it. The order is the creed's: a zone
// grown past one persistent agent is cut (split); a configuration the code contradicts is written again (rebirth);
// a working notes that repeats itself is consolidated (mutate); a working notes with no repetition is still consolidated
// (Principle 5 — past the limit every note finds its final form), as a mutate whose invariants are
// its families and whose wrap-up the log keeps.
func (w *Workspace) planEvolution(c *Member, workingNotes []workingNotesEntry, evidence []string) (*EvolvePlan, error) {
	o := w.Evolution()
	now := time.Now().UTC()
	p := &EvolvePlan{Agent: c.Name, Evidence: evidence, Mode: o.Mode, At: now, Verify: map[string][]string{}, Ownership: map[string][]string{}}
	h := sha256.Sum256([]byte(c.Name + "\x00" + now.Format(time.RFC3339Nano)))
	p.ID = hex.EncodeToString(h[:])[:10]
	var tried []string
	if o.arm("split") {
		if why, ok := w.splitEvidence(c, workingNotes); ok {
			p.Arm, p.Reason = "split", why
			if err := w.planSplit(p, c, workingNotes); err == nil {
				return p, nil
			} else {
				tried = append(tried, "split: "+err.Error())
			}
		} else {
			tried = append(tried, "split: "+why)
		}
	}
	if o.arm("rebirth") {
		if why, ok := w.rebirthEvidence(c); ok {
			p.Arm, p.Reason = "rebirth", why
			if err := w.planRebirth(p, c, workingNotes); err == nil {
				return p, nil
			} else {
				tried = append(tried, "rebirth: "+err.Error())
			}
		} else {
			tried = append(tried, "rebirth: "+why)
		}
	}
	if o.arm("mutate") {
		fams, _ := families(workingNotes, c)
		if len(fams) > 0 && len(fams[0].Members) >= 3 {
			p.Arm, p.Reason = "mutate", fmt.Sprintf("its Notes repeat a lesson: %d of %d name %q", len(fams[0].Members), len(workingNotes), fams[0].Term)
		} else {
			p.Arm, p.Reason = "mutate", fmt.Sprintf("no arm's own evidence held (%s); the working notes past its limit is consolidated to its final forms (Principle 5)", strings.Join(tried, "; "))
		}
		if err := w.planMutate(p, c, workingNotes); err != nil {
			return nil, err
		}
		return p, nil
	}
	return nil, fmt.Errorf("no arm policy.evolution allows fits: %s", strings.Join(tried, "; "))
}

// --- the proof and the swap ---

// applyPlan proves a plan and lands it: the workspace moved under it → hole; staged; verify green;
// the ontology over the overlay no worse and every agent on the soil the plan meant → swapped in,
// logged, noted, announced. Any failure keeps every old doc.
func (w *Workspace) applyPlan(ctx context.Context, p *EvolvePlan) EvolveEvent {
	fail := func(why string) EvolveEvent {
		p.Status, p.Why = "hole", why
		ev := p.event("hole")
		ev.Hole = why
		w.evolutionHole(ev, p)
		return ev
	}
	for _, f := range p.Files {
		b, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(f.Path)))
		switch {
		case f.Existed && (err != nil || string(b) != f.Before):
			return fail(f.Path + " changed since the plan was drawn — nothing written")
		case !f.Existed && err == nil:
			return fail(f.Path + " now stands on disk — the plan would overwrite it; nothing written")
		}
	}
	stage := filepath.Join(w.Dir(), "tmp", "evolution", p.ID)
	_ = os.RemoveAll(stage)
	for _, f := range p.Files {
		dst := filepath.Join(stage, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fail("staging: " + err.Error())
		}
		if err := os.WriteFile(dst, []byte(f.After), 0o644); err != nil {
			return fail("staging: " + err.Error())
		}
	}
	o := w.Evolution()
	ran, agents := 0, make([]string, 0, len(p.Verify))
	for b := range p.Verify {
		agents = append(agents, b)
	}
	sort.Strings(agents)
	for _, b := range agents {
		for _, cmd := range p.Verify[b] {
			ran++
			if code, tail := w.run(ctx, o.Sandbox, cmd, o.VerifyTimeout); code != 0 {
				return fail(fmt.Sprintf("%s's verify `%s` exit %d%s", b, cmd, code, tail))
			}
		}
	}
	if why := w.proveOverlay(p, stage); why != "" {
		return fail(why)
	}
	// the swap: every write lands, or every write is undone
	var done []evoFile
	for _, f := range p.Files {
		abs := filepath.Join(w.Root, filepath.FromSlash(f.Path))
		err := os.MkdirAll(filepath.Dir(abs), 0o755)
		if err == nil {
			err = os.WriteFile(abs, []byte(f.After), 0o644)
		}
		if err != nil {
			for _, d := range done {
				a := filepath.Join(w.Root, filepath.FromSlash(d.Path))
				if d.Existed {
					_ = os.WriteFile(a, []byte(d.Before), 0o644)
				} else {
					_ = os.Remove(a)
				}
			}
			return fail("swap: " + err.Error())
		}
		done = append(done, f)
	}
	_ = os.RemoveAll(stage)
	p.Status = "applied"
	// the zone changed: its strain is spent (a clean slate, like a clean landing)
	w.mu.Lock()
	for _, b := range append([]string{p.Agent}, p.Into...) {
		delete(w.stress, b)
		delete(w.consults, b)
		w.ledgerNote(ledgerRow{Kind: "stressClear", Agent: b})
	}
	w.mu.Unlock()
	w.noteEvolution(p, "applied")
	if p.Rebirth && w.val != nil {
		w.val.mu.Lock()
		w.appendValue(value.Row{Kind: value.KindRebirth, Agent: p.Agent, Rank: w.RankOf(p.Agent).Name, Arm: "rebirth", Reason: p.Reason})
		w.val.mu.Unlock()
	}
	_ = w.Reload()
	ev := p.event("applied")
	var files []string
	for _, f := range p.Files {
		files = append(files, f.Path)
	}
	learned := []string{"arm: " + p.Arm + " — " + p.Reason}
	if p.Skill != "" {
		learned = append(learned, "registered skill "+p.Skill+" (status "+or(o.SkillStatus, "trial")+"), held by "+p.Agent)
	}
	for _, t := range p.WrapUp {
		learned = append(learned, "wrap-up: "+t)
	}
	if err := w.AppendLog(Entry{At: ev.At, Author: p.Agent, Title: "evolution — " + ev.Line(),
		Task:    "policy.evolution (" + p.Mode + "): " + strings.Join(p.Evidence, "; "),
		Files:   files,
		Gate:    fmt.Sprintf("staged and proven before the swap: %d verify line%s green, the ontology no worse over the overlay", ran, plural(ran)),
		Result:  evolveResult(p),
		Learned: learned}); err != nil {
		w.mu.Lock()
		w.Notes = append(w.Notes, "evolution log: "+err.Error())
		w.mu.Unlock()
	}
	w.announce(ev)
	return ev
}

func evolveResult(p *EvolvePlan) string {
	switch p.Arm {
	case "split":
		return fmt.Sprintf("%s retired; %s hold its halves, each with the Notes that belong to it (the parent's doc rests in the attic too)", p.Agent, strings.Join(p.Into, " and "))
	case "rebirth":
		return p.Agent + " reborn: a fresh doc from the code and the consolidated lessons; the old doc rests in the attic; its value record starts again"
	}
	return p.Agent + "'s Notes consolidated into its configuration; same name, same ownership"
}

// noteEvolution is the value ledger's row: the cooldown counts from it.
func (w *Workspace) noteEvolution(p *EvolvePlan, status string) {
	if w.val == nil {
		return
	}
	var paths []string
	for _, f := range p.Files {
		paths = append(paths, f.Path)
	}
	w.val.mu.Lock()
	w.appendValue(value.Row{Kind: value.KindEvolve, Agent: p.Agent, Rank: w.RankOf(p.Agent).Name, Arm: p.Arm, Reason: p.Reason, Into: p.Into, Status: status, Paths: paths})
	w.val.mu.Unlock()
}

// evolutionHole keeps the old docs, logs the hole, notes the try (the cooldown counts from it, so
// a plan that cannot prove itself is not redrawn on every landing) and announces it.
func (w *Workspace) evolutionHole(ev EvolveEvent, p *EvolvePlan) {
	if p != nil {
		w.noteEvolution(p, "hole")
	} else if w.val != nil {
		w.val.mu.Lock()
		w.appendValue(value.Row{Kind: value.KindEvolve, Agent: ev.Agent, Rank: w.RankOf(ev.Agent).Name, Arm: ev.Arm, Reason: ev.Hole, Status: "hole"})
		w.val.mu.Unlock()
	}
	_ = w.AppendLog(Entry{Author: ev.Agent, Title: "evolution hole — " + ev.Agent, Task: "policy.evolution: " + strings.Join(ev.Evidence, "; "),
		Gate: "n/a (the harness's own act; nothing swapped)", Result: "hole: " + ev.Hole + " — the old doc stands", Learned: []string{"arm tried: " + ev.Arm}})
	w.announce(ev)
}

// proveOverlay reads the ontology over the root with the plan's files laid on top: no finding the
// workspace did not already carry, and every planned agent on exactly the soil the plan meant.
func (w *Workspace) proveOverlay(p *EvolvePlan, stage string) string {
	layout := w.Ranks.Layout(w.Lex)
	base, err := onto.LoadLayout(w.Root, layout)
	if err != nil {
		return "ontology (as it stands): " + err.Error()
	}
	before := map[string]bool{}
	for _, f := range base.Validate() {
		before[f.String()] = true
	}
	ov, err := os.MkdirTemp("", "agent-one-evolve-")
	if err != nil {
		return "overlay: " + err.Error()
	}
	defer os.RemoveAll(ov)
	var paths []string
	for _, f := range p.Files {
		paths = append(paths, f.Path)
	}
	if err := overlay(w.Root, ov, w.Lex.WorkspaceDir, paths); err != nil {
		return "overlay: " + err.Error()
	}
	for _, f := range p.Files {
		dst := filepath.Join(ov, filepath.FromSlash(f.Path))
		_ = os.Remove(dst)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "overlay: " + err.Error()
		}
		if err := os.WriteFile(dst, []byte(f.After), 0o644); err != nil {
			return "overlay: " + err.Error()
		}
	}
	ow, err := onto.LoadLayout(ov, layout)
	if err != nil {
		return "ontology over the planned docs: " + err.Error()
	}
	var fresh []string
	for _, f := range ow.Validate() {
		if s := f.String(); !before[s] {
			fresh = append(fresh, s)
		}
	}
	if len(fresh) > 0 {
		return "the ontology over the planned docs carries new findings: " + strings.Join(fresh, "; ")
	}
	got := map[string][]string{}
	for _, c := range membersOf(ov, ow, w.Ranks) {
		got[c.Name] = c.Ownership
	}
	for b, want := range p.Ownership {
		have, ok := got[b]
		if !ok {
			return b + " is not on the roster the planned docs derive"
		}
		if !sameSoil(have, want) {
			return fmt.Sprintf("%s derives %s from its planned doc, not %s — the ontology would not hold the claim", b, strings.Join(have, ", "), strings.Join(want, ", "))
		}
	}
	return ""
}

func sameSoil(a, b []string) bool {
	norm := func(xs []string) []string {
		var out []string
		for _, x := range xs {
			out = append(out, strings.TrimSuffix(normRel(strings.Trim(x, "` ")), "/"))
		}
		sort.Strings(out)
		return out
	}
	x, y := norm(a), norm(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// overlay mirrors root into dst for a read of the planned workspace: every top-level entry a symlink,
// except the workspace dir, which is real down to the member and skill dirs (the ontology lists
// directories, and a symlinked one does not list as a dir) with its files symlinked — so the
// planned files can be laid over it without touching the root.
func overlay(root, dst, workspaceDir string, paths []string) error {
	ents, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range ents {
		src := filepath.Join(root, e.Name())
		if e.Name() == workspaceDir {
			if err := mirror(src, filepath.Join(dst, e.Name()), 3); err != nil {
				return err
			}
			continue
		}
		if err := os.Symlink(src, filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	// a planned file under a symlinked tree (a skill's working notes under .claude/skills) needs its dirs real
	for _, p := range paths {
		parts := strings.Split(filepath.ToSlash(p), "/")
		cur := dst
		for _, part := range parts[:len(parts)-1] {
			next := filepath.Join(cur, part)
			if fi, err := os.Lstat(next); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(next)
				_ = os.Remove(next)
				if err := mirror(target, next, 1); err != nil {
					return err
				}
			}
			cur = next
		}
	}
	return nil
}

// skipMirror are the workspace dir's children that are read as a whole or never by the ontology.
var skipMirror = map[string]bool{"instruments": true, "tmp": true, "memory": true, "attic": true, "evolution": true}

// mirror makes dst a real dir holding src's entries: dirs real down to depth, everything else a
// symlink.
func mirror(src, dst string, depth int) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		return nil
	}
	for _, e := range ents {
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if e.IsDir() && depth > 1 && !skipMirror[e.Name()] {
			if err := mirror(s, d, depth-1); err != nil {
				return err
			}
			continue
		}
		if err := os.Symlink(s, d); err != nil {
			return err
		}
	}
	return nil
}

// --- the proposals (propose mode) ---

func (w *Workspace) proposalDir() string { return filepath.Join(w.Dir(), "evolution") }

func (w *Workspace) saveProposal(p *EvolvePlan) error {
	if err := os.MkdirAll(w.proposalDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(w.proposalDir(), p.ID+".json"), append(b, '\n'), 0o644)
}

// Proposals is the evolution queue, oldest first.
func (w *Workspace) Proposals() []*EvolvePlan {
	files, _ := filepath.Glob(filepath.Join(w.proposalDir(), "*.json"))
	var out []*EvolvePlan
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var p EvolvePlan
		if json.Unmarshal(b, &p) == nil && p.ID != "" {
			out = append(out, &p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// Proposal resolves an id (or a unique prefix of 4+ characters).
func (w *Workspace) Proposal(id string) (*EvolvePlan, error) {
	var hit []*EvolvePlan
	for _, p := range w.Proposals() {
		if p.ID == id {
			return p, nil
		}
		if len(id) >= 4 && strings.HasPrefix(p.ID, id) {
			hit = append(hit, p)
		}
	}
	switch len(hit) {
	case 1:
		return hit[0], nil
	case 0:
		return nil, fmt.Errorf("no evolution proposal %q", id)
	}
	return nil, fmt.Errorf("%q names %d proposals — give more of the id", id, len(hit))
}

// ApplyProposal lands a queued plan through the same proof an auto evolution passes.
func (w *Workspace) ApplyProposal(ctx context.Context, id string) (EvolveEvent, error) {
	p, err := w.Proposal(id)
	if err != nil {
		return EvolveEvent{}, err
	}
	if p.Status != "open" {
		return EvolveEvent{}, fmt.Errorf("proposal %s is %s — only an open one applies", p.ID, p.Status)
	}
	w.evo.run.Lock()
	ev := w.applyPlan(ctx, p)
	w.evo.run.Unlock()
	if err := w.saveProposal(p); err != nil {
		return ev, err
	}
	if ev.Status == "hole" {
		return ev, fmt.Errorf("%s", ev.Hole)
	}
	return ev, nil
}

// RejectProposal closes a queued plan with the human's reason; nothing is written but the mark.
func (w *Workspace) RejectProposal(id, why string) (*EvolvePlan, error) {
	p, err := w.Proposal(id)
	if err != nil {
		return nil, err
	}
	if p.Status != "open" {
		return nil, fmt.Errorf("proposal %s is %s — only an open one is rejected", p.ID, p.Status)
	}
	p.Status, p.Why = "rejected", why
	if err := w.saveProposal(p); err != nil {
		return nil, err
	}
	w.announce(p.event("rejected"))
	return p, nil
}

// Diff is the plan as line diffs, file by file (the proposal's show).
func (p *EvolvePlan) Diff() string {
	var b strings.Builder
	for _, f := range p.Files {
		if f.Existed {
			fmt.Fprintf(&b, "--- %s\n+++ %s\n", f.Path, f.Path)
		} else {
			fmt.Fprintf(&b, "--- /dev/null\n+++ %s\n", f.Path)
		}
		b.WriteString(lineDiff(f.Before, f.After))
	}
	return b.String()
}

// lineDiff is a minimal LCS line diff: unchanged lines with a space, removals -, additions +.
func lineDiff(a, b string) string {
	x, y := splitLines(a), splitLines(b)
	n, m := len(x), len(y)
	if n*m > 4_000_000 { // too large to align: show both whole
		var s strings.Builder
		for _, l := range x {
			s.WriteString("-" + l + "\n")
		}
		for _, l := range y {
			s.WriteString("+" + l + "\n")
		}
		return s.String()
	}
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var s strings.Builder
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case x[i] == y[j]:
			s.WriteString(" " + x[i] + "\n")
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			s.WriteString("-" + x[i] + "\n")
			i++
		default:
			s.WriteString("+" + y[j] + "\n")
			j++
		}
	}
	for ; i < n; i++ {
		s.WriteString("-" + x[i] + "\n")
	}
	for ; j < m; j++ {
		s.WriteString("+" + y[j] + "\n")
	}
	return s.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// evolveAfterRun is the subagent seam: after a report folds, the agent is weighed. It answers the
// report's lines and the files the harness wrote on the agent's behalf (the dispatcher's gate
// leaves them alone, like a subagent's own gated writes).
func (w *Workspace) evolveAfterRun(agent string) ([]string, []string) {
	ev, ok := w.EvolveMaybe(agent)
	if !ok {
		return nil, nil
	}
	var files []string
	if ev.Status == "applied" {
		files = ev.Files
	}
	return w.evolveLines(), files
}
