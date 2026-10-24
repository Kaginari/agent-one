package workspace

import (
	"path"
	"strings"
)

// Soil is what kind of ground a root-relative path stands on — the one reading every
// ownership question asks first (the gate, the ownership policy, the dispatch card).
type Soil int

const (
	// SoilOpen is ground no living member claims.
	SoilOpen Soil = iota
	// SoilOwnership is a member's claimed soil (its Ownership prefixes).
	SoilOwnership
	// SoilSeat is a member's own seat: its directory under <workspaceDir>/<rank>/, where its doc
	// lives. The seat is the member's, whatever another member's broad claim covers.
	SoilSeat
	// SoilHarness is the harness's own bookkeeping — the shared shelf (queue, ledgers, value),
	// the instruments shelf (journals, reports, usage) and the attic (old docs, births). It lands from the harness only and
	// never bills an agent: no member owns it, so no doc is owed for it.
	SoilHarness
	// SoilScratch is the workspace's scratch (<workspaceDir>/tmp/): an agent's own nest — its redirected
	// /tmp, its plan deck. Lawful for every agent, owned by none, never a Docs-as-code debt.
	SoilScratch
)

func (s Soil) String() string {
	return [...]string{"open ground", "ownership", "seat", "harness soil", "scratch"}[s]
}

// Zone is ZoneOfPath's answer: the soil and, on a seat or ownership, the member it is.
type Zone struct {
	Soil   Soil
	Member *Member
}

// harnessShelves and scratchShelf are the workspace-dir children ZoneOfPath reads as harness soil
// and scratch. The attic keeps what the harness set aside: old docs, the births ledger.
var harnessShelves = []string{sharedShelf, "instruments", "attic"}

const scratchShelf = "tmp"

// ZoneOfPath is the one ownership truth for a root-relative path, in order: the harness's
// shelves and the scratch (nobody's), a member's seat (the seat's, even under a broader
// claim — a member's README is never another agent's pairing debt), then the longest
// ownership claim of an authoring member, then of a gate holder. Owner is its member.
func (w *Workspace) ZoneOfPath(rel string) Zone {
	rel = normRel(rel)
	wd := normRel(w.Lex.WorkspaceDir)
	for _, s := range harnessShelves {
		if under(rel, wd+"/"+s) {
			return Zone{Soil: SoilHarness}
		}
	}
	if under(rel, wd+"/"+scratchShelf) {
		return Zone{Soil: SoilScratch}
	}
	var seat *Member
	for i := range w.Members {
		c := &w.Members[i]
		// a foreign agent (another harness's agent file) has a doc and no dir: its doc is its seat
		in := (c.Dir != "" && under(rel, c.Dir)) || (c.Doc != "" && normRel(c.Doc) == rel)
		if in && (seat == nil || len(c.Dir) > len(seat.Dir)) {
			seat = c
		}
	}
	if seat != nil {
		return Zone{Soil: SoilSeat, Member: seat}
	}
	for _, pick := range []func(Rank) bool{func(r Rank) bool { return r.Authors }, func(r Rank) bool { return r.HoldsGate && !r.Authors }} {
		var best *Member
		bestLen := -1
		for i := range w.Members {
			c := &w.Members[i]
			if r, ok := w.Ranks.Get(c.Rank); !ok || !pick(r) {
				continue
			}
			for _, t := range c.Ownership {
				if under(rel, t) && len(t) > bestLen {
					best, bestLen = c, len(t)
				}
			}
		}
		if best != nil {
			return Zone{Soil: SoilOwnership, Member: best}
		}
	}
	return Zone{Soil: SoilOpen}
}

// plansShelf is where the plan tool pens each agent's deck (<workspaceDir>/tmp/plans/<body>.json).
const plansShelf = scratchShelf + "/plans"

// foreignDeck names the agent whose plan deck rel is, when it is not `as`'s own: the scratch is
// every agent's nest, but a deck speaks for its agent — "" when rel is no deck or `as`'s own.
func (w *Workspace) foreignDeck(as, rel string) string {
	dir := normRel(w.Lex.WorkspaceDir + "/" + plansShelf)
	rel = normRel(rel)
	if !under(rel, dir) || strings.Contains(strings.TrimPrefix(rel, dir+"/"), "/") {
		return "" // the archive and anything nested below it is the plan tool's own sweep
	}
	deck := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	if deck == "" || deck == strings.ToLower(or(as, Orchestrator)) {
		return ""
	}
	return deck
}
