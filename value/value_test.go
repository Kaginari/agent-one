package value

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExprArithmeticAndConditions(t *testing.T) {
	m := map[string]float64{"landRate": 80, "gateRate": 50, "holeRate": 10, "reports": 12}
	e, err := ParseNumber("0.6*landRate + 0.3*gateRate - 0.1*holeRate", IsSignal)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Eval(m); got != 0.6*80+0.3*50-0.1*10 {
		t.Fatalf("score = %v", got)
	}
	if got, _ := ParseNumber("(landRate + gateRate) / 2", IsSignal); got.Eval(m) != 65 {
		t.Fatalf("parens and division: %v", got.Eval(m))
	}
	if got, _ := ParseNumber("landRate / (gateRate - 50)", IsSignal); got.Eval(m) != 0 {
		t.Fatal("a division by zero reads zero")
	}
	if got, _ := ParseNumber("-holeRate + 100", IsSignal); got.Eval(m) != 90 {
		t.Fatal("unary minus")
	}
	c, err := ParseCondition("reports >= 10 && 0.5*landRate + 0.5*gateRate >= 65%", IsSignal)
	if err != nil {
		t.Fatal(err)
	}
	if !c.True(m) {
		t.Fatal("arithmetic inside a comparison")
	}
	// the rendered form parses back to the same reading
	back, err := ParseCondition(c.String(), IsSignal)
	if err != nil || back.True(m) != c.True(m) {
		t.Fatalf("round trip %q: %v", c.String(), err)
	}
	for _, bad := range []string{"", "landRate >=", "foo >= 1", "landRate && gateRate", "reports > 1 + (2 >= 1)", "a = 1", "1 & 2", "(landRate"} {
		if _, err := Parse(bad, IsSignal); err == nil {
			if _, err2 := ParseCondition(bad, IsSignal); err2 == nil {
				t.Errorf("%q must not parse", bad)
			}
		}
	}
	if _, err := ParseNumber("reports >= 1", IsSignal); err == nil {
		t.Error("a condition is not a score")
	}
	if _, err := ParseCondition("reports + 1", IsSignal); err == nil {
		t.Error("a number is not a condition")
	}
	leaves := c.Leaves(map[string]float64{"reports": 4, "landRate": 80, "gateRate": 50})
	if len(leaves) != 2 || leaves[0].Met || leaves[0].Distance != 6 || !leaves[1].Met {
		t.Fatalf("leaves read each comparison's distance: %+v", leaves)
	}
}

func TestDefOneDefinitionOfSuccess(t *testing.T) {
	d := DefaultDef()
	cases := map[string]string{"DONE": Pass, "@S PASS — tests green": Pass, "FAIL": Fail, "escalate": Fail, "CHECKPOINT": Fail,
		"DENIED": Neutral, "INTERRUPTED": Neutral, "DRY": Neutral, "SUSPENDED": Neutral, "HOLE": Neutral, "": Neutral}
	for w, want := range cases {
		if got := d.Outcome(w); got != want {
			t.Errorf("%q → %s, want %s", w, got, want)
		}
	}
	// the run's word: the loop's when it did not close DONE, else the agent's own
	if RunWord("DONE", "FAIL — red") != "FAIL" || RunWord("CHECKPOINT", "DONE") != "CHECKPOINT" || RunWord("DONE", "") != "DONE" || RunWord("DENIED", "DONE") != "DENIED" {
		t.Fatal("RunWord")
	}
}

func run(agent string, at time.Time, loop, word string, mod func(*Row)) Row {
	r := Row{Kind: KindRun, At: at, Agent: agent, Loop: loop, Word: word, Status: RunWord(loop, word)}
	if mod != nil {
		mod(&r)
	}
	return r
}

func TestFoldSignals(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	t0 := now.Add(-time.Hour)
	rows := []Row{
		run("zone-a", t0, "DONE", "DONE", func(r *Row) { r.Gate = "pass"; r.By = "domain-x"; r.Holes = Holes{Infra: 3, Self: 2} }),
		run("zone-a", t0, "DONE", "DONE", func(r *Row) { r.Gate = "pass"; r.GateTries = 1; r.By = "domain-x"; r.Holes = Holes{Harness: 4} }),
		run("zone-a", t0, "DONE", "FAIL", func(r *Row) { r.By = "domain-x"; r.Strain = 3; r.Warrant = true }),
		run("zone-a", t0, "DENIED", "", func(r *Row) { r.By = "domain-x" }), // the human's word: neutral
		run("zone-a", t0, "DONE", "DONE", func(r *Row) { r.By = "domain-x"; r.Strain = 0 }),
		run("zone-a", now.Add(-60*24*time.Hour), "FAIL", "", nil), // outside the window
		{Kind: KindVerdict, At: t0, Agent: "domain-x", Of: "zone-a", Status: "pass", Paths: []string{"a.go"}},
		{Kind: KindVerdict, At: t0, Agent: "domain-x", Of: "zone-a", Status: "pass", Paths: []string{"b.go"}},
		{Kind: KindOverturn, At: t0, Agent: "domain-x", Of: "zone-a", Paths: []string{"a.go"}},
	}
	out := Fold(rows, Opts{Window: 28 * 24 * time.Hour, Now: now, EscalateAt: 5})
	s := out["zone-a"]
	if s["runs"] != 5 || s["reports"] != 4 || s["passes"] != 3 || s["fails"] != 1 || s["neutral"] != 1 {
		t.Fatalf("counts (DENIED is neutral, the old run is out of the window): %v", s)
	}
	if s["holes"] != 4 || s["holeRate"] != 25 {
		t.Fatalf("only harness holes cost, per report: holes %v holeRate %v", s["holes"], s["holeRate"])
	}
	if s["passRate"] != 75 || s["landRate"] != 75 {
		t.Fatalf("passRate %v landRate %v", s["passRate"], s["landRate"])
	}
	if s["gateRate"] != 50 {
		t.Fatalf("first-try gate passes: %v", s["gateRate"])
	}
	o := out["domain-x"]
	if o["verdicts"] != 2 || o["overturned"] != 1 || o["gateRight"] != 50 {
		t.Fatalf("the holder's verdicts: %v", o)
	}
	if o["planLandRate"] != 60 || o["reaskRate"] != 20 {
		t.Fatalf("dispatches landed 3 of 5 (the DENIED one never lands), one re-ask after a warrant: plan %v reask %v", o["planLandRate"], o["reaskRate"])
	}
	if o["zoneHealth"] != 100 {
		t.Fatalf("zone-a's strain cleared on its last landing: %v", o["zoneHealth"])
	}
	// holeRate never passes 100%, however many holes one report carries
	many := Fold([]Row{run("x", t0, "DONE", "DONE", func(r *Row) { r.Holes = Holes{Harness: 40} })}, Opts{Now: now})
	if many["x"]["holeRate"] != 100 {
		t.Fatalf("holeRate cap: %v", many["x"]["holeRate"])
	}
}

func TestAskDoneRateReopens(t *testing.T) {
	now := time.Now()
	turn := func(word string, paths ...string) Row {
		return Row{Kind: KindRun, At: now, Agent: "orchestrator", Loop: "DONE", Word: word, Status: RunWord("DONE", word), Turn: true, Paths: paths}
	}
	rows := []Row{turn("", "a.go"), turn("", "b.go"), turn("", "a.go"), turn("", "c.go")}
	s := Fold(rows, Opts{Now: now, ReopenTurns: 3})["orchestrator"]
	// the first turn's a.go was rewritten two turns later: reopened
	if s["askDoneRate"] != 75 {
		t.Fatalf("askDoneRate %v", s["askDoneRate"])
	}
}

func TestMeterBands(t *testing.T) {
	up := MeterDef{Signal: "landRate", Label: "Landed", Bands: Bands{OK: 80, Warn: 50}}
	if Render(up, 85, true).Band != BandOK || Render(up, 60, true).Band != BandWarn || Render(up, 10, true).Band != BandBad || Render(up, 99, false).Band != BandNone {
		t.Fatal("rising bands")
	}
	ctx := CtxMeter(200000, 180000)
	if ctx.Bands.Warn != 90 || ctx.Bands.OK != 68 || ctx.Direction != "down" {
		t.Fatalf("ctx bands derive from policy.budget: %+v", ctx)
	}
	if Render(ctx, 50, true).Band != BandOK || Render(ctx, 80, true).Band != BandWarn || Render(ctx, 95, true).Band != BandBad {
		t.Fatal("falling bands")
	}
	if err := CheckMeter(MeterDef{Signal: "landRate", Bands: Bands{OK: 10, Warn: 50}}); err == nil {
		t.Fatal("a rising meter's ok under its warn is refused")
	}
	if err := CheckMeter(MeterDef{Signal: "nope"}); err == nil {
		t.Fatal("an unknown signal is refused")
	}
}

func TestSeedFromThePromotionTally(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"kind":"report","agent":"domain-a","status":"DONE","holes":1,"at":"2026-10-01T00:00:00Z"}
{"kind":"report","agent":"domain-a","status":"FAIL","at":"2026-10-01T00:00:00Z"}
{"kind":"promote","agent":"domain-a","new":"principal-domain-a","at":"2026-10-01T00:00:00Z"}
`
	if err := os.MkdirAll(filepath.Join(dir, "instruments"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LegacyShelf), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := Seed(dir)
	if err != nil || n != 3 {
		t.Fatalf("seeded %d: %v", n, err)
	}
	rows, _ := Read(filepath.Join(dir, Shelf))
	s := Fold(rows, Opts{})["domain-a"]
	if s["reports"] != 2 || s["passes"] != 1 || s["fails"] != 1 || s["holes"] != 1 {
		t.Fatalf("the legacy record carries over: %v", s)
	}
	if n, _ := Seed(dir); n != 0 {
		t.Fatal("a ledger is seeded once")
	}
	b, _ := os.ReadFile(filepath.Join(dir, Shelf))
	if !strings.Contains(string(b), `"kind":"seed"`) {
		t.Fatal("the seed is recorded")
	}
}
