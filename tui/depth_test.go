package tui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// defaultFg is the terminal's own text colour the text role inherits (VS Code's).
func defaultFg(dark bool) rgb {
	if dark {
		return hexRGB("#cccccc")
	}
	return hexRGB("#3b3b3b")
}

// faces is every surface a text sits on, by name, in a theme.
func faces(t Theme) map[string]color.Color {
	out := map[string]color.Color{
		"plane": t.dep.plane.col(), "side": t.dep.side.col(), "well": t.dep.well.col(),
		"card": t.dep.card.col(), "float": t.dep.float.col(),
	}
	add := func(name string, s surf) {
		out[name] = s.face
		out[name+".band"] = s.band
	}
	add("user", t.surfUser())
	add("answer", t.surfAnswer())
	for _, c := range []string{"read", "write", "outward", "destructive"} {
		add("tool."+c, t.surfTool(c))
	}
	for _, r := range []string{"orchestrator", "coord", "domain", "zone", "service", "auditor"} {
		add("subagent."+r, t.surfSubagent(r, t.lane(r)))
		add("tile."+r, t.surfTile(t.lane(r)))
	}
	add("tile", t.surfTile(nil))
	h, g := t.surfHero()
	add("hero", h)
	out["hero.end"] = g
	add("toast.ok", t.surfToast(t.ok.GetForeground()))
	add("toast.bad", t.surfToast(t.bad.GetForeground()))
	add("toast", t.surfToast(t.accent.GetForeground()))
	add("float", t.surfFloat())
	from, to := t.headerTints()
	out["header.from"], out["header.to"] = from.col(), to.col()
	out["veil"] = t.veil()
	out["status"] = t.statusBg()
	t.Depth = true
	out["code"] = t.codeBg()
	return out
}

// The contrast policy: every text role holds 4.5:1 on every surface it can sit on, dark and light.
func TestDepthContrast(t *testing.T) {
	for _, dark := range []bool{true, false} {
		th := NewTheme(dark)
		roles := map[string]rgb{"text": defaultFg(dark), "dim": toRGB(th.dim.GetForeground()), "label": toRGB(th.label.GetForeground()),
			"user": toRGB(th.user.GetForeground()), "accent": toRGB(th.accent.GetForeground()), "magenta": toRGB(th.magenta.GetForeground()),
			"ok": toRGB(th.ok.GetForeground()), "bad": toRGB(th.bad.GetForeground()), "warn": toRGB(th.warn.GetForeground())}
		for fname, f := range faces(th) {
			for rname, r := range roles {
				if c := contrast(r, toRGB(f)); c < 4.5 {
					t.Errorf("dark=%v: %s on %s (%s) is %.2f:1", dark, rname, fname, toRGB(f).hex(), c)
				}
			}
		}
	}
}

// checkSlab holds a raised block's rows (x0: the cells left of the slab, a principles's) to the
// bevel's policies: every row exactly w cells; the lit top face (when drawn) a closed run of ▗▄▖ or
// ▄ over the slab's width with the shadow's column clear; the bottom edge ▝ then ▀ to the
// shadow's column, which ends it; every agent row's last cell the shadow's (blank). A shadow
// never overflows the width the block was asked for.
func checkSlab(t *testing.T, name, block string, w, x0 int, top bool) {
	t.Helper()
	rows := strings.Split(block, "\n")
	strip := make([][]rune, len(rows))
	for i, r := range rows {
		if got := ansi.StringWidth(r); got != w {
			t.Fatalf("%s: row %d is %d cells, want %d: %q", name, i, got, w, ansi.Strip(r))
		}
		strip[i] = []rune(ansi.Strip(r))
	}
	bw := w - x0 - 1 // the slab's face; its shadow takes the last cell
	first := 0
	if top {
		first = 1
		f := string(strip[0][x0 : x0+bw])
		if strings.Trim(f, "▗▄▖") != "" || strip[0][x0+bw] != ' ' {
			t.Fatalf("%s: the top face is open: %q", name, string(strip[0]))
		}
	}
	last := strip[len(strip)-1]
	if last[x0] != '▝' || strings.Trim(string(last[x0+1:x0+bw]), "▀") != "" || last[x0+bw] != ' ' {
		t.Fatalf("%s: the bottom edge is open: %q", name, string(last))
	}
	for i := first; i < len(strip)-1; i++ {
		// a chip without a top face drops its shadow half a row: its first row's shadow is ▄
		if c := strip[i][x0+bw]; c != ' ' && !(i == 0 && !top && c == '▄') {
			t.Fatalf("%s: row %d spills into the shadow's column: %q", name, i, string(strip[i]))
		}
	}
}

// Every raised block at every width: bevels closed, shadows inside the asked width — dark and
// light.
func TestDepthBlocksClosed(t *testing.T) {
	for _, dark := range []bool{true, false} {
		th := depthTheme(dark)
		th.Depth = true
		ic := th.Icon("agent-one", "domain", 0)
		for _, w := range []int{36, 50, 64, 80, 100, 120, 160} {
			n := func(s string) string { return fmt.Sprintf("%s dark=%v w=%d", s, dark, w) }
			checkSlab(t, n("user"), th.User("explain the gate in this workspace and then run the tests under src/auth — take your time", w), w, 0, true)
			checkSlab(t, n("answer"), th.Assistant("The gate has **four checks**:\n\n1. Right zone worker\n2. Invariants\n\n```go\nfunc Gate() {}\n```\n\nRun `go test`.", w), w, 0, true)
			checkSlab(t, n("tool"), th.Tool(ToolView{Name: "bash", Summary: "go test ./...", Class: "read", Status: "done", Ms: 1234, Output: "ok\n"}, w), w, 0, false)
			checkSlab(t, n("tool-diff"), th.Tool(ToolView{Name: "edit", Summary: "a.go", Class: "write", Status: "done", Expand: true, Diff: func() *Diff { d := DiffText("a\n", "b\n", 2); return &d }()}, w), w, 0, false)
			checkSlab(t, n("subagent"), th.Subagent(SubagentView{Name: "zone-auth", Rank: "zone", Role: "analyst", State: "done", Elapsed: 9 * time.Second, Ask: "findings",
				Report: &ReportView{Status: "PASS", Findings: []string{"src/auth/token.go:1 token ttl is 15m"}}}, w), w, 0, false)
			if w-8 >= 30 {
				checkSlab(t, n("subagent-principles"), th.Subagent(SubagentView{Name: "domain-security", Rank: "domain", State: "thinking", Elapsed: 8 * time.Second, Ask: "verdict"}, w,
					Pulse{Lane: th.lane("domain"), Phase: 4, Principles: [2]string{ic[0], ic[1]}}), w, 8, false)
			}
			cw := min(w, 84)
			checkSlab(t, n("welcome"), th.Welcome(Welcome{Dist: "agent-one", Version: "v0.2.0", Workspace: "/home/monta/kaginari/pfcli", Model: "gemini/x", Session: "s"}, w), cw, 0, true)
			ch := th.Choice(ChoiceView{Title: "Approval", Class: "outward", Tool: "bash", Summary: "git push", Why: "outward", Options: []string{"Yes", "No"}}, w)
			checkSlab(t, n("choice"), ch, min(w-5, 96)+5, 0, true)
			checkSlab(t, n("panel"), th.panel(th.title("roles"), []string{"a", "b"}, w), w, 0, true)
			tc := th.toastCard(toast{text: "✓ zone-auth done", style: th.pill("ok")})
			checkSlab(t, n("toast"), tc, ansi.StringWidth("✓ zone-auth done")+8, 0, true)
		}
		// the sidebar's tiles: each a closed slab inside the column
		for _, rw := range []int{28, 36, 50} {
			rows := th.railRows(RailView{Workspace: "agent-one", Model: "m", CtxKnown: true, CtxPct: 40, Tokens: 10,
				Subagents: []RailSubagent{{Name: "coord-agent-one", State: "thinking", Lane: th.lane("coord")}},
				Files:     []RailFile{{Path: "tui/blocks.go", Lane: th.lane("zone")}}}, rw)
			var tile []string
			tiles := 0
			for _, r := range rows {
				if r.sep {
					t.Fatalf("rw=%d: a depth sidebar draws tiles, not rules", rw)
				}
				tile = append(tile, r.text)
				if strings.HasPrefix(ansi.Strip(r.text), "▝") {
					checkSlab(t, fmt.Sprintf("tile %d dark=%v rw=%d", tiles, dark, rw), strings.Join(tile, "\n"), rw-1, 0, tiles == 0)
					tile, tiles = nil, tiles+1
				}
			}
			if tiles != 3 || len(tile) != 0 {
				t.Fatalf("rw=%d: %d tiles (want vitals, subagents, files), %d rows left over", rw, tiles, len(tile))
			}
		}
	}
}

// A drag over a raised slab copies its text: never the spine, the bevel or the shadow.
func TestDepthSelectionCopiesText(t *testing.T) {
	m := New(&fakeHost{}, depthTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m.Update(evBgWait{})
	m.Update(EvTurnStart{Text: "redesign the frame please"})
	m.Render()
	if !m.theme.Depth {
		t.Fatal("the depth is off on a truecolor frame")
	}
	line := -1
	for i, l := range m.vpLines {
		if strings.Contains(ansi.Strip(l), "redesign the frame please") {
			line = i
		}
	}
	if line < 1 {
		t.Fatalf("the ask's slab is not in the pane:\n%s", ansi.Strip(strings.Join(m.vpLines, "\n")))
	}
	// from the top face over the whole slab to its bottom edge, every cell
	m.sel = &selection{anchor: spot{line - 1, 0}, head: spot{line + 1, m.vpCW - 1}, shown: true}
	if got := m.selText(); got != "redesign the frame please" {
		t.Fatalf("the copy carries the slab's drawing: %q", got)
	}
}

// A turn's answer pieces join into one slab: one lit top face, one bottom edge.
func TestDepthAnswerPiecesJoin(t *testing.T) {
	m := New(&fakeHost{}, depthTheme(true), DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m.Update(evBgWait{})
	m.Update(EvTurnStart{Text: "go"})
	m.Render()
	m.printAnswer("The first paragraph.")
	m.printAnswer("The second paragraph.")
	m.stream.WriteString("The third, still being written")
	m.Render()
	var tops, bottoms int
	for _, l := range m.vpLines {
		p := ansi.Strip(l)
		if strings.HasPrefix(p, "▄") && strings.Trim(p, "▄ ") == "" { // a slab's top face, not the spinner's mascot
			tops++
		}
		if strings.HasPrefix(p, "▝") {
			bottoms++
		}
	}
	// the ask's slab and the answer's (and the welcome's bottom edge)
	if tops != 2 || bottoms != 3 {
		t.Fatalf("the answer's pieces did not join (tops %d, bottoms %d):\n%s", tops, bottoms, ansi.Strip(strings.Join(m.vpLines, "\n")))
	}
}

// A 256-colour terminal keeps the flat frame: its tonal steps would band.
func TestDepthNeedsTrueColor(t *testing.T) {
	th := NewTheme(true)
	th.Color, th.TrueColor = true, false
	m := New(&fakeHost{}, th, DefaultWords())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m.Update(evBgWait{})
	f := m.Render()
	if m.theme.Depth || strings.ContainsAny(ansi.Strip(f), "▝▗▐") {
		t.Fatalf("a 256-colour frame drew the depth")
	}
	// and a window too small for the frame prints the scrollback flat
	m2 := New(&fakeHost{}, depthTheme(true), DefaultWords())
	m2.Update(tea.WindowSizeMsg{Width: 50, Height: 20})
	m2.Render()
	if m2.theme.Depth {
		t.Fatal("the inline scrollback holds the depth")
	}
}

// The ground follows the terminal's answer; a contradicting answer keeps the default.
func TestDepthGroundFromTerminal(t *testing.T) {
	th := NewTheme(true).OnGround(lipgloss.Color("#0d1117"))
	if th.dep.ground.hex() != "#0d1117" {
		t.Fatalf("ground %s", th.dep.ground.hex())
	}
	if l := NewTheme(true).OnGround(lipgloss.Color("#ffffff")); l.dep.ground.hex() != "#1f1f1f" {
		t.Fatalf("a light answer re-grounded a dark theme: %s", l.dep.ground.hex())
	}
	for _, g := range []string{"#0d1117", "#1e1e1e", "#282c34"} {
		d := NewTheme(true).OnGround(lipgloss.Color(g)).dep
		if !(d.well.lum() < d.ground.lum() && d.ground.lum() < d.plane.lum() && d.plane.lum() < d.card.lum() && d.card.lum() < d.float.lum()) {
			t.Fatalf("%s: the elevations are out of order: well %s ground %s plane %s card %s float %s", g, d.well.hex(), d.ground.hex(), d.plane.hex(), d.card.hex(), d.float.hex())
		}
	}
	d := NewTheme(false).dep
	if !(d.well.lum() < d.plane.lum() && d.plane.lum() < d.card.lum() && d.card.lum() <= d.float.lum()) {
		t.Fatalf("light: the elevations are out of order")
	}
}
