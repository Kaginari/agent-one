package workspace

import (
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/rank"
	"github.com/Kaginari/agent-one/value"
)

// The value policy, coded (ranks.md §Value). Every Subagent's report and every orchestrator turn folds one
// row into the value ledger (`<workspaceDir>/instruments/value.jsonl`), always — a workspace with no
// promotion still measures its agents. A gate verdict credits the agent that held the gate; a
// pass a later fail on the same paths undoes is an overturn. The ledger is replayed at Open; the
// promotion condition, the council's reading, the board's leaderboard and the meters all read
// the one fold (value.Fold) under the one definition of success (policy.value).

// ValueOptions is policy.value as the workspace reads it. The zero value keeps the ledger with the
// shipped definition over the whole record.
type ValueOptions struct {
	Off         bool          // policy.value.enabled: false — no row is written, nothing is measured
	Def         value.Def     // pass / fail / neutral words; zero = value.DefaultDef
	Window      time.Duration // the fold's window; 0 = the whole ledger
	WindowText  string        // the window as config wrote it (28d)
	ReopenTurns int           // an orchestrator pass rewritten within this many turns reopens
	MinRecord   int           // reports below which a score is low-confidence; 0 = value.DefaultMinRecord
	EscalateAt  int           // policy.stress.escalateAt: the strain that reads 100%
	Meters      map[string]value.MeterDef
}

// valueState is the ledger in memory, guarded by its own lock.
type valueState struct {
	mu       sync.Mutex
	rows     []value.Row
	passRefs map[string]value.Row // path → the standing pass verdict on it
	gates    map[string]gateNote  // agent → its last end-gate verdict, taken by the subagent's fold
	scores   map[string]*value.Expr
}

// gateNote is one end-gate verdict held for the fold.
type gateNote struct {
	Word      string
	Paths     []string
	VerifyRun int
	VerifyRed int
}

func (w *Workspace) valuePath() string {
	return filepath.Join(w.Dir(), filepath.FromSlash(value.Shelf))
}

// openValue seeds the ledger once from the legacy promotion tally, replays it, rebuilds the
// standing pass verdicts and marks the agents already promoted. An unreadable ledger opens the
// workspace with an empty record, never blocks it.
func (w *Workspace) openValue() {
	w.val = &valueState{passRefs: map[string]value.Row{}, gates: map[string]gateNote{}, scores: map[string]*value.Expr{}}
	if w.valOpt.Off {
		return
	}
	if n, err := value.Seed(w.Dir()); err != nil {
		w.Notes = append(w.Notes, "value ledger: "+err.Error())
	} else if n > 0 {
		w.Notes = append(w.Notes, "value ledger seeded from "+value.LegacyShelf)
	}
	rows, err := value.Read(w.valuePath())
	if err != nil {
		w.Notes = append(w.Notes, "value ledger: "+err.Error())
	}
	w.val.rows = rows
	for _, r := range rows {
		switch r.Kind {
		case value.KindVerdict:
			w.val.track(r)
		case value.KindPromote:
			if w.promote != nil {
				w.promote.done[r.Agent] = true
			}
		}
	}
}

// track keeps the standing pass verdicts: a pass stands on its paths until a fail overturns it
// (callers hold mu). It answers the passes this verdict overturns.
func (v *valueState) track(r value.Row) []value.Row {
	var undone []value.Row
	switch r.Status {
	case "pass":
		for _, p := range r.Paths {
			v.passRefs[p] = r
		}
	case "fail":
		seen := map[string]bool{}
		for _, p := range r.Paths {
			ref, ok := v.passRefs[p]
			if !ok {
				continue
			}
			delete(v.passRefs, p)
			id := ref.At.Format(time.RFC3339Nano) + "\x00" + ref.Agent
			if !seen[id] {
				seen[id] = true
				undone = append(undone, ref)
			}
		}
		// a pass overturned once is gone from every path it stood on
		for p, ref := range v.passRefs {
			for _, u := range undone {
				if ref.Agent == u.Agent && ref.At.Equal(u.At) {
					delete(v.passRefs, p)
				}
			}
		}
	}
	return undone
}

// appendValue writes one row to the ledger and the memory (callers hold val.mu).
func (w *Workspace) appendValue(r value.Row) {
	if w.valOpt.Off || w.val == nil {
		return
	}
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	if err := value.Append(w.valuePath(), r); err != nil {
		w.mu.Lock()
		w.Notes = append(w.Notes, "value ledger: "+err.Error())
		w.mu.Unlock()
	}
	w.val.rows = append(w.val.rows, r)
}

// RunFold is one run as the value ledger reads it.
type RunFold struct {
	Agent     string
	By        string // the dispatcher; "" for an orchestrator turn
	Loop      string // the loop's status
	Word      string // the agent's own @S word
	Gate      string // pass | fail | off | "" (nothing gated)
	GateTries int
	VerifyRun int
	VerifyRed int
	Holes     value.Holes
	Findings  int
	Strain    int
	Warrant   bool
	Guts      loop.GutsSum
	Tokens    int
	Paths     []string
	Turn      bool
}

// FoldRun appends one run to the ledger, then runs the promotion pass for its agent. The record
// belongs to the agent that ran; promotion reads its rank, never the dispatcher's.
func (w *Workspace) FoldRun(f RunFold) {
	if w.val == nil {
		return
	}
	row := value.Row{Kind: value.KindRun, Agent: f.Agent, Rank: w.RankOf(f.Agent).Name, By: f.By, Loop: value.Word(f.Loop), Word: value.Word(f.Word),
		Status: value.RunWord(f.Loop, f.Word), Gate: f.Gate, GateTries: f.GateTries, VerifyRun: f.VerifyRun, VerifyRed: f.VerifyRed,
		Holes: f.Holes, Findings: f.Findings, Strain: f.Strain, Warrant: f.Warrant, Thrash: f.Guts.Thrash, Momentum: f.Guts.Momentum,
		Guts: f.Guts.Read, Tokens: f.Tokens, Paths: f.Paths, Turn: f.Turn}
	w.val.mu.Lock()
	w.appendValue(row)
	w.val.mu.Unlock()
	w.promoteMaybe(f.Agent)
}

// FoldMetrics counts one report by its status word and harness-hole count — the bare fold a
// caller with no run in hand makes. A loop word (FAIL, CHECKPOINT, DENIED …) stands for the
// loop's status; any other word is the agent's own on a run the loop closed DONE.
func (w *Workspace) FoldMetrics(agent, status string, holes int) {
	loopStatus := "DONE"
	switch value.Word(status) {
	case loop.Fail, loop.Checkpoint, loop.Escalate, loop.Denied, loop.Dry, loop.Suspended, "INTERRUPTED":
		loopStatus = value.Word(status)
	}
	w.FoldRun(RunFold{Agent: agent, Loop: loopStatus, Word: status, Holes: value.Holes{Harness: holes}})
}

// FoldVerdict credits a gate verdict to the agent that held the gate; a fail overturns every
// standing pass on the same paths, each an overturn row against the holder that issued it.
func (w *Workspace) FoldVerdict(holder, of, word string, paths []string) {
	if w.val == nil || holder == "" || (word != "pass" && word != "fail") {
		return
	}
	row := value.Row{Kind: value.KindVerdict, Agent: holder, Rank: w.RankOf(holder).Name, Of: of, Status: word, Paths: append([]string(nil), paths...)}
	w.val.mu.Lock()
	defer w.val.mu.Unlock()
	if w.valOpt.Off {
		return
	}
	row.At = time.Now().UTC()
	w.appendValue(row)
	for _, ref := range w.val.track(row) {
		w.appendValue(value.Row{Kind: value.KindOverturn, Agent: ref.Agent, Rank: ref.Rank, Of: ref.Of, Ref: ref.At.Format(time.RFC3339Nano), Paths: ref.Paths})
	}
}

// noteGate holds an agent's end-gate verdict for the fold that takes its report.
func (w *Workspace) noteGate(as string, v Verdict, paths []string) {
	if w.val == nil {
		return
	}
	word := v.Word
	if word != "pass" && word != "fail" && word != "off" {
		word = "" // n/a: no gate holder — nothing was judged
	}
	w.val.mu.Lock()
	w.val.gates[as] = gateNote{Word: word, Paths: append([]string(nil), paths...), VerifyRun: v.VerifyRun, VerifyRed: v.VerifyRed}
	w.val.mu.Unlock()
}

// takeGate drains an agent's held verdict.
func (w *Workspace) takeGate(as string) (gateNote, bool) {
	if w.val == nil {
		return gateNote{}, false
	}
	w.val.mu.Lock()
	defer w.val.mu.Unlock()
	g, ok := w.val.gates[as]
	delete(w.val.gates, as)
	return g, ok
}

// gateHolderFor is the agent a verdict on `author`'s writes credits: the dispatcher when its rank
// holds the gate, else the author itself when it holds its own (a service owner), else no one.
func (w *Workspace) gateHolderFor(dispatcher, author string) string {
	if dispatcher != "" && w.RankOf(dispatcher).HoldsGate {
		return dispatcher
	}
	if w.RankOf(author).HoldsGate {
		return author
	}
	return ""
}

// foldResult folds a closed run: its words, its holes split by kind, its gate, its gut, its spend.
func (w *Workspace) foldResult(agent, by string, r *loop.Result, strain int, warrant, turn bool) {
	infra, harness, self := r.HoleCount()
	f := RunFold{Agent: agent, By: by, Loop: r.Status, Word: r.Report.Status, GateTries: r.GateTries,
		Holes: value.Holes{Infra: infra, Harness: harness, Self: self}, Strain: strain, Warrant: warrant,
		Guts: r.Guts, Tokens: r.Usage.Input + r.Usage.Output + r.Usage.CacheRead + r.Usage.CacheWrite, Paths: r.Wrote, Turn: turn}
	if r.Interrupted {
		f.Loop = "INTERRUPTED"
	}
	if r.IsWire {
		f.Findings = len(r.Report.Findings)
	}
	if g, ok := w.takeGate(agent); ok {
		f.Gate, f.VerifyRun, f.VerifyRed = g.Word, g.VerifyRun, g.VerifyRed
		if len(g.Paths) > 0 {
			f.Paths = g.Paths
		}
		if holder := w.gateHolderFor(by, agent); holder != "" && !turn {
			w.FoldVerdict(holder, agent, g.Word, g.Paths)
		}
	}
	w.FoldRun(f)
}

// turnEnded is the orchestrator's Ended hook: every human turn is a row of the root's record.
func (w *Workspace) turnEnded(as string) func(s *loop.Session, r *loop.Result) {
	return func(s *loop.Session, r *loop.Result) {
		w.mu.Lock()
		strain := w.stress[as]
		w.mu.Unlock()
		w.foldResult(as, "", r, strain, false, true)
	}
}

// Ledger is a copy of the value ledger's rows (for the board and tests).
func (w *Workspace) Ledger() []value.Row {
	if w.val == nil {
		return nil
	}
	w.val.mu.Lock()
	defer w.val.mu.Unlock()
	return append([]value.Row(nil), w.val.rows...)
}

// ValueOpts is the fold's tuning as the workspace reads it.
func (w *Workspace) ValueOpts() value.Opts {
	def := w.valOpt.Def
	if len(def.Pass)+len(def.Fail)+len(def.Neutral) == 0 {
		def = value.DefaultDef()
	}
	return value.Opts{Def: def, Window: w.valOpt.Window, Now: time.Now(), EscalateAt: w.valOpt.EscalateAt, ReopenTurns: w.valOpt.ReopenTurns}
}

// Spec is a rank's resolved value policy: its own score and meter, else its family's.
func (w *Workspace) Spec(rankName string) value.Spec {
	fam := w.Ranks.FamilyOf(rankName)
	sp := value.FamilyDefault(fam)
	if r, ok := w.Ranks.Get(rankName); ok && rankName != Orchestrator {
		if r.Score != "" {
			sp.Score = r.Score
		}
		if r.Meter.Signal != "" {
			sp.Meter = r.Meter
		}
		if r.MinRecord > 0 {
			sp.MinRecord = r.MinRecord
		}
	}
	if sp.MinRecord <= 0 {
		sp.MinRecord = w.valOpt.MinRecord
	}
	if sp.MinRecord <= 0 {
		sp.MinRecord = value.DefaultMinRecord
	}
	return sp
}

// scoreExpr parses a rank's score once.
func (w *Workspace) scoreExpr(rankName string) *value.Expr {
	w.val.mu.Lock()
	defer w.val.mu.Unlock()
	if e, ok := w.val.scores[rankName]; ok {
		return e
	}
	e, err := value.ParseNumber(w.Spec(rankName).Score, value.IsSignal)
	if err != nil {
		e = nil // config refused this at load; a broken score reads zero, never panics
	}
	w.val.scores[rankName] = e
	return e
}

// AllSignals folds the ledger over the window into every agent's signals, each with its rank's
// score (clamped 0..100).
func (w *Workspace) AllSignals() map[string]value.Signals {
	if w.val == nil {
		return map[string]value.Signals{}
	}
	out := value.Fold(w.Ledger(), w.ValueOpts())
	for agent, s := range out {
		s["score"] = value.Clamp(w.scoreExpr(w.RankOf(agent).Name).Eval(s))
	}
	return out
}

// Metrics reads one agent's signals (zero for an agent with no record).
func (w *Workspace) Metrics(agent string) Metrics {
	if s, ok := w.AllSignals()[agent]; ok {
		return s
	}
	return Metrics{}
}

// AgentValue is one agent's value as every surface reads it.
type AgentValue struct {
	Agent      string           `json:"agent"`
	Rank       string           `json:"rank"`
	Family     string           `json:"family"`
	Signals    value.Signals    `json:"signals"`
	Score      float64          `json:"score"`
	Confidence string           `json:"confidence"` // ok | low (reports under the rank's minRecord: never ranked)
	Meter      value.Meter      `json:"meter"`
	Strain     int              `json:"strain"`
	Promotion  *PromoteProgress `json:"promotion,omitempty"`
}

// PromoteProgress is an agent's distance to its promotion: each condition leaf's reading.
type PromoteProgress struct {
	To     string       `json:"to"`
	When   string       `json:"when"`
	Met    bool         `json:"met"`
	Leaves []value.Leaf `json:"leaves"`
}

// RankValue is one rank's value policy and its agents' standing.
type RankValue struct {
	Rank      string         `json:"rank"`
	Family    string         `json:"family"`
	Score     string         `json:"score"`
	Meter     value.MeterDef `json:"meter"`
	MinRecord int            `json:"minRecord"`
	Agents    int            `json:"agents"`
	Confident int            `json:"confident"`
	Mean      float64        `json:"mean"` // the mean score of its confident agents
	Reading   value.Meter    `json:"reading"`
}

// ValueReport is the whole value picture: what /api/value serves.
type ValueReport struct {
	Window string                    `json:"window"`
	Def    value.Def                 `json:"def"`
	Ledger string                    `json:"ledger"`
	Rows   int                       `json:"rows"`
	Off    bool                      `json:"off"`
	Agents []AgentValue              `json:"agents"`
	Ranks  []RankValue               `json:"ranks"`
	Meters map[string]value.MeterDef `json:"meters"`
}

// valueOf renders one agent's value from its signals.
func (w *Workspace) valueOf(agent string, s value.Signals) AgentValue {
	rk := w.RankOf(agent).Name
	sp := w.Spec(rk)
	bv := AgentValue{Agent: agent, Rank: rk, Family: w.Ranks.FamilyOf(rk), Signals: s, Score: s["score"], Strain: int(s["strain"])}
	bv.Confidence = "ok"
	if s["reports"] < float64(sp.MinRecord) {
		bv.Confidence = "low"
	}
	bv.Meter = value.Render(sp.Meter, s[sp.Meter.Signal], s["reports"] > 0)
	if w.promote != nil {
		w.promote.mu.Lock()
		watcher, ok := w.promote.watch[rk]
		w.promote.mu.Unlock()
		if ok {
			bv.Promotion = &PromoteProgress{To: watcher.To.Name, When: watcher.ASL.Cond.String(), Met: watcher.ASL.Cond.Eval(s), Leaves: watcher.ASL.Cond.Leaves(s)}
		}
	}
	return bv
}

// Value is one agent's value (an agent with no record reads zeros, low-confidence).
func (w *Workspace) Value(agent string) AgentValue {
	s, ok := w.AllSignals()[agent]
	if !ok {
		s = value.Signals{}
	}
	return w.valueOf(agent, s)
}

// Meter is one agent's meter, rendered.
func (w *Workspace) Meter(agent string) value.Meter { return w.Value(agent).Meter }

// ValueReport folds the ledger once into every agent's and every rank's value.
func (w *Workspace) ValueReport() ValueReport {
	rep := ValueReport{Window: w.valOpt.WindowText, Def: w.ValueOpts().Def, Ledger: filepath.ToSlash(filepath.Join(w.Lex.WorkspaceDir, value.Shelf)), Off: w.valOpt.Off, Meters: map[string]value.MeterDef{}}
	if rep.Window == "" {
		rep.Window = "all"
	}
	for k, m := range w.valOpt.Meters {
		rep.Meters[k] = m
	}
	all := w.AllSignals()
	rep.Rows = len(w.Ledger())
	// every agent on the roster appears, an agent with no record as a low-confidence zero
	names := map[string]bool{}
	for b := range all {
		names[b] = true
	}
	for _, c := range w.Roster() {
		names[c.Name] = true
	}
	var sorted []string
	for b := range names {
		sorted = append(sorted, b)
	}
	sort.Strings(sorted)
	byRank := map[string][]AgentValue{}
	for _, b := range sorted {
		s, ok := all[b]
		if !ok {
			s = value.Signals{}
		}
		bv := w.valueOf(b, s)
		rep.Agents = append(rep.Agents, bv)
		byRank[bv.Rank] = append(byRank[bv.Rank], bv)
	}
	ranks := []string{Orchestrator}
	for _, r := range w.Ranks {
		ranks = append(ranks, r.Name)
	}
	for _, rk := range ranks {
		sp := w.Spec(rk)
		rv := RankValue{Rank: rk, Family: w.Ranks.FamilyOf(rk), Score: sp.Score, Meter: sp.Meter, MinRecord: sp.MinRecord}
		sum, meterSum := 0.0, 0.0
		for _, bv := range byRank[rk] {
			rv.Agents++
			if bv.Confidence == "ok" {
				rv.Confident++
				sum += bv.Score
				meterSum += bv.Signals[sp.Meter.Signal]
			}
		}
		if rv.Confident > 0 {
			rv.Mean = sum / float64(rv.Confident)
			rv.Reading = value.Render(sp.Meter, meterSum/float64(rv.Confident), true)
		} else {
			rv.Reading = value.Render(sp.Meter, 0, false)
		}
		rep.Ranks = append(rep.Ranks, rv)
	}
	return rep
}

// Roster is the members on the roster, foreign agents included.
func (w *Workspace) Roster() []Member {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append(append([]Member(nil), w.Members...), w.Foreign...)
}

// CanonRank is the one spelling of a rank name (rank.Canon), re-exported for callers that hold
// the workspace only.
func CanonRank(name string) string { return rank.Canon(name) }

// RankMeters is every rank's resolved meter, keyed by rank (the board's /api/meters).
func (r ValueReport) RankMeters() map[string]value.MeterDef {
	out := map[string]value.MeterDef{}
	for _, rv := range r.Ranks {
		out[rv.Rank] = rv.Meter
	}
	return out
}
