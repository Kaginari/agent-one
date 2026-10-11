package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/tui"
	"github.com/Kaginari/agent-one/value"
	"github.com/Kaginari/agent-one/workspace"
)

// policy.evolution as the app arms it (workspace/evolve.go does the work): the founding sketch reads a
// ownership from the code, the gate's sandbox contains the verify lines a plan runs, and every
// evolution reaches the surfaces — the session stream and the dashboard (an `evolution` event),
// the terminal (a toast and a sidebar line), the plain console (one line).

// evolutionOptions is policy.evolution for the workspace: config's numbers (armEvolution adds the
// app's hands).
func evolutionOptions(cfg *config.Config) workspace.EvolutionOptions {
	e := cfg.Policy.Evolution
	return workspace.EvolutionOptions{Enabled: e.Enabled, Mode: e.Mode, NotesMax: e.NotesMax, StressAt: cfg.EvolutionStressAt(), FailStreak: e.FailStreak,
		CooldownRuns: e.CooldownRuns, Arms: append([]string(nil), e.Arms...), SplitLines: e.SplitLines, SplitFiles: e.SplitFiles, SkillStatus: cfg.Policy.Forge.EvolvedStatus}
}

// armEvolution hands the workspace its policy.evolution with the app's hands: the survey, the sandbox,
// the verify clock, the surfaces.
func (a *App) armEvolution() {
	if a.Workspace == nil {
		return
	}
	o := evolutionOptions(a.Cfg)
	o.Survey = a.surveyOwnership
	o.Sandbox = a.Sandbox
	o.VerifyTimeout = a.Cfg.Policy.Gate.VerifyTimeout.D()
	o.On = a.evolved
	a.Workspace.SetEvolution(o)
}

// evolved carries one evolution to every surface that is up.
func (a *App) evolved(ev workspace.EvolveEvent) {
	msg := tui.EvEvolution{ID: ev.ID, Agent: ev.Agent, Arm: ev.Arm, Into: ev.Into, Reason: ev.Reason, Status: ev.Status, Mode: ev.Mode, Hole: ev.Hole, Skill: ev.Skill, Line: ev.Line()}
	a.mu.Lock()
	sink, web := a.evoSink, a.web
	a.mu.Unlock()
	switch {
	case sink != nil:
		sink(msg) // the terminal and the dashboard (the host fans out)
	case web != nil:
		web.Send(msg)
	}
	if sink == nil && !a.Opt.Quiet {
		mark := "✦"
		if ev.Status == "hole" {
			mark = "@?"
		}
		fmt.Fprintf(a.Opt.Err, "%s %s\n", mark, ev.Line())
	}
}

// evolveSweep is the run-start pass (policy.evolution): every stressed agent whose working notes overflowed
// is weighed before the first turn.
func (a *App) evolveSweep() {
	if a.Workspace == nil || !a.Cfg.EvolutionOn() {
		return
	}
	a.Workspace.EvolveSweep()
}

// --- the survey: the founding sketch's eyes on one ownership ---

var surveyCache struct {
	mu   sync.Mutex
	root string
	at   time.Time
	s    *survey
}

// surveyOwnership reads an ownership the way the medium founding reads a zone worker's unit: its files,
// lines, packages and exports, its tests and the runner that proves them.
func (a *App) surveyOwnership(prefixes []string) workspace.OwnershipSurvey {
	surveyCache.mu.Lock()
	if surveyCache.s == nil || surveyCache.root != a.Root || time.Since(surveyCache.at) > 30*time.Second {
		surveyCache.s, surveyCache.root, surveyCache.at = readSurvey(a.Root, a.Cfg.Dist.WorkspaceDir), a.Root, time.Now()
	}
	s := surveyCache.s
	surveyCache.mu.Unlock()
	u := &unit{langs: map[string]int{}}
	var clean []string
	for _, p := range prefixes {
		p = strings.Trim(strings.TrimSpace(p), "`")
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			continue
		}
		clean = append(clean, p)
		if fi, err := os.Stat(filepath.Join(a.Root, filepath.FromSlash(p))); err == nil && fi.IsDir() {
			u.dirs = append(u.dirs, p)
		} else {
			u.files = append(u.files, p)
		}
	}
	if len(clean) == 0 {
		return workspace.OwnershipSurvey{}
	}
	u.name = slug(filepath.Base(clean[len(clean)-1]))
	files := 0
	for i := range s.files {
		f := &s.files[i]
		for _, p := range clean {
			if f.rel == p || strings.HasPrefix(f.rel, p+"/") {
				s.absorb(u, f)
				files++
				break
			}
		}
	}
	if files == 0 {
		return workspace.OwnershipSurvey{}
	}
	s.place(u)
	pl := s.zonePlan(u, []*unit{u}, [][]float64{{0}}, a.Lex.Prefixes)
	return workspace.OwnershipSurvey{Files: files, Lines: u.lines, Purpose: pl.Purpose, Invariants: pl.Invariants, Verify: pl.Verify}
}

// --- the working notes: agent-one evolve list | show <id> | apply <id> | reject <id> <why> ---

func (a *App) cmdEvolve(args []string, io IO) int {
	if a.Workspace == nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? no %s here\n", worldWord(a.Cfg.Dist.Name))
		return 2
	}
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list", "ls":
		ps := a.Workspace.Proposals()
		open := 0
		for _, p := range ps {
			if p.Status == "open" {
				open++
			}
		}
		mode := a.Cfg.Policy.Evolution.Mode
		fmt.Fprintf(io.Out, "evolution · mode %s · %d open proposal%s of %d\n", mode, open, plural(open), len(ps))
		for i := len(ps) - 1; i >= 0; i-- {
			p := ps[i]
			fmt.Fprintf(io.Out, "  %s  %-9s %-8s %-18s %s — %s\n", p.ID, p.Status, p.Arm, p.Agent, p.At.Local().Format("2006-01-02 15:04"), p.Reason)
		}
		for _, r := range a.recentEvolutions(5) {
			fmt.Fprintln(io.Out, "  "+r)
		}
		return 0
	case "show":
		if len(args) < 2 {
			fmt.Fprintf(io.Err, "@S FAIL\n@? evolve show <id>\n")
			return 2
		}
		p, err := a.Workspace.Proposal(args[1])
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
			return 2
		}
		fmt.Fprintf(io.Out, "%s · %s · %s · %s\nwhy: %s\ntrigger: %s\n", p.ID, p.Agent, p.Arm, p.Status, p.Reason, strings.Join(p.Evidence, "; "))
		if len(p.Into) > 0 {
			fmt.Fprintf(io.Out, "into: %s\n", strings.Join(p.Into, ", "))
		}
		if p.Why != "" {
			fmt.Fprintf(io.Out, "note: %s\n", p.Why)
		}
		fmt.Fprint(io.Out, p.Diff())
		return 0
	case "apply":
		if len(args) < 2 {
			fmt.Fprintf(io.Err, "@S FAIL\n@? evolve apply <id>\n")
			return 2
		}
		ev, err := a.Workspace.ApplyProposal(context.Background(), args[1])
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
			return 1
		}
		fmt.Fprintf(io.Out, "@S OK %s (log.md)\n", ev.Line())
		return 0
	case "reject":
		if len(args) < 3 {
			fmt.Fprintf(io.Err, "@S FAIL\n@? evolve reject <id> <why>\n")
			return 2
		}
		p, err := a.Workspace.RejectProposal(args[1], strings.Join(args[2:], " "))
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
			return 2
		}
		fmt.Fprintf(io.Out, "@S OK rejected %s (%s %s): %s\n", p.ID, p.Arm, p.Agent, p.Why)
		return 0
	}
	fmt.Fprintf(io.Err, "@S FAIL\n@? evolve %s — list | show <id> | apply <id> | reject <id> <why>\n", sub)
	return 2
}

// recentEvolutions are the ledger's latest evolution rows, one line each, newest first.
func (a *App) recentEvolutions(n int) []string {
	var out []string
	for _, r := range a.Workspace.EvolutionView(n).Recent {
		out = append(out, evolutionRow(r))
	}
	return out
}

func evolutionRow(r value.Row) string {
	what := r.Arm
	if len(r.Into) > 0 && r.Arm == "split" {
		what += " → " + strings.Join(r.Into, ", ")
	}
	return fmt.Sprintf("%s %s %s: %s (%s) — %s", r.At.Local().Format("2006-01-02 15:04"), r.Agent, or(r.Status, "applied"), what, r.Rank, r.Reason)
}

// evolutionStatus is status's evolution block: the policy as armed, the queue, the latest rows.
func (a *App) evolutionStatus() []string {
	if a.Workspace == nil {
		return nil
	}
	v := a.Workspace.EvolutionView(5)
	state := "on"
	if !v.Enabled {
		state = "off"
	}
	lines := []string{fmt.Sprintf("evolution: %s · mode %s · working notes > %d · stressAt %d · cooldown %d runs · %d open proposal%s", state, v.Mode, v.NotesMax, v.StressAt, v.CooldownRuns, v.Open, plural(v.Open))}
	for _, r := range v.Recent {
		lines = append(lines, "  "+evolutionRow(r))
	}
	return lines
}
