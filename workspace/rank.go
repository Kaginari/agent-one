package workspace

import (
	"fmt"
	"path"
	"strings"

	"github.com/Kaginari/agent-one/onto"
	"github.com/Kaginari/agent-one/rank"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/value"
)

// Rank is one row of the hierarchy — data, not code (binary.md §Ranks and agents). The
// built-in table is the policy's §The workspace; config may override a field, add a rank, or replace
// the whole table. Ownership, the gate and dispatch read Authors, HoldsGate and Tools; nothing
// below names coord, domain or zone worker.
type Rank struct {
	Name         string   // canonical id (rank.Canon): coordinator | domain owner | zone worker | service owner | principal-coordinator | principal-domain-owner | auditor | anything from config
	ReportsTo    string   // the rank one hop up; "orchestrator" at the top
	Job          string   // one line
	Authors      bool     // authors changes in its ownership (ownership is enforced on it)
	HoldsGate    bool     // gates the authoring ranks below it
	Sideways     bool     // may dispatch its own rank (domain owner → domain owner) and know its layer-mates
	Role         string   // default role: analyst | judge | drafter
	Tools        []string // tool names its shelf is cut to
	Dir          string   // folder under the workspace dir; "" for a rank with no members on disk
	Prefix       string   // member-id prefix, Dir + "-"
	Base         string   // a promoted rank keeps everything its base held (its dir and prefix are its own)
	Layer        int      // config-declared layer (0 when unset); two ranks share a layer iff equal and nonzero
	DivineRules  []string // a promoted rank's absolute rules; they reach the base rank as coded rules
	PromotesWhen string   // the metrics condition under which a body of this rank promotes (ASL); "" = never
	// The value policy (ranks.md §Value): the family a rank's job is measured as ("" = derived from
	// its flags), its score expression and meter ("" / zero = the family's), the record below
	// which an agent's score is low-confidence (0 = the workspace's), and a configured portrait file.
	Family    string
	Score     string
	Meter     value.MeterDef
	MinRecord int
	Portrait  string
}

// Relation names the three coded bonds between two ranks.
type Relation int

const (
	RelNone     Relation = iota // no bond
	RelParent                   // a is one hop above b (a= ReportsTo b)
	RelChild                    // a is one hop below b
	RelDivine                   // a promotes b (a.Base == b) or b promotes a
	RelNeighbor                 // a and b share a layer (one rank, or two at one configured Layer)
)

// Divine reports whether this rank is a promotion (a principal-domain-owner over domain owner).
func (r Rank) Divine() bool { return r.Base != "" }

// relate is the bond between two ranks, coded from the tree — never inferred from names.
func (rs Ranks) relate(a, b string) Relation {
	if a == "" || b == "" {
		return RelNone
	}
	if a == b {
		return RelNeighbor
	}
	ra, oka := rs.Get(a)
	rb, okb := rs.Get(b)
	if !oka || !okb {
		return RelNone
	}
	if ra.Base == b || rb.Base == a {
		return RelDivine
	}
	if ra.ReportsTo == b {
		return RelChild // a reports to b
	}
	if rb.ReportsTo == a {
		return RelParent // a is above b
	}
	if ra.Layer != 0 && ra.Layer == rb.Layer {
		return RelNeighbor
	}
	return RelNone
}

// Orchestrator is the root of every table: the session, no dir, every tool.
const Orchestrator = "orchestrator"

// AllTools is the shelf a rank with no Tools listed gets (orchestrator's).
var AllTools = []string{"read", "write", "edit", "bash", "glob", "grep", "policy", "dispatch", "plan"}

var baseTools = []string{"read", "write", "edit", "bash", "glob", "grep", "policy"}

// DefaultRanks is the policy's table (AGENT-ONE.md §The workspace), dirs and prefixes from the lexicon. The
// promoted ranks carry no promotesWhen here: config is the one source of the default conditions
// (config.DefaultPromoteCoord / DefaultPromoteDomain), so a workspace opened without config never promotes.
func DefaultRanks(lex Lexicon) []Rank {
	d := func(r string) string { return lex.Ranks[r] }
	p := func(r string) string { return lex.Prefixes[r] }
	return []Rank{
		{Name: "coord", ReportsTo: Orchestrator, Job: "the shared skill and voice: thinks across domains, routes work to domain owners, speaks back up — on an order from above it first writes its plan deck (`plan` tool: one line per slice, each named to the domain owner whose domain owns it), then dispatches — domain owners in parallel (background: true) when the slices are independent", Role: "drafter", Tools: AllTools, Dir: d("coord"), Prefix: p("coord")},
		{Name: "domain", ReportsTo: "coord", Job: "rules its domain, commands its zone workers, holds the gate — the slice's verify is written before its zone worker is ordered (the gate proves the landing, never after)", HoldsGate: true, Sideways: true, Role: "judge", Tools: AllTools, Dir: d("domain"), Prefix: p("domain")},
		{Name: "zone", ReportsTo: "domain", Job: "the ground truth of its zone: authors changes there", Authors: true, Role: "analyst", Tools: baseTools, Dir: d("zone"), Prefix: p("zone")},
		{Name: "service", ReportsTo: Orchestrator, Job: "standing domain lead: owns one subsystem end-to-end across sessions", Authors: true, HoldsGate: true, Role: "judge", Tools: AllTools, Dir: d("service"), Prefix: p("service")},
		{Name: "principal-coordinator", ReportsTo: Orchestrator, Job: "promoted coordinator: the cross-domain invariants a coordinator consolidated", Role: "drafter", Tools: AllTools, Dir: d("principal-coordinator"), Prefix: p("principal-coordinator"), Base: "coord"},
		{Name: "principal-domain-owner", ReportsTo: "coord", Job: "promoted domain owner: the domain invariants a domain owner consolidated", HoldsGate: true, Sideways: true, Role: "judge", Tools: AllTools, Dir: d("principal-domain-owner"), Prefix: p("principal-domain-owner"), Base: "domain"},
		{Name: "auditor", ReportsTo: Orchestrator, Job: "the auditor: reviews verdicts and the log, never authors", Role: "judge", Tools: []string{"read", "glob", "grep", "policy"}, Dir: d("auditor"), Prefix: p("auditor")},
	}
}

// Ranks is a table with lookups.
type Ranks []Rank

// Get finds a rank by name; "orchestrator" is the synthetic root, whose shelf is everything. The
// orchestrator holds no role: its calls run on the mount and are journaled with no role label —
// roles belong to dispatched Subagents and to the binary's own routed calls.
func (rs Ranks) Get(name string) (Rank, bool) {
	name = rank.Canon(name)
	if name == Orchestrator {
		return Rank{Name: Orchestrator, Job: "the session: thinks, decides, orders", Tools: []string{"*"}}, true
	}
	for _, r := range rs {
		if r.Name == name {
			return r, true
		}
	}
	return Rank{}, false
}

// Of reads a member's rank off its id prefix ("" for orchestrator or an unknown name). The first
// rank on a prefix wins.
func (rs Ranks) Of(member string) string {
	member = strings.ToLower(member)
	for _, r := range rs {
		if r.Prefix != "" && strings.HasPrefix(member, r.Prefix) {
			return r.Name
		}
	}
	return ""
}

// ByDir is the base rank living in a folder.
func (rs Ranks) ByDir(dir string) (Rank, bool) {
	for _, r := range rs {
		if r.Dir == dir && r.Dir != "" {
			return r, true
		}
	}
	return Rank{}, false
}

// Below reports whether rank `lower` sits under `upper` in the tree (any number of hops).
func (rs Ranks) Below(lower, upper string) bool {
	if upper == Orchestrator {
		return lower != Orchestrator
	}
	seen := map[string]bool{}
	for cur := lower; cur != "" && cur != Orchestrator && !seen[cur]; {
		seen[cur] = true
		r, ok := rs.Get(cur)
		if !ok {
			return false
		}
		if r.ReportsTo == upper {
			return true
		}
		cur = r.ReportsTo
	}
	return false
}

// GateAbove reports whether some rank at or above `name` holds a gate.
func (rs Ranks) GateAbove(name string) bool {
	seen := map[string]bool{}
	for cur := name; cur != "" && cur != Orchestrator && !seen[cur]; {
		seen[cur] = true
		r, ok := rs.Get(cur)
		if !ok {
			return false
		}
		if r.HoldsGate {
			return true
		}
		cur = r.ReportsTo
	}
	return false
}

// Validate checks the policy's shape: a tree rooted at orchestrator, every rank with an escalation
// parent, every authoring rank with a gate holder at or above it (Policy 3), promoted ranks with
// an existing base, dirs and prefixes consistent.
func (rs Ranks) Validate() error {
	seen := map[string]bool{}
	for _, r := range rs {
		if r.Name == "" || r.Name == Orchestrator {
			return fmt.Errorf("rank %q: not a name", r.Name)
		}
		if seen[r.Name] {
			return fmt.Errorf("rank %s listed twice", r.Name)
		}
		seen[r.Name] = true
	}
	for _, r := range rs {
		if r.ReportsTo == "" {
			return fmt.Errorf("rank %s has no escalation parent (Absolute Rule III)", r.Name)
		}
		if _, ok := rs.Get(r.ReportsTo); !ok {
			return fmt.Errorf("rank %s reports to unknown rank %s", r.Name, r.ReportsTo)
		}
		if !rs.Below(r.Name, Orchestrator) || !reachesRoot(rs, r.Name) {
			return fmt.Errorf("rank %s does not reach orchestrator (the hierarchy is a tree rooted there)", r.Name)
		}
		if r.Authors && !rs.GateAbove(r.Name) {
			return fmt.Errorf("rank %s authors but no rank at or above it holds a gate (Policy 3)", r.Name)
		}
		if r.Base != "" {
			if _, ok := rs.Get(r.Base); !ok {
				return fmt.Errorf("promoted rank %s has unknown base %s", r.Name, r.Base)
			}
		}
		if r.PromotesWhen != "" {
			if r.Base == "" {
				return fmt.Errorf("rank %s: promotesWhen is a promoted rank's field (%s has no base)", r.Name, r.Name)
			}
			as, err := ParseASL(r.PromotesWhen)
			if err != nil {
				return fmt.Errorf("rank %s: promotesWhen %q: %v", r.Name, r.PromotesWhen, err)
			}
			if as.Target != r.Base {
				return fmt.Errorf("rank %s: promotesWhen promotes %s, not its base %s", r.Name, as.Target, r.Base)
			}
		}
		if len(r.DivineRules) > 0 && r.Base == "" {
			return fmt.Errorf("rank %s: divineRules is a promoted rank's field (%s has no base)", r.Name, r.Name)
		}
		if r.Dir != "" && r.Prefix != "" && r.Prefix != r.Dir+"-" {
			return fmt.Errorf("rank %s: prefix %q must be dir %q + \"-\"", r.Name, r.Prefix, r.Dir)
		}
	}
	return nil
}

func reachesRoot(rs Ranks, name string) bool {
	seen := map[string]bool{}
	for cur := name; ; {
		if cur == Orchestrator {
			return true
		}
		if seen[cur] {
			return false
		}
		seen[cur] = true
		r, ok := rs.Get(cur)
		if !ok {
			return false
		}
		cur = r.ReportsTo
	}
}

// Layout is the onto layout of this table under a lexicon.
func (rs Ranks) Layout(lex Lexicon) onto.Layout {
	l := onto.Layout{WorkspaceDir: lex.WorkspaceDir, Policy: lex.Policy, Ranks: []onto.RankDir{}}
	baseDirs := map[string]bool{}
	for _, r := range rs {
		if r.Base == "" && r.Dir != "" {
			baseDirs[r.Dir] = true
		}
	}
	for _, r := range rs {
		if r.Dir == "" {
			continue
		}
		parent := r.ReportsTo
		// a promoted rank's members live in the base's dir under the base's class — unless
		// config gave the promoted its own dir, which walks as its own class
		if r.Base != "" && baseDirs[r.Dir] {
			continue
		}
		l.Ranks = append(l.Ranks, onto.RankDir{Name: r.Name, Dir: r.Dir, Parent: parent})
	}
	return l
}

// ExpandTools resolves a rank's tool list against the names on offer: `*` is every tool, a
// glob (path.Match) picks by pattern, a plain name is itself (kept even when absent, so a
// later add still lands). Order follows the offer for globs.
func ExpandTools(patterns, offer []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, p := range patterns {
		switch {
		case p == "*":
			for _, n := range offer {
				add(n)
			}
		case strings.ContainsAny(p, "*?["):
			for _, n := range offer {
				if ok, _ := path.Match(p, n); ok {
					add(n)
				}
			}
		default:
			add(p)
		}
	}
	return out
}

// Tools is the shelf a rank is cut to: its own list, else its base's, else everything.
func (rs Ranks) Tools(name string) []string {
	r, ok := rs.Get(name)
	if !ok {
		return AllTools
	}
	if len(r.Tools) == 0 && r.Base != "" {
		if b, ok := rs.Get(r.Base); ok && len(b.Tools) > 0 {
			return b.Tools
		}
	}
	if len(r.Tools) == 0 {
		return AllTools
	}
	return r.Tools
}

// RoleOf reads the role a commission's @ASK names (tool.RoleOf); no word → analyst,
// the role that perceives.
func RoleOf(ask string) string {
	if o := tool.RoleOf(ask); o != "" {
		return o
	}
	return "analyst"
}

// ─── Layers, neighbors, divine rules, strict flow, promotion ────────────────────────────

// NeighborRanks lists the rank names sharing `rank`'s layer. Knowledge, not permission: every
// rank knows its layer-mates (you told it zone workers know their neighbors; so do domain owners and coordinators).
// Whether an agent may *dispatch* a neighbor is the rank's Sideways flag, enforced in the
// Dispatcher — never here. A layer is the configured Layer number when set, else the set of
// ranks reporting to one parent. orchestrator is alone.
func (rs Ranks) NeighborRanks(rank string) []string {
	if rank == "" || rank == Orchestrator {
		return nil
	}
	r, ok := rs.Get(rank)
	if !ok {
		return nil
	}
	out := []string{rank}
	bound := map[string]bool{rank: true, Orchestrator: true}
	for _, o := range rs {
		if bound[o.Name] {
			continue
		}
		if r.Layer != 0 && o.Layer == r.Layer {
			out = append(out, o.Name)
			bound[o.Name] = true
			continue
		}
		if r.Layer == 0 && o.Layer == 0 && o.ReportsTo == r.ReportsTo {
			out = append(out, o.Name)
			bound[o.Name] = true
		}
	}
	return out
}

// DivineRulesFor collects the absolute rules promoted ranks declare for `base` — every rank
// whose Base is `base`, in table order. Coded, never prompted: the harness laces them into the
// base rank's "Rules in force" block.
func (rs Ranks) DivineRulesFor(base string) []string {
	var out []string
	for _, r := range rs {
		if r.Base == base {
			out = append(out, r.DivineRules...)
		}
	}
	return out
}

// nextHop is the rank directly under `from` on the path to `to`, walking `to`'s ReportsTo
// chain; "" when `to` is not below `from`.
func (rs Ranks) nextHop(from, to string) string {
	if from == to {
		return from
	}
	hop := ""
	seen := map[string]bool{}
	for cur := to; cur != "" && cur != Orchestrator && !seen[cur]; {
		seen[cur] = true
		r, ok := rs.Get(cur)
		if !ok {
			return ""
		}
		if r.ReportsTo == from {
			hop = cur
		}
		cur = r.ReportsTo
	}
	return hop
}

// firstChildRank is the first rank directly under `name` in table order; "" when none.
// Coded, used by the refusal texts to name the lawful next hop.
func (rs Ranks) firstChildRank(name string) string {
	for _, r := range rs {
		if r.ReportsTo == name {
			return r.Name
		}
	}
	return ""
}

// DirectChild reports whether `to` is exactly one hop under `from`: the strict layer flow.
// Between two ranks at one layer it reports whether they neighbor (a sideways hop). Strict
// flow holds neighbors and the ladder; the divine bond is not a layer — a promoted overload
// dispatches over its base's layer.
func (rs Ranks) DirectChild(from, to string) bool {
	if from == Orchestrator {
		r, ok := rs.Get(to)
		return ok && r.ReportsTo == Orchestrator
	}
	if !rs.In(to) || !rs.In(from) {
		return false
	}
	if rel := rs.relate(from, to); rel == RelNeighbor {
		return true // a sideways intead of down, within one layer
	}
	return rs.nextHop(from, to) == to
}

// BelowAs is Below for a dispatcher of rank `upper`: a promoted rank commands what its base
// commanded (a principal domain owner keeps its zone workers), so a rank under the base counts as under it.
func (rs Ranks) BelowAs(lower, upper string) bool {
	if rs.Below(lower, upper) {
		return true
	}
	if r, ok := rs.Get(upper); ok && r.Base != "" && lower != upper {
		return rs.Below(lower, r.Base)
	}
	return false
}

// DirectChildAs is DirectChild for a dispatcher of rank `from`, its base's children included
// when it is a promoted rank.
func (rs Ranks) DirectChildAs(from, to string) bool {
	if rs.DirectChild(from, to) {
		return true
	}
	if r, ok := rs.Get(from); ok && r.Base != "" && to != from {
		if t, ok := rs.Get(to); ok && t.ReportsTo == r.Base {
			return true
		}
	}
	return false
}

// Commands reports whether a body of this rank has anyone to dispatch: orchestrator, a sideways rank,
// or a rank some other rank reports to (its base's children count for a promoted rank). A rank
// that commands no one is never handed the dispatch tool, whatever its tools list says — the
// service owner reports to orchestrator and no rank reports to it.
func (rs Ranks) Commands(name string) bool {
	r, ok := rs.Get(name)
	if !ok {
		return false
	}
	if r.Name == Orchestrator || r.Sideways {
		return true
	}
	for _, o := range rs {
		if o.Name == name {
			continue
		}
		if o.ReportsTo == name || (r.Base != "" && o.ReportsTo == r.Base) {
			return true
		}
	}
	return false
}

// In reports whether the rank name is on the table (orchestrator counts).
func (rs Ranks) In(name string) bool {
	_, ok := rs.Get(name)
	return ok
}

// ChainDepth is the longest ReportsTo chain under orchestrator — how deep a dispatch may nest before
// it passes the table's floor rank. It never counts the floor's own dispatch: the floor holds
// no dispatch tool (its Tools list decides), so one rank past it suffices to stop. Used as the
// effective dispatch depth when config sets none (defaults.go maxDepth pairs with the default
// table's orchestrator → coordinator → domain owner → zone worker = 3; a configured table walks its own length).
func (rs Ranks) ChainDepth() int {
	best := 0
	for _, r := range rs {
		depth, cur, seen := 0, r.Name, map[string]bool{}
		for cur != "" && cur != Orchestrator && !seen[cur] {
			seen[cur] = true
			depth++
			p, ok := rs.Get(cur)
			if !ok || p.Name == Orchestrator {
				break
			}
			cur = p.ReportsTo
		}
		if depth > best {
			best = depth
		}
	}
	return best
}

// Promotion pairs a rank that can promote with the rank it promotes to.
type Promotion struct {
	From, To string
	When     string // the ASL condition on the promoted rank
}

// Promotions walks the table's divine bonds: each promoted rank that names an promotesWhen is
// one evaluation the runtime owes.
func (rs Ranks) Promotions() []Promotion {
	var out []Promotion
	for _, r := range rs {
		if r.Base != "" && strings.TrimSpace(r.PromotesWhen) != "" {
			when, err := ParseASL(r.PromotesWhen)
			if err == nil && when.Target == r.Base {
				out = append(out, Promotion{From: r.Base, To: r.Name, When: r.PromotesWhen})
			}
		}
	}
	return out
}

// Flags are a rank's capabilities as the value policy reads them.
func (rs Ranks) Flags(name string) rank.Flags {
	if rank.Canon(name) == Orchestrator {
		return rank.Flags{Root: true, Commands: true}
	}
	r, ok := rs.Get(name)
	if !ok {
		return rank.Flags{}
	}
	return rank.Flags{Authors: r.Authors, HoldsGate: r.HoldsGate, Commands: rs.Commands(r.Name), ReportsToRoot: r.ReportsTo == Orchestrator, ReadOnly: rank.ReadOnly(rs.Tools(r.Name))}
}

// FamilyOf is a rank's value family: its own when config named one, else derived from its flags.
func (rs Ranks) FamilyOf(name string) string {
	if r, ok := rs.Get(name); ok && r.Family != "" {
		return r.Family
	}
	return rank.FamilyOf(rs.Flags(name))
}
