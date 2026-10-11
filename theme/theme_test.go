package theme

import (
	"regexp"
	"strings"
	"testing"
)

var (
	reHex  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	reDecl = regexp.MustCompile(`(?m)^\s+--[\w-]+: [^;]+;$`)
)

// The contrast policy, over every palette in both modes: every text role holds 4.5:1 on every
// surface (well, ground, planes, card, float), the primary's ink on a primary fill, the
// selection's text on its face. The terminal holds the stricter policy over its gradients too
// (tui TestDepthContrast).
func TestEveryTextRoleReads(t *testing.T) {
	for _, p := range All {
		for _, dark := range []bool{true, false} {
			r := p.Variant(dark)
			for sn, s := range r.Surfaces() {
				for tn, tx := range r.TextRoles() {
					if c := Contrast(Hex(tx), Hex(s)); c < 4.5 {
						t.Errorf("%s dark=%v: %s %s on %s %s is %.2f:1", p.Name, dark, tn, tx, sn, s, c)
					}
				}
			}
			if c := Contrast(Hex(r.PrimaryInk), Hex(r.Primary)); c < 4.5 {
				t.Errorf("%s dark=%v: primary-ink on primary is %.2f:1", p.Name, dark, c)
			}
			if c := Contrast(Hex(r.SelText), Hex(r.Selection)); c < 4.5 {
				t.Errorf("%s dark=%v: the selection's text is %.2f:1", p.Name, dark, c)
			}
		}
	}
}

// Every palette is whole: a unique name, its source and swatches, every role a hex, every lane
// and class, and the elevations in order (dark: well < ground < plane < card < float; light: well
// < plane < card ≤ float).
func TestPalettesAreWhole(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range All {
		if p.Name == "" || seen[p.Name] || p.Label == "" || p.Source == "" || len(p.Swatches) != 5 {
			t.Errorf("%q: a palette has a unique name, a label, a source and five swatches", p.Name)
		}
		seen[p.Name] = true
		for _, sw := range p.Swatches {
			if !reHex.MatchString(sw.Hex) {
				t.Errorf("%s: swatch %s %q", p.Name, sw.Name, sw.Hex)
			}
		}
		for _, dark := range []bool{true, false} {
			r := p.Variant(dark)
			for _, kv := range CSSVars(r, dark) {
				if kv[0] == "--realm-primary-wash" {
					continue
				}
				if !reHex.MatchString(kv[1]) {
					t.Errorf("%s dark=%v: %s is %q", p.Name, dark, kv[0], kv[1])
				}
			}
			for _, h := range []string{r.Lift, r.Shade} {
				if !reHex.MatchString(h) {
					t.Errorf("%s dark=%v: lift/shade %q", p.Name, dark, h)
				}
			}
			if len(r.Lanes) != len(LaneKeys) || len(r.Classes) != len(ClassKeys) {
				t.Errorf("%s dark=%v: %d lanes, %d classes", p.Name, dark, len(r.Lanes), len(r.Classes))
			}
			l := func(h string) float64 { return Hex(h).Luminance() }
			if dark && !(l(r.Well) < l(r.Ground) && l(r.Ground) < l(r.Plane) && l(r.Plane) < l(r.Card) && l(r.Card) < l(r.Float)) {
				t.Errorf("%s dark: the elevations are out of order", p.Name)
			}
			if !dark && !(l(r.Well) < l(r.Plane) && l(r.Plane) < l(r.Card) && l(r.Card) <= l(r.Float)) {
				t.Errorf("%s light: the elevations are out of order", p.Name)
			}
		}
	}
	if _, ok := Get(Default); !ok {
		t.Fatalf("the default %q is not a palette", Default)
	}
}

// The stylesheet carries every role three times (dark, light, a light system) and refuses
// nothing: an unknown name is the default's.
func TestCSS(t *testing.T) {
	for _, p := range All {
		css := CSS(p)
		n := len(CSSVars(p.Dark, true))
		if got := strings.Count(css, "--realm-ground:"); got != 3 {
			t.Errorf("%s: --realm-ground set %d times, want 3", p.Name, got)
		}
		if got := len(reDecl.FindAllString(css, -1)); got != 3*n {
			t.Errorf("%s: %d declarations, want %d", p.Name, got, 3*n)
		}
		if !strings.Contains(css, `:root[data-theme="light"]`) || !strings.Contains(css, "prefers-color-scheme: light") {
			t.Errorf("%s: the light blocks are missing", p.Name)
		}
	}
	if Must("nope").Name != Default {
		t.Error("an unknown name is not the default")
	}
	if err := Check("synthwave", "system"); err != nil {
		t.Error(err)
	}
	if err := Check("vaporwave", "dark"); err == nil || !strings.Contains(err.Error(), "cyberpunk | neon-dystopia") {
		t.Errorf("an unknown theme names the list: %v", err)
	}
	if err := Check("glitch", "dim"); err == nil {
		t.Error("an unknown mode passed")
	}
}

// The role tables, for the eye (go test -v -run TestRoleTable).
func TestRoleTable(t *testing.T) {
	for _, p := range All {
		for _, dark := range []bool{true, false} {
			r := p.Variant(dark)
			t.Logf("%s dark=%v ground %s card %s text %s muted %s primary %s accent %s ok %s warn %s danger %s plan %s lanes %v",
				p.Name, dark, r.Ground, r.Card, r.Text, r.Muted, r.Primary, r.Accent, r.OK, r.Warn, r.Danger, r.Plan, r.Lanes)
		}
	}
}
