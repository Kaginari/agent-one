package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kaginari/agent-one/loop"
)

// Rule is one injected rule text (config supplies them). Scope is `all`, `rank:<rank>` or
// `member:<name>`.
type Rule struct {
	Text  string
	Scope string
}

// Applies reports whether the rule reaches an agent.
func (r Rule) Applies(as, rank string) bool {
	s := strings.ToLower(strings.TrimSpace(r.Scope))
	switch {
	case s == "" || s == "all":
		return true
	case strings.HasPrefix(s, "rank:"):
		return strings.TrimPrefix(s, "rank:") == rank || (rank == "" && strings.TrimPrefix(s, "rank:") == "orchestrator")
	case strings.HasPrefix(s, "member:"):
		return strings.TrimPrefix(s, "member:") == strings.ToLower(as)
	}
	return false
}

// OntologyOptions is the projection's switch and budget (tokens).
type OntologyOptions struct {
	Enabled bool
	Budget  int
}

// PromptOptions is the system-prompt builder's switchboard.
type PromptOptions struct {
	Principles   bool // policy.principles: the nine breaths, first and always
	Identity     bool // who the agent is, the classes, the gate, the wire (the engine's default text)
	Rules        []Rule
	Ontology     OntologyOptions
	Instructions bool // project instruction files (AGENTS.md / CLAUDE.md)
	Member       bool // the agent's own card: ownership, parent, skills, verify
	Memory       bool // one line on how the tiers are reached (the manifest itself rides the Recall hook)
	SelfState    bool // the loop's own instruments named: the pulse tool, guts notes, the check-loop policy
}

// DefaultPrompt is everything on.
func DefaultPrompt() PromptOptions {
	return PromptOptions{Principles: true, Identity: true, Ontology: OntologyOptions{Enabled: true, Budget: 800}, Instructions: true, Member: true, Memory: true, SelfState: true}
}

// System is the loop's System hook: principles · identity · injected rules · the agent's card ·
// ontology projection · instructions · memory line. The recall manifest and the toolbox brief
// are appended by the loop from the Recall hook, so a provider call sees one prompt.
func (w *Workspace) System(opt PromptOptions) func(s *loop.Session) string {
	return func(s *loop.Session) string {
		as := s.Engine.As
		if as == "" {
			as = s.Engine.Lexicon.Agent
		}
		return w.Prompt(as, opt, s.Engine)
	}
}

// Prompt builds the system prompt for an agent outside a session (a dispatcher's preview, the
// drain's rebuilt context). engine may be nil.
//
// w.mu is held over the whole build, not just the lookup below: the roster
// loop at 129 reads w.Members, rankPolicyPath → RankOf → Member's locked
// tail reads w.Members, and promptOntoSection projects w.Onto — all three
// rank Member's mutation of the same two fields when a subagent-rendered
// foreign agent lands mid-fanout (the app-package shutdown-concurrency rank,
// 2026-10-03). One existing mutex, widened — no new fine-grained lock and no
// lock-order policy invented (adding onto's own mutex would order the two
// locks; everything Prompt reaches under it — rankPolicyText, membersOf's
// memo/Rd readers, engine hooks — takes no workspace lock of its own).
func (w *Workspace) Prompt(as string, opt PromptOptions, engine *loop.Engine) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var b strings.Builder
	rank := w.rankOf(as).Name
	if rank == Orchestrator {
		rank = ""
	}
	if opt.Principles {
		if w.Policy != nil && w.Policy.Principles != "" {
			fmt.Fprintf(&b, "%s — %s\n\n%s\n\n", w.Policy.Path, or(w.Lex.Principles, "the principles"), w.Policy.Principles)
		} else {
			fmt.Fprintf(&b, "@? no principles: %s/%s is missing or has no principles section\n\n", w.Lex.WorkspaceDir, w.Lex.Policy)
		}
	}
	// the rank's own page of policy (Operator 2026-10-03: global principles stay the principles's; the
	// per-race page is thin on purpose — it never restates the principles, names only what the rank
	// answers for; every other arm of the code stays consult-on-demand).
	if rp := rankPolicyPath(w, as); rp != "" {
		if data, err := os.ReadFile(rp); err == nil && strings.TrimSpace(string(data)) != "" {
			fmt.Fprintf(&b, "Policy of your rank (%s):\n%s\n\n", w.Rel(rp), strings.TrimSpace(string(data)))
		}
	}
	if opt.Identity {
		if engine != nil {
			b.WriteString(strings.TrimSpace(engine.DefaultSystem(nil)) + "\n\n")
		} else {
			fmt.Fprintf(&b, "You are %s, an agent working inside the workspace rooted at %s.\n\n", as, w.Root)
		}
	}
	if len(opt.Rules) > 0 {
		var lines []string
		for _, r := range opt.Rules {
			if r.Applies(as, rank) && strings.TrimSpace(r.Text) != "" {
				lines = append(lines, "- "+oneLine(r.Text))
			}
		}
		if len(lines) > 0 {
			b.WriteString("Rules in force for you:\n" + strings.Join(lines, "\n") + "\n\n")
		}
	}
	if opt.Member {
		if c := w.member(as); c != nil {
			r, _ := w.Ranks.Get(c.Rank)
			fmt.Fprintf(&b, "You are %s (%s%s). Doc: %s. Owns: %s.", c.Name, c.Rank, jobOf(r), or(c.Doc, "none"), or(strings.Join(c.Ownership, ", "), "none declared"))
			if c.Parent != "" {
				fmt.Fprintf(&b, " One hop up: %s.", c.Parent)
			}
			if len(c.Skills) > 0 {
				fmt.Fprintf(&b, " Skills held: %s.", strings.Join(c.Skills, ", "))
			}
			if len(c.Verify) > 0 {
				fmt.Fprintf(&b, " Verify: %s.", strings.Join(c.Verify, " · "))
			}
			b.WriteString(" A write outside your ownership is refused; a change under it changes its doc in the same turn (Docs-as-code).\n\n")
			// neighbourKnows: a sideways rank's layer-mates are named to it, coded — the
			// harness reads the bonds, never the agent asking. Divine rules of the promotion
			// rank above a base reach it as absolute rules (coded, not a prompt rule).
			if nb := w.Ranks.NeighborRanks(c.Rank); len(nb) > 0 {
				var mates []string
				for _, other := range w.Members {
					for _, nr := range nb {
						if other.Rank == nr && other.Name != c.Name {
							mates = append(mates, other.Name)
							break
						}
					}
				}
				if len(mates) > 0 {
					sort.Strings(mates)
					fmt.Fprintf(&b, "Neighbors in your layer (%s): %s — work may reach them sideways, but only through dispatch.\n\n", strings.Join(nb, ", "), strings.Join(mates, ", "))
				}
			}
			if dr := w.Ranks.DivineRulesFor(c.Rank); len(dr) > 0 {
				b.WriteString("Absolute rules from the promoted above you (divine, never overridden):\n")
				for _, d := range dr {
					b.WriteString("- " + oneLine(d) + "\n")
				}
				b.WriteString("\n")
			}
		}
	}
	if opt.SelfState {
		b.WriteString("Self-state (the loop's own instruments, yours to read):\n" +
			"- `pulse` (tool) returns your live run state: steps, context, momentum, and every bench you have hit with its red/green/muted counts. Read it before re-running a bench you have already struck.\n" +
			"- A `[guts: …]` note inside a tool result means the run is losing ground; a `[doomLoop: …]` note means it checkpointed. Both are readings, not verdicts.\n" +
			"- Repeated reds on one bench with no green is a check-loop, not TDD. If the counts say thrash, stop re-running: change the approach, instrument the probe once, or answer on the wire. Never pipe a bench's output through a filter that hides its exit status.\n\n")
	}
	if opt.Ontology.Enabled && w.Onto != nil {
		budget := opt.Ontology.Budget
		if budget == 0 {
			budget = 800
		}
		name := as
		if w.member(as) == nil {
			name = "orchestrator"
		}
		if lines, err := w.Onto.Project(name, budget); err == nil && len(lines) > 0 {
			b.WriteString("@ONTO\n" + strings.Join(lines, "\n") + "\n\n")
		}
	}
	if opt.Instructions {
		for _, in := range w.Instructions {
			fmt.Fprintf(&b, "Instructions (%s, %s):\n%s\n\n", in.Scope, w.Rel(in.Path), strings.TrimSpace(in.Text))
		}
	}
	if opt.Memory {
		fmt.Fprintf(&b, "Memory: @RECALL below carries anchors (path#section) into the tiers and @TOOLS the level-1 manifest; load a section or a Skill only on your own decision, never pre-emptively. What you learned that nothing on disk says goes out as @U <%s|%s|%s>; the policy's code is read with the `policy` tool by heading.\n",
			w.Lex.Token("policy"), w.Lex.Token("team"), w.Lex.Token("domain"))
	}
	return strings.TrimSpace(b.String())
}

// rankPolicyPath is <workspaceDir>/policy/rank/<rank>.md — "" when the workspace hasn't authored this rank's
// page. No leak policy: an agent's page rides its own member card or the orchestrator — a name the
// roster doesn't know reads NO rank page (RankOf's orchestrator fallback is a default for rules
// filtering, never a warrant to read the orchestrator's policy).
// Callers must hold w.mu — Prompt locks over the whole build and this fans into the
// unlocked Member/RankOf agents through member / rankOf.
func rankPolicyPath(w *Workspace, as string) string {
	if as != Orchestrator && w.member(as) == nil {
		return ""
	}
	rank := w.rankOf(as).Name
	if rank == "" {
		return ""
	}
	p := filepath.Join(w.Root, w.Lex.WorkspaceDir, "policy", "rank", rank+".md")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

func jobOf(r Rank) string {
	var tags []string
	if r.Authors {
		tags = append(tags, "authors")
	}
	if r.HoldsGate {
		tags = append(tags, "holds the gate")
	}
	if len(tags) == 0 {
		return ""
	}
	return ": " + strings.Join(tags, ", ")
}
