package config

import (
	"bytes"
	"strings"
	"testing"
)

// A custom rank gets its family from its flags, the family's score and meter with origins; its
// own value policy overrides the family's; a signal it can never produce is a hole naming it.
func TestCustomRankValuePolicy(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": `
ranks:
  night_watch:
    reportsTo: orchestrator
    tools: [read, grep]
  scribe:
    reportsTo: domain
    authors: true
    value:
      score: "0.5*landRate + 0.5*gateRate"
      meter: {signal: gateRate, label: First try, bands: {ok: 85, warn: 60}}
      minRecord: 8
  herald:
    reportsTo: orchestrator
    tools: [read]
    value:
      score: "gateRight"
`})
	c := w.load()
	nw, ok := c.Rank("night-watch")
	if !ok || nw.Name != "night-watch" {
		t.Fatalf("night_watch is spelled night-watch everywhere: %+v", c.Ranks())
	}
	if nw.Family != "auditor" || !strings.HasPrefix(nw.Origins["family"], "derived:") || nw.Value.Meter.Signal != "upheldRate" || nw.Origins["value.score"] != "family:auditor" {
		t.Fatalf("the auditor family's policy with its origin: %+v %v", nw.Value, nw.Origins)
	}
	sc, _ := c.Rank("scribe")
	if sc.Family != "author" || sc.Value.Score != "0.5*landRate + 0.5*gateRate" || sc.Value.Meter.Label != "First try" || sc.Value.MinRecord != 8 || !strings.Contains(sc.Origins["value.score"], "config.yaml") {
		t.Fatalf("the rank's own policy: %+v %v", sc.Value, sc.Origins)
	}
	hole := false
	for _, h := range c.Holes {
		if strings.Contains(h, "ranks.herald.value reads gateRight, which this rank never produces") {
			hole = true
		}
	}
	if !hole {
		t.Fatalf("a read-only rank scored on gate verdicts is a hole: %v", c.Holes)
	}
	var out bytes.Buffer
	if code := cli([]string{"explain", "value", "--root", w.root}, &out, &out, Options{Home: w.home, Env: func(string) string { return "" }}); code != 0 {
		t.Fatalf("explain value: %d %s", code, out.String())
	}
	for _, frag := range []string{"night-watch  family auditor  # derived: under the orchestrator, read-only", "score: 0.5*landRate + 0.5*gateRate  # ", "family:planner", "ctx: ctx \"Context\" (percent, down; ok 68 · warn 90)  # derived: policy.budget", "pass:    DONE, PASS"} {
		if !strings.Contains(out.String(), frag) {
			t.Errorf("explain value lacks %q:\n%s", frag, out.String())
		}
	}
}

// A score that does not parse, an unknown signal, a family that does not exist and bands a meter
// cannot honour are refused at load.
func TestValuePolicyRefusals(t *testing.T) {
	for name, cfg := range map[string]string{
		"grammar": "ranks:\n  zone:\n    value:\n      score: \"landRate +\"\n",
		"signal":  "ranks:\n  zone:\n    value:\n      score: \"luck\"\n",
		"family":  "ranks:\n  zone:\n    family: wizard\n",
		"bands":   "ranks:\n  zone:\n    value:\n      meter: {signal: landRate, bands: {ok: 10, warn: 50}}\n",
		"window":  "policy:\n  value:\n    window: soon\n",
		"words":   "policy:\n  value:\n    pass: [DONE, FAIL]\n",
		"asl":     "ranks:\n  principal-domain-owner:\n    promotesWhen: \"promote domain when luck >= 1\"\n",
		"floor":   "policy:\n  guts:\n    floor: 2\n",
		"repin":   "policy:\n  guts:\n    repin: 1\n",
	} {
		w := newWorkspace(t, map[string]string{".agent-one/config.yaml": cfg})
		if err := w.loadErr(); err == nil {
			t.Errorf("%s: must be refused", name)
		}
	}
	// a floor of zero is a floor like any other
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "policy:\n  guts:\n    floor: 0\n"})
	if c := w.load(); c.Policy.Guts.Floor != 0 {
		t.Fatal("floor 0 is honoured")
	}
	// the stall guards ship armed: the loop reads 3 and 40, and a workspace tunes or turns them off
	if g := newWorkspace(t, nil).load().Policy.Guts; g.Repin != 3 || g.ActBudget != 40 {
		t.Fatalf("stall guards default: repin %d, actBudget %d", g.Repin, g.ActBudget)
	}
	w = newWorkspace(t, map[string]string{".agent-one/config.yaml": "policy:\n  guts:\n    repin: -1\n    actBudget: 120\n"})
	if g := w.load().Policy.Guts; g.Repin != -1 || g.ActBudget != 120 {
		t.Fatalf("stall guards tuned: %+v", g)
	}
}

// The session meters: ctx derives from policy.budget unless ui.meters.ctx says otherwise.
func TestMetersFromConfig(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "policy:\n  budget:\n    contextTokens: 100000\n    stressTokens: 80000\ncompaction:\n  trigger:\n    tokens: 70000\nui:\n  meters:\n    plan: {signal: plan, label: Deck, bands: {ok: 90, warn: 30}}\n"})
	c := w.load()
	m, o := c.Meters()
	if m["ctx"].Bands.Warn != 80 || m["ctx"].Bands.OK != 60 || !strings.HasPrefix(o["ctx"], "derived: policy.budget") {
		t.Fatalf("ctx from policy.budget: %+v %s", m["ctx"], o["ctx"])
	}
	if m["plan"].Label != "Deck" || m["plan"].Bands.OK != 90 || !strings.Contains(o["plan"], "config.yaml") {
		t.Fatalf("plan from ui.meters: %+v %s", m["plan"], o["plan"])
	}
}

// Off() names the numeric and string switches someone threw, with their origins.
func TestOffListsNumericSwitches(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": `
policy:
  stress: {escalateAt: 0, relieveAt: 0}
  doomLoop: {threshold: 0}
  guts: {benchThrashVisits: -1, digestEvery: 0}
ranks:
  principal-domain-owner:
    promotesWhen: ""
`})
	c := w.load()
	got := strings.Join(c.OffLines(), "\n")
	for _, k := range []string{"policy.stress.escalateAt", "policy.stress.relieveAt", "policy.doomLoop.threshold", "policy.guts.benchThrashVisits", "policy.guts.digestEvery", "ranks.principal-domain-owner.promotesWhen"} {
		if !strings.Contains(got, "@? off "+k+" — ") || !strings.Contains(got, "config.yaml") {
			t.Errorf("Off() lacks %s:\n%s", k, got)
		}
	}
	// the shipped values are not findings
	if lines := newWorkspace(t, nil).load().OffLines(); strings.Contains(strings.Join(lines, "\n"), "policy.stress") {
		t.Fatalf("defaults are not off: %v", lines)
	}
}

// One spelling: every alias of a rank name reads the same rank, keys and values both.
func TestRankCanonSpellings(t *testing.T) {
	w := newWorkspace(t, map[string]string{".agent-one/config.yaml": "ranks:\n  auditor:\n    job: watches\n  DarkCoord2:\n    reportsTo: auditor\n"})
	c := w.load()
	de, ok := c.Rank("auditor")
	if !ok || de.Job != "watches" {
		t.Fatalf("auditor / auditor / auditor are one rank: %+v", de)
	}
	r, ok := c.Rank("dark-coord2")
	if !ok || r.ReportsTo != "auditor" {
		t.Fatalf("a value spelled auditor reads auditor: %+v", r)
	}
}

// policy.evolution: on by default with the owner's numbers; switched off (enabled: false, or a working notes
// limit of 0) it is an honesty finding with its origin; stressAt falls back to the council's
// escalateAt; a mode or an arm the binary cannot run is refused.
func TestEvolutionPolicy(t *testing.T) {
	c := newWorkspace(t, nil).load()
	e := c.Policy.Evolution
	if !c.EvolutionOn() || e.Mode != "auto" || e.NotesMax != 7 || e.CooldownRuns != 10 || strings.Join(e.Arms, ",") != "split,mutate,rebirth" || c.EvolutionStressAt() != c.Policy.Stress.EscalateAt {
		t.Fatalf("the shipped policy: %+v stressAt %d", e, c.EvolutionStressAt())
	}
	if c.Policy.Forge.EvolvedStatus != "trial" {
		t.Fatalf("an evolved skill lands at trial: %q", c.Policy.Forge.EvolvedStatus)
	}
	off := newWorkspace(t, map[string]string{".agent-one/config.yaml": "policy:\n  evolution: {enabled: false, notesMax: 0, stressAt: 4}\n"}).load()
	got := strings.Join(off.OffLines(), "\n")
	for _, k := range []string{"policy.evolution.enabled", "policy.evolution.notesMax"} {
		if !strings.Contains(got, "@? off "+k+" — ") {
			t.Errorf("Off() lacks %s:\n%s", k, got)
		}
	}
	if off.EvolutionOn() || off.EvolutionStressAt() != 4 {
		t.Fatalf("off, with its own stressAt: %v %d", off.EvolutionOn(), off.EvolutionStressAt())
	}
	if !strings.Contains(off.ExplainEvolution(), "OFF") {
		t.Fatalf("explain says off:\n%s", off.ExplainEvolution())
	}
	for _, bad := range []string{"policy:\n  evolution: {mode: yolo}\n", "policy:\n  evolution: {arms: [split, grow]}\n", "policy:\n  forge: {evolvedStatus: live}\n"} {
		if err := newWorkspace(t, map[string]string{".agent-one/config.yaml": bad}).loadErr(); err == nil {
			t.Errorf("refused: %s", bad)
		}
	}
}
