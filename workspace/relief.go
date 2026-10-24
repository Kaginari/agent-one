package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The council's act (policy.stress.relieveAt, default 7): an agent whose strain crosses the act
// threshold needs the zone to change, not another warrant. This is relieve's knife:
//
// split  — a ≥2-prefix zone divides into two same-race young ones (deterministic halves,
//          each inheriting the parent's doc, rank, verify and reports; the parent retires);
// retire — a narrower zone's member marks its own doc retired: the seat keeps its graph
//          memory but claims no soil; the zone's files become open ground;
// configuration — the relief never silently mutates a lane: a configuration-shift proposal is drafted
//          to the slab for the human to sign (Operator's 2026-10-02 policy stands).
//
// Ontology-safe by construction: retirement and provisioning live WHERE the roster reads (docs +
// one marker line), never in a side file — the next Reload re-derives truth, and derived
// ownerships of the retiree vanish from Owner() the moment the roster re-reads.

// relieve acts on agent when its strain crossed the act threshold. One fire per agent per
// streak (the ledger stamps the act; a later landing wakes a fresh ledger clean). wrote is every
// doc the act wrote — the young, the retiree's marker, its children re-seated — the harness's
// writes, which the dispatcher's end gate leaves alone.
//
// The cascade: a retirement walks the retiree's children. Each one's Reports-to line is
// re-seated onto the heir — the young whose half covers its claims (a split), or the retiree's
// own one hop up (a retirement) — and the retiree's marker names its heir (`- **Heir:**`), so a
// child the cascade could not reach still resolves through it (onto.HeirOf).
func (w *Workspace) relieve(agent string) (msg string, wrote []string, ok bool) {
	c := w.Member(agent)
	if c == nil {
		return "", nil, false
	}
	kids := w.childrenOf(c.Name)
	terr := cleanPrefixes(c.Ownership)
	now := time.Now().UTC().Format("2006-01-02")
	note := ""
	// split wins: zone breadth provisioned to cut, not to kill
	halves := [][]string{}
	lane := ""
	if len(terr) >= 2 {
		mid := (len(terr) + 1) / 2
		halves, lane = [][]string{terr[:mid], terr[mid:]}, "split"
	} else if len(terr) == 1 {
		// single-prefix split by tree (the 2026-10-09 zone-sizing policy): a zone holding one wide
		// dir narrows by what actually lives under it — sub-dirs and direct files halved into
		// two same-race young. A one-prefix zone with nothing under it to divide still retires.
		if a, b, ok := w.splitByTree(terr[0]); ok {
			halves, lane = [][]string{a, b}, "split-tree"
		}
	}
	if lane != "" {
		young, docs, err := w.splitZone(c, halves[0], halves[1], now)
		if err == nil {
			wrote = append(docs, w.cascadeReSeat(c.Name, kids, func(k Member) string { return heirByClaims(k, young, halves) }, now)...)
			w.mu.Lock()
			w.ledgerNote(ledgerRow{Kind: "relief", Agent: agent, Lane: lane, Path: strings.Join(young, ",")})
			w.mu.Unlock()
			w.Reload()
			how := "zone divided at the count's middle"
			if lane == "split-tree" {
				how = "one wide zone divided by the tree under " + terr[0]
			}
			return fmt.Sprintf("split into %s + %s (%s)%s", young[0], young[1], how, reSeated(kids)), wrote, true
		}
		if lane == "split" {
			note = "(split failed: " + err.Error() + ") — falling back to retire"
		}
	}
	// retire: narrow zone or split failed — the seat stays in the graph with its memory,
	// claims no soil, and its children report to its own one hop up
	heir := c.Parent
	if err := w.retireSeat(c, "council relief "+now+": the zone relents (its strain kept landing)", heir); err != nil {
		return "(retire failed: " + err.Error() + ")", nil, false
	}
	wrote = append([]string{c.Doc}, w.cascadeReSeat(c.Name, kids, func(Member) string { return heir }, now)...)
	w.mu.Lock()
	w.ledgerNote(ledgerRow{Kind: "relief", Agent: agent, Lane: "retire", Path: c.Doc})
	w.mu.Unlock()
	w.Reload()
	return "retired " + agent + " " + strings.TrimSpace(note) + "; zone open for future dispatches" + reSeated(kids), wrote, true
}

// childrenOf is the living members whose one hop up is name, as the roster reads them now.
func (w *Workspace) childrenOf(name string) []Member {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []Member
	for _, c := range w.Members {
		if c.Parent == name && c.Name != name {
			out = append(out, c)
		}
	}
	return out
}

// reSeated renders the cascade for the warrant line.
func reSeated(kids []Member) string {
	if len(kids) == 0 {
		return ""
	}
	names := make([]string, len(kids))
	for i, k := range kids {
		names[i] = k.Name
	}
	return "; re-seated " + strings.Join(names, ", ")
}

// heirByClaims picks the young whose half holds the most of a child's claims (a claim under a
// half's prefix, or a half's prefix under the claim); the first young on a tie or no overlap.
func heirByClaims(k Member, young []string, halves [][]string) string {
	best, bestN := 0, 0
	for i, half := range halves {
		n := 0
		for _, t := range k.Ownership {
			for _, p := range half {
				if under(normRel(t), p) || under(normRel(p), t) {
					n++
				}
			}
		}
		if n > bestN {
			best, bestN = i, n
		}
	}
	return young[best]
}

// reParentLine is a doc's declared one-hop-up line, the field onto's derivation reads first.
var reParentLine = regexp.MustCompile(`(?mi)^([ \t]*[-*][ \t]*\*\*(?:reports to|parent|domain owner|coordinator):?\*\*:?[ \t]*).*$`)

// cascadeReSeat rewrites each child's Reports-to line onto its heir (a doc naming none gains
// one) and returns the docs it wrote. A child whose doc cannot be read or written keeps its
// line; it still resolves through the retiree's Heir mark.
func (w *Workspace) cascadeReSeat(retiree string, kids []Member, heirOf func(Member) string, date string) []string {
	var wrote []string
	for _, k := range kids {
		heir := heirOf(k)
		if k.Doc == "" || heir == "" {
			continue
		}
		p := filepath.Join(w.Root, filepath.FromSlash(k.Doc))
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		line := heir + " — re-seated " + date + ": " + retiree + " retired"
		doc := string(b)
		if loc := reParentLine.FindStringSubmatchIndex(doc); loc != nil {
			doc = doc[:loc[3]] + line + doc[loc[1]:]
		} else {
			if !strings.HasSuffix(doc, "\n") {
				doc += "\n"
			}
			doc += "- **Reports to:** " + line + "\n"
		}
		if os.WriteFile(p, []byte(doc), 0o644) == nil {
			wrote = append(wrote, k.Doc)
		}
	}
	return wrote
}

// cleanPrefixes folds ownership entries: backticks/quotes carried, only real path prefixes.
func cleanPrefixes(in []string) []string {
	var out []string
	for _, p := range in {
		p = strings.Trim(p, " `\"'")
		if p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// the marker line a retired seat carries in its own doc: the roster parser's skip-key
const retiredMark = "- **Retired:** true"

// retireDocWhy lands the marker with its reason (promotion: the promoted's `Promoted from` line
// names the heir); idempotent.
func (w *Workspace) retireDocWhy(c *Member, why string) error { return w.retireSeat(c, why, "") }

// retireSeat lands the marker with its reason and, when the line has one, the heir that holds
// it now (`- **Heir:** <name>`, read by onto.HeirOf); idempotent.
func (w *Workspace) retireSeat(c *Member, why, heir string) error {
	if c.Doc == "" {
		return fmt.Errorf("member %s carries no doc — it never parsed a README", c.Name)
	}
	p := filepath.Join(w.Root, filepath.FromSlash(c.Doc))
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	s := string(b)
	if strings.Contains(s, retiredMark) {
		return nil
	}
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	s += retiredMark + " — " + why + "\n"
	if heir != "" && heir != c.Name {
		s += "- **Heir:** " + heir + "\n"
	}
	return os.WriteFile(p, []byte(s), 0o644)
}

// splitZone births two same-race young members at <dir>-a / <dir>-b, each with its half
// of the parent's prefixes + the parent's doc content so memory/traditions ride along;
// the parent retires naming the first young its heir (its doc marker lands LAST, only once both
// young stand, so a failed provisioning never orphans the zone). docs is every doc it wrote.
func (w *Workspace) splitZone(c *Member, a, b []string, date string) (names, docs []string, err error) {
	src := filepath.Join(w.Root, filepath.FromSlash(c.Doc))
	agent, err := os.ReadFile(src)
	if err != nil {
		return nil, nil, err
	}
	text := string(agent)
	names = []string{c.Name + "-a", c.Name + "-b"}
	halves := [][]string{a, b}
	for i, name := range names {
		rel := c.Dir + "-" + string(rune('a'+i))
		dir := filepath.Join(w.Root, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, err
		}
		doc := rebuildChildDoc(text, name, halves[i], names[(i+1)%2], date)
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(doc), 0o644); err != nil {
			return nil, nil, err
		}
		docs = append(docs, rel+"/README.md")
	}
	if err := w.retireSeat(c, "council relief "+date+": the zone relents (its strain kept landing)", names[0]); err != nil {
		return nil, nil, err
	}
	return names, append(docs, c.Doc), nil
}

// splitByTree halves a single-prefix zone by what lives under it: sub-directories and
// direct files, each entry becoming a young one's claim — nothing under the prefix is
// orphaned by the provisioning. ok is false when the tree offers nothing to divide (an empty or
// single-entry zone retires instead).
func (w *Workspace) splitByTree(prefix string) (a, b []string, ok bool) {
	trim := strings.TrimSuffix(prefix, "/")
	ents, err := os.ReadDir(filepath.Join(w.Root, filepath.FromSlash(trim)))
	if err != nil {
		return nil, nil, false
	}
	var paths []string
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") || skipDirRelief[name] {
			continue
		}
		rel := trim + "/" + name
		if e.IsDir() {
			rel += "/"
		}
		paths = append(paths, rel)
	}
	if len(paths) < 2 {
		return nil, nil, false
	}
	sort.Strings(paths)
	mid := (len(paths) + 1) / 2
	return paths[:mid], paths[mid:], true
}

// skipDirRelief mirrors the founding sketch's skip set: junk dirs never carry a claim.
var skipDirRelief = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, "target": true, "out": true, "bin": true, "testdata": true, "coverage": true, "__pycache__": true, ".venv": true, "venv": true}

// rebuildChildDoc drafts the young one's doc: the parent's text carries over (memory,
// verify, notes) with the provisioning inside-bracketed in the Ownership slug (halves) and a
// provisioning line — so the next Reload parses it whole, ownership lawfully narrower.
func rebuildChildDoc(parent, name string, ownership []string, sib string, date string) string {
	doc := parent
	// one h1 rename: the youngest tells its own name
	if i := strings.Index(doc, "\n"); i > 0 {
		doc = "# " + name + doc[i:]
	}
	terrLine := "- **Owns:** "
	for i, p := range ownership {
		terrLine += "`" + p + "`"
		if i < len(ownership)-1 {
			terrLine += ", "
		}
	}
	doc += "\n- **Provisioning:** " + date + " — relieved young (zone split), twin " + sib + "; see the parent's doc for the strain lineage\n"
	// its ownership replaces any carried parent Ownership line: the structured claim is the
	// one start-wide-span and POLICY gives the young half the prefixes only (never the breadth again)
	if from := strings.Index(doc, "- **Owns:**"); from >= 0 {
		end := strings.Index(doc[from:], "\n")
		if end < 0 {
			end = len(doc) - from
		} else {
			end += from
		}
		doc = doc[:from] + terrLine + doc[end:]
	} else {
		doc += terrLine + "\n"
	}
	return doc
}

// docRetired is the roster's skip-key check (membersOf skips retired seats; the graph still
// remembers them).
func docRetired(text string) bool {
	for _, ln := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), retiredMark) {
			return true
		}
	}
	return false
}
