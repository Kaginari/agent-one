package board

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Kaginari/agent-one/rank"
)

// Icon is how a surface draws a rank: its portrait image, or its family's glyph (a symbol of
// web/icons/icons.svg, stroked in the theme's colour), or the generic glyph.
type Icon struct {
	Kind   string `json:"kind"` // portrait | family | generic
	URL    string `json:"url"`  // the portrait (portraits/<rank>) or the sprite symbol (web/icons/icons.svg#family-gate)
	Family string `json:"family,omitempty"`
}

// embeddedPortraits are the rank portraits the binary ships, by canonical rank.
func embeddedPortraits() []string {
	var out []string
	ents, _ := fs.ReadDir(content, "web/portraits")
	for _, e := range ents {
		if n := strings.TrimSuffix(e.Name(), ".png"); n != e.Name() {
			out = append(out, n)
		}
	}
	return out
}

// PortraitNames lists the embedded rank portraits, in canonical spelling (the files' names).
func PortraitNames() []string { return embeddedPortraits() }

// Portrait returns an embedded rank portrait PNG; any spelling of the rank reads it.
func Portrait(name string) ([]byte, error) {
	c := rank.Canon(name)
	b, err := content.ReadFile("web/portraits/" + c + ".png")
	if err != nil {
		return nil, fmt.Errorf("portrait %q: not one of %v", name, embeddedPortraits())
	}
	return b, nil
}

// ResolvePortrait finds a rank's portrait in the one order every surface shares: the workspace's own
// <workspace>/portraits/<rank>.png (any accepted spelling), then the configured ranks.<r>.portrait
// (root-relative), then the embedded rank portrait. It answers the bytes and where they came
// from; no portrait is (nil, "").
func ResolvePortrait(workspaceRoot, workspaceDir, rankName, configured string) ([]byte, string) {
	if workspaceRoot != "" {
		for _, alias := range rank.Aliases(rankName) {
			p := filepath.Join(workspaceRoot, workspaceDir, "portraits", alias+".png")
			if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
				return b, filepath.ToSlash(filepath.Join(workspaceDir, "portraits", alias+".png"))
			}
		}
		if configured != "" {
			p := configured
			if !filepath.IsAbs(p) {
				p = filepath.Join(workspaceRoot, p)
			}
			if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
				return b, configured
			}
		}
	}
	if b, err := Portrait(rankName); err == nil {
		return b, "embedded"
	}
	return nil, ""
}

// meta is the integrator's word on a rank, zero when it said nothing.
func (b *Board) meta(rankName string) RankMeta {
	c := rank.Canon(rankName)
	for _, m := range b.rankMetas() {
		if m.Rank == c {
			return m
		}
	}
	return RankMeta{Rank: c}
}

// iconFor resolves a rank's icon: a portrait when one resolves, else its family's glyph, else the
// generic one.
func (b *Board) iconFor(rankName string) Icon {
	c := rank.Canon(rankName)
	if c == "" {
		return Icon{Kind: "generic", URL: "web/icons/icons.svg#family-generic", Family: rank.FamilyGeneric}
	}
	m := b.meta(c)
	if data, _ := ResolvePortrait(b.opt.WorkspaceRoot, b.opt.WorkspaceDir, c, m.Portrait); data != nil {
		return Icon{Kind: "portrait", URL: "portraits/" + c, Family: m.Family}
	}
	if m.Family != "" {
		return Icon{Kind: "family", URL: "web/icons/icons.svg#family-" + m.Family, Family: m.Family}
	}
	return Icon{Kind: "generic", URL: "web/icons/icons.svg#family-generic", Family: rank.FamilyGeneric}
}

var reSymbol = regexp.MustCompile(`(?s)<symbol id="([^"]+)"([^>]*)>(.*?)</symbol>`)

// glyph is one sprite symbol as a standalone svg document (for an <img>, which cannot reach into
// a sprite): stroked in a neutral grey that reads on both themes.
func glyph(id string) []byte {
	sprite, err := content.ReadFile("web/icons/icons.svg")
	if err != nil {
		return nil
	}
	for _, m := range reSymbol.FindAllSubmatch(sprite, -1) {
		if string(m[1]) == id {
			return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"` + strings.ReplaceAll(string(m[2]), "currentColor", "#8A8F98") + `>` + string(m[3]) + `</svg>`)
		}
	}
	return nil
}

// apiPortrait serves /portraits/<rank> through the resolver: the portrait PNG, else the family's
// glyph as an svg, else the generic glyph. Any spelling of the rank is accepted.
func (b *Board) apiPortrait(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/portraits/"), ".png")
	if name == "" || strings.ContainsAny(name, "/\\.") {
		http.NotFound(w, r)
		return
	}
	m := b.meta(name)
	if data, _ := ResolvePortrait(b.opt.WorkspaceRoot, b.opt.WorkspaceDir, name, m.Portrait); data != nil {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
		return
	}
	fam := m.Family
	if fam == "" {
		fam = rank.FamilyGeneric
	}
	g := glyph("family-" + fam)
	if g == nil {
		g = glyph("family-generic")
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write(g)
}
