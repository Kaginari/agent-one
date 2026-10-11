<p align="center"><img src="portraits/orchestrator.png" width="96" alt="Orchestrator"></p>

<h1 align="center">Agent-One</h1>

<p align="center"><b>A coding-agent harness that enforces its policies in code.</b><br>
One static Go binary. Review gates, an append-only change log, human approval for risky actions,
a sandboxed shell and native memory are part of the runtime — not instructions the model may
ignore.</p>

---

## Why

Agents read a project's rules and then, sometimes, don't follow them. Agent-One turns a directory
into a **workspace** governed by [`AGENT-ONE.md`](AGENT-ONE.md) and runs the agent inside it:

- **Review gate** — every turn that changed the workspace is checked before it's reported done:
  the owning member made the change, its invariants hold, the requested work is complete, and the
  owning doc changed with the code. The verdict is appended to `log.md`.
- **Human approval** — every action is classified (`read` · `write` · `outward` ·
  `destructive`) before it runs. Outward and destructive actions ask you; per-command rules
  (`bash:git push*` → allow / ask / deny) tune it. A model can only make a classification stricter.
- **Sandbox** — `bash` runs in a persistent shell under `bwrap`: read-only filesystem except the
  workspace, no network unless the action was approved as outward, secrets stripped from the env.
- **Native memory** — recall before every step, record after; knowledge the agent held but never
  wrote down is surfaced and filed. When the context window fills, it is **drained** to memory by
  reference instead of being summarized away.
- **Roles and model routing** — the orchestrator dispatches ephemeral subagents to coordinators,
  domain owners and zone workers; the analyst, judge and drafter roles can each run on their own
  model (a cheap model to read, a strong one to judge).
- **Everything on, configuration takes away** — every tool, policy and instrument can be switched
  off in YAML or JSON, and a disabled policy is always reported, never silent.

## Install

```sh
curl -fsSL -o install.sh https://raw.githubusercontent.com/Kaginari/agent-one/main/install.sh
sh install.sh                     # downloads the release for your OS and checks it against checksums.txt
# or
go install github.com/Kaginari/agent-one/cmd/agent-one@latest
```

Release archives for linux/macOS × amd64/arm64 ship with `checksums.txt` and build provenance:
`gh attestation verify <archive> --repo Kaginari/agent-one`. A container image to try the binary:
`docker run --rm -it -e ANTHROPIC_API_KEY -v "$PWD:/work" ghcr.io/kaginari/agent-one:latest` — the
binary alone on Debian slim (no bubblewrap, git or toolchains); for real work in a container use
`--containered`, which builds a runtime with them.

**Requirements.** Linux or macOS. `bubblewrap` (`bwrap`) for the sandboxed shell on Linux — without it
the shell runs unsandboxed and `status` says so; macOS has no bwrap. Docker for `--containered`.
Chromium or Chrome for `ui check`'s screenshots.

**Data boundary.** No telemetry. The binary contacts only the providers you configure (and the
package or container registries you name); any other network act is `outward` and asks you, and the
sandboxed shell has no network unless an `outward` act was approved.

## Quick start

```sh
cd your-project
export ANTHROPIC_API_KEY=…        # or any OpenAI-compatible endpoint (vLLM, Ollama, OpenRouter)
agent-one init --level complex      # found a workspace here — light: the policy only · medium: a team read from the tree, no model · complex: the model reads all the code and hires the team
agent-one                           # the live session — the dashboard opens at http://127.0.0.1:7411
agent-one run "add a health check endpoint and its test"
```

## Inside Claude Code

You can keep Claude Code as the agent and let agent-one hold the policy. Claude Code runs the model, the
tools and the session on your own plan; the binary runs from Claude Code's hooks, where the model
cannot skip it. It never calls a model itself, so it needs no API key.

```sh
cd your-project && agent-one init   # found the workspace, if it is not one yet
agent-one install --claude          # shows the hooks; --yes writes .claude/settings.local.json
agent-one install --claude --global --yes   # or for every workspace on this machine (silent elsewhere)
```

| Claude Code event | What the policy does |
|---|---|
| `SessionStart` (also after `/clear` and compaction) | the principles, the log's last entries and how the policy runs here, as context |
| `UserPromptSubmit` | the turn's baseline, and recall for the ask: memory anchors and the tools that fit |
| `PreToolUse` | the guard refuses the catastrophic; outward and destructive acts ask — it only ever tightens Claude Code's own permissions |
| `PostToolUse` | the paths a file tool wrote, for the gate |
| `Stop` | the gate over everything the turn changed (shell writes too): an owned path changed without its doc, or a failing verify line, sends the turn back with the reasons; the verdict lands in `log.md` |

## The CLI, seen

A working session: the ask up top, every classified step drawn as it lands:

![the session](docs/screens/agent-one-welcome.png)

## Architecture

**Wrapped layers.** A model on its own is text in, text out — no hands, no memory, no check on
itself. A *agent* is that model inside a loop with tools. A *workspace* is everything around the loop.
Agent-One is one static Go binary at the outermost shell — the policy runs code:

<table>
<tr><th>layer</th><th>wraps</th><th>is</th></tr>
<tr><td><b>LLM</b></td><td>—</td><td>the bounded skill — text in, text out; no hands, no memory, no check on itself</td></tr>
<tr><td><b>agent</b></td><td>the LLM</td><td>the model inside a loop with tools — the model walks</td></tr>
<tr><td><b>workspace</b></td><td>the agent</td><td>loop + context/memory + permissions/sandbox — the code protects</td></tr>
<tr><td><b>Agent-One</b></td><td>the workspace</td><td>one static Go binary at the outermost shell — the policy runs code</td></tr>
</table>

**Six beats per step, one model call.** The model lives only in the plan-and-act beats; perceive,
recall, verify and record are code — no tokens, milliseconds. `policy.guts` scores each act at verify
and speaks once inside a slide; the witness demand runs at close when files were written.

<table>
<tr><th>beat</th><th>runs on</th><th>spend</th></tr>
<tr><td>1 · perceive — context read, budgets before any call</td><td>code</td><td>no tokens</td></tr>
<tr><td>2 · recall — anchors rebuilt per call, never cached</td><td>code</td><td>no tokens</td></tr>
<tr><td>3 · plan — <b>the one model call</b></td><td>model</td><td>tokens</td></tr>
<tr><td>4 · act — the tool calls the plan ordered</td><td>model directs, code runs</td><td>tokens (the plan's)</td></tr>
<tr><td>5 · verify — checks run; <code>policy.guts</code> scores the act</td><td>code</td><td>no tokens</td></tr>
<tr><td>6 · record — journal, memory, roster state</td><td>code</td><td>no tokens</td></tr>
</table>

**A task, end to end** — tools act inside the workspace; nothing closes on belief.

<ol>
<li>the task lands — a prompt, or a commission a subagent carries</li>
<li>perceive: context read, budgets checked before any call</li>
<li>recall: anchors + the workspace's policy rebuilt for this call</li>
<li>one model call — the plan
<ol type="a">
<li>each tool call it orders is classified first (read · write · outward · destructive)</li>
<li>the guard refuses catastrophic patterns before any ask</li>
<li>outward and destructive ride to the human gate; a denial comes back as a message the model reads</li>
<li>an approved act runs sandboxed — timeout, kill tree, fs under the root</li>
<li>the step lands: observe marks it, record journals it — and while tool calls remain, the loop turns again</li>
</ol></li>
<li>the model stops calling tools: the turn-end gate runs its checks — wrote files? a domain owner holds the gate</li>
<li>the wire rides up: status, findings, holes, unsaid — the context dies with the task</li>
</ol>

## What lands where

Three tiers keep the policy clean: rules teach, code enforces, instruments remember.

<table>
<tr><th>tier</th><th>duty</th><th>homes</th></tr>
<tr><td><b>rules teach</b></td><td>steer the model</td><td><code>AGENT-ONE.md</code> · <code>rules:</code> entries — injected into the system prompt, advice never policy</td></tr>
<tr><td><b>code enforces</b></td><td>protect the workspace</td><td>the gate · permissions · the guard · budgets · ownership ownership</td></tr>
<tr><td><b>instruments remember</b></td><td>make runs accountable</td><td>the wire formats (<code>@S</code>/<code>@F</code>/<code>@?</code>/<code>@U</code>) · the journal · memory anchors</td></tr>
</table>

**Terminology, once and for all.** A *prompt* is what you type — the ask. A *system prompt* is the
machine's standing instruction, auto-composed at every turn before the ask rides in. *Code* is
everything the workspace enforces without asking an agent to believe anything.

**The system prompt, composed per turn, contains:**

- identity + standing: who this agent is, the workspace it belongs to, its ranks and roles
- the policy: `AGENT-ONE.md` in full when `policy.principles` is on (default), plus every `rules:` entry
  verbatim
- the four action classes and who asks before a privileged one runs
- the wire it must answer in (`@S` / `@F` / `@U` / `@D` / `@E`)
- instrument readings it may claim: budgets read per step; the guts' announced behavior
- docs-as-code duty and the team map for this workspace

**In per-step context, on top of the ask:**

- memory anchors: notes matched to this ask (short / long / shared tiers, budgeted)
- the toolbox manifest — only the tools that fit this step's question
- working notes, journal tails, the last tool results (the step's own evidence)
- the witness demands when files were written and the agent's close is being checked

**Never in any prompt — coded and enforced:**

- the guard (dangerous commands refused before any approval) and the gate (who may approve)
- the sandbox knot (bwrap / `--containered` envelope)
- budgets read as instruments: context window % readings, stress-zone behavior, session wallets
- the closing witness's admissibility rule (machine-check or independent method)
- the guts' scoring, the echo penalty, and where every reading journals
- wire parsing, reports, sessions, undo, hooks — surface, not advice

- **`rules:` are converted to the system prompt.** Each `text:` line is policy text the binary reads
  into every turn's system message — shape culture here, never private muscle (omitted from a
  subagent's prompt unless `raw` is on; an entry can be a file path instead of inline text).
- **The ask, memory anchors, the toolbox manifest and witness evidence go in per-step context** —
  what this step needs, nothing it doesn't.
- **`policy:` is enforced in code, never prompted** — the guard and gate classes, the sandbox,
  budgets read as instruments, the closing witness demand, the guts' scoring. An agent can only
  tighten a policy, never loosen it; `status` names every switched-off policy aloud.

## The terminal

A terminal UI in the class of Claude Code: the conversation flows into your scrollback, and every
event of the loop is drawn — thinking, tool calls, diffs, subagents, the review gate.

![welcome](docs/screens/agent-one-welcome.png)

Each role has a robot in its colour beside the spinner; when a turn changed files, the domain owner
reviews the change before the turn is done:

![the review gate](docs/screens/agent-one-gating.png)

`ctrl+t` watches every agent — the orchestrator and each subagent, its thinking and its steps, live:

![every agent](docs/screens/agent-one-agents.png)

`/board` shows the workspace full screen — the agents, the ownership graph, the roles, the usage:

![the graph](docs/screens/agent-one-board-graph.png)

![tool cards](docs/screens/agent-one-tools.png)

| Key | |
|---|---|
| `enter` · `shift+enter` | send · newline |
| `/` · `ctrl+k` | commands · the palette |
| `ctrl+t` | every agent, live |
| `ctrl+o` | expand the last folded block |
| `esc` · `ctrl+c` | interrupt the turn · clear the input (↑ brings it back) |
| `pgup` · `pgdn` · wheel | scroll the chat |
| drag · `ctrl+y` · `/mouse` | select and copy · copy the last answer · the mouse to the terminal |
| `alt+s` · `alt+c` / `alt+p` / `alt+f` | hide the sidebar · fold its subagents / plan / files |
| `/settings` · `/roles` | theme, mouse and sidebar in one form · every role and its model |

## The CLI

| Command | What it does |
|---|---|
| `agent-one` / `repl` | the live session: keep talking while subagents work; `/agents`, `/send`, `/usage`, `/status`, `/compact` |
| `run "<task>"` | one task to completion, non-interactive; `--json` for automation |
| `resume <id>` · `sessions` | continue or list sessions |
| `status` | every live reading (models, sandbox, guard, memory) and every policy switched off, with where — gaps print as `@?` lines |
| `config show\|explain\|check\|path\|patch` | the effective configuration with the origin of every value |
| `memory` · `toolbox` · `onto` | memory tiers, the two-level tool registry, the ownership graph |
| `usage` | tokens and cost by agent, role, model and day |
| `bench` | a fixed task set on every configured model |
| `goal --validate "<cmd>" "<objective>"` | work turn after turn until the command passes; the binary runs it, the model cannot skip it |
| `review [range]` | two reviewers on two models in parallel, one merged shortlist; nothing fixed before you approve |
| `handoff [focus]` | a handoff note for a fresh session (`/handoff read` picks it up) |
| `install --claude [--global] [--yes]` · `hook <event>` | run the policy inside Claude Code through its hooks (§Inside Claude Code) |
| `guard check\|test\|show\|export\|hook\|install` | the global dangerous-command guard; `install --yes` wires it into Claude Code and OpenCode |
| `ui init\|scan\|check` | web pages built from one token system: lay the foundation, regenerate the legend, lint and screenshot at 360/768/1280 in both themes |
| `dash [--open]` | the session in the browser: the console, the neural net, metrics, relations; approvals answered there |
| `board` | the board and a read-only dashboard without a session, on 127.0.0.1 |
| `--containered` | the whole binary in a Docker container: the workspace read-write, the rest read-only |
| `init [--level light\|medium\|complex]` | found a workspace: the policy only; a team read from the tree with no model — the build files (go.mod, package.json workspaces, pyproject, Cargo, Maven, Gradle), sizes and imports give a zone worker per package or module dir, domain owners over related zone workers, a coordinator per three domain owners (lane coordinators and a design coordinator on a large project), a service owner for CI and infra, verify lines only where tests exist, existing docs kept; or a founding session that reads all the code and decides the coordinators, domain owners, zone workers, service owner, skills, agents and relations |
| `selftest` · `version` | |

## Configuration

Configuration is YAML (or JSON), layered — each layer overrides the one before it:

| Layer | Where | Kept in git |
|---|---|---|
| built-in defaults | in the binary — everything on | — |
| global | `~/.config/agent-one/` | no |
| project | `.agent-one/` in the workspace | yes |
| project local | `.agent-one/config.local.yaml` | no (machine-only) |
| environment | `AGENT_ONE_CONFIG=<file>`, `AGENT_ONE_CONFIG_CONTENT=<yaml>`, `AGENT_ONE_MODEL`, `AGENT_ONE_DISABLE_PROJECT_CONFIG=1` (skip the project layers), `AGENT_ONE_PROVIDER_<NAME>_API_KEY` (built-in provider names only; a custom provider uses `apiKeyEnv`) | — |
| flags | `--model`, `--set key=value`, `--approve outward`, `--dry-run`, `--no-<feature>` (all of them: `agent-one --help`) | — |

`agent-one config explain` shows every effective value and the file and line it came from;
`agent-one status` shows every model, the guard, the sandbox and anything switched off.

### Keys and registry credentials

Providers prefer `apiKeyEnv` (name the variable, never the value). Where an env var cannot exist —
headless containers — the designated escape is `providers.<name>.apiKey` holding the
literal; every other field still refuses credentials in a file (headers, urls, unknown keys).

`--containered`'s registry pulls authenticate the same way: env-first, literal fallback —
`registry.containers: { usernameEnv, tokenEnv }` or `{ username, token }`. Docker receives the
token on stdin, never argv.

### One file per part

A layer can be one `config.yaml`, or split: beside it, a file named after a section holds just that
section. The example in [`examples/gateway/`](examples/gateway/) — agent-one on vLLM-hosted models
behind a gateway — is laid out this way:

```
.agent-one/
├── config.yaml        # the workspace's own settings
├── providers.yaml     # model endpoints
├── registry.yaml      # models, tools, package and container registries
├── models.yaml        # who runs on what
├── guards.yaml        # catastrophic commands, refused before any approval
├── rules.yaml         # rules every agent reads, optionally checked at the gate
└── permissions.yaml   # ask / allow / deny per command
```

**`providers.yaml`** — where the models are. Keys are named, never written:

```yaml
gateway:
  enabled: true
  type: openai                       # any OpenAI-compatible endpoint: vLLM, a gateway, OpenRouter, Ollama
  baseURL: https://llm-gateway.example.corp/v1
  apiKeyEnv: GATEWAY_API_KEY         # the variable's name; export the key in your shell
  toolCalls: native                  # vLLM with --enable-auto-tool-choice; "text" for tags in the text
  contextWindow: auto                # read from /v1/models (max_model_len)
  timeout: 10m
  # headers: { X-Tenant: platform }        # a routing header; a credential here is refused
  # tls: { caFile: /etc/ssl/certs/corp-ca.pem }
```

`contextWindow: auto` reads the served maximum from `/v1/models`; a gateway that does not report it
needs `contextWindow: <n>` here or per model in `registry.yaml`. (A provider named `vllm` needs no
`type`: it is known.)

**`registry.yaml`** — where things come from: the models a gateway serves, the tools the agent can
discover, a private Artifactory, the container registry (the example ships `packages` and `containers`
commented out):

```yaml
models:                                     # "<provider>/<id>" — tier orders the roles; price in USD per 1M tokens
  gateway/muse-glimmer: { tier: 1, contextWindow: 131072, price: { input: 0, output: 0 } }
  gateway/glm-5-3:      { tier: 2 }
  gateway/kimi-k3:      { tier: 3 }
tools:                                      # external tools the toolbox indexes
  - { name: kubectl, description: "the cluster CLI", triggers: [kube, pods, deploy] }
packages:                                   # set in every shell command and in the container
  npm: https://artifactory.example.corp/artifactory/api/npm/npm-remote/
  pip: https://artifactory.example.corp/artifactory/api/pypi/pypi-remote/simple
  go:  https://artifactory.example.corp/artifactory/api/go/go-remote
  tokenEnv: ARTIFACTORY_TOKEN               # let through by name — the agent can read it: use a read-only token
  env: { GONOSUMDB: example.corp }
containers:                                 # for --containered
  base: artifactory.example.corp/docker-remote/debian:bookworm-slim
  apt:  https://artifactory.example.corp/artifactory/debian-remote
  # image: artifactory.example.corp/docker-local/agent-one-runtime:1   # pull a prebuilt runtime instead
```

For a private container registry, `docker login artifactory.example.corp` first: docker's own login
answers for the pull.

**`models.yaml`** — the session's model and the three roles (the analyst reads, the judge reviews, the
drafter writes). A call is retried on 429 and 5xx (honouring `Retry-After`); when it still fails, or
fails outright on a transport error, the fallback is tried — never on a refusal. A *role* is what a
subagent does for a call; a *rank* is its place in the tree (coordinator → domain owner → zone
worker); a *member* is one named agent — each can be given its own model:

```yaml
default: { model: gateway/kimi-k3, fallback: gateway/glm-5-3 }
roles:
  analyst: { model: gateway/muse-glimmer, fallback: gateway/glm-5-3 }
  judge:   { model: gateway/glm-5-3,      fallback: gateway/kimi-k3 }
  drafter: { model: gateway/kimi-k3,      fallback: gateway/glm-5-3 }
# ranks: { zone: …, domain: … }   members: { zone-auth: … }   tasks: { drain: …, gate: … }
```

**`guards.yaml`** — added to the built-in denylist (disk wipes, `rm -rf ~`, force-pushes, repository and
secret deletion, history purges, secret-store reads, `curl … | sh`). A match is refused before any
approval — no rule or `--approve` can run it:

```yaml
- '(^|[[:space:]])terraform[[:space:]]+destroy'
- 'kubectl[[:space:]]+delete[[:space:]]+(ns|namespace|node)'
- 'helm[[:space:]]+uninstall'
```

**`rules.yaml`** and **`permissions.yaml`**:

```yaml
# rules.yaml — every agent reads them; a check runs at the review gate
- { text: "Tests must pass before a change lands.", check: "make test" }
- { text: "No secret, key or token is ever written to a file." }
```

```yaml
# permissions.yaml — ask / allow / deny per command. An allow on bash, git or webfetch is a
# loosening that `config check` lists; a bare bash:* allow is refused. In the tracked project
# layer an allow never pre-approves an outward or destructive act (a clone would carry it):
# put those in .agent-one/config.local.yaml or the global config.
rules:
  - { match: "bash:git push*", action: ask }
  - { match: "bash:kubectl*", action: ask }
# and in .agent-one/config.local.yaml, the machine's own word:
#   permissions: { rules: [{ match: "bash:kubectl get*", action: allow }] }
```

### Every other section

| Section | What it controls | A taste |
|---|---|---|
| `tools` | the builtin tools, their limits, custom tools, the bash sandbox | `bash: { sandbox: bwrap, timeout: 2m }` · `custom: { kube-pods: { run: [kubectl, get, pods], class: read } }` |
| `policy` | the review gate and its checks, the human gate, budgets per turn | `gate: { retries: 1, testsIntact: true }` · `humanGate: { approve: [outward] }` |
| `memory` · `compaction` | recall and notes; draining the context when it fills | `compaction: { trigger: { fraction: 0.85 } }` |
| `mcp` | MCP servers (Claude Code's `.mcp.json` and OpenCode's are imported) | `servers: { gh: { command: [gh-mcp] } }` |
| `hooks` | shell hooks at preTool, postTool, sessionStart, preCompact, stop, userPrompt | `preTool: [{ match: "bash", command: "./check.sh" }]` |
| `discovery` | where instructions, skills, commands and agents are found (Claude Code and OpenCode dirs included) | `skills: { paths: [.claude/skills] }` |
| `budgets` | spend ceilings for the session and each subagent | `session: { tokens: 400000, usd: 5 }` |
| `ui` · `output` | the dashboard, the status line, output format | `board: { autostart: false }` |
| `sessions` · `undo` | where sessions and undo snapshots live, how long | `sessions: { keepDays: 30 }` |

| `toolbox` · `ontology` · `instruments` | the searchable tool registry, the ownership graph, the journals | `toolbox: { budgetTokens: 1500 }` |
| `mode` · `smallModel` · `logLevel` | `build` or `plan` (plan writes nothing); a small model for small tasks | `mode: plan` |
| `tools.profile` · `tools.missing` | the toolset shape (`max`, `anthropic`, `openai`, `minimal`); what happens when a tool is missing | `profile: minimal` |
| `permissions.import` · `mcp.import` | take Claude Code's and OpenCode's permissions and MCP servers | `import: { claudeCode: { enabled: false } }` |
| `guard` | more denylist files, patterns inline | `files: [~/corp-guards.txt]` |

Everything is on by default; switching something off is always shown in `status`, never silent.
`agent-one config show --yaml` prints every key with its effective value; `agent-one config explain`
adds the file and line each came from.

## Web UIs

When the agent builds a page, it holds the built-in **`ui` skill** (a project skill of the same name
overrides it). The app's own `ui/` dir is the source of truth; nothing is copied into the workspace:

```
ui/tokens.css      raw scales → meaning tokens (--surface --ink --accent --space-m --step-1 …), both themes, density
ui/palettes.css    thirteen palettes in OKLCH, contrast-checked; <html data-palette="forest"> swaps every colour
ui/layout.css      the page grid and primitives: .page .stack .cluster .grid .cols .sidebar .switcher .center .cover .frame
ui/components/<name>/<name>.{css,html}   one component; its css starts /* @component <name> — tokens: … */
```

A global or radical change is one edit in one tier (a palette, `--density`, `--type-ratio`, a
primitive); no component is touched. `agent-one ui scan` derives the **legend** —
`.agent-one/ui-assets/{manifest.json,legend.md,catalogue.html}`: every component, the classes it owns,
its variants, the tokens it reads. The agent reads it before it changes anything. `agent-one ui check`
lints the system (no literal colour or pixel length outside the token files, every class declared,
every token defined, every header true) and renders the catalogue with headless Chromium at 360, 768
and 1280px in light and dark, failing any component that scrolls sideways. The gate runs the lints on
every turn that touched the ui dir (`policy.gate.ui`). A project already on Tailwind, Bootstrap or a
design-system package keeps it: the skill follows the existing system.

## Safety, in code

Beyond the review gate, human approval and the sandbox described above:

- **The guard** — the catastrophic and irreversible are refused before any approval, held to a
  151-case test corpus; `agent-one guard install --yes` wires the same list into Claude Code and
  OpenCode (without `--yes` it only shows the change).
- **The review gate, hardened** — it runs the checks as they were before the turn, fails a turn that
  deleted or skipped tests, and sends a failure back to the model once, then to you.
- **Dry-run and previews** — `--dry-run` plans and shows what it would run or ask, running nothing;
  every edit shows its diff; `/board` and `ctrl+t` show every agent live.
- **The sandbox** (`bwrap`) or **the container** (`--containered`: the workspace read-write, the rest
  read-only, pulled or built from your private registry).

## Benchmarks

Benchmarks run under [Harbor](https://github.com/harbor-framework/harbor) in Docker — one container
per task, the agent inside it, the task's own tests deciding. Every launch is recorded in
[`bench/runs.jsonl`](bench/runs.jsonl); [`bench/RESULTS.md`](bench/RESULTS.md) is generated from it.
CI runs a no-cost smoke benchmark (`bench/smoke.sh`) on every commit against a fake
OpenAI-compatible server. A real run, agent-one against OpenCode on the same gateway models, is
`bench/harbor.sh` — see [`bench/HARBOR.md`](bench/HARBOR.md).

### Experimental loop features (off-switches, measured)

| feature | default | off with |
|---|---|---|
| `policy.guts` | on | `--set policy.guts.enabled=false` |
| `policy.closingCheck` | on under bench, off live | `--set policy.closingCheck.enabled=false` |
| echo penalty + witness admissibility | inside guts / closingCheck | ride their toggles |

Same 6 tasks × 2 attempts, the same model via an OpenAI-compatible gateway, same cells:

| run | loop state | score | input / out |
|---|---|---|---|
| `tb2-final-165052` | everything off | 8/12 (.67) | 3.1M / 0.25M |
| `tb2-fixed-220431` | policies + ceilings off | 7/12 (.58) | 2.4M / 0.18M |
| `tb2-clean-010744` | + witness close + guts | **10/12 (.83)** | 12.2M / 0.32M |
| `tb2-final2-024306` | + echo + admissibility | **10/12 (.83)** | **5.8M** / 0.25M |

On tokens: the winning harness grid spent 5.8M in / 0.25M out against the control's 6.1M in / 78k
out.

agent-one's current loop needs **23% less input per point of score** than the control's weakest
day — same tasks, same model. (For honesty: the control's best reading tonight was 0.92 — its loop
never changed; lifts came only from fixes around it. gcode-to-text sits at the model's glyph
precision edge for every harness: probe runs read the flag twice and miss a 0/O glyph; refusals
are honest, not confused.)

The culture that came from the bench is in [`examples/workspace-rules.yaml`](examples/workspace-rules.yaml)
— two policies earned, kept as principles, plus the close-time falsification witness (`policy.closingCheck`)
and the per-step value sense (`policy.guts`) they motivated.

## License

MIT — see [LICENSE](LICENSE). Role badges use [Tabler Icons](https://tabler.io/icons) (MIT);
the dashboard embeds Bootstrap 5 (MIT).
