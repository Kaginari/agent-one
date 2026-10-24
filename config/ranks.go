package config

import (
	"fmt"
	"github.com/Kaginari/agent-one/value"
	"sort"
	"strings"
)

// RankDef is a rank as config writes it; every field is optional and overrides the built-in
// table's field of the same name (rankSet: extend) or defines a new rank outright.
type RankDef struct {
	ReportsTo    string     `json:"reportsTo"`
	Job          string     `json:"job"`
	Authors      *bool      `json:"authors"`
	HoldsGate    *bool      `json:"holdsGate"`
	Agent        string     `json:"agent"` // read by nothing: every dispatched agent is a Subagent (a set value is a hole)
	Role         string     `json:"role"`
	Tools        []string   `json:"tools"` // globs over tool names
	Model        ModelRef   `json:"model"`
	Dir          string     `json:"dir"`
	Prefix       string     `json:"prefix"`
	PromotesFrom string     `json:"promotesFrom"`
	Sideways     *bool      `json:"sideways"`     // may speak to its own rank (domain owner ↔ domain owner) and name its layer-mates
	Layer        int        `json:"layer"`        // a layer, 0 when unset; two ranks share a layer iff equal and nonzero
	DivineRules  []string   `json:"divineRules"`  // a promoted rank's absolute rules, laced into the base rank's prompt
	PromotesWhen string     `json:"promotesWhen"` // the ASL condition under which a body of this rank promotes ("" = never)
	Family       string     `json:"family"`       // the value family (rank.Families); unset = derived from the flags
	Portrait     string     `json:"portrait"`     // a portrait file, relative to the workspace root, before the embedded one
	Value        *RankValue `json:"value"`        // the rank's own value policy over its family's
}

// Rank is one resolved rank with the origin of each field ("policy" for the built-in table).
type Rank struct {
	Name         string
	ReportsTo    string
	Job          string
	Authors      bool
	HoldsGate    bool
	Role         string
	Tools        []string
	Model        ModelRef
	Dir          string
	Prefix       string
	PromotesFrom string
	Sideways     bool
	Layer        int
	DivineRules  []string
	PromotesWhen string
	Family       string
	Portrait     string
	// Value is the rank's resolved value policy: its own score/meter/minRecord over its family's.
	Value   value.Spec
	Origins map[string]string // field → origin (value.score, value.meter, value.minRecord, family included)
	Builtin bool
}

// RootRank is the session's rank: the tree's root, never configured.
const RootRank = "orchestrator"

// The default promotion conditions: a long record (twenty reports), nearly all landed (a pass rate
// of 90% or more), nearly none leaving a hole (at most one per ten reports). workspace.DefaultRanks
// carries the same strings; `ranks.principal-coordinator.promotesWhen` / `ranks.principal-domain-owner.promotesWhen` override
// them, and an empty string turns promotion off.
const (
	DefaultPromoteCoord  = "promote coord when reports >= 20 && passRate >= 90% && holeRate <= 10%"
	DefaultPromoteDomain = "promote domain when reports >= 20 && passRate >= 90% && holeRate <= 10%"
)

// RenderPromote is the shipped promotion sentence for a base rank with policy.promote's green-floor
// knobs folded in: a landRate floor and a tenure, each clause only when its knob is set — zero
// knobs render the shipped sentence itself.
func RenderPromote(base string, k PromotePolicy) string {
	s := "promote " + base + " when reports >= 20 && passRate >= 90% && holeRate <= 10%"
	if k.GreenFloor > 0 {
		s += fmt.Sprintf(" && landRate >= %d%%", k.GreenFloor)
	}
	if k.Tenure > 0 {
		s += fmt.Sprintf(" && tenure >= %d", k.Tenure)
	}
	return s
}

// applyPromoteKnobs folds policy.promote into every promoted rank still carrying the shipped
// sentence (a workspace that wrote its own promotesWhen keeps it whole).
func (c *Config) applyPromoteKnobs(byName map[string]*Rank, order []string) error {
	k := c.Policy.Promote
	if k.GreenFloor < 0 || k.GreenFloor > 100 {
		return fmt.Errorf("%s: policy.promote.greenFloor %d is a percent, 0..100", c.Where("policy.promote.greenFloor"), k.GreenFloor)
	}
	if k.Tenure < 0 {
		return fmt.Errorf("%s: policy.promote.tenure %d is a count of days, 0 or more", c.Where("policy.promote.tenure"), k.Tenure)
	}
	if k == (PromotePolicy{}) {
		return nil
	}
	where := c.Where("policy.promote.greenFloor")
	if k.GreenFloor == 0 {
		where = c.Where("policy.promote.tenure")
	}
	for _, name := range order {
		r := byName[name]
		if r.PromotesFrom == "" || r.Origins["promotesWhen"] != "policy" || r.PromotesWhen != RenderPromote(r.PromotesFrom, PromotePolicy{}) {
			continue
		}
		r.PromotesWhen = RenderPromote(r.PromotesFrom, k)
		r.Origins["promotesWhen"] = where
	}
	return nil
}

// builtinRanks is AGENT-ONE.md §The workspace as data.
var builtinRanks = []Rank{
	{Name: "coord", ReportsTo: "orchestrator", Job: "the shared skill and voice: thinks across domains, routes work to domain owners, speaks back up", Role: "drafter", Tools: []string{"*"}, Dir: "coord", Prefix: "coord-"},
	{Name: "domain", ReportsTo: "coord", Job: "rules its domain, holds the gate, commands and validates its zone workers", HoldsGate: true, Sideways: true, Role: "judge", Tools: []string{"*"}, Dir: "domain", Prefix: "domain-"},
	{Name: "zone", ReportsTo: "domain", Job: "the ground truth of its zone; authors changes there", Authors: true, Role: "analyst", Tools: []string{"*"}, Dir: "zone", Prefix: "zone-"},
	{Name: "service", ReportsTo: "orchestrator", Job: "a standing domain lead owning one subsystem end-to-end across sessions", Authors: true, HoldsGate: true, Role: "judge", Tools: []string{"*"}, Dir: "service", Prefix: "service-"},
	{Name: "principal-coordinator", PromotesFrom: "coord", Job: "promoted coordinator: holds the accumulated cross-domain invariants", Dir: "principal-coordinator", Prefix: "principal-coord-", PromotesWhen: DefaultPromoteCoord},
	{Name: "principal-domain-owner", PromotesFrom: "domain", Job: "promoted domain owner: holds the accumulated domain invariants", Dir: "principal-domain-owner", Prefix: "principal-domain-", PromotesWhen: DefaultPromoteDomain},
	{Name: "auditor", ReportsTo: "orchestrator", Job: "the auditor: reviews gate verdicts and log.md for policy violations; never authors", Role: "judge", Tools: []string{"read", "glob", "grep", "ls", "recall", "onto", "toolbox", "git", "ask"}, Dir: "auditor", Prefix: "auditor-"},
}

// RankNames lists the built-in ranks in policy order.
func RankNames() []string {
	var out []string
	for _, r := range builtinRanks {
		out = append(out, r.Name)
	}
	return out
}

func (c *Config) buildRanks() error {
	byName := map[string]*Rank{}
	var order []string
	if c.RankSet == "" {
		c.RankSet = "extend"
	}
	if c.RankSet != "extend" && c.RankSet != "replace" {
		return fmt.Errorf("%s: rankSet: %q is not extend or replace", c.Where("rankSet"), c.RankSet)
	}
	if c.RankSet == "extend" {
		for _, r := range builtinRanks {
			rr := r
			rr.Builtin = true
			rr.Origins = map[string]string{}
			for _, f := range rankFields {
				rr.Origins[f] = "policy"
			}
			byName[rr.Name] = &rr
			order = append(order, rr.Name)
		}
	}
	for _, name := range sortedKeys(c.RankDefs) {
		if strings.EqualFold(name, RootRank) {
			return fmt.Errorf("%s: ranks.%s: the root rank is the session and is not configured", c.Where("ranks."+name), name)
		}
		def := c.RankDefs[name]
		r, ok := byName[name]
		if !ok {
			r = &Rank{Name: name, Origins: map[string]string{}}
			byName[name] = r
			order = append(order, name)
		}
		set := func(field string, apply func()) {
			path := "ranks." + name + "." + field
			if _, ok := c.Origins[path]; ok {
				apply()
				r.Origins[field] = c.Where(path)
			}
		}
		set("reportsTo", func() { r.ReportsTo = canonicalRank(def.ReportsTo) })
		set("job", func() { r.Job = def.Job })
		set("authors", func() { r.Authors = *def.Authors })
		set("holdsGate", func() { r.HoldsGate = *def.HoldsGate })
		if _, ok := c.Origins["ranks."+name+".agent"]; ok {
			// a persistent agent does not exist: every dispatched agent is a Subagent with a fresh
			// context, a service owner's included — the field is accepted and named a hole, never obeyed
			c.Holes = append(c.Holes, fmt.Sprintf("ranks.%s.agent is ignored — every dispatched agent runs as a Subagent with a fresh context (%s)", name, c.Where("ranks."+name+".agent")))
		}
		set("role", func() { r.Role = def.Role })
		set("tools", func() { r.Tools = def.Tools })
		set("model", func() { r.Model = def.Model })
		set("dir", func() { r.Dir = def.Dir })
		set("prefix", func() { r.Prefix = def.Prefix })
		set("promotesFrom", func() { r.PromotesFrom = canonicalRank(def.PromotesFrom) })
		set("sideways", func() { r.Sideways = *def.Sideways })
		set("layer", func() { r.Layer = def.Layer })
		set("divineRules", func() { r.DivineRules = def.DivineRules })
		set("promotesWhen", func() { r.PromotesWhen = def.PromotesWhen })
		set("family", func() { r.Family = strings.ToLower(strings.TrimSpace(def.Family)) })
		set("portrait", func() { r.Portrait = def.Portrait })
		// models.ranks.<r> and ranks.<r>.model are one slot.
		if mr, ok := c.Models.Ranks[name]; ok && def.Model.Model != "" && mr.Model != def.Model.Model {
			return fmt.Errorf("%s: ranks.%s.model %q conflicts with models.ranks.%s %q (%s) — one slot, one value",
				c.Where("ranks."+name+".model"), name, def.Model.Model, name, mr.Model, c.Where("models.ranks."+name))
		}
	}
	// Promoted ranks inherit every base field they did not set; dropping one is refused.
	for _, name := range order {
		r := byName[name]
		if r.PromotesFrom == "" {
			continue
		}
		base, ok := byName[r.PromotesFrom]
		if !ok {
			return fmt.Errorf("ranks.%s.promotesFrom: no rank %q (%s)", name, r.PromotesFrom, c.rankWhere(name, "promotesFrom"))
		}
		if base.PromotesFrom != "" {
			return fmt.Errorf("ranks.%s.promotesFrom: %q is itself promoted", name, r.PromotesFrom)
		}
		// A field config did not set is inherited; a held capability config drops is refused.
		inherit := func(field string, apply func()) {
			if o := r.Origins[field]; o == "" || o == "policy" {
				apply()
				r.Origins[field] = "promotes:" + base.Name
			}
		}
		inherit("reportsTo", func() { r.ReportsTo = base.ReportsTo })
		inherit("authors", func() { r.Authors = base.Authors })
		inherit("holdsGate", func() { r.HoldsGate = base.HoldsGate })
		inherit("sideways", func() { r.Sideways = base.Sideways })
		inherit("role", func() { r.Role = base.Role })
		inherit("tools", func() { r.Tools = base.Tools })
		inherit("model", func() { r.Model = base.Model })
		inherit("layer", func() { r.Layer = base.Layer })
		for f, dropped := range map[string]bool{"authors": base.Authors && !r.Authors, "holdsGate": base.HoldsGate && !r.HoldsGate, "sideways": base.Sideways && !r.Sideways} {
			if dropped {
				return fmt.Errorf("ranks.%s.%s: an promoted rank keeps everything %s held (%s)", name, f, base.Name, r.Origins[f])
			}
		}
		// divineRules and promotesWhen are the promoted rank's own — never inherited from a base
		// (a base does not carry its overload's rules or its evolution condition). So are dir and
		// prefix: the promoted agent lives in its own dir (default: the rank's name), or its registered
		// name would be its base's.
	}
	if err := c.applyPromoteKnobs(byName, order); err != nil {
		return err
	}
	// Validations that need every name resolved (promotesWhen parses and must name its base).
	for _, name := range order {
		r := byName[name]
		if r.PromotesWhen != "" {
			if r.PromotesFrom == "" {
				return fmt.Errorf("ranks.%s.promotesWhen: a promoted rank's field — %s promotes from nothing", name, name)
			}
			// the condition's shape is checked at decode; the workspace parses it again before use.
			fields := strings.Fields(r.PromotesWhen)
			if len(fields) < 3 || fields[0] != "promote" {
				return fmt.Errorf("ranks.%s.promotesWhen %q: opens with `promote <rank> when …`", name, r.PromotesWhen)
			}
			if fields[2] != "when" {
				return fmt.Errorf("ranks.%s.promotesWhen %q: missing `when` after the promoted rank", name, r.PromotesWhen)
			}
			if fields[1] != r.PromotesFrom {
				return fmt.Errorf("ranks.%s.promotesWhen: promotes %s, not its base %s", name, fields[1], r.PromotesFrom)
			}
		}
		if len(r.DivineRules) > 0 && r.PromotesFrom == "" {
			return fmt.Errorf("ranks.%s.divineRules: a promoted rank's field — %s promotes from nothing", name, name)
		}
	}
	// Defaults for new ranks.
	for _, name := range order {
		r := byName[name]
		if len(r.Tools) == 0 {
			r.Tools = []string{"*"}
		}
		if r.Dir == "" {
			r.Dir = name
		}
		if r.Prefix == "" {
			r.Prefix = name + "-"
		}
	}
	// The policy's shape.
	for _, name := range order {
		r := byName[name]
		if r.ReportsTo == "" {
			return fmt.Errorf("ranks.%s.reportsTo: every rank has an escalation parent (Absolute Rule III)", name)
		}
		if r.ReportsTo != RootRank {
			if _, ok := byName[r.ReportsTo]; !ok {
				return fmt.Errorf("ranks.%s.reportsTo: no rank %q (%s)", name, r.ReportsTo, c.rankWhere(name, "reportsTo"))
			}
		}
	}
	for _, name := range order {
		seen := map[string]bool{name: true}
		gateAbove := false
		for cur := byName[name]; cur.ReportsTo != RootRank; {
			next := byName[cur.ReportsTo]
			if seen[next.Name] {
				return fmt.Errorf("ranks: a cycle through %s — the hierarchy is a tree rooted at %s", next.Name, RootRank)
			}
			seen[next.Name] = true
			if next.HoldsGate {
				gateAbove = true
			}
			cur = next
		}
		r := byName[name]
		if r.Authors && !gateAbove && !r.HoldsGate {
			return fmt.Errorf("ranks.%s authors changes but no rank above it holds a gate (Policy 3: nothing lands without the gate)", name)
		}
	}
	c.ranks = make([]Rank, 0, len(order))
	for _, name := range order {
		c.ranks = append(c.ranks, *byName[name])
	}
	return nil
}

var rankFields = []string{"reportsTo", "job", "authors", "holdsGate", "role", "tools", "model", "dir", "prefix", "promotesFrom", "sideways", "layer", "divineRules", "promotesWhen", "family", "portrait"}

func (c *Config) rankWhere(name, field string) string { return c.Where("ranks." + name + "." + field) }

// Ranks returns the resolved hierarchy in policy order then config order; Orchestrator is implicit.
func (c *Config) Ranks() []Rank { return append([]Rank(nil), c.ranks...) }

// Rank finds one rank by any spelling of its name (rank.Canon).
func (c *Config) Rank(name string) (Rank, bool) {
	name = canonicalRank(name)
	for _, r := range c.ranks {
		if r.Name == name {
			return r, true
		}
	}
	return Rank{}, false
}

// Deviations lists every rank field whose value did not come from the policy's table, and every
// rank the policy does not have; empty when the workspace runs on the policy as written.
func (c *Config) Deviations() []string {
	var out []string
	if c.RankSet == "replace" {
		out = append(out, "rankSet: replace — the policy's table is not loaded ("+c.Where("rankSet")+")")
	}
	for _, r := range c.ranks {
		if !r.Builtin {
			out = append(out, fmt.Sprintf("rank %s: not in the policy (%s)", r.Name, c.Where("ranks."+r.Name)))
			continue
		}
		fields := sortedKeys(r.Origins)
		for _, f := range fields {
			o := r.Origins[f]
			// a derived family and a family's own policy are the policy's; a rank's own value is not
			if o != "policy" && !strings.HasPrefix(o, "promotes:") && !strings.HasPrefix(o, "derived:") && !(strings.HasPrefix(o, "family:") && !strings.Contains(o, "(")) {
				out = append(out, fmt.Sprintf("rank %s.%s: %s", r.Name, f, o))
			}
		}
	}
	return out
}

// RankTree renders the hierarchy as an indented tree under the orchestrator.
func (c *Config) RankTree() string {
	children := map[string][]string{}
	for _, r := range c.ranks {
		children[r.ReportsTo] = append(children[r.ReportsTo], r.Name)
	}
	for k := range children {
		sort.Strings(children[k])
	}
	var b strings.Builder
	var walk func(name, pad string)
	walk = func(name, pad string) {
		for _, ch := range children[name] {
			r, _ := c.Rank(ch)
			var marks []string
			if r.Authors {
				marks = append(marks, "authors")
			}
			if r.HoldsGate {
				marks = append(marks, "gate")
			}
			if r.Sideways {
				marks = append(marks, "sideways")
			}
			if r.Layer != 0 {
				marks = append(marks, fmt.Sprintf("layer %d", r.Layer))
			}
			if r.PromotesFrom != "" {
				marks = append(marks, "promotes "+r.PromotesFrom)
			}
			if len(r.DivineRules) > 0 {
				marks = append(marks, fmt.Sprintf("%d divine rules", len(r.DivineRules)))
			}
			if r.PromotesWhen != "" {
				// the condition only: "promote <base>" is canonical words, and the base is named above
				cond := r.PromotesWhen
				if _, after, ok := strings.Cut(cond, " when "); ok {
					cond = after
				}
				marks = append(marks, "when `"+strings.TrimSpace(cond)+"`")
			}
			if r.Role != "" {
				marks = append(marks, "role "+r.Role)
			}
			if len(marks) == 0 {
				fmt.Fprintf(&b, "%s%s\n", pad, ch)
			} else {
				fmt.Fprintf(&b, "%s%s (%s)\n", pad, ch, strings.Join(marks, ", "))
			}
			walk(ch, pad+"  ")
		}
	}
	fmt.Fprintf(&b, "%s [rankSet: %s]\n", RootRank, c.RankSet)
	walk(RootRank, "  ")
	return b.String()
}
