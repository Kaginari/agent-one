package board

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// getLocal is a request the board's own page makes: addressed to 127.0.0.1.
func getLocal(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", path, nil)
	req.Host = "127.0.0.1:7485"
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func getJSONLocal(t *testing.T, h http.Handler, path string, v any) {
	t.Helper()
	code, agent := getLocal(t, h, path)
	if code != 200 {
		t.Fatalf("%s: %d %s", path, code, agent)
	}
	if err := json.Unmarshal([]byte(agent), v); err != nil {
		t.Fatalf("%s: not JSON: %v\n%s", path, err, agent)
	}
}

// The root is the dashboard, Overview first, with every view a tab and no way back to the old
// board pages; the page carries its token, its words and the strict CSP.
func TestRootIsTheDashboard(t *testing.T) {
	b := newBoard(t, "workspace", Options{})
	rec := serve(t, b, "/")
	if rec.Code != 200 {
		t.Fatalf("/: %d", rec.Code)
	}
	html := rec.Body.String()
	if !strings.Contains(html, `content="`+b.Token()+`"`) || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Error("the root must be the dashboard: its token and its CSP")
	}
	for _, v := range []string{"overview", "console", "net", "metrics", "subagent", "memory", "toolbox", "relations", "tree", "config"} {
		if !strings.Contains(html, `data-view="`+v+`"`) {
			t.Errorf("no %s tab", v)
		}
		if v != "overview" && v != "console" && v != "net" && !strings.Contains(html, `data-panel="`+v+`"`) {
			t.Errorf("no %s panel", v)
		}
	}
	if i, j := strings.Index(html, `data-view="overview"`), strings.Index(html, `data-view="console"`); i < 0 || i > j || !strings.Contains(html, `id="main" data-view="overview"`) {
		t.Error("Overview is the first tab and the view the root opens on")
	}
	for _, gone := range []string{"board pages", `href="./"`, "bootstrap"} {
		if strings.Contains(html, gone) {
			t.Errorf("the dashboard still links the old pages: %q", gone)
		}
	}
	for _, need := range []string{`id="tabs-more-btn"`, `aria-haspopup="menu"`, `role="tablist"`, `src="web/workspace.js"`} {
		if !strings.Contains(html, need) {
			t.Errorf("the tabs need %s", need)
		}
	}
	if code, _ := get(t, b, "/web/workspace.js"); code != 200 {
		t.Error("workspace.js is served")
	}
}

// Old addresses keep working: /dash and the pages that became tabs redirect for good; the
// pages that did not, and the old assets, are gone.
func TestOldAddressesRedirectOrAreGone(t *testing.T) {
	b := newBoard(t, "workspace", Options{})
	for path, to := range map[string]string{"/dash": "/", "/subagent": "/#subagent", "/memory": "/#memory", "/toolbox": "/#toolbox", "/config": "/#config", "/usage": "/#metrics", "/team": "/#net"} {
		rec := serve(t, b, path)
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != to {
			t.Errorf("%s: %d → %q, want 301 → %q", path, rec.Code, rec.Header().Get("Location"), to)
		}
	}
	for _, path := range []string{"/log", "/static/board.css", "/assets/bootstrap.min.css", "/nope", "/subagent/x"} {
		if code, _ := get(t, b, path); code != 404 {
			t.Errorf("%s: %d, want 404", path, code)
		}
	}
	// the stream answers a loopback request by streaming: a short deadline ends the probe
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/dash/stream?probe", nil).WithContext(ctx)
	req.Host = "127.0.0.1:7411"
	b.ServeHTTP(rec, req)
	if rec.Code == 301 {
		t.Error("the session stream keeps its path")
	}
}

func TestPrefixMount(t *testing.T) {
	b := newBoard(t, "workspace", Options{Prefix: "/w"})
	if code, html := get(t, b, "/w/"); code != 200 || !strings.Contains(html, `data-view="memory"`) {
		t.Errorf("the dashboard under the prefix: %d", code)
	}
	if rec := serve(t, b, "/w/dash"); rec.Code != 301 || rec.Header().Get("Location") != "/w/" {
		t.Errorf("/w/dash: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := serve(t, b, "/w/config"); rec.Header().Get("Location") != "/w/#config" {
		t.Errorf("/w/config: %q", rec.Header().Get("Location"))
	}
	if code, _ := get(t, b, "/"); code != 404 {
		t.Errorf("unprefixed path must 404, got %d", code)
	}
	if code, _ := get(t, b, "/w/web/dash.js"); code != 200 {
		t.Error("assets under the prefix")
	}
}

// The workspace's records answer only a request addressed to this machine (DNS rebinding).
func TestWorkspaceAPIsRefuseAReboundHost(t *testing.T) {
	b := newBoard(t, "workspace", Options{Sources: Sources{Config: func() any { return map[string]any{"a": 1} }}})
	for _, p := range []string{"/api/config", "/api/memory", "/api/memory/recall?q=auth", "/api/memory/review", "/api/toolbox", "/api/runs", "/api/run?id=r1",
		"/events", "/api/doc?path=README.md", "/api/subagent", "/api/usage", "/api/team", "/api/activity", "/api/tree", "/api/tree/children?path=.", "/api/subagents", "/api/plan",
		"/", "/api/workspace", "/api/live", "/api/theme", "/theme.css", "/web/dash.js", "/api/value", "/api/meters", "/portraits/zone", "/dash/stream"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", p, nil)
		req.Host = "evil.example:7485"
		b.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s from a rebound host: %d", p, rec.Code)
		}
	}
}

func TestLexiconWordsReachThePage(t *testing.T) {
	b := newBoard(t, "workspace", Options{Names: Names{"page.subagent": "Subagents", "rank.zone": "Zone worker"}})
	_, html := get(t, b, "/")
	if !strings.Contains(html, "Subagents") || !strings.Contains(html, "Zone worker") || !strings.Contains(html, `data-word="page.subagent"`) {
		t.Error("the page carries the distribution's words for its tabs")
	}
}

func TestAPIMemory(t *testing.T) {
	b := newBoard(t, "workspace", Options{})
	var m struct {
		Reading      struct{ Lit bool }
		Notes        []struct{ Text, Kind string }
		ByKind       map[string]int
		WorkingNotes []struct{ Skill string }
		Status       *struct {
			Short  struct{ WorkingMemory struct{ WorkingNotes int } }
			Shared struct{ Workspace int }
		}
		Holes []string
	}
	getJSONLocal(t, b, "/api/memory", &m)
	if !m.Reading.Lit || len(m.Notes) != 3 || m.Notes[0].Text != "an untyped note" || m.ByKind["team"] != 1 || len(m.WorkingNotes) == 0 || m.Status == nil || m.Holes == nil {
		t.Errorf("memory: %+v", m)
	}
	e := newBoard(t, "empty", Options{})
	var s struct {
		Reading      struct{ Lit bool }
		Notes        []any
		WorkingNotes []any
	}
	getJSONLocal(t, e, "/api/memory", &s)
	if s.Reading.Lit || s.Notes == nil || s.WorkingNotes == nil {
		t.Errorf("an empty workspace's memory is silent, its lists empty lists: %+v", s)
	}
}

// Recall is read-only: it answers, and no semantic-cache line is written.
func TestAPIRecallIsReadOnly(t *testing.T) {
	root := t.TempDir()
	copyTree(t, fixtureRoot(t, "workspace"), root)
	b := New(Options{WorkspaceRoot: root, Now: func() time.Time { return fixedNow }})
	t.Cleanup(b.Close)
	before := treeSig(t, filepath.Join(root, ".agent-one"))
	var r struct {
		Results []struct{ Kind, Src string }
		Holes   []string `json:"@?"`
	}
	getJSONLocal(t, b, "/api/memory/recall?q=auth+zone&tier=all", &r)
	if r.Results == nil || r.Holes == nil {
		t.Errorf("recall: %+v", r)
	}
	if after := treeSig(t, filepath.Join(root, ".agent-one")); after != before {
		t.Error("a recall from the board wrote into the workspace")
	}
	if code, _ := getLocal(t, b, "/api/memory/recall"); code != 400 {
		t.Errorf("a recall without ?q: %d", code)
	}
	if code, _ := getLocal(t, b, "/api/memory/recall?q=x&tier=bogus"); code != 400 {
		t.Errorf("a bad tier: %d", code)
	}
	var rv struct {
		Tiers []struct {
			Name     string
			Facts    []string
			Findings []any
		}
	}
	getJSONLocal(t, b, "/api/memory/review", &rv)
	if len(rv.Tiers) < 3 || rv.Tiers[0].Facts == nil || rv.Tiers[0].Findings == nil {
		t.Errorf("review: %+v", rv)
	}
	if after := treeSig(t, filepath.Join(root, ".agent-one")); after != before {
		t.Error("the review wrote into the workspace")
	}
}

func treeSig(t *testing.T, dir string) string {
	t.Helper()
	var sb strings.Builder
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			sb.WriteString(p + "|" + info.ModTime().String() + "|" + strconv.FormatInt(info.Size(), 10) + "\n")
		}
		return nil
	})
	return sb.String()
}

func TestAPIToolbox(t *testing.T) {
	b := newBoard(t, "workspace", Options{})
	var tb struct {
		Reading    struct{ Lit bool }
		Entries    []toolEntry
		Recent     []LoadEvent
		Instrument struct{ Offers, Loads int }
		Holes      []string
		Missing    []string
	}
	getJSONLocal(t, b, "/api/toolbox", &tb)
	state := map[string]string{}
	for _, e := range tb.Entries {
		state[e.Name] = e.State
	}
	if !tb.Reading.Lit || state["tdd"] != "loaded" || state["zone-auth-skill"] != "offered" || tb.Holes == nil || tb.Missing == nil {
		t.Errorf("toolbox: %+v", tb)
	}
	if len(tb.Recent) != 2 || tb.Recent[0].Ev != "load" || tb.Recent[1].Names[0] != "tdd" || tb.Instrument.Offers != 1 || tb.Instrument.Loads != 1 {
		t.Errorf("the loads journal, newest first: %+v %+v", tb.Recent, tb.Instrument)
	}
}

func TestAPIRunsAndOneRun(t *testing.T) {
	b := newBoard(t, "workspace", Options{})
	var rs struct {
		Reading struct{ Lit bool }
		Runs    []runRow
	}
	getJSONLocal(t, b, "/api/runs", &rs)
	if !rs.Reading.Lit || len(rs.Runs) == 0 {
		t.Fatalf("runs: %+v", rs)
	}
	found := false
	for _, r := range rs.Runs {
		if r.ID == "r1" {
			found = r.As == "zone-auth" && r.Status == "PASS" && r.Ask == "smoke"
		}
	}
	if !found {
		t.Errorf("run r1: %+v", rs.Runs)
	}
	var only struct{ Runs []runRow }
	getJSONLocal(t, b, "/api/runs?as=nobody", &only)
	if only.Runs == nil || len(only.Runs) != 0 {
		t.Errorf("?as keeps one agent's runs, an empty list when it has none: %+v", only)
	}
	var d RunDetail
	getJSONLocal(t, b, "/api/run?id=r1", &d)
	if d.As != "zone-auth" || d.Status != "PASS" || len(d.Steps) != 1 || d.Steps[0].Tool != "noop" || d.Steps[0].Status != "done" || d.Holes == nil {
		t.Errorf("run r1 whole: %+v", d)
	}
	for _, id := range []string{"nope", "../runs", ""} {
		if code, _ := getLocal(t, b, "/api/run?id="+id); code != 404 {
			t.Errorf("run %q: %d", id, code)
		}
	}
	var e struct {
		Reading struct{ Lit bool }
		Runs    []runRow
	}
	getJSONLocal(t, newBoard(t, "empty", Options{}), "/api/runs", &e)
	if e.Reading.Lit || e.Runs == nil {
		t.Errorf("an empty workspace's history is silent: %+v", e)
	}
}

func TestAPIConfigLayersOffListAndSecrets(t *testing.T) {
	cfg := map[string]any{
		"tools":     map[string]any{"webfetch": map[string]any{"enabled": map[string]any{"value": false, "origin": ".agent-one/config.yaml:12"}}},
		"models":    map[string]any{"default": map[string]any{"value": "anthropic/claude-opus-5-5", "origin": "(unset)"}},
		"providers": map[string]any{"gw": map[string]any{"apiKey": map[string]any{"value": "sk-live-123", "origin": ".agent-one/providers.local.yaml:3"}, "apiKeyEnv": map[string]any{"value": "GW_KEY", "origin": "default"}}},
		"budgets":   map[string]any{"maxTokens": map[string]any{"value": "200000", "origin": "default"}},
	}
	b := newBoard(t, "workspace", Options{Sources: Sources{
		Config: func() any { return cfg },
		Off:    func() []Off { return []Off{{Feature: "tools.webfetch", Origin: ".agent-one/config.yaml:12"}} },
	}})
	code, agent := getLocal(t, b, "/api/config")
	if code != 200 {
		t.Fatal(code)
	}
	for _, want := range []string{`".agent-one/config.yaml:12"`, `"tools.webfetch"`, "claude-opus-5-5", `"live":true`, "GW_KEY", "redacted", `"200000"`} {
		if !strings.Contains(agent, want) {
			t.Errorf("config missing %q:\n%s", want, agent)
		}
	}
	if strings.Contains(agent, "sk-live-123") {
		t.Error("a credential reached the page")
	}
	_, agent = getLocal(t, newBoard(t, "workspace", Options{}), "/api/config")
	if !strings.Contains(agent, `"live":false`) || !strings.Contains(agent, "no config feed") || !strings.Contains(agent, `"offKnown":false`) {
		t.Errorf("no config feed → silent: %s", agent)
	}
}

func TestSubagentFeedAndAPI(t *testing.T) {
	usd := 0.12
	agents := []Agent{{Name: "zone-auth", Rank: "zone", State: "tool", Started: fixedNow.Add(-90 * time.Second), ContextTokens: 50000, ContextLimit: 200000, USD: &usd}}
	b := newBoard(t, "workspace", Options{Sources: Sources{Subagent: func() []Agent { return agents }}})
	_, js := get(t, b, "/api/subagent")
	var got struct {
		Live   bool    `json:"live"`
		Agents []Agent `json:"agents"`
	}
	if err := json.Unmarshal([]byte(js), &got); err != nil || !got.Live || len(got.Agents) != 1 {
		t.Errorf("api/subagent: %v %+v", err, got)
	}
	_, js = get(t, newBoard(t, "workspace", Options{}), "/api/subagent")
	if !strings.Contains(js, `"live":false`) || !strings.Contains(js, `"agents":[]`) {
		t.Errorf("without a feed the subagent is not live: %s", js)
	}
}

func TestUsageAPIAndDoc(t *testing.T) {
	b := newBoard(t, "workspace", Options{})
	_, js := get(t, b, "/api/usage?range=all")
	if !strings.Contains(js, `"byModel"`) || !strings.Contains(js, "zone-auth") {
		t.Errorf("api/usage: %s", js)
	}
	code, doc := get(t, b, "/api/doc?path=.agent-one/zone/auth/README.md")
	if code != 200 || !strings.Contains(doc, "zone-auth") {
		t.Errorf("api/doc: %d", code)
	}
	if code, _ := get(t, b, "/api/doc?path=../../go.mod"); code != 404 {
		t.Errorf("doc outside the workspace must 404, got %d", code)
	}
}

func TestSecretKey(t *testing.T) {
	for k, want := range map[string]bool{"apiKey": true, "apiKeyEnv": false, "password": true, "token": true, "maxTokens": false, "contextTokens": false, "model": false, "clientSecret": true} {
		if secretKey(k) != want {
			t.Errorf("secretKey(%q) = %v", k, !want)
		}
	}
}
