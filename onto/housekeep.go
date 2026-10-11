package onto

// Housekeeping — the workspace's own chain policy, enforced by code:
// no rank without its chain (a coord without domains, a domain without zones,
// a zone without its truth edge is broken), no member without a home, no
// overlap left silent. `check` reports; `heal` seals the mechanical classes
// (register homes, rewire missing rank edges to their policy parent, seat uncovered
// hot dirs as draft zones); overlaps and dangling prose bonds are reported,
// never silently rewritten — those want an operator.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Repair is one sealed mechanical fix (file + what).
type Repair struct {
	Path, What string
}

// HouseReport is one housekeeping run: what the machine sealed, what still wants an operator.
type HouseReport struct {
	Repairs   []Repair
	Leftover  []string
	Healed    int
	Spawned   []string
	DeadBonds []string // seats whose Reports-to names a name with no live seat — healable mechanically
	Notes     []string // the after-pass holes (@?)
}

// Housekeep audits the workspace at root and, when heal is set, seals the mechanical classes.
// With spawn it also seats draft zone homes for tracked top-level dirs no member covers.
func Housekeep(root string, heal, spawn bool) (*HouseReport, []Finding, error) {
	rep := &HouseReport{}
	w, err := Load(root)
	if err != nil {
		return nil, nil, err
	}
	l := w.Layout.norm()
	rep.probeDeadBonds(w, l, heal)
	if !heal && !spawn {
		rep.Notes = w.Notes
		return rep, w.Validate(), nil
	}
	if heal {
		for _, n := range w.Notes {
			if strings.Contains(n, " has no doc in ") {
				if r, ok := healMissingDoc(w, l, n); ok {
					rep.Repairs = append(rep.Repairs, r)
					continue
				}
			}
			rep.Leftover = append(rep.Leftover, n)
		}
		for _, f := range w.Validate() {
			switch {
			case strings.Contains(f.Msg, "wants exactly 1"):
				if r, ok := healMissingEdge(w, l, f); ok {
					rep.Repairs = append(rep.Repairs, r)
				} else {
					rep.Leftover = append(rep.Leftover, f.String())
				}
			default:
				rep.Leftover = append(rep.Leftover, f.String())
			}
		}
	}
	if spawn {
		rep.Spawned = seatUncovered(w, l, root)
	}
	rep.Healed = len(rep.Repairs) + len(rep.Spawned)
	if rep.Healed > 0 {
		journal(root, l, rep)
	}
	w2, err2 := Load(root)
	if err2 != nil {
		return rep, nil, err2
	}
	rep.Notes = w2.Notes
	return rep, w2.Validate(), nil
}

// healMissingDoc registers a member home scaffold for a note like
// "<ws>/<rank>/<dir>/:1: <member> has no doc in <ws>/<rank>/<dir>/".
func healMissingDoc(w *Workspace, l Layout, note string) (Repair, bool) {
	i := strings.Index(note, " has no doc in ")
	if i < 0 {
		return Repair{}, false
	}
	dir := strings.TrimSuffix(strings.TrimSpace(note[i+len(" has no doc in "):]), "/")
	head := note[:i]
	if j := strings.Index(head, ": "); j >= 0 {
		head = head[j+2:]
	}
	name := strings.TrimSpace(head)
	subj := Ao(name)
	rank := rankOf(w, subj)
	if rank == "" {
		// the member is a dir-shape ghost the graph never derived; the dir names the rank
		parts := strings.Split(filepath.ToSlash(dir), "/")
		if len(parts) >= 2 {
			rank = parts[len(parts)-2]
		}
	}
	parent := parentMember(w, l, rank, "")
	if parent == "" {
		return Repair{}, false
	}
	path := filepath.Join(w.Root, dir, "README.md")
	body := fmt.Sprintf("# %s\n\n- **Rank:** %s\n- **Reports to:** %s\n- **Purpose:** seated by housekeep %s; describe the working notes and rewrite this line.\n\n## Invariants\n\n1. This doc was registered by the chain policy; its invariants and verify lines wait on their first persistent agent.\n\n## Verify\n\n- `test -f %s`\n\n## Working notes\n\n- %s — home registered by housekeep (missing member doc).\n",
		name, displayRank(rank), parent, today(), filepath.ToSlash(path), today())
	if err := os.MkdirAll(filepath.Join(w.Root, dir), 0o755); err != nil {
		return Repair{}, false
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return Repair{}, false
	}
	return Repair{Path: dir + "/", What: "registered a member home (" + rank + ", reports to " + parent + ")"}, true
}

// healMissingEdge sets a member's Reports-to to its policy parent when its chain edge is absent.
func healMissingEdge(w *Workspace, l Layout, f Finding) (Repair, bool) {
	rank := rankOf(w, f.Subject)
	if rank == "" {
		return Repair{}, false
	}
	doc := strings.SplitN(f.Doc, ":", 2)[0]
	parent := parentMember(w, l, rank, ownsOf(w, f.Subject))
	if parent == "" {
		return Repair{}, false
	}
	b, err := os.ReadFile(filepath.Join(w.Root, doc))
	if err != nil {
		return Repair{}, false
	}
	lines := strings.Split(string(b), "\n")
	set := false
	for i, s := range lines {
		if strings.HasPrefix(s, "- **Reports to:**") {
			lines[i] = "- **Reports to:** " + parent
			set = true
			break
		}
	}
	if !set {
		for i, s := range lines {
			if strings.HasPrefix(s, "- **Rank:**") {
				lines = append(lines[:i+1], append([]string{"- **Reports to:** " + parent}, lines[i+1:]...)...)
				set = true
				break
			}
		}
	}
	if !set {
		return Repair{}, false
	}
	if err := os.WriteFile(filepath.Join(w.Root, doc), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return Repair{}, false
	}
	return Repair{Path: doc, What: "rewired " + f.Subject.Local() + "'s chain edge to " + parent}, true
}

// seatUncovered registers draft zone homes for tracked top-level dirs no member covers.
func seatUncovered(w *Workspace, l Layout, root string) []string {
	zrank := ""
	for _, r := range l.Ranks {
		if ClassOf(r.Name).Local() == "Zone worker" {
			zrank = r.Name
			break
		}
	}
	var out []string
	for _, d := range trackedDirs(root) {
		covered := false
		for _, own := range allOwns(w) {
			if own == d || strings.HasPrefix(own, d+"/") || strings.HasPrefix(d, own+"/") {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		base := strings.TrimSuffix(d, "/")
		dir := filepath.Join(w.Root, l.WorkspaceDir, "zone", base)
		if _, err := os.Stat(filepath.Join(dir, "README.md")); err == nil {
			continue
		}
		parent := parentMember(w, l, zrank, base)
		if parent == "" {
			continue
		}
		name := zrank + "-" + base
		body := fmt.Sprintf("# %s\n\n- **Rank:** %s\n- **Owns:** `%s/`\n- **Reports to:** %s\n- **Purpose:** seated by housekeep --spawn %s (uncovered dir); its persistent agent refines this working notes.\n\n## Invariants\n\n1. Draft seat; ownership and invariants wait on their first persistent agent's proof.\n\n## Working notes\n\n- %s — seated by housekeep (no member covered this dir).\n",
			name, displayRank(zrank), base, parent, today(), today())
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(body), 0o644); err != nil {
			continue
		}
		out = append(out, name+" → "+parent)
	}
	return out
}

// — helpers

// rankOf answers a member's rank word from its graph class ("Zone worker" → "zone").
func rankOf(w *Workspace, m Term) string {
	for _, t := range w.Graph.Objects(m, rdfType) {
		cl := t.Local()
		for _, r := range w.Layout.norm().Ranks {
			if ClassOf(r.Name).Local() == cl {
				return r.Name
			}
		}
	}
	return ""
}

// ownsOf answers the member's first asserted owns path (used for parent matching).
func ownsOf(w *Workspace, m Term) string {
	for _, t := range w.Graph.Objects(m, pOwns) {
		if t.Kind == Literal {
			return t.Value
		}
	}
	return ""
}

// allOwns lists every asserted owns path.
func allOwns(w *Workspace) []string {
	var out []string
	for _, tr := range w.Graph.All() {
		if tr.P == pOwns && tr.O.Kind == Literal {
			out = append(out, tr.O.Value)
		}
	}
	sort.Strings(out)
	return out
}

// parentMember picks the concrete parent member for a rank (Egress control match wins, else first by name).
func parentMember(w *Workspace, l Layout, rank, childOwns string) string {
	want := ""
	for _, r := range l.Ranks {
		// the doc's Rank field reads the display name ("Domain owner"); the layout keys the canonical one
		if r.Name == rank || strings.EqualFold(displayRank(r.Name), rank) {
			want = r.Parent
			break
		}
	}
	if want == "" {
		return ""
	}
	if want == "orchestrator" {
		return "orchestrator"
	}
	prefix := want + "-"
	var cands []string
	best := ""
	for _, tr := range w.Graph.All() {
		if tr.P == pName && tr.O.Kind == Literal && strings.HasPrefix(tr.O.Value, prefix) {
			cands = append(cands, tr.O.Value)
			if childOwns != "" && ownsHas(w, tr.S, childOwns) {
				best = tr.O.Value
			}
		}
	}
	if best != "" {
		return best
	}
	if len(cands) == 0 {
		return ""
	}
	sort.Strings(cands)
	return cands[0]
}

// ownsHas: the member owns a path containing child (prefix either way).
func ownsHas(w *Workspace, m Term, child string) bool {
	for _, tr := range w.Graph.All() {
		if tr.S == m && tr.P == pOwns && tr.O.Kind == Literal {
			s := tr.O.Value
			if s == child || strings.HasPrefix(child, s+"/") || strings.HasPrefix(s, child+"/") {
				return true
			}
		}
	}
	return false
}

func displayRank(rank string) string {
	// Class labels are lexicon truth; the doc's Rank field is "Domain owner" style.
	s := ClassOf(rank).Local()
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// trackedDirs answers the repo's tracked top-level dirs (git ls-files).
func trackedDirs(root string) []string {
	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var dirs []string
	for _, ln := range strings.Split(string(out), "\n") {
		i := strings.IndexByte(ln, '/')
		if i <= 0 {
			continue
		}
		d := ln[:i]
		if strings.HasPrefix(d, ".") || seen[d] {
			continue
		}
		seen[d] = true
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs
}

func today() string { return time.Now().Format("2006-01-02") }

// journal appends the run to the workspace chronicle, house shape.
func journal(root string, l Layout, rep *HouseReport) {
	p := filepath.Join(root, l.WorkspaceDir, "log.md")
	var lines []string
	for _, r := range rep.Repairs {
		lines = append(lines, "  - "+r.Path+" — "+r.What)
	}
	for _, s := range rep.Spawned {
		lines = append(lines, "  - "+s+" (draft seat)")
	}
	entry := "### [" + time.Now().Format("2006-01-02 15:04") + "] housekeep — chain repairs\n\n- Repairs:\n" + strings.Join(lines, "\n")
	if len(rep.Leftover) > 0 {
		entry += "\n- Leftover (operator):\n  - " + strings.Join(rep.Leftover, "\n  - ")
	}
	entry += "\n"
	if _, err := os.Stat(p); err != nil {
		entry = "# Log\n\n" + entry
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprint(f, "\n"+entry)
}

// probeDeadBonds flags every member whose Reports-to names a name with no
// live seat; a dead bond mechanically heals to the seat's policy parent; a live
// one — whatever the policy says — is the operator's bond.
func (rep *HouseReport) probeDeadBonds(w *Workspace, l Layout, heal bool) {
	workspaceDir := filepath.Join(w.Root, l.WorkspaceDir)
	live := map[string]bool{}
	filepath.WalkDir(workspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "README.md" {
			return nil
		}
		rel, _ := filepath.Rel(workspaceDir, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) == 3 {
			live[parts[1]] = true // <rank>/<name>/README.md -> name
		}
		return nil
	})
	filepath.WalkDir(workspaceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "README.md" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		body := string(b)
		bond := bondOf(body, "Reports to")
		if bond == "" || live[bond] {
			return nil // empty, or a live seat — including a live non-policy target, the operator's bond
		}
		rank, name := memberRankName(body, workspaceDir, path)
		parent := parentMember(w, l, rank, ownsLine(body))
		if parent == "" || parent == bond {
			return nil // no policy parent for this rank, or already on it — nothing mechanical to seal
		}
		rep.DeadBonds = append(rep.DeadBonds, name)
		if heal {
			lines := strings.Split(body, "\n")
			for i, ln := range lines {
				if strings.HasPrefix(ln, "- **Reports to:** ") {
					lines[i] = "- **Reports to:** " + parent
					break
				}
			}
			if os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644) == nil {
				rep.Repairs = append(rep.Repairs, Repair{Path: l.WorkspaceDir + "/" + rank + "/" + name + "/README.md", What: "rewired " + name + "'s dead bond (" + bond + ") to policy parent " + parent})
			} else {
				rep.Notes = append(rep.Notes, "@? "+name+" dead bond unhealed")
			}
		}
		return nil
	})
}

// memberRankName derives a seat's rank dir and name from its doc — Rank line first, path fallback.
func memberRankName(body, workspaceDir, path string) (rank, name string) {
	rel, _ := filepath.Rel(workspaceDir, path)
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 3 {
		rank, name = parts[0], parts[1]
	}
	if r := bondOf(body, "Rank"); r != "" {
		rank = strings.ToLower(r)
	}
	return rank, name
}

// ownsLine reads a doc's `- **Owns:** `x/“ first own, or "" — feeds the policy-parent map.
func ownsLine(body string) string {
	for ln := range strings.Lines(body) {
		if strings.HasPrefix(ln, "- **Owns:**") {
			s := strings.TrimSpace(strings.TrimPrefix(ln, "- **Owns:**"))
			return strings.TrimSuffix(strings.Trim(strings.TrimSpace(strings.Split(s, ",")[0]), "`"), "/")
		}
	}
	return ""
}

// bondOf reads `- **<field>:** value` from a member doc, or "".
func bondOf(body, field string) string {
	for ln := range strings.Lines(body) {
		if strings.HasPrefix(ln, "- **"+field+":** ") {
			return strings.TrimSpace(strings.TrimPrefix(ln, "- **"+field+":** "))
		}
	}
	return ""
}
