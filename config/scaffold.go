package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Kaginari/agent-one/config/yaml"
)

// `config scaffold [--check]` — the local-layer register. It writes the machine-only config
// set a workspace may carry — config.local.yaml plus one file per section, <section>.local.yaml —
// into the workspace's workspace-dir (e.g. .agent-one/), each with default values and a comment per key.
// The set is never overwritten: an existing file is kept, only reported. .agent-one/.gitignore
// gains `*.local.yaml`, because machine-local files never travel by git (Absolute Rule I).
// The files' names and shapes come from the running binary's own defaults, so when design
// moves — a new section, a renamed key — re-running the same command syncs the scaffold:
// `--check` writes nothing and answers what drifted (unknown keys in local files as `@?`,
// defaults sections with no local file yet as `@F`).

// scaffoldAgent: one registered section file's agent (value form, defaults + a comment per key).
// The set is filtered by the running binary's own sections (knownSections): a distribution
// that ships no `policy` section registers no policy.local.yaml — design moves, the register follows.
var scaffoldAgents = map[string]string{}

// scaffoldFile is one registered file: its name and its canonical agent.
type scaffoldFile struct {
	Name  string
	Agent string
}

// scaffoldFiles returns the register set: the base config.local.yaml plus one local file per
// curated section the binary actually knows — filtered on this binary's own sections, so
// a design move (new/renamed/dropped section) is followed, not shipped broken.
func scaffoldFiles(known map[string]bool) []scaffoldFile {
	files := []scaffoldFile{{"config.local.yaml", fmt.Sprintf(localHead, "config.local.yaml") + `
# Anything may be set here whole-map style (all sections, nested keys); for one section per
# file, prefer <section>.local.yaml beside this file — it loads after this file, so it wins.
{}
`}}
	type namedAgent struct{ name, section string }
	for _, nb := range []namedAgent{
		{"providers.local.yaml", "providers"},
		{"models.local.yaml", "models"},
		{"policy.local.yaml", "policy"},
		{"budgets.local.yaml", "budgets"},
		{"permissions.local.yaml", "permissions"},
		{"guards.local.yaml", "guards"},
	} {
		sec := nb.section
		if a, ok := sectionAliases[sec]; ok {
			sec = a
		}
		if !known[sec] {
			continue
		}
		agent, ok := scaffoldAgents[nb.section]
		if !ok {
			continue
		}
		files = append(files, scaffoldFile{nb.name, agent})
	}
	return files
}

const localHead = `# %s — the machine-local layer of this workspace.
# Git-ignored by policy (Absolute Rule I: machine-local state never travels); the workspace's own
# choices live in config.yaml (tracked), machine-wide ones in the global layer.
# Layer ladder (later wins): built-in defaults → global → project → local (here) → env → flags.
# 'agent-one config explain' names the file and line every value came from.
# Re-register or drift-check this set after a harness upgrade: 'agent-one config scaffold [--check]'.
`

func init() {
	scaffoldAgents["providers"] = fmt.Sprintf(localHead, "providers.local.yaml") + `
# providers — where the models are (the VALUE of the providers section: no wrapper key).
# Keys are named, never written: apiKeyEnv holds a variable's NAME, never the secret.
#   anthropic: { enabled: false }
#   gateway:                        # the conduit front-door — any OpenAI-compatible endpoint is type: openai
#     enabled: true
#     type: openai
#     baseURL: https://gateway.example.com/v1/
#     apiKeyEnv: GATEWAY_API_KEY
#     toolCalls: native             # native | text (tags in the text)
#     contextWindow: 131072         # or "auto" to read /v1/models
{}
`
	scaffoldAgents["models"] = fmt.Sprintf(localHead, "models.local.yaml") + `
# models — who runs on what (the VALUE of the models section).
#   default: { model: gateway/kimi-latest, fallback: gateway/glm-latest }
#   roles:
#     analyst: { model: ~ }      # reads
#     judge:    { model: ~ }      # verdicts
#     drafter:       { model: ~ }      # drafts
#   ranks: { zone worker: ~, domain owner: ~ }  ·  members: { zone-auth: ~ }  ·  tasks: { drain: ~, gate: ~ }
{}
`
	scaffoldAgents["policy"] = fmt.Sprintf(localHead, "policy.local.yaml") + `
# policy — budgets, the context-stress instrument, and the council (the VALUE of the policy section).
budget:
  steps: -1             # run step ceiling; -1 = off (0 reads as "take the default")
  minutes: -1           # wall-clock ceiling; -1 = off
  contextTokens: 200000 # the hard wall — an agent's context ceiling
  stressTokens: 180000  # the stress zone floor; the drain must trigger below this
  salvageTokens: 4096   # budget kept to checkpoint an ending run honestly
stress:
  escalateAt: 5         # strain reading at which a dispatched agent's council warrant fires;
                        # 0/-1 = the council is off (no warrants, no ledger)
  maxConsults: 3        # warrants an agent may carry across re-dispatches; past this the run
                        # hardens to ESCALATE and rises to the human; a clean landing clears the ledger
`
	scaffoldAgents["budgets"] = fmt.Sprintf(localHead, "budgets.local.yaml") + `
# budgets — spend ceilings (the VALUE of the budgets section). Off = 0.
session: { tokens: 0, usd: 0 }
subagent:   { tokens: 0, usd: 0 }
`
	scaffoldAgents["permissions"] = fmt.Sprintf(localHead, "permissions.local.yaml") + `
# permissions — the human gate's ask / allow / deny per command (the VALUE of the
# permissions section). An allow on bash/git/webfetch is a loosening that
# 'agent-one status' always lists; a bare bash:* allow is refused.
rules: []
#   - { match: "bash:kubectl get*", action: allow }
#   - { match: "bash:git push*",    action: deny  }
`
	scaffoldAgents["guards"] = fmt.Sprintf(localHead, "guards.local.yaml") + `
# guard — catastrophic patterns REFUSED before any approval; no rule or --approve
# can run them (the VALUE of the guard section; a bare list of patterns is also lawful).
patterns: []
#   - '(^|[[:space:]])terraform[[:space:]]+destroy'
#   - 'kubectl[[:space:]]+delete[[:space:]]+(ns|namespace|node)'
`
}

// knownSections is the legal top-level key set for this binary: the defaults' keys (the
// section-file naming policy) ∪ the Config struct's json tags (a key may be lawful without a
// default — rankSet, ranks). Drift must name what this binary cannot read, never a legal
// key that merely ships no default.
func knownSections() map[string]bool {
	known := map[string]bool{}
	if n, err := yaml.ParseJSON([]byte(defaultsJSON), "default"); err == nil {
		for _, k := range n.Keys {
			known[k] = true
		}
	}
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		if j := t.Field(i).Tag.Get("json"); j != "" && j != "-" {
			known[strings.Split(j, ",")[0]] = true
		}
	}
	return known
}

// resolveScaffoldTarget answers where the local layer lives: the workspace root and the
// distribution, without loading the config (a broken config must not block the register).
func resolveScaffoldTarget(o Options) (root string, d Dist, err error) {
	if o.Env == nil {
		o.Env = os.Getenv
	}
	if o.Home == "" {
		o.Home = o.Env("HOME")
	}
	if o.Home == "" {
		o.Home, _ = os.UserHomeDir() // a "~/" path must never be taken literally
	}
	if o.Cwd == "" {
		o.Cwd, _ = os.Getwd() // an empty cwd must never register "<cwd>/.agent-one" by luck
	}
	root = o.Root
	if root == "" {
		root = walkUp(o.Cwd, dist.WorkspaceDir)
		if root == "" {
			root = o.Cwd
		}
	}
	d, err = ParseDist(Name, o.Home, o.Env)
	return root, d, err
}

// runScaffold registers (or, with checkOnly, drift-checks) the local-layer config set.
func runScaffold(o Options, checkOnly bool, stdout, stderr io.Writer) int {
	root, d, err := resolveScaffoldTarget(o)
	if err != nil {
		fmt.Fprintf(stderr, "@S FAIL\n@? %v\n", err)
		return 2
	}
	dir := filepath.Join(root, d.WorkspaceDir)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "@S FAIL\n@? %s is no workspace — no %s here\n", root, d.WorkspaceDir)
		return 2
	}
	name := filepath.Base(root)
	known := knownSections()
	if checkOnly {
		return scaffoldCheck(dir, name, known, stdout)
	}
	wrote, kept := 0, 0
	fmt.Fprintf(stdout, "@S SCAFFOLD %s · %s\n", d.Name, root)
	for _, sf := range scaffoldFiles(known) {
		p := filepath.Join(dir, sf.Name)
		if _, err := os.Stat(p); err == nil {
			fmt.Fprintf(stdout, "@F kept %s (never overwritten)\n", sf.Name)
			kept++
			continue
		}
		if err := os.WriteFile(p, []byte(sf.Agent), 0o644); err != nil {
			fmt.Fprintf(stdout, "@? %s: %v\n", sf.Name, err)
			continue
		}
		fmt.Fprintf(stdout, "@F wrote %s\n", sf.Name)
		wrote++
	}
	gi := filepath.Join(dir, ".gitignore")
	if b, err := os.ReadFile(gi); err == nil && strings.Contains(string(b), "*.local.yaml") {
		// already ignored
	} else {
		f, err := os.OpenFile(gi, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintf(stdout, "@? .gitignore untouched: %v\n", err)
		} else {
			fmt.Fprintln(f, "*.local.yaml")
			_ = f.Close()
			fmt.Fprintf(stdout, "@F .gitignore += *.local.yaml\n")
		}
	}
	fmt.Fprintf(stdout, "@E %d wrote · %d kept\n", wrote, kept)
	return 0
}

// scaffoldCheck writes nothing: it names drift between the workspace's local files and the
// running binary's defaults — unknown sections or top keys as `@?`, defaults sections
// with no local file yet as `@F`. Exit 1 when drift is found, mirroring `check`.
func scaffoldCheck(dir, name string, known map[string]bool, stdout io.Writer) int {
	ents, _ := os.ReadDir(dir)
	var files []string
	for _, e := range ents {
		n := e.Name()
		if strings.HasSuffix(n, ".local.yaml") || strings.HasSuffix(n, ".local.yml") || strings.HasSuffix(n, ".local.json") ||
			n == "config.local.yaml" || n == "config.local.yml" || n == "config.local.json" {
			files = append(files, n)
		}
	}
	sort.Strings(files)
	fmt.Fprintf(stdout, "@S CHECK %s — local-layer drift vs this binary's defaults\n", name)
	// Parse every local file through the loader's YAML so checks see real keys.
	seenSections := map[string]bool{}
	drift := 0
	r := &reader{o: Options{}}
	for _, n := range files {
		p := filepath.Join(dir, n)
		node, err := r.file(p)
		if err != nil {
			fmt.Fprintf(stdout, "@? %s: %v\n", n, err)
			drift++
			continue
		}
		var section string
		if strings.HasSuffix(strings.TrimSuffix(n, filepath.Ext(n)), ".local") {
			section = strings.TrimSuffix(strings.TrimSuffix(n, filepath.Ext(n)), ".local")
			if section == "config" {
				section = "" // config.local.yaml is the whole local map, not a section
			}
			if a, ok := sectionAliases[section]; ok && section != "" {
				section = a
			}
			if section != "" && !known[section] {
				fmt.Fprintf(stdout, "@? %s: names section %q — no such section in this binary\n", n, section)
				drift++
				continue
			}
			if section != "" {
				seenSections[section] = true
			}
		}
		for _, k := range node.Keys {
			if !known[k] {
				if section != "" {
					continue // wrapped under its own section — key level checked on load
				}
				fmt.Fprintf(stdout, "@? %s: key %q — no such section in this binary\n", n, k)
				drift++
			} else {
				seenSections[k] = true
			}
		}
		fmt.Fprintf(stdout, "@F %s loads clean\n", n)
	}
	for _, sf := range scaffoldFiles(known) {
		s := strings.TrimSuffix(sf.Name, ".yaml")
		if s == "config.local" {
			continue
		}
		s = strings.TrimSuffix(s, ".local") // the registered name carries the section's own name
		if a, ok := sectionAliases[s]; ok {
			s = a
		}
		if !seenSections[s] {
			fmt.Fprintf(stdout, "@F no local file for section %q — run 'agent-one config scaffold' to register %s\n", s, sf.Name)
		}
	}
	fmt.Fprintf(stdout, "@E %d drift\n", drift)
	if drift > 0 {
		return 1
	}
	return 0
}
