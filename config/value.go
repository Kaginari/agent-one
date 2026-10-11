package config

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/rank"
	"github.com/Kaginari/agent-one/value"
)

// The value policy at load (ranks.md §Value): every rank gets a family (its own, or one derived from
// its flags), a score expression and a meter (its own over its family's over the binary's), each
// with its origin; a signal a rank can never produce is a hole that names it; a grammar or
// band config cannot honour is refused.

// RankFlags are a resolved rank's capabilities as the value policy reads them.
func (c *Config) RankFlags(name string) rank.Flags {
	name = rank.Canon(name)
	if name == RootRank {
		return rank.Flags{Root: true, Commands: true}
	}
	r, ok := c.Rank(name)
	if !ok {
		return rank.Flags{}
	}
	commands := r.Sideways
	for _, o := range c.ranks {
		if o.Name == r.Name {
			continue
		}
		if o.ReportsTo == r.Name || (r.PromotesFrom != "" && o.ReportsTo == r.PromotesFrom) {
			commands = true
		}
	}
	return rank.Flags{Authors: r.Authors, HoldsGate: r.HoldsGate, Commands: commands, ReportsToRoot: r.ReportsTo == RootRank, ReadOnly: rank.ReadOnly(r.Tools)}
}

// FamilySpec is a family's value policy with the origin of each part.
func (c *Config) FamilySpec(family string) (value.Spec, map[string]string) {
	sp := value.FamilyDefault(family)
	origins := map[string]string{"score": "policy", "meter": "policy", "minRecord": "policy"}
	if f := c.Policy.Value.Families[family]; f != nil {
		base := "policy.value.families." + family
		if f.Score != "" {
			sp.Score, origins["score"] = f.Score, c.policyOr(base+".score")
		}
		if f.Meter != nil && f.Meter.Signal != "" {
			sp.Meter, origins["meter"] = *f.Meter, c.policyOr(base+".meter")
		}
		if f.MinRecord > 0 {
			sp.MinRecord, origins["minRecord"] = f.MinRecord, c.policyOr(base+".minRecord")
		}
	}
	if sp.MinRecord <= 0 {
		sp.MinRecord = c.Policy.Value.MinRecord
		origins["minRecord"] = c.policyOr("policy.value.minRecord")
	}
	if sp.MinRecord <= 0 {
		sp.MinRecord = value.DefaultMinRecord
		origins["minRecord"] = "policy"
	}
	return sp, origins
}

// policyOr is a key's origin, "policy" when the binary's defaults set it.
func (c *Config) policyOr(path string) string {
	if o, ok := c.Origins[path]; !ok || o.Layer == "default" {
		return "policy"
	}
	return c.Where(path)
}

// ValueDef is policy.value's definition of success.
func (c *Config) ValueDef() value.Def {
	d := value.Def{Pass: c.Policy.Value.Pass, Fail: c.Policy.Value.Fail, Neutral: c.Policy.Value.Neutral}
	if len(d.Pass)+len(d.Fail)+len(d.Neutral) == 0 {
		return value.DefaultDef()
	}
	return d
}

// ValueWindow is policy.value.window, parsed (0 = the whole ledger).
func (c *Config) ValueWindow() time.Duration {
	d, _ := value.ParseWindow(c.Policy.Value.Window)
	return d
}

// Meters are the session's own meters, resolved: ui.meters.<name> when set, else ctx from
// policy.budget and plan from the binary — each with its origin.
func (c *Config) Meters() (map[string]value.MeterDef, map[string]string) {
	out := map[string]value.MeterDef{"ctx": value.CtxMeter(c.Policy.Budget.ContextTokens, c.Policy.Budget.StressTokens), "plan": value.PlanMeter(), "score": value.ScoreMeter()}
	origins := map[string]string{"ctx": "derived: policy.budget (" + c.Where("policy.budget.stressTokens") + ")", "plan": "policy", "score": "policy"}
	for name, m := range c.UI.Meters {
		if m == nil || m.Signal == "" {
			continue
		}
		out[name], origins[name] = *m, c.Where("ui.meters."+name)
	}
	return out, origins
}

// resolveValues runs after buildRanks: policy.value's own shape, then each rank's value policy.
func (c *Config) resolveValues() error {
	v := &c.Policy.Value
	if _, err := value.ParseWindow(v.Window); err != nil {
		return fmt.Errorf("%s: policy.value.window: %v", c.Where("policy.value.window"), err)
	}
	seen := map[string]string{}
	for _, list := range []struct {
		name  string
		words []string
	}{{"pass", v.Pass}, {"fail", v.Fail}, {"neutral", v.Neutral}} {
		for _, w := range list.words {
			u := strings.ToUpper(strings.TrimSpace(w))
			if prev, ok := seen[u]; ok && prev != list.name {
				return fmt.Errorf("%s: policy.value: %s is both %s and %s — one word, one outcome", c.Where("policy.value."+list.name), u, prev, list.name)
			}
			seen[u] = list.name
		}
	}
	if v.MinRecord < 0 {
		return fmt.Errorf("%s: policy.value.minRecord: %d is below zero", c.Where("policy.value.minRecord"), v.MinRecord)
	}
	for _, fam := range sortedKeys(v.Families) {
		if !rank.IsFamily(fam) {
			return fmt.Errorf("%s: policy.value.families.%s: not a family (%s)", c.Where("policy.value.families."+fam), fam, strings.Join(rank.Families(), " | "))
		}
	}
	for name, m := range c.UI.Meters {
		if m == nil {
			continue
		}
		if err := value.CheckMeter(*m); err != nil {
			return fmt.Errorf("%s: ui.meters.%s: %v", c.Where("ui.meters."+name), name, err)
		}
	}
	// the families' policies parse on their own, whether or not a rank holds them
	for _, fam := range rank.Families() {
		sp, origins := c.FamilySpec(fam)
		if err := c.checkSpec("policy.value.families."+fam, sp, origins, rank.Flags{}, false); err != nil {
			return err
		}
	}
	for i := range c.ranks {
		r := &c.ranks[i]
		flags := c.RankFlags(r.Name)
		if r.Family != "" {
			if !rank.IsFamily(r.Family) {
				return fmt.Errorf("%s: ranks.%s.family: %q is not a family (%s)", c.rankWhere(r.Name, "family"), r.Name, r.Family, strings.Join(rank.Families(), " | "))
			}
		} else {
			r.Family = rank.FamilyOf(flags)
			r.Origins["family"] = "derived: " + flagWords(flags)
		}
		sp, origins := c.FamilySpec(r.Family)
		for k, o := range origins {
			if o == "policy" {
				origins[k] = "family:" + r.Family
			} else {
				origins[k] = "family:" + r.Family + " (" + o + ")"
			}
		}
		if def := c.RankDefs[r.Name]; def != nil && def.Value != nil {
			base := "ranks." + r.Name + ".value"
			if def.Value.Score != "" {
				sp.Score, origins["score"] = def.Value.Score, c.Where(base+".score")
			}
			if def.Value.Meter != nil && def.Value.Meter.Signal != "" {
				sp.Meter, origins["meter"] = *def.Value.Meter, c.Where(base+".meter")
			}
			if def.Value.MinRecord > 0 {
				sp.MinRecord, origins["minRecord"] = def.Value.MinRecord, c.Where(base+".minRecord")
			}
		}
		if err := c.checkSpec("ranks."+r.Name+".value", sp, origins, flags, true); err != nil {
			return err
		}
		r.Value = sp
		for k, o := range origins {
			r.Origins["value."+k] = o
		}
		// promotesWhen: the full grammar, and the base's signals (the ranking agent is the base)
		if r.PromotesWhen != "" {
			_, cond, _ := strings.Cut(r.PromotesWhen, " when ")
			e, err := value.ParseCondition(cond, value.IsSignal)
			if err != nil {
				return fmt.Errorf("%s: ranks.%s.promotesWhen %q: %v", c.rankWhere(r.Name, "promotesWhen"), r.Name, r.PromotesWhen, err)
			}
			baseFlags := c.RankFlags(r.PromotesFrom)
			for _, sig := range e.Signals() {
				if !value.Produces(baseFlags, sig) {
					c.Holes = append(c.Holes, fmt.Sprintf("ranks.%s.promotesWhen reads %s, which a %s never produces — the condition stands on a zero (%s)", r.Name, sig, r.PromotesFrom, c.rankWhere(r.Name, "promotesWhen")))
				}
			}
		}
	}
	return nil
}

// checkSpec parses a value policy; with produced set, a signal the flags never produce is a hole.
func (c *Config) checkSpec(at string, sp value.Spec, origins map[string]string, flags rank.Flags, produced bool) error {
	e, err := value.ParseNumber(sp.Score, value.IsSignal)
	if err != nil {
		return fmt.Errorf("%s: %s.score %q: %v", origins["score"], at, sp.Score, err)
	}
	if err := value.CheckMeter(sp.Meter); err != nil {
		return fmt.Errorf("%s: %s.meter: %v", origins["meter"], at, err)
	}
	if !produced {
		return nil
	}
	sigs := e.Signals()
	if sp.Meter.Signal != "score" {
		sigs = append(sigs, sp.Meter.Signal)
	}
	for _, sig := range sigs {
		if !value.Produces(flags, sig) {
			c.Holes = append(c.Holes, fmt.Sprintf("%s reads %s, which this rank never produces (%s) — it stands on a zero; name a signal it makes or set the rank's family (%s)", at, sig, flagWords(flags), origins["score"]))
		}
	}
	return nil
}

// flagWords names a rank's capabilities for an origin line.
func flagWords(f rank.Flags) string {
	var w []string
	for _, x := range []struct {
		on   bool
		word string
	}{{f.Root, "root"}, {f.Authors, "authors"}, {f.HoldsGate, "holdsGate"}, {f.Commands, "commands"}, {f.ReportsToRoot, "under the orchestrator"}, {f.ReadOnly, "read-only"}} {
		if x.on {
			w = append(w, x.word)
		}
	}
	if len(w) == 0 {
		return "no capability flag"
	}
	return strings.Join(w, ", ")
}

// ExplainValue renders each rank's resolved value policy — family, score, meter, minimum record —
// with the origin of each part (policy / family / rank / file:line), the definition of success,
// the window and the session meters.
func (c *Config) ExplainValue() string {
	var b strings.Builder
	d := c.ValueDef()
	state := "on"
	if !c.Policy.Value.Enabled {
		state = "OFF (" + c.Where("policy.value.enabled") + ")"
	}
	window := c.Policy.Value.Window
	if window == "" {
		window = "all"
	}
	fmt.Fprintf(&b, "# value policy · %s · window %s  # %s\n", state, window, c.Where("policy.value.window"))
	fmt.Fprintf(&b, "pass:    %s  # %s\n", strings.Join(d.Pass, ", "), c.Where("policy.value.pass"))
	fmt.Fprintf(&b, "fail:    %s  # %s\n", strings.Join(d.Fail, ", "), c.Where("policy.value.fail"))
	fmt.Fprintf(&b, "neutral: %s  # %s\n", strings.Join(d.Neutral, ", "), c.Where("policy.value.neutral"))
	meter := func(m value.MeterDef) string {
		dir := m.Direction
		if dir == "" {
			dir = "up"
		}
		return fmt.Sprintf("%s %q (%s, %s; ok %g · warn %g)", m.Signal, m.Label, or(m.Scale, "percent"), dir, m.Bands.OK, m.Bands.Warn)
	}
	root, ro := c.FamilySpec(rank.FamilyRoot)
	fmt.Fprintf(&b, "\n%s  family root  # derived: the session\n", RootRank)
	fmt.Fprintf(&b, "  score: %s  # %s\n  meter: %s  # %s\n  minRecord: %d  # %s\n", root.Score, ro["score"], meter(root.Meter), ro["meter"], root.MinRecord, ro["minRecord"])
	for _, r := range c.ranks {
		fmt.Fprintf(&b, "\n%s  family %s  # %s\n", r.Name, r.Family, or(r.Origins["family"], c.rankWhere(r.Name, "family")))
		fmt.Fprintf(&b, "  score: %s  # %s\n", r.Value.Score, r.Origins["value.score"])
		fmt.Fprintf(&b, "  meter: %s  # %s\n", meter(r.Value.Meter), r.Origins["value.meter"])
		fmt.Fprintf(&b, "  minRecord: %d  # %s\n", r.Value.MinRecord, r.Origins["value.minRecord"])
		if r.PromotesWhen != "" {
			fmt.Fprintf(&b, "  promotesWhen: %s  # %s\n", r.PromotesWhen, or(r.Origins["promotesWhen"], "policy"))
		}
		if r.Portrait != "" {
			fmt.Fprintf(&b, "  portrait: %s  # %s\n", r.Portrait, r.Origins["portrait"])
		}
	}
	ms, mo := c.Meters()
	names := make([]string, 0, len(ms))
	for n := range ms {
		names = append(names, n)
	}
	sort.Strings(names)
	b.WriteString("\n# session meters\n")
	for _, n := range names {
		fmt.Fprintf(&b, "%s: %s  # %s\n", n, meter(ms[n]), mo[n])
	}
	for _, h := range c.Holes {
		if strings.Contains(h, ".value") || strings.Contains(h, "promotesWhen") {
			fmt.Fprintf(&b, "@? %s\n", h)
		}
	}
	return b.String()
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
