package onto

import (
	"os"
	"strings"
)

// ownerLine mirrors workspace.Owner() against the onto graph and returns the
// winning member, or "" with no owner. Kept as a helper so CLI can wire it
// with its `case` + stdout conventions (cli.go is the dispatch surface).
func ownerLine(root, p string) (string, error) {
	if root == "" {
		root = "."
		if v := os.Getenv("AGENT_ONE_ROOT"); v != "" {
			root = v
		}
	}
	w, err := Load(root)
	if err != nil {
		return "", err
	}
	return ownerOf(w, normPath(p)), nil
}

// ownerOf is the census-side mirror of workspace.Owner(). Longest matching
// ownership wins (same under() semantics as the write gate); on a tie, an
// authoring rank (orchestrator/coord/zone) beats a gate-holding one, mirroring
// workspace/rank.go's Authors table. Lives in onto/ because workspace imports onto —
// a reverse import would cycle.
func ownerOf(w *Workspace, p string) string {
	bestLen := -1
	var candidates []string
	for _, tr := range w.Graph.triples {
		if tr.P != pOwns {
			continue
		}
		owner, ter := tr.S.Value, tr.O.Value
		if !(p == ter || strings.HasPrefix(p, ter+"/")) {
			continue
		}
		if len(ter) > bestLen {
			bestLen, candidates = len(ter), []string{strings.TrimPrefix(owner, NS)}
		} else if len(ter) == bestLen {
			candidates = append(candidates, strings.TrimPrefix(owner, NS))
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	for _, name := range candidates {
		if isAuthoringRank(name) {
			return name
		}
	}
	return candidates[0]
}

// isAuthoringRank maps a member name to whether its rank authors ownership
// (orchestrator/coord/zone true; domain/service/etc. false) — a static mirror of
// workspace/rank.go rank.<name>.Authors. Name prefix before the first '-' is the
// rank word (zone-auth → zone worker).
func isAuthoringRank(name string) bool {
	rank := name
	if i := strings.IndexByte(name, '-'); i > 0 {
		rank = name[:i]
	}
	switch rank {
	case "orchestrator", "coord", "zone":
		return true
	}
	return false
}
