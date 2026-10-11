package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Kaginari/agent-one/loop"
)

// onto hooks — the census rides the write path (Operator's ontology order, 2026-10-03).
//
// Ledger: every write-class act lands an `onto` row in the run's own journal (path, owner, agent)
// so attribution is observable without anyone counting by hand. Nudge: the first unowned write of
// a run rides the tool result back with one advisory line — claim the soil in a member's
// Ownership field, or journal provenance. Advisory by policy: the seat of refusal is the gate; the
// seat of refusal-off is `policy.ownership.enabled: false`. Foreign-owner refusal arrives (warn-first,
// refuse later) once the census is complete enough that a wrong "not yours" cannot stop work.
//
// Switch: `policy.ownership.enabled` — the onto hooks ride ownership policy because ownership is what
// that policy names.

var ontoWriteTools = map[string]bool{
	"write": true, "edit": true, "multiedit": true, "patch": true,
	"str_replace_based_edit_tool": true, "notebook_edit": true,
}

func (a *App) ontoNote() func(ctx context.Context, s *loop.Session, st *loop.StepRecord) string {
	if a.Cfg == nil || !a.Cfg.Policy.Ownership.Enabled || a.Workspace == nil {
		return nil
	}
	return func(ctx context.Context, s *loop.Session, st *loop.StepRecord) string {
		// st.Status is unwritten at Note time (the record step stamps it later) — the run's own
		// lesson: gate on the result, not the stamp (caught live: the day's first real writes
		// all journals zero onto rows because "done" hadn't landed yet)
		if !ontoWriteTools[st.Tool] || st.Result.Err {
			return ""
		}
		var in struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(st.Input, &in) != nil {
			return ""
		}
		if in.Path == "" {
			var alt struct {
				Path string `json:"file_path"`
			}
			if json.Unmarshal(st.Input, &alt) == nil {
				in.Path = alt.Path
			}
		}
		if in.Path == "" {
			return ""
		}
		rel := relOrAbs(a.Root, in.Path)
		owner := a.Workspace.Owner(rel)
		own := "none"
		if owner != nil {
			own = owner.Name
		}
		// the ledger row rides the journal always — the audit counts attribution by reading
		// journals, not by asking
		s.Journal.Log(loop.Event{"t": "onto", "id": st.ID, "tool": st.Tool, "path": rel, "owner": own, "as": agentName(s)})
		if owner != nil {
			return "" // claimed soil speaks no nudge
		}
		a.ontoMu.Lock()
		if a.ontoNudged == nil {
			a.ontoNudged = map[string]bool{}
		}
		if a.ontoNudged[s.RunID] {
			a.ontoMu.Unlock()
			return "" // one nudge per run is the cadence; ten nudges is a tantrum
		}
		a.ontoNudged[s.RunID] = true
		a.ontoMu.Unlock()
		return fmt.Sprintf("\n[onto: `%s` is unowned soil — no member's Ownership field holds it. Claim it there if this is your zone (`agent-one onto owner %s` says so on the wire), or journal provenance. Unowned writes are counted in this run's ledger.]", rel, rel)
	}
}
