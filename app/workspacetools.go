package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/toolbox"
	"github.com/Kaginari/agent-one/workspace"
)

// workspaceTools are the tools that read the workspace itself (binary.md §The default toolset):
// recall · remember (the memory tiers) · toolbox (search the registry, load level 2) · onto
// (who owns a path, what a member may see) · skill (load a Skill) · workingNotes (working memory) ·
// drained (what left the context). Each honours its switch; a switched-off one is absent and
// the missing-tool policy names why.
func (a *App) workspaceTools(agent string) []*tool.Tool {
	var out []*tool.Tool
	on := func(name string) bool { ok, _ := a.Cfg.ToolEnabled(name); return ok }
	if on("recall") {
		out = append(out, a.recallTool(agent))
	}
	if on("remember") {
		out = append(out, a.rememberTool(agent))
	}
	if on("toolbox") {
		out = append(out, a.toolboxTool(agent))
	}
	if on("onto") {
		out = append(out, a.ontoTool())
	}
	if on("pulse") {
		out = append(out, tool.PulseTool())
	}
	if on("skill") {
		out = append(out, a.skillTool(agent))
	}
	if on("workingNotes") {
		out = append(out, a.workingNotesTool(agent))
	}
	if on("plan") {
		out = append(out, a.planTool(agent))
	}
	if on("queue") && a.Workspace != nil {
		out = append(out, tool.QueueTool(a.Workspace.Queue(), agent))
	}
	if a.Drainer != nil && a.Cfg.Compaction.Enabled {
		out = append(out, a.Drainer.RecallTool())
	}
	return out
}

// planItem is one line of an agent's plan deck: the step, its state, and whose zone it belongs to.
type planItem struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"` // pending · in_progress · done · blocked
	Who    string `json:"who,omitempty"`
	// Score and Why are the scenario funnel's (crafted-skill scenario-3-2-1) face on a card:
	// Score is the card's value verdict (0..1 or "0.4"), Why quotes the instrument rows that
	// earned it. A naked score without Why reads as a hole — the wire's honesty stands.
	Score string `json:"score,omitempty"`
	Why   string `json:"why,omitempty"`
}

// planDeck is the file: one deck per agent, machine-local (the plan dir lives under tmp).
type planDeck struct {
	Agent   string     `json:"agent"`
	Updated string     `json:"updated"`
	Items   []planItem `json:"items"`
}

var planPath = struct{ sepReplace *strings.Replacer }{strings.NewReplacer("/", "-", "\\", "-", ":", "-")}

func planFile(dir, agent string) string {
	return filepath.Join(dir, planPath.sepReplace.Replace(agent)+".json")
}

// planRead resolves a deck live-first, then the archive — an archived deck is memory
// kept browsable ('continue on last plan' re-pens it), never deleted and never a gate.
func planRead(dir, agent string) planDeck {
	var d planDeck
	if b, err := os.ReadFile(planFile(dir, agent)); err == nil {
		_ = json.Unmarshal(b, &d)
		return d
	}
	if b, err := os.ReadFile(planFile(planArchiveDir(dir), agent)); err == nil {
		_ = json.Unmarshal(b, &d)
	}
	return d
}

func planWrite(dir string, d planDeck) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	d.Updated = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(planFile(dir, d.Agent), append(b, '\n'), 0o644)
}

func renderPlan(d planDeck) string {
	var b strings.Builder
	fmt.Fprintf(&b, "plan — %s — %s\n", d.Agent, d.Updated)
	for _, it := range d.Items {
		who := ""
		if it.Who != "" {
			who = " → " + it.Who
		}
		score := ""
		if it.Score != "" {
			score = " — ≈ " + it.Score
			if it.Why != "" {
				score += " (" + it.Why + ")"
			} else {
				score += " (@? why missing — a naked score is a hole, per scenario-3-2-1)"
			}
		}
		fmt.Fprintf(&b, "- [%s] %s (%s%s)%s\n", it.Status, it.Text, it.ID, who, score)
	}
	return strings.TrimRight(b.String(), "\n")
}

// planTool is the agent's plan deck (the OpenCode discipline, native): think the order aloud
// here BEFORE dispatching — one line per slice, each named to the zone that owns it ('who'),
// exactly one in_progress (the loop walks one step). A planner (orchestrator, a coordinator, a domain owner) keeps
// its deck; `read` another agent's deck to see where its plan stands.
// planDir resolves the decks' directory (config override, else <workspace>/tmp/plans) — shared by
// the plan tool and the loop's PlanOpen reader.
func (a *App) planDir() string {
	cfg := a.liveConfig() // the TUI polls it while an approval reloads the config
	dir := cfg.Plan.Dir
	if dir == "" {
		dir = filepath.Join(cfg.Dist.WorkspaceDir, "tmp", "plans")
	}
	return filepath.Join(a.Root, dir)
}

// processBoot approximates a session's provisioning for the plan dir when nobody set one (single-shot
// runs). Decks penned before the session's provisioning belong to an earlier session's working memory:
// they stay on disk (an agent's "continue on last plan" reads them — memory, never a jailer),
// but they never show as live on the rail and never hold a new session's PlanOpen end-gate
// (the 2026-10-02 orchestrator deck greeting the 2026-10-03 relaunch was both lies at once).
var processBoot = time.Now()

// plansBoot is the session's provisioning instant: interactive modes stamp it exactly
// (App.planBoot), single-shot runs ride the process boot.
func (a *App) plansBoot() time.Time {
	if !a.planBoot.IsZero() {
		return a.planBoot
	}
	return processBoot
}

// planArchiveDir is where the boot sweep moves last session's decks: <live>/archive —
// append-only, browsable (planRead falls through), never the session's live intent.
func planArchiveDir(live string) string { return filepath.Join(live, "archive") }

// planArchive runs the boot sweep (LEC-2026-10-04-STALE-PLANS Order 1): stale decks move
// out of the live dir into the archive; it never archives other's decks and returns their
// count. Callers pass their own agent — the session's own deck never moves even when stale
// (the 'continue on last plan' lifeline stands: re-pen it, don't obey it blindly). Renames
// only, additive — an existing archived twin is left untouched, the stale live deck stays,
// and nothing under the archive is ever deleted by this path. Fail-open on unparsable
// stamps holds: a corrupted stamp reads not-stale and stays live, as planStale rules.
func planArchive(dir, agent string, boot time.Time) (int, error) {
	if err := os.MkdirAll(planArchiveDir(dir), 0o755); err != nil {
		return 0, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, p := range matches {
		name := strings.TrimSuffix(filepath.Base(p), ".json")
		if name == planPath.sepReplace.Replace(agent) {
			continue
		}
		d, err := planReadLive(dir, name)
		if err != nil || d.Agent == "" {
			continue // unreadable or bodyless file: not ours to judge — it stays live
		}
		if !planStale(d, boot) {
			continue
		}
		dst := planFile(planArchiveDir(dir), name)
		if _, err := os.Stat(dst); err == nil {
			continue // an archived twin already holds this name — append-only defers the rename,
			// the stale deck stays live rather than overwrite the archive's memory; the next sweep re-tries
		}
		if err := os.Rename(p, dst); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}

// planReadLive reads only the live dir (os.ReadFile, stricter than planRead's fall-through):
// the sweep judges what the live dir actually holds, never the archive's memory.
func planReadLive(dir, agent string) (planDeck, error) {
	var d planDeck
	b, err := os.ReadFile(planFile(dir, agent))
	if err != nil {
		return d, err
	}
	err = json.Unmarshal(b, &d)
	return d, err
}

// planUpdatedAt parses a deck's stamp; an unparsable/unstamped deck is NOT stale (fail-open:
// a corrupted stamp must never close a gate or hide an agent's own living deck).
func planUpdatedAt(d planDeck) (time.Time, bool) {
	t, err := time.Parse("2006-01-02T15:04:05Z", d.Updated)
	return t, err == nil
}

// planStale is true when the deck was penned before this session's provisioning. Deck stamps carry
// second precision — the boot is cut to the same so a deck penned the very second the session
// opens reads current, never phantom-stale on a sub-second sliver.
func planStale(d planDeck, boot time.Time) bool {
	t, ok := planUpdatedAt(d)
	return ok && t.Before(boot.Truncate(time.Second))
}

// planOpenReason is "" when the agent's deck may close, else one line naming its first open
// item. Decks from an earlier session never hold the gate — they are memory of that session's
// intent, not this one's outstanding work.
func planOpenReason(dir, agent string, boot time.Time) string {
	d := planRead(dir, agent)
	if planStale(d, boot) {
		return ""
	}
	for _, it := range d.Items {
		switch it.Status {
		case "pending", "in_progress", "blocked":
			return fmt.Sprintf("%s is %s — %s", it.ID, it.Status, it.Text)
		}
	}
	return ""
}

// renderPlanWith is renderPlan plus the staleness banner a stale deck owes its reader: the
// deck is last session's written intent — continue it by re-penning, don't obey it blindly.
func renderPlanWith(d planDeck, boot time.Time) string {
	if !planStale(d, boot) || len(d.Items) == 0 {
		return renderPlan(d)
	}
	return fmt.Sprintf("plan: stale deck — penned %s, before this session; it is memory of last session's intent, not your outstanding work — re-pen it to continue it\n%s", d.Updated, renderPlan(d))
}

func (a *App) planTool(agent string) *tool.Tool {
	dir := a.planDir()
	swept := false // the boot sweep runs once per process, at the first plan act after the dir resolves
	return &tool.Tool{
		Name:        "plan",
		Description: "Your plan deck — the todo list you write before dispatching. `items` replaces the whole deck (each line: text · status pending|in_progress|done|blocked · who the slice belongs to); `tick` moves one item; `read` another agent's deck. One in_progress at a time — the loop walks one step.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"text":{"type":"string"},"status":{"type":"string"},"who":{"type":"string"},"score":{"type":"string","description":"the card's value verdict (scenario-3-2-1): read off instruments"},"why":{"type":"string","description":"the instrument rows that earned the score"}}}},"tick":{"type":"string","description":"the id of the item to move"},"status":{"type":"string","enum":["pending","in_progress","done","blocked"]},"read":{"type":"string","description":"another agent's deck"},"drop":{"type":"boolean"}}}`),
		Class:       tool.Write,
		Classify: func(env tool.Env, in json.RawMessage) tool.Classification {
			var q struct {
				Items []json.RawMessage
				Tick  string
				Drop  bool
			}
			_ = json.Unmarshal(in, &q)
			if len(q.Items) == 0 && q.Tick == "" && !q.Drop {
				return tool.Classification{Class: tool.Read, Why: "reads a plan deck"}
			}
			return tool.Classification{Class: tool.Write, Why: "writes a plan deck (" + dir + ")"}
		},
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			if !swept {
				if _, err := planArchive(dir, agent, a.plansBoot()); err != nil {
					return failResult("plan: boot sweep: %v", err) // fail-loud: a half-swept live dir must say so
				}
				swept = true
			}
			var q struct {
				Items  []planItem
				Tick   string
				Status string
				Read   string
				Drop   bool
			}
			if err := json.Unmarshal(in, &q); err != nil {
				return failResult("plan: %v", err)
			}
			switch {
			case q.Read != "":
				d := planRead(dir, q.Read)
				if len(d.Items) == 0 {
					return tool.Result{Output: q.Read + " has no deck yet"}
				}
				return tool.Result{Output: renderPlanWith(d, a.plansBoot())}
			case q.Drop:
				_ = os.Remove(planFile(dir, agent))
				return tool.Result{Output: "plan dropped — " + relOrAbs(a.Root, planFile(dir, agent))}
			case len(q.Items) > 0:
				var seen map[string]bool = map[string]bool{}
				inProg := 0
				for i := range q.Items {
					if strings.TrimSpace(q.Items[i].Text) == "" {
						return failResult("plan: item %d has an empty text", i+1)
					}
					if q.Items[i].ID == "" {
						q.Items[i].ID = fmt.Sprintf("%d", i+1)
					}
					if seen[q.Items[i].ID] {
						return failResult("plan: duplicate id %q", q.Items[i].ID)
					}
					seen[q.Items[i].ID] = true
					st := strings.ToLower(q.Items[i].Status)
					if st == "" {
						st = "pending"
					}
					if st != "pending" && st != "in_progress" && st != "done" && st != "blocked" {
						return failResult("plan: status %q is not pending|in_progress|done|blocked", st)
					}
					if st == "in_progress" {
						inProg++
					}
					q.Items[i].Status = st
				}
				d := planDeck{Agent: agent, Items: q.Items}
				if err := planWrite(dir, d); err != nil {
					return failResult("plan: %v", err)
				}
				out := fmt.Sprintf("plan: %d item%s → %s", len(q.Items), plural(len(q.Items)), relOrAbs(a.Root, planFile(dir, agent)))
				if inProg > 1 {
					out += fmt.Sprintf("\n⚠ %d items marked in_progress — the loop walks one step at a time", inProg)
				}
				return tool.Result{Output: out + "\n" + renderPlan(d)}
			case q.Tick != "":
				if q.Status == "" {
					return failResult("plan: tick needs a status")
				}
				d := planRead(dir, agent)
				if len(d.Items) == 0 {
					return failResult("plan: no deck for %s — write it first", agent)
				}
				for i := range d.Items {
					if d.Items[i].ID == q.Tick {
						st := strings.ToLower(q.Status)
						if st != "pending" && st != "in_progress" && st != "done" && st != "blocked" {
							return failResult("plan: status %q is not pending|in_progress|done|blocked", st)
						}
						d.Items[i].Status = st
						if err := planWrite(dir, d); err != nil {
							return failResult("plan: %v", err)
						}
						out := fmt.Sprintf("plan: #%s → %s\n%s", q.Tick, st, renderPlan(d))
						other := 0
						for _, it := range d.Items {
							if it.ID != q.Tick && it.Status == "in_progress" {
								other++
							}
						}
						if st == "in_progress" && other > 0 {
							out += "\n⚠ another item is already in_progress — one step at a time"
						}
						return tool.Result{Output: out}
					}
				}
				return failResult("plan: no item %q in %s's deck", q.Tick, agent)
			default:
				d := planRead(dir, agent)
				if len(d.Items) == 0 {
					return tool.Result{Output: "no deck yet — `items` writes it, one line per slice"}
				}
				return tool.Result{Output: renderPlan(d)}
			}
		},
	}
}

func failResult(format string, args ...interface{}) tool.Result {
	return tool.Result{Output: fmt.Sprintf(format, args...), Err: true}
}

func (a *App) recallTool(agent string) *tool.Tool {
	kinds := strings.Join([]string{a.Lex.Token("policy"), a.Lex.Token("team"), a.Lex.Token("domain")}, "|")
	return &tool.Tool{
		Name:        "recall",
		Description: "Recall from the memory tiers by meaning (long: the workspace's docs and log; shared: notes). Returns anchors (path#section) and snippets, never whole files. kind filters shared notes by " + kinds + ".",
		Schema:      json.RawMessage(`{"type":"object","properties":{"question":{"type":"string"},"k":{"type":"integer"},"tier":{"type":"string","enum":["long","shared","all"]},"kind":{"type":"string"}},"required":["question"]}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Question, Tier, Kind string
				K                    int
			}
			if err := json.Unmarshal(in, &q); err != nil || strings.TrimSpace(q.Question) == "" {
				return failResult("recall: question is required")
			}
			m, err := a.Workspace.Memory()
			if err != nil {
				return failResult("recall: %v", err)
			}
			if _, why := m.LoadIndex(); why != "" && a.Cfg.Memory.Long.RebuildOnStale {
				_, _ = m.Index()
			}
			kind := q.Kind
			if k, ok := a.Lex.Canonical(kind); ok {
				kind = k
			}
			k := q.K
			if k <= 0 {
				k = a.Cfg.Memory.Recall.K
			}
			res, err := m.Recall(q.Question, memory.RecallOpts{As: agent, Tier: q.Tier, Kind: kind, K: k, NoCache: !a.Cfg.Memory.Short.Enabled})
			if err != nil {
				return failResult("recall: %v", err)
			}
			var b strings.Builder
			for _, r := range res.Results {
				b.WriteString(memory.FormatResult(r) + "\n")
				if snip := strings.TrimSpace(r.Snippet); snip != "" {
					b.WriteString("   " + firstLine(snip) + "\n")
				}
			}
			for _, h := range res.Holes {
				b.WriteString("@? " + h + "\n")
			}
			if b.Len() == 0 {
				return tool.Result{Output: "no memory matches " + q.Question}
			}
			return tool.Result{Output: strings.TrimRight(b.String(), "\n")}
		},
	}
}

func (a *App) rememberTool(agent string) *tool.Tool {
	kinds := strings.Join([]string{a.Lex.Token("policy"), a.Lex.Token("team"), a.Lex.Token("domain")}, "|")
	return &tool.Tool{
		Name:        "remember",
		Description: "Append a shared note (workspace tier; machine: true for every workspace on this machine). kind marks the unsaid: " + kinds + ". Notes are append-only.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"},"kind":{"type":"string"},"tag":{"type":"string"},"machine":{"type":"boolean"}},"required":["text"]}`),
		Class:       tool.Write,
		Classify: func(env tool.Env, in json.RawMessage) tool.Classification {
			var q struct{ Machine bool }
			_ = json.Unmarshal(in, &q)
			if q.Machine {
				return tool.Classification{Class: tool.Write, Why: "appends a machine-shared note (~/" + a.Cfg.Dist.WorkspaceDir + ")"}
			}
			return tool.Classification{Class: tool.Write, Why: "appends a workspace note"}
		},
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Text, Kind, Tag string
				Machine         bool
			}
			if err := json.Unmarshal(in, &q); err != nil || strings.TrimSpace(q.Text) == "" {
				return failResult("remember: text is required")
			}
			for _, t := range workspace.ForgeTags {
				if strings.EqualFold(strings.TrimSpace(q.Tag), t) {
					return failResult("remember: the %q tag is the harness's own — drafts ride your report as `@FORGE skill:<name> | <when> | <how>` (or `@FORGE tool:<name> | <summary> | <argv>`) and the harness files them; the working notes registers", t)
				}
			}
			if q.Machine && !a.Cfg.Memory.Shared.Machine.Enabled {
				return failResult("remember: the machine tier is off (memory.shared.machine.enabled)")
			}
			if !q.Machine && !a.Cfg.Memory.Shared.Workspace.Enabled {
				return failResult("remember: the workspace tier is off (memory.shared.workspace.enabled)")
			}
			kind := q.Kind
			if kind != "" {
				k, ok := a.Lex.Canonical(kind)
				if !ok {
					return failResult("remember: kind %q is not %s", kind, kinds)
				}
				kind = k
			}
			m, err := a.Workspace.Memory()
			if err != nil {
				return failResult("remember: %v", err)
			}
			r, err := m.Remember(q.Text, memory.RememberOpts{As: agent, Kind: kind, Tag: q.Tag, Machine: q.Machine})
			if err != nil {
				return failResult("remember: %v", err)
			}
			return tool.Result{Output: fmt.Sprintf("noted (%s, %s)", r.Scope, r.Src), Wrote: nil}
		},
	}
}

func (a *App) toolboxTool(agent string) *tool.Tool {
	return &tool.Tool{
		Name:        "toolbox",
		Description: "Search the toolbox registry (Skills, commands, tools, Agents, externals, MCP tools, switched-off tools) by question — level 1: names, one line, cost; or load one entry's agent by name (level 2, journaled). Loading is your own decision, never pre-emptive.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"load":{"type":"string","description":"entry name to load (level 2)"},"section":{"type":"integer","description":"one section of the loaded entry"},"k":{"type":"integer"}},"required":[]}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Query, Load string
				Section     *int
				K           int
			}
			if err := json.Unmarshal(in, &q); err != nil {
				return failResult("toolbox: %v", err)
			}
			tb, err := a.Workspace.Toolbox()
			if err != nil {
				return failResult("toolbox: %v", err)
			}
			if q.Load != "" {
				if r, err := tb.Load(q.Load, toolbox.LoadOpts{As: agent, Sec: q.Section}); err == nil {
					return tool.Result{Output: r.Text}
				}
				if s, ok := a.Found.Skill(q.Load); ok {
					text, err := s.Load()
					if err != nil {
						return failResult("toolbox: %v", err)
					}
					return tool.Result{Output: text}
				}
				if t, ok := a.mcpTool(q.Load); ok {
					return tool.Result{Output: fmt.Sprintf("%s — %s\nclass %s\nschema %s", t.Name, t.Description, t.Class, string(t.Schema))}
				}
				return failResult("toolbox: nothing named %q — search first", q.Load)
			}
			if strings.TrimSpace(q.Query) == "" {
				return failResult("toolbox: query or load is required")
			}
			if L := tb.LoadRegistry(); L.Live || tb.IsStale(L) {
				_, _ = tb.Index()
			}
			var b strings.Builder
			if br, err := tb.Brief(q.Query, toolbox.PickOpts{As: agent, Budget: a.Cfg.Toolbox.BudgetTokens, K: q.K}); err == nil {
				for _, l := range br.Lines {
					b.WriteString(l + "\n")
				}
			} else {
				b.WriteString("@? registry: " + err.Error() + "\n")
			}
			// the registry the files hold, plus what only the session knows: mcp tools, off tools
			words := strings.Fields(strings.ToLower(q.Query))
			hit := func(s string) bool {
				s = strings.ToLower(s)
				for _, w := range words {
					if strings.Contains(s, w) {
						return true
					}
				}
				return false
			}
			if a.MCP != nil {
				for _, t := range a.MCP.Tools() {
					if hit(t.Name + " " + t.Description) {
						fmt.Fprintf(&b, "@T mcp %s · %s · class %s\n", t.Name, firstLine(t.Description), t.Class)
					}
				}
			}
			for name, why := range a.Shelf.Disabled() {
				if hit(name) {
					fmt.Fprintf(&b, "@T off %s · %s\n", name, why)
				}
			}
			if b.Len() == 0 {
				return tool.Result{Output: "nothing in the toolbox matches " + q.Query}
			}
			return tool.Result{Output: strings.TrimRight(b.String(), "\n")}
		},
	}
}

func (a *App) mcpTool(name string) (*tool.Tool, bool) {
	if a.MCP == nil {
		return nil, false
	}
	for _, t := range a.MCP.Tools() {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

func (a *App) ontoTool() *tool.Tool {
	return &tool.Tool{
		Name:        "onto",
		Description: "Query the member graph: who owns a path (owner — the dispatch card: owner, strain, domain, and the widen/birth/route arms), what a member may see (member: its projection), or the roster (neither).",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"member":{"type":"string"},"budget":{"type":"integer"}}}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Path, Member string
				Budget       int
			}
			_ = json.Unmarshal(in, &q)
			switch {
			case q.Path != "":
				rel := strings.TrimPrefix(env.Rel(env.Resolve(q.Path)), "./")
				return tool.Result{Output: a.Workspace.DispatchCard(rel)}
			case q.Member != "":
				if a.Workspace.Onto == nil {
					return failResult("onto: the ontology is off (ontology.enabled)")
				}
				budget := q.Budget
				if budget <= 0 {
					budget = a.Cfg.Ontology.ProjectBudgetTokens
				}
				lines, err := a.Workspace.Onto.Project(strings.ToLower(q.Member), budget)
				if err != nil {
					return failResult("onto: %v", err)
				}
				if len(lines) == 0 {
					return tool.Result{Output: q.Member + " sees nothing yet"}
				}
				return tool.Result{Output: strings.Join(lines, "\n")}
			}
			var b strings.Builder
			for _, c := range a.Workspace.Members {
				fmt.Fprintf(&b, "%s (%s) owns %s", c.Name, c.Rank, orStr(strings.Join(c.Ownership, ", "), "nothing"))
				if c.Parent != "" {
					fmt.Fprintf(&b, " → %s", c.Parent)
				}
				b.WriteString("\n")
			}
			if b.Len() == 0 {
				return tool.Result{Output: "the roster is empty: no member under " + a.Cfg.Dist.WorkspaceDir + "/"}
			}
			return tool.Result{Output: strings.TrimRight(b.String(), "\n")}
		},
	}
}

func (a *App) skillTool(agent string) *tool.Tool {
	return &tool.Tool{
		Name:        "skill",
		Description: "Import a skill by name: load its full agent (level 2). Names come from the toolbox manifest (@T skill …) and the skill dirs.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"section":{"type":"integer"}},"required":["name"]}`),
		Class:       tool.Read,
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Name    string
				Section *int
			}
			if err := json.Unmarshal(in, &q); err != nil || q.Name == "" {
				return failResult("skill: name is required")
			}
			if tb, err := a.Workspace.Toolbox(); err == nil {
				if r, err := tb.Load(q.Name, toolbox.LoadOpts{As: agent, Kind: "skill", Sec: q.Section}); err == nil {
					return tool.Result{Output: r.Text}
				}
			}
			s, ok := a.Found.Skill(q.Name)
			if !ok {
				var names []string
				for _, sk := range a.Found.Skills {
					names = append(names, sk.Name)
				}
				return failResult("skill: no Skill named %q (known: %s)", q.Name, orStr(strings.Join(names, ", "), "none"))
			}
			text, err := s.Load()
			if err != nil {
				return failResult("skill: %v", err)
			}
			return tool.Result{Output: text}
		},
	}
}

// workingNotesTool is the working memory: at most ~5 dated notes in
// <workspace>/instruments/working-notes/<session>.md; over the limit is a stress reading, said out loud.
func (a *App) workingNotesTool(agent string) *tool.Tool {
	limit := a.Cfg.Tools.WorkingNotes.Limit
	if limit <= 0 {
		limit = memory.WorkingNotesLimit
	}
	return &tool.Tool{
		Name:        "workingNotes",
		Description: fmt.Sprintf("Your working memory: the goal, where the plan stands, decisions and why, open holes, the next step — at most %d notes. `notes` replaces the working notes; `add` appends one; neither reads it.", limit),
		Schema:      json.RawMessage(`{"type":"object","properties":{"notes":{"type":"array","items":{"type":"string"}},"add":{"type":"string"}}}`),
		Class:       tool.Write,
		Classify: func(env tool.Env, in json.RawMessage) tool.Classification {
			return tool.Classification{Class: tool.Write, Why: "writes the workingNotes"}
		},
		Run: func(ctx context.Context, env tool.Env, in json.RawMessage) tool.Result {
			var q struct {
				Notes []string
				Add   string
			}
			_ = json.Unmarshal(in, &q)
			// the orchestrator keeps the session's one working notes; a subagent's is its own run's, the file the
			// drainer and a resume read — never the orchestrator's working memory written over
			run := env.RunID
			if agent == workspace.Orchestrator || run == "" {
				run = a.SessionID
			}
			p := a.workingNotesPath(run)
			cur := readWorkingNotes(p)
			switch {
			case len(q.Notes) > 0:
				cur = q.Notes
			case strings.TrimSpace(q.Add) != "":
				cur = append(cur, strings.TrimSpace(q.Add))
			default:
				if len(cur) == 0 {
					return tool.Result{Output: "the working notes is empty"}
				}
				return tool.Result{Output: renderWorkingNotes(cur)}
			}
			if err := writeWorkingNotes(p, agent, cur); err != nil {
				return failResult("workingNotes: %v", err)
			}
			out := fmt.Sprintf("workingNotes: %d note%s → %s", len(cur), plural(len(cur)), relOrAbs(a.Root, p))
			if len(cur) > limit {
				out += fmt.Sprintf("\n@? the working notes holds %d notes, over the limit of %d — a stress reading (Principle 5): consolidate to a rule, a invariant or a wrap-up, then let go", len(cur), limit)
			}
			return tool.Result{Output: out}
		},
	}
}

func readWorkingNotes(p string) []string {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "- ") {
			out = append(out, strings.TrimPrefix(l, "- "))
		}
	}
	return out
}

func renderWorkingNotes(notes []string) string {
	var b strings.Builder
	for _, t := range notes {
		b.WriteString("- " + t + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeWorkingNotes(p, agent string, notes []string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	head := fmt.Sprintf("# workingNotes — %s — %s\n\n", agent, time.Now().UTC().Format("2006-01-02"))
	return os.WriteFile(p, []byte(head+renderWorkingNotes(notes)+"\n"), 0o644)
}

func sortedNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
