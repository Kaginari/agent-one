package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seeded(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := Init(root, "ui"); err != nil {
		t.Fatal(err)
	}
	return root
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFoundationIsClean(t *testing.T) {
	root := seeded(t)
	if d := FindDir(root); d != "ui" {
		t.Fatalf("FindDir = %q", d)
	}
	m, err := Scan(root, "ui")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Components) != 2 {
		t.Fatalf("components = %d, want button and card", len(m.Components))
	}
	if fs := Lint(root, m); len(fs) > 0 {
		t.Fatalf("the shipped foundation must lint clean:\n%v", fs)
	}
	for _, want := range []string{"--surface", "--ink", "--accent", "--space-m", "--step-1", "--n-9", "--p-1-dark"} {
		if !contains(m.Tokens, want) {
			t.Errorf("token %s missing from the system", want)
		}
	}
	b := m.Components[0]
	if b.Name != "button" || !contains(b.Variants, "data-variant=primary") || contains(b.Consumes, "--button-bg") || !contains(b.Own, "--button-bg") {
		t.Errorf("button scanned wrong: %+v", b)
	}
}

func TestLintCatchesDrift(t *testing.T) {
	root := seeded(t)
	write(t, root, "ui/components/badge/badge.css", `/* @component badge — tokens: --ink, --gone. A small label. */
@layer components {
  .badge { color: var(--ink); background: #ff0066; padding: 3px 6px; }
  .badge { border: 1px solid var(--line); }
  .badge[data-tone="warn"] { color: var(--nope); }
  #main-badge { margin: 0; }
  @media (min-width: 600px) { .badge { font-size: var(--step-0); } }
}
`)
	write(t, root, "ui/components/badge/badge.html", `<span class="badge badge-dot">New</span>`)
	write(t, root, "ui/components/tag/tag.css", `.tag { color: var(--ink); }`)
	m, err := Scan(root, "ui")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range Lint(root, m) {
		got = append(got, f.String())
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{
		"does not list --line, --nope, --step-0",   // header missing what the styles read
		"lists --gone, which the styles no longer", // header lists a dead token
		"--nope is read but no token file",         // unknown token
		"#main-badge styled in a reusable piece",   // id
		".badge-dot is used but no css",            // undeclared class
		"a literal colour",                         // #ff0066
		"a pixel length",                           // 3px 6px
		"tag/tag.css:1 [header] no header",         // tag has no header
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing finding %q in:\n%s", want, all)
		}
	}
	if strings.Contains(all, "600px") {
		t.Errorf("a media query length is not a literal:\n%s", all)
	}
	if strings.Contains(all, "badge.css:4 [literal]") {
		t.Errorf("a 1px hairline is allowed:\n%s", all)
	}
}

func TestLegendIsDerivedAndStaleness(t *testing.T) {
	root := seeded(t)
	m, _ := Scan(root, "ui")
	changed, err := Write(root, ".agent-one", m)
	if err != nil || len(changed) != 3 {
		t.Fatalf("Write = %v, %v", changed, err)
	}
	if s := Stale(root, ".agent-one", m); len(s) != 0 {
		t.Fatalf("fresh legend reported stale: %v", s)
	}
	legend, _ := os.ReadFile(filepath.Join(root, ".agent-one", AssetsDir, "legend.md"))
	for _, want := range []string{"`button`", "`.card-title`", "data-variant=raised", "never edit it"} {
		if !strings.Contains(string(legend), want) {
			t.Errorf("legend lacks %q", want)
		}
	}
	cat, _ := os.ReadFile(filepath.Join(root, ".agent-one", AssetsDir, "catalogue.html"))
	if !strings.Contains(string(cat), `href="../../ui/tokens.css"`) || !strings.Contains(string(cat), "<option>forest</option>") {
		t.Errorf("catalogue links or palettes wrong")
	}
	write(t, root, "ui/components/card/card.css", "/* @component card — tokens: --ink. Changed. */\n.card { color: var(--ink); }\n")
	m2, _ := Scan(root, "ui")
	if s := Stale(root, ".agent-one", m2); len(s) == 0 {
		t.Fatal("a changed component must make the legend stale")
	}
	if again, _ := Write(root, ".agent-one", m2); len(again) == 0 {
		t.Fatal("rewrite reported no change")
	}
}

func TestInputsAndDataClass(t *testing.T) {
	root := seeded(t)
	write(t, root, "ui/components/meter/meter.css", "/* @component meter — tokens: --ink. A reading. */\n.meter { color: var(--ink); inline-size: var(--meter-value, 0%); }\n")
	write(t, root, "ui/components/meter/meter.html", `<span class="meter" data-class="read" style="--meter-value: 4%"></span>`)
	m, _ := Scan(root, "ui")
	if fs := Lint(root, m); len(fs) > 0 {
		t.Fatalf("an input and a data-class attribute are not findings: %v", fs)
	}
	for _, c := range m.Components {
		if c.Name == "meter" && (len(c.Inputs) != 1 || c.Inputs[0] != "--meter-value" || len(c.Uses) != 1) {
			t.Fatalf("meter scanned wrong: %+v", c)
		}
	}
}

func TestParseHeader(t *testing.T) {
	toks, sum := parseHeader(" — tokens: --a, --b-c, --step--1. What it is; data-variant=\"x\". ")
	if strings.Join(toks, " ") != "--a --b-c --step--1" || sum != `What it is; data-variant="x"` {
		t.Fatalf("got %v %q", toks, sum)
	}
}

// TestShoot renders the catalogue when a browser is at hand (UI_SHOTS=1).
func TestShoot(t *testing.T) {
	b := Browser(os.Getenv)
	if os.Getenv("UI_SHOTS") == "" || b == "" {
		t.Skip("UI_SHOTS=1 and a Chromium render the catalogue")
	}
	root := seeded(t)
	write(t, root, "ui/components/wide/wide.css", "/* @component wide — tokens: --ink. Too wide. */\n.wide { inline-size: 60rem; color: var(--ink); }\n")
	write(t, root, "ui/components/wide/wide.html", `<div class="wide">wide</div>`)
	m, _ := Scan(root, "ui")
	if _, err := Write(root, ".agent-one", m); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	shots, err := Shoot(context.Background(), b, root, ".agent-one", m, home)
	if err != nil || len(shots) != len(Widths)*len(Themes) {
		t.Fatalf("Shoot = %d shots, %v", len(shots), err)
	}
	for _, s := range shots {
		if s.Err != "" {
			t.Errorf("%dpx %s: %s", s.Width, s.Theme, s.Err)
		}
		if s.Width < 1000 && !strings.Contains(s.Overflow, "wide") {
			t.Errorf("%dpx: overflow %q, want wide", s.Width, s.Overflow)
		}
		if strings.Contains(s.Overflow, "card") || strings.Contains(s.Overflow, "button") {
			t.Errorf("%dpx: the foundation overflows: %q", s.Width, s.Overflow)
		}
	}
}
