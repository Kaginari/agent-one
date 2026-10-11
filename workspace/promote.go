package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/value"
)

// Promotion is the coded answer to "a domain owner with experience evolves": a rank declares
// promotesWhen (ASL), the workspace keeps the value ledger every Subagent report folds into, and
// after each fold the workspace evaluates the divine bonds. A base agent whose snapshot crosses the
// condition promotes: the harness registers the promoted agent in the promoted rank's own dir under
// the base's short name (domain-engine → principal-domain-engine), carrying the base's doc whole — every
// invariant, ownership, verify line and Skill (AGENT-ONE.md Principle 5: "a promoted member keeps every
// invariant and ownership it held before") — and retires the base's seat, so one agent holds the
// soil and ownerships stay disjoint. The retired seat keeps its graph memory; members that
// reported to it now report to its promoted (the roster resolves the succession).
//
// Each report folds into the value ledger (`<workspaceDir>/instruments/value.jsonl`, workspace/value.go)
// replayed at Open, so a record earned over many sessions counts. Each promotion is appended
// to log.md with the reading that earned it; each failure is a hole, logged and surfaced to
// the dispatcher as `@?`.
//
// Nothing here reads a prompt. Evaluation is the ASL condition over the tally, deterministic;
// the register and the doc write are the workspace's own.

// ascender is one resolved divine bond the workspace watches.
type ascender struct {
	To  Rank // the promoted rank (holds PromotesWhen)
	ASL ASL  // its parsed condition
}

// promoteState is Workspace's promotion machinery, guarded by its own lock. The record it reads is
// the value ledger (workspace/value.go); it keeps only the watch list and what it registered.
type promoteState struct {
	mu     sync.Mutex
	watch  map[string]ascender // base rank → what promotes from it
	done   map[string]bool     // base agents already promoted (replayed from the ledger)
	Events []PromoteEvent      // the promotions of this session
	news   []PromoteEvent      // registered, not yet announced to a dispatcher
	holes  []string            // register failures, drained by PromoteHoles
}

// PromoteEvent is one registered promotion.
type PromoteEvent struct {
	Base  string    `json:"base"` // the ranking agent (domain-auth)
	New   string    `json:"new"`  // the promoted agent (principal-domain-auth)
	Rank  string    `json:"rank"` // the promoted rank (principal-domain-owner)
	When  string    `json:"when"` // the ASL condition, as config wrote it
	At    time.Time `json:"at"`
	Doc   string    `json:"doc"` // the doc the harness wrote for the new agent
	MetAt string    `json:"met"` // the snapshot line that crossed the condition
}

// newPromoteState builds the watch list from the rank table, or nil when no rank promotes.
func newPromoteState(rs Ranks) *promoteState {
	as := rs.Promotions()
	if len(as) == 0 {
		return nil
	}
	st := &promoteState{watch: map[string]ascender{}, done: map[string]bool{}}
	for _, a := range as {
		cond, err := ParseASL(a.When)
		if err != nil { // Validate has already refused this; belt and braces
			continue
		}
		to, _ := rs.Get(a.To)
		st.watch[a.From] = ascender{To: to, ASL: cond}
	}
	return st
}

// promoteMaybe evaluates the divine bond over the agent's rank. An agent promotes once: the shelf
// and the roster (a live member naming it as its base) both remember.
func (w *Workspace) promoteMaybe(agent string) {
	if w.promote == nil {
		return
	}
	rank := w.RankOf(agent).Name
	if w.successorOf(agent) != "" {
		return
	}
	w.promote.mu.Lock()
	watcher, ok := w.promote.watch[rank]
	if !ok || w.promote.done[agent] {
		w.promote.mu.Unlock()
		return
	}
	w.promote.mu.Unlock()
	m := w.Metrics(agent)
	if !watcher.ASL.Cond.Eval(m) {
		return
	}
	w.promote.mu.Lock()
	if w.promote.done[agent] {
		w.promote.mu.Unlock()
		return
	}
	// the crossing is real: mark before the write so a reload cannot double-register
	w.promote.done[agent] = true
	w.promote.mu.Unlock()

	ev, err := w.registerPromoted(agent, watcher, m)
	if err != nil {
		w.promoteHole(agent, watcher, fmt.Sprintf("promote %s: %v", agent, err))
		return
	}
	w.promote.mu.Lock()
	w.promote.Events = append(w.promote.Events, ev)
	w.promote.news = append(w.promote.news, ev)
	w.promote.mu.Unlock()
	if w.val != nil {
		w.val.mu.Lock()
		w.appendValue(value.Row{Kind: value.KindPromote, Agent: agent, New: ev.New, At: ev.At})
		w.val.mu.Unlock()
	}
	if err := w.AppendLog(Entry{At: ev.At, Author: ev.New, Title: fmt.Sprintf("promotion — %s rose as %s", agent, ev.New),
		Task:    "promotesWhen `" + ev.When + "`",
		Files:   []string{ev.Doc, w.docOf(agent)},
		Gate:    "n/a (registered by the harness; no agent wrote it)",
		Result:  fmt.Sprintf("%s (%s) holds %s's ownership, invariants and verify; %s's seat retired", ev.New, ev.Rank, agent, agent),
		Learned: []string{"earned: " + ev.MetAt}}); err != nil {
		w.promoteHole(agent, watcher, fmt.Sprintf("promote %s: log.md: %v", agent, err))
	}
	// the promoted agent must be dispatchable on the next commission and the base must leave
	// the roster: re-derive it from the docs
	if err := w.Reload(); err != nil {
		w.promoteHole(agent, watcher, fmt.Sprintf("reload after %s: %v", ev.New, err))
	}
}

// promoteHole records one failure: drained by PromoteHoles for the dispatcher, and appended to
// log.md so the record names it even when no dispatcher reads it.
func (w *Workspace) promoteHole(agent string, a ascender, msg string) {
	w.promote.mu.Lock()
	w.promote.holes = append(w.promote.holes, msg)
	w.promote.mu.Unlock()
	_ = w.AppendLog(Entry{Author: agent, Title: "promotion hole — " + agent, Task: "promotesWhen `" + a.ASL.String() + "`",
		Gate: "n/a (the harness's own act)", Result: "hole: " + msg})
}

// docOf is a member's doc path, "" when it is not on the roster.
func (w *Workspace) docOf(name string) string {
	if c := w.Member(name); c != nil {
		return c.Doc
	}
	return ""
}

// successorOf names the live member that promoted from `base`, "" when none.
func (w *Workspace) successorOf(base string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range w.Members {
		if c.PromotedOf == base {
			return c.Name
		}
	}
	return ""
}

// registerPromoted writes the promoted agent's doc in the promoted rank's own dir — the base's doc
// carried whole under the new name, with the bond and the reading that earned it — then retires
// the base's seat. The base retires only once the new doc stands, so a failed register never
// orphans the ownership.
func (w *Workspace) registerPromoted(base string, a ascender, m Metrics) (PromoteEvent, error) {
	baseC := w.Member(base)
	if baseC == nil {
		return PromoteEvent{}, fmt.Errorf("no member %q on the roster", base)
	}
	if baseC.Doc == "" {
		return PromoteEvent{}, fmt.Errorf("%s carries no doc to promote with", base)
	}
	baseRank, _ := w.Ranks.Get(baseC.Rank)
	if a.To.Dir == "" || a.To.Prefix == "" || a.To.Dir == baseRank.Dir || a.To.Prefix == baseRank.Prefix {
		return PromoteEvent{}, fmt.Errorf("rank %s needs a dir and prefix of its own (ranks.%s.dir / .prefix) — on %s's the promoted name would be the base's", a.To.Name, strings.ReplaceAll(a.To.Name, "_", "-"), baseRank.Name)
	}
	short := strings.TrimPrefix(base, baseRank.Prefix)
	newName := a.To.Prefix + short
	if w.Member(newName) != nil {
		return PromoteEvent{}, fmt.Errorf("%q is already on the roster", newName)
	}
	docRel := filepath.ToSlash(filepath.Join(w.Lex.WorkspaceDir, a.To.Dir, short, "README.md"))
	abs := filepath.Join(w.Root, filepath.FromSlash(docRel))
	if _, err := os.Stat(abs); err == nil {
		return PromoteEvent{}, fmt.Errorf("%s already stands on disk", docRel)
	}
	text, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(baseC.Doc)))
	if err != nil {
		return PromoteEvent{}, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return PromoteEvent{}, err
	}
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	metAt := fmt.Sprintf("reports %.0f, passes %.0f, fails %.0f, holes %.0f, passRate %.0f%%, holeRate %.0f%%, score %.0f",
		m["reports"], m["passes"], m["fails"], m["holes"], m["passRate"], m["holeRate"], m["score"])
	when := a.ASL.String()
	if err := os.WriteFile(abs, []byte(promoteDoc(string(text), newName, a.To, base, when, metAt, date)), 0o644); err != nil {
		return PromoteEvent{}, err
	}
	if err := w.retireDocWhy(baseC, "promoted "+date+": the agent rose as "+newName+", its ownership and invariants with it"); err != nil {
		_ = os.Remove(abs) // the base still holds the soil: the half-registered promoted goes
		return PromoteEvent{}, fmt.Errorf("retiring %s: %v", base, err)
	}
	return PromoteEvent{Base: base, New: newName, Rank: a.To.Name, When: when, At: now, Doc: docRel, MetAt: metAt}, nil
}

var (
	reH1     = regexp.MustCompile(`(?m)^#[ \t]+.*$`)
	reRankLn = regexp.MustCompile(`(?mi)^[ \t]*[-*][ \t]*\*\*rank:?\*\*:?.*$`)
	reAscOf  = regexp.MustCompile("(?mi)^[ \\t]*[-*][ \\t]*\\*\\*promoted from:?\\*\\*:?[ \\t]*`?([a-z0-9][a-z0-9_-]*)`?")
)

// promoteDoc is the promoted agent's card: the base's doc carried whole — identity renamed, the
// rank line naming the promoted rank, the bond to the base — and a Promotion section with the
// condition and the reading that crossed it.
func promoteDoc(base, name string, to Rank, from, when, metAt, date string) string {
	doc := base
	if loc := reH1.FindStringIndex(doc); loc != nil {
		doc = doc[:loc[0]] + "# " + name + doc[loc[1]:]
	} else {
		doc = "# " + name + "\n\n" + doc
	}
	bond := fmt.Sprintf("- **Rank:** %s\n- **Promoted from:** `%s` — promoted %s; its ownership, invariants and verify carried whole", to.Name, from, date)
	if loc := reRankLn.FindStringIndex(doc); loc != nil {
		doc = doc[:loc[0]] + bond + doc[loc[1]:]
	} else if loc := reH1.FindStringIndex(doc); loc != nil {
		doc = doc[:loc[1]] + "\n\n" + bond + doc[loc[1]:]
	}
	if !strings.HasSuffix(doc, "\n") {
		doc += "\n"
	}
	doc += fmt.Sprintf("\n## Promotion\n\n- **Condition:** `%s`\n- **Crossed at:** %s\n- **Date:** %s\n", when, metAt, date)
	return doc
}

// promotedOf reads the base a member's doc says it promoted from ("" for none).
func promotedOf(doc string) string {
	if m := reAscOf.FindStringSubmatch(doc); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// Promotions is the session's ledger of registered promotions.
func (w *Workspace) Promotions() []PromoteEvent {
	if w.promote == nil {
		return nil
	}
	w.promote.mu.Lock()
	defer w.promote.mu.Unlock()
	out := append([]PromoteEvent(nil), w.promote.Events...)
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// PromoteNews drains the promotions registered since the last call — the dispatcher announces them.
func (w *Workspace) PromoteNews() []PromoteEvent {
	if w.promote == nil {
		return nil
	}
	w.promote.mu.Lock()
	defer w.promote.mu.Unlock()
	out := w.promote.news
	w.promote.news = nil
	return out
}

// PromoteHoles drains the promotion failures — the dispatcher surfaces them as `@?`.
func (w *Workspace) PromoteHoles() []string {
	if w.promote == nil {
		return nil
	}
	w.promote.mu.Lock()
	defer w.promote.mu.Unlock()
	out := append([]string(nil), w.promote.holes...)
	w.promote.holes = nil
	return out
}

// promoteLines renders what the last fold did for the dispatcher's report: each promotion
// announced, each hole as `@?`.
func (w *Workspace) promoteLines() []string {
	var out []string
	for _, ev := range w.PromoteNews() {
		out = append(out, fmt.Sprintf("[promotion] %s rose as %s (%s): %s — its ownership, invariants and verify carried whole; %s's seat retired; dispatch %s from now on (log.md)", ev.Base, ev.New, ev.Rank, ev.MetAt, ev.Base, ev.New))
	}
	for _, h := range w.PromoteHoles() {
		out = append(out, "@? promotion hole — "+h)
	}
	return out
}
