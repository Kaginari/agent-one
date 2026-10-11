package board

// The workspace's records, as JSON for the dashboard's Memory, Toolbox, Subagent and Config tabs: the
// memory tiers (and a read-only recall and review over them), the toolbox registry with what was
// offered and loaded, every journaled Subagent run with its steps, and the effective config with
// its origins and the off-list. Every answer carries its reading: a silent instrument says so.

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/memory"
)

// RunDetail is one journaled run read back whole: the commission, how it ended, every step.
type RunDetail struct {
	ID       string         `json:"id"`
	As       string         `json:"as"`
	Ask      string         `json:"ask"` // verbatim, the wire header included
	Status   string         `json:"status"`
	At       string         `json:"at"`
	Ended    string         `json:"ended,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Turns    int            `json:"turns"`
	Events   int            `json:"events"`
	MS       int            `json:"ms,omitempty"`
	Holes    []string       `json:"holes"`
	Context  map[string]any `json:"context,omitempty"`
	Steps    []RunStep      `json:"steps"`
	Exit     string         `json:"exit,omitempty"` // a run cut short: what cut it
}

// RunStep is one step of a run: the tool, how the gate classed it, what was asked, how it ended.
type RunStep struct {
	ID       string `json:"id"`
	Tool     string `json:"tool,omitempty"`
	Class    string `json:"class,omitempty"`
	Decision string `json:"decision,omitempty"` // the gate's: not-needed · approved · denied …
	Why      string `json:"why,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Status   string `json:"status"`
	Attempts int    `json:"attempts"`
	MS       int    `json:"ms,omitempty"`
}

var errNoRun = errors.New("no journaled run with that id")

// runDetail folds one journal again, keeping what the state fold drops: the gate's reasons, the
// act summaries, the end's holes and length.
func (f *fileSources) runDetail(id string) (*RunDetail, error) {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return nil, errNoRun
	}
	states, err := loop.Runs(f.loopDir())
	if err != nil {
		return nil, err
	}
	for _, s := range states {
		if s.ID != id {
			continue
		}
		events, _ := loop.ReadJournal(s.Path)
		d := &RunDetail{ID: s.ID, As: s.As, Ask: s.Ask, Status: s.Status, At: s.At, Provider: s.Provider, Turns: s.Turns, Events: s.Events, Holes: []string{}, Steps: []RunStep{}}
		if len(s.Context) > 0 {
			d.Context = map[string]any(s.Context)
		}
		idx := map[string]int{}
		step := func(id string) *RunStep {
			if i, ok := idx[id]; ok {
				return &d.Steps[i]
			}
			d.Steps = append(d.Steps, RunStep{ID: id, Status: "pending"})
			idx[id] = len(d.Steps) - 1
			return &d.Steps[len(d.Steps)-1]
		}
		str := func(e loop.Event, k string) string { v, _ := e[k].(string); return v }
		num := func(e loop.Event, k string) int { v, _ := e[k].(float64); return int(v) }
		for _, e := range events {
			sid := str(e, "id")
			switch str(e, "t") {
			case "gate":
				st := step(sid)
				st.Tool, st.Class, st.Decision, st.Why = str(e, "tool"), str(e, "effective"), str(e, "decision"), str(e, "why")
				if st.Decision == "denied" {
					st.Status = "denied"
				}
			case "act":
				st := step(sid)
				if t := str(e, "tool"); t != "" {
					st.Tool = t
				}
				if sm := str(e, "summary"); sm != "" {
					st.Summary = sm
				}
				if b, _ := e["start"].(bool); b {
					st.Status = "interrupted mid-act"
					st.Attempts = num(e, "attempt")
				}
			case "record":
				st := step(sid)
				st.Status, st.MS = str(e, "status"), num(e, "ms")
				if n := num(e, "attempts"); n > 0 {
					st.Attempts = n
				}
			case "end":
				d.Ended, d.MS = str(e, "at"), num(e, "ms")
				if hs, ok := e["holes"].([]any); ok {
					for _, h := range hs {
						if s, ok := h.(string); ok {
							d.Holes = append(d.Holes, s)
						}
					}
				}
			case "exit":
				d.Exit = str(e, "cause")
			}
		}
		return d, nil
	}
	return nil, errNoRun
}

func (f *fileSources) recall(q, tier, kind string) (*memory.RecallResult, error) {
	w, err := memory.OpenIn(f.root, f.workspaceDir)
	if err != nil {
		return nil, err
	}
	w.Now = f.now
	return w.Recall(q, memory.RecallOpts{Tier: tier, Kind: kind, K: 12, NoCache: true})
}

func (f *fileSources) review() (*memory.ReviewResult, error) {
	w, err := memory.OpenIn(f.root, f.workspaceDir)
	if err != nil {
		return nil, err
	}
	w.Now = f.now
	return w.Review(), nil
}

// askWords is a commission without its wire header (@ROOT, @ASK, @CAP …): the words a reader reads.
func askWords(ask string) string {
	var words []string
	for _, l := range strings.Split(ask, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "@") {
			words = append(words, l)
		}
	}
	return strings.Join(words, " ")
}

// readingJSON is a reading on the wire.
func readingJSON(r Reading) map[string]any {
	m := map[string]any{"text": r.String(), "lit": r.Lit, "src": r.Src}
	if !r.At.IsZero() {
		m["at"] = r.At
	}
	if r.Why != "" {
		m["why"] = r.Why
	}
	return m
}

// guarded serves a read of the workspace's own records only to a request addressed to this
// machine by a loopback name (a rebound DNS name cannot read the config or the memory).
func guarded(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !local(r) {
			http.Error(w, "refused: the board answers its own page", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func (b *Board) workspaceRoutes() {
	b.mux.HandleFunc("/api/memory", guarded(b.apiMemory))
	b.mux.HandleFunc("/api/memory/recall", guarded(b.apiRecall))
	b.mux.HandleFunc("/api/memory/review", guarded(b.apiReview))
	b.mux.HandleFunc("/api/toolbox", guarded(b.apiToolbox))
	b.mux.HandleFunc("/api/runs", guarded(b.apiRuns))
	b.mux.HandleFunc("/api/run", guarded(b.apiRun))
	b.mux.HandleFunc("/api/config", guarded(b.apiConfig))
}

func nz[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// apiMemory is the three tiers: short (the context window, the working notes, the semantic caches), long
// (the index), shared (the notes, by kind) — and the holes the instrument names.
func (b *Board) apiMemory(w http.ResponseWriter, r *http.Request) {
	m := b.src.Memory()
	out := map[string]any{"reading": readingJSON(m.Reading), "workingNotes": nz(m.WorkingNotes), "notes": nz(m.Notes), "byKind": m.ByKind, "status": nil, "holes": []string{}}
	if m.ByKind == nil {
		out["byKind"] = map[string]int{}
	}
	if st := m.Status; st != nil {
		out["status"] = map[string]any{"workspace": st.Workspace, "at": st.At, "short": st.Short, "long": st.Long, "shared": st.Shared}
		out["holes"] = nz(st.Holes)
	}
	writeJSON(w, out)
}

// apiRecall searches the memory read-only: ?q, ?tier (long|shared|all), ?kind.
func (b *Board) apiRecall(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "a recall needs ?q", http.StatusBadRequest)
		return
	}
	if len(q) > 500 {
		q = q[:500]
	}
	res, err := b.src.Recall(q, r.URL.Query().Get("tier"), r.URL.Query().Get("kind"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if res.Results == nil {
		res.Results = []memory.Result{}
	}
	if res.Holes == nil {
		res.Holes = []string{}
	}
	writeJSON(w, res)
}

// apiReview is the memory's own review: each tier read with its lens, every signal routed.
func (b *Board) apiReview(w http.ResponseWriter, r *http.Request) {
	res, err := b.src.Review()
	if err != nil {
		writeJSON(w, map[string]any{"tiers": []any{}, "silent": err.Error()})
		return
	}
	for i := range res.Tiers {
		res.Tiers[i].Facts = nz(res.Tiers[i].Facts)
		res.Tiers[i].Findings = nz(res.Tiers[i].Findings)
		for j := range res.Tiers[i].Findings {
			res.Tiers[i].Findings[j].Evidence = nz(res.Tiers[i].Findings[j].Evidence)
		}
	}
	writeJSON(w, res)
}

type toolEntry struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Path        string   `json:"path,omitempty"`
	Description string   `json:"description"`
	Serves      string   `json:"serves,omitempty"`
	Holders     []string `json:"holders"`
	Resident    float64  `json:"resident"`
	Full        float64  `json:"full"`
	State       string   `json:"state"` // loaded · offered · idle
	Offers      int      `json:"offers"`
	Loads       int      `json:"loads"`
	Mode        string   `json:"mode,omitempty"`
	Model       string   `json:"model,omitempty"`
	Usage       string   `json:"usage,omitempty"`
}

func deref[T any](p *T) T {
	var z T
	if p == nil {
		return z
	}
	return *p
}

func finite(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// apiToolbox is the registry (every skill, command, tool and agent it harvested), what briefs
// offered and agents loaded, and the loads journal's newest lines.
func (b *Board) apiToolbox(w http.ResponseWriter, r *http.Request) {
	v := b.src.Toolbox()
	out := map[string]any{"reading": readingJSON(v.Reading), "entries": []toolEntry{}, "recent": nz(v.Recent), "holes": []string{}, "missing": []string{}}
	entries := []toolEntry{}
	for _, e := range v.Entries {
		if e == nil {
			continue
		}
		te := toolEntry{Kind: e.Kind, Name: e.Name, Path: deref(e.Path), Description: e.Description, Serves: e.Serves, Holders: nz(e.Holders),
			Resident: finite(e.Cost.Resident), Full: finite(e.Cost.Full), State: "idle", Offers: v.Offers[e.Name], Loads: v.Loads[e.Name],
			Mode: deref(e.Mode), Model: deref(e.Model), Usage: deref(e.Usage)}
		switch {
		case v.Loaded[e.Name]:
			te.State = "loaded"
		case v.Offered[e.Name]:
			te.State = "offered"
		}
		entries = append(entries, te)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Name < entries[j].Name
	})
	out["entries"] = entries
	if st := v.Status; st != nil {
		reg := map[string]any{"live": st.Loaded.Live, "why": st.Loaded.Why, "stale": st.Stale}
		if st.Loaded.Reg != nil {
			reg["builtAt"] = st.Loaded.Reg.BuiltAt
			reg["head"] = deref(st.Loaded.Reg.Head)
		}
		out["registry"] = reg
		out["byKind"] = st.ByKind
		out["resident"], out["full"] = finite(st.Resident), finite(st.Full)
		out["external"] = st.Ext
		out["missing"] = nz(st.Missing)
		out["holes"] = nz(st.Holes)
		ins := st.Ins
		ins.ResidentTokens, ins.LoadedTokens = finite(ins.ResidentTokens), finite(ins.LoadedTokens)
		out["instrument"] = ins
	}
	writeJSON(w, out)
}

type runRow struct {
	ID       string `json:"id"`
	As       string `json:"as"`
	Ask      string `json:"ask"`
	Status   string `json:"status"`
	At       string `json:"at"`
	Turns    int    `json:"turns"`
	Steps    int    `json:"steps"`
	Failed   int    `json:"failed"`
	Provider string `json:"provider,omitempty"`
}

// apiRuns is the Subagent's history: every journaled run, newest first, with its counts. ?as
// keeps one agent's runs.
func (b *Board) apiRuns(w http.ResponseWriter, r *http.Request) {
	runs, rd := b.src.Loop()
	as := r.URL.Query().Get("as")
	rows := []runRow{}
	for _, x := range runs {
		if as != "" && x.As != as {
			continue
		}
		ask := askWords(x.Ask)
		if len(ask) > 240 {
			ask = ask[:240] + "…"
		}
		rows = append(rows, runRow{ID: x.ID, As: x.As, Ask: ask, Status: x.Status, At: x.At, Turns: x.Turns, Steps: x.Steps, Failed: x.Failed, Provider: x.Provider})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].At > rows[j].At })
	writeJSON(w, map[string]any{"reading": readingJSON(rd), "runs": rows, "at": b.opt.Now().Format(time.RFC3339)})
}

// apiRun is one run, whole: ?id.
func (b *Board) apiRun(w http.ResponseWriter, r *http.Request) {
	d, err := b.src.RunDetail(r.URL.Query().Get("id"))
	if err != nil || d == nil {
		http.Error(w, "no journaled run with that id", http.StatusNotFound)
		return
	}
	d.Holes, d.Steps = nz(d.Holes), nz(d.Steps)
	writeJSON(w, d)
}

// apiConfig is the effective config as the integrator resolved it — a tree whose leaves are
// {value, origin} — and the policy features it switched off, each with where that was written.
// Read-only: the board never writes config.
func (b *Board) apiConfig(w http.ResponseWriter, r *http.Request) {
	cfg := b.src.Config()
	rd := Reading{Src: "effective config (integrator)", Lit: b.liveCfg && cfg != nil, Read: b.opt.Now()}
	if !rd.Lit {
		rd.Why = "no config feed wired — the integrator supplies Sources.Config"
	}
	var tree any
	if rd.Lit {
		// through JSON: the page reads one shape whatever Go type the integrator handed over
		if raw, err := json.Marshal(cfg); err == nil {
			_ = json.Unmarshal(raw, &tree)
		}
		tree = redact(tree, false)
	}
	off := []map[string]string{}
	for _, o := range b.src.Off() {
		off = append(off, map[string]string{"feature": o.Feature, "origin": o.Origin})
	}
	// docs: what each key does, a line per path the tree holds (config.Describe)
	writeJSON(w, map[string]any{"reading": readingJSON(rd), "live": rd.Lit, "tree": tree, "docs": config.Docs(tree), "off": off, "offKnown": b.liveCfg})
}

// secretKey names a config key whose value is a credential: the page shows that one is set,
// never what it is (a key named …Env holds the variable's name, not the secret).
func secretKey(k string) bool {
	l := strings.ToLower(k)
	if strings.HasSuffix(l, "env") || strings.Contains(l, "tokens") {
		return false
	}
	for _, s := range []string{"apikey", "api_key", "password", "passwd", "secret", "token", "credential", "bearer"} {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// redact walks the config tree and blanks every non-empty string under a secret key.
func redact(v any, secret bool) any {
	switch x := v.(type) {
	case map[string]any:
		if _, origin, ok := originLeafOf(x); ok && secret {
			if s, isS := x["value"].(string); isS && s != "" {
				return map[string]any{"value": "•••• (set, redacted)", "origin": origin}
			}
			return x
		}
		for k, val := range x {
			x[k] = redact(val, secret || secretKey(k))
		}
		return x
	case []any:
		for i, val := range x {
			x[i] = redact(val, secret)
		}
		return x
	case string:
		if secret && x != "" {
			return "•••• (set, redacted)"
		}
	}
	return v
}

// originLeafOf says a map is a leaf the integrator wrapped as {value, origin}.
func originLeafOf(m map[string]any) (val any, origin string, ok bool) {
	if len(m) != 2 {
		return nil, "", false
	}
	o, hasO := m["origin"].(string)
	val, hasV := m["value"]
	return val, o, hasV && hasO
}
