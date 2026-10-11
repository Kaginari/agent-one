package board

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kaginari/agent-one/value"
)

// pngish is a stand-in portrait: the resolver reads bytes, it never decodes them.
var pngish = []byte("\x89PNG\r\n\x1a\nworkspace")

// The one portrait order: the workspace's own file (any spelling) before config's before the
// embedded rank portrait; nothing at all reads nil.
func TestPortraitResolutionOrder(t *testing.T) {
	root := t.TempDir()
	emb, _ := Portrait("auditor")
	if b, src := ResolvePortrait(root, ".agent-one", "auditor", ""); !bytes.Equal(b, emb) || src != "embedded" {
		t.Fatalf("the embedded portrait answers any spelling: %q", src)
	}
	_ = os.MkdirAll(filepath.Join(root, "art"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "art", "watch.png"), []byte("configured"), 0o644)
	if b, src := ResolvePortrait(root, ".agent-one", "auditor", "art/watch.png"); string(b) != "configured" || src != "art/watch.png" {
		t.Fatalf("ranks.<r>.portrait comes before the embedded one: %q", src)
	}
	_ = os.MkdirAll(filepath.Join(root, ".agent-one", "portraits"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".agent-one", "portraits", "auditor.png"), pngish, 0o644)
	if b, src := ResolvePortrait(root, ".agent-one", "Auditor", "art/watch.png"); !bytes.Equal(b, pngish) || src != ".agent-one/portraits/auditor.png" {
		t.Fatalf("the workspace's own portrait (an old spelling) comes first: %q", src)
	}
	if b, _ := ResolvePortrait(root, ".agent-one", "night-watch", ""); b != nil {
		t.Fatal("a custom rank with no portrait has none")
	}
	if names := strings.Join(PortraitNames(), ","); names != "auditor,coord,domain,orchestrator,principal-coordinator,principal-domain-owner,service,zone" {
		t.Fatalf("the embedded portraits are spelled the one way: %s", names)
	}
}

// The value policy reaches the board: /api/value and /api/meters answer from the integrator, a
// custom rank's node carries its family's glyph and its value, /portraits/<rank> resolves.
func TestValueAPIsAndIcons(t *testing.T) {
	b := newBoard(t, "empty", Options{})
	code, agent := get(t, b, "/api/value")
	if code != 200 || !strings.Contains(agent, `"lit":false`) {
		t.Fatalf("no integrator, a silent value API: %d %s", code, agent)
	}
	var m struct {
		Session map[string]value.MeterDef `json:"session"`
	}
	if code, agent := get(t, b, "/api/meters"); code != 200 || json.Unmarshal([]byte(agent), &m) != nil || m.Session["ctx"].Direction != "down" || m.Session["score"].Bands.OK == 0 {
		t.Fatalf("the shipped meters: %d %s", code, agent)
	}
	rep := map[string]any{"agents": []any{}}
	view := &ValueView{Lit: true, Window: 7 * 24 * time.Hour, Def: value.DefaultDef(), Report: rep,
		Meters: map[string]value.MeterDef{"ctx": {Signal: "ctx", Direction: "down", Bands: value.Bands{OK: 50, Warn: 75}}},
		Agents: map[string]NodeValue{"orchestrator": {Score: 88, Confidence: "ok", Band: "ok", Family: "root"}}}
	b2 := newBoard(t, "empty", Options{Sources: Sources{
		Value: func() *ValueView { return view },
		Ranks: func() []RankMeta {
			return []RankMeta{{Rank: "orchestrator", Family: "root"}, {Rank: "night_watch", Family: "auditor"}}
		},
	}})
	if code, agent := get(t, b2, "/api/value"); code != 200 || !strings.Contains(agent, `"lit":true`) || !strings.Contains(agent, `"night-watch"`) {
		t.Fatalf("the integrator's value: %d %s", code, agent)
	}
	if code, agent := get(t, b2, "/api/meters"); code != 200 || !strings.Contains(agent, `"warn":75`) {
		t.Fatalf("the meters are config's: %d %s", code, agent)
	}
	var c struct {
		Nodes []Node `json:"nodes"`
	}
	_, agent = get(t, b2, "/api/team")
	if err := json.Unmarshal([]byte(agent), &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Nodes) != 1 || c.Nodes[0].Icon == nil || c.Nodes[0].Icon.Kind != "portrait" || c.Nodes[0].Icon.URL != "portraits/orchestrator" || c.Nodes[0].Value == nil || c.Nodes[0].Value.Score != 88 || c.Nodes[0].Family != "root" {
		t.Fatalf("the orchestrator's node carries its icon, family and value: %+v", c.Nodes)
	}
	if ic := b2.iconFor("NightWatch"); ic.Kind != "family" || ic.URL != "web/icons/icons.svg#family-auditor" {
		t.Fatalf("a custom rank with no portrait holds its family's glyph: %+v", ic)
	}
	if ic := b2.iconFor("ghost"); ic.Kind != "generic" {
		t.Fatalf("a rank nobody knows holds the generic glyph: %+v", ic)
	}
	if code, agent := get(t, b2, "/portraits/auditor"); code != 200 || !strings.HasPrefix(agent, "\x89PNG") {
		t.Fatalf("/portraits/<rank> resolves any spelling: %d", code)
	}
	if code, agent := get(t, b2, "/portraits/night-watch"); code != 200 || !strings.HasPrefix(agent, "<svg") || !strings.Contains(agent, "#8A8F98") {
		t.Fatalf("a custom rank's portrait is its family's glyph: %d %s", code, agent)
	}
	// the activity window and the failure word come from policy.value
	if b2.activityDays() != 7 {
		t.Fatalf("the activity window is policy.value.window: %d", b2.activityDays())
	}
	// the icon sprite ships with its licences and its source line
	for _, f := range []string{"web/icons/icons.svg", "web/icons/LICENSE-lucide", "web/icons/LICENSE-tabler-icons", "web/icons/SOURCE"} {
		if _, err := content.ReadFile(f); err != nil {
			t.Errorf("%s is embedded: %v", f, err)
		}
	}
	sprite, _ := content.ReadFile("web/icons/icons.svg")
	for _, id := range []string{"skill", "family-root", "family-planner", "family-gate", "family-author", "family-steward", "family-auditor", "family-generic"} {
		if !strings.Contains(string(sprite), `id="`+id+`"`) {
			t.Errorf("the sprite lacks #%s", id)
		}
	}
}

// Mass is the code a member owns: its own lines (the files whose deepest owner it is), its
// subtree's summed up the tree, the orchestrator the whole workspace; tests and generated code left out.
func TestMasses(t *testing.T) {
	root := t.TempDir()
	w := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		_ = os.WriteFile(filepath.Join(root, p), []byte(s), 0o644)
	}
	w("go.mod", "module x\n")
	w("auth/login/login.go", "package login\n\nfunc A() {}\n")
	w("auth/session.go", "package auth\n")
	w("auth/login/login_test.go", "package login\n\n\n\n\n")
	w("auth/gen.go", "// Code generated by x. DO NOT EDIT.\npackage auth\n\n\n")
	c := &Team{Nodes: []Node{
		{ID: "orchestrator", Kind: "member", Orchestrator: true},
		{ID: "domain-auth", Kind: "member", Parent: "orchestrator", Owns: []string{"auth"}},
		{ID: "zone-login", Kind: "member", Parent: "domain-auth", Owns: []string{"auth/login"}},
		{ID: "skill-x", Kind: "skill"},
	}, Edges: []Edge{{From: "domain-auth", To: "skill-x", Bond: "holds"}, {From: "zone-login", To: "skill-x", Bond: "holds"}}}
	sourceMemo.root = "" // a fresh walk
	Masses(root, c)
	by := map[string]Node{}
	for _, n := range c.Nodes {
		by[n.ID] = n
	}
	if by["zone-login"].Mass != 3 || by["domain-auth"].Own != 1 || by["domain-auth"].Mass != 4 || by["orchestrator"].Mass != 4 || by["skill-x"].Mass != 2 {
		t.Fatalf("masses: %+v", c.Nodes)
	}
}

// Every member holds its face at every depth (a zone worker too), the 3D overlay draws it as a badge
// through the one resolver, and the focus card shows the rank's image large. The page's code is
// read as text: the rule is that no depth gate stands between a node and its icon.
func TestFacesAtEveryDepth(t *testing.T) {
	net, _ := content.ReadFile("web/components/net/net.js")
	if strings.Contains(string(net), "p.d <= 2") || !strings.Contains(string(net), "if (icon) {") {
		t.Fatal("the 2D net draws every member's icon, whatever its depth")
	}
	n3, _ := content.ReadFile("web/components/net/net3d.js")
	if !strings.Contains(string(n3), "'portraits/' + encodeURIComponent(rank)") {
		t.Fatal("the 3D net's badges come from /portraits/<rank>")
	}
	dash, _ := content.ReadFile("web/dash.js")
	if !strings.Contains(string(dash), "kcard-face") || !strings.Contains(string(dash), "kcard-portrait") || !strings.Contains(string(dash), "kcard-glyph") {
		t.Fatal("the focus card shows the rank's portrait, else its family's glyph")
	}
	for _, old := range []string{"web/portraits/' + n.rank", "PORTRAITS", "BRAIN"} {
		if strings.Contains(string(net), old) || strings.Contains(string(n3), old) {
			t.Errorf("%q is gone: the resolver and the sprite are the one source", old)
		}
	}
}
