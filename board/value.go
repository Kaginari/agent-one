package board

import (
	"net/http"
	"time"

	"github.com/Kaginari/agent-one/rank"
	"github.com/Kaginari/agent-one/value"
)

// ValueView is the value policy's picture as the integrator hands it to the board: every agent's
// value (keyed by its name), the full report /api/value serves, the resolved meters, and the one
// definition of success the activity counts its failures by.
type ValueView struct {
	Lit    bool                      // the ledger was read (a workspace with policy.value off is silent)
	Why    string                    // why it is silent
	Window time.Duration             // policy.value.window; 0 = the whole ledger
	Def    value.Def                 // policy.value's pass / fail / neutral words
	Agents map[string]NodeValue      // agent name → its value
	Report any                       // the whole report (workspace.ValueReport), served as is
	Meters map[string]value.MeterDef // the session's meters (ctx, plan) and nothing else
}

// NodeValue is one agent's value as a node carries it: the score, how sure, the band its rank's
// meter puts it in, the meter itself, and its strain.
type NodeValue struct {
	Score      float64     `json:"score"`
	Confidence string      `json:"confidence"` // ok | low (under the rank's minRecord: never ranked)
	Band       string      `json:"band"`       // ok | warn | bad | none
	Meter      value.Meter `json:"meter"`
	Family     string      `json:"family"`
	Strain     int         `json:"strain"`
	Reports    int         `json:"reports"`
}

// RankMeta is what the integrator knows of a rank beyond its name: its value family (the icon and
// the hue fall back to it) and a configured portrait file (ranks.<r>.portrait, root-relative).
type RankMeta struct {
	Rank     string `json:"rank"`
	Family   string `json:"family"`
	Portrait string `json:"portrait,omitempty"`
	Label    string `json:"label,omitempty"`
}

// activityDays is the activity tiles' window: policy.value.window in whole days, else ActivityDays.
func (b *Board) activityDays() int {
	if b.src.Value != nil {
		if v := b.src.Value(); v != nil && v.Window >= 24*time.Hour {
			return int(v.Window / (24 * time.Hour))
		}
	}
	return ActivityDays
}

// def is policy.value's definition, the shipped one when the integrator gave none.
func (b *Board) def() value.Def {
	if b.src.Value != nil {
		if v := b.src.Value(); v != nil && len(v.Def.Pass)+len(v.Def.Fail) > 0 {
			return v.Def
		}
	}
	return value.DefaultDef()
}

// meters are the session meters: the integrator's (from config), else the binary's own.
func (b *Board) meters() map[string]value.MeterDef {
	if b.src.Value != nil {
		if v := b.src.Value(); v != nil && len(v.Meters) > 0 {
			return v.Meters
		}
	}
	return map[string]value.MeterDef{"ctx": value.CtxMeter(0, 0), "plan": value.PlanMeter(), "score": value.ScoreMeter()}
}

// apiValue serves the value report: per agent and per rank — signals, score, confidence, meter,
// strain, promotion progress — and the meters. A board with no integrator answers silent.
func (b *Board) apiValue(w http.ResponseWriter, r *http.Request) {
	if b.src.Value == nil {
		writeJSON(w, map[string]any{"lit": false, "why": "no running session: the value ledger is read by the session that keeps it"})
		return
	}
	v := b.src.Value()
	if v == nil || !v.Lit {
		why := "the value ledger is silent"
		if v != nil && v.Why != "" {
			why = v.Why
		}
		writeJSON(w, map[string]any{"lit": false, "why": why})
		return
	}
	writeJSON(w, map[string]any{"lit": true, "report": v.Report, "meters": b.meters(), "ranks": b.rankMetas()})
}

// apiMeters serves the resolved meters: the session's (ctx, plan) and every rank's.
func (b *Board) apiMeters(w http.ResponseWriter, r *http.Request) {
	ranks := map[string]value.MeterDef{}
	if b.src.Value != nil {
		if v := b.src.Value(); v != nil {
			if rep, ok := v.Report.(interface {
				RankMeters() map[string]value.MeterDef
			}); ok {
				ranks = rep.RankMeters()
			}
		}
	}
	writeJSON(w, map[string]any{"session": b.meters(), "ranks": ranks})
}

// rankMetas lists every rank the integrator knows, canonical.
func (b *Board) rankMetas() []RankMeta {
	if b.src.Ranks == nil {
		return []RankMeta{}
	}
	out := b.src.Ranks()
	for i := range out {
		out[i].Rank = rank.Canon(out[i].Rank)
	}
	if out == nil {
		out = []RankMeta{}
	}
	return out
}

// valueOf is an agent's value, nil when the ledger holds nothing for it.
func (b *Board) valueOf(name string) *NodeValue {
	if b.src.Value == nil {
		return nil
	}
	v := b.src.Value()
	if v == nil || !v.Lit {
		return nil
	}
	if nv, ok := v.Agents[name]; ok {
		return &nv
	}
	return nil
}
