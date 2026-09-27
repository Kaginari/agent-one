package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Kaginari/agent-one/config"
	"github.com/Kaginari/agent-one/discover"
	"github.com/Kaginari/agent-one/ui"
	"github.com/Kaginari/agent-one/workspace"
)

// cmdUI is `ui init|scan|check`: the component system's instrument (README, Web UIs). The app's ui
// dir is the source of truth; the legend under <workspace>/ui-assets/ is derived from it.
func cmdUI(dist string, f cliFlags, args []string, io IO) int {
	d, err := parseDist(dist)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	root := f.root
	if root == "" {
		root, _ = os.Getwd()
	}
	sub, dir, noShots, asJSON := "", "", false, f.json
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--dir" && i+1 < len(args):
			i++
			dir = args[i]
		case strings.HasPrefix(a, "--dir="):
			dir = strings.TrimPrefix(a, "--dir=")
		case a == "--no-shots":
			noShots = true
		case a == "--json":
			asJSON = true
		case sub == "":
			sub = a
		case dir == "":
			dir = a
		}
	}
	switch sub {
	case "init":
		if dir == "" {
			dir = "ui"
		}
		made, err := ui.Init(root, dir)
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? ui init: %v\n", err)
			return 2
		}
		for _, m := range made {
			fmt.Fprintln(io.Out, "  + "+m)
		}
		m, err := ui.Scan(root, dir)
		if err == nil {
			_, err = ui.Write(root, d.WorkspaceDir, m)
		}
		if err != nil {
			fmt.Fprintf(io.Err, "@S FAIL\n@? ui init: %v\n", err)
			return 2
		}
		fmt.Fprintf(io.Out, "@S PASS ui foundation in %s/ (%d new files); the legend is %s/%s/legend.md\n", dir, len(made), d.WorkspaceDir, ui.AssetsDir)
		return 0
	case "scan", "check":
	case "", "help":
		fmt.Fprintf(io.Out, "%[1]s ui init [dir]              lay the foundation (tokens, palettes, layout, two example components) into dir (default ui/)\n"+
			"%[1]s ui scan [--dir d]          regenerate the legend: %[2]s/ui-assets/{manifest.json,legend.md,catalogue.html}\n"+
			"%[1]s ui check [--dir d] [--no-shots] [--json]\n"+
			"                              scan, lint (tokens only, declared classes, true headers), render the catalogue at %v × %v\n", dist, d.WorkspaceDir, ui.Widths, ui.Themes)
		return 0
	default:
		fmt.Fprintf(io.Err, "@S FAIL\n@? unknown ui command %q — init, scan, check\n", sub)
		return 2
	}
	if dir == "" {
		dir = ui.FindDir(root)
	}
	if dir == "" {
		fmt.Fprintf(io.Err, "@S FAIL\n@? no ui dir (looked in %s) — `%s ui init` lays one, or name it with --dir\n", strings.Join(ui.Candidates, ", "), dist)
		return 2
	}
	m, err := ui.Scan(root, dir)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? ui %s: %v\n", sub, err)
		return 2
	}
	changed, err := ui.Write(root, d.WorkspaceDir, m)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? ui %s: %v\n", sub, err)
		return 2
	}
	if sub == "scan" {
		fmt.Fprintf(io.Out, "@S PASS legend of %s: %d components, %d tokens (%d files rewritten)\n", dir, len(m.Components), len(m.Tokens), len(changed))
		return 0
	}
	findings := ui.Lint(root, m)
	var shots []ui.Shot
	hole := ""
	if !noShots {
		if b := ui.Browser(io.Env); b == "" {
			hole = "no headless Chromium (chromium, google-chrome, or $UI_CHROMIUM) — the catalogue was not rendered; the lints ran"
		} else {
			home, _ := os.UserHomeDir()
			shots, err = ui.Shoot(context.Background(), b, root, d.WorkspaceDir, m, home)
			if err != nil {
				hole = "the catalogue did not render: " + err.Error()
			}
		}
	}
	for _, s := range shots {
		if s.Overflow != "" {
			findings = append(findings, ui.Finding{Level: "error", File: s.File, Rule: "overflow", Msg: fmt.Sprintf("scrolls sideways at %dpx (%s): %s", s.Width, s.Theme, s.Overflow)})
		}
		if s.Err != "" {
			hole = fmt.Sprintf("%dpx %s did not render: %s", s.Width, s.Theme, s.Err)
		}
	}
	errs := ui.Errors(findings)
	if asJSON {
		out := map[string]any{"dir": dir, "components": len(m.Components), "findings": findings, "shots": shots, "hole": hole, "pass": errs == 0}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(io.Out, string(b))
	} else {
		for _, fd := range findings {
			fmt.Fprintln(io.Out, "  "+fd.String())
		}
		for _, s := range shots {
			if s.Err == "" {
				fmt.Fprintln(io.Out, "  shot "+s.File)
			}
		}
		if hole != "" {
			fmt.Fprintln(io.Out, "@? "+hole)
		}
		word := "PASS"
		if errs > 0 {
			word = "FAIL"
		}
		fmt.Fprintf(io.Out, "@S %s ui %s: %d components, %d errors, %d warnings, %d shots — look at the shots before calling it done\n", word, dir, len(m.Components), errs, len(findings)-errs, len(shots))
	}
	if errs > 0 {
		return 1
	}
	return 0
}

// uiGate is the gate's ui check: on a turn that wrote under the ui dir, the legend is
// regenerated (it is derived, never the model's to write) and the lints must pass.
func (a *App) uiGate(wrote []string) []string {
	dir := ui.FindDir(a.Root)
	if dir == "" {
		return nil
	}
	touched := false
	for _, w := range wrote {
		if w == dir || strings.HasPrefix(strings.TrimPrefix(w, "./"), dir+"/") {
			touched = true
			break
		}
	}
	if !touched {
		return nil
	}
	m, err := ui.Scan(a.Root, dir)
	if err != nil {
		return []string{err.Error()}
	}
	_, _ = ui.Write(a.Root, a.Cfg.Dist.WorkspaceDir, m)
	var out []string
	for _, f := range ui.Lint(a.Root, m) {
		if f.Level == "error" {
			out = append(out, f.String())
		}
	}
	if len(out) > 8 {
		out = append(out[:8], fmt.Sprintf("… %d more (`%s ui check`)", len(out)-8, a.Cfg.Dist.Name))
	}
	return out
}

// withUIGate hangs the ui lints on the gate when policy.gate.ui is on.
func (a *App) withUIGate(h workspace.HookOptions) workspace.HookOptions {
	if a.Cfg.Policy.Gate.UI {
		h.Gate.UI = a.uiGate
	}
	return h
}

// builtinSkills are the Skills every workspace gets from the binary: the ui Skill, spoken in this
// distribution's words.
func builtinSkills(d config.Dist) []discover.Skill {
	text := strings.NewReplacer("{{BIN}}", d.Name, "{{WORLD}}", d.WorkspaceDir).Replace(ui.Skill())
	return []discover.Skill{discover.Builtin(text)}
}
