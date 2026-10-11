// Package rank holds the two facts every surface needs about a rank name and nothing else: its
// one spelling (Design) and the family its capabilities put it in (FamilyOf). Config, the workspace,
// the ontology, the board, the TUI's keys and the portrait files all spell a rank through Design;
// the value policy reads a rank's job through its family. No package below this one names a rank.
package rank

import (
	"sort"
	"strings"
)

// Root is the session's rank, the tree's root.
const Root = "orchestrator"

// squashed are the spellings the ontology's class names and the old board keys left behind
// (PrincipalCoordinator → principalcoordinator, ZoneWorker → zone-worker): a reader folds them back to the one spelling.
var squashed = map[string]string{"principalcoordinator": "principal-coordinator", "principaldomainowner": "principal-domain-owner",
	// the class names (ZoneWorker) and the display words (Zone worker) of the built-in ranks
	"zone-worker": "zone", "domain-owner": "domain", "coordinator": "coord", "service-owner": "service"}

// Design folds any spelling of a rank name to the one canonical form: lower case, words joined
// by a hyphen — principal_coordinator, PrincipalCoordinator and principalcoordinator all read principal-coordinator. A custom rank keeps its
// own words (night_watch → night-watch). The empty string stays empty.
func Canon(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	// CamelCase (an ontology class local name) splits on its humps before it folds
	if strings.ToLower(s) != s && strings.ToUpper(s) != s && !strings.ContainsAny(s, "-_ ") {
		var b strings.Builder
		for i, r := range s {
			if i > 0 && r >= 'A' && r <= 'Z' {
				b.WriteByte('-')
			}
			b.WriteRune(r)
		}
		s = b.String()
	}
	s = strings.ToLower(s)
	s = strings.NewReplacer("_", "-", " ", "-").Replace(s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if to, ok := squashed[s]; ok {
		return to
	}
	return s
}

// Aliases lists the spellings a reader accepts for a canonical name (the name first): the
// canonical, the underscore form and the squashed form — the shapes older files carry.
func Aliases(name string) []string {
	c := Canon(name)
	out := []string{c}
	add := func(s string) {
		for _, x := range out {
			if x == s {
				return
			}
		}
		out = append(out, s)
	}
	add(strings.ReplaceAll(c, "-", "_"))
	add(strings.ReplaceAll(c, "-", ""))
	return out
}

// Equal reports whether two spellings name one rank.
func Equal(a, b string) bool { return Canon(a) == Canon(b) }

// The families: what a rank's capabilities say its job is. A rank's value policy defaults to its
// family's, so a custom rank measured by nothing it can produce never happens silently.
const (
	FamilyRoot    = "root"    // the orchestrator: answers the human
	FamilyAuthor  = "author"  // authors changes and holds no gate (zone worker)
	FamilyGate    = "gate"    // holds the gate and commands (domain owner, principal-domain-owner)
	FamilyPlanner = "planner" // commands and holds no gate (coordinator, principal-coordinator)
	FamilySteward = "steward" // authors and holds its own gate, reporting to the orchestrator (service owner)
	FamilyAuditor = "auditor" // read-only: never writes (auditor)
	FamilyGeneric = "generic" // none of the above
)

// Families lists every family in the docs order.
func Families() []string {
	return []string{FamilyRoot, FamilyPlanner, FamilyGate, FamilyAuthor, FamilySteward, FamilyAuditor, FamilyGeneric}
}

// IsFamily reports whether s names a family.
func IsFamily(s string) bool {
	for _, f := range Families() {
		if f == s {
			return true
		}
	}
	return false
}

// Flags are the capabilities a family is derived from — data from the rank table, never a name.
type Flags struct {
	Root          bool // the session itself
	Authors       bool // authors changes in its ownership
	HoldsGate     bool // gates the authoring ranks below it
	Commands      bool // has anyone to dispatch (a child rank, sideways, or its base's children)
	ReportsToRoot bool // one hop under the orchestrator
	ReadOnly      bool // its tool shelf holds no write
}

// FamilyOf derives a family from capabilities, in this order: the root; authors that hold their
// own gate under the orchestrator (steward); authors with no gate (author); a gate that commands
// (gate); a commander with no gate (planner); a read-only rank (auditor); anything else generic.
func FamilyOf(f Flags) string {
	switch {
	case f.Root:
		return FamilyRoot
	case f.Authors && f.HoldsGate && f.ReportsToRoot:
		return FamilySteward
	case f.Authors && !f.HoldsGate:
		return FamilyAuthor
	case f.HoldsGate && f.Commands:
		return FamilyGate
	case f.Commands && !f.HoldsGate:
		return FamilyPlanner
	case f.ReadOnly:
		return FamilyAuditor
	}
	return FamilyGeneric
}

// writeTools are the tool names a read-only rank's shelf lacks.
var writeTools = []string{"write", "edit", "multiedit", "patch", "delete", "bash", "shell"}

// ReadOnly reports whether a shelf of tool patterns (globs, `*`) holds no writing tool.
func ReadOnly(tools []string) bool {
	if len(tools) == 0 {
		return false // an empty shelf is every tool
	}
	for _, p := range tools {
		for _, w := range writeTools {
			if globMatch(p, w) {
				return false
			}
		}
	}
	return true
}

// globMatch is a `*`/`?` match without path semantics (a tool name has no slash).
func globMatch(pat, s string) bool {
	if pat == "*" || pat == s {
		return true
	}
	if !strings.ContainsAny(pat, "*?") {
		return false
	}
	var m func(p, t string) bool
	m = func(p, t string) bool {
		for len(p) > 0 {
			switch p[0] {
			case '*':
				for i := 0; i <= len(t); i++ {
					if m(p[1:], t[i:]) {
						return true
					}
				}
				return false
			case '?':
				if len(t) == 0 {
					return false
				}
			default:
				if len(t) == 0 || p[0] != t[0] {
					return false
				}
			}
			p, t = p[1:], t[1:]
		}
		return len(t) == 0
	}
	return m(pat, s)
}

// Sorted returns canonical names, sorted and deduplicated.
func Sorted(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		c := Canon(n)
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}
