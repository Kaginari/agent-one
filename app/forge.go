package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/discover"
	"github.com/Kaginari/agent-one/tool"
	"github.com/Kaginari/agent-one/wire"
	"github.com/Kaginari/agent-one/workspace"
)

// --- the working notes's forge ---
//
// Drafts ride a Subagent's report as `@FORGE skill:<name> | <when> | <how>` or
// `@FORGE tool:<name> | <summary> | <argv>`. The harness files them from the report it holds
// (workspace.FileForges): one shared note tagged "forge" per draft, with the drafting agent, its run
// and a content hash — the draft's stable id. The working notes (/forge here, `agent-one forge` headless) is
// where curation meets policy: list, show, register, reject. A registered skill becomes
// <workspace>/crafted-skills/<name>/SKILL.md in Claude Code's format; a registered tool becomes a
// tools.custom patch with its class read by the classifier. The working notes's marks ("forge-registered",
// "forge-rejected") are append-only notes naming the draft's hash — never a line struck.

// skillName is Claude Code's skill-name rule.
var skillName = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// forgeItem is one open draft on the working notes.
type forgeItem struct {
	ID      string // the draft's content hash (stable across sessions)
	F       wire.Forge
	By, Run string
	At      time.Time
	Version int    // skills: the version a register lands (an existing crafted skill + 1)
	Holder  string // skills: a non-crafted home already holding the name; tools: "tools.custom"
}

// forgeOpts are a register's switches.
type forgeOpts struct {
	yes, version, force bool
	// confirm asks the human (a TTY prompt); nil means only --yes agrees.
	confirm func(question string) bool
}

func (a *App) notesPath() string {
	return workspace.ForgeLedgerPath(a.Root, a.Cfg.Dist.WorkspaceDir)
}

func (a *App) craftedDir() string {
	return filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, "crafted-skills")
}

// minter is who registers: the human of this workspace (the working notes is theirs), by the lexicon's name.
func (a *App) minter() string {
	if h := strings.TrimSpace(a.Lex.Human); h != "" {
		return strings.ToLower(h)
	}
	return "human"
}

// forgeWorkingNotes reads the ledger into the open drafts: every "forge" note whose hash no mark names,
// each once (the first filer). A legacy mark (no hash, text "kind:name") closes the drafts of
// that name filed before it.
func (a *App) forgeWorkingNotes() []forgeItem {
	notes := workspace.ReadForgeLedger(a.notesPath())
	closed := map[string]bool{}
	legacy := map[string]int{} // kind:name → index of its last hashless register mark
	for i, n := range notes {
		switch n.Tag {
		case "forge-registered", "forge-rejected":
			if n.Hash != "" {
				closed[n.Hash] = true
			} else if n.Tag == "forge-registered" {
				legacy[strings.TrimSpace(n.Text)] = i
			}
		}
	}
	var out []forgeItem
	seen := map[string]bool{}
	for i, n := range notes {
		if n.Tag != "forge" || closed[n.Hash] || seen[n.Hash] {
			continue
		}
		f, ok := wire.ParseForge(n.Text)
		if !ok {
			continue
		}
		if j, ok := legacy[f.Kind+":"+f.Name]; ok && j > i {
			continue
		}
		seen[n.Hash] = true
		at, _ := time.Parse(time.RFC3339, n.At)
		it := forgeItem{ID: n.Hash, F: f, By: n.By, Run: n.Run, At: at, Version: 1}
		if f.Kind == "skill" {
			if v, ok := a.craftedVersion(f.Name); ok {
				it.Version = v + 1
			} else if h := a.skillHolder(f.Name); h != "" {
				it.Holder = h
			}
		} else if _, taken := a.Cfg.Tools.Custom[f.Name]; taken {
			it.Holder = "tools.custom"
		}
		out = append(out, it)
	}
	return out
}

// craftedVersion is the version of the crafted skill already at <workspace>/crafted-skills/<name>.
func (a *App) craftedVersion(name string) (int, bool) {
	b, err := os.ReadFile(filepath.Join(a.craftedDir(), name, "SKILL.md"))
	if err != nil {
		return 0, false
	}
	fm, _ := discover.Frontmatter(string(b))
	v, err := strconv.Atoi(strings.TrimSpace(fm["version"]))
	if err != nil || v < 1 {
		v = 1
	}
	return v, true
}

// skillHolder is the path of a skill of this name in any other home of the one registry (base,
// the hosts', the global ones) — a crafted skill there would shadow it.
func (a *App) skillHolder(name string) string {
	skills, _ := discover.SkillsIndexed(discover.Options{Root: a.Root, Home: a.Opt.Home, WorkspaceDir: a.Cfg.Dist.WorkspaceDir})
	crafted := filepath.Join(a.craftedDir(), name, "SKILL.md")
	for _, s := range skills {
		if (s.Name == name || filepath.Base(s.Dir) == name) && s.Path != crafted {
			return relOrAbs(a.Root, s.Path)
		}
	}
	return ""
}

// findDraft resolves an id (the hash or a unique prefix of 4+ characters).
func findDraft(items []forgeItem, id string) (forgeItem, error) {
	id = strings.TrimPrefix(strings.TrimSpace(id), "#")
	if len(id) < 4 {
		return forgeItem{}, fmt.Errorf("a draft id is its hash (4+ characters) — `forge list` shows them")
	}
	var hits []forgeItem
	for _, it := range items {
		if strings.HasPrefix(it.ID, id) {
			hits = append(hits, it)
		}
	}
	switch len(hits) {
	case 0:
		return forgeItem{}, fmt.Errorf("no open draft %q (registered and rejected drafts leave the working notes)", id)
	case 1:
		return hits[0], nil
	}
	return forgeItem{}, fmt.Errorf("%q names %d drafts — give more of the hash", id, len(hits))
}

// forgeCmd is the slash command: /forge [list | show <id> | register <id> [--version] [--yes]
// [--force] | reject <id> <why>].
func (a *App) forgeCmd(out, errw io.Writer, rest string) {
	args := strings.Fields(rest)
	if len(args) > 0 && args[0] == "reject" && len(args) > 2 {
		// the why is free text: keep its spacing as typed
		why := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(rest, "reject")), args[1]))
		args = []string{"reject", args[1], "--why", strings.TrimSpace(strings.TrimPrefix(why, "--why"))}
	}
	a.forgeRun(out, errw, args, nil, "/forge")
}

// forgeRun is the working notes, for the slash command and the CLI alike; it returns the exit code.
func (a *App) forgeRun(out, errw io.Writer, args []string, confirm func(string) bool, endpoint string) int {
	sub := "list"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	var opt forgeOpts
	opt.confirm = confirm
	var pos []string
	why := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--yes", "-y":
			opt.yes = true
		case "--version":
			opt.version = true
		case "--force":
			opt.force = true
		case "--why":
			why = strings.TrimSpace(strings.Join(args[i+1:], " "))
			i = len(args)
		default:
			pos = append(pos, args[i])
		}
	}
	items := a.forgeWorkingNotes()
	usage := func() int {
		fmt.Fprintf(errw, "usage: %s list | show <id> | register <id> [--yes] [--version] [--force] | reject <id> --why <reason>\n", endpoint)
		return 2
	}
	switch sub {
	case "list", "ls":
		a.forgeList(out, items, endpoint)
		return 0
	case "show", "register", "reject":
		if len(pos) != 1 {
			return usage()
		}
		it, err := findDraft(items, pos[0])
		if err != nil {
			fmt.Fprintln(errw, "forge: "+err.Error())
			return 1
		}
		switch sub {
		case "show":
			a.forgeShow(out, it)
			return 0
		case "reject":
			if why == "" {
				fmt.Fprintln(errw, "forge reject: say why (--why <reason>) — the mark is the record")
				return 2
			}
			if err := a.Workspace.FileForgeMark(a.minter(), "forge-rejected", it.F.Kind+":"+it.F.Name+" — "+why, it.ID); err != nil {
				fmt.Fprintln(errw, "forge reject: "+err.Error())
				return 1
			}
			fmt.Fprintf(out, "rejected %s %s:%s — %s\n", it.ID[:7], it.F.Kind, it.F.Name, why)
			return 0
		}
		var err2 error
		if it.F.Kind == "skill" {
			err2 = a.registerSkill(out, it, opt)
		} else {
			err2 = a.registerTool(out, it, opt)
		}
		if err2 != nil {
			fmt.Fprintln(errw, "forge register: "+err2.Error())
			return 1
		}
		return 0
	}
	return usage()
}

func (a *App) forgeList(out io.Writer, items []forgeItem, endpoint string) {
	if len(items) == 0 {
		fmt.Fprintln(out, "the forge is quiet — subagents carry drafts home as @FORGE lines; the harness files them here")
		return
	}
	env := tool.Env{Root: a.Root, WorkspaceDir: a.Cfg.Dist.WorkspaceDir}
	now := time.Now()
	for _, it := range items {
		ver := "v" + strconv.Itoa(it.Version)
		if it.Holder != "" {
			ver = "taken"
		}
		meta := []string{it.By, orStr(shortRun(it.Run), "run ?"), forgeAge(now, it.At), it.F.Kind}
		if it.F.Kind == "tool" {
			if _, argv, err := toolDraft(it.F); err != nil {
				meta = append(meta, "argv?")
			} else {
				meta = append(meta, classOfArgv(env, argv).Class.String())
			}
		}
		fmt.Fprintf(out, "%s  %-24s %-5s %s\n", it.ID[:7], it.F.Name, ver, strings.Join(meta, " · "))
		if s := forgeOneLine(it.F.Text, 110); s != "" {
			fmt.Fprintf(out, "         %s\n", s)
		}
	}
	why := "<why>"
	if !strings.HasPrefix(endpoint, "/") {
		why = "--why <reason>"
	}
	fmt.Fprintf(out, "%s show <id> · %s register <id> [--version] [--yes] · %s reject <id> %s\n", endpoint, endpoint, endpoint, why)
}

func (a *App) forgeShow(out io.Writer, it forgeItem) {
	fmt.Fprintf(out, "draft %s — %s:%s\n", it.ID, it.F.Kind, it.F.Name)
	fmt.Fprintf(out, "  by %s · run %s · filed %s\n", it.By, orStr(it.Run, "?"), orStr(fmtTime(it.At), "?"))
	fmt.Fprintf(out, "  text: %s\n", it.F.Text)
	if it.F.Kind == "skill" {
		if it.Holder != "" {
			fmt.Fprintf(out, "  the name is held at %s — a crafted skill would shadow it; the draft needs another name\n", it.Holder)
			return
		}
		if it.Version > 1 {
			fmt.Fprintf(out, "  a crafted %s stands at v%d — this draft registers v%d only with --version\n", it.F.Name, it.Version-1, it.Version)
		}
		agent, err := a.skillFile(it, time.Now())
		if err != nil {
			fmt.Fprintf(out, "  cannot register: %v\n", err)
			return
		}
		fmt.Fprintf(out, "  a register writes %s:\n%s", relOrAbs(a.Root, filepath.Join(a.craftedDir(), it.F.Name, "SKILL.md")), indent(agent, "    "))
		return
	}
	plan, err := a.toolPlan(it, forgeOpts{force: true})
	if err != nil {
		fmt.Fprintf(out, "  cannot register: %v\n", err)
		return
	}
	if plan.eval != "" {
		fmt.Fprintf(out, "  interpreter-eval shape (%s): a register refuses it unless --force\n", plan.eval)
	}
	fmt.Fprintf(out, "  class %s (%s)\n  a register writes tools.custom.%s: %s\n", plan.class.Class, plan.class.Why, it.F.Name, plan.yaml)
}

// --- skills ---

// skillParts splits a skill draft's text into its when (the description: what calls for it,
// triggers first) and its how (the agent). A one-part draft is both.
func skillParts(text string) (when, how string) {
	when, how, _ = strings.Cut(text, " | ")
	when, how = strings.TrimSpace(when), strings.TrimSpace(how)
	if how == "" {
		how = when
	}
	return when, how
}

// skillFile renders the SKILL.md a register writes: Claude Code's frontmatter (name, description)
// plus the workspace's fields — version, status, by (the drafting agent), provisioned (its run), registered.
func (a *App) skillFile(it forgeItem, at time.Time) (string, error) {
	if !skillName.MatchString(it.F.Name) {
		return "", fmt.Errorf("skill name %q must be [a-z0-9-]{1,64} (Claude Code's rule) — the draft needs another name", it.F.Name)
	}
	when, how := skillParts(it.F.Text)
	desc := strings.Join(strings.Fields(when), " ")
	if desc == "" {
		return "", fmt.Errorf("a skill draft needs its when: @FORGE skill:%s | <when: the triggers> | <how: the steps>", it.F.Name)
	}
	if utf8.RuneCountInString(desc) > 1024 {
		desc = string([]rune(desc)[:1023]) + "…"
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + it.F.Name + "\n")
	b.WriteString("description: " + forgeQuote(desc) + "\n")
	b.WriteString("version: " + strconv.Itoa(it.Version) + "\n")
	b.WriteString("status: active\n")
	b.WriteString("by: " + forgeQuote(it.By) + "\n")
	if it.Run != "" {
		b.WriteString("provisioned: " + forgeQuote(it.Run) + "\n")
	}
	registered := "{by: " + forgeQuote(a.minter())
	if a.SessionID != "" {
		registered += ", session: " + forgeQuote(a.SessionID)
	}
	registered += ", at: " + forgeQuote(at.UTC().Format(time.RFC3339)) + ", draft: " + forgeQuote(it.ID) + "}"
	b.WriteString("registered: " + registered + "\n")
	b.WriteString("---\n\n")
	b.WriteString("# " + it.F.Name + "\n\n")
	b.WriteString(strings.TrimSpace(how) + "\n")
	return b.String(), nil
}

// registerSkill lands a skill draft as <workspace>/crafted-skills/<name>/SKILL.md. It never overwrites:
// a name another home already holds is refused; an existing crafted skill of the name is
// refused with the diff unless --version asks for v(n+1), and then the old file is kept under
// versions/v<n>.md beside it.
func (a *App) registerSkill(out io.Writer, it forgeItem, opt forgeOpts) error {
	if it.Holder != "" {
		return fmt.Errorf("skill %q is already held at %s — a crafted skill would shadow it; draft it under another name", it.F.Name, it.Holder)
	}
	now := time.Now()
	text, err := a.skillFile(it, now)
	if err != nil {
		return err
	}
	dir := filepath.Join(a.craftedDir(), it.F.Name)
	p := filepath.Join(dir, "SKILL.md")
	rel := relOrAbs(a.Root, p)
	if old, err := os.ReadFile(p); err == nil {
		if !opt.version {
			fmt.Fprintf(out, "%s stands at v%d; this draft would land v%d:\n%s", rel, it.Version-1, it.Version, lineDiff(string(old), text))
			return fmt.Errorf("%s exists — nothing overwritten; register %s --version to land v%d (v%d is kept under versions/)", rel, it.ID[:7], it.Version, it.Version-1)
		}
		keep := filepath.Join(dir, "versions", fmt.Sprintf("v%d.md", it.Version-1))
		if _, err := os.Stat(keep); err == nil {
			return fmt.Errorf("%s already holds v%d — refusing to overwrite the kept version", relOrAbs(a.Root, keep), it.Version-1)
		}
		if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(keep, old, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "kept v%d as %s\n", it.Version-1, relOrAbs(a.Root, keep))
	}
	fmt.Fprintf(out, "writing %s:\n%s", rel, indent(text, "  "))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return err
	}
	if err := a.Workspace.FileForgeMark(a.minter(), "forge-registered", fmt.Sprintf("skill:%s v%d → %s", it.F.Name, it.Version, rel), it.ID); err != nil {
		a.hole("forge-registered mark: " + err.Error())
	}
	a.Found = Discover(a.Cfg, a.Root, a.Opt.Home) // the skill tool and status see it at once
	fmt.Fprintf(out, "registered skill:%s v%d — an agent whose doc names it holds it; any agent loads it with skill{name: %q}\n", it.F.Name, it.Version, it.F.Name)
	return nil
}

// --- tools ---

// toolDraft splits a tool draft into its summary and its argv, quoted as a shell would split it.
func toolDraft(f wire.Forge) (string, []string, error) {
	summary, argvText, ok := strings.Cut(f.Text, "|")
	if !ok || strings.TrimSpace(argvText) == "" {
		return "", nil, fmt.Errorf("a tool draft needs an argv template: @FORGE tool:%s | <summary> | <argv>", f.Name)
	}
	argv, err := tool.SplitArgv(strings.TrimSpace(argvText))
	if err != nil {
		return "", nil, fmt.Errorf("the argv %q: %v", strings.TrimSpace(argvText), err)
	}
	if len(argv) == 0 {
		return "", nil, fmt.Errorf("the argv is empty")
	}
	return strings.TrimSpace(summary), argv, nil
}

// classOfArgv is the real classifier's reading of the argv with sample params filled.
func classOfArgv(env tool.Env, argv []string) tool.Classification {
	sample := map[string]string{}
	for _, p := range tool.ArgvParams(argv) {
		sample[p] = "sample"
	}
	return env.ClassifyCommand(tool.QuoteArgv(tool.FillArgv(argv, sample)))
}

type toolRegisterPlan struct {
	class tool.Classification
	eval  string
	yaml  string
}

// toolPlan validates a tool draft and renders the tools.custom value a register writes. The name is
// checked against tools.custom's rule before any config path is built from it.
func (a *App) toolPlan(it forgeItem, opt forgeOpts) (toolRegisterPlan, error) {
	var p toolRegisterPlan
	if !tool.ValidCustomName(it.F.Name) {
		return p, fmt.Errorf("tool name %q must be [a-z][a-z0-9_-]{0,63} (tools.custom's rule)", it.F.Name)
	}
	if _, exists := a.Cfg.Tools.Custom[it.F.Name]; exists {
		return p, fmt.Errorf("tools.custom.%s already declares %s — refuse to overwrite the working notes's policy; rename the draft", it.F.Name, it.F.Name)
	}
	summary, argv, err := toolDraft(it.F)
	if err != nil {
		return p, err
	}
	p.eval = tool.InterpreterEval(argv)
	if p.eval != "" && !opt.force {
		return p, fmt.Errorf("the argv hands a string to an interpreter (%s): what it runs would be decided by a parameter, not by its declaration — refused; --force registers it anyway, at the class the classifier reads", p.eval)
	}
	p.class = classOfArgv(tool.Env{Root: a.Root, WorkspaceDir: a.Cfg.Dist.WorkspaceDir}, argv)
	var args []string
	for _, piece := range argv {
		args = append(args, forgeQuote(piece))
	}
	parts := []string{
		"description: " + forgeQuote(orStr(summary, it.F.Name)+" — crafted from "+it.By+"'s run"),
		"run: [" + strings.Join(args, ", ") + "]",
		"class: " + p.class.Class.String(),
	}
	if ps := tool.ArgvParams(argv); len(ps) > 0 {
		var decl []string
		for _, n := range ps {
			decl = append(decl, n+": {type: string, required: true}")
		}
		parts = append(parts, "params: {"+strings.Join(decl, ", ")+"}")
	}
	parts = append(parts, "crafted: {by: "+forgeQuote(it.By)+", run: "+forgeQuote(it.Run)+", at: "+forgeQuote(time.Now().UTC().Format(time.RFC3339))+", draft: "+forgeQuote(it.ID)+"}")
	p.yaml = "{" + strings.Join(parts, ", ") + "}"
	return p, nil
}

// registerTool patches tools.custom.<name> with the draft's argv, at the class the classifier
// reads; an outward or destructive tool needs the human's yes (--yes, or the TTY's y). The
// YAML is printed before it is written; the config reloads live (the Provisioning file policy).
func (a *App) registerTool(out io.Writer, it forgeItem, opt forgeOpts) error {
	plan, err := a.toolPlan(it, opt)
	if err != nil {
		return err
	}
	if plan.class.Class >= tool.Outward && !opt.yes {
		q := fmt.Sprintf("tool %s would land as %s (%s) — register it? [y/N] ", it.F.Name, plan.class.Class, plan.class.Why)
		if opt.confirm == nil || !opt.confirm(q) {
			return fmt.Errorf("tool %s classifies %s (%s): the human gate must agree — register %s --yes", it.F.Name, plan.class.Class, plan.class.Why, it.ID[:7])
		}
	}
	file := a.Cfg.ProjectFile()
	fmt.Fprintf(out, "writing %s — tools.custom.%s: %s\n", relOrAbs(a.Root, file), it.F.Name, plan.yaml)
	ops := []config.Op{{Path: "tools.custom." + it.F.Name, Value: plan.yaml}}
	if err := a.Cfg.ApplyPatch(file, ops); err != nil {
		return fmt.Errorf("the register failed: %w", err)
	}
	next, changes, err := a.Cfg.Reload()
	if err != nil {
		return fmt.Errorf("written, but the reload failed: %w", err)
	}
	a.reload(next, changes)
	a.Missing.SetConfig(next)
	if err := a.Workspace.FileForgeMark(a.minter(), "forge-registered", fmt.Sprintf("tool:%s → tools.custom.%s (class %s)", it.F.Name, it.F.Name, plan.class.Class), it.ID); err != nil {
		a.hole("forge-registered mark: " + err.Error())
	}
	fmt.Fprintf(out, "registered tool:%s — class %s\n", it.F.Name, plan.class.Class)
	return nil
}

// --- the CLI ---

// cmdForge is `agent-one forge list|show <id>|register <id> [--yes] [--version] [--force]|reject <id>
// --why …` — the working notes for headless runs, CI and Claude Code mode. On a TTY an outward register asks;
// headless it needs --yes.
func (a *App) cmdForge(args []string, io IO) int {
	var confirm func(string) bool
	if a.Opt.IsTTY != nil && a.Opt.IsTTY() && io.In != nil {
		in := bufio.NewReader(io.In)
		confirm = func(q string) bool {
			fmt.Fprint(io.Err, q)
			line, _ := in.ReadString('\n')
			l := strings.ToLower(strings.TrimSpace(line))
			return l == "y" || l == "yes"
		}
	}
	return a.forgeRun(io.Out, io.Err, args, confirm, a.Cfg.Dist.Name+" forge")
}

// --- small helpers ---

func forgeQuote(s string) string { return strconv.Quote(s) }

func forgeOneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > max {
		return string([]rune(s)[:max-1]) + "…"
	}
	return s
}

func shortRun(run string) string {
	if len(run) > 28 {
		return run[:27] + "…"
	}
	return run
}

func forgeAge(now, at time.Time) string {
	if at.IsZero() {
		return "age ?"
	}
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	}
	return strconv.Itoa(int(d.Hours()/24)) + "d"
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func indent(s, pad string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString(pad + l + "\n")
	}
	return b.String()
}

// lineDiff is a small line diff (LCS): "- " lines leave, "+ " lines arrive, "  " lines stay.
func lineDiff(a, b string) string {
	x, y := strings.Split(strings.TrimRight(a, "\n"), "\n"), strings.Split(strings.TrimRight(b, "\n"), "\n")
	n, m := len(x), len(y)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && x[i] == y[j]:
			out.WriteString("    " + x[i] + "\n")
			i, j = i+1, j+1
		case j < m && (i == n || lcs[i][j+1] >= lcs[i+1][j]):
			out.WriteString("  + " + y[j] + "\n")
			j++
		default:
			out.WriteString("  - " + x[i] + "\n")
			i++
		}
	}
	return out.String()
}
