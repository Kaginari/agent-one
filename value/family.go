package value

import (
	"math"

	"github.com/Kaginari/agent-one/rank"
)

// Spec is a rank's value policy, resolved: its score expression, the meter that shows it, and the
// record below which an agent's score is low-confidence (never ranked).
type Spec struct {
	Score     string   `json:"score"`
	Meter     MeterDef `json:"meter"`
	MinRecord int      `json:"minRecord,omitempty"`
}

// families is the one source of each family's default value policy — config reads it as the policy's
// layer, the workspace falls back to it.
var families = map[string]Spec{
	rank.FamilyAuthor:  {Score: "0.6*landRate + 0.3*gateRate - 0.1*holeRate", Meter: MeterDef{Signal: "landRate", Label: "Landed", Scale: "percent", Bands: Bands{OK: 80, Warn: 50}, Direction: "up"}},
	rank.FamilyGate:    {Score: "0.5*gateRight + 0.3*zoneHealth + 0.2*landRate", Meter: MeterDef{Signal: "gateRight", Label: "Gate right", Scale: "percent", Bands: Bands{OK: 90, Warn: 70}, Direction: "up"}},
	rank.FamilyPlanner: {Score: "0.6*planLandRate - 0.2*reaskRate + 0.2*landRate", Meter: MeterDef{Signal: "planLandRate", Label: "Plans landed", Scale: "percent", Bands: Bands{OK: 80, Warn: 50}, Direction: "up"}},
	rank.FamilyRoot:    {Score: "askDoneRate", Meter: MeterDef{Signal: "askDoneRate", Label: "Asks done", Scale: "percent", Bands: Bands{OK: 80, Warn: 50}, Direction: "up"}},
	rank.FamilySteward: {Score: "0.7*verifyGreenRate + 0.3*landRate", Meter: MeterDef{Signal: "verifyGreenRate", Label: "Verify green", Scale: "percent", Bands: Bands{OK: 90, Warn: 70}, Direction: "up"}},
	rank.FamilyAuditor: {Score: "0.7*upheldRate + 0.3*passRate", Meter: MeterDef{Signal: "upheldRate", Label: "Upheld", Scale: "percent", Bands: Bands{OK: 80, Warn: 50}, Direction: "up"}},
	rank.FamilyGeneric: {Score: "0.8*passRate - 0.2*holeRate", Meter: MeterDef{Signal: "passRate", Label: "Pass", Scale: "percent", Bands: Bands{OK: 90, Warn: 50}, Direction: "up"}},
}

// FamilyDefault is a family's shipped value policy (generic for an unknown family).
func FamilyDefault(family string) Spec {
	if s, ok := families[family]; ok {
		return s
	}
	return families[rank.FamilyGeneric]
}

// DefaultMinRecord is the record below which a score is low-confidence when config says nothing.
const DefaultMinRecord = 5

// CtxNear is the share of the stress line past which the context instrument reads NEAR — the
// one number the context meter's warn band and the instrument's zone both derive from.
const CtxNear = 0.75

// CtxMeter derives the context meter from policy.budget: ok under the NEAR line, warn up to the
// stress line, bad past it — all as a percent of the window.
func CtxMeter(contextTokens, stressTokens int) MeterDef {
	stress := 90.0
	if contextTokens > 0 && stressTokens > 0 {
		stress = 100 * float64(stressTokens) / float64(contextTokens)
	}
	return MeterDef{Signal: "ctx", Label: "Context", Scale: "percent", Direction: "down",
		Bands: Bands{OK: math.Round(stress * CtxNear), Warn: math.Round(stress)}}
}

// PlanMeter is the plan deck's meter: slices ticked over slices planned.
func PlanMeter() MeterDef {
	return MeterDef{Signal: "plan", Label: "Plan", Scale: "percent", Direction: "up", Bands: Bands{OK: 80, Warn: 40}}
}

// ScoreMeter bands a value score: the net's health tint and any surface that colours a score
// read it (ui.meters.score overrides it).
func ScoreMeter() MeterDef {
	return MeterDef{Signal: "score", Label: "Score", Scale: "percent", Direction: "up", Bands: Bands{OK: 70, Warn: 40}}
}
