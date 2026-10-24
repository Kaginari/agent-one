package onto

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Finding is one shape violation, pointed at the doc that owns it.
type Finding struct {
	Shape   string
	Subject Term
	Doc     string // file:line when known
	Msg     string
}

func (f Finding) String() string {
	if f.Doc != "" {
		return f.Doc + ": " + f.Msg + " (" + f.Shape + ")"
	}
	return f.Msg + " (" + f.Shape + ")"
}

// Validate applies every ao:Shape in the graph and returns the findings, sorted.
func (w *Workspace) Validate() []Finding {
	g := w.Graph
	var out []Finding
	for _, shape := range g.Instances(cShape) {
		on := first(g.Objects(shape, pOn))
		prop := first(g.Objects(shape, pProperty))
		if on == nil || prop == nil {
			continue
		}
		name := shape.Local()
		min, hasMin := intOf(g, shape, pMinCount)
		max, hasMax := intOf(g, shape, pMaxCount)
		disjoint := boolOf(g, shape, pDisjoint)
		valuesHave := first(g.Objects(shape, pValuesHave))
		unless := first(g.Objects(shape, pUnless))
		insts := g.Instances(*on)
		var owners []Term
		var paths []string
		for _, x := range insts {
			if unless != nil && boolOf(g, x, *unless) {
				continue
			}
			vals := g.Objects(x, *prop)
			n := len(vals)
			switch {
			case hasMin && hasMax && min == max && n != min:
				out = append(out, w.finding(name, x, prop.Local(), fmt.Sprintf("%s has %d %s edge%s, wants exactly %d", x.Local(), n, prop.Local(), plural(n), min)))
			case hasMin && n < min:
				out = append(out, w.finding(name, x, prop.Local(), fmt.Sprintf("%s has %d %s edge%s, wants at least %d", x.Local(), n, prop.Local(), plural(n), min)))
			case hasMax && n > max:
				out = append(out, w.finding(name, x, prop.Local(), fmt.Sprintf("%s has %d %s edge%s, wants at most %d", x.Local(), n, prop.Local(), plural(n), max)))
			}
			if valuesHave != nil {
				for _, v := range vals {
					if v.Kind == Literal || len(g.Objects(v, *valuesHave)) == 0 {
						out = append(out, w.finding(name, x, prop.Local(), fmt.Sprintf("%s %s %s, which has no %s", x.Local(), prop.Local(), v.Local(), valuesHave.Local())))
					}
				}
			}
			if disjoint {
				for _, v := range vals {
					owners = append(owners, x)
					paths = append(paths, v.Value)
				}
			}
		}
		if disjoint {
			for i := range paths {
				for j := i + 1; j < len(paths); j++ {
					if owners[i] == owners[j] || !w.overlap(paths[i], paths[j]) {
						continue
					}
					f := w.finding(name, owners[i], "ownership", fmt.Sprintf("%s owns %s%s, overlapping %s of %s%s", owners[i].Local(), paths[i], w.claimNote(owners[i], paths[i]), paths[j], owners[j].Local(), w.claimNote(owners[j], paths[j])))
					if cl, ok := w.ClaimOf(owners[i], paths[i]); ok {
						f.Doc = fmt.Sprintf("%s:%d", w.Docs[owners[i]].Path, cl.Line)
					}
					out = append(out, f)
				}
			}
		}
	}
	out = append(out, w.orphans()...)
	sortFindings(out)
	return out
}

// orphans is the HeirLive check: a living member whose one hop up is a retired seat that names
// no live heir has a line that ends nowhere — its doc must name a new one hop up.
func (w *Workspace) orphans() []Finding {
	g := w.Graph
	var out []Finding
	for _, x := range g.Instances(cMember) {
		if g.Has(Triple{x, pRetired, Bool(true)}) {
			continue
		}
		seen := map[Term]bool{}
		for _, p := range bondsByHop {
			for _, o := range g.Objects(x, p) {
				if seen[o] || !g.Has(Triple{o, pRetired, Bool(true)}) || !OneHop(g, x, o) || w.HeirOf(o.Local()) != "" {
					continue
				}
				seen[o] = true
				out = append(out, w.finding("HeirLive", x, p.Local(), fmt.Sprintf("%s reports to %s, a retired seat that names no live heir — name its new one hop up", x.Local(), o.Local())))
			}
		}
	}
	return out
}

// OneHop is true when no third member sits between s and o on the above chain.
func OneHop(g *Graph, s, o Term) bool {
	for _, mid := range g.Objects(s, pAbove) {
		if mid != o && g.Has(Triple{mid, pAbove, o}) {
			return false
		}
	}
	return true
}

// ClaimOf is the provenance of x's owns edge on path: the lane that registered it and its line.
func (w *Workspace) ClaimOf(x Term, path string) (Claim, bool) {
	if d, ok := w.Docs[x]; ok && d.Claims != nil {
		c, ok := d.Claims[path]
		return c, ok
	}
	return Claim{}, false
}

// claimNote renders a claim's provenance for a message: ` (prose, line 12)`, "" when unknown.
func (w *Workspace) claimNote(x Term, path string) string {
	if c, ok := w.ClaimOf(x, path); ok {
		return " (" + c.String() + ")"
	}
	return ""
}

func (w *Workspace) finding(shape string, x Term, field, msg string) Finding {
	f := Finding{Shape: shape, Subject: x, Msg: msg}
	if d, ok := w.Docs[x]; ok {
		line := 1
		switch field {
		case "truth", "verdict", "reports", "above":
			if l, ok := d.Lines["parent"]; ok {
				line = l
			}
		case "ownership", "owns":
			if l, ok := d.Lines["ownership"]; ok {
				line = l
			}
		}
		f.Doc = fmt.Sprintf("%s:%d", d.Path, line)
	} else if docs := w.Graph.Objects(x, pDoc); len(docs) > 0 {
		if p := w.Graph.Objects(docs[0], pPath); len(p) > 0 {
			f.Doc = p[0].Value + ":1"
		}
	}
	return f
}

// overlap reports a real ownership collision. Two members asserting the SAME file literal
// (a == b) is NOT a collision: it is a lawful sub-file split — e.g. zone-db-schema owns the
// tables/migrate() of store.go while zone-db-queries owns its query/scan functions, one file,
// disjoint responsibility (Principle 2). The same DIRECTORY claimed twice ao: two seats over one
// whole tree is one zone with two persistent agents. A true collision is also namespace nesting: one path
// a strict ancestor directory of the other (src/auth vs src/auth/login).
func (w *Workspace) overlap(a, b string) bool {
	if a == b {
		fi, err := os.Stat(filepath.Join(w.Root, filepath.FromSlash(a)))
		return err == nil && fi.IsDir()
	}
	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func first(ts []Term) *Term {
	if len(ts) == 0 {
		return nil
	}
	return &ts[0]
}

func intOf(g *Graph, s, p Term) (int, bool) {
	if t := first(g.Objects(s, p)); t != nil {
		return t.AsInt()
	}
	return 0, false
}

func boolOf(g *Graph, s, p Term) bool {
	t := first(g.Objects(s, p))
	return t != nil && t.Value == "true"
}

func sortFindings(fs []Finding) {
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].Shape != fs[j].Shape {
			return fs[i].Shape < fs[j].Shape
		}
		if fs[i].Doc != fs[j].Doc {
			return fs[i].Doc < fs[j].Doc
		}
		return fs[i].Msg < fs[j].Msg
	})
}
