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
