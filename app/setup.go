package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"

	"github.com/Kaginari/agent-one/tui"
)

// The first-run setup: after `init` founds a workspace on a terminal, a Huh form asks which model the
// workspace runs on and writes the answer to the workspace's config.yaml. Skipped on a pipe, with --plain,
// when the workspace already has a config, or when <PREFIX>NO_SETUP is set.

type setupChoice struct {
	provider string // "" keeps the global config
	model    string
	fallback string
	baseURL  string
	keyEnv   string
	kind     string // provider wire type: anthropic | openai
}

var setupPresets = map[string]setupChoice{
	"openrouter": {provider: "openrouter", model: "openrouter/google/gemma-4-31b-it:free", fallback: "openrouter/thinkingmachines/inkling-small:free", keyEnv: "OPENROUTER_API_KEY", kind: "openai"},
	"anthropic":  {provider: "anthropic", model: "anthropic/claude-sonnet-5", keyEnv: "ANTHROPIC_API_KEY", kind: "anthropic"},
	"openai":     {provider: "openai", model: "openai/", keyEnv: "OPENAI_API_KEY", kind: "openai"},
	"ollama":     {provider: "ollama", model: "ollama/", kind: "openai"},
	"custom":     {provider: "vllm", model: "vllm/", baseURL: "http://127.0.0.1:8000/v1", keyEnv: "VLLM_API_KEY", kind: "openai"},
}

// wantsSetup says whether init should ask.
func wantsSetup(dist, root string, f cliFlags, io IO) bool {
	if f.plain || io.Env(prefixOf(dist)+"NO_SETUP") != "" {
		return false
	}
	in, ok1 := io.In.(*os.File)
	out, ok2 := io.Out.(*os.File)
	if !ok1 || !ok2 || !isTerminal(in) || !isTerminal(out) {
		return false
	}
	d, err := parseDist(dist)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(root, d.WorkspaceDir, "config.yaml"))
	return errors.Is(err, os.ErrNotExist)
}

// runSetup shows the mascot, asks, and writes the workspace's config.yaml; the path written + the
// board port picked free, or ("", 0, nil) when the global config is kept.
func runSetup(dist, root string, io IO) (string, int, error) {
	art, tag := tui.Mascot(dist)
	fmt.Fprintln(io.Out)
	for i, a := range art {
		side := ""
		switch i {
		case 1:
			side = tui.Title(dist) + " setup"
		case 2:
			side = tag
		}
		fmt.Fprintf(io.Out, "  %s   %s\n", a, side)
	}
	fmt.Fprintln(io.Out)

	pick := "keep"
	keep := "Keep the global config (~/.config/" + dist + "/config.yaml)"
	if !globalConfigExists(dist) {
		keep = "Decide later — no config yet (`" + dist + " config path` shows where it goes)"
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Which model should this workspace run on?").
			Options(
				huh.NewOption(keep, "keep"),
				huh.NewOption("OpenRouter free models — gemma-4-31b, inkling-small as fallback", "openrouter"),
				huh.NewOption("Anthropic — Claude", "anthropic"),
				huh.NewOption("OpenAI", "openai"),
				huh.NewOption("Ollama — a local model, no key", "ollama"),
				huh.NewOption("An OpenAI-compatible server (vLLM, LM Studio…)", "custom"),
			).Value(&pick),
	)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin))
	if err := form.Run(); err != nil {
		return "", 0, err
	}
	if pick == "keep" {
		return "", 0, nil
	}
	c := setupPresets[pick]
	fields := []huh.Field{
		huh.NewInput().Title("Model").Description("<provider>/<model-id>").Value(&c.model).Validate(func(s string) error {
			if _, id, ok := strings.Cut(s, "/"); !ok || id == "" {
				return errors.New("name the model after the provider: <provider>/<model-id>")
			}
			return nil
		}),
	}
	if pick == "custom" {
		fields = append(fields, huh.NewInput().Title("Base URL").Value(&c.baseURL))
	}
	if c.keyEnv != "" {
		fields = append(fields, huh.NewInput().Title("The environment variable that holds the key").
			Description("only its name is written — the key itself never lands in a file").Value(&c.keyEnv))
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run(); err != nil {
		return "", 0, err
	}
	d, _ := parseDist(dist)
	port := pickBoardPort()
	p := filepath.Join(root, d.WorkspaceDir, "config.yaml")
	if err := os.WriteFile(p, []byte(setupYAML(c, port)), 0o644); err != nil {
		return "", 0, err
	}
	if c.keyEnv != "" && io.Env(c.keyEnv) == "" {
		fmt.Fprintf(io.Out, "\n  %s is not set in this shell — export it before the first session.\n", c.keyEnv)
	}
	return relOrAbs(root, p), port, nil
}

// setupYAML is the workspace's config for a choice: every entry carries a comment naming what it does
// and its possible values — examples/workspace-rules.yaml preaches the same shape. ui.board.port is
// picked free at init above the fixed ports.
func setupYAML(c setupChoice, port int) string {
	var b strings.Builder
	b.WriteString("# agent-one workspace config — written by the init setup; edit freely.\n")
	b.WriteString("# `config explain` shows where each live value came from.\n")
	fmt.Fprintf(&b, "# The board this session serves is http://127.0.0.1:%d (picked free at init, clear of the fixed ports).\n\n", port)
	b.WriteString("providers:\n")
	fmt.Fprintf(&b, "  %s:\n", c.provider)
	b.WriteString("    enabled: true          # only enabled providers get dialed · true | false\n")
	fmt.Fprintf(&b, "    type: %s             # anthropic | openai | mock — openai = any OpenAI-compatible endpoint\n", c.kind)
	if c.baseURL != "" {
		fmt.Fprintf(&b, "    baseURL: %s   # where calls go\n", c.baseURL)
	}
	if c.keyEnv != "" {
		fmt.Fprintf(&b, "    apiKeyEnv: %s    # the env var's NAME — the key's value never lands in files\n", c.keyEnv)
	}
	b.WriteString("    toolCalls: native      # native | text — native: the API's tool channel; text: tags parsed from the reply\n")
	b.WriteString("    contextWindow: auto    # auto | <n> — auto reads /v1/models (max_model_len)\n\n")
	b.WriteString("models:\n")
	if c.fallback != "" {
		fmt.Fprintf(&b, "  default: { model: %s, fallback: %s }   # provider/id the session rides; fallback on 429/5xx/transport, never on refusal\n", c.model, c.fallback)
	} else {
		fmt.Fprintf(&b, "  default: %s    # provider/id the session rides; roles/ranks/tasks/members can override per kind\n", c.model)
	}
	fmt.Fprintf(&b, "\nui:\n  board:\n    autostart: true   # the live board starts with each session · true | false\n    port: %d        # the dashboard's address — http://127.0.0.1:%d\n\n", port, port)
	b.WriteString("policy:                     # the convention as code paths\n")
	b.WriteString("  humanGate: { enabled: true }    # a human answers asks — outward/destructive rides here · true | false\n")
	b.WriteString("  closingCheck: { enabled: true } # at closing: an answer earns its close by evidence (the witness demand) · true | false\n")
	b.WriteString("  guts: { enabled: true }         # the per-step value sense: scores acts at verify, speaks once in a slide past floor · true | false; window/floor tunable\n")
	return b.String()
}

func worldWord(dist string) string {
	if dist == "agent-one" {
		return "workspace"
	}
	return "workspace"
}

// askLevel asks how deep to found the workspace (init --level).
func askLevel(dist string) (string, error) {
	level := "light"
	w := worldWord(dist)
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("How should this "+w+" be founded?").
			Options(
				huh.NewOption("Light — the policy, the log and the instruments; members come later", "light"),
				huh.NewOption("Medium — also a team read from the tree: packages, imports, tests (no model)", "medium"),
				huh.NewOption("Complex — the model reads all the code and founds the team: skills, agents, relations", "complex"),
			).Value(&level),
	)).WithTheme(huh.ThemeFunc(huh.ThemeCatppuccin)).Run()
	return level, err
}

// globalConfigExists is whether the machine-wide layer holds any config file.
func globalConfigExists(dist string) bool {
	d, err := parseDist(dist)
	if err != nil {
		return false
	}
	for _, n := range []string{"config.yaml", "config.yml", "config.json"} {
		if _, err := os.Stat(filepath.Join(d.GlobalDir, n)); err == nil {
			return true
		}
	}
	return false
}
