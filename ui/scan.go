// Package ui is the workspace's instrument for web pages built from a component system: the app's
// own ui/ directory (tokens.css, layout.css, components/<name>/{<name>.html,<name>.css}) is the
// source of truth; the legend — manifest.json, legend.md, catalogue.html under
// <workspace>/ui-assets/ — is derived from it by scan, never written by hand, so it cannot drift.
// check lints the system (tokens only, every class declared, every header true) and renders the
// catalogue at three widths in both themes (README, Web UIs).
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Manifest is the legend's data: what every component declares, uses and consumes.
type Manifest struct {
	Version    int         `json:"version"`
	Dir        string      `json:"dir"` // the ui dir, relative to the root
	Tokens     []string    `json:"tokens"`
	Globals    []Global    `json:"globals"`
	Components []Component `json:"components"`
}

// Global is a system file outside components/ (tokens.css, layout.css, palettes.css, …).
type Global struct {
	File    string   `json:"file"`
	Classes []string `json:"classes,omitempty"`
	Tokens  []string `json:"tokens,omitempty"` // custom properties it defines
}

// Component is one components/<name>/ directory.
type Component struct {
	Name     string   `json:"name"`
	HTML     string   `json:"html,omitempty"`
	CSS      string   `json:"css,omitempty"`
	Summary  string   `json:"summary,omitempty"` // the header's prose, after the tokens list
	Header   []string `json:"header"`            // the tokens the header declares
	Classes  []string `json:"classes"`           // classes its CSS defines
	IDs      []string `json:"ids,omitempty"`     // ids its CSS styles (a smell in a reusable piece)
	Variants []string `json:"variants,omitempty"`
	Consumes []string `json:"consumes"`       // system tokens its CSS reads (var(--x) not its own)
	Own      []string `json:"own,omitempty"`  // custom properties it defines itself
	Uses     []string `json:"uses,omitempty"` // classes its HTML uses
}

// Candidates are where a ui dir is looked for, in order, when none is named.
var Candidates = []string{"ui", "src/ui", "web/ui", "app/ui", "static/ui", "public/ui", "frontend/ui"}

// FindDir is the first candidate holding tokens.css or components/; "" when none does.
func FindDir(root string) string {
	for _, c := range Candidates {
		for _, probe := range []string{"tokens.css", "components"} {
			if _, err := os.Stat(filepath.Join(root, c, probe)); err == nil {
				return c
			}
		}
	}
	return ""
}

var (
	reComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reHeader   = regexp.MustCompile(`(?s)/\*\s*@component\s+([\w-]+)(.*?)\*/`)
	reClassSel = regexp.MustCompile(`\.(-?[_a-zA-Z][\w-]*)`)
	reIDSel    = regexp.MustCompile(`#(-?[_a-zA-Z][\w-]*)`)
	reVariant  = regexp.MustCompile(`\[(data-[\w-]+)(?:[~|^$*]?=\s*"?([\w-]+)"?)?\]`)
	reVarUse   = regexp.MustCompile(`var\(\s*(--[\w-]+)`)
	reVarDef   = regexp.MustCompile(`(?:^|[;{\s])(--[\w-]+)\s*:`)
	reTokens   = regexp.MustCompile(`--[\w-]+`)
	reClassAt  = regexp.MustCompile(`\bclass\s*=\s*["']([^"']*)["']`)
	reURL      = regexp.MustCompile(`url\([^)]*\)`)
	reString   = regexp.MustCompile(`"[^"]*"|'[^']*'`)
)

// Scan reads the ui dir under root into a manifest.
func Scan(root, dir string) (*Manifest, error) {
	base := filepath.Join(root, dir)
	if st, err := os.Stat(base); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("no ui dir at %s", dir)
	}
	m := &Manifest{Version: 1, Dir: filepath.ToSlash(dir)}
	tokens := map[string]bool{}
	globs, _ := filepath.Glob(filepath.Join(base, "*.css"))
	sort.Strings(globs)
	for _, p := range globs {
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		body := reComment.ReplaceAllString(string(src), "")
		g := Global{File: rel(root, p), Classes: selectors(body, reClassSel), Tokens: defs(body)}
		for _, t := range g.Tokens {
			tokens[t] = true
		}
		m.Globals = append(m.Globals, g)
	}
	m.Tokens = keys(tokens)
	ents, _ := os.ReadDir(filepath.Join(base, "components"))
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		c, err := scanComponent(root, filepath.Join(base, "components", e.Name()), e.Name())
		if err != nil {
			return nil, err
		}
		m.Components = append(m.Components, c)
	}
	return m, nil
}

func scanComponent(root, dir, name string) (Component, error) {
	c := Component{Name: name, Header: []string{}, Classes: []string{}, Consumes: []string{}}
	cssPath, htmlPath := filepath.Join(dir, name+".css"), filepath.Join(dir, name+".html")
	if src, err := os.ReadFile(cssPath); err == nil {
		c.CSS = rel(root, cssPath)
		if h := reHeader.FindStringSubmatch(string(src)); h != nil {
			c.Header, c.Summary = parseHeader(h[2])
		}
		body := reComment.ReplaceAllString(string(src), "")
		c.Classes = selectors(body, reClassSel)
		c.IDs = selectors(body, reIDSel)
		c.Variants = variants(body)
		c.Own = defs(body)
		own := setOf(c.Own)
		c.Consumes = []string{}
		for _, t := range uniq(submatches(body, reVarUse)) {
			if !own[t] {
				c.Consumes = append(c.Consumes, t) // what it reads from the system; its own aliases are listed apart
			}
		}
	}
	if src, err := os.ReadFile(htmlPath); err == nil {
		c.HTML = rel(root, htmlPath)
		c.Uses = htmlClasses(string(src))
	}
	if c.CSS == "" && c.HTML == "" {
		return c, fmt.Errorf("%s: a component dir holds %s.html and/or %s.css", rel(root, dir), name, name)
	}
	return c, nil
}

// parseHeader reads "— tokens: --a, --b. What it is." into the token list and the summary.
func parseHeader(s string) ([]string, string) {
	s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "—-–:"))
	toks := []string{}
	summary := s
	if i := strings.Index(s, "tokens:"); i >= 0 {
		rest := s[i+len("tokens:"):]
		end := len(rest)
		// the list ends at the first sentence stop or line break after it
		for j, r := range rest {
			if r == '\n' || r == '.' || r == '—' {
				end = j
				break
			}
		}
		toks = uniq(reTokens.FindAllString(rest[:end], -1))
		summary = strings.TrimSpace(strings.Trim(strings.TrimSpace(s[:i]+" "+rest[min(end+1, len(rest)):]), "—-–.:"))
	}
	return toks, strings.Join(strings.Fields(summary), " ")
}

// selectors are the names a CSS body uses in selectors (not in declaration values).
func selectors(body string, re *regexp.Regexp) []string {
	var out []string
	for _, sel := range preludes(body) {
		sel = reString.ReplaceAllString(sel, "")
		if re == reIDSel && strings.HasPrefix(strings.TrimSpace(sel), "@") {
			continue
		}
		out = append(out, submatches(sel, re)...)
	}
	return uniq(out)
}

// preludes are the texts before each "{": selectors and at-rule preludes.
func preludes(body string) []string {
	var out []string
	start := 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '{':
			pre := body[start:i]
			if j := strings.LastIndexAny(pre, ";}"); j >= 0 {
				pre = pre[j+1:]
			}
			out = append(out, pre)
			start = i + 1
		case '}', ';':
			start = i + 1
		}
	}
	return out
}

func variants(body string) []string {
	var out []string
	for _, sel := range preludes(body) {
		for _, m := range reVariant.FindAllStringSubmatch(sel, -1) {
			if m[2] != "" {
				out = append(out, m[1]+"="+m[2])
			} else {
				out = append(out, m[1])
			}
		}
	}
	return uniq(out)
}

func defs(body string) []string {
	return uniq(submatches(body, reVarDef))
}

func htmlClasses(src string) []string {
	var out []string
	for _, m := range reClassAt.FindAllStringSubmatch(src, -1) {
		out = append(out, strings.Fields(m[1])...)
	}
	return uniq(out)
}

func submatches(s string, re *regexp.Regexp) []string {
	var out []string
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

func uniq(in []string) []string {
	set := map[string]bool{}
	for _, s := range in {
		set[s] = true
	}
	return keys(set)
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func rel(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(p)
}

// JSON is the manifest as written: stable, indented, newline-terminated.
func (m *Manifest) JSON() []byte {
	b, _ := json.MarshalIndent(m, "", "  ")
	return append(b, '\n')
}
