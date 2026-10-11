package board

import (
	"bytes"
	"go/parser"
	"go/token"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Relate is a bond the ontology does not assert but the code does: two members whose ownerships
// import each other, or who hold the same skills. The net's 3D brain pulls related members
// together with it, so the neighbourhoods the work keeps show as neighbourhoods on the stage.
type Relate struct {
	From    string  `json:"from"`
	To      string  `json:"to"`
	Weight  float64 `json:"weight"`  // 0..1, the strongest bond in the workspace is 1
	Imports int     `json:"imports"` // source files of one ownership importing the other's code
	Skills  int     `json:"skills"`  // skills both hold
}

// relateKeep is how many relates a member keeps (its strongest); relateFloor drops the faint.
const (
	relateKeep  = 4
	relateFloor = 0.08
	relateFiles = 20000   // a walk stops after this many source files
	relateSize  = 1 << 20 // a source file larger than this is not read
)

var (
	rePyImp = regexp.MustCompile(`(?m)^[ \t]*(?:from[ \t]+(\.*[\w.]*)[ \t]+import|import[ \t]+([\w.]+))`)
	reJSImp = regexp.MustCompile(`(?:import|export)[^'"\n;]*?from[ \t]*['"]([^'"]+)['"]|import[ \t]*\(?[ \t]*['"]([^'"]+)['"]|require\([ \t]*['"]([^'"]+)['"][ \t]*\)`)
)

// Relates couples members by their ownerships' imports (Go through go/parser's imports-only
// pass; Python and JS/TS by their import lines, relative ones resolved) and by the skills they both
// hold. A file belongs to the member whose owned path names it most closely, so a zone worker's code
// counts for the zone worker, not for the domain owner and coordinator whose wider ownerships also hold it; a pair where
// one is the other's ancestor is the tree's business and is left out. Weights are log-scaled and
// normalised; each member keeps its strongest few. Never nil.
func Relates(root string, nodes []Node, edges []Edge) []Relate {
	owns := map[string][]string{} // member id → its owned paths, lowercased and slash-cleaned
	parent := map[string]string{}
	for _, n := range nodes {
		if n.Kind == "skill" {
			continue
		}
		parent[n.ID] = n.Parent
		for _, o := range n.Owns {
			o = strings.Trim(path.Clean(strings.ToLower(filepath.ToSlash(o))), "/")
			if o != "" && o != "." {
				owns[n.ID] = append(owns[n.ID], o)
			}
		}
	}
	above := func(a, b string) bool { // a is an ancestor of b
		for p, guard := parent[b], 0; p != "" && guard < 64; p, guard = parent[p], guard+1 {
			if p == a {
				return true
			}
		}
		return false
	}
	type pair struct{ a, b string }
	key := func(a, b string) pair {
		if a > b {
			a, b = b, a
		}
		return pair{a, b}
	}
	related := func(a, b string) bool { return a != b && !above(a, b) && !above(b, a) }
	// of the owners tied on a path, the deepest: a zone worker and the domain owner above it both naming `loop`
	// leave the file to the zone worker
	deepest := func(ids []string) []string {
		out := ids[:0:0]
		for _, a := range ids {
			keep := true
			for _, b := range ids {
				if above(a, b) {
					keep = false
					break
				}
			}
			if keep {
				out = append(out, a)
			}
		}
		return out
	}
	imports := map[pair]int{}
	if len(owns) > 0 {
		for src, tgts := range importGraph(root) {
			from := deepest(ownersOf(owns, src))
			if len(from) == 0 {
				continue
			}
			for _, t := range tgts {
				for _, b := range deepest(ownersOf(owns, t)) {
					for _, a := range from {
						if related(a, b) {
							imports[key(a, b)]++
						}
					}
				}
			}
		}
	}
	// shared skills: a skill held by many weighs less on each pair it ties
	holders := map[string][]string{}
	isSkill := map[string]bool{}
	for _, n := range nodes {
		isSkill[n.ID] = n.Kind == "skill"
	}
	for _, e := range edges {
		if e.Bond == "holds" && isSkill[e.To] && !isSkill[e.From] {
			if _, ok := parent[e.From]; ok {
				holders[e.To] = append(holders[e.To], e.From)
			}
		}
	}
	skills, skillW := map[pair]int{}, map[pair]float64{}
	for _, ws := range holders {
		sort.Strings(ws)
		for i := range ws {
			for j := i + 1; j < len(ws); j++ {
				if related(ws[i], ws[j]) {
					k := key(ws[i], ws[j])
					skills[k]++
					skillW[k] += 1 / float64(len(ws)-1)
				}
			}
		}
	}
	score := map[pair]float64{}
	top := 0.0
	for k, n := range imports {
		score[k] += math.Log2(1 + float64(n))
	}
	for k, w := range skillW {
		score[k] += 1.5 * w
	}
	for _, s := range score {
		top = math.Max(top, s)
	}
	all := make([]Relate, 0, len(score))
	for k, s := range score {
		w := s / top
		if w < relateFloor {
			continue
		}
		all = append(all, Relate{From: k.a, To: k.b, Weight: math.Round(w*1000) / 1000, Imports: imports[k], Skills: skills[k]})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Weight != all[j].Weight {
			return all[i].Weight > all[j].Weight
		}
		if all[i].From != all[j].From {
			return all[i].From < all[j].From
		}
		return all[i].To < all[j].To
	})
	// each member keeps its strongest few; a bond stays when either end keeps it
	kept, out := map[string]int{}, []Relate{}
	for _, r := range all {
		if kept[r.From] < relateKeep || kept[r.To] < relateKeep {
			kept[r.From]++
			kept[r.To]++
			out = append(out, r)
		}
	}
	return out
}

// ownersOf: the members whose owned path names rel most closely (the longest match wins; ties
// all own it). rel is lowercased, root-relative, slash-separated.
func ownersOf(owns map[string][]string, rel string) []string {
	rel = strings.ToLower(rel)
	best, who := 0, []string(nil)
	for id, list := range owns {
		for _, o := range list {
			hit := rel == o || strings.HasPrefix(rel, o+"/")
			if !hit && strings.ContainsAny(o, "*?[") {
				hit, _ = path.Match(o, rel)
			}
			if !hit {
				continue
			}
			switch {
			case len(o) > best:
				best, who = len(o), []string{id}
			case len(o) == best && (len(who) == 0 || who[len(who)-1] != id):
				who = append(who, id)
			}
		}
	}
	sort.Strings(who)
	return who
}

// importGraph is the source walk's import half (see sourceWalk).
func importGraph(root string) map[string][]string { return sourceOf(root).imports }

// sourceIndex is one walk of a workspace's source: each file's in-repo imports and its line count.
type sourceIndex struct {
	imports map[string][]string
	lines   map[string]int
}

// sources are a walk of the workspace: kept a minute per root, so the team's polling stays cheap.
var sourceMemo struct {
	sync.Mutex
	root string
	at   time.Time
	val  sourceIndex
}

// sourceOf is sourceWalk, remembered for a minute.
func sourceOf(root string) sourceIndex {
	sourceMemo.Lock()
	defer sourceMemo.Unlock()
	if sourceMemo.root == root && time.Since(sourceMemo.at) < time.Minute {
		return sourceMemo.val
	}
	v := sourceWalk(root)
	sourceMemo.root, sourceMemo.at, sourceMemo.val = root, time.Now(), v
	return v
}

// generated reads a file's head for the generator's mark (Go's "Code generated … DO NOT EDIT",
// and the same words in any language's header).
func generated(b []byte) bool {
	head := b
	if len(head) > 512 {
		head = head[:512]
	}
	return bytes.Contains(head, []byte("Code generated")) || bytes.Contains(head, []byte("@generated")) || bytes.Contains(head, []byte("DO NOT EDIT"))
}

// sourceWalk walks root's source (tests, vendored and generated trees left out — the same cut
// init's survey makes) and returns each file's in-repo imports as root-relative paths (a Go
// package's directory, a Python module's file or package, a JS/TS module's resolved path) and
// each file's lines.
func sourceWalk(root string) sourceIndex {
	mod := ""
	if b, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		if m := regexp.MustCompile(`(?m)^module[ \t]+(\S+)`).FindSubmatch(b); m != nil {
			mod = string(m[1])
		}
	}
	out := map[string][]string{}
	lines := map[string]int{}
	files := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "testdata" || name == "dist" || name == "build" || name == "__pycache__") {
				return filepath.SkipDir
			}
			return nil
		}
		if files >= relateFiles {
			return filepath.SkipAll
		}
		ext := path.Ext(name)
		if ext != ".go" && ext != ".py" && ext != ".js" && ext != ".mjs" && ext != ".ts" && ext != ".tsx" && ext != ".jsx" {
			return nil
		}
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".min.js") || strings.HasSuffix(name, ".d.ts") || strings.HasPrefix(name, "test_") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > relateSize {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil || generated(b) {
			return nil
		}
		files++
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		lines[rel] = bytes.Count(b, []byte("\n"))
		if len(b) > 0 && b[len(b)-1] != '\n' {
			lines[rel]++
		}
		dir := path.Dir(rel)
		var tgts []string
		switch ext {
		case ".go":
			if mod == "" {
				return nil
			}
			af, err := parser.ParseFile(token.NewFileSet(), name, b, parser.ImportsOnly)
			if err != nil {
				return nil
			}
			for _, im := range af.Imports {
				ip := strings.Trim(im.Path.Value, `"`)
				if strings.HasPrefix(ip, mod+"/") {
					tgts = append(tgts, strings.TrimPrefix(ip, mod+"/"))
				}
			}
		case ".py":
			for _, m := range rePyImp.FindAllSubmatch(b, -1) {
				spec := string(m[1]) + string(m[2])
				if t := pyTarget(root, dir, spec); t != "" {
					tgts = append(tgts, t)
				}
			}
		default:
			for _, m := range reJSImp.FindAllSubmatch(b, -1) {
				spec := string(bytes.Join([][]byte{m[1], m[2], m[3]}, nil))
				if strings.HasPrefix(spec, ".") {
					t := path.Clean(path.Join(dir, spec))
					if !strings.HasPrefix(t, "..") {
						tgts = append(tgts, t)
					}
				}
			}
		}
		if len(tgts) > 0 {
			out[rel] = tgts
		}
		return nil
	})
	return sourceIndex{imports: out, lines: lines}
}

// Masses sizes the team: each member's own lines (the files whose deepest owner it is — the
// rule relates count by), its mass the own lines of its whole subtree, the orchestrator's the whole
// workspace's source; a skill's mass is how many hold it. Activity never enters it.
func Masses(root string, c *Team) {
	if c == nil || root == "" {
		return
	}
	owns := map[string][]string{}
	parent := map[string]string{}
	for _, n := range c.Nodes {
		if n.Kind == "skill" {
			continue
		}
		parent[n.ID] = n.Parent
		for _, o := range n.Owns {
			o = strings.Trim(path.Clean(strings.ToLower(filepath.ToSlash(o))), "/")
			if o != "" && o != "." {
				owns[n.ID] = append(owns[n.ID], o)
			}
		}
	}
	above := func(a, b string) bool {
		for p, guard := parent[b], 0; p != "" && guard < 64; p, guard = parent[p], guard+1 {
			if p == a {
				return true
			}
		}
		return false
	}
	own := map[string]int{}
	total := 0
	for file, n := range sourceOf(root).lines {
		total += n
		ids := ownersOf(owns, file)
		var deep []string
		for _, a := range ids {
			keep := true
			for _, b := range ids {
				if above(a, b) {
					keep = false
					break
				}
			}
			if keep {
				deep = append(deep, a)
			}
		}
		for _, id := range deep {
			own[id] += n / len(deep)
		}
	}
	mass := map[string]int{}
	for id, n := range own {
		for cur, guard := id, 0; cur != "" && guard < 64; cur, guard = parent[cur], guard+1 {
			mass[cur] += n
		}
	}
	holders := map[string]int{}
	for _, e := range c.Edges {
		if e.Bond == "holds" {
			holders[e.To]++
		}
	}
	for i := range c.Nodes {
		n := &c.Nodes[i]
		switch {
		case n.Kind == "skill":
			n.Mass = holders[n.ID]
		case n.Orchestrator:
			n.Mass, n.Own = total, own[n.ID]
		default:
			n.Mass, n.Own = mass[n.ID], own[n.ID]
		}
	}
}

// pyTarget resolves a Python import to a root-relative path when it names this repo's code.
func pyTarget(root, dir, spec string) string {
	lead := len(spec) - len(strings.TrimLeft(spec, "."))
	mod := strings.ReplaceAll(spec[lead:], ".", "/")
	base := ""
	if lead > 0 {
		base = dir
		for i := 1; i < lead; i++ {
			base = path.Dir(base)
		}
	}
	p := path.Clean(path.Join(base, mod))
	if p == "." || strings.HasPrefix(p, "..") {
		return ""
	}
	if isDirAt(filepath.Join(root, p)) {
		return p
	}
	if _, err := os.Stat(filepath.Join(root, p+".py")); err == nil {
		return p + ".py"
	}
	return ""
}

func isDirAt(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }

// relates are a walk of the source: kept a minute per workspace, so the team's polling stays cheap.
var relateMemo struct {
	sync.Mutex
	root, sig string
	at        time.Time
	val       []Relate
}

// RelatesFor is Relates over a team, remembered for a minute (or until the team's shape
// changes). The dash reads it as the team's "relates".
func RelatesFor(root string, c *Team) []Relate {
	if c == nil || root == "" {
		return []Relate{}
	}
	var sig strings.Builder
	for _, n := range c.Nodes {
		sig.WriteString(n.ID + "|" + n.Parent + "|" + strings.Join(n.Owns, ",") + ";")
	}
	for _, e := range c.Edges {
		if e.Bond == "holds" {
			sig.WriteString(e.From + ">" + e.To + ";")
		}
	}
	relateMemo.Lock()
	defer relateMemo.Unlock()
	if relateMemo.root == root && relateMemo.sig == sig.String() && time.Since(relateMemo.at) < time.Minute {
		return relateMemo.val
	}
	v := Relates(root, c.Nodes, c.Edges)
	relateMemo.root, relateMemo.sig, relateMemo.at, relateMemo.val = root, sig.String(), time.Now(), v
	return v
}
