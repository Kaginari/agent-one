package memory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// memory scrub — the mid-flight cure (Operator order 2026-10-03: "make it possible to clean
// memory via cmd"). How memories pollute: an agent speaks words that were never lawful
// in-workspace (tonight: the room's outer-report markers "**Maintenance:** ⚙️" /
// "**Evolution:** 🚀", banned by the speech policy note), the speech lands in the log, the
// scheme's notes and the short recall cache, and every later recall re-quotes the stain —
// the habit feeds itself. Pick the words, run `agent-one memory clean` dry, then --apply:
// matched spans normalize in place, empty hulls drop (cache entries are recall caches, not
// records), every applied write leaves a byte-exact backup under .agent-one/tmp/scrub/, and the
// long index reseeds from cleaned sources — the next recall quotes clean text.

// scrubRule is one cure pattern with the span that replaces what it finds, and the configuration
// act its recurrence calls for (Principle 3 + 4): the first naming is THIS clean landing on the
// record; any later hit is the second naming — /provisioning a member to own it or craft a
// skill under .agent-one/crafted-skills/ so every agent carries the cure.
type scrubRule struct {
	name   string         // what it cures, for the report
	re     *regexp.Regexp // what it finds
	repl   string         // what stands after
	evolve string         // the configuration act when this family recurs
}

// ScrubRules is the cure deck. Markers normalise to their plain classification word (the
// sentence keeps meaning; the bold + emoji squib that breaks widths and smuggles the habit
// dies); any remaining U+FE0F (emoji presentation selector) goes outright.
func ScrubRules() []scrubRule {
	return []scrubRule{
		{"maintenance-marker", regexp.MustCompile(`\*\*Maintenance:\*\*[ \t]*⚙️?[ \t]*`), "Maintenance: ",
			"speech policy note of 2026-10-03 is the cure of record; a recurrence = the second naming — decide: /provisioning a speech persistent agent, or craft .agent-one/crafted-skills/speech-register so the cure rides every agent"},
		{"evolution-marker", regexp.MustCompile(`\*\*Evolution:\*\*[ \t]*🚀[ \t]*`), "Evolution: ",
			"same family as the Maintenance marker — the speech persistent agent / speech-register decision covers both"},
		{"vs16", regexp.MustCompile("️"), "",
			"TUI sanitises at render since 2026-10-03; memory hits are echo of pre-policy speech — a recurrence in NEW speech names the same need twice"},
	}
}

// ScrubFile is one file's scrub ledger: how many rule hits it carried, how many whole cache
// entries dropped (the rule emptied them), and where its backup rests after --apply.
type ScrubFile struct {
	Path    string `json:"path"`
	Hits    int    `json:"hits"`
	Dropped int    `json:"dropped"`
	Backup  string `json:"backup,omitempty"`
}

// ScrubFamily is one rule's roll-up across all files: how many hits, and the configuration act its
// recurrence calls for — the memory review's route to provisioning (a member provisioned to own the
// need) or to a crafted skill (the cure every agent carries).
type ScrubFamily struct {
	Name   string `json:"name"`
	Hits   int    `json:"hits"`
	Files  int    `json:"files"`
	Evolve string `json:"evolve"`
}

// ScrubResult is the clean command's report: dry-run measures, --apply writes + reseeds.
type ScrubResult struct {
	Applied  bool          `json:"applied"`
	Files    []ScrubFile   `json:"files"`
	Families []ScrubFamily `json:"families"`
	Hits     int           `json:"hits"`
	Dropped  int           `json:"dropped"`
	Indexed  int           `json:"indexed,omitempty"` // memories reseeded after apply; 0 when nothing or dry
}

// scrubTargets lists the memory surfaces rules run against: the workspace log (index source),
// shared + machine notes (records — normalised spans, never dropped), and the short recall
// caches (cache lines may drop when a rule empties them).
func (w *Workspace) scrubTargets() []string {
	var out []string
	if p := filepath.Join(w.Base(), "log.md"); Exists(p) {
		out = append(out, p)
	}
	for _, p := range []string{w.sharedNotes(), w.machineNotes()} {
		if Exists(p) {
			out = append(out, p)
		}
	}
	if cs, err := filepath.Glob(filepath.Join(w.shortDir(), "*.jsonl")); err == nil {
		out = append(out, cs...)
	}
	return out
}

// scrubLine applies every rule in deck order, returning the cleaned text and per-rule hit
// counts. Sequential, not raw-double-counted: once a rule consumed ⚙️, the vs16 rule sees
// nothing there — the ledger and the written text always agree.
func scrubLine(rules []scrubRule, line string) (string, map[string]int) {
	hits := map[string]int{}
	for _, r := range rules {
		found := r.re.FindAllString(line, -1)
		if len(found) > 0 {
			hits[r.name] = len(found)
			line = r.re.ReplaceAllString(line, r.repl)
		}
	}
	return line, hits
}

// droppedChart: a cache line drops when its query survives with zero content tokens — a
// question reduced to pure squib keeps nothing worth caching. Notes and the log are records
// (Policy 4): spans normalise in place, lines never drop there.
func scrubJSONLLine(rules []scrubRule, line string) (out string, hits map[string]int, dropped bool) {
	clean, n := scrubLine(rules, line)
	if len(n) == 0 {
		return line, n, false
	}
	var probe struct {
		Q *string `json:"q"`
	}
	if err := json.Unmarshal([]byte(clean), &probe); err == nil && probe.Q != nil && len(Tokens(*probe.Q)) == 0 {
		return "", n, true
	}
	return clean, n, false
}

// Scrub walks the memory surfaces with the cure deck. Dry-run (apply=false) touches nothing
// and reports what --apply would do. Applied writes go through backup-then-atomic, and the
// long index reseeds from the cleaned sources so the next recall quotes clean text.
//
// The walk also keeps the review roll-up (Principle 3 + 4): per cure family, hits and stained
// surfaces, and the configuration act a recurrence demands. THIS clean landing on the record is the
// first naming; a later hit of the same family is the second — then the decision is spoken
// out loud: /provisioning a member to own the need, or craft a skill that carries the cure to
// every agent (.agent-one/crafted-skills/). Scrub reports, never decides — births wait on the
// human's word (Egress control, Policy 6).
func (w *Workspace) Scrub(apply bool) (*ScrubResult, error) {
	rules := ScrubRules()
	res := &ScrubResult{Applied: apply}
	byRule := map[string]*ScrubFamily{}
	for _, r := range rules {
		byRule[r.name] = &ScrubFamily{Name: r.name, Evolve: r.evolve}
	}
	fileTouched := map[string]map[string]bool{} // rule -> set of stained files (for family Files count)
	for _, p := range w.scrubTargets() {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, failf("scrub reads %s: %v", p, err)
		}
		lines := strings.Split(string(raw), "\n")
		isJSONL := strings.HasSuffix(p, ".jsonl")
		var kept []string
		f := ScrubFile{Path: w.Rel(p)}
		for _, line := range lines {
			var out string
			var hits map[string]int
			var dropped bool
			if isJSONL && strings.TrimSpace(line) != "" {
				out, hits, dropped = scrubJSONLLine(rules, line)
			} else {
				out, hits = scrubLine(rules, line)
			}
			n := 0
			for rule, c := range hits {
				n += c
				byRule[rule].Hits += c
				if fileTouched[rule] == nil {
					fileTouched[rule] = map[string]bool{}
				}
				fileTouched[rule][f.Path] = true
			}
			f.Hits += n
			if dropped {
				f.Dropped++
				continue
			}
			kept = append(kept, out)
		}
		if f.Hits == 0 {
			continue
		}
		res.Hits += f.Hits
		res.Dropped += f.Dropped
		if apply {
			stamp := strings.NewReplacer(":", "", ".", "", "Z", "", "T", "-").Replace(w.now())
			backup := filepath.Join(w.Base(), "tmp", "scrub", stamp+"-"+strings.TrimPrefix(f.Path, w.dir()+"/"))
			if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
				return nil, failf("scrub backup dir: %v", err)
			}
			if err := os.WriteFile(backup, raw, 0o644); err != nil {
				return nil, failf("scrub backup %s: %v", backup, err)
			}
			if err := WriteAtomic(p, []byte(strings.Join(kept, "\n"))); err != nil {
				return nil, failf("scrub write %s: %v", p, err)
			}
			f.Backup = w.tilde(backup)
		}
		res.Files = append(res.Files, f)
	}
	if apply && (res.Hits > 0 || res.Dropped > 0) {
		idx, err := w.BuildIndex()
		if err != nil {
			return nil, failf("scrub landed but the reseed crashed: %v — rerun `memory index`", err)
		}
		res.Indexed = idx.N
	}
	for name, files := range fileTouched {
		byRule[name].Files = len(files)
	}
	var names []string
	for name, fam := range byRule {
		if fam.Hits > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		res.Families = append(res.Families, *byRule[name])
	}
	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	return res, nil
}

// printScrub renders the clean command's lines on the wire register: the cure ledger, then
// the review roll-up routing every struck family to its configuration act (per Principle 3 + 4).
func printScrub(out_ io.Writer, r *ScrubResult) {
	if !r.Applied {
		if r.Hits == 0 {
			fmt.Fprintln(out_, "@S CLEAN — no scrub hits anywhere")
			return
		}
		fmt.Fprintf(out_, "@? scrub dry-run: %d hits across %d files (%d cache entr%s would drop) — `agent-one memory clean --apply` writes (backups under .agent-one/tmp/scrub/)\n", r.Hits, len(r.Files), r.Dropped, pluralS(r.Dropped))
	} else if r.Hits == 0 {
		fmt.Fprintln(out_, "@S CLEAN — nothing needed")
	} else {
		fmt.Fprintf(out_, "@S SCRUBBED %d hits across %d files, dropped %d cache entr%s; index reseeded (%d memories)\n", r.Hits, len(r.Files), r.Dropped, pluralS(r.Dropped), r.Indexed)
	}
	for _, f := range r.Files {
		line := fmt.Sprintf("@F %s — %d hits", f.Path, f.Hits)
		if f.Dropped > 0 {
			line += fmt.Sprintf(", %d dropped", f.Dropped)
		}
		if f.Backup != "" {
			line += fmt.Sprintf(" (backup %s)", f.Backup)
		}
		fmt.Fprintln(out_, line)
	}
	for _, fam := range r.Families {
		fmt.Fprintf(out_, "@F family %s — %d hits on %d files\n", fam.Name, fam.Hits, fam.Files)
		fmt.Fprintf(out_, "@? review: %s\n", fam.Evolve)
	}
}

func pluralS(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
