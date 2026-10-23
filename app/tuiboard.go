package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/board"
	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/onto"
	"github.com/Kaginari/agent-one/tui"
)

// officeRoles says what each office of the triad does (the analyst reads, the judge rules, the drafter writes).
var officeRoles = map[string]string{"analyst": "reads", "judge": "verdicts", "drafter": "drafts"}

// Board is what /board draws, read from the same feeds as the web board: the live subagents, the
// reasoned ontology, the roles as config resolves them, the usage journal.
func (h *tuiHost) Board(rng string) tui.BoardView { return h.a.BoardView(rng) }

// Plans is what the rail's plan card draws: every deck under <workspace>/tmp/plans, in name
// order, each flagged stale when it was penned before this session's provisioning — a stale deck is
// last session's intent left readable (the 'continue on last plan' lifeline), never this
// session's outstanding work. The boot sweep (LEC-2026-10-04-STALE-PLANS) keeps the live dir
// to this session's decks; last session's decks merge in from the archive under Archived=true
// so the rail renders them and alt+n can snap off them (contract: tui/blocks.go:597).
// The rail asks five times a second (off the UI goroutine); the decks are read again only
// when a deck file or a live agent's role moved.
func (h *tuiHost) Plans() []tui.RailPlan {
	dir := h.a.planDir()
	boot := h.a.plansBoot()
	sig := plansSig(h.a, dir, boot)
	h.plans.mu.Lock()
	defer h.plans.mu.Unlock()
	if !h.plans.ok || h.plans.sig != sig {
		h.plans.out, h.plans.sig, h.plans.ok = railPlans(h.a, dir, boot), sig, true
	}
	return append([]tui.RailPlan(nil), h.plans.out...)
}

// plansMemo is the last read of the decks and the signature it was read under.
type plansMemo struct {
	mu  sync.Mutex
	sig string
	out []tui.RailPlan
	ok  bool
}

// plansSig names what a read of the decks depends on: every deck file's name, size and mtime,
// live and archived, the live agents' roles (a deck holds its agent's), the session's provisioning.
func plansSig(a *App, dir string, boot time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d;", boot.UnixNano())
	for _, d := range []string{dir, planArchiveDir(dir)} {
		es, _ := os.ReadDir(d)
		for _, e := range es {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			if info, err := e.Info(); err == nil {
				fmt.Fprintf(&b, "%s/%s %d %d;", d, e.Name(), info.Size(), info.ModTime().UnixNano())
			}
		}
	}
	if a.Subagent != nil {
		for _, x := range a.Subagent.Agents() {
			fmt.Fprintf(&b, "%s=%s;", x.Name, x.Role)
		}
	}
	return b.String()
}

// railPlans renders one plans dir into rail entries: live decks first (in name order), then
// the archive's decks with Archived=true — deduped against live names, so a deck present in
// both (sweep deferred its rename that wave) shows once, as live truth.
func railPlans(a *App, dir string, boot time.Time) []tui.RailPlan {
	entry := func(dir, file string, archived bool) (tui.RailPlan, bool) {
		rank := strings.TrimSuffix(filepath.Base(file), ".json")
		d := planRead(dir, rank)
		if d.Agent == "" {
			return tui.RailPlan{}, false
		}
		if i := strings.IndexByte(rank, '-'); i >= 0 {
			rank = rank[:i]
		}
		// the role the agent's turn runs under: a live subagent answers first, the rank's
		// default role answers second (zone worker→analyst, domain owner→judge, coordinator→drafter)
		role := ""
		for _, b := range a.liveAgents() {
			if b.Name == d.Agent && b.Role != "" {
				role = b.Role
				break
			}
		}
		if role == "" {
			if r, ok := a.Cfg.Rank(rank); ok && r.Role != "" {
				role = r.Role
			}
		}
		rp := tui.RailPlan{Agent: d.Agent, Rank: rank, Role: role, Stale: planStale(d, boot), Archived: archived}
		for _, it := range d.Items {
			rp.Items = append(rp.Items, tui.RailPlanItem{Text: it.Text, Status: it.Status})
		}
		return rp, true
	}
	live, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	sort.Strings(live)
	var out []tui.RailPlan
	seen := map[string]bool{}
	for _, p := range live {
		rp, ok := entry(dir, p, false)
		if !ok {
			continue
		}
		seen[filepath.Base(p)] = true
		out = append(out, rp)
	}
	arch, _ := filepath.Glob(filepath.Join(planArchiveDir(dir), "*.json"))
	sort.Strings(arch)
	for _, p := range arch {
		if seen[filepath.Base(p)] {
			continue
		}
		if rp, ok := entry(planArchiveDir(dir), p, true); ok {
			out = append(out, rp)
		}
	}
	return out
}

// BoardView is the board as /board draws it.
func (a *App) BoardView(rng string) tui.BoardView {
	now := time.Now()
	opt := a.boardOptions()
	src := board.FileSources(opt.WorkspaceRoot, opt.WorkspaceDir, opt.Layout, time.Now)
	v := tui.BoardView{At: now}

	live := a.liveAgents()
	for _, b := range live {
		v.Agents = append(v.Agents, tui.AgentRow{Name: b.Name, Rank: b.Rank, Role: b.Role, Model: b.Model, Provider: b.Provider,
			State: b.State, Started: b.Started, CtxTokens: b.ContextTokens, CtxLimit: b.ContextLimit,
			Input: b.Input, Output: b.Output, Cache: b.CacheRead + b.CacheWrite, USD: b.USD})
	}

	v.Graph = a.boardGraph(opt.Layout)

	liveIn := map[string]int{}
	for _, b := range live {
		if b.State != "done" && b.Role != "" {
			liveIn[b.Role]++
		}
	}
	for _, off := range config.Roles {
		m, origin := a.Cfg.ResolveModel("", "", off, "")
		row := tui.OfficeRow{Name: off, Role: officeRoles[off], Model: m.Ref.Model, Fallback: m.Ref.Fallback, Origin: origin.String(), Live: liveIn[off]}
		for _, r := range a.Workspace.Ranks {
			if r.Role == off {
				row.Ranks = append(row.Ranks, r.Name)
			}
		}
		v.Roles = append(v.Roles, row)
	}

	recs, rd := src.Usage()
	since, name := board.RangeSince(rng, now)
	rep := board.Rollup(recs, since, name)
	u := tui.UsageView{Range: name, Calls: rep.Total.Calls, Input: rep.Total.Input, Output: rep.Total.Output, Cache: rep.Total.CacheRead + rep.Total.CacheWrite,
		USD: rep.Total.USD, Unpriced: rep.Total.Unpriced}
	if !rd.Lit {
		u.Note = rd.String()
	}
	rows := func(ss []board.UsageSum) []tui.UsageRow {
		var out []tui.UsageRow
		for _, s := range ss {
			out = append(out, tui.UsageRow{Key: s.Key, Calls: s.Calls, Tokens: s.Tokens(), USD: s.USD, Unpriced: s.Unpriced})
		}
		return out
	}
	u.ByBody, u.ByModel, u.ByOffice, u.ByDay = rows(rep.ByAgent), rows(rep.ByModel), rows(rep.ByRole), rows(rep.ByDay)
	v.Usage = u
	return v
}

// boardGraph projects the reasoned ontology: every member, its one-hop bonds up, and what the
// graph knows about it — its classes, derived chain, territory, minds, facts it can see (the
// flow rules), and the shape findings against it.
func (a *App) boardGraph(layout onto.Layout) tui.GraphView {
	w, err := onto.Cached(a.Root, layout)
	if err != nil {
		return tui.GraphView{Note: "the ontology did not load: " + err.Error()}
	}
	g := w.Graph
	findings := map[string][]string{}
	all := w.Validate()
	for _, f := range all {
		findings[f.Subject.Local()] = append(findings[f.Subject.Local()], f.Msg+" ("+f.Shape+")")
	}
	isP := func(n string) onto.Term { return onto.Ao(n) }
	members := g.Instances(isP("Member"))
	ids := map[string]bool{}
	for _, c := range members {
		if c.IsIRI() {
			ids[c.Local()] = true
		}
	}
	names := func(ts []onto.Term) []string {
		var out []string
		for _, t := range ts {
			out = append(out, t.Local())
		}
		sort.Strings(out)
		return out
	}
	var nodes []tui.GraphNode
	// the one-hop bonds up, as the web board draws them (an inferred `above` is not a bond)
	col := board.BuildTeam(w)
	up := map[string][]tui.GraphBond{}
	rankOf := map[string]string{}
	for _, n := range col.Nodes {
		rankOf[n.ID] = n.Rank
	}
	for _, e := range col.Edges {
		if e.Bond != "wears" && e.Bond != "holds" && ids[e.From] && ids[e.To] {
			up[e.From] = append(up[e.From], tui.GraphBond{To: e.To, Bond: e.Bond})
		}
	}
	// level = the longest bond chain up to a root
	level := map[string]int{}
	var depth func(id string, guard int) int
	depth = func(id string, guard int) int {
		if l, ok := level[id]; ok {
			return l
		}
		if guard > len(ids) {
			return 0
		}
		l := 0
		for _, u := range up[id] {
			if d := depth(u.To, guard+1) + 1; d > l {
				l = d
			}
		}
		level[id] = l
		return l
	}
	triples := g.Len()
	facts := len(g.Instances(isP("Fact")))
	minds := len(g.Instances(isP("Skill")))
	for id := range ids {
		t := isP(id)
		n := tui.GraphNode{ID: id, Name: id, Rank: rankOf[id], Level: depth(id, 0), Up: up[id], Findings: len(findings[id])}
		kv := func(k, v string) {
			if v != "" {
				n.Knowledge = append(n.Knowledge, tui.KV{Key: k, Value: v})
			}
		}
		list := func(k string, vs []string, limit int) {
			for i, v := range vs {
				if i == limit {
					kv("", fmt.Sprintf("… %d more", len(vs)-limit))
					break
				}
				if i == 0 {
					kv(k, v)
				} else {
					kv("", v)
				}
			}
		}
		var classes []string
		for _, c := range g.Objects(t, onto.I(onto.RDFType)) {
			classes = append(classes, c.Local())
		}
		sort.Strings(classes)
		kv("is a", strings.Join(classes, " ⊂ "))
		// the derived chain: every member above, by the transitive `above`
		var chain []string
		for _, o := range g.Objects(t, isP("above")) {
			chain = append(chain, o.Local())
		}
		sort.Slice(chain, func(i, j int) bool { return depth(chain[i], 0) > depth(chain[j], 0) })
		if len(chain) > 0 {
			kv("chain", strings.Join(chain, " → ")+" (inferred)")
		}
		below := names(g.Subjects(isP("above"), t))
		if len(below) > 0 {
			kv("under it", fmt.Sprintf("%d member%s", len(below), plural(len(below))))
		}
		for _, d := range g.Objects(t, isP("doc")) {
			for _, p := range g.Objects(d, isP("path")) {
				kv("doc", p.Value)
			}
		}
		var owns []string
		for _, o := range g.Objects(t, isP("owns")) {
			owns = append(owns, o.Value)
		}
		sort.Strings(owns)
		list("owns", owns, 4)
		list("holds", names(g.Objects(t, isP("holds"))), 4)
		known := g.Objects(t, isP("knows"))
		if len(known) > 0 {
			kv("knows", fmt.Sprintf("%d fact%s", len(known), plural(len(known))))
		}
		seen := w.Visible(t)
		if len(seen) > 0 {
			kv("sees", fmt.Sprintf("%d fact%s by the flow rules", len(seen), plural(len(seen))))
			var ls []string
			for _, s := range seen {
				ls = append(ls, s.Kind+": "+oneLineOf(s.Text))
			}
			list("", ls, 3)
		}
		list("finding", findings[id], 3)
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	gv := tui.GraphView{Nodes: nodes,
		Summary: fmt.Sprintf("%d members · %d minds · %d facts · %d triples (reasoned) · %d finding%s", len(ids), minds, facts, triples, len(all), plural(len(all)))}
	if len(w.Notes) > 0 {
		gv.Note = fmt.Sprintf("%d hole%s: %s", len(w.Notes), plural(len(w.Notes)), w.Notes[0])
	}
	return gv
}

func oneLineOf(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		s = s[:79] + "…"
	}
	return s
}
