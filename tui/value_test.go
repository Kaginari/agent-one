package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/Kaginari/agent-one/value"
)

func meterOf(def value.MeterDef, v float64, basis bool) value.Meter {
	return value.Render(def, v, basis)
}

// The sidebar holds the value policy: each rank's meter (label, reading, a bar in its band)
// confident ranks first by their mean, a rank with a short record dimmed and unranked ("low"),
// a custom rank in its family's hue; a subagent row carries its agent's reading; the vitals carry
// the orchestrator's score and the nearest promotion.
func TestRailValueMeters(t *testing.T) {
	th := depthTheme(true)
	landed := value.MeterDef{Signal: "landRate", Label: "Landed", Scale: "percent", Bands: value.Bands{OK: 80, Warn: 50}}
	gate := value.MeterDef{Signal: "gateRight", Label: "Gate right", Scale: "percent", Bands: value.Bands{OK: 90, Warn: 70}}
	asks := value.MeterDef{Signal: "askDoneRate", Label: "Asks done", Scale: "percent", Bands: value.Bands{OK: 80, Warn: 50}}
	v := RailView{Workspace: "agent-one", CtxKnown: true, CtxPct: 40,
		Layers: []RailLayer{{Rank: "orchestrator", Tokens: 9000}, {Rank: "zone", Tokens: 3000}},
		RankNames: []RailRank{
			{Rank: "zone", Family: "author", Meter: meterOf(landed, 62, true), Agents: 3, Confident: 2, Mean: 61},
			{Rank: "domain", Family: "gate", Meter: meterOf(gate, 95, true), Agents: 1, Confident: 1, Mean: 93},
			{Rank: "scribe", Family: "gate", Meter: meterOf(gate, 0, false), Agents: 1, Confident: 0},
			{Rank: "service", Family: "steward", Agents: 0},
		},
		Score:     &RailValue{Meter: meterOf(asks, 88, true)},
		Promote:   "domain-engine · reports 17/20 to promote",
		Subagents: []RailSubagent{{Name: "zone-auth", State: "thinking", Value: &RailValue{Meter: meterOf(landed, 45, true)}}},
	}
	rows := railTexts(th.railRows(v, 48))
	plain := ansi.Strip(strings.Join(rows, "\n"))
	for _, want := range []string{"asks done", "88%", "domain-engine · reports 17/20 to promote", "gate right", "95%", "landed", "62%", "low", "45%"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("the sidebar misses %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "service") {
		t.Fatal("a rank with no agent and no spend is listed")
	}
	// order: confident by mean (domain owner 93, zone worker 61), then the low and the spend-only
	at := func(s string) int { return strings.Index(plain, "● "+s) }
	if !(at("domain") < at("zone") && at("zone") < at("scribe") && at("zone") < at("orchestrator")) {
		t.Fatalf("the ranks' order:\n%s", plain)
	}
	// the bands' colours: domain owner's reading ok, zone worker's warn, the subagent's 45% bad
	joined := strings.Join(rows, "\n")
	for name, hue := range map[string]string{"95%": th.pal.OK, "62%": th.pal.Warn, "45%": th.pal.Danger} {
		if !strings.Contains(joined, sgrFg(hexRGB(hue).col())) {
			t.Errorf("%s is not in its band's colour %s", name, hue)
		}
	}
	// a custom rank holds its family's hue: scribe (gate) the domain owner's lane
	if got, want := th.rankHue("scribe", "gate"), th.lane("domain"); toRGB(got) != toRGB(want) {
		t.Errorf("scribe holds %s, want the domain owner's %s", toRGB(got).hex(), toRGB(want).hex())
	}
}

// The context gauge bands by config's meter, not by numbers of its own: with ui.meters.ctx
// banding 30% as bad, a 40% gauge is drawn in the danger colour.
func TestCtxGaugeBandsFromConfig(t *testing.T) {
	th := depthTheme(true)
	strict := map[string]value.MeterDef{"ctx": {Signal: "ctx", Label: "Context", Scale: "percent", Direction: "down", Bands: value.Bands{OK: 10, Warn: 20}}}
	rows := ansi.Strip(strings.Join(railTexts(th.railRows(RailView{Workspace: "w", CtxKnown: true, CtxPct: 40, Meters: strict}, 40)), "\n"))
	if !strings.Contains(rows, "40%") {
		t.Fatal(rows)
	}
	raw := strings.Join(railTexts(th.railRows(RailView{Workspace: "w", CtxKnown: true, CtxPct: 40, Meters: strict}, 40)), "\n")
	if !strings.Contains(raw, sgrFg(hexRGB(th.pal.Danger).col())) {
		t.Fatal("a 40% context under a 10/20 policy is not drawn as danger")
	}
	calm := strings.Join(railTexts(th.railRows(RailView{Workspace: "w", CtxKnown: true, CtxPct: 40}, 40)), "\n")
	if strings.Contains(calm, sgrFg(hexRGB(th.pal.Danger).col())) {
		t.Fatal("a 40% context under the default policy is danger")
	}
}
