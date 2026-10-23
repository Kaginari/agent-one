package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/loop"
	"github.com/Kaginari/agent-one/memory"
	"github.com/Kaginari/agent-one/onto"
	"github.com/Kaginari/agent-one/queue"
	"github.com/Kaginari/agent-one/toolbox"
	"github.com/Kaginari/agent-one/workspace"
)

// IO is what the CLI talks through; tests swap it.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
	Env func(string) string
}

// cliFlags are the binary's own flags, taken before config's ParseFlags reads the rest.
type cliFlags struct {
	root, dist, session, format string
	json, quiet, noBoard, bench bool
	heal, spawn                 bool
	attribute                   bool
	plain                       bool
	since                       string
	goal                        goalContract
	level                       string // init --level light|medium|complex
}

func splitFlags(args []string) (cliFlags, []string) {
	var f cliFlags
	var rest []string
	take := func(i *int, name string) string {
		a := args[*i]
		if eq := strings.IndexByte(a, '='); eq > 0 {
			return a[eq+1:]
		}
		if *i+1 < len(args) {
			*i++
			return args[*i]
		}
		return ""
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := a
		if eq := strings.IndexByte(a, '='); eq > 0 {
			name = a[:eq]
		}
		switch name {
		case "--root", "-root":
			f.root = take(&i, name)
		case "--dist":
			f.dist = take(&i, name)
		case "--session", "-session":
			f.session = take(&i, name)
		case "--since":
			f.since = take(&i, name)
		case "--json", "-json":
			f.json = true
		case "--heal", "-heal":
			f.heal = true
		case "--spawn", "-spawn":
			f.spawn = true
		case "--attribute", "-attribute":
			f.attribute = true
		case "--quiet", "-quiet", "-q":
			f.quiet = true
		case "--no-board":
			f.noBoard = true
		case "--level":
			f.level = take(&i, name)
		case "--bench":
			f.bench = true
		case "--plain":
			f.plain = true
		case "--validate":
			f.goal.validate = take(&i, name)
		case "--read":
			f.goal.read = take(&i, name)
		case "--constraints":
			f.goal.constraints = take(&i, name)
		case "--max-turns":
			fmt.Sscan(take(&i, name), &f.goal.maxTurns)
		default:
			rest = append(rest, a)
		}
	}
	return f, rest
}

const globalFlags = "flags: --root <dir> --model <provider/id> --approve <classes> --dry-run --strict --set key=value --no-<feature> --format text|json|wire --json --quiet --session <id> --plain --no-board --docker-agent [--image <ref>]"

// asked reports whether one of the flags is among the arguments, before a `--` that ends them.
func asked(args []string, flags ...string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

// named reports whether the command was written out (`repl --help`), not defaulted.
func named(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func commandSummary(name string) (string, bool) {
	for _, c := range commands {
		if c.name == name {
			return c.summary, true
		}
	}
	return "", false
}

// isCommand is a command the dispatcher answers: the listed ones, and found (init's second half).
func isCommand(name string) bool {
	_, ok := commandSummary(name)
	return ok || name == "found"
}

var commands = []struct{ name, summary string }{
	{"repl", "a live session (the default)"},
	{"dash", "a live session in the browser: prompt, the neural net, metrics, relations — dash [--open]"},
	{"run", "run one ask to its end: run [--json] [--format text|json|wire] \"<ask>\""},
	{"resume", "resume a session: resume <id> [ask]"},
	{"goal", "work to a goal until a command proves it: goal --validate \"<cmd>\" [--read …] [--constraints …] [--max-turns N] \"<objective>\""},
	{"review", "two reviewers on two models, one merged shortlist: review [range]"},
	{"handoff", "write a handoff for a fresh session: handoff [focus]"},
	{"sessions", "list the sessions of this workspace"},
	{"status", "every reading — models, sandbox, guard, memory — and every policy feature switched off, with where"},
	{"config", "config show [--yaml] | explain | check | path | patch"},
	{"memory", "the memory instrument (status, index, recall, remember, forget)"},
	{"mcp", "the workspace's memory as an MCP server on stdio — mcp serve (recall, remember, memory_status)"},
	{"toolbox", "the toolbox instrument (index, brief, pick, load, status)"},
	{"onto", "the ontology (check, project, query)"},
	{"evolve", "the evolution queue (policy.evolution, propose mode): evolve list | show <id> | apply <id> | reject <id> <why>"},
	{"q", "the shared work queue: " + queue.Usage + " — --json prints items"},
	{"forge", "the forge working notes — the drafts subagents carried home: forge list | show <id> | register <id> [--yes] [--version] [--force] | reject <id> --why <reason>"},
	{"housekeep", "the chain policy: audit member homes and links, heal the mechanical classes — housekeep [--heal] [--spawn]"},
	{"usage", "the usage journal: usage [--session <id>] [--since <date>]"},
	{"lessons", "the guts' readings ranked — the autopsy instrument: lessons [<journal dir|bench run dir>] [<top N>]"},
	{"bench", "a fixed task set on every configured model (mock always; real providers with keys)"},
	{"selftest", "every package's selftest, one @S PASS n checks"},
	{"init", "found a workspace here: init [--level light|medium|complex] [--bench] — light: the policy only; medium: a team read from the tree (build files, sizes, imports), no model; complex: the model reads all the code and founds the team"},
	{"board", "serve the board and the dashboard (read-only) without a session, on 127.0.0.1"},
	{"guard", "the global dangerous-command guard: check, test, show, export, hook, install"},
	{"install", "run the policy inside Claude Code through its hooks: install --claude [--global] [--yes]"},
	{"hook", "a Claude Code hook (wired by install): hook session-start|prompt|pre-tool|post-tool|stop, the event's JSON on stdin"},
	{"ui", "the web ui system: init (tokens, layout, components), scan (the legend), check (lint + screenshots)"},
	{"version", "print the version"},
	{"help", "this list"},
}

// healRunStart keeps the member graph healthy on every workspace-touching CLI run start
// (relayed commission 2026-10-13): synchronous and in-goroutine — never spawned async, an
// async heal would let the command agent rank the heal for the same member README files.
// Called from Main just above New(opt): App's session/history/journal (os.Executable-backed)
// file claims all happen inside New and below, so a heal here naturally precedes every claim.
// Idempotent by design: probeDeadBonds (onto/housekeep.go) skips living bonds and rewrites
// only a dead Reports-to, so a second heal on an already-healed workspace repairs nothing.
// Fail-open: a Housekeep error surfaces as a wire @? line on stderr and the command
// proceeds read-only — never os.Exit here (log-and-continue, not knot-check's fail-loud:
// a wobbling graph must not brick the CLI).
func healRunStart(stderr io.Writer, root string) {
	if root == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-one")); err != nil {
		return // no workspace to heal (findRoot caller will fail-loud itself)
	}
	if _, _, err := ontoHousekeepHeal(root); err != nil {
		fmt.Fprintf(stderr, "@? run-start heal: %v (command proceeds read-only)\n", err)
	}
}

// ontoHousekeepHeal is the one-call indirection over onto.Housekeep so the run-start seam
// names its own verb (heal), and one shim exists if the heal signature/flags ever shift.
func ontoHousekeepHeal(root string) (*onto.HouseReport, []onto.Finding, error) {
	return onto.Housekeep(root, true, false)
}

// Main is the entry of both binaries: dist names the distribution the binary was built as.
func Main(dist string, args []string, io IO, v Version) int {
	if io.Env == nil {
		io.Env = os.Getenv
	}
	if io.In == nil {
		io.In = os.Stdin
	}
	if io.Out == nil {
		io.Out = os.Stdout
	}
	if io.Err == nil {
		io.Err = os.Stderr
	}
	if code, handled := DockerAgent(dist, args, io); handled {
		return code
	}
	args, _, _, _ = stripContainerFlags(args)
	helpFlag, versionFlag, replNamed := asked(args, "-h", "--help"), asked(args, "--version"), named(args, "repl")
	name, args := command(args)
	switch {
	case name == "version" || versionFlag && name != "forge": // forge register --version is the working notes's own flag
		fmt.Fprintln(io.Out, versionLine(dist, v))
		return 0
	case name == "help" || helpFlag && name == "repl" && !replNamed:
		fmt.Fprintf(io.Out, "%s [command] [flags]\n", dist)
		for _, c := range commands {
			fmt.Fprintf(io.Out, "  %-9s %s\n", c.name, c.summary)
		}
		fmt.Fprintln(io.Out, globalFlags)
		fmt.Fprintf(io.Out, "`%s <command> --help` shows one command.\n", dist)
		return 0
	case helpFlag:
		summary, ok := commandSummary(name)
		if !ok {
			fmt.Fprintf(io.Err, "@S FAIL\n@? unknown command %q — `%s help` lists them\n", name, dist)
			return 2
		}
		fmt.Fprintf(io.Out, "%s %s — %s\n%s\n", dist, name, summary, globalFlags)
		return 0
	case !isCommand(name):
		fmt.Fprintf(io.Err, "@S FAIL\n@? unknown command %q — `%s help` lists them\n", name, dist)
		return 2
	}
	f, rest := splitFlags(args)
	if f.dist != "" {
		dist = f.dist
	}
	overrides, leftover, err := config.ParseFlags(rest)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	if f.json {
		overrides = append(overrides, config.Override{Path: "output.format", Value: "json", Flag: "--json"})
	}
	if f.format != "" {
		overrides = append(overrides, config.Override{Path: "output.format", Value: f.format, Flag: "--format"})
	}
	opt := Options{Dist: dist, Root: f.root, Env: io.Env, Flags: overrides, In: io.In, Out: io.Out, Err: io.Err, Version: v, Session: f.session, Quiet: f.quiet, NoBoard: f.noBoard, Plain: f.plain}
	if opt.Root != "" {
		opt.Cwd = opt.Root
	}
	switch name {
	case "init":
		code, level := cmdInit(dist, f, io)
		if code != 0 || level != "complex" {
			return code
		}
		name = "found" // the complex level goes on as a founding session, below
		overrides = append(overrides, config.Override{Path: "policy.gate.rightAuthor", Value: "false", Flag: "init --level complex"})
		opt.Flags = overrides
	case "config":
		cargs := []string{"--dist", dist}
		if f.root != "" {
			cargs = append(cargs, "--root", f.root)
		}
		return config.CLI(append(cargs, rest...), io.Out, io.Err)
	case "memory", "toolbox", "onto":
		return cmdInstrument(name, dist, f, leftover, io)
	case "mcp":
		return cmdMCP(dist, f, leftover, io, v)
	case "housekeep":
		return cmdHousekeep(dist, f, io)
	case "guard":
		return cmdGuard(dist, rest, io)
	case "hook":
		return cmdHook(dist, leftover, io)
	case "install":
		return cmdInstall(dist, f, rest, io)
	case "ui":
		return cmdUI(dist, f, rest, io)
	case "selftest":
		return Selftest(dist, io)
	}

	// Run-start heal (relayed commission 2026-10-13): keep the member graph healthy on
	// every workspace-touching run, not only at founding (cmdInit already heals on `found` at :386).
	// Synchronous, BEFORE New(opt) — the App's os.Executable/session/history/journal file
	// claims all happen inside New and the command agents after it — so the heal never ranks
	// a command agent for the same README files. Fail-open: a Housekeep wobble is logged and
	// the command proceeds read-only; init/instrument/loop-instrument branches above return
	// before New and stay unhealed (no workspace, nothing to heal; also keeps `found` from
	// double-healing).
	healRunStart(io.Err, f.root)

	a, err := New(opt)
	if err != nil {
		if name == "status" || name == "sessions" || name == "usage" {
			opt.NoWorkspace = true
			if a2, err2 := New(opt); err2 == nil {
				a = a2
				a.hole(err.Error())
			} else {
				fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
				return 2
			}
		} else {
			fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
			return 2
		}
	}
	defer a.Close()
	ctx := context.Background()
	switch name {
	case "run", "resume", "goal", "handoff", "review", "found":
		a.evolveSweep() // policy.evolution's run-start pass (the repl and the terminal run theirs once they are up)
	}
	switch name {
	case "dash":
		open := false
		for _, l := range leftover {
			if l == "--open" || l == "open" {
				open = true
			}
		}
		return a.Dash(ctx, open)
	case "repl":
		a.startBoard(ctx)
		if a.wantsTUI(f.plain) {
			return a.TUI(ctx)
		}
		return a.REPL(ctx)
	case "found":
		d, _ := parseDist(dist)
		lex := lexiconFor()
		return a.cmdRun(ctx, foundingAsk(a.Root, d, sketchTeam(a.Root, d), lex.Ranks, lex.Prefixes, lex.Display), io)
	case "review":
		merge, err := a.runReview(ctx, strings.Join(leftover, " "), func(l string) { fmt.Fprintln(io.Err, l) })
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? review: %v\n", err)
			return 2
		}
		return a.cmdRun(ctx, merge, io)
	case "goal":
		g := f.goal
		g.objective = strings.TrimSpace(strings.Join(leftover, " "))
		return a.cmdGoal(ctx, g, io)
	case "handoff":
		return a.cmdRun(ctx, a.handoffAsk(ctx, strings.TrimSpace(strings.Join(leftover, " ")), ""), io)
	case "run":
		return a.cmdRun(ctx, strings.TrimSpace(strings.Join(leftover, " ")), io)
	case "resume":
		if len(leftover) == 0 {
			// bare resume = continue the newest session (the checkpoint hint says exactly this;
			// typing no id after a stuck turn lands here, and an error where a continuation was
			// asked reads as "can't resume" — caught live 2026-10-03)
			list, err := a.Sessions.List()
			if err != nil || len(list) == 0 {
				fmt.Fprintf(io.Err, "@S FAIL\n@? resume needs a session id (see `%s sessions`)\n", dist)
				return 2
			}
			fmt.Fprintf(io.Err, "resuming latest: %s\n", list[0].ID)
			leftover = []string{list[0].ID}
		}
		return a.cmdResume(ctx, leftover[0], strings.TrimSpace(strings.Join(leftover[1:], " ")), io)
	case "sessions":
		return a.cmdSessions(io)
	case "status":
		return a.cmdStatus(io)
	case "q":
		return a.cmdQueue(leftover, io)
	case "forge":
		return a.cmdForge(leftover, io)
	case "evolve":
		return a.cmdEvolve(leftover, io)
	case "usage":
		return a.cmdUsage(f, io)
	case "lessons":
		return a.cmdLessons(leftover, io)
	case "bench":
		return a.Bench(ctx, io)
	case "board":
		return a.cmdBoard(ctx, io)
	}
	fmt.Fprintf(io.Err, "@S FAIL\n@? unknown command %q — `%s help` lists them\n", name, dist)
	return 2
}

// valueFlags are the config flags that take a value in the next token.
var valueFlags = map[string]bool{"--model": true, "--mode": true, "--approve": true, "--format": true, "--max-steps": true, "--budget": true, "--small-model": true, "--log-level": true, "--profile": true, "--set": true,
	"--validate": true, "--read": true, "--level": true, "--constraints": true, "--max-turns": true,
	"--root": true, "-root": true, "--dist": true, "--session": true, "-session": true, "--since": true}

// command finds the subcommand in an argv where flags may come first (`agent-one --root x run
// "…"`), and returns the argv without it. No command means the REPL.
func command(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if valueFlags[a] {
				i++
			}
			continue
		}
		return a, append(append([]string(nil), args[:i]...), args[i+1:]...)
	}
	return "repl", args
}

func versionLine(dist string, v Version) string {
	return fmt.Sprintf("%s %s (%s, %s, %s/%s)", dist, orStr(v.Version, "dev"), orStr(v.Commit, "none"), orStr(v.Date, "unknown"), runtime.GOOS, runtime.GOARCH)
}

func cmdInit(dist string, f cliFlags, io IO) (int, string) {
	root := f.root
	if root == "" {
		root, _ = os.Getwd()
	}
	root, _ = filepath.Abs(root)
	level := normLevel(f.level)
	if level == "" {
		fmt.Fprintf(io.Err, "@S FAIL\n@? init: unknown level %q — light (or soft), medium, complex\n", f.level)
		return 2, ""
	}
	rep := initReport{dist: dist, root: root, level: level}
	made, err := Init(dist, root)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? init: %v\n", err)
		return 2, ""
	}
	rep.made = made
	if wantsSetup(dist, root, f, io) {
		if f.level == "" {
			if l, err := askLevel(dist); err == nil {
				level = l
				rep.level = l
			}
		}
		switch p, port, err := runSetup(dist, root, io); {
		case err != nil:
			fmt.Fprintf(io.Err, "@? setup: %v — the %s is founded; its config can be written later\n", err, worldWord(dist))
		case p != "":
			rep.setupPath = p
			rep.boardPort = port
		}
	}
	d, _ := parseDist(dist)
	lex := lexiconFor()
	if level == "medium" {
		s := sketchTeam(root, d)
		res, err := writeSketch(root, d, s, lex.Ranks, lex.Prefixes, lex.Display)
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? init --level medium: %v\n", err)
			return 2, ""
		}
		rep.medium = &mediumRep{members: s.all(), wrote: res.wrote, kept: len(res.kept), covered: len(res.covered), prefix: lex.Prefixes,
			coords: s.count("coord"), domains: s.count("domain"), zones: s.count("zone"), service: s.count("service"), lanes: s.lanes(lex.Prefixes)}
	}
	rep.bench = f.bench
	// The chain policy at provisioning: a founded workspace never stands broken — audit it and seal
	// the mechanical classes (light founds the policy only; no members, so nothing to chain).
	if level != "light" {
		if hk, left, herr := onto.Housekeep(root, true, false); herr == nil {
			rep.housekept = hk.Healed
			rep.leftover = len(left)
		}
	}
	// the founding session is a model's work: name a missing model now, not after the founding
	if level == "complex" && !f.bench {
		if why := modelMissing(dist, root, io); why != "" {
			fmt.Fprintf(io.Err, "@? the founding session needs a model: %s\n@? the %s is founded with the policy only — set the model, then run `%s init --level complex` again (`%s config path` shows where config lives)\n", why, worldWord(dist), dist, dist)
			return 2, ""
		}
	}
	// A terminal gets the styled face (init_render.go); pipes, bench and tests read the wire.
	if styledInit(io.Out, f.plain) {
		renderInit(io.Out, rep, defaultInitWords())
		return 0, level
	}
	if len(rep.made) == 0 {
		fmt.Fprintf(io.Out, "@S OK %s already founded at %s — nothing touched\n", dist, root)
	} else {
		fmt.Fprintf(io.Out, "@S OK %s founded at %s\n", dist, root)
		for _, m := range rep.made {
			fmt.Fprintln(io.Out, "@F "+m)
		}
	}
	if rep.setupPath != "" {
		fmt.Fprintln(io.Out, "@F "+rep.setupPath)
	}
	if rep.medium != nil {
		for _, wf := range rep.medium.wrote {
			fmt.Fprintln(io.Out, "@F "+wf)
		}
		m := rep.medium
		for _, l := range m.lanes {
			fmt.Fprintln(io.Out, "@F lane "+l)
		}
		if m.covered > 0 {
			fmt.Fprintf(io.Out, "@F %d sketched member%s left out: the workspace's members already hold their ground\n", m.covered, plural(m.covered))
		}
		fmt.Fprintf(io.Out, "@S OK a team sketched from the tree (%s): %d members written (%d kept as they were) — refine their invariants and verify lines, or run `%s init --level complex` to have the model read the code\n",
			m.census(), len(m.wrote), m.kept, dist)
	}
	if level == "complex" {
		fmt.Fprintf(io.Out, "@S OK the founding session starts: the model reads all the code and founds the team\n")
	}
	if rep.bench {
		fmt.Fprintln(io.Out, "@F bench: config comes from "+strings.ToUpper(strings.ReplaceAll(dist, "-", "_"))+"_CONFIG_CONTENT; the gate needs a TTY or a file-layer pre-approval")
	}
	return 0, level
}

// modelMissing names why the workspace's default model cannot be reached, or "" when it can.
func modelMissing(dist, root string, io IO) string {
	cfg, err := config.LoadWith(config.Options{Dist: dist, Root: root, Env: io.Env})
	if err != nil {
		return err.Error()
	}
	ps := NewProviders(cfg, io.Env)
	if _, _, err := ps.Route(Route{}); err != nil {
		if h := ps.Holes(); len(h) > 0 {
			return strings.Join(h, "; ")
		}
		return err.Error()
	}
	return ""
}

// cmdInstrument delegates to the memory, toolbox and ontology CLIs under the distribution's
// workspace dir and root.
func cmdInstrument(name, dist string, f cliFlags, args []string, io IO) int {
	d, err := parseDist(dist)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	memory.DefaultWorkspaceDir = d.WorkspaceDir
	root := f.root
	if root == "" {
		cwd, _ := os.Getwd()
		if r, ok := findWorkspace(cwd, d.WorkspaceDir); ok {
			root = r
		}
	}
	switch name {
	case "memory":
		if root != "" {
			args = append([]string{root}, args...)
		}
		return memory.CLI(args, io.Out, io.Err)
	case "toolbox":
		if root != "" {
			args = append([]string{root}, args...)
		}
		return toolbox.CLI(args, io.Out, io.Err)
	default:
		lex := lexiconFor()
		onto.CLILayout = lex.Layout()
		if root != "" {
			args = append([]string{"--root", root}, args...)
		}
		return onto.CLI(args, io.Out, io.Err)
	}
}

func findWorkspace(dir, workspaceDir string) (string, bool) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if st, err := os.Stat(filepath.Join(d, workspaceDir)); err == nil && st.IsDir() {
			return d, true
		}
		up := filepath.Dir(d)
		if up == d {
			return "", false
		}
		d = up
	}
}

// cmdRun is the non-interactive form. The ask comes from the arguments; stdin is read only when
// it is asked for — no ask, or the ask "-" — so an inherited pipe that never closes (CI, another
// agent, cron) cannot hang a run. The answer is printed in the output format; the exit code is
// the loop's.
func (a *App) cmdRun(ctx context.Context, ask string, io IO) int {
	if strings.TrimSpace(ask) == "" || strings.TrimSpace(ask) == "-" {
		ask = ""
		if f, ok := io.In.(*os.File); !ok || f != os.Stdin || !isTerminal(f) {
			if in, _ := readAllLimited(io.In, 4<<20); strings.TrimSpace(in) != "" {
				ask = strings.TrimSpace(in)
			}
		}
	}
	if ask == "" {
		fmt.Fprintf(io.Err, "@S FAIL\n@? run needs an ask: %s run \"<ask>\"\n", a.Cfg.Dist.Name)
		return 2
	}
	e, err := a.Engine()
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	a.Hooks.SessionStart(ctx)
	if why := a.Hooks.UserPrompt(ctx, ask); why != "" {
		fmt.Fprintf(io.Err, "@S FAIL\n@? refused by a userPrompt hook: %s\n", why)
		return 2
	}
	if a.Cfg.Output.Stream && a.Cfg.Output.Format == "text" {
		e.OnDelta = func(t string) { fmt.Fprint(io.Out, t) }
	}
	s := e.NewSession()
	r, err := s.Turn(ctx, ask)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	if e.OnDelta != nil && strings.TrimSpace(r.Text) != "" {
		fmt.Fprintln(io.Out)
	}
	a.printResult(r, e.OnDelta != nil)
	a.Sessions.Sync(a.SessionID, s, a.mountModel.Ref.Model)
	if r.Verdict != "" {
		a.record(s, "verdict", map[string]interface{}{"verdict": r.Verdict, "wrote": s.Wrote})
	}
	if !a.Opt.Quiet {
		fmt.Fprintln(io.Err, a.Subagent.StatusLine(s))
	}
	return r.Exit()
}

func (a *App) cmdResume(ctx context.Context, id, ask string, io IO) int {
	e, err := a.Engine()
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	s := e.NewSession()
	if err := a.loadSession(s, id); err != nil {
		// a loop transcript (a checkpointed run) is the other thing a session can resume from
		if r, err2 := e.Resume(ctx, id, ask); err2 == nil {
			a.printResult(r, false)
			return r.Exit()
		}
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	if ask == "" {
		ask = "resumed — continue from where the session stopped"
	}
	if io.In != nil {
		if f, ok := io.In.(*os.File); ok && f == os.Stdin && isTerminal(f) {
			if a.wantsTUI(false) {
				a.startBoard(ctx)
				return a.TUI(ctx)
			}
			fmt.Fprintf(io.Err, "resumed %s (%d messages)\n", id, len(s.Messages))
			return a.REPL(ctx)
		}
	}
	r, err := s.Turn(ctx, ask)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	a.printResult(r, false)
	a.Sessions.Sync(a.SessionID, s, a.mountModel.Ref.Model)
	return r.Exit()
}

func (a *App) cmdSessions(io IO) int {
	if a.Sessions == nil {
		fmt.Fprintln(io.Err, "@S FAIL\n@? sessions are off (sessions.enabled)")
		return 2
	}
	list, err := a.Sessions.List()
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	if len(list) == 0 {
		fmt.Fprintf(io.Out, "no sessions under %s\n", a.Sessions.Dir)
		return 0
	}
	for _, i := range list {
		fmt.Fprintln(io.Out, i.Line())
	}
	return 0
}

func (a *App) cmdStatus(io IO) int {
	if a.Workspace == nil {
		for _, l := range a.Cfg.StatusLines() {
			fmt.Fprintln(io.Out, l)
		}
		for _, h := range a.Holes() {
			fmt.Fprintln(io.Out, "@? "+h)
		}
		return 0
	}
	for _, l := range a.StatusLines() {
		fmt.Fprintln(io.Out, l)
	}
	fmt.Fprintf(io.Out, "workspace: %s · %s · %d members · %d skills · %d commands · %d foreign agents\n", a.Root, a.Lex.Vocab, len(a.Workspace.Members), len(a.Found.Skills), len(a.Found.Commands), len(a.Found.Agents))
	for _, h := range a.Found.Holes {
		fmt.Fprintln(io.Out, "@? "+h)
	}
	if dir := a.journalDir(); dir != "-" {
		if dir == "" {
			dir = filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, "instruments", "loop")
		}
		runs, _ := loop.Runs(dir)
		fmt.Fprintf(io.Out, "runs: %d journals under %s\n", len(runs), relOrAbs(a.Root, dir))
		for i, r := range runs {
			if i == 10 {
				fmt.Fprintf(io.Out, "  … %d more\n", len(runs)-10)
				break
			}
			fmt.Fprintf(io.Out, "  %s — %s — as %s — %d steps\n", r.ID, r.Status, r.As, len(r.Steps))
		}
	}
	if a.Sessions != nil {
		list, _ := a.Sessions.List()
		fmt.Fprintf(io.Out, "sessions: %d under %s\n", len(list), a.Sessions.Dir)
	}
	for _, l := range a.evolutionStatus() {
		fmt.Fprintln(io.Out, l)
	}
	if bl := a.boardLine(); bl != "" {
		fmt.Fprintln(io.Out, bl)
	}
	return 0
}

func (a *App) cmdUsage(f cliFlags, io IO) int {
	dir := filepath.Join(a.Root, a.Cfg.Dist.WorkspaceDir, "instruments", "usage")
	recs, err := ReadUsage(dir)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	var since time.Time
	if f.since != "" {
		for _, layout := range []string{"2006-01-02", time.RFC3339} {
			if t, err := time.Parse(layout, f.since); err == nil {
				since = t
				break
			}
		}
		if since.IsZero() {
			fmt.Fprintf(io.Err, "@S FAIL\n@? --since wants YYYY-MM-DD or RFC3339, got %q\n", f.since)
			return 2
		}
	}
	if len(recs) == 0 {
		fmt.Fprintf(io.Out, "no usage journaled under %s\n", relOrAbs(a.Root, dir))
		return 0
	}
	for _, l := range RollupOf(recs, since, f.session).Lines() {
		fmt.Fprintln(io.Out, l)
	}
	return 0
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func readAllLimited(r io.Reader, n int64) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, n))
	return string(b), err
}

// ranksSummary is one line per rank for the human.
func ranksSummary(rs workspace.Ranks) string {
	var names []string
	for _, r := range rs {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
