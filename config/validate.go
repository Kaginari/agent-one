package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// fileLayers are the layers whose word is the human's written word.
var fileLayers = map[string]bool{"global": true, "project": true, "local": true}

// finish runs every post-decode step and the policy's refusals.
func (c *Config) finish() error {
	steps := []func() error{
		c.checkScalars, c.checkProviders, c.checkTools, c.checkMCP, c.checkPermissions,
		c.resolveRules, c.mergeRegistry, c.buildRanks, c.resolveValues, c.checkModels, c.checkGate, c.checkCompaction, c.checkGuts,
	}
	for _, s := range steps {
		if err := s(); err != nil {
			return err
		}
	}
	sort.Strings(c.Holes)
	return nil
}

func (c *Config) checkScalars() error {
	enum := func(path, val string, allowed ...string) error {
		if val == "" || contains(allowed, val) {
			return nil
		}
		return fmt.Errorf("%s: %s: %q is not %s", c.Where(path), path, val, strings.Join(allowed, " | "))
	}
	if err := enum("mode", c.Mode, "build", "plan"); err != nil {
		return err
	}
	if err := enum("output.format", c.Output.Format, "text", "json", "wire"); err != nil {
		return err
	}
	if err := enum("compaction.strategy", c.Compaction.Strategy, "drain", "summary"); err != nil {
		return err
	}
	if err := enum("logLevel", strings.ToUpper(c.LogLevel), "DEBUG", "INFO", "WARN", "ERROR"); err != nil {
		return err
	}
	if err := enum("output.color", c.Output.Color, "auto", "always", "never"); err != nil {
		return err
	}
	switch m := c.Tools.TmpRedirect.Mode; m {
	case "", "rewrite", "refuse":
	default:
		return fmt.Errorf("tools.tmpRedirect.mode: %q is not rewrite | refuse", m)
	}
	return nil
}

// checkGate applies the refusals around the human gate: a full gate off is honored only from
// the machine's own config files; standing approvals of the gated classes never come from env or
// the tracked project layer; classes must exist.
func (c *Config) checkGate() error {
	if !c.Policy.HumanGate.Enabled {
		o, _ := c.Origins["policy.humanGate.enabled"]
		if !fileLayers[o.Layer] {
			return fmt.Errorf("%s: policy.humanGate.enabled: false is honored only from a config file (global or local) — never env or a flag", o)
		}
		if o.Layer == "project" {
			return fmt.Errorf("%s: policy.humanGate.enabled: false %s", o, fromClone)
		}
	}
	for i, a := range c.Policy.HumanGate.Approve {
		path := fmt.Sprintf("policy.humanGate.approve[%d]", i)
		if _, ok := classOrder[a]; !ok {
			return fmt.Errorf("%s: %s: %q is not read, write, outward or destructive", c.Where(path), path, a)
		}
		o, _ := c.Origins[path]
		if o.Layer == "env" || o.Layer == "env-file" {
			return fmt.Errorf("%s: %s: a standing pre-approval is the human's written word — a config file or --approve, never env", o, path)
		}
		if o.Layer == "project" && classOrder[a] >= classOrder["outward"] {
			return fmt.Errorf("%s: %s: %s %s", o, path, a, fromClone)
		}
	}
	return nil
}

// fromClone is the refusal for a loosening of the human gate in the tracked project layer.
const fromClone = "is refused from the project config: it travels with the repository, so anyone who clones it " +
	"would run with the gate loosened — put it in .agent-one/config.local.yaml or the global config, which are the machine's own"

func (c *Config) checkCompaction() error {
	if c.Compaction.Enabled && c.Compaction.Trigger.Tokens >= c.Policy.Budget.ContextTokens {
		return fmt.Errorf("%s: compaction.trigger.tokens %d ≥ policy.budget.contextTokens %d — a drain that never fires is a disabled drain; say enabled: false instead",
			c.Where("compaction.trigger.tokens"), c.Compaction.Trigger.Tokens, c.Policy.Budget.ContextTokens)
	}
	if c.Policy.Budget.StressTokens > c.Policy.Budget.ContextTokens {
		return fmt.Errorf("%s: policy.budget.stressTokens %d is past contextTokens %d", c.Where("policy.budget.stressTokens"), c.Policy.Budget.StressTokens, c.Policy.Budget.ContextTokens)
	}
	return nil
}

// Finding is one honesty-rule line: a switched-off policy feature and where it was switched.
type Finding struct {
	Key    string
	Origin Origin
}

func (f Finding) String() string { return fmt.Sprintf("@? off %s — %s", f.Key, f.Origin) }

// honestyRoots are the sections whose `enabled: false` is a finding, not silence.
var honestyRoots = []string{"policy", "memory", "instruments", "toolbox", "ontology", "compaction", "hooks", "permissions", "mcp", "discovery"}

// Off lists every policy feature whose live value is enabled: false, plus the standing
// approvals and status knobs the spec treats the same way, each with its origin. A feature
// the binary ships off (origin default) is not listed: the finding is a switch someone threw.
func (c *Config) Off() []Finding {
	var out []Finding
	seen := map[string]bool{}
	add := func(key string) {
		if seen[key] {
			return
		}
		seen[key] = true
		o, ok := c.Origins[key]
		if !ok || o.Layer == "default" {
			return // shipped off (a LATER feature): nobody threw a switch
		}
		out = append(out, Finding{Key: key, Origin: o})
	}
	walkEnabled(reflect.ValueOf(c).Elem(), "", func(path string, on bool) {
		if on {
			return
		}
		for _, root := range honestyRoots {
			if path == root+".enabled" || strings.HasPrefix(path, root+".") {
				add(path)
				return
			}
		}
	})
	for i := range c.Policy.HumanGate.Approve {
		add(fmt.Sprintf("policy.humanGate.approve[%d]", i))
	}
	if !c.Instruments.Status.ShowOff {
		add("instruments.status.showOff")
	}
	if c.Policy.Forge.DemandAtSteps <= 0 {
		add("policy.forge.demandAtSteps")
	}
	// the numeric and string switches: a zero, a negative or an empty string someone wrote is a
	// feature turned off, named with its origin like any `enabled: false`
	if c.Policy.Stress.EscalateAt <= 0 {
		add("policy.stress.escalateAt")
	}
	if c.Policy.Stress.RelieveAt <= 0 {
		add("policy.stress.relieveAt")
	}
	if c.Policy.DoomLoop.Threshold <= 0 {
		add("policy.doomLoop.threshold")
	}
	if c.Policy.Guts.BenchThrashVisits < 0 {
		add("policy.guts.benchThrashVisits")
	}
	if c.Policy.Guts.DigestEvery <= 0 {
		add("policy.guts.digestEvery")
	}
	if c.Policy.Budget.SalvageTokens < 0 {
		add("policy.budget.salvageTokens")
	}
	for _, r := range c.ranks {
		if r.PromotesFrom != "" && strings.TrimSpace(r.PromotesWhen) == "" {
			add("ranks." + r.Name + ".promotesWhen")
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// walkEnabled visits every `enabled` bool reachable from a struct, with its dotted path.
func walkEnabled(v reflect.Value, path string, visit func(string, bool)) {
	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			walkEnabled(v.Elem(), path, visit)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
			if tag == "" || tag == "-" {
				continue
			}
			f := v.Field(i)
			p := join(path, tag)
			if tag == "enabled" && f.Kind() == reflect.Bool {
				visit(p, f.Bool())
				continue
			}
			walkEnabled(f, p, visit)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkEnabled(v.MapIndex(k), join(path, k.String()), visit)
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			walkEnabled(v.Index(i), fmt.Sprintf("%s[%d]", path, i), visit)
		}
	}
}

// OffLines renders the honesty-rule findings for status, one line each.
func (c *Config) OffLines() []string {
	var out []string
	for _, f := range c.Off() {
		out = append(out, f.String())
	}
	return out
}

// mergeRegistry folds registry.models into the providers' catalogues: an entry joins
// providers.<p>.models unless the provider declares that model itself; a provider that is not
// configured is a hole, never a silent drop.
func (c *Config) mergeRegistry() error {
	for _, key := range sortedKeys(c.Registry.Models) {
		p, id, err := SplitModel(key)
		if err != nil {
			return fmt.Errorf("%s: registry.models.%s: %v", c.Where("registry.models."+key), key, err)
		}
		prov := c.Providers[p]
		if prov == nil {
			c.Holes = append(c.Holes, fmt.Sprintf("registry.models.%s: no provider %q is configured (%s)", key, p, c.Where("registry.models."+key)))
			continue
		}
		if prov.Models == nil {
			prov.Models = map[string]*ModelEntry{}
		}
		if _, ok := prov.Models[id]; !ok {
			prov.Models[id] = c.Registry.Models[key]
		}
	}
	return nil
}

// checkGuts refuses a guts floor above zero: the floor is the momentum the gut speaks at, and a
// positive one would speak on a run that is gaining ground. Zero is a floor like any other.
func (c *Config) checkGuts() error {
	g := c.Policy.Guts
	if g.Floor > 0 {
		return fmt.Errorf("%s: policy.guts.floor: %d — the gut speaks when the momentum falls to the floor, so the floor is ≤ 0", c.Where("policy.guts.floor"), g.Floor)
	}
	if g.Clamp != nil && g.Clamp.Min > g.Clamp.Max {
		return fmt.Errorf("%s: policy.guts.clamp: min %d over max %d", c.Where("policy.guts.clamp"), g.Clamp.Min, g.Clamp.Max)
	}
	if g.Ring < 0 {
		return fmt.Errorf("%s: policy.guts.ring: %d is below zero", c.Where("policy.guts.ring"), g.Ring)
	}
	for i, b := range g.Benches {
		if len(b.Match) == 0 {
			return fmt.Errorf("%s: policy.guts.benches[%d]: a bench names the commands it matches (match)", c.Where(fmt.Sprintf("policy.guts.benches[%d]", i)), i)
		}
	}
	return nil
}
