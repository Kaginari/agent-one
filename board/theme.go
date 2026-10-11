package board

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Kaginari/agent-one/theme"
)

// The look on the dashboard: /theme.css is the palette's roles as custom properties, generated
// from the binary's palette (package theme) — web/theme.css's meaning tier reads them;
// /api/theme lists the palettes and the look in force, and a POST from this board's own page
// switches it for the session (the terminal follows, config.local.yaml keeps it).

// look is the palette and mode in force (the integrator's, else the defaults).
func (b *Board) look() (name, mode string) {
	if b.opt.Theme != nil {
		name, mode = b.opt.Theme()
	}
	if _, ok := theme.Get(name); !ok {
		name = theme.Default
	}
	if mode != "light" && mode != "system" {
		mode = "dark"
	}
	return name, mode
}

// modeAttr is the page's data-theme for a mode: none for system (the browser decides).
func modeAttr(mode string) string {
	if mode == "system" {
		return ""
	}
	return ` data-theme="` + mode + `"`
}

func (b *Board) themeRoutes() {
	b.mux.HandleFunc("/theme.css", func(w http.ResponseWriter, r *http.Request) {
		name, _ := b.look()
		if q := r.URL.Query().Get("name"); q != "" {
			if _, ok := theme.Get(q); ok {
				name = q
			}
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(theme.CSS(theme.Must(name))))
	})
	b.mux.HandleFunc("/api/theme", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b.setTheme(w, r)
			return
		}
		name, mode := b.look()
		type swatch struct {
			Name string `json:"name"`
			Hex  string `json:"hex"`
		}
		type entry struct {
			Name     string            `json:"name"`
			Label    string            `json:"label"`
			Source   string            `json:"source"`
			Swatches []swatch          `json:"swatches"`
			Dark     map[string]string `json:"dark"`
			Light    map[string]string `json:"light"`
		}
		chip := func(r theme.Roles) map[string]string {
			return map[string]string{"ground": r.Ground, "card": r.Card, "text": r.Text, "primary": r.Primary, "accent": r.Accent}
		}
		var list []entry
		for _, p := range theme.All {
			e := entry{Name: p.Name, Label: p.Label, Source: p.Source, Dark: chip(p.Dark), Light: chip(p.Light)}
			for _, s := range p.Swatches {
				e.Swatches = append(e.Swatches, swatch{s.Name, s.Hex})
			}
			list = append(list, e)
		}
		writeJSON(w, map[string]any{"name": name, "mode": mode, "modes": theme.Modes, "themes": list, "settable": b.opt.SetTheme != nil})
	})
}

// setTheme is the dashboard's switch: this board's own page only (loopback, the token), even
// without a session behind the board — the look is the workspace's config, not the session's.
func (b *Board) setTheme(w http.ResponseWriter, r *http.Request) {
	if !local(r) || r.Header.Get("X-Board-Token") != b.token {
		http.Error(w, "refused: acts come from this board's own page", http.StatusForbidden)
		return
	}
	if b.opt.SetTheme == nil {
		http.Error(w, "this board cannot keep a theme", http.StatusConflict)
		return
	}
	var in struct{ Name, Mode string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&in); err != nil {
		http.Error(w, "a theme needs a name or a mode", http.StatusBadRequest)
		return
	}
	cur, curMode := b.look()
	name, mode := strings.ToLower(strings.TrimSpace(in.Name)), strings.ToLower(strings.TrimSpace(in.Mode))
	if name == "" {
		name = cur
	}
	if mode == "" {
		mode = curMode
	}
	if err := theme.Check(name, mode); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := b.opt.SetTheme(name, mode); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "name": name, "mode": mode})
}
