package workspace

import (
	"fmt"
	"os"
	"path/filepath"
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

// reliefResult is what the warrant says happened to the member.
type reliefResult struct {
	Act      string   // split | retire
	Children []string // split young names
	SpareDoc string   // retiree's marker line in its own doc
}

// relieve acts on agent when its strain crossed the act threshold. One fire per agent per
// streak (the ledger stamps the act; a later landing wakes a fresh ledger clean).
func (w *Workspace) relieve(agent string) (string, bool) {
	c := w.Member(agent)
	if c == nil {
		return "", false
	}
	msg := ""
	terr := cleanPrefixes(c.Ownership)
	now := time.Now().UTC().Format("2006-01-02")
	// split wins: zone breadth provisioned to cut, not to kill
	if len(terr) >= 2 {
		mid := (len(terr) + 1) / 2
		young, err := w.splitZone(c, terr[:mid], terr[mid:], now)
		if err != nil {
			msg = "(split failed: " + err.Error() + ") — falling back to retire"
		} else {
			w.mu.Lock()
			w.ledgerNote(ledgerRow{Kind: "relief", Agent: agent, Lane: "split", Path: strings.Join(young, ",")})
			w.mu.Unlock()
			w.Reload()
			return fmt.Sprintf("split into %s + %s (zone divided at the count's middle)", young[0], young[1]), true
		}
	}
	// single-prefix split by tree (the 2026-10-09 zone-sizing policy): a zone holding one wide
	// dir narrows by what actually lives under it — sub-dirs and direct files halved into
	// two same-race young. A one-prefix zone with nothing under it to divide still retires.
	if len(terr) == 1 {
		if a, b, ok := w.splitByTree(terr[0]); ok {
			if young, err := w.splitZone(c, a, b, now); err == nil {
				w.mu.Lock()
				w.ledgerNote(ledgerRow{Kind: "relief", Agent: agent, Lane: "split-tree", Path: strings.Join(young, ",")})
				w.mu.Unlock()
				w.Reload()
				return fmt.Sprintf("split into %s + %s (one wide zone divided by the tree under %s)", young[0], young[1], terr[0]), true
			}
		}
	}
	// retire: narrow zone or split failed — the seat stays in the graph with its memory,
	// claims no soil; the workspace's instruments record the cut
	if err := w.retireDoc(c, now); err != nil {
		return "(retire failed: " + err.Error() + ")", false
	}
	w.mu.Lock()
	w.ledgerNote(ledgerRow{Kind: "relief", Agent: agent, Lane: "retire", Path: c.Doc})
	w.mu.Unlock()
	w.Reload()
	return "retired " + agent + " " + strings.TrimSpace(msg) + "; zone open for future dispatches", true
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

// retireDoc lands the marker; a second call is a no-op (idempotent policy).
func (w *Workspace) retireDoc(c *Member, date string) error {
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
	s += retiredMark + " — council relief " + date + ": the zone relents (its strain kept landing)\n"
	return os.WriteFile(p, []byte(s), 0o644)
}

// splitZone births two same-race young members at <dir>-a / <dir>-b, each with its half
// of the parent's prefixes + the parent's doc content so memory/traditions ride along;
// the parent retires (its doc marker lands LAST, only once both young stand, so a failed
// provisioning never orphans the zone).
func (w *Workspace) splitZone(c *Member, a, b []string, date string) ([]string, error) {
	src := filepath.Join(w.Root, filepath.FromSlash(c.Doc))
	agent, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	text := string(agent)
	names := []string{c.Name + "-a", c.Name + "-b"}
	halves := [][]string{a, b}
	for i, name := range names {
		dir := filepath.Join(w.Root, filepath.FromSlash(c.Dir+"-"+string(rune('a'+i))))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		doc := rebuildChildDoc(text, name, halves[i], names[(i+1)%2], date)
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(doc), 0o644); err != nil {
			return nil, err
		}
	}
	if err := w.retireDoc(c, date); err != nil {
		return nil, err
	}
	return names, nil
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
