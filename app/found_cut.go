package app

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Cutting the team from the survey (found_survey.go), with the hierarchy the ranks require
// (workspace/rank.go): a zone worker per meaningful package or module dir — tiny siblings merged into
// what they import, giant dirs split into their sub-dirs — domain owners grouping related zone workers into
// domains by imports, role and naming, coordinators over the domain owners so none carries more than three,
// a service owner when there is CI, release or infrastructure to own. Ownerships are directories (the
// root's own source files are the one exception: the root dir cannot be claimed without
// covering every other zone).

const (
	maxZoneLines    = 6000 // a dir larger than this is split into its sub-dirs when it can be
	tinyLines       = 120  // a unit smaller than this merges into what it is bound to
	zonesPerDomain  = 4    // the target; a domain owner carries at most maxPerDomain
	maxPerDomain    = 6
	domainsPerCoord = 3
	maxUnits        = 60
)

// unit is one zone-to-be: one or more directories (or the root's files) and what they hold.
type unit struct {
	dirs, files              []string // claimed dirs (slash, no trailing slash) and root files
	name, role               string
	src, tests, lines        int
	langs                    map[string]int
	pkgs, docs, entry, names []string
	exported                 int
	mod                      *module // the build module this unit is or sits in
	modDir                   string
	goMod                    string // the go module dir it sits in ("" root; "-" none)
	pyRoot                   bool
}

func (u *unit) lang() string {
	best, n := "", 0
	for l, c := range u.langs {
		if c > n || (c == n && l < best) {
			best, n = l, c
		}
	}
	return best
}

func (u *unit) claims() []string {
	out := append([]string(nil), u.files...)
	for _, d := range u.dirs {
		out = append(out, d+"/")
	}
	return out
}

var (
	nsDirs   = map[string]bool{"src": true, "internal": true, "pkg": true, "lib": true, "libs": true, "packages": true, "apps": true, "crates": true, "modules": true, "services": true, "plugins": true, "extensions": true, "projects": true}
	jvmChain = map[string]bool{"main": true, "java": true, "kotlin": true, "scala": true, "com": true, "org": true, "net": true, "io": true, "dev": true}
	roleOf   = map[string]string{
		"cmd": "app", "app": "app", "apps": "app", "cli": "app", "server": "app", "main": "app",
		"ui": "interfaces", "tui": "interfaces", "web": "interfaces", "webui": "interfaces", "frontend": "interfaces", "client": "interfaces", "board": "interfaces",
		"dashboard": "interfaces", "views": "interfaces", "components": "interfaces", "pages": "interfaces", "site": "interfaces", "www": "interfaces", "gui": "interfaces",
		"api": "interfaces", "http": "interfaces", "handlers": "interfaces", "routes": "interfaces", "rest": "interfaces", "grpc": "interfaces", "graphql": "interfaces",
		"config": "config", "configs": "config", "conf": "config", "settings": "config",
		"test": "tests", "tests": "tests", "e2e": "tests", "integration": "tests", "spec": "tests", "__tests__": "tests",
	}
)

// carve picks the unit dirs: top-level source dirs, namespace containers opened, module
// containers opened down to their modules, giants split into their sub-dirs — never a dir
// that holds source files of its own (its files would be left without a persistent agent).
func (s *survey) carve() []string {
	cand := append([]string(nil), s.dirs[""].kids...)
	for pass := 0; pass < 10; pass++ {
		lines := make([]int, 0, len(cand))
		for _, c := range cand {
			lines = append(lines, s.dirs[c].aggLines)
		}
		med := median(lines)
		var next []string
		changed := false
		for _, c := range cand {
			di := s.dirs[c]
			base := path.Base(c)
			open := false
			if di.src == 0 && di.tests == 0 && len(di.kids) > 0 && len(next)+len(di.kids) <= maxUnits {
				switch {
				case s.modules[c] == nil && s.holdsModule(c):
					open = true
				case s.modules[c] == nil && nsDirs[base]:
					open = true
				case jvmChain[base] && s.langs["jvm"] > 0:
					open = true
				case len(di.kids) >= 2 && (di.aggLines > maxZoneLines || (len(cand) > 1 && di.aggLines > 3*med && di.aggLines > 2500)):
					open = true
				}
			}
			if open && !nameable(di.kids) {
				open = false // its sub-dirs could not be claimed under any name the ontology derives
			}
			if open {
				next = append(next, di.kids...)
				changed = true
			} else {
				next = append(next, c)
			}
		}
		cand = next
		if !changed {
			break
		}
	}
	sort.Strings(cand)
	return cand
}

func (s *survey) holdsModule(dir string) bool {
	for m := range s.modules {
		if strings.HasPrefix(m, dir+"/") {
			return true
		}
	}
	return false
}

func median(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	c := append([]int(nil), xs...)
	sort.Ints(c)
	return c[len(c)/2]
}

// units turns the carved dirs (and the root's own source files) into units with their facts.
func (s *survey) units() []*unit {
	var out []*unit
	byDir := map[string]*unit{}
	for _, d := range s.carve() {
		u := &unit{dirs: []string{d}, langs: map[string]int{}}
		byDir[d] = u
		out = append(out, u)
	}
	var rootU *unit
	owner := func(dir string) *unit {
		for d := dir; ; d = path.Dir(d) {
			if d == "." || d == "" {
				return nil
			}
			if u := byDir[d]; u != nil {
				return u
			}
		}
	}
	for i := range s.files {
		f := &s.files[i]
		u := owner(f.dir)
		if f.dir == "" {
			if rootU == nil {
				rootU = &unit{langs: map[string]int{}}
			}
			u = rootU
			rootU.files = append(rootU.files, f.rel)
		}
		if u == nil {
			continue
		}
		s.absorb(u, f)
	}
	if rootU != nil {
		sort.Strings(rootU.files)
		out = append([]*unit{rootU}, out...)
	}
	for _, u := range out {
		s.place(u)
	}
	return out
}

func (s *survey) absorb(u *unit, f *srcFile) {
	if !f.test {
		u.src++
	}
	if f.hasTests {
		u.tests++
	}
	u.lines += f.lines
	u.langs[f.lang]++
	if f.pkg != "" && !f.test && f.lang != "jvm" && !contains(u.pkgs, f.pkg) {
		u.pkgs = append(u.pkgs, f.pkg)
	}
	if f.doc != "" && !f.test && len(u.docs) < 3 && !contains(u.docs, f.doc) {
		u.docs = append(u.docs, f.doc)
	}
	if f.main && !f.test {
		e := f.dir
		if e == "" {
			e = f.rel
		}
		if !contains(u.entry, e) {
			u.entry = append(u.entry, e)
		}
	}
	u.exported += len(f.exported)
	for _, n := range f.exported {
		if len(u.names) < 8 && !contains(u.names, n) {
			u.names = append(u.names, n)
		}
	}
}

// place finds the build module and go module a unit sits in.
func (s *survey) place(u *unit) {
	u.goMod = "-"
	probe := ""
	if len(u.dirs) > 0 {
		probe = u.dirs[0]
	}
	for d := probe; ; d = path.Dir(d) {
		if d == "." {
			d = ""
		}
		if m := s.modules[d]; m != nil && u.mod == nil && d != "" {
			u.mod, u.modDir = m, d
		}
		if _, ok := s.goMods[d]; ok && u.goMod == "-" {
			u.goMod = d
		}
		if d == "" {
			break
		}
	}
}

// bonds weighs the imports between units: each file importing into another unit counts one
// (a declared module dependency counts three).
func (s *survey) bonds(us []*unit) [][]float64 {
	n := len(us)
	w := make([][]float64, n)
	for i := range w {
		w[i] = make([]float64, n)
	}
	type claim struct {
		dir string
		i   int
	}
	var cl []claim
	for i, u := range us {
		for _, d := range u.dirs {
			cl = append(cl, claim{d, i})
		}
	}
	sort.Slice(cl, func(a, b int) bool { return len(cl[a].dir) > len(cl[b].dir) })
	rootUnit := -1
	for i, u := range us {
		if len(u.files) > 0 {
			rootUnit = i
		}
	}
	unitOfDir := func(dir string) int {
		if dir == "" {
			return rootUnit
		}
		for _, c := range cl {
			if dir == c.dir || strings.HasPrefix(dir, c.dir+"/") {
				return c.i
			}
		}
		return -1
	}
	fileUnit := map[string]int{}
	for i, u := range us {
		for _, f := range u.files {
			fileUnit[f] = i
		}
	}
	unitOfFile := func(f *srcFile) int {
		if i, ok := fileUnit[f.rel]; ok {
			return i
		}
		return unitOfDir(f.dir)
	}
	// resolvers
	type gomod struct{ dir, path string }
	var gms []gomod
	for d, p := range s.goMods {
		if p != "" {
			gms = append(gms, gomod{d, p})
		}
	}
	sort.Slice(gms, func(a, b int) bool { return len(gms[a].path) > len(gms[b].path) })
	modByName := map[string]string{}
	for d, m := range s.modules {
		modByName[m.name] = d
	}
	pyDirs := map[string]string{} // dotted → dir
	jvmPkgs := map[string]string{}
	for _, f := range s.files {
		if f.lang == "python" && f.dir != "" {
			pyDirs[strings.ReplaceAll(f.dir, "/", ".")] = f.dir
			if strings.HasPrefix(f.dir, "src/") {
				pyDirs[strings.ReplaceAll(strings.TrimPrefix(f.dir, "src/"), "/", ".")] = f.dir
			}
		}
		if f.lang == "jvm" && f.pkg != "" {
			jvmPkgs[f.pkg] = f.dir
		}
	}
	resolve := func(f *srcFile, spec string) int {
		switch f.lang {
		case "go":
			for _, g := range gms {
				if spec == g.path || strings.HasPrefix(spec, g.path+"/") {
					return unitOfDir(strings.Trim(path.Join(g.dir, strings.TrimPrefix(spec, g.path)), "/"))
				}
			}
		case "python":
			if strings.HasPrefix(spec, ".") {
				dots := len(spec) - len(strings.TrimLeft(spec, "."))
				d := f.dir
				for k := 1; k < dots; k++ {
					d = path.Dir(d)
				}
				rest := strings.ReplaceAll(strings.TrimLeft(spec, "."), ".", "/")
				return unitOfDir(strings.Trim(path.Join(d, rest), "/"))
			}
			for p := spec; p != ""; {
				if d, ok := pyDirs[p]; ok {
					return unitOfDir(d)
				}
				i := strings.LastIndexByte(p, '.')
				if i < 0 {
					break
				}
				p = p[:i]
			}
		case "node":
			if strings.HasPrefix(spec, ".") {
				return unitOfDir(strings.Trim(path.Join(f.dir, spec), "/"))
			}
			name := spec
			parts := strings.Split(spec, "/")
			if strings.HasPrefix(spec, "@") && len(parts) > 1 {
				name = parts[0] + "/" + parts[1]
			} else {
				name = parts[0]
			}
			if d, ok := modByName[name]; ok {
				return unitOfDir(d)
			}
		case "jvm":
			for p := spec; p != ""; {
				if d, ok := jvmPkgs[p]; ok {
					return unitOfDir(d)
				}
				i := strings.LastIndexByte(p, '.')
				if i < 0 {
					break
				}
				p = p[:i]
			}
		}
		return -1
	}
	for k := range s.files {
		f := &s.files[k]
		from := unitOfFile(f)
		if from < 0 {
			continue
		}
		seen := map[int]bool{}
		for _, im := range f.imports {
			if to := resolve(f, im); to >= 0 && to != from && !seen[to] {
				seen[to] = true
				w[from][to]++
			}
		}
	}
	for d, m := range s.modules {
		from := unitOfDir(d)
		if from < 0 {
			continue
		}
		for _, dep := range m.deps {
			if dd, ok := modByName[dep]; ok {
				if to := unitOfDir(dd); to >= 0 && to != from {
					w[from][to] += 3
				}
			}
		}
	}
	return w
}

// mergeTiny folds a tiny unit into the unit it is most bound to; tiny siblings with no bond
// fold together. Returns the surviving units and their bonds.
func (s *survey) mergeTiny(us []*unit) []*unit {
	for {
		w := s.bonds(us)
		idx := make([]int, len(us))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool { return us[idx[a]].lines < us[idx[b]].lines })
		merged := false
		for _, i := range idx {
			u := us[i]
			if len(u.files) > 0 || !(u.lines < tinyLines || (u.src+u.tests <= 1 && u.lines < 2*tinyLines)) || len(us) <= 1 {
				continue
			}
			best, bw := -1, 0.0
			for j, v := range us {
				if j == i || len(v.files) > 0 || !mergeable(v, u) {
					continue
				}
				b := w[i][j] + w[j][i]
				if b > bw || (b == bw && b > 0 && v.lines > us[best].lines) {
					best, bw = j, b
				}
			}
			if best < 0 {
				// no bond: a tiny sibling under the same parent dir
				for j, v := range us {
					if j != i && len(v.files) == 0 && path.Dir(v.dirs[0]) == path.Dir(u.dirs[0]) && path.Dir(u.dirs[0]) != "." && mergeable(v, u) &&
						(v.lines < tinyLines || v.src+v.tests <= 1) {
						best = j
						break
					}
				}
			}
			if best < 0 {
				continue
			}
			into := us[best]
			if u.lines > into.lines && bw == 0 {
				into, u = u, into
				best, i = i, best
			}
			into.dirs = append(into.dirs, u.dirs...)
			sort.Strings(into.dirs)
			into.src += u.src
			into.tests += u.tests
			into.lines += u.lines
			for l, c := range u.langs {
				into.langs[l] += c
			}
			for _, p := range u.pkgs {
				if !contains(into.pkgs, p) {
					into.pkgs = append(into.pkgs, p)
				}
			}
			into.docs = append(into.docs, u.docs...)
			into.entry = append(into.entry, u.entry...)
			into.exported += u.exported
			for _, n := range u.names {
				if len(into.names) < 8 && !contains(into.names, n) {
					into.names = append(into.names, n)
				}
			}
			us = append(us[:i], us[i+1:]...)
			merged = true
			break
		}
		if !merged {
			return us
		}
	}
}

// nameUnits gives each unit its short name: the dir's base (the module's name when the base is
// generic), the parent's prefix on a collision.
func (s *survey) nameUnits(us []*unit, project string) {
	base := func(u *unit) string {
		if len(u.dirs) == 0 {
			if len(u.pkgs) == 1 && u.pkgs[0] != "main" {
				return slug(u.pkgs[0])
			}
			if project != "" {
				return project
			}
			return "root"
		}
		d := u.dirs[0]
		if u.mod != nil && u.modDir == d && u.mod.name != "" {
			n := u.mod.name
			if i := strings.LastIndexByte(n, '/'); i >= 0 {
				n = n[i+1:]
			}
			return slug(n)
		}
		b := path.Base(d)
		if (b == "src" || b == "lib" || b == "main" || b == "java") && path.Dir(d) != "." {
			b = path.Base(path.Dir(d)) + "-" + b
		}
		return slug(b)
	}
	used := map[string]bool{}
	// nested units first: their names are bound by the ontology's zone rule (zoneOK)
	for _, u := range us {
		if r, _ := unitRoot(u.dirs); r != "" {
			u.name = nestedName(r, u.dirs, used, true)
			if u.name == "" {
				u.name = r // carve and mergeTiny keep this from happening; the check in tests says so
			}
			used[u.name] = true
		}
	}
	count := map[string]int{}
	for _, u := range us {
		if r, _ := unitRoot(u.dirs); r == "" {
			u.name = base(u)
			count[u.name]++
		}
	}
	for _, u := range us {
		if r, _ := unitRoot(u.dirs); r == "" {
			if count[u.name] > 1 && len(u.dirs) > 0 && path.Dir(u.dirs[0]) != "." {
				u.name = slug(path.Base(path.Dir(u.dirs[0]))) + "-" + u.name
			}
			n := u.name
			for k := 2; used[n]; k++ {
				n = fmt.Sprintf("%s-%d", u.name, k)
			}
			u.name = n
			used[n] = true
		}
		u.role = unitRole(u)
	}
}

// mergeable: the dirs of two units can live under one name the ontology derives.
func mergeable(into, u *unit) bool {
	dirs := append(append([]string(nil), into.dirs...), u.dirs...)
	r, ok := unitRoot(dirs)
	if !ok {
		return false
	}
	if r == "" {
		return true
	}
	// the host's name keeps its first dir's tail; every merged nested dir must carry it
	return nestedName(r, dirs, map[string]bool{}, false) != ""
}

// nameable: every dir can be a claim under some name the ontology derives.
func nameable(dirs []string) bool {
	for _, d := range dirs {
		if r := nestRoot(d); r != "" && (!rootNameable(r) || len(tailsOf(d)) == 0) {
			return false
		}
	}
	return true
}

func unitRole(u *unit) string {
	if u.src == 0 && u.tests > 0 {
		return "tests"
	}
	for _, d := range u.dirs {
		if r := roleOf[path.Base(d)]; r != "" {
			return r
		}
		if strings.HasPrefix(d, "apps/") {
			return "app"
		}
	}
	if len(u.pkgs) == 1 && u.pkgs[0] == "main" {
		return "app"
	}
	if len(u.dirs) == 0 && len(u.entry) > 0 {
		return "app"
	}
	return ""
}

// affinity is the pairwise bond between units: imports normalised by both sides' degree (so
// a hub everyone imports does not glue everything), plus a role bonus and a shared-parent bonus.
func affinity(us []*unit, w [][]float64, roleBonus func(a, b string) float64) [][]float64 {
	n := len(us)
	deg := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			deg[i] += w[i][j] + w[j][i]
		}
	}
	a := make([][]float64, n)
	for i := range a {
		a[i] = make([]float64, n)
	}
	// shared neighbours: two units imported by (or importing) the same others belong together
	// even with no import between them; a neighbour's weight shrinks with its own degree
	nb := func(i, skip, k int) float64 {
		if k == i || k == skip || deg[k] == 0 {
			return 0
		}
		return (w[i][k] + w[k][i]) / math.Sqrt(deg[k])
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			v := 0.0
			if b := w[i][j] + w[j][i]; b > 0 {
				v = b / math.Sqrt(deg[i]*deg[j])
			}
			dot, ni, nj := 0.0, 0.0, 0.0
			for k := 0; k < n; k++ {
				x, y := nb(i, j, k), nb(j, i, k)
				dot, ni, nj = dot+x*y, ni+x*x, nj+y*y
			}
			if ni > 0 && nj > 0 {
				v += 0.5 * dot / math.Sqrt(ni*nj)
			}
			v += roleBonus(us[i].role, us[j].role)
			if pi, pj := parentOf(us[i]), parentOf(us[j]); pi != "" && pi == pj {
				v += 0.2
			}
			if us[i].goMod != us[j].goMod {
				v -= 0.3 // two go modules: separate builds
			}
			a[i][j], a[j][i] = v, v
		}
	}
	return a
}

func parentOf(u *unit) string {
	if len(u.dirs) == 0 {
		return ""
	}
	p := path.Dir(u.dirs[0])
	if p == "." {
		return ""
	}
	return p
}

// agglomerate groups items into at most target clusters of at most maxPer, merging the pair
// with the best average affinity first; pairs with no affinity merge smallest-first only to
// reach the target.
func agglomerate(n, target, maxPer int, aff [][]float64, size func(i int) int) [][]int {
	cl := make([][]int, n)
	for i := range cl {
		cl[i] = []int{i}
	}
	weight := func(c []int) int {
		t := 0
		for _, i := range c {
			t += size(i)
		}
		return t
	}
	avg := func(a, b []int) float64 {
		sum := 0.0
		for _, x := range a {
			for _, y := range b {
				sum += aff[x][y]
			}
		}
		return sum / float64(len(a)*len(b))
	}
	for {
		bi, bj, best := -1, -1, math.Inf(-1)
		for i := 0; i < len(cl); i++ {
			for j := i + 1; j < len(cl); j++ {
				if len(cl[i])+len(cl[j]) > maxPer {
					continue
				}
				if a := avg(cl[i], cl[j]); a > best {
					bi, bj, best = i, j, a
				}
			}
		}
		// past the target only a strong bond still merges
		if bi < 0 || (len(cl) <= target && best < strongBond) {
			break
		}
		if best <= 0 {
			// nothing binds: merge the two lightest that fit
			bi, bj = -1, -1
			order := make([]int, len(cl))
			for i := range order {
				order[i] = i
			}
			sort.SliceStable(order, func(a, b int) bool { return weight(cl[order[a]]) < weight(cl[order[b]]) })
			for x := 0; x < len(order) && bi < 0; x++ {
				for y := x + 1; y < len(order); y++ {
					if len(cl[order[x]])+len(cl[order[y]]) <= maxPer {
						bi, bj = order[x], order[y]
						break
					}
				}
			}
			if bi < 0 {
				break
			}
			if bi > bj {
				bi, bj = bj, bi
			}
		}
		cl[bi] = append(cl[bi], cl[bj]...)
		cl = append(cl[:bj], cl[bj+1:]...)
	}
	// a lone item joins the cluster it is bound to when there is room
	for i := 0; i < len(cl); i++ {
		if len(cl[i]) != 1 || len(cl) <= 1 {
			continue
		}
		bj, best := -1, 0.0
		for j := range cl {
			if j != i && len(cl[j]) < maxPer {
				if a := avg(cl[i], cl[j]); a > best {
					bj, best = j, a
				}
			}
		}
		if bj >= 0 && best >= weakBond {
			cl[bj] = append(cl[bj], cl[i]...)
			cl = append(cl[:i], cl[i+1:]...)
			i = -1
		}
	}
	for _, c := range cl {
		sort.Ints(c)
	}
	sort.SliceStable(cl, func(i, j int) bool { return weight(cl[i]) > weight(cl[j]) })
	return cl
}

// strongBond merges past the target; weakBond is enough for a lone item to join a cluster.
const (
	strongBond = 0.35
	weakBond   = 0.08
)

// The ontology's zone rule (onto/workspace.go derive, inZone): a member's Ownership entry derives
// only when it is a single segment (`app/`, `main.go`), sits under `src/` or the workspace dir, or
// starts with the member's own name — `agent-one/loop/` belongs to a seat named `agent-one` or
// `agent-one-<t>` where <t> appears in the rest of the path. A claim that does not derive is no
// claim at all (the gate sees no owner, housekeep rewires the bond), so every name the sketch
// gives is checked against the same rule (zoneOK), and a dir whose sub-dirs cannot be named so
// is not opened.

// zoneOK mirrors onto's inZone: does the entry derive as a claim of the member named short?
func zoneOK(short, entry string) bool {
	p := strings.TrimPrefix(entry, "./")
	for _, suf := range []string{"/**/*", "/**", "/*", "**", "*"} {
		p = strings.TrimSuffix(p, suf)
	}
	p = strings.ToLower(strings.TrimRight(p, "/"))
	head, tail := short, ""
	if i := strings.Index(short, "-"); i > 0 {
		head, tail = short[:i], short[i+1:]
	}
	i := strings.Index(p, "/")
	if i <= 0 {
		return true
	}
	first := p[:i]
	switch {
	case tail != "" && first == head && strings.Contains(strings.TrimPrefix(p, first)+"/", tail):
		return true
	case first == ".agent-one" || first == "src":
		return true
	case tail == "" && first == head:
		return true
	}
	return false
}

// nestRoot is the root segment that binds a nested claim's name ("" for a free claim: one
// segment, or under src/).
func nestRoot(dir string) string {
	p := strings.ToLower(strings.Trim(dir, "/"))
	i := strings.Index(p, "/")
	if i <= 0 {
		return ""
	}
	if r := p[:i]; r != "src" && r != ".agent-one" {
		return r
	}
	return ""
}

// rootNameable: a root a member's name can start with (one word, nothing slug would change).
func rootNameable(r string) bool {
	return r != "" && slug(r) == r && !strings.Contains(r, "-")
}

// tailsOf are the literal segments after the root a name's tail can be, the base first.
func tailsOf(dir string) []string {
	segs := strings.Split(strings.ToLower(strings.Trim(dir, "/")), "/")
	var out []string
	for i := len(segs) - 1; i >= 1; i-- {
		if s := segs[i]; s != "" && slug(s) == s {
			out = append(out, s)
		}
	}
	return out
}

// unitRoot is the one nested root a unit's dirs share ("" when all free); false when two roots mix.
func unitRoot(dirs []string) (string, bool) {
	root := ""
	for _, d := range dirs {
		if r := nestRoot(d); r != "" {
			if root != "" && r != root {
				return "", false
			}
			root = r
		}
	}
	return root, true
}

// nestedName is a name for a set of dirs under one root: root-<tail> with a tail every nested
// dir carries (or the root alone when allowed), "" when none fits.
func nestedName(root string, dirs []string, used map[string]bool, bareOK bool) string {
	if len(dirs) == 0 {
		return ""
	}
	for _, t := range tailsOf(dirs[0]) {
		n := root + "-" + t
		if used[n] {
			continue
		}
		ok := true
		for _, d := range dirs {
			if !zoneOK(n, d) {
				ok = false
				break
			}
		}
		if ok {
			return n
		}
	}
	if bareOK && !used[root] {
		return root
	}
	return ""
}

// cut is the whole team: units → zone workers, domain owners, coordinators, a service owner.
func (s *survey) cut(project string, prefix map[string]string, skills []skill) sketch {
	sk := sketch{Project: project, Lang: s.lang}
	us := s.mergeTiny(s.units())
	s.nameUnits(us, project)
	w := s.bonds(us)
	zones := make([]plan, len(us))
	for i, u := range us {
		zones[i] = s.zonePlan(u, us, w, prefix)
	}
	aff := affinity(us, w, func(a, b string) float64 {
		switch {
		case a != "" && a == b:
			return 0.5 // one role (the entry points, the interfaces, the config) is one domain
		case a != b && a != "tests" && b != "tests":
			return -0.15 // a role keeps to itself unless the imports bind hard
		}
		return 0
	})
	size := func(i int) int { return us[i].lines }
	// zone workers → domain owners. Free zone workers (top-level dirs, src/…, the root's files) are cut by imports,
	// role and naming; the zone workers under a nested root go to domain owners named from that root.
	type group struct {
		name string
		idx  []int
	}
	var free []int
	byRoot := map[string][]int{}
	var roots []string
	for i, u := range us {
		r, _ := unitRoot(u.dirs)
		if r == "" {
			free = append(free, i)
			continue
		}
		if byRoot[r] == nil {
			roots = append(roots, r)
		}
		byRoot[r] = append(byRoot[r], i)
	}
	sort.Strings(roots)
	used := map[string]bool{}
	var groups []group
	sub := func(idx []int) [][]int {
		a := make([][]float64, len(idx))
		for x := range idx {
			a[x] = make([]float64, len(idx))
			for y := range idx {
				a[x][y] = aff[idx[x]][idx[y]]
			}
		}
		cl := agglomerate(len(idx), (len(idx)+zonesPerDomain-1)/zonesPerDomain, maxPerDomain, a, func(x int) int { return size(idx[x]) })
		for _, c := range cl {
			for k := range c {
				c[k] = idx[c[k]]
			}
		}
		return cl
	}
	if len(us) <= zonesPerDomain && len(roots) <= 1 {
		all := make([]int, len(us))
		for i := range all {
			all[i] = i
		}
		name := project
		if len(roots) == 1 {
			name = roots[0]
		}
		used[name] = true
		groups = append(groups, group{name, all})
	} else {
		for _, r := range roots {
			idx := byRoot[r]
			if len(idx) <= maxPerDomain {
				used[r] = true
				groups = append(groups, group{r, idx})
				continue
			}
			// a big root splits only where a shared path word names each part
			var rest []int
			for _, c := range sub(idx) {
				var dirs []string
				for _, i := range c {
					dirs = append(dirs, us[i].dirs...)
				}
				if n := nestedName(r, dirs, used, false); n != "" && n != r {
					used[n] = true
					groups = append(groups, group{n, c})
				} else {
					rest = append(rest, c...)
				}
			}
			if len(rest) > 0 {
				used[r] = true
				sort.Ints(rest)
				groups = append(groups, group{r, rest})
			}
		}
		if len(free) > 0 {
			var cl [][]int
			if len(free) <= zonesPerDomain {
				cl = [][]int{free}
			} else {
				cl = sub(free)
			}
			for _, c := range cl {
				groups = append(groups, group{domainName(us, w, c, project, len(cl) == 1 && len(roots) == 0, used), c})
			}
		}
	}
	var domains []plan
	var domainUnits [][]int
	domainRoot := map[int]string{}
	for _, g := range groups {
		o := plan{Rank: "domain", Short: g.name}
		for _, i := range g.idx {
			sl := zones[i]
			sl.Parent = prefix["domain"] + g.name
			o.Zones = append(o.Zones, sl)
			o.Ownership = append(o.Ownership, sl.Ownership...)
		}
		o.Purpose = fmt.Sprintf("rules the %s domain (%s) and holds the gate over every landing there.", g.name, plainList(o.Ownership, 8))
		o.Verify = domainVerify(o.Zones, s)
		o.Invariants = domainInvariants(o, us, g.idx, prefix)
		if r, _ := unitRoot(o.Ownership); r != "" {
			domainRoot[len(domains)] = r
		}
		domains = append(domains, o)
		domainUnits = append(domainUnits, g.idx)
	}
	// domain owners → coordinators: one coordinator for up to three domain owners; lane coordinators and a design coordinator above that
	coordOf := func(short string) string { return prefix["coord"] + short }
	designVerify := s.projectVerify()
	if len(domains) <= domainsPerCoord {
		e := plan{Rank: "coord", Short: project, Parent: "orchestrator", Ownership: s.design, Verify: designVerify,
			Purpose: fmt.Sprintf("the shared skill and voice of %s: keeps the project-wide files and the cross-domain rules, routes work to its domain owners, speaks back up — never authors code.", filepath.Base(s.root))}
		for k := range domains {
			domains[k].Parent = coordOf(project)
			e.Domains = append(e.Domains, prefix["domain"]+domains[k].Short)
		}
		e.Invariants = coordInvariants(e, s, true)
		sk.Coords = []plan{e}
	} else {
		var freeDomains []int
		laneOf := map[string][]int{}
		var laneRoots []string
		for i := range domains {
			if r, ok := domainRoot[i]; ok {
				if laneOf[r] == nil {
					laneRoots = append(laneRoots, r)
				}
				laneOf[r] = append(laneOf[r], i)
				continue
			}
			freeDomains = append(freeDomains, i)
		}
		var lanes [][]int
		var laneNames []string
		lused := map[string]bool{"design": true}
		for _, r := range laneRoots {
			lused[r] = true
		}
		if len(freeDomains) > 0 {
			ow := make([][]float64, len(freeDomains))
			ou := make([]*unit, len(freeDomains))
			for x, i := range freeDomains {
				ow[x] = make([]float64, len(freeDomains))
				for y, j := range freeDomains {
					if i == j {
						continue
					}
					for _, a := range domainUnits[i] {
						for _, b := range domainUnits[j] {
							ow[x][y] += w[a][b]
						}
					}
				}
				ou[x] = &unit{role: laneRole(domains[i].Short), dirs: firstDirs(us, domainUnits[i]), goMod: us[domainUnits[i][0]].goMod}
			}
			la := affinity(ou, ow, func(a, b string) float64 {
				if a != "" && a == b {
					return 0.6
				}
				return 0
			})
			for _, l := range agglomerate(len(freeDomains), (len(freeDomains)+domainsPerCoord-1)/domainsPerCoord, domainsPerCoord, la, func(x int) int {
				t := 0
				for _, a := range domainUnits[freeDomains[x]] {
					t += us[a].lines
				}
				return t
			}) {
				var lane []int
				for _, x := range l {
					lane = append(lane, freeDomains[x])
				}
				lanes = append(lanes, lane)
				laneNames = append(laneNames, laneName(domains, lane, lused))
			}
		}
		for _, r := range laneRoots {
			lanes = append(lanes, laneOf[r])
			laneNames = append(laneNames, r)
		}
		for k, l := range lanes {
			name := laneNames[k]
			e := plan{Rank: "coord", Short: name, Parent: "orchestrator"}
			var names []string
			for _, i := range l {
				domains[i].Parent = coordOf(name)
				e.Ownership = append(e.Ownership, domains[i].Ownership...)
				e.Domains = append(e.Domains, prefix["domain"]+domains[i].Short)
				names = append(names, prefix["domain"]+domains[i].Short)
			}
			e.Purpose = fmt.Sprintf("the shared skill of the %s lane: plans an order across %s, routes each slice to the domain owner whose domain owns it, and speaks the lane's verdicts back up — never authors code.", name, joinAnd(names))
			e.Invariants = coordInvariants(e, s, false)
			sk.Coords = append(sk.Coords, e)
		}
		design := plan{Rank: "coord", Short: "design", Parent: "orchestrator", Ownership: s.design, Verify: designVerify,
			Purpose: fmt.Sprintf("the persistent agent of %s's shape and its written word: the manifests, the project-wide docs and the cross-lane rules. It routes nothing itself — the lane coordinators carry the domain owners — and is consulted when a change crosses lanes or touches the manifests and docs.", filepath.Base(s.root))}
		for _, e := range sk.Coords {
			design.Domains = append(design.Domains, coordOf(e.Short))
		}
		design.Invariants = coordInvariants(design, s, true)
		sk.Coords = append([]plan{design}, sk.Coords...)
	}
	// order domain owners by lane
	laneIdx := map[string]int{}
	for i, e := range sk.Coords {
		laneIdx[coordOf(e.Short)] = i
	}
	sort.SliceStable(domains, func(i, j int) bool { return laneIdx[domains[i].Parent] < laneIdx[domains[j].Parent] })
	sk.Domains = domains
	if len(s.infra) > 0 {
		name := "infra"
		purpose := "owns the infrastructure end to end, across sessions — the container and deploy files; anything that provisions or deploys waits for the human."
		switch {
		case len(s.ci) > 0 && s.release:
			name = "release"
			purpose = "owns CI, builds and releases end to end, across sessions; anything that tags, publishes or provisions waits for the human."
		case len(s.ci) > 0:
			name = "ci"
			purpose = "owns CI and the build end to end, across sessions; anything that publishes or provisions waits for the human."
		}
		k := plan{Rank: "service", Short: name, Parent: "orchestrator", Ownership: s.infra, Purpose: purpose}
		k.Invariants = s.serviceInvariants()
		sk.Service = &k
	}
	sk.applySkills(skills)
	return sk
}

func joinAnd(xs []string) string {
	if len(xs) <= 1 {
		return strings.Join(xs, "")
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

func firstDirs(us []*unit, g []int) []string {
	for _, i := range g {
		if len(us[i].dirs) > 0 {
			return us[i].dirs[:1]
		}
	}
	return nil
}

func laneRole(domain string) string {
	if strings.HasSuffix(domain, "app") || strings.HasSuffix(domain, "interfaces") {
		return "host"
	}
	return ""
}

func domainName(us []*unit, w [][]float64, g []int, project string, only bool, used map[string]bool) string {
	roles := map[string]int{}
	for _, i := range g {
		roles[us[i].role]++
	}
	name := ""
	for r, c := range roles {
		if r != "" && c*2 >= len(g) && (name == "" || r < name) {
			name = r
		}
	}
	if name == "" && only {
		name = project
	}
	if name == "" {
		// the most imported unit is the domain's core; the largest on a tie
		best, bw := g[0], -1.0
		for _, i := range g {
			in := 0.0
			for j := range us {
				if j != i {
					in += w[j][i]
				}
			}
			if in > bw || (in == bw && us[i].lines > us[best].lines) {
				best, bw = i, in
			}
		}
		name = us[best].name
	}
	if used[name] {
		if p := commonTop(us, g); p != "" && !used[p+"-"+name] {
			name = p + "-" + name
		}
	}
	n := name
	for k := 2; used[n]; k++ {
		n = fmt.Sprintf("%s-%d", name, k)
	}
	used[n] = true
	return n
}

func commonTop(us []*unit, g []int) string {
	top := ""
	for _, i := range g {
		if len(us[i].dirs) == 0 {
			return ""
		}
		t := strings.SplitN(us[i].dirs[0], "/", 2)[0]
		if top != "" && t != top {
			return ""
		}
		top = t
	}
	if !strings.Contains(us[g[0]].dirs[0], "/") {
		return ""
	}
	return slug(top)
}

func laneName(domains []plan, l []int, used map[string]bool) string {
	name := ""
	for _, i := range l {
		if laneRole(domains[i].Short) == "host" {
			name = "host"
			if p := strings.TrimSuffix(strings.TrimSuffix(domains[i].Short, "app"), "interfaces"); p != "" {
				name = strings.TrimSuffix(p, "-") + "-host"
			}
		}
	}
	if name == "" {
		name = domains[l[0]].Short
	}
	n := name
	for k := 2; used[n]; k++ {
		n = fmt.Sprintf("%s-%d", name, k)
	}
	used[n] = true
	return n
}

// plainList names ownership entries in prose, never backticked (a backticked path in prose is
// a claim the ontology may read), bounded.
func plainList(xs []string, max int) string {
	if len(xs) <= max {
		return strings.Join(xs, ", ")
	}
	return strings.Join(xs[:max], ", ") + fmt.Sprintf(" and %d more", len(xs)-max)
}

var langWord = map[string]string{"go": "Go", "node": "JS/TS", "python": "Python", "rust": "Rust", "jvm": "JVM", "ruby": "Ruby", "php": "PHP", "dotnet": ".NET", "swift": "Swift", "c": "C/C++", "elixir": "Elixir"}

// zonePlan writes a unit as a zone worker: ownership, purpose, the sketch's invariants, verify lines.
func (s *survey) zonePlan(u *unit, us []*unit, w [][]float64, prefix map[string]string) plan {
	p := plan{Rank: "zone", Short: u.name, Ownership: u.claims()}
	ticks := make([]string, len(p.Ownership))
	for i, t := range p.Ownership {
		ticks[i] = "`" + t + "`"
	}
	where := strings.Join(ticks, " and ")
	if len(ticks) > 2 {
		where = strings.Join(ticks[:len(ticks)-1], ", ") + " and " + ticks[len(ticks)-1]
	}
	what, readme := "", ""
	if len(u.dirs) > 0 {
		readme = readmeLine(filepath.Join(s.root, filepath.FromSlash(u.dirs[0])))
	}
	switch {
	case len(u.docs) > 0 && (len(u.pkgs) <= 2 || readme == ""):
		what = u.docs[0] // one package's own doc says best what it holds
	case u.mod != nil && u.mod.desc != "" && u.modDir == firstOr(u.dirs):
		what = u.mod.desc
	case readme != "":
		what = readme // a dir of many packages is said by its README
	}
	if what == "" {
		what = s.describe(u)
	}
	what = strings.TrimSuffix(what, ".")
	if !strings.HasSuffix(what, "…") {
		what += "."
	}
	p.Purpose = "the ground truth of " + where + ": " + what
	p.Invariants = s.zoneInvariants(u, us, w, prefix)
	p.Verify = s.unitVerify(u)
	return p
}

func firstOr(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}

// describe is a unit's honest one-liner when no doc says what it holds.
func (s *survey) describe(u *unit) string {
	lw := langWord[u.lang()]
	switch {
	case u.mod != nil && u.modDir == firstOr(u.dirs):
		return fmt.Sprintf("the %s module %s (%s)", lw, u.mod.name, u.mod.kind)
	case len(u.pkgs) > 0 && u.lang() == "go":
		return "Go package" + plural(len(u.pkgs)) + " " + strings.Join(u.pkgs, ", ")
	case u.role == "tests":
		return lw + " tests"
	}
	return lw + " source"
}

// readmeLine is the first prose line of a dir's README, when it has one.
func readmeLine(dir string) string {
	for _, n := range []string{"README.md", "README", "README.rst", "README.txt"} {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimSpace(l)
			if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "=") || strings.HasPrefix(l, "-") || strings.HasPrefix(l, "[") || strings.HasPrefix(l, "<") || strings.HasPrefix(l, "!") || strings.HasPrefix(l, "```") {
				continue
			}
			return firstSentence(strings.NewReplacer("`", "", "**", "", "__", "").Replace(l))
		}
	}
	return ""
}

func (s *survey) zoneInvariants(u *unit, us []*unit, w [][]float64, prefix map[string]string) []string {
	var t []string
	lw := langWord[u.lang()]
	what := lw + " source"
	if len(u.pkgs) > 0 && u.lang() == "go" {
		what = "Go package" + plural(len(u.pkgs)) + " " + plainList(u.pkgs, 8)
	}
	if u.mod != nil {
		what += fmt.Sprintf(" — in the %s module %s", u.mod.kind, u.mod.name)
	}
	t = append(t, fmt.Sprintf("%s: %d source file%s, %d test file%s, %s lines.", what, u.src, plural(u.src), u.tests, plural(u.tests), thousands(u.lines)))
	if u.exported > 0 {
		more := ""
		if u.exported > len(u.names) {
			more = fmt.Sprintf(" (%d exported names in all)", u.exported)
		}
		t = append(t, "Exports "+strings.Join(u.names, ", ")+more+".")
	}
	if len(u.entry) > 0 {
		t = append(t, "Entry point"+plural(len(u.entry))+": "+plainList(u.entry, 5)+".")
	}
	if u.mod != nil && len(u.mod.entry) > 0 {
		t = append(t, "Binaries declared in its package.json: "+strings.Join(u.mod.entry, ", ")+".")
	}
	i := indexOf(us, u)
	var out, in []string
	for j, v := range us {
		if j == i {
			continue
		}
		if w[i][j] > 0 {
			out = append(out, prefix["zone"]+v.name)
		}
		if w[j][i] > 0 {
			in = append(in, prefix["zone"]+v.name)
		}
	}
	if len(out) > 0 {
		t = append(t, "Imports from "+plainList(out, 6)+".")
	}
	if len(in) > 0 {
		t = append(t, "Imported by "+plainList(in, 6)+".")
	}
	if u.tests == 0 {
		t = append(t, "No tests found in this ownership — no verify line was written.")
	} else if len(s.unitVerify(u)) == 0 {
		t = append(t, "Tests exist here but no runner the sketch could name — no verify line was written.")
	}
	for k := range t {
		t[k] = "sketch: " + t[k]
	}
	return t
}

func indexOf(us []*unit, u *unit) int {
	for i, v := range us {
		if v == u {
			return i
		}
	}
	return -1
}

func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// unitVerify is the unit's test command, only when tests exist there and a runner is known.
func (s *survey) unitVerify(u *unit) []string {
	if u.tests == 0 {
		return nil
	}
	switch u.lang() {
	case "go":
		if u.goMod == "-" {
			// no module above: the go modules inside it prove it, each from its own root
			var mods []string
			for d := range s.goMods {
				for _, ud := range u.dirs {
					if strings.HasPrefix(d, ud+"/") && s.dirs[d] != nil && s.dirs[d].aggTests > 0 {
						mods = append(mods, "cd "+d+" && go test ./...")
					}
				}
			}
			sort.Strings(mods)
			return mods
		}
		var pk []string
		if len(u.dirs) == 0 {
			pk = append(pk, ".")
		}
		for _, d := range u.dirs {
			if di := s.dirs[d]; di == nil || di.aggTests == 0 {
				continue
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(d, u.goMod), "/")
			if u.goMod != "" && rel == d {
				continue
			}
			switch {
			case rel == "":
				pk = append(pk, "./...")
			case strings.HasPrefix(path.Base(rel), "_"):
				pk = append(pk, "./"+rel+"/") // ./... skips a leading underscore
			default:
				pk = append(pk, "./"+rel+"/...")
			}
		}
		if len(pk) == 0 {
			return nil
		}
		cmd := "go test " + strings.Join(pk, " ")
		if u.goMod != "" {
			cmd = "cd " + u.goMod + " && " + cmd
		}
		return []string{cmd}
	case "python":
		var ds []string
		for _, d := range u.dirs {
			if di := s.dirs[d]; di != nil && di.aggTests > 0 {
				ds = append(ds, d)
			}
		}
		if len(u.dirs) == 0 {
			for _, f := range u.files {
				if b := path.Base(f); strings.HasPrefix(b, "test_") || strings.HasSuffix(b, "_test.py") {
					ds = append(ds, f)
				}
			}
		}
		if len(ds) == 0 {
			return nil
		}
		return []string{"python -m pytest -q " + strings.Join(ds, " ")}
	case "node":
		if u.mod != nil && u.mod.kind == "npm" && u.mod.test != "" {
			return []string{u.mod.test}
		}
		if s.rootPkg.realTest() && len(u.dirs) > 0 {
			t := s.rootPkg.Scripts["test"]
			runner := ""
			switch {
			case strings.HasPrefix(t, "vitest"):
				runner = "npx vitest run "
			case strings.HasPrefix(t, "jest"):
				runner = "npx jest "
			}
			if runner != "" {
				return []string{runner + strings.Join(u.dirs, " ")}
			}
		}
	case "rust", "jvm":
		if u.mod != nil && u.mod.test != "" {
			return []string{u.mod.test}
		}
		if u.mod == nil {
			switch {
			case exists(filepath.Join(s.root, "Cargo.toml")) && u.lang() == "rust":
				return []string{"cargo test"}
			case exists(filepath.Join(s.root, "gradlew")):
				return []string{"./gradlew test"}
			case exists(filepath.Join(s.root, "pom.xml")):
				return []string{"mvn -q test"}
			}
		}
	}
	return nil
}

// projectVerify is the whole project's build/test line, for the coordinator that holds the manifests.
func (s *survey) projectVerify() []string {
	var v []string
	if _, ok := s.goMods[""]; ok && s.langs["go"] > 0 {
		v = append(v, "go build ./...", "go vet ./...")
	}
	if s.rootPkg.realTest() {
		v = append(v, map[string]string{"npm": "npm test", "pnpm": "pnpm test", "yarn": "yarn test"}[s.pm])
	}
	if exists(filepath.Join(s.root, "Cargo.toml")) && s.dirs[""].aggTests > 0 {
		v = append(v, "cargo test")
	}
	return v
}

// domainVerify is the union of the zone workers' lines (Go packages folded into one go test).
func domainVerify(sl []plan, s *survey) []string {
	goPk := map[string][]string{} // "cd mod && " (or "") → packages
	var cds []string
	var out []string
	for _, p := range sl {
		for _, v := range p.Verify {
			cd, rest := "", v
			if strings.HasPrefix(v, "cd ") {
				if i := strings.Index(v, " && "); i > 0 {
					cd, rest = v[:i+4], v[i+4:]
				}
			}
			if strings.HasPrefix(rest, "go test ") {
				if goPk[cd] == nil {
					cds = append(cds, cd)
				}
				goPk[cd] = append(goPk[cd], strings.Fields(strings.TrimPrefix(rest, "go test "))...)
				continue
			}
			if !contains(out, v) {
				out = append(out, v)
			}
		}
	}
	var gos []string
	for _, cd := range cds {
		gos = append(gos, cd+"go test "+strings.Join(goPk[cd], " "))
	}
	out = append(gos, out...)
	if len(out) == 0 && s.rootPkg.realTest() && s.lang == "node" {
		for _, p := range sl {
			if strings.Contains(strings.Join(p.Invariants, " "), "Tests exist here") {
				return []string{map[string]string{"npm": "npm test", "pnpm": "pnpm test", "yarn": "yarn test"}[s.pm]}
			}
		}
	}
	return out
}

func domainInvariants(o plan, us []*unit, g []int, prefix map[string]string) []string {
	var t []string
	var parts []string
	lines := 0
	for k, i := range g {
		parts = append(parts, fmt.Sprintf("%s (%s)", prefix["zone"]+us[i].name, plainList(o.Zones[k].Ownership, 3)))
		lines += us[i].lines
	}
	t = append(t, fmt.Sprintf("sketch: %d zone worker%s, %s lines: %s.", len(g), plural(len(g)), thousands(lines), strings.Join(parts, "; ")))
	if len(o.Verify) == 0 {
		t = append(t, "sketch: no zone worker here has a test the sketch could run — the gate has no verify line to prove a landing yet.")
	}
	return t
}

func coordInvariants(e plan, s *survey, design bool) []string {
	var t []string
	if len(e.Domains) > 0 {
		t = append(t, "sketch: carries "+strings.Join(e.Domains, ", ")+".")
	}
	if design {
		var ms []string
		for _, f := range e.Ownership {
			if f != "README.md" && !strings.HasSuffix(f, "/") && !strings.HasPrefix(f, "LICENSE") {
				ms = append(ms, f)
			}
		}
		if len(ms) > 0 {
			t = append(t, "sketch: build and project files at the root: "+strings.Join(ms, ", ")+".")
		}
		langs := make([]string, 0, len(s.langs))
		for l := range s.langs {
			langs = append(langs, l)
		}
		sort.Slice(langs, func(i, j int) bool { return s.langs[langs[i]] > s.langs[langs[j]] })
		var lw []string
		for _, l := range langs {
			lw = append(lw, fmt.Sprintf("%s %d", langWord[l], s.langs[l]))
		}
		if len(lw) > 0 {
			t = append(t, "sketch: source files by language — "+strings.Join(lw, ", ")+".")
		}
		if len(s.vendored) > 0 {
			t = append(t, "sketch: vendored, generated or build output skipped: "+strings.Join(s.vendored, ", ")+".")
		}
	}
	return t
}

func (s *survey) serviceInvariants() []string {
	var t []string
	if wf, err := os.ReadDir(filepath.Join(s.root, ".github", "workflows")); err == nil {
		var names []string
		for _, w := range wf {
			if !w.IsDir() {
				names = append(names, w.Name())
			}
		}
		if len(names) > 0 {
			t = append(t, "sketch: GitHub Actions workflows: "+strings.Join(names, ", ")+".")
		}
	}
	var rest []string
	for _, x := range s.infra {
		if !strings.HasPrefix(x, ".github") {
			rest = append(rest, x)
		}
	}
	if len(rest) > 0 {
		t = append(t, "sketch: holds "+strings.Join(rest, ", ")+".")
	}
	t = append(t, "sketch: anything that tags, publishes, deploys or provisions waits for the human.")
	return t
}

// skill is a skill in the workspace's base-skills, as the sketch matches it.
type skill struct{ name, desc string }

func readSkills(root, workspaceDir string) []skill {
	var out []skill
	for _, base := range []string{"base-skills", "crafted-skills"} {
		ents, _ := os.ReadDir(filepath.Join(root, workspaceDir, base))
		for _, e := range ents {
			b, err := os.ReadFile(filepath.Join(root, workspaceDir, base, e.Name(), "SKILL.md"))
			if !e.IsDir() || err != nil {
				continue
			}
			m := skill{name: e.Name()}
			for _, l := range strings.Split(string(b), "\n") {
				if d, ok := strings.CutPrefix(l, "description:"); ok {
					m.desc = strings.ToLower(strings.TrimSpace(d))
				}
			}
			out = append(out, m)
		}
	}
	return out
}

var langTokens = map[string][]string{"go": {"go", "golang"}, "python": {"python", "pytest"}, "node": {"node", "javascript", "typescript", "npm", "js", "ts"}, "rust": {"rust", "cargo"}, "jvm": {"java", "kotlin", "maven", "gradle"}}

// applySkills gives a member the skills that fit it: a rank-named skill (domain-…, zone-…) whose
// words name the member's language or ownership, or name no language at all; any skill whose
// name or description names one of the member's ownership dirs. At most three.
func (sk *sketch) applySkills(ms []skill) {
	if len(ms) == 0 {
		return
	}
	fit := func(p *plan) {
		words := map[string]bool{}
		for _, t := range p.Ownership {
			for _, seg := range strings.Split(strings.Trim(t, "/"), "/") {
				words[strings.ToLower(seg)] = true
			}
		}
		for _, m := range ms {
			if len(p.Skills) >= 3 {
				break
			}
			text := m.name + " " + m.desc
			ranked := strings.HasPrefix(m.name, p.Rank+"-")
			names := false
			for w := range words {
				if len(w) > 2 && wordInText(text, w) {
					names = true
				}
			}
			anyLang, ownLang := false, false
			for l, toks := range langTokens {
				for _, tk := range toks {
					if wordInText(text, tk) {
						anyLang = true
						if l == sk.Lang {
							ownLang = true
						}
					}
				}
			}
			if (ranked && (names || ownLang || !anyLang)) || (!ranked && names && !strings.HasPrefix(m.name, "domain-") && !strings.HasPrefix(m.name, "zone-") && !strings.HasPrefix(m.name, "coord-") && !strings.HasPrefix(m.name, "service-")) {
				p.Skills = append(p.Skills, m.name)
			}
		}
	}
	for i := range sk.Coords {
		fit(&sk.Coords[i])
	}
	for i := range sk.Domains {
		fit(&sk.Domains[i])
		for j := range sk.Domains[i].Zones {
			fit(&sk.Domains[i].Zones[j])
		}
	}
	if sk.Service != nil {
		fit(sk.Service)
	}
}

func wordInText(text, w string) bool {
	text = strings.ToLower(text)
	for i := 0; ; {
		j := strings.Index(text[i:], w)
		if j < 0 {
			return false
		}
		s, e := i+j, i+j+len(w)
		isW := func(b byte) bool { return b == '_' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') }
		if (s == 0 || !isW(text[s-1])) && (e == len(text) || !isW(text[e])) {
			return true
		}
		i = s + 1
	}
}
