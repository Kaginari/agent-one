package onto

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Kaginari/agent-one/discover"
)

// Workspace is a loaded ontology: the schema, the asserted graph files, the
// triples derived from the members on disk, and the reasoner's closure.
type Workspace struct {
	Root     string // the directory that holds .agent-one/
	Graph    *Graph
	Prefixes map[string]string
	Notes    []string // holes (@?), e.g. a missing schema file
	Docs     map[Term]*DocRef
	Layout   Layout
}

// DocRef points findings back at the doc a member was derived from.
type DocRef struct {
	Path  string         // relative to Root
	Lines map[string]int // field → line number (1-based)
	// Claims is each owns edge's provenance, by the owned path: the lane that registered it and
	// the doc line it stands on — a finding about a claim points at the line that made it.
	Claims map[string]Claim
}

// Claim is one owns edge's provenance.
type Claim struct {
	Lane string // structured (the Ownership field) | prose (a self-naming backticked span)
	Line int    // 1-based line in the doc
}

// String renders the provenance for a finding: `prose, line 12`.
func (c Claim) String() string { return fmt.Sprintf("%s, line %d", c.Lane, c.Line) }

// RankDir is one rank as the derivation needs it: its canonical name, the folder its
// members live in (also their id prefix, `<dir>-<name>`) and the rank it reports to
// ("orchestrator" at the top). Ranks are data (binary.md §Ranks and agents): a name the built-in
// schema knows (coord, domain, zone, service) maps to its class; any other becomes a derived class
// ⊂ Member, and its bond to its parent is the generic `above` unless it is truth- or
// verdict-shaped (zone ⇒ domain, domain ⇒ coord) or reports to orchestrator.
type RankDir struct {
	Name   string
	Dir    string
	Parent string
}

// Layout is what a distribution renames on disk: the workspace directory, the policy file and the
// rank table.
type Layout struct {
	WorkspaceDir string    // ".agent-one"
	Policy       string    // "AGENT-ONE.md"
	Ranks        []RankDir // nil = DefaultLayout's
}

// DefaultLayout is the agent-one distribution's: the policy's own ranks.
func DefaultLayout() Layout {
	return Layout{WorkspaceDir: ".agent-one", Policy: "AGENT-ONE.md", Ranks: []RankDir{
		{"coord", "coord", "orchestrator"}, {"domain", "domain", "coord"}, {"zone", "zone", "domain"}, {"service", "service", "orchestrator"},
	}}
}

func (l Layout) norm() Layout {
	d := DefaultLayout()
	if l.WorkspaceDir != "" {
		d.WorkspaceDir = l.WorkspaceDir
	}
	if l.Policy != "" {
		d.Policy = l.Policy
	}
	if l.Ranks != nil {
		d.Ranks = l.Ranks
	}
	return d
}

// ClassOf is the class term of a rank: the schema's for the policy's ranks, a derived one
// (CamelCase of the name, ⊂ Member) for any other.
func ClassOf(rank string) Term {
	rank = strings.ToLower(strings.TrimSpace(rank))
	if c, ok := rankClass[rank]; ok {
		return c
	}
	var b strings.Builder
	for _, part := range strings.FieldsFunc(rank, func(r rune) bool { return r == '_' || r == '-' || r == ' ' }) {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	if b.Len() == 0 {
		return cMember
	}
	return Ao(b.String())
}

// bondFor is the bond a child rank draws to its parent rank.
func bondFor(child, parent string) Term {
	switch {
	case child == "zone" && parent == "domain":
		return pTruth
	case child == "domain" && parent == "coord":
		return pVerdict
	case parent == "orchestrator":
		return pReports
	}
	return pAbove
}

// namePattern is the member-id pattern for this layout, e.g. (?:zone|domain|coord)-x.
func (l Layout) namePattern() string {
	var ds []string
	seen := map[string]bool{}
	for _, r := range l.Ranks {
		if r.Dir != "" && !seen[r.Dir] {
			seen[r.Dir] = true
			ds = append(ds, regexp.QuoteMeta(r.Dir))
		}
	}
	if len(ds) == 0 {
		ds = []string{"never-matches-anything"}
	}
	return `(?:` + strings.Join(ds, "|") + `)-[a-z0-9][a-z0-9-]*`
}

// FindRoot walks up from dir to the nearest directory holding .agent-one/.
func FindRoot(dir string) (string, error) { return FindRootIn(dir, DefaultLayout()) }

// FindRootIn walks up from dir to the nearest directory holding the layout's workspace dir.
func FindRootIn(dir string, l Layout) (string, error) {
	l = l.norm()
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if st, err := os.Stat(filepath.Join(d, l.WorkspaceDir)); err == nil && st.IsDir() {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", errors.New("no " + l.WorkspaceDir + "/ found here or above")
		}
		d = parent
	}
}

// OntologyDir is where the workspace keeps its schema and asserted graph.
func OntologyDir(root string) string { return OntologyDirIn(root, DefaultLayout()) }

// OntologyDirIn is OntologyDir under a layout's workspace dir.
func OntologyDirIn(root string, l Layout) string {
	return filepath.Join(root, l.norm().WorkspaceDir, "ontology")
}

// Load reads schema + graph/*.ttl, derives the workspace's members, and reasons
// to a fixpoint.
func Load(root string) (*Workspace, error) { return LoadLayout(root, DefaultLayout()) }

// LoadLayout is Load under a distribution's layout.
func LoadLayout(root string, l Layout) (*Workspace, error) {
	l = l.norm()
	w := &Workspace{Root: root, Graph: New(), Prefixes: DefaultPrefixes(), Docs: map[Term]*DocRef{}, Layout: l}
	dir := OntologyDirIn(root, l)
	schema := filepath.Join(dir, "schema.ttl")
	if b, err := os.ReadFile(schema); err == nil {
		if w.Prefixes, err = Parse(rel(root, schema), string(b), w.Graph, w.Prefixes); err != nil {
			return nil, err
		}
	} else {
		w.Notes = append(w.Notes, rel(root, schema)+" missing; using the built-in schema")
		if w.Prefixes, err = Parse("schema.ttl", DefaultSchema, w.Graph, w.Prefixes); err != nil {
			return nil, err
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "graph", "*.ttl"))
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if w.Prefixes, err = Parse(rel(root, f), string(b), w.Graph, w.Prefixes); err != nil {
			return nil, err
		}
	}
	if err := w.derive(); err != nil {
		return nil, err
	}
	Infer(w.Graph)
	return w, nil
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}

var (
	reField   = regexp.MustCompile(`(?i)^\s*[-*]?\s*(?:\*\*)?([A-Za-z][A-Za-z ]*?)(?::\*\*|\*\*:|:)\s*(.+?)\s*$`)
	reTick    = regexp.MustCompile("`([^`\n\\s]*/[^`\n\\s]*)`") // a path never holds a space; a command does
	rePath    = regexp.MustCompile(`[\w.\-*]+(?:/[\w.\-*]*)+`)
	reWord    = regexp.MustCompile(`[a-z0-9][a-z0-9-]*`)
	reRetired = regexp.MustCompile(`(?m)^[ \t]*[-*][ \t]*\*\*Retired:\*\*[ \t]*true\b`)
	fieldKeys = map[string]string{"reports to": "parent", "domain owner": "parent", "coordinator": "parent", "parent": "parent",
		"owns": "ownership", "ownership": "ownership", "zone": "ownership", "skills": "skills", "skill": "skills", "holds": "skills",
		"heir": "heir", "promoted from": "promoted"}
)

// fields reads `- **Key:** value` lines leniently; keys are normalized.
// A `## Ownership` (or `**Owns:**`) HEADING ZONE is also honored: any
// `- `code“ bullets under it are joined into a comma-list so lane filters
// run on one closet shape in both markups (class (f) fixture pins this
// bullet form that earlier regex never matched, registering no owns from it).
func fields(doc string) (map[string]string, map[string]int) {
	vals, lines := map[string]string{}, map[string]int{}
	var headingKey string // seat field being filled by `- `_`` bullets
	for i, line := range strings.Split(doc, "\n") {
		t := strings.TrimSpace(line)
		// end any open bullet-accumulation when a new heading begins
		if strings.HasPrefix(t, "#") {
			if !strings.HasPrefix(t, "## ") && !strings.HasPrefix(t, "##\t") {
				headingKey = "" // top-level headings reset the accumulator to silence
				continue
			}
			h := strings.ToLower(strings.TrimSpace(strings.TrimLeft(t, "# ")))
			h = strings.TrimSuffix(h, ":")
			if k, ok := fieldKeys[h]; ok {
				headingKey = k
			} else {
				headingKey = ""
			}
			continue
		}
		if headingKey != "" && strings.HasPrefix(t, "- ") {
			// bullets under the heading — keep just the backticked code spans,
			// falling back to whole-line only when no code marks the row (a
			// `# heading` may precede or follow them).
			agent := ""
			if idx := strings.Index(t, "`"); idx >= 0 {
				rest := t[idx+1:]
				if end := strings.Index(rest, "`"); end >= 0 {
					agent = rest[:end]
				}
			}
			if agent == "" {
				agent = strings.TrimSpace(strings.TrimPrefix(t, "- "))
			}
			if _, seen := vals[headingKey]; !seen {
				vals[headingKey] = agent
			} else if !strings.Contains(vals[headingKey], agent) {
				vals[headingKey] += ", " + agent
			}
			if _, seen := lines[headingKey]; !seen {
				lines[headingKey] = i + 1
			}
			continue
		}
		m := reField.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(m[1]))
		k, ok := fieldKeys[key]
		if !ok {
			continue
		}
		if _, seen := vals[k]; seen {
			continue
		}
		vals[k] = strings.TrimSpace(m[2])
		lines[k] = i + 1
	}
	return vals, lines
}

func normPath(p string) string {
	p = strings.TrimPrefix(p, "./")
	for _, suf := range []string{"/**/*", "/**", "/*", "**", "*"} {
		p = strings.TrimSuffix(p, suf)
	}
	return strings.ToLower(strings.TrimRight(p, "/"))
}

// derive adds the structural triples the workspace's files imply. Nothing here is
// ever written back to disk.
func (w *Workspace) derive() error {
	g := w.Graph
	l := w.Layout.norm()
	reName := regexp.MustCompile(`(?i)\b(?:orchestrator|` + l.namePattern() + `)`)
	// the first rank on a dir is the class its members take (an promoted rank shares its base's dir)
	rankOfDir := map[string]RankDir{}
	var dirs []string
	for _, r := range l.Ranks {
		if r.Dir == "" {
			continue
		}
		if _, dup := rankOfDir[r.Dir]; !dup {
			rankOfDir[r.Dir] = r
			dirs = append(dirs, r.Dir)
		}
	}
	rankByName := map[string]RankDir{}
	for _, r := range l.Ranks {
		rankByName[r.Name] = r
	}
	add := func(s, p, o Term) { g.AddDerived(Triple{s, p, o}) }
	for _, d := range dirs {
		c := ClassOf(rankOfDir[d].Name)
		if _, builtin := rankClass[rankOfDir[d].Name]; !builtin {
			add(c, rdfType, Ao("Class"))
			add(c, pSubClassOf, cMember)
		}
	}
	docNode := func(owner Term, path string) {
		d := Ao("doc-" + owner.Local())
		add(owner, pDoc, d)
		add(d, rdfType, cDoc)
		add(d, pPath, L(path))
	}
	add(tOrchestrator, rdfType, cOrchestrator)
	add(tOrchestrator, pName, L("orchestrator"))
	docNode(tOrchestrator, l.WorkspaceDir+"/"+l.Policy)

	// skills first, so docs can be matched against their names
	skills := map[string]string{}
	// the one skill registry (discover.SkillBases): crafted, then base — the dist names its workspace
	// dir — then the hosts'. A member's Skills field names them, so the graph must know them to
	// bond `holds`.
	bases := discover.SkillBases(l.WorkspaceDir)
	for _, base := range bases {
		ents, err := os.ReadDir(filepath.Join(w.Root, base))
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(w.Root, base, e.Name(), "SKILL.md")
			if _, err := os.Stat(p); err == nil {
				if _, dup := skills[e.Name()]; !dup {
					skills[e.Name()] = rel(w.Root, p)
				}
			}
		}
	}
	skillNames := make([]string, 0, len(skills))
	for n := range skills {
		skillNames = append(skillNames, n)
	}
	sort.Strings(skillNames)
	for _, n := range skillNames {
		m := Ao("skill-" + n)
		add(m, rdfType, cSkill)
		add(m, pName, L(n))
		docNode(m, skills[n])
		// a skill with no rank prefix is a shared host tool (the policy's shared-skills row), not a
		// member's skill: it is nobody's to hold, so SkillHeld does not apply to it
		if _, ranked := rankOfDir[strings.SplitN(n, "-", 2)[0]]; !ranked {
			add(m, pShared, Bool(true))
		}
	}

	type member struct {
		rank, dir, id, doc string
		text               string
	}
	var cs []member
	for _, rd := range dirs {
		rank := rankOfDir[rd].Name
		d := filepath.Join(w.Root, l.WorkspaceDir, rd)
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			c := member{rank: rank, dir: e.Name(), id: rd + "-" + strings.ToLower(e.Name())}
			files, _ := os.ReadDir(filepath.Join(d, e.Name()))
			var mds []string
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(f.Name(), ".md") {
					mds = append(mds, f.Name())
				}
			}
			sort.Strings(mds)
			for _, want := range []string{"README.md", e.Name() + ".md", "SKILL.md"} {
				for _, f := range mds {
					if f == want && c.doc == "" {
						c.doc = f
					}
				}
			}
			if c.doc == "" && len(mds) > 0 {
				c.doc = mds[0]
			}
			if c.doc != "" {
				p := filepath.Join(d, e.Name(), c.doc)
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				c.text = string(b)
				c.doc = rel(w.Root, p)
			}
			cs = append(cs, c)
		}
	}
	known := map[string]bool{"orchestrator": true}
	for _, c := range cs {
		known[c.id] = true
	}
	for _, c := range cs {
		self := Ao(c.id)
		add(self, rdfType, ClassOf(c.rank))
		add(self, pName, L(c.id))
		rdir := rankByName[c.rank].Dir
		if c.doc == "" {
			w.Notes = append(w.Notes, c.id+" has no doc in "+l.WorkspaceDir+"/"+rdir+"/"+c.dir+"/")
			w.Docs[self] = &DocRef{Path: l.WorkspaceDir + "/" + rdir + "/" + c.dir + "/", Lines: map[string]int{}}
			continue
		}
		docNode(self, c.doc)
		vals, lines := fields(c.text)
		ref := &DocRef{Path: c.doc, Lines: lines, Claims: map[string]Claim{}}
		w.Docs[self] = ref
		own := func(p, lane string, line int) {
			add(self, pOwns, L(p))
			if _, ok := ref.Claims[p]; !ok {
				ref.Claims[p] = Claim{Lane: lane, Line: line}
			}
		}

		// parent: the declared field, else the first member of the expected rank the doc names
		parent := ""
		if v, ok := vals["parent"]; ok {
			if m := reName.FindString(v); m != "" {
				parent = strings.ToLower(m)
			}
		}
		prank := rankByName[c.rank].Parent
		if parent == "" {
			wantRank := ""
			if pr, ok := rankByName[prank]; ok && pr.Dir != "" {
				wantRank = pr.Dir + "-"
			}
			for _, m := range reName.FindAllString(c.text, -1) {
				m = strings.ToLower(m)
				if wantRank != "" && strings.HasPrefix(m, wantRank) && m != c.id {
					parent = m
					break
				}
			}
			if parent == "" && prank == "orchestrator" {
				parent = "orchestrator"
			}
		}
		if parent != "" && parent != c.id {
			prace := "orchestrator"
			if parent != "orchestrator" {
				prace = rankOfDir[strings.SplitN(parent, "-", 2)[0]].Name
			}
			add(self, bondFor(c.rank, prace), Ao(parent))
			if !known[parent] {
				add(Ao(parent), pName, L(parent))
			}
		}

		// Member identity for the prose gate: the name the census trusts
		// (frontmatter, falling back to the seat id) and the zone dir base.
		// Both are what "self-naming" means for the boundary test below.
		name := normName(vals["name"])
		if name == "" {
			name = c.id
		}
		zone := strings.TrimSuffix(c.dir, "/") // seat dir — self-naming + blob claims
		// ownership: STRUCTURED field is authoritative; prose is supplement.
		seen := map[string]bool{}
		// The structured Ownership field is a claim list, consumed exactly
		// the way the write gate consumes it (workspace.Workspace.Owner,
		// workspace/workspace.go:418 chain): split on commas, trim spaces, strip
		// wrapping backticks, normPath, keep case.
		//
		// Ruling (2026-10-11, census class (f) derive heal — supersedes the
		// 2026-10-07 class-(e) zone-root-disjoint rule): the carve-out to
		// "every entry derives" — a structured entry whose resolved path is
		// NOT contained by the member's seat (`c.dir`, the directory under
		// `.agent-one/<rank>/` where its doc lives) is a cross-ref, never owns
		// (census schema registers no owns: onto/schema.ttl has no crossref
		// predicate → silent drop). Egress control is seat-scoped so disjoint
		// seats sharing a leading root token (`app-commands` → `app/`) do NOT
		// guard: `app/app.go` under seat `zone-app-commands` drops outright.
		// Containsment is root-first: the blob's root token must equal the
		// seat's leaf name OR live under it (`bench/tasks-tb2` under seat
		// `zone-bench-tasks`: root `bench` = `base(bench-tasks)` → derives).
		// zone-owna declares `.agent-one/zone/owna/...` under seat `zone-owna`:
		// root `.agent-one` ≠ `owna`, ∉ owna/ → drop. zone-auth declares
		// `src/auth/` under seat `zone-auth`: root `src` ≠ `auth`, ∉ auth/
		// → drop (verbatim shape guard, census class (e) pin — kept live per
		// TestDeriveOwns_StructuredBareFilename and the census-heal ruling).
		// Bare tokens (`local.go`, `app`) are member-local (workspace_test.go:
		// 387, TestDeriveOwns_StructuredBareFilename).
		// a retired seat (`- **Retired:** true` — relief, promotion, evolution) keeps its graph
		// memory — its name, its bonds, its skills — but claims no soil: its old Ownership line
		// and its prose register no owns, so the young that took its zone never overlap it
		retired := reRetired.MatchString(c.text)
		// succession (HeirOf): a retired seat is marked, and its Heir line names who took its
		// line; a promoted's `Promoted from` line makes it its base's heir
		if retired {
			add(self, pRetired, Bool(true))
			if m := strings.ToLower(reName.FindString(vals["heir"])); m != "" && m != c.id {
				add(self, pHeir, Ao(m))
			}
		}
		if m := strings.ToLower(reName.FindString(vals["promoted"])); m != "" && m != c.id {
			add(Ao(m), pHeir, self)
		}
		entries := []string{}
		for _, e := range strings.Split(vals["ownership"], ",") {
			if retired {
				break
			}
			e = strings.Trim(strings.TrimSpace(e), "`")
			if p := normPath(e); p != "" && !seen[p] {
				seen[p] = true
				entries = append(entries, p)
			}
		}
		slug := filepath.Base(zone)
		// class-(f) 2026-10-11: README ピコで stage unreliable: hyphen-span slug
		// head-vs-tail separation is the razor. headFrag keeps only the first
		// token; tailFrags keeps anything after first hyphen. Hoisted so both
		// the structured lane (below) and the diagnostic banner (Load)
		// inspect the same fragments.
		headFrag := slug
		if i := strings.Index(slug, "-"); i > 0 {
			headFrag = slug[:i]
		}
		tailFrags := ""
		if i := strings.Index(slug, "-"); i > 0 {
			tailFrags = slug[i+1:]
		}
		inZone := func(p string) bool {
			if i := strings.Index(p, "/"); i > 0 {
				slashFirst := p[:i]
				// hyphen-stem: the seat slug must split (e.g. bench-tasks); if
				// the slug is unhyphenated, it never admits hyphen-adjacent roots
				// like "app-commands". Then the leading path root must equal
				// headFrag, and the tailFrags must appear as a later segment in
				// the path (`bench/tasks-tb2` under seat zone-bench-tasks;
				// `app/app.go` under seat zone-app-commands has no "commands"
				// segment → drops).
				if tailFrags != "" && slashFirst == headFrag &&
					strings.Contains(strings.TrimPrefix(p, slashFirst)+"/", tailFrags) {
					return true
				}
				// class-(f) fixture pin (workspace_test.go:564-575): bare `.agent-one`
				// stays mintable by EVERY member — dedupe is Validate's job.
				// class-(e): rank-policy claims the policy path; shared `src/` trunk
				// is shared by every member and never binds one seat.
				if slashFirst == ".agent-one" || slashFirst == "src" {
					return true
				}
				// own-name arm: name-eponymous seat traces its own root
				// (zone-onto → `onto/…`, zone-owna → `owna/…`).
				// For a hyphenated slug (zone-app-commands): after hyphen-stem
				// failed (no later segment matching the tail), root == headFrag
				// is the foreign-shared lookalike — silent drop so
				// `app/app.go` under seat zone-app-commands does NOT register.
				if tailFrags == "" && slashFirst == headFrag {
					return true
				}
				// foreign disjoint — silent drop (onto/schema.ttl: no crossref).
				return false
			}
			if p == ".agent-one" {
				return true
			}
			// bare filename, member-local (workspace_test.go:387)
			return true
		}
		for _, p := range entries {
			if !inZone(p) {
				continue // zone-foreign structured claim: crossref, never owns
			}
			own(p, "structured", lines["ownership"])
		}
		// Prose supplement: backticked path spans in the doc agent — but only
		// spans that SELF-NAME the member (its name or zone dir adjoining
		// the span) derive ao:owns. Structured Ownership stays the one
		// authoritative claim list; a backticked cross-reference to foreign
		// soil (`workspace/workspace.go` in another member's doc) is a path, not
		// a claim. Slashes hold the filesystem — plain words never derive.
		for _, m := range reTick.FindAllStringSubmatchIndex(c.text, -1) {
			if retired {
				break
			}
			span := c.text[m[2]:m[3]]
			if !selfNamesSpan(c.text, m[2], m[3], name, zone) {
				continue
			}
			p := normPath(span)
			// prose never registers a whole tree: a span that normalizes to one bare segment (`src/`,
			// `docs/`) or carries a glob (`**/`, `*/*`) names a region, never this member's
			// claim — the structured Ownership field is the only lane for a top-level tree
			if !strings.Contains(p, "/") || strings.Contains(p, "*") {
				continue
			}
			// census class (f) 2026-10-11: prose claims must cite the member's
			// seat-root; those that do not are foreign territorials registered by
			// selfNamesSpan whole-word adjacency from the SAME comment naming the
			// member's doc (class (f) commission: "zone-onto's Ownership claims
			// bench/tasks-tb2 as own ... prose mention registered under TWO seats
			// simultaneously"). Real fixture: auditor-eye registers its `.agent-one/log.md`
			// EXACTLY BECAUSE Fixture auth also claims it — but the structured
			// Ownership field is the only legitimate channel for that; markdown
			// a member reads (or humans annotate into its doc) is never itself
			// a claim the ORCHESTRATOR wrote.
			if i := strings.Index(p, "/"); i > 0 {
				switch p[:i] {
				case ".agent-one":
					// Workspace-shared prefix — the SELF-NAME lane already requires
					// adjacency to the member's name.
				case zone:
					// Seat-rooted claim, e.g. bench/tasks-tb2 under zone-bench-tasks.
				default:
					// class-(f) sharpen: prose registers only what the ORCHESTRATOR wrote.
					// smoke's Ownership authoritatively lists `.agent-one`; prose under
					// seat smoke naming `bench/tasks-tb2` (root=bench) is a third-party
					// commentary, never a claim — the prose lane admits only roots
					// that are the seat's own slug or a hyphen-stem of it.
					continue
				}
			}
			// The seat-root prefixes are historical artifacts (dead `.opencode/skills`,
			// moved `.agent-one/skills`) that agents mention in Notes when recording
			// their own corrections — they are never a claim of ownership. Drop them,
			// root or anything nested under them.
			if p == ".opencode/skills" || strings.HasPrefix(p, ".opencode/skills/") ||
				p == ".agent-one/skills" || strings.HasPrefix(p, ".agent-one/skills/") {
				continue
			}
			// census-heal 2026-10-11: a prose line that names the member may
			// still register owns the workspace grammar reserves as shared soil — bare
			// `.agent-one`, anything under `.agent-one/tmp/` — or may register URI
			// templates (query placeholders like `api/plan?agent=<id>`) as if
			// they were filesystem paths. Neither is ownable; the structured
			// Ownership field is the only legitimate channel for those claims.
			if p == ".agent-one" || p == ".agent-one/" {
				continue
			}
			if p == ".agent-one/tmp" || strings.HasPrefix(p, ".agent-one/tmp/") {
				continue
			}
			if strings.ContainsAny(p, "?<>") {
				continue
			}
			// class (d) 2026-10-07: a prose-narrated span with a leading
			// slash and NO dot-extension (`/api/tree`, `/api/tree/children`)
			// is a REST route, never filesystem soil — normPath kept the
			// leading slash (normalising it merely because it holds a
			// slash), so the route arrives here intact. Mirror of the
			// committed census-heal refusal above: structured Ownership
			// already owns the claim channel, so the prose-register lane
			// refuses the route. Dot-bearing code paths survive.
			if strings.HasPrefix(p, "/") && !strings.Contains(p, ".") {
				continue
			}
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			own(p, "prose", 1+strings.Count(c.text[:m[2]], "\n"))
		}

		// held skills: named in a Skills field, or any skill's name in the doc as a whole word
		lower := strings.ToLower(c.text)
		for _, n := range skillNames {
			if wordIn(lower, strings.ToLower(n)) {
				add(self, pHolds, Ao("skill-"+n))
			}
		}
		if v, ok := vals["skills"]; ok {
			for _, n := range reWord.FindAllString(strings.ToLower(v), -1) {
				if _, exists := skills[n]; exists {
					add(self, pHolds, Ao("skill-"+n))
				}
			}
		}
	}
	return nil
}

func wordIn(text, word string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], word)
		if j < 0 {
			return false
		}
		s, e := i+j, i+j+len(word)
		before := s == 0 || !isWordByte(text[s-1])
		after := e == len(text) || !isWordByte(text[e])
		if before && after {
			return true
		}
		i = s + 1
	}
}

func isWordByte(b byte) bool {
	return b == '-' || b == '_' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b >= 0x80
}

// normName normalizes a frontmatter name into the census-stable lower form:
// lower-cased, trimmed, internal whitespace collapsed. May return "".
func normName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// selfNamesSpan reports whether the backticked span doc[start:end] self-names
// the member — the prose gate for the census owns-supplement. The member
// itself is its zone dir base and its frontmatter name stripped of the rank
// prefix (a rank holds no claim). A span self-names when the member heads
// the path (`owna/…`) or bases the file (`leaf/owna.go`), or when the name
// adjoins the span as a whole word. A bare span fails by reTick's slash
// before ever reaching here; the equals-guards are the belt to its braces.
// `workspace/workspace.go` is a foreign cross-ref (head workspace ≠ owna) and `owna` a
// zone word, not a file — neither derives; OWNERSHIP.md and child owna pass.
func selfNamesSpan(doc string, start, end int, name, zone string) bool {
	span := doc[start:end]
	self := zone
	if rest, ok := strings.CutPrefix(name, "-"+zone); zone != "" && ok && rest != "" {
		self = rest // frontmatter name stripped of its rank prefix (zone-owna → owna)
	}
	if span == zone || (self != "" && span == self) {
		return false // bare zone/name word, no file shape
	}
	if self != "" && span == self+"/" {
		return false // zone prefix alone is a name, not a path — the trailing
		// slash holds no file, and normPath would collapse it to the bare zone.
	}
	if self != "" && (strings.HasPrefix(span, self+"/") ||
		strings.HasSuffix(span, "/"+self) || baseOf(span) == self) {
		return true // the path heads or bases on the member itself
	}
	if dot := strings.LastIndexByte(span, '.'); dot > 0 && dot < len(span)-1 {
		return false // a foreign file path (workspace/workspace.go) is a cross-ref, not a claim
	}
	if name == "" {
		return false
	}
	// A name-like span (OWNERSHIP.md, dir/owna, owna/child) self-names when the
	// member's whole name adjoins it as a word, backticks ignored.
	lo, hi := start, end
	if lo > 0 && doc[lo-1] == '`' {
		lo--
	}
	if hi < len(doc) && doc[hi] == '`' {
		hi++
	}
	return (lo > 0 && wordIn(doc[:lo], name)) || (hi < len(doc) && wordIn(doc[hi:], name))
}

func baseOf(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// HeirOf names the live member that holds a retired seat's line: its heir edges (the seat's
// Heir line, a promoted naming it as base) walked through any heir that retired in turn,
// nearest first. "" when name is not a retired seat, or its line ends with no live heir.
func (w *Workspace) HeirOf(name string) string {
	start := Ao(strings.ToLower(strings.TrimSpace(name)))
	if !w.Graph.Has(Triple{start, pRetired, Bool(true)}) {
		return ""
	}
	seen := map[Term]bool{start: true}
	for queue := []Term{start}; len(queue) > 0; queue = queue[1:] {
		heirs := w.Graph.Objects(queue[0], pHeir)
		sort.Slice(heirs, func(i, j int) bool { return heirs[i].Value < heirs[j].Value })
		for _, h := range heirs {
			if seen[h] {
				continue
			}
			seen[h] = true
			if !w.Graph.Has(Triple{h, pRetired, Bool(true)}) {
				if w.Graph.Has(Triple{h, rdfType, cMember}) {
					return h.Local()
				}
				continue // a name with no seat of its own is no heir
			}
			queue = append(queue, h)
		}
	}
	return ""
}

// Member resolves a name like zone-auth (or auth, when unambiguous) to its term.
func (w *Workspace) Member(name string) (Term, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "orchestrator" {
		return tOrchestrator, nil
	}
	if w.Graph.Has(Triple{Ao(name), rdfType, cMember}) {
		return Ao(name), nil
	}
	var hits []Term
	for _, c := range w.Graph.Instances(cMember) {
		if strings.HasSuffix(c.Local(), "-"+name) {
			hits = append(hits, c)
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	if len(hits) > 1 {
		return Term{}, fmt.Errorf("ambiguous member %q: %s", name, joinLocal(hits))
	}
	return Term{}, fmt.Errorf("unknown member %q", name)
}

func joinLocal(ts []Term) string {
	s := make([]string, len(ts))
	for i, t := range ts {
		s[i] = t.Local()
	}
	return strings.Join(s, ", ")
}
