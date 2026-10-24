package workspace

import (
	"sort"
	"strings"
)

// Claims and trust: an agent's README names its ownership, and the agent edits that README itself
// (Docs-as-code). Read straight off the docs, that would let an agent widen its own claim onto a
// rank-mate's soil and land there in the same turn — the longest prefix would make it the owner.
// So the claims an agent's writes are judged by are the claims the gate trusts (claimBase), the
// same discipline as the verify lines (selfheal.go):
//
//   - at Open, and at a harness register (relief, promotion, evolution), every claim on disk is
//     trusted;
//   - a claim an agent added since is trusted at once only when it takes nothing from another
//     living agent — no owning agent's trusted claim or seat sits at, under or over it; open
//     ground is an agent's to claim;
//   - a claim that does take another living agent's soil is held back until a gate other than
//     the claimant's — its parent's line or the orchestrator, carrying the claimant's README — passes;
//     the claimant's own gate fails it, and a reload never trusts it on its own.

// trustedClaimsLocked is c's claims as the gate trusts them; w.mu held.
func (w *Workspace) trustedClaimsLocked(c *Member) []string {
	if w.claimBase == nil {
		return c.Ownership
	}
	base := w.claimBase[c.Name]
	var out []string
	for _, t := range c.Ownership {
		if containsRel(base, t) || w.claimTakesLocked(c.Name, t) == "" {
			out = append(out, t)
		}
	}
	return out
}

// claimTakesLocked names the living agent whose trusted soil a claim t of agent takes ("" when it
// takes none): an owning agent's (one that authors or holds a gate) trusted claim or seat at,
// under or over t. w.mu held.
func (w *Workspace) claimTakesLocked(agent, t string) string {
	t = normRel(t)
	for i := range w.Members {
		o := &w.Members[i]
		if o.Name == agent {
			continue
		}
		if r, ok := w.Ranks.Get(o.Rank); !ok || !(r.Authors || r.HoldsGate) {
			continue
		}
		if o.Dir != "" && (under(t, o.Dir) || under(normRel(o.Dir), t)) {
			return o.Name
		}
		claims := o.Ownership
		if base, known := w.claimBase[o.Name]; known {
			claims = base
		}
		for _, p := range claims {
			if under(t, p) || under(normRel(p), t) {
				return o.Name
			}
		}
	}
	return ""
}

// heldClaims is the claims of agent the gate does not trust yet, each with the living agent whose
// soil it takes.
func (w *Workspace) heldClaims(agent string) map[string]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.memberByName(agent)
	if c == nil || w.claimBase == nil {
		return nil
	}
	out := map[string]string{}
	for _, t := range c.Ownership {
		if containsRel(w.claimBase[c.Name], t) {
			continue
		}
		if o := w.claimTakesLocked(c.Name, t); o != "" {
			out[t] = o
		}
	}
	return out
}

// trustedRosterLocked is the roster with each agent's ownership cut to its trusted claims.
func (w *Workspace) trustedRosterLocked() []Member {
	out := make([]Member, len(w.Members))
	for i := range w.Members {
		out[i] = w.Members[i]
		out[i].Ownership = w.trustedClaimsLocked(&w.Members[i])
	}
	return out
}

// trustedZone is ZoneOfPath over the trusted claims, and the agent named on it as the trusted
// roster reads it (nil when the agent is not on the roster).
func (w *Workspace) trustedZone(rel, agent string) (Zone, *Member) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cs := w.trustedRosterLocked()
	var self *Member
	for i := range cs {
		if cs[i].Name == agent {
			self = &cs[i]
		}
	}
	return w.zoneAmong(cs, rel), self
}

// rebaseClaimsLocked takes the claims the gate trusts after a fresh read: everything when all
// is set (Open, a harness register), else what the last base held plus what takes no living agent's
// soil. w.mu held, the fresh roster on w.Members.
func (w *Workspace) rebaseClaimsLocked(all bool) {
	next := map[string][]string{}
	for i := range w.Members {
		c := &w.Members[i]
		if all || w.claimBase == nil {
			next[c.Name] = append([]string(nil), c.Ownership...)
			continue
		}
		next[c.Name] = append([]string(nil), w.trustedClaimsLocked(c)...)
	}
	w.claimBase = next
}

// grantClaims trusts every claim of the agents whose README a passing gate carried for a writer
// in their line above (the parent, its line, the orchestrator) — never the claimant's own gate.
func (w *Workspace) grantClaims(as string, wrote []string) {
	orchestrator := as == "" || as == Orchestrator || w.RankOf(as).Name == Orchestrator
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.claimBase == nil {
		return
	}
	writer := strings.ToLower(or(as, Orchestrator))
	for i := range w.Members {
		c := &w.Members[i]
		if c.Doc == "" || c.Name == writer || !containsRel(wrote, c.Doc) {
			continue
		}
		above := orchestrator
		for cur, seen := c, map[string]bool{}; !above && cur != nil && cur.Parent != "" && !seen[cur.Parent]; cur = w.memberByName(cur.Parent) {
			seen[cur.Parent] = true
			above = cur.Parent == writer
		}
		if above {
			w.claimBase[c.Name] = append([]string(nil), c.Ownership...)
		}
	}
}

func containsRel(xs []string, x string) bool {
	x = normRel(x)
	for _, y := range xs {
		if normRel(y) == x {
			return true
		}
	}
	return false
}

// heldAdmits reports whether rel is admitted to agent only by a claim the gate holds back.
func (w *Workspace) heldAdmits(rel, agent string) bool {
	_, tc := w.trustedZone(rel, agent)
	return tc != nil && !tc.InOwnership(rel)
}

// heldList renders held claims for a refusal: "src/x/ (zone-y's soil), ...".
func heldList(held map[string]string) string {
	ts := make([]string, 0, len(held))
	for t := range held {
		ts = append(ts, t)
	}
	sort.Strings(ts)
	for i, t := range ts {
		ts[i] = t + " (" + held[t] + "'s soil)"
	}
	return strings.Join(ts, ", ")
}

func memberName(c *Member) string {
	if c == nil {
		return ""
	}
	return c.Name
}
