package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Promotion is the coded answer to "a domain owner with experience evolves": a rank declares
// promotesWhen (ASL), the workspace keeps a MetricsStore that every Subagent report folds into, and
// after each fold the workspace evaluates the divine bonds. A base agent whose snapshot crosses the
// condition is promoted: the harness registers the promoted agent — its doc lands on disk under the
// base's dir, the roster gains it — without a prompt, without the agent asking. rankSet: replace
// still governs which ranks exist; promotesWhen is what config adds to a promoted rank so the
// workspace knows the bond is lived, not just declared.
//
// Nothing here reads a prompt. Evaluation is the ASL condition over the tally, deterministic;
// the register and the doc write are the workspace's own.

// ascender is one resolved divine bond the workspace watches.
type ascender struct {
	To   Rank            // the promoted rank (holds PromotesWhen)
	ASL  ASL             // its parsed condition
	Done map[string]bool // agents already promoted this session — register once
}

// promoteState is Workspace's promotion machinery, guarded by the workspace lock.
type promoteState struct {
	mu      sync.Mutex
	metrics *MetricsStore
	watch   map[string]ascender // base rank → what promotes from it
	Events  []PromoteEvent      // the ledger of promotions this session
	holes   []string            // register failures, surfaced at Close as an error
}

// PromoteEvent is one registered promotion, journaled so the log can say what happened.
type PromoteEvent struct {
	Base  string    // the ranking agent (domain-auth)
	New   string    // the promoted agent (principal-domain-owner-auth)
	Rank  string    // the promoted rank (principal_domain_owner)
	When  string    // the ASL condition, as config wrote it
	At    time.Time `json:"at"`
	Doc   string    // the doc the harness wrote for the new agent
	MetAt string    // the snapshot line that crossed the condition
}

// promoteState builds the watch list from the rank table, or nil when no rank promotes.
func newPromoteState(rs Ranks) *promoteState {
	as := rs.Promotions()
	if len(as) == 0 {
		return nil
	}
	st := &promoteState{metrics: NewMetricsStore(), watch: map[string]ascender{}}
	for _, a := range as {
		cond, err := ParseASL(a.When)
		if err != nil { // Validate has already refused this; belt and braces
			continue
		}
		to, _ := rs.Get(a.To)
		st.watch[a.From] = ascender{To: to, ASL: cond, Done: map[string]bool{}}
	}
	return st
}

// FoldMetrics counts one Subagent report and runs the promotion pass. The parent agent is who gets
// the tally (the ranking agent is the dispatcher's *target* — but the target filed the report,
// so its own record grows; promotion reads the target's rank, never the dispatcher's).
func (w *Workspace) FoldMetrics(agent, status string, holes int) {
	if w.promote == nil {
		return
	}
	w.promote.mu.Lock()
	w.promote.metrics.Fold(agent, status, holes)
	w.promote.mu.Unlock()
	w.promoteMaybe(agent)
}

// promoteMaybe evaluates every divine bond against the agent's rank. It registers at most one
// promotion per base agent per session; the event is journaled and the roster reloaded.
func (w *Workspace) promoteMaybe(agent string) {
	if w.promote == nil {
		return
	}
	w.promote.mu.Lock()
	rank := w.RankOf(agent).Name
	watcher, ok := w.promote.watch[rank]
	if !ok || watcher.Done[agent] {
		w.promote.mu.Unlock()
		return
	}
	m := w.promote.metrics.Metrics(agent)
	if !watcher.ASL.Cond.Eval(m) {
		w.promote.mu.Unlock()
		return
	}
	// the crossing is real: mark before the write so a reload cannot double-register
	watcher.Done[agent] = true
	w.promote.watch[rank] = watcher
	w.promote.mu.Unlock()

	newName, ev, err := w.registerPromoted(agent, watcher, m)
	if err != nil {
		w.promote.mu.Lock()
		w.promote.holes = append(w.promote.holes, fmt.Sprintf("promote %s: %v", agent, err))
		w.promote.mu.Unlock()
		return
	}
	w.promote.mu.Lock()
	ev.At = time.Now()
	w.promote.Events = append(w.promote.Events, ev)
	w.promote.mu.Unlock()
	// the promoted agent must be dispatchable on the next commission: re-derive the roster so
	// membersOf reads the registered doc through the ontology, agent consistent with the others.
	if err := w.Reload(); err != nil {
		w.promote.mu.Lock()
		w.promote.holes = append(w.promote.holes, fmt.Sprintf("reload %s: %v", newName, err))
		w.promote.mu.Unlock()
	}
}

// registerPromoted names the promoted agent, writes its doc beside the base's, and reloads the
// roster so the new member is dispatchable. The doc is the harness's own — a record of the
// promotion, never the base's doc edited.
func (w *Workspace) registerPromoted(base string, a ascender, m Metrics) (string, PromoteEvent, error) {
	baseC := w.Member(base)
	if baseC == nil {
		return "", PromoteEvent{}, fmt.Errorf("no member %q on the roster", base)
	}
	// the promoted id: the promoted rank's prefix + the base's short name, in its dir
	short := base
	for _, p := range []string{baseC.Rank + "-", a.To.Base + "-"} {
		short = strings.TrimPrefix(short, p)
	}
	prefix := a.To.Prefix
	if prefix == "" && a.To.Base != "" {
		if br, ok := w.Ranks.Get(a.To.Base); ok {
			prefix = br.Prefix
		}
	}
	newName := prefix + short
	if w.Member(newName) != nil {
		return "", PromoteEvent{}, fmt.Errorf("%q is already on the roster", newName)
	}
	dir := a.To.Dir
	if dir == "" {
		dir = baseC.Dir
	}
	// the folder is the base's short name; in the promoted rank's own dir there is no
	// collision. When the promoted shares the base's dir, the README would land on the base's
	// own doc — the stat below refuses rather than overwrite.
	docRel := filepath.ToSlash(filepath.Join(w.Lex.WorkspaceDir, dir, short, "README.md"))
	abs := filepath.Join(w.Root, filepath.FromSlash(docRel))
	if _, err := os.Stat(abs); err == nil {
		return "", PromoteEvent{}, fmt.Errorf("%s already stands on disk", docRel)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", PromoteEvent{}, err
	}
	metAt := fmt.Sprintf("reports %d passes %d fails %d holes %d passRate %.0f%%",
		m.Reports, m.Passes, m.Fails, m.Holes, m.rate(m.Passes))
	doc := promoteDoc(newName, a.To, base, watcherWhen(a), metAt, rankParentOf(w.Ranks, a.To.Name, base))
	if err := os.WriteFile(abs, []byte(doc), 0o644); err != nil {
		return "", PromoteEvent{}, err
	}
	ev := PromoteEvent{Base: base, New: newName, Rank: a.To.Name, When: watcherWhen(a), Doc: docRel, MetAt: metAt}
	return newName, ev, nil
}

// promoteDoc is the agent card the harness registers for a promotion — the workspace's words, carrying
// the rank, the ownership it inherits, the bond to its base, and why it rose.
func promoteDoc(name string, to Rank, base, when, metAt, parent string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	fmt.Fprintf(&b, "- **Rank:** %s (promoted %s)\n", to.Name, to.Base)
	fmt.Fprintf(&b, "- **Owns:** inherits the base's — the promoted keeps everything %s held\n", to.Base)
	fmt.Fprintf(&b, "- **Ascendant of:** `%s`\n", base)
	fmt.Fprintf(&b, "- **Purpose:** %s\n", or(to.Job, "the consolidated shape of its base's experience"))
	fmt.Fprintf(&b, "\n## Promotion\n\n")
	fmt.Fprintf(&b, "- **Condition:** `%s`\n- **Crossed at:** %s\n", when, metAt)
	if parent != "" {
		fmt.Fprintf(&b, "\n## Bond\n\n- **Above:** `%s`\n", parent)
	}
	return b.String()
}

// rankParentOf names the promoted agent's one-hop-up — the promoted rank's own reportsTo,
// falling back to the base agent it promoted from when config left the rank's parent blank.
func rankParentOf(rs Ranks, promotedRank, base string) string {
	if r, ok := rs.Get(promotedRank); ok && r.ReportsTo != "" {
		return r.ReportsTo
	}
	return base
}

func watcherWhen(a ascender) string { return a.ASL.String() }

// Promotions is the session's ledger of registered promotions (the log sink reads it).
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

// PromoteHoles drains the promotion failures so a whole-workspace error surface can report them.
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

// Metrics reads one agent's promotion snapshot (for the workspace's self-measure and tests).
func (w *Workspace) Metrics(agent string) Metrics {
	if w.promote == nil {
		return Metrics{}
	}
	w.promote.mu.Lock()
	defer w.promote.mu.Unlock()
	return w.promote.metrics.Metrics(agent)
}
