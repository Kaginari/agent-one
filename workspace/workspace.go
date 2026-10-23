package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/onto"
)

// Member is one ranked agent read off its doc — through onto's derivation, never a second
// parser: rank, ownership, parent bond, held skills; plus its verify commands (invariants as
// readings) and the doc that owns it.
type Member struct {
	Name      string   // zone-auth (zone-auth under agent-one)
	Rank      string   // the rank's name in the table (zone, domain, coord, service, or a custom one)
	Doc       string   // relative path of the owning doc
	Dir       string   // relative path of the member's directory
	Ownership []string // normalized path prefixes it owns
	Parent    string   // one hop up (domain owner for a zone worker, coordinator for a domain owner, orchestrator for a coordinator); a retired parent's promoted stands in for it
	// PromotedOf names the base this agent promoted from (its doc's `Promoted from` line); "" for
	// an agent that never promoted
	PromotedOf string
	Skills     []string // held skills
	Verify     []string // shell commands whose exit codes are the invariants' reading
}

// Options is what Open needs beyond the root and the lexicon.
type Options struct {
	Instructions InstructionOptions
	Ontology     bool         // load the ontology (members need it; off = an empty roster)
	Ranks        Ranks        // the rank table; nil = DefaultRanks(lex) (config's `rankSet: replace` passes the whole table)
	Value        ValueOptions // policy.value: the one definition of success and the ledger's window
	Evolution    EvolutionOptions
}

// Workspace is one opened workspace.
type Workspace struct {
	Root string
	// ToolboxExtra are externals declared in config (registry.tools), for the toolbox.
	ToolboxExtra []json.RawMessage
	Lex          Lexicon
	Ranks        Ranks
	Policy       *Policy
	// System is the parsed `.agent-one/system.yaml` (nil when absent or malformed — the
	// hole sits in Notes). It grounds the git self-init at open: a manifest root outside any
	// work-tree is versioned once, at provisioning.
	Manifest     *System
	Onto         *onto.Workspace
	Members      []Member
	Instructions []Instruction
	Notes        []string // holes met while opening

	// Foreign are agents read from another harness's agent files (discovery.agents): they
	// stay on the roster across Reload, beside the members derived from the docs.
	Foreign []Member

	mu       sync.Mutex
	sessions map[*loop.Engine]*loop.Session
	seats    map[string]*loop.Engine // agent → the one subagent sitting it (seat, Close)
	closers  map[*loop.Engine]func()
	// subagentWrote is the subagent-write ledger's SAME-ENGINE LANE: what the Subagents of a
	// dispatcher's engine wrote and gated on their own account, kept as path → {writer, at};
	// the dispatcher's end gate leaves those paths alone (never a second gate), and the
	// orchestrator's second pairing arm (gate.go, decree 2026-10-01) reads this lane to attribute
	// a doc on disk to this session's own subagents when the code lands a turn later.
	subagentWrote map[*loop.Engine]map[string]subagentWrite
	// subagentWrites is the subagent-write ledger's CROSS-SESSION LANE (3rd-strike fix,
	// 2026-10-03): the same record doubled under the session member's name, workspace-resident
	// instead of engine-keyed. Subagent authoring at turn N survives both the subagent engine's
	// and the dispatcher engine's Close, so when the orchestrator lands code on that soil at turn
	// N+1 the right-author arm can pair code and doc by member name. Every write mirrors
	// into subagentWrote — gate.go's read stays put (OUT-2); Close clears per-engine state
	// only and never this lane. Bounded per member (subagentWritesBound entries) because an
	// unbounded map gains an entry every subagent turn forever.
	subagentWrites map[string]map[string]subagentWrite
	// promote is the promotion engine: the watch list and the register when a condition crosses. nil
	// when no rank declares promotesWhen.
	promote *promoteState
	// val is the value ledger (workspace/value.go): every run of every agent, always kept.
	val    *valueState
	valOpt ValueOptions
	// the council ledger: per-agent governance strain — a sum of the runs' sent-backs (failed
	// acts standing, gate re-asks, wire re-asks) plus one per failed or stalled run, reset by a
	// clean landing. At policy.stress.escalateAt the dispatcher stamps a council warrant into the
	// report so the agent's manager answers; consults bounds the warrants an agent may carry.
	stress   map[string]int
	consults map[string]int
	// stressDeaths records which member last died under the subagent clock on which lane —
	// the memo the dispatcher consults before re-dispatching the same agent. Configuration
	// mutation is advisory (the memo's next-lane hint is printed into the report), never
	// silent: Operator's 2026-10-02 policy — the agents carry the memory, they don't swap DNA alone.
	stressDeaths map[string]stressEnd
	// pairMarks + gateStreaks carry the 2026-10-07 session policies: doc-move marks (arm-3
	// pairing, transactional — the gate spends them) and circuit-breaker streaks (the
	// same gate failing on the same files for the same agent hardens into a spec directive).
	pairMarks   map[string]time.Time
	gateStreaks map[string]*gateStreak
	// receipts is the write book every engine of this workspace shares: a change the tree watch sees
	// that another run's tool reported writing is that run's (loop.Receipts)
	receipts *loop.Receipts
	// evo is policy.evolution: the armed options, this session's evolutions (workspace/evolve.go)
	evo evoState
}

// Discover walks up from dir for the workspace dir.
func Discover(dir string) (string, Lexicon, error) {
	l := Default()
	root, err := onto.FindRootIn(dir, l.Layout())
	if err != nil {
		return "", Lexicon{}, err
	}
	return root, l, nil
}

// Open loads the workspace at root under a lexicon: the policy, the ontology and its members, the
// instructions. A missing policy is a note, not a failure — the principles is then empty and the
// system prompt says so.
func Open(root string, lex Lexicon, opt Options) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(filepath.Join(abs, lex.WorkspaceDir)); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("no %s/ under %s", lex.WorkspaceDir, abs)
	}
	w := &Workspace{Root: abs, Lex: lex, Ranks: opt.Ranks, valOpt: opt.Value, sessions: map[*loop.Engine]*loop.Session{}, closers: map[*loop.Engine]func(){}, subagentWrote: map[*loop.Engine]map[string]subagentWrite{}, subagentWrites: map[string]map[string]subagentWrite{}, stress: map[string]int{}, consults: map[string]int{}, stressDeaths: map[string]stressEnd{}, pairMarks: map[string]time.Time{}, gateStreaks: map[string]*gateStreak{}, receipts: loop.NewReceipts()}
	if w.Ranks == nil {
		w.Ranks = DefaultRanks(lex)
	}
	if err := w.Ranks.Validate(); err != nil {
		return nil, fmt.Errorf("ranks: %v", err)
	}
	w.promote = newPromoteState(w.Ranks)
	w.SetEvolution(opt.Evolution)
	w.migrateShelf()                             // ledgers and value move onto the shared shelf once (workspace/shared.go)
	w.Notes = append(w.Notes, w.CheckShelf()...) // what the harness did not write leaves before a fold reads it
	w.loadLedgers()                              // the scars come home before the first agent runs (freeze policy 2026-10-07)
	w.openValue()                                // and the value ledger: an agent's runs count across sessions
	policyPath := filepath.Join(abs, lex.WorkspaceDir, lex.Policy)
	if w.Policy, err = LoadPolicy(policyPath, lex.WorkspaceDir+"/"+lex.Policy, lex.Principles); err != nil {
		w.Notes = append(w.Notes, err.Error())
	} else if w.Policy.Principles == "" {
		w.Notes = append(w.Notes, w.Policy.Path+" has no principles section")
	}
	dimPath := filepath.Join(abs, lex.WorkspaceDir, "system.yaml")
	if _, serr := os.Stat(dimPath); serr == nil {
		if w.Manifest, err = LoadSystem(dimPath); err != nil {
			w.Notes = append(w.Notes, err.Error())
		} else if ran, err := GitSelfInit(abs); err != nil {
			w.Notes = append(w.Notes, err.Error())
		} else if ran {
			w.Notes = append(w.Notes, "system self-init: git init -b main at "+abs)
		}
	}
	if opt.Ontology {
		if err := w.Reload(); err != nil {
			return nil, err
		}
	}
	w.Instructions = LoadInstructions(abs, opt.Instructions)
	return w, nil
}

// Dir is <root>/<workspaceDir>.
func (w *Workspace) Dir() string { return filepath.Join(w.Root, w.Lex.WorkspaceDir) }

// Rel renders an absolute path relative to the root (slash-separated); outside paths stay as is.
func (w *Workspace) Rel(abs string) string {
	if r, err := filepath.Rel(w.Root, abs); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return abs
}

// Reload re-derives the roster from the docs (they change during a session).
func (w *Workspace) Reload() error {
	ow, err := onto.LoadLayout(w.Root, w.Ranks.Layout(w.Lex))
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Onto = ow
	w.Notes = append(w.Notes, ow.Notes...)
	w.Members = membersOf(w.Root, ow, w.Ranks)
	for _, f := range w.Foreign {
		if w.memberLocked(f.Name) == nil {
			w.Members = append(w.Members, f)
		}
	}
	return nil
}

// AddForeign puts an agent from another harness's agent file on the roster (a name already on
// it is one agent with two sources: the native one wins, Principle 2).
func (w *Workspace) AddForeign(c Member) {
	w.mu.Lock()
	w.Foreign = append(w.Foreign, c)
	exists := w.memberLocked(c.Name) != nil
	if !exists {
		w.Members = append(w.Members, c)
	}
	w.mu.Unlock()
}

func (w *Workspace) memberLocked(name string) *Member {
	for i := range w.Members {
		if w.Members[i].Name == name {
			return &w.Members[i]
		}
	}
	return nil
}

var (
	cMember   = onto.Ao("Member")
	pOwns     = onto.Ao("owns")
	pHolds    = onto.Ao("holds")
	pName     = onto.Ao("name")
	bondProps = []onto.Term{onto.Ao("truth"), onto.Ao("verdict"), onto.Ao("reports"), onto.Ao("above")}
	reRank    = regexp.MustCompile(`(?mi)^\s*[-*]\s*\*\*rank:?\*\*:?\s*(.+?)\s*$`)
)

func membersOf(root string, ow *onto.Workspace, ranks Ranks) []Member {
	g := ow.Graph
	var out []Member
	for _, t := range g.Instances(cMember) {
		name := t.Local()
		if name == Orchestrator {
			continue
		}
		c := Member{Name: name}
		d, ok := ow.Docs[t]
		if !ok {
			continue // a bond target with no dir of its own: the BondTarget shape names it
		}
		if strings.HasSuffix(d.Path, "/") {
			c.Dir = strings.TrimSuffix(d.Path, "/")
		} else {
			c.Doc = d.Path
			c.Dir = filepath.ToSlash(filepath.Dir(d.Path))
		}
		// the rank: the folder's base rank, unless the doc's Rank field names an promoted rank on that dir
		parts := strings.Split(c.Dir, "/")
		if len(parts) < 2 {
			continue
		}
		base, ok := ranks.ByDir(parts[1])
		if !ok {
			continue
		}
		c.Rank = base.Name
		text := ""
		if c.Doc != "" {
			text = memory.RdOr(filepath.Join(root, filepath.FromSlash(c.Doc)))
		}
		if docRetired(text) {
			// the council's relief mark: this seat keeps its graph memory but claims no
			// soil — the roster drops it, its ownership prefixes open again (policy known since
			// the first relief wave: an ended zone worker is not an invisible zone worker)
			continue
		}
		if m := reRank.FindStringSubmatch(text); m != nil {
			want := strings.ToLower(strings.NewReplacer(" ", "_", "-", "_").Replace(strings.TrimSpace(m[1])))
			for _, r := range ranks {
				if r.Name == want && r.Base == base.Name {
					c.Rank = r.Name
				}
			}
		}
		for _, o := range g.Objects(t, pOwns) {
			c.Ownership = append(c.Ownership, o.Value)
		}
		for _, p := range bondProps {
			for _, o := range g.Objects(t, p) {
				if g.Derived(onto.Triple{S: t, P: p, O: o}) && !oneHopAbove(g, t, o) {
					continue // the transitive closure, not a hop
				}
				c.Parent = o.Local()
				break
			}
			if c.Parent != "" {
				break
			}
		}
		for _, m := range g.Objects(t, pHolds) {
			if n := g.Objects(m, pName); len(n) > 0 {
				c.Skills = append(c.Skills, n[0].Value)
			} else {
				c.Skills = append(c.Skills, strings.TrimPrefix(m.Local(), "skill-"))
			}
		}
		c.Verify = VerifyOf(text)
		c.PromotedOf = promotedOf(text)
		out = append(out, c)
	}
	// succession: a member whose one-hop-up retired by promoting reports to its promoted —
	// the base's doc stays as the graph's memory, its line now runs through the risen agent
	succ, live := map[string]string{}, map[string]bool{}
	for _, c := range out {
		live[c.Name] = true
	}
	for _, c := range out {
		if c.PromotedOf != "" && !live[c.PromotedOf] {
			succ[c.PromotedOf] = c.Name
		}
	}
	for i := range out {
		seen := map[string]bool{}
		for p := out[i].Parent; succ[p] != "" && !seen[p]; p = out[i].Parent {
			seen[p] = true
			out[i].Parent = succ[p]
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var (
	reVerifyField = regexp.MustCompile("(?mi)^\\s*[-*]\\s*\\*\\*verify:?\\*\\*:?\\s*`?([^`\\n]+?)`?\\s*$")
	reVerifyHead  = regexp.MustCompile(`(?mi)^##+\s*verify\s*$`)
	reBullet      = regexp.MustCompile("(?m)^\\s*[-*]\\s+`?([^`\\n]+?)`?\\s*$")
)

// VerifyOf reads a member doc's verify commands: every `- **Verify:** cmd` line, plus the
// bullets of a `## Verify` section. Exit codes, not prose, are the invariants' reading.
func VerifyOf(doc string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, m := range reVerifyField.FindAllStringSubmatch(doc, -1) {
		add(m[1])
	}
	if loc := reVerifyHead.FindStringIndex(doc); loc != nil {
		agent := doc[loc[1]:]
		if end := regexp.MustCompile(`\n#`).FindStringIndex(agent); end != nil {
			agent = agent[:end[0]]
		}
		for _, m := range reBullet.FindAllStringSubmatch(agent, -1) {
			add(m[1])
		}
	}
	return out
}

// Member finds an agent by id (or by its unprefixed name when unambiguous). A seat registered
// mid-session is not on the in-memory roster: on a miss, the roster is re-derived from the
// docs once before giving up, so a dispatch for an agent provisioned after Open still lands.
func (w *Workspace) Member(name string) *Member {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.member(name)
}

// member is Member's agent, callable only with w.mu held — Prompt locks over the
// whole prompt build and fans into this through rankPolicyPath / RankOf.
func (w *Workspace) member(name string) *Member {
	name = strings.ToLower(strings.TrimSpace(name))
	if c := w.memberByName(name); c != nil {
		return c
	}
	ow, err := onto.LoadLayout(w.Root, w.Ranks.Layout(w.Lex))
	if err != nil {
		return nil
	}
	fresh := membersOf(w.Root, ow, w.Ranks)
	w.Onto = ow
	for _, c := range fresh {
		if w.memberLocked(c.Name) == nil {
			w.Members = append(w.Members, c)
		}
	}
	return w.memberByName(name)
}

func (w *Workspace) memberByName(name string) *Member {
	var hit *Member
	for i := range w.Members {
		c := &w.Members[i]
		if c.Name == name {
			return c
		}
		if strings.HasSuffix(c.Name, "-"+name) {
			if hit != nil {
				return nil
			}
			hit = c
		}
	}
	return hit
}

// oneHopAbove is true when no third member sits between s and o on the above chain.
func oneHopAbove(g *onto.Graph, s, o onto.Term) bool {
	above := onto.Ao("above")
	for _, mid := range g.Objects(s, above) {
		if mid != o && g.Has(onto.Triple{S: mid, P: above, O: o}) {
			return false
		}
	}
	return true
}

// RankOf is a member's rank row (orchestrator's for the session or an unknown name).
func (w *Workspace) RankOf(name string) Rank {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rankOf(name)
}

// rankOf is RankOf's agent with w.mu already held (rankPolicyPath runs inside
// Prompt's build-wide lock).
func (w *Workspace) rankOf(name string) Rank {
	if c := w.member(name); c != nil {
		if r, ok := w.Ranks.Get(c.Rank); ok {
			return r
		}
	}
	if rn := w.Ranks.Of(name); rn != "" {
		if r, ok := w.Ranks.Get(rn); ok {
			return r
		}
	}
	r, _ := w.Ranks.Get(Orchestrator)
	return r
}

// GateHolders reports whether any member on the roster holds a gate (an domain owner, by the policy's
// table). Domain owners is its old name.
func (w *Workspace) GateHolders() bool {
	for _, c := range w.Members {
		if r, ok := w.Ranks.Get(c.Rank); ok && r.HoldsGate {
			return true
		}
	}
	return false
}

// Domain owners is GateHolders under the policy's name.
func (w *Workspace) DomainOwners() bool { return w.GateHolders() }

// InOwnership reports whether a root-relative path is inside a member's ownership: one of its
// owned prefixes, or its own directory (an agent may always keep its own doc truthful).
func (c *Member) InOwnership(rel string) bool {
	rel = normRel(rel)
	if c.Dir != "" && under(rel, c.Dir) {
		return true
	}
	for _, t := range c.Ownership {
		if under(rel, t) {
			return true
		}
	}
	return false
}

func normRel(p string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(p)), "./"))
}

func under(rel, prefix string) bool {
	prefix = normRel(prefix)
	return prefix != "" && prefix != "." && (rel == prefix || strings.HasPrefix(rel, prefix+"/"))
}

// Owner is the agent that owns a root-relative path: the authoring member with the longest
// matching ownership, else the gate holder's, else nil. Its doc is the one Docs-as-code demands.
func (w *Workspace) Owner(rel string) *Member {
	rel = normRel(rel)
	for _, pick := range []func(Rank) bool{func(r Rank) bool { return r.Authors }, func(r Rank) bool { return r.HoldsGate && !r.Authors }} {
		var best *Member
		bestLen := -1
		for i := range w.Members {
			c := &w.Members[i]
			if r, ok := w.Ranks.Get(c.Rank); !ok || !pick(r) {
				continue
			}
			for _, t := range c.Ownership {
				if under(rel, t) && len(t) > bestLen {
					best, bestLen = c, len(t)
				}
			}
		}
		if best != nil {
			return best
		}
	}
	// Seat fallback (auditor verdict 2026-10-02, arm-2 discrimination follow-up
	// 2026-10-04): seat docs are the member's authorship; ownership prefixes
	// never cover the seat — a doc at a member's Dir matches no ownership
	// prefix (e.g. .agent-one/zone/api/README.md, ownership src/api/), so without
	// this pass Owner returns nil and the gate's memberDoc branch stays dead.
	for i := range w.Members {
		c := &w.Members[i]
		if c.Doc != "" && normRel(c.Doc) == rel {
			return c
		}
	}
	return nil
}

func parentDir(d string) string { return filepath.Dir(d) }

// VerifyCoded reports whether the member's doc carries at least one verify line beyond the
// trivial sentinels (`true`, `exit 0`): the domain-TDD dispatch precondition (workspace/subagent.go)
// reads this to prove a verdict is coded BEFORE a zone worker is ordered.
func (c *Member) VerifyCoded() bool {
	for _, v := range c.Verify {
		v = strings.TrimSpace(strings.ToLower(v))
		if v != "" && v != "true" && v != "exit 0" {
			return true
		}
	}
	return false
}
