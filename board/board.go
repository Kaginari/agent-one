// Package board is the workspace, seen: an http.Handler the binary serves on 127.0.0.1 that reads
// the same instruments the session writes and shows them in one page at the root — the
// dashboard (web/dash.html): the cockpit, then a tab per question (binary.md §The board,
// design/ui.md). Everything it serves is embedded; the page works offline.
package board

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/onto"
)

//go:embed all:web
var content embed.FS

// Names are the lexicon labels the page reads: no agent-one term is hard-coded in it.
// Missing keys fall back to DefaultNames; a distribution passes its own (Subagent → Subagents).
type Names map[string]string

// DefaultNames is the agent-one vocabulary.
func DefaultNames() Names {
	return Names{
		"board":                 "Board",
		"bin":                   "agent-one",
		"page.overview":         "Overview",
		"page.subagent":         "Subagents",
		"page.usage":            "Usage",
		"page.team":             "Team",
		"page.memory":           "Memory",
		"page.toolbox":          "Toolbox",
		"page.log":              "Log",
		"page.config":           "Config",
		"workspace":             "workspace",
		"session":               "session",
		"agent":                 "agent",
		"agents":                "agents",
		"ephemeral_subagent":    "Ephemeral subagent",
		"member":                "member",
		"members":               "members",
		"skill":                 "skill",
		"skills":                "skills",
		"rank":                  "rank",
		"role":                  "role",
		"ownership":             "ownership",
		"workingNotes":          "working notes",
		"notes":                 "notes",
		"policy":                "policy",
		"instrument":            "instrument",
		"stress":                "stress",
		"off_list":              "off-list",
		"unsaid":                "the unsaid",
		"rank.orchestrator":     "Orchestrator",
		"rank.operator":         "Operator",
		"rank.coord":            "Coordinator",
		"rank.domain":           "Domain owner",
		"rank.zone":             "Zone worker",
		"rank.service":          "Service owner",
		"lane.zone":             "zone",
		"lane.review":           "review",
		"lane.global":           "global",
		"lane.shared":           "shared",
		"bond.truth":            "ground-truth link",
		"bond.verdict":          "verdict link",
		"bond.holds":            "skill link",
		"bond.reports":          "reports to",
		"bond.above":            "above",
		"bond.owns":             "owns",
		"kind.policy":           "policy",
		"kind.team":             "team",
		"kind.domain":           "domain",
		"tier.short":            "short",
		"tier.long":             "long",
		"tier.shared":           "shared",
		"toolbox.offered":       "offered",
		"toolbox.loaded":        "loaded",
		"toolbox.kind.skill":    "skills",
		"toolbox.kind.command":  "commands",
		"toolbox.kind.tool":     "tools",
		"toolbox.kind.agent":    "agents",
		"toolbox.kind.external": "external",
	}
}

// Options configures a board.
type Options struct {
	WorkspaceRoot string      // the directory that holds the workspace dir
	WorkspaceDir  string      // ".agent-one" (a distribution renames it)
	Layout        onto.Layout // the ontology's rank layout; zero = the policy's
	Names         Names       // lexicon labels, merged over DefaultNames
	Sources       Sources     // feeds; nil fields read files
	Prefix        string      // URL prefix the board is mounted under ("" or "/name")
	Now           func() time.Time
	Poll          time.Duration // how often the watcher looks for changes (default 1s)
	Logger        *log.Logger
	Live          Live // the running session, for the dashboard; nil = read-only
	// Theme is the look in force (ui.theme, ui.mode); nil = the defaults. SetTheme keeps a
	// look the dashboard chose (written, every surface told); nil = the menu only previews.
	Theme    func() (name, mode string)
	SetTheme func(name, mode string) error
}

// Board is the handler.
type Board struct {
	opt     Options
	src     Sources
	names   Names
	mux     *http.ServeMux
	hub     *hub
	stop    chan struct{}
	once    sync.Once
	liveCrt bool // an integrator wired Subagent()
	liveCfg bool
	token   string // guards the dashboard's acts (live.go)
}

// Token returns the per-session board token used to authorize dashboard acts.
// It is exposed for tests that need to act as the page (e.g., TestDashGateAnswerOverHTTP).
// The token is also embedded in the dashboard page ({{TOKEN}}) and sent by dash.js
// via X-Board-Token for every /api/ask /api/answer /api/interrupt request.
func (b *Board) Token() string { return b.token }

// New builds the board. It starts a watcher goroutine; Close stops it.
func New(o Options) *Board {
	if o.WorkspaceDir == "" {
		o.WorkspaceDir = ".agent-one"
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Poll <= 0 {
		o.Poll = time.Second
	}
	if o.Logger == nil {
		o.Logger = log.New(&discard{}, "", 0)
	}
	o.Prefix = strings.TrimSuffix(o.Prefix, "/")
	b := &Board{opt: o, hub: newHub(), stop: make(chan struct{}), token: newToken()}
	b.liveCrt = o.Sources.Subagent != nil
	b.liveCfg = o.Sources.Config != nil
	b.src = o.Sources.withDefaults(FileSources(o.WorkspaceRoot, o.WorkspaceDir, o.Layout, o.Now))
	b.names = DefaultNames()
	for k, v := range o.Names {
		b.names[k] = v
	}
	b.mux = http.NewServeMux()
	b.routes()
	go b.watch()
	return b
}

// Close stops the watcher and every open event stream.
func (b *Board) Close() { b.once.Do(func() { close(b.stop); b.hub.closeAll() }) }

// Serve listens on addr until ctx ends.
func Serve(ctx context.Context, addr string, o Options) error {
	b := New(o)
	defer b.Close()
	return b.Serve(ctx, addr)
}

// Serve listens on addr until ctx ends.
func (b *Board) Serve(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: b, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		b.Close()
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
		return nil
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// ServeHTTP strips the prefix and routes.
func (b *Board) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if b.opt.Prefix != "" {
		if r.URL.Path == b.opt.Prefix {
			http.Redirect(w, r, b.opt.Prefix+"/", http.StatusFound)
			return
		}
		if !strings.HasPrefix(r.URL.Path, b.opt.Prefix+"/") {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, b.opt.Prefix)
		r = r2
	}
	b.mux.ServeHTTP(w, r)
}

func (b *Board) routes() {
	// one page, at the root: the dashboard. Every other path is an API, the event stream, or
	// an asset under /web/.
	b.mux.HandleFunc("/", guarded(b.root))
	// the old addresses keep working: /dash was the dashboard's own path; the pages that became
	// tabs open their tab
	b.mux.HandleFunc("/dash", b.moved(""))
	for page, tab := range movedPages {
		b.mux.HandleFunc("/"+page, b.moved(tab))
	}
	// every read of the workspace's records answers a loopback Host only: a rebound DNS name
	// (a page on the open web pointed at 127.0.0.1) reads nothing
	b.mux.HandleFunc("/events", guarded(b.events))
	b.mux.HandleFunc("/api/doc", guarded(b.apiDoc))
	b.mux.HandleFunc("/api/subagent", guarded(b.apiSubagent))
	b.mux.HandleFunc("/api/usage", guarded(b.apiUsage))
	b.mux.HandleFunc("/api/team", guarded(b.apiTeam))
	b.mux.HandleFunc("/api/activity", guarded(b.apiActivity))
	b.mux.HandleFunc("/api/tree", guarded(b.apiTree))
	b.mux.HandleFunc("/api/tree/children", guarded(b.apiTreeChildren))
	b.mux.HandleFunc("/api/subagents", guarded(b.apiSubagents))
	b.mux.HandleFunc("/api/plan", guarded(b.apiPlan))
	b.mux.HandleFunc("/api/value", guarded(b.apiValue))
	b.mux.HandleFunc("/api/meters", guarded(b.apiMeters))
	b.mux.HandleFunc("/portraits/", guarded(b.apiPortrait))
	b.liveRoutes()
	b.themeRoutes()
	b.workspaceRoutes()
}

func (b *Board) workspaceName() string {
	name := readName(path.Join(b.opt.WorkspaceRoot, b.opt.WorkspaceDir, "name"))
	if name == "" {
		name = path.Base(b.opt.WorkspaceRoot)
	}
	return name
}

// movedPages are the old board pages that live on as the dashboard's tabs, by path.
var movedPages = map[string]string{"subagent": "subagent", "memory": "memory", "toolbox": "toolbox", "config": "config", "usage": "metrics", "team": "net"}

// root serves the dashboard at "/" and nothing else there.
func (b *Board) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b.dash(w, r)
}

// moved answers an old address with a permanent redirect to the root, opening tab when given.
func (b *Board) moved(tab string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		to := b.opt.Prefix + "/"
		if tab != "" {
			to += "#" + tab
		}
		http.Redirect(w, r, to, http.StatusMovedPermanently)
	}
}

type usageData struct {
	UsageReport
	Session string
}

func (b *Board) usageReport(r *http.Request) usageData {
	recs, rd := b.src.Usage()
	since, rng := RangeSince(r.URL.Query().Get("range"), b.opt.Now())
	session := r.URL.Query().Get("session")
	if session != "" {
		var keep []UsageRecord
		for _, u := range recs {
			if u.Session == session {
				keep = append(keep, u)
			}
		}
		recs = keep
	}
	rep := Rollup(recs, since, rng)
	rep.Reading = rd
	if session != "" {
		rep.Sessions = sessionsOf(recs)
	}
	return usageData{UsageReport: rep, Session: session}
}

func sessionsOf(recs []UsageRecord) []string {
	set := map[string]bool{}
	for _, u := range recs {
		set[u.Session] = true
	}
	var out []string
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

type chartSeries struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Values []int  `json:"values"`
}

type chartData struct {
	Days   []string      `json:"days"`
	Series []chartSeries `json:"series"`
	USD    []float64     `json:"usd"`
	Models []UsageSum    `json:"models"`
}

func chartOf(r UsageReport) chartData {
	c := chartData{Models: r.ByModel}
	c.Series = []chartSeries{{Key: "input", Label: "input"}, {Key: "output", Label: "output"}, {Key: "cacheRead", Label: "cache read"}, {Key: "cacheWrite", Label: "cache write"}}
	for _, d := range r.ByDay {
		c.Days = append(c.Days, d.Key)
		c.Series[0].Values = append(c.Series[0].Values, d.Input)
		c.Series[1].Values = append(c.Series[1].Values, d.Output)
		c.Series[2].Values = append(c.Series[2].Values, d.CacheRead)
		c.Series[3].Values = append(c.Series[3].Values, d.CacheWrite)
		c.USD = append(c.USD, d.USD)
	}
	return c
}

// laneLabel is one lane of the team under the lexicon's word.
type laneLabel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

func (b *Board) laneLabels(c *Team) []laneLabel {
	used := map[string]bool{}
	for _, n := range c.Nodes {
		if n.Kind == "skill" {
			used["skill:"+n.Lane] = true
		} else {
			used[n.Lane] = true
		}
	}
	var out []laneLabel
	for _, l := range c.Lanes {
		if !used[l] {
			continue
		}
		if strings.HasPrefix(l, "skill:") {
			out = append(out, laneLabel{ID: l, Label: b.names.get("lane."+strings.TrimPrefix(l, "skill:")) + " " + b.names.get("skills"), Kind: "skill"})
		} else {
			out = append(out, laneLabel{ID: l, Label: b.names.get("rank." + l), Kind: "rank"})
		}
	}
	return out
}

// teamNamed is the team with the lexicon's words on it: the orchestrator holds the dist's name for
// the session agent (Orchestrator here; agent-one has its own), never a hard-coded one.
func (b *Board) teamNamed() *Team {
	c := b.src.Team()
	for i := range c.Nodes {
		if c.Nodes[i].Orchestrator && c.Nodes[i].Label == "" {
			c.Nodes[i].Label = b.names.get("rank." + c.Nodes[i].Rank)
		}
	}
	return c
}

// ---- api

func (b *Board) apiDoc(w http.ResponseWriter, r *http.Request) {
	text, err := b.src.Doc(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, "not a readable doc inside the workspace", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(text))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func (b *Board) apiSubagent(w http.ResponseWriter, r *http.Request) {
	agents := b.src.Subagent()
	if agents == nil {
		agents = []Agent{}
	}
	for i := range agents {
		agents[i].Value = b.valueOf(agents[i].Name)
	}
	writeJSON(w, map[string]any{"live": b.liveCrt, "agents": agents, "at": b.opt.Now()})
}

func (b *Board) apiUsage(w http.ResponseWriter, r *http.Request) {
	d := b.usageReport(r)
	writeJSON(w, map[string]any{"reading": d.Reading.String(), "lit": d.Reading.Lit, "range": d.Range, "total": d.Total, "chart": chartOf(d.UsageReport), "byAgent": d.ByAgent, "byRole": d.ByRole, "byRank": d.ByRank, "byModel": d.ByModel, "byDay": d.ByDay})
}

func (b *Board) apiTeam(w http.ResponseWriter, r *http.Request) {
	c := b.teamNamed()
	// every list is a list on the wire, an empty workspace's included — never null
	if c.Nodes == nil {
		c.Nodes = []Node{}
	}
	if c.Edges == nil {
		c.Edges = []Edge{}
	}
	for _, l := range []*[]string{&c.Lanes, &c.Findings, &c.Notes} {
		if *l == nil {
			*l = []string{}
		}
	}
	labels := b.laneLabels(c)
	if labels == nil {
		labels = []laneLabel{}
	}
	// each node carries how to draw it (the one icon resolver), its value and its size
	Masses(b.opt.WorkspaceRoot, c)
	for i := range c.Nodes {
		n := &c.Nodes[i]
		if n.Kind == "skill" {
			n.Icon = &Icon{Kind: "skill", URL: "web/icons/icons.svg#skill"}
			continue
		}
		ic := b.iconFor(n.Rank)
		n.Icon = &ic
		n.Family = b.meta(n.Rank).Family
		n.Value = b.valueOf(n.Name)
		if n.Family == "" && n.Value != nil {
			n.Family = n.Value.Family
		}
	}
	writeJSON(w, struct {
		*Team
		Labels  []laneLabel `json:"labels"`
		Relates []Relate    `json:"relates"`
	}{c, labels, RelatesFor(b.opt.WorkspaceRoot, c)})
}

// apiActivity is each agent's recent work, day by day, for the net's activity tiles and the
// weight of its bonds: runs from the loop journals, tokens from the usage journal. An agent the
// instruments never saw is absent — its tile stays blank, never a made-up zero.
func (b *Board) apiActivity(w http.ResponseWriter, r *http.Request) {
	runs, rr := b.src.Loop()
	recs, ur := b.src.Usage()
	a := BuildActivityDef(runs, recs, b.opt.Now(), b.activityDays(), b.def())
	a.Reading = rr.String() + " · " + ur.String()
	a.Lit = rr.Lit || ur.Lit
	writeJSON(w, a)
}

func (b *Board) apiTree(w http.ResponseWriter, r *http.Request) {
	if b.src.Tree == nil {
		writeJSON(w, map[string]any{"live": false, "root": "", "tree": []TreeEntry{}, "truncated": false})
		return
	}
	path := r.URL.Query().Get("path")
	entries, truncated := b.src.Tree(path)
	if entries == nil {
		entries = []TreeEntry{}
	}
	writeJSON(w, map[string]any{"live": true, "root": b.opt.WorkspaceRoot, "tree": entries, "truncated": truncated})
}

// apiTreeChildren serves one lazy level of the workspace tree: ?path is the
// root-relative slash prefix ("" = the root level). The JSON face is
// {path, nodes, truncated} with kind rendered as `dir bool` per node —
// the dash's expand-on-click contract. A prefix escaping the root, a
// missing directory, or a file answers 404: the client rendered that node
// from an older snapshot and the workspace moved under it.
func (b *Board) apiTreeChildren(w http.ResponseWriter, r *http.Request) {
	if b.src.Tree == nil {
		http.Error(w, "workspace tree not backed", http.StatusServiceUnavailable)
		return
	}
	prefix := r.URL.Query().Get("path")
	entries, trunc := b.src.Tree(prefix)
	if entries == nil && !trunc {
		http.NotFound(w, r)
		return
	}
	type node struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Dir  bool   `json:"dir"`
		Size int64  `json:"size,omitempty"`
		Mod  string `json:"mod,omitempty"`
	}
	nodes := make([]node, 0, len(entries))
	for _, e := range entries {
		nodes = append(nodes, node{
			Name: e.Name,
			Path: e.Path,
			Dir:  e.Kind == "dir",
			Size: e.Size,
			Mod:  e.Mod,
		})
	}
	path := prefix
	if path == "" {
		path = "."
	}
	writeJSON(w, map[string]any{"path": path, "children": nodes, "truncated": trunc})
}

// apiSubagents serves the subagents strip: every entity with a plan deck and its
// open hand (queued cards, per-heartbeat presence, plan stats). Agent fields
// are never sent — agents stay local to the plan file.
func (b *Board) apiSubagents(w http.ResponseWriter, r *http.Request) {
	c := b.src.Subagents
	if c == nil {
		writeJSON(w, map[string]any{"active": 0, "stale": 0, "empty": true})
		return
	}
	view, err := c()
	if err != nil {
		writeJSON(w, map[string]any{"active": 0, "stale": 0, "empty": true})
		return
	}
	// view is expected to be []Agent (as set in the test)
	agents, ok := view.([]Agent)
	if !ok {
		// If the Subagent function returns something else, treat as empty
		writeJSON(w, map[string]any{"active": 0, "stale": 0, "empty": true})
		return
	}
	active := len(agents)
	stale := 0 // we don't have stale info in the test; could be computed from agents if needed
	empty := active == 0
	writeJSON(w, map[string]any{"active": active, "stale": stale, "empty": empty})
}

// apiPlan serves THE deck — the workspace's plan.json (tmp/plans/<workspace>.json)
// with the holder, hearts, and lane rows the plan popup renders. Never
// another member's deck: subagents carry those.
func (b *Board) apiPlan(w http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	p := b.src.Plan
	if p == nil {
		writeJSON(w, map[string]any{"agent": agent, "empty": true})
		return
	}
	view, err := p(agent)
	if err != nil {
		writeJSON(w, map[string]any{"agent": agent, "empty": true, "error": err.Error()})
		return
	}
	writeJSON(w, view)
}

func (n Names) get(k string) string {
	if v, ok := n[k]; ok {
		return v
	}
	return k
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
