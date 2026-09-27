package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding is one lint result.
type Finding struct {
	Level string `json:"level"` // error | warn
	File  string `json:"file"`
	Line  int    `json:"line,omitempty"`
	Rule  string `json:"rule"`
	Msg   string `json:"msg"`
}

func (f Finding) String() string {
	at := f.File
	if f.Line > 0 {
		at = fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return fmt.Sprintf("%s %s [%s] %s", f.Level, at, f.Rule, f.Msg)
}

// TokenFiles are the files where literal values belong: the raw tier and its palettes.
var TokenFiles = map[string]bool{"tokens.css": true, "palettes.css": true}

var (
	reColour = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch)\(`)
	rePx     = regexp.MustCompile(`(?:^|[^\w.-])(\d*\.?\d+)px\b`)
)

// Lint checks the system: every component has a true header, consumes only tokens that exist,
// uses only declared classes, and no file outside the token files holds a literal colour or a
// pixel length (1px and 2px hairlines, and lengths in @media/@container queries, excepted).
func Lint(root string, m *Manifest) []Finding {
	var out []Finding
	add := func(level, file string, line int, rule, msg string, a ...any) {
		out = append(out, Finding{Level: level, File: file, Line: line, Rule: rule, Msg: fmt.Sprintf(msg, a...)})
	}
	tokens := setOf(m.Tokens)
	classes := map[string]bool{}
	for _, g := range m.Globals {
		for _, c := range g.Classes {
			classes[c] = true
		}
	}
	for _, c := range m.Components {
		for _, k := range c.Classes {
			classes[k] = true
		}
	}
	for _, c := range m.Components {
		if c.CSS == "" {
			add("error", c.HTML, 0, "header", "%s has no %s.css — a component carries its own styles and its header", c.Name, c.Name)
		} else {
			src, _ := os.ReadFile(filepath.Join(root, c.CSS))
			if !reHeader.Match(src) {
				add("error", c.CSS, 1, "header", "no header — the file starts `/* @component %s — tokens: %s. <what it is> */`", c.Name, strings.Join(c.Consumes, ", "))
			} else {
				own := setOf(c.Own)
				var missing, extra []string
				for _, t := range c.Consumes {
					if !contains(c.Header, t) && !own[t] {
						missing = append(missing, t)
					}
				}
				for _, t := range c.Header {
					if !contains(c.Consumes, t) {
						extra = append(extra, t)
					}
				}
				if len(missing) > 0 {
					add("error", c.CSS, 1, "header", "the header does not list %s, which the styles read", strings.Join(missing, ", "))
				}
				if len(extra) > 0 {
					add("error", c.CSS, 1, "header", "the header lists %s, which the styles no longer read", strings.Join(extra, ", "))
				}
			}
			own := setOf(c.Own)
			for _, t := range c.Consumes {
				if !tokens[t] && !own[t] {
					add("error", c.CSS, lineOf(src, "var("+t), "token", "%s is read but no token file defines it", t)
				}
			}
			for _, id := range c.IDs {
				add("warn", c.CSS, lineOf(src, "#"+id), "id", "#%s styled in a reusable piece — ids are one per page; use a class or a data- attribute", id)
			}
		}
		if c.HTML != "" {
			src, _ := os.ReadFile(filepath.Join(root, c.HTML))
			for _, k := range c.Uses {
				if !classes[k] {
					add("error", c.HTML, lineOf(src, k), "class", ".%s is used but no css in %s declares it", k, m.Dir)
				}
			}
		}
	}
	for _, f := range cssFiles(root, m) {
		if TokenFiles[filepath.Base(f)] {
			continue
		}
		out = append(out, literals(root, f)...)
	}
	return out
}

// literals finds literal colours and pixel lengths in one css file.
func literals(root, rel string) []Finding {
	src, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil
	}
	body := blank(string(src))
	var out []Finding
	for i, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "@media") || strings.HasPrefix(t, "@container") || strings.HasPrefix(t, "@import") {
			continue
		}
		if strings.Contains(t, "--") && strings.Index(t, "--") < strings.Index(t+":", ":") {
			// a custom property defined here is the component tier: it may alias a token, not
			// hold a literal colour — lengths in it are still judged below
			if reColour.MatchString(t) {
				out = append(out, Finding{"error", rel, i + 1, "literal", "a literal colour in a component token — alias a meaning token instead: " + t})
			}
		} else if reColour.MatchString(t) {
			out = append(out, Finding{"error", rel, i + 1, "literal", "a literal colour — use a token (var(--…)): " + t})
		}
		for _, m := range rePx.FindAllStringSubmatch(t, -1) {
			if m[1] == "0" || m[1] == "1" || m[1] == "2" {
				continue
			}
			out = append(out, Finding{"error", rel, i + 1, "literal", "a pixel length — use a spacing or size token, rem, or a fluid clamp(): " + t})
			break
		}
	}
	return out
}

// blank replaces comments, strings and url() with spaces, keeping line numbers.
func blank(s string) string {
	sp := func(m string) string {
		return strings.Map(func(r rune) rune {
			if r == '\n' {
				return r
			}
			return ' '
		}, m)
	}
	s = reComment.ReplaceAllStringFunc(s, sp)
	s = reURL.ReplaceAllStringFunc(s, sp)
	return reString.ReplaceAllStringFunc(s, sp)
}

func cssFiles(root string, m *Manifest) []string {
	var out []string
	for _, g := range m.Globals {
		out = append(out, g.File)
	}
	for _, c := range m.Components {
		if c.CSS != "" {
			out = append(out, c.CSS)
		}
	}
	return out
}

func lineOf(src []byte, needle string) int {
	for i, l := range strings.Split(string(src), "\n") {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	return 0
}

func setOf(in []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range in {
		m[s] = true
	}
	return m
}

func contains(in []string, s string) bool {
	for _, x := range in {
		if x == s {
			return true
		}
	}
	return false
}

// Errors counts the error-level findings.
func Errors(fs []Finding) int {
	n := 0
	for _, f := range fs {
		if f.Level == "error" {
			n++
		}
	}
	return n
}
