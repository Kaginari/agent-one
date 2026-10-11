package app

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Kaginari/agent-one/config"
)

// The three ways to found a workspace (`init --level`):
//   - light (soft): the policy, the log, the instruments and the memory tiers — no member.
//   - medium: light, and a team sketched from the tree with no model, sized to the project:
//     a zone worker per meaningful package or module dir, domain owners grouping related zone workers by imports,
//     role and naming, a coordinator per three domain owners (lane coordinators and a design coordinator on a large project),
//     a service owner when there is CI, release or infra; verify lines only where tests exist.
//   - complex: light, then a founding session — the model reads all the code and decides the
//     team: how many coordinators, domain owners, zone workers, service owners, which skills and which agents, the relations,
//     verify lines that pass; the sketch is its first hint. Like hiring a team of experts.

// Levels are the init levels, in order.
var Levels = []string{"light", "medium", "complex"}

// normLevel reads a level name ("soft" is light); "" when unknown.
func normLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "light", "soft":
		return "light"
	case "medium", "mid":
		return "medium"
	case "complex", "full", "deep":
		return "complex"
	}
	return ""
}

// sketch is a team read from the tree alone (found_survey.go reads, found_cut.go cuts).
type sketch struct {
	Project string
	Lang    string
	Coords  []plan // the design coordinator (or the one coordinator) first, then the lane coordinators
	Domains []plan // each carries its zone workers
	Service *plan
}

type plan struct {
	Rank, Short, Purpose string
	Ownership            []string
	Parent               string
	Skills               []string
	Invariants           []string // the sketch's reading, each marked "sketch:"
	Verify               []string
	Zones                []plan
	Domains              []string // a coordinator's carried members, by name
}

var reName = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	return strings.Trim(reName.ReplaceAllString(strings.ToLower(strings.TrimLeft(s, "_.@")), "-"), "-")
}

// sketchTeam reads the tree into a team, without a model.
func sketchTeam(root string, d config.Dist) sketch {
	project := slug(filepath.Base(root))
	if project == "" {
		project = "core"
	}
	lex := lexiconFor()
	return readSurvey(root, d.WorkspaceDir).cut(project, lex.Prefixes, readSkills(root, d.WorkspaceDir))
}

// doc is a member's README in the shape the ontology reads.
func (p plan) doc(prefix, display map[string]string, level string) string {
	var b strings.Builder
	name := prefix[p.Rank] + p.Short
	fmt.Fprintf(&b, "# %s\n\n", name)
	rank := display[p.Rank]
	if rank == "" {
		rank = strings.ToUpper(p.Rank[:1]) + p.Rank[1:]
	}
	fmt.Fprintf(&b, "- **Rank:** %s\n", rank)
	if len(p.Ownership) > 0 {
		t := make([]string, len(p.Ownership))
		for i, x := range p.Ownership {
			t[i] = "`" + x + "`"
		}
		fmt.Fprintf(&b, "- **Owns:** %s\n", strings.Join(t, ", "))
	}
	fmt.Fprintf(&b, "- **Reports to:** %s\n", p.Parent)
	if len(p.Skills) > 0 {
		fmt.Fprintf(&b, "- **Skills:** %s\n", strings.Join(p.Skills, ", "))
	} else {
		b.WriteString("- **Skills:** none\n")
	}
	fmt.Fprintf(&b, "- **Purpose:** %s\n", p.Purpose)
	fmt.Fprintf(&b, "- **Founded:** `init --level %s`, from the tree alone — the invariants marked sketch are the founding sketch's reading; refine them and the verify lines from the code\n", level)
	b.WriteString("\n## Invariants\n")
	if len(p.Invariants) == 0 {
		b.WriteString("- (none recorded yet: the first session that works here writes what it learns)\n")
	}
	for _, t := range p.Invariants {
		fmt.Fprintf(&b, "- %s\n", t)
	}
	if len(p.Verify) > 0 {
		b.WriteString("\n## Verify\n")
		for _, v := range p.Verify {
			fmt.Fprintf(&b, "- `%s`\n", v)
		}
	}
	b.WriteString("\n## Working notes\n")
	return b.String()
}

// all is every member of the sketch, parents first.
func (s sketch) all() []plan {
	out := append([]plan(nil), s.Coords...)
	out = append(out, s.Domains...)
	for _, o := range s.Domains {
		out = append(out, o.Zones...)
	}
	if s.Service != nil {
		out = append(out, *s.Service)
	}
	return out
}

// count is the sketch's census by rank.
func (s sketch) count(rank string) int {
	n := 0
	for _, p := range s.all() {
		if p.Rank == rank {
			n++
		}
	}
	return n
}

// lanes names each coordinator with the domain owners it carries — the report's summary.
func (s sketch) lanes(prefix map[string]string) []string {
	var out []string
	for _, e := range s.Coords {
		var domains []string
		for _, o := range s.Domains {
			if o.Parent == prefix["coord"]+e.Short {
				domains = append(domains, fmt.Sprintf("%s (%d)", prefix["domain"]+o.Short, len(o.Zones)))
			}
		}
		if len(domains) == 0 {
			domains = append(domains, "the project-wide files")
		}
		out = append(out, prefix["coord"]+e.Short+": "+strings.Join(domains, ", "))
	}
	return out
}

// sketchWrite is what writeSketch did: the docs written, the ones kept as they were (a doc of
// that name exists), the ones left out because a member already in the workspace covers them.
type sketchWrite struct {
	wrote, kept, covered []string
}

// existingClaims reads the ownership claims of the members already in the workspace, by name.
func existingClaims(root string, d config.Dist, dirs map[string]string) map[string][]string {
	out := map[string][]string{}
	seen := map[string]bool{}
	for rank, dir := range dirs {
		if dir == "" || seen[dir] || (rank != "zone" && rank != "service") {
			continue
		}
		seen[dir] = true
		ents, _ := os.ReadDir(filepath.Join(root, d.WorkspaceDir, dir))
		for _, e := range ents {
			b, err := os.ReadFile(filepath.Join(root, d.WorkspaceDir, dir, e.Name(), "README.md"))
			if err != nil {
				continue
			}
			for _, l := range strings.Split(string(b), "\n") {
				if !strings.Contains(l, "**Owns:**") {
					continue
				}
				for _, m := range reTick.FindAllStringSubmatch(l, -1) {
					out[dir+"/"+e.Name()] = append(out[dir+"/"+e.Name()], strings.TrimSuffix(strings.TrimPrefix(m[1], "./"), "/"))
				}
			}
		}
	}
	return out
}

var reTick = regexp.MustCompile("`([^`]+)`")

func claimsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// writeSketch writes the sketch's docs under the workspace dir. An existing doc is never touched
// (kept); a zone worker or service owner whose ownership a member already in the workspace claims is left out
// (covered) — a re-run over a refined team adds only what nobody holds; a domain owner or coordinator is
// written only when something below it is (or it is the design coordinator of a workspace with no coordinator).
func writeSketch(root string, d config.Dist, s sketch, dirs, prefix, display map[string]string) (sketchWrite, error) {
	var res sketchWrite
	held := existingClaims(root, d, dirs)
	docPath := func(p plan) string { return filepath.Join(d.WorkspaceDir, dirs[p.Rank], p.Short, "README.md") }
	have := func(p plan) bool { return exists(filepath.Join(root, docPath(p))) }
	covered := func(p plan) bool {
		for owner, cs := range held {
			if strings.HasSuffix(owner, "/"+p.Short) && strings.HasPrefix(owner, dirs[p.Rank]+"/") {
				continue
			}
			for _, c := range cs {
				for _, t := range p.Ownership {
					if claimsOverlap(c, strings.TrimSuffix(t, "/")) {
						return true
					}
				}
			}
		}
		return false
	}
	write := func(p plan) error {
		rel := docPath(p)
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(p.doc(prefix, display, "medium")), 0o644); err != nil {
			return err
		}
		res.wrote = append(res.wrote, filepath.ToSlash(rel))
		return nil
	}
	name := func(p plan) string { return prefix[p.Rank] + p.Short }
	needed := map[string]bool{} // parents something new reports to
	decide := func(p plan) (bool, error) {
		switch {
		case have(p):
			res.kept = append(res.kept, name(p))
			return false, nil
		case covered(p):
			res.covered = append(res.covered, name(p))
			return false, nil
		}
		needed[p.Parent] = true
		return true, write(p)
	}
	for _, o := range s.Domains {
		for _, sl := range o.Zones {
			if _, err := decide(sl); err != nil {
				return res, err
			}
		}
	}
	if s.Service != nil {
		if _, err := decide(*s.Service); err != nil {
			return res, err
		}
	}
	anyCoord := false
	if ents, err := os.ReadDir(filepath.Join(root, d.WorkspaceDir, dirs["coord"])); err == nil && len(ents) > 0 {
		anyCoord = true
	}
	for _, o := range s.Domains {
		if have(o) {
			res.kept = append(res.kept, name(o))
			continue
		}
		if needed[name(o)] {
			needed[o.Parent] = true
			if err := write(o); err != nil {
				return res, err
			}
		}
	}
	for i, e := range s.Coords {
		if have(e) {
			res.kept = append(res.kept, name(e))
			continue
		}
		if needed[name(e)] || (i == 0 && !anyCoord) {
			if err := write(e); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// table is the sketch as the founding brief shows it.
func (s sketch) table(prefix map[string]string) string {
	var b strings.Builder
	for _, p := range s.all() {
		fmt.Fprintf(&b, "- %s%s → %s · %s\n", prefix[p.Rank], p.Short, p.Parent, strings.Join(p.Ownership, ", "))
	}
	return b.String()
}

//go:embed found.md
var foundBrief string

// foundingAsk is the complex level's turn: the brief, the words of this distribution, the sketch.
func foundingAsk(root string, d config.Dist, s sketch, dirs, prefix, display map[string]string) string {
	r := strings.NewReplacer(
		"{{BIN}}", d.Name, "{{WORLD}}", d.WorkspaceDir, "{{PROJECT}}", filepath.Base(root), "{{LANG}}", or(s.Lang, "unknown"),
		"{{VOICE_DIR}}", dirs["coord"], "{{GATE_DIR}}", dirs["domain"], "{{TRUTH_DIR}}", dirs["zone"], "{{LEAD_DIR}}", dirs["service"],
		"{{VOICE}}", prefix["coord"], "{{GATE}}", prefix["domain"], "{{TRUTH}}", prefix["zone"], "{{LEAD}}", prefix["service"],
		"{{TRUTH_RANK}}", or(display["zone"], "Zone worker"),
		"{{SKETCH}}", s.table(prefix))
	return r.Replace(foundBrief)
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
