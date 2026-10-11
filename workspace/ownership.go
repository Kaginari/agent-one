package workspace

import (
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/tool"
)

// OwnershipOptions is policy.ownership: an authoring agent's write outside its ownership is
// refused, not warned (Policy 2). Which ranks are held to it is the table's Authors column; Ranks
// narrows it to a list when set.
type OwnershipOptions struct {
	Enabled                  bool
	Ranks                    []string
	OrchestratorNeverAuthors bool
}

func (o OwnershipOptions) held(w *Workspace, c *Member) bool {
	if len(o.Ranks) > 0 {
		return contains(o.Ranks, c.Rank)
	}
	r, ok := w.Ranks.Get(c.Rank)
	return ok && r.Authors
}

// Policy is the tool.Policy for an agent: every path a write/edit touches must be inside its
// ownership. A refusal carries the escalation hint — one hop up, never sideways. The orchestrator is
// bound too under the hard policy (Operator decree 2026-09-30): orchestrator writing member soil is
// refused with a dispatch-redirection — dispatching through the layer chain is the coded
// default; only a member's own README (co-touches pass so pairing stays possible) and the
// orchestrator's own seat files are exempt.
func (w *Workspace) ToolPolicy(agent string, opt OwnershipOptions) tool.Policy {
	if !opt.Enabled {
		return nil
	}
	return func(a tool.Access) error {
		if a.Class < tool.Write || len(a.Paths) == 0 {
			return nil
		}
		// engine soil: journals, reports, ledgers under instruments/loop and instruments/usage
		// land from the harness only. Every agent's write there is refused — Policy 2 grows a
		// sub-case: an agent's report speaks through the wire, the engine files it, nobody edits.
		eng1 := normRel(w.Lex.WorkspaceDir + "/instruments/loop")
		eng2 := normRel(w.Lex.WorkspaceDir + "/instruments/usage")
		for _, p := range a.Paths {
			rel := normRel(w.Rel(p))
			if under(rel, eng1) || under(rel, eng2) {
				return fmt.Errorf("ownership: %s is engine soil (journals and reports land there from the harness only) — an agent never writes under %s; speak on the wire and the engine files it", rel, eng1)
			}
		}
		// the orchestrator, bound (pure hard policy): an orchestrator write inside another member's soil is
		// refused with the chain named — dispatch + the pairing gate are the lawful shapes.
		if agent == "" || agent == Orchestrator || w.RankOf(agent).Name == Orchestrator {
			var seat1, seat2 = normRel(w.Lex.WorkspaceDir + "/system.yaml"), normRel(w.Lex.WorkspaceDir + "/" + w.Lex.Policy)
			for _, p := range a.Paths {
				rel := normRel(w.Rel(p))
				if rel == seat1 || rel == seat2 {
					continue // the orchestrator's own seat files
				}
				if opt.OrchestratorNeverAuthors && rel != seat2 {
					first := w.Ranks.firstChildRank(Orchestrator)
					childHint := "coord"
					if first != "" {
						childHint = first
					}
					return fmt.Errorf("ownership: %s — Policy 7 and the operator's decision of 2026-10-01: the orchestrator authors nothing but the policy (.agent-one/AGENT-ONE.md); dispatch the chain (coordinator -> first layer: `%s`)", rel, childHint)
				}
				owner := w.Owner(rel)
				if owner == nil {
					continue // no-member soil stays lawful for the orchestrator
				}
				if owner.Doc != "" && rel == normRel(owner.Doc) {
					continue // touching the owner's doc itself is the pairing act; allowed
				}
				first := w.Ranks.firstChildRank(Orchestrator)
				childHint := "coord"
				if first != "" {
					childHint = first
				}
				return fmt.Errorf("ownership: %s belongs to %s — Policy 2 plus the hard policy: orchestrator does not author another member's soil; dispatch the chain (coordinator -> domain owner -> zone worker; first layer: `%s`), or patch the config's policy.ownership to loosen this",
					rel, owner.Name, childHint)
			}
			return nil
		}
		c := w.Member(agent)
		if c == nil || !opt.held(w, c) || (len(c.Ownership) == 0 && c.Dir == "") {
			return nil
		}
		for _, p := range a.Paths {
			rel := w.Rel(p)
			if c.InOwnership(rel) {
				continue
			}
			owner := w.Owner(rel)
			if owner != nil && owner.Name != c.Name && owner.Rank == c.Rank {
				// sideways soil of the same rank IS licensed — dispatch the owner directly.
				myRank, _ := w.Ranks.Get(c.Rank)
				if myRank.Sideways {
					return fmt.Errorf("ownership: %s is outside %s's ownership — it belongs to %s, your rank-mate. Dispatch %s directly (sideways licensed across %s); escalation one hop up to %s only if the sideways layer fails",
						rel, c.Name, owner.Name, owner.Name, owner.Rank, or(c.Parent, "the dispatcher"))
				}
			}
			up := c.Parent
			if up == "" {
				up = "the dispatcher"
			}
			// the consult card (2026-10-09 human policy): an agent on foreign soil does not
			// re-derive who owns what — the refusal itself names the owner and the three
			// arms (widen / provisioning / route), so the parent decides without a drain
			ownerLine := " — the soil is unrouted (a Provisioning trigger when it recurs)"
			if owner != nil {
				ownerLine = fmt.Sprintf(" — it belongs to %s (%s, domain %s)", owner.Name, owner.Rank, or(owner.Parent, "orchestrator"))
			}
			return fmt.Errorf("ownership: %s is outside %s's ownership%s — Policy 2: do not write there; consult one hop up to %s with `@? %s`. Arms for %s: widen (grant %s the claim on %s — its README Ownership grows, the doc pairs in-turn), provisioning (register a member for that zone), route (dispatch the owner's line)",
				rel, c.Name, ownerLine, up, oneLine("needs "+rel+" which "+c.Name+" does not own"), up, c.Name, rel)
		}
		return nil
	}
}

// DispatchCard is the dispatcher's one-read answer for a path: the owner, its rank,
// its domain, its strain reading, and the consult arms (widen / provisioning / route) — so an
// domain owner or coordinator grounds its do-it / provisioning / route decision on live state instead of
// re-deriving who owns what (the 2026-10-09 audit: re-derivation chains burned a day).
// The `onto` tool serves it on path queries.
func (w *Workspace) DispatchCard(rel string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	rel = normRel(rel)
	c := w.Owner(rel)
	if c == nil {
		return rel + " is unrouted — no member owns it (a Provisioning trigger when it recurs). Arms: widen an adjacent member's claim, provisioning a member for this zone, or leave it open."
	}
	strain := ""
	if s := w.stress[c.Name]; s > 0 {
		strain = fmt.Sprintf(", strain %d", s)
	}
	dom := or(c.Parent, "orchestrator")
	return fmt.Sprintf("%s is owned by %s (%s%s) — doc %s; domain %s; ownership %s. Arms for the dispatcher: widen (grant another member the claim — its README Ownership grows, Docs-as-code pairs in-turn), provisioning (register a member for a slice of this zone), or route (dispatch %s through %s).",
		rel, c.Name, c.Rank, strain, or(c.Doc, "none"), dom, strings.Join(c.Ownership, ", "), c.Name, dom)
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
