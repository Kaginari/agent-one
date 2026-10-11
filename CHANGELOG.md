# Changelog

All notable changes to Agent-One are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org).

## [Unreleased]

### Added

- **The policy inside Claude Code.** `agent-one install --claude` wires `agent-one hook` into Claude Code's
  hooks: the principles and the log's last entries at every session start (and after `/clear` and
  compaction), recall on every prompt, the guard and the human gate before every tool (it only
  tightens), and the gate when a turn ends — an owned path changed without its doc sends the turn
  back with the reasons, and the verdict lands in `log.md`. Claude Code runs the model on your plan;
  the hooks never call one and need no key.

- **Web UIs from one token system.** A built-in `ui` skill (method + design craft, offline), and
  `agent-one ui init|scan|check`: a foundation (tokens in three tiers, 13 OKLCH palettes checked for
  WCAG AA, layout primitives, cascade layers, two example components), a derived legend under
  `.agent-one/ui-assets/` (manifest, legend.md, a live catalogue), lints (tokens only, declared
  classes, defined tokens, true headers) and screenshots at 360/768/1280 in both themes with
  sideways-scroll detection. The gate runs the lints on a turn that touched the ui dir
  (`policy.gate.ui`, on by default). The container image ships Chromium.
- **The dashboard** at `/dash` on the board, and `agent-one dash` to run a session in the browser: a
  console like the CLI (streamed answers, thinking, tool and subagent cards with diffs, approvals),
  the team as a live neural net with knowledge cards, metrics, and the relations matrix with its
  dimensions — built on the ui system and held to `ui check`. Acts carry a per-run token and a
  loopback/same-origin check.

- **Three ways to found a workspace**: `init --level light|medium|complex` (a terminal asks). Medium
  sketches a team from the tree with no model; complex runs a founding session that reads all the
  code and hires the team — coordinators, domain owners, zone workers, a service owner, skills and
  agents, relations, verify lines that pass — until `onto check` is clean.

### Removed

- The SSH board (`board --ssh`): the board is local only — 127.0.0.1, or the host's loopback from
  `--containered`.

### Security

- **The gate runs no command the model wrote this turn.** A verify line added to a member doc in
  the same turn is a hole for the gate holder, not a reading; every verify line and injected check
  runs under the sandbox (no network, secrets scrubbed, its process group killed at the timeout).
- **A command is its name however it is called.** `/usr/bin/git push`, `/bin/rm -rf`, `\rm`,
  `xargs /bin/rm` and `sudo -u root env -i /bin/rm` classify and hit the guard like `git push`
  and `rm`; `>|` into a system path is outward.
- **An allow never carries a chained act.** `bash:npm test*` allows `npm test`, not
  `npm test && curl …`, `npm test & curl …` or `npm test $(curl …)`: each outward or destructive
  part needs its own allow. An allow in the tracked project config never pre-approves an outward
  or destructive act — put it in `config.local.yaml` or the global config.
- **`/dev/tcp` is the network, system directories are read-only.** A redirect or write into
  `/dev/tcp|udp`, `/etc`, `/usr`, `/opt`… is outward; `/library` is no longer read as `/lib`.
- **The tracked project config may not loosen the human gate.** `policy.humanGate.enabled: false` and
  `approve: [outward|destructive]` are refused from `.agent-one/config.yaml` (they travel with every
  clone) and honored from `config.local.yaml` or the global config; every standing approval is in
  the off-list.
- **More secrets scrubbed**: names holding `PRIVATE_KEY`, `ACCESS_KEY`, `CREDENTIAL` (so
  `AWS_SECRET_ACCESS_KEY`), and URLs carrying a password (`DATABASE_URL`).

### Changed

- **The policy reads the same on every host.** `AGENT-ONE.md` names instruments (the context reading,
  memory, toolbox, the loop) and maps them once to each host — the binary's commands, or the JS
  tools and `/don`·`/register` of a Claude Code or OpenCode workspace. Three contradictions resolved: the
  working notes lives in the held Skill, or the member's own doc when it wore none; the call to the
  configured model is Egress control's one standing outward act; Policy 2 names the system's
  Coordinator ⇄ Coordinator channel. The board's drawing rules moved to `design/binary.md`; memory tiers, the loop
  and the toolbox keep their policy here and point to their design for the spec.
- **`init` talks to a person in prose** on a terminal — what was founded and the next step (the
  model to set, the session to open); the wire lines stay for pipes and `--format`.
- **`init --level complex` checks the model first**: with none reachable it founds the light workspace,
  names the missing key and the rerun, and does not start a founding session it cannot finish.

### Fixed

- **`--help`, `-h` and `--version` answer at once**, outside a workspace too, and `<command> --help`
  shows that command; an unknown command is named unknown before any workspace lookup.
- **Hints name the binary's own commands** (`agent-one memory index`), not `memory.js`, in a workspace
  that does not ship the JS instruments.
- **The setup form no longer offers to keep a global config that does not exist.**
- **The README's Quick start sets the key before `init --level complex`**; Chromium is listed for
  `ui check`; `install.sh` no longer advertises piping into `sh`.
- **The design describes this repository**: its index, the toolchain (Go ≥ 1.26), one `cmd/`, and
  `init` as what creates a workspace.
- **Anthropic streams.** A tool with no arguments arrives as `{}`, not as a malformed call;
  redacted thinking keeps its data and replays as it came; a stream cut before `message_stop` is
  an error, never a partial answer taken for a whole one.
- **429 and 529 are retried on Anthropic too** — one retry policy for both clients (Retry-After,
  else 1s/2s/4s, three times, never past the context).
- **A malformed native tool call is named to the model** ("the arguments to read are not valid
  JSON") instead of running the tool on an input it misreads.
- **A turn that stops early keeps its writes** — the ones a read-classified command made too — in
  the transcript the resume and the gate read.
- **The write watch skips `node_modules`, `.venv` and `__pycache__`**: no more walking a
  dependency tree after every write.
- **A deadline is a deadline.** Gate readings, hooks, rule checks, `goal --validate` and custom
  tools kill their whole process group at the timeout and stop waiting on pipes a backgrounded
  child kept open.

## [0.2.0] - 2026-09-27

### Highlights

**A terminal in the class of Claude Code**, on Charm v2: the mascot drops in; the model's thinking
streams and folds; each rank has a face and its own verbs, and the domain owner reviews the change at the gate;
tool calls are class-coloured cards and diffs keep the code's colours; `ctrl+t` watches every agent,
live; `/board` full screen with the ownership graph; a `ctrl+k` palette and toasts; the
window title and tab progress follow the session; the transcript reprints on resize.

**Safety in code**: a global dangerous-command guard refused before any gate (151-case corpus;
`guard install` wires it into Claude Code and OpenCode); the gate runs the pre-turn check lines, keeps
the tests intact, and sends a failed gate back to the model once.

**New commands**: `goal`, `review`, `handoff`, `board --ssh`, `--containered`, a setup form at `init`;
config one file per section (`guards.yaml`, `rules.yaml`…).

**Fixes**: an upstream `finish_reason: "error"` is a failed (retryable) call, not an answer; a TUI that
cannot start says why; a piped slash command's turn can no longer be skipped by the next line.

## [0.1.3] - 2026-09-27

### Highlights

Works with Google Gemini's OpenAI-compatible API, free tier included: each tool call's `thought_signature` travels back on the next turn (multi-step tool use failed before), 429 and 5xx answers are retried with backoff (honouring `Retry-After`), and Gemini's array-form errors are shown instead of "unreadable body". The orchestrator answers you in plain language; only dispatched subagents answer on the wire. HTTP messages no longer say "agent" where they mean the request or response body.

## [0.1.2] - 2026-09-27

### Highlights

`run "<task>"` no longer reads stdin — an inherited pipe that never closes (CI, another agent, cron) could hang it; stdin is read only for `run` with no task or `run -`. Background jobs killed by a timeout no longer print a stray "Killed" line into the next command's output. The dashboard's off-list now names switched-off tools (it said "everything on" while a tool was off), long origins wrap inside their card, and `memory status` reads "working N note sets".

## [0.1.1] - 2026-09-27

### Highlights

A patch release for two defects found by running the released binary in a plain workspace.

### Fixed
- A machine-wide agent definition named `orchestrator` could be imported as a member, so the session would run with that member's role, model route and tools. The orchestrator's name is now reserved.
- The tool registry's load hint named a JavaScript tool that is not shipped; it now names `agent-one toolbox`.
- The dashboard's theme variables and the default model (`anthropic/claude-opus-5`) match the tested build.

## [0.1.0] - 2026-09-27

### Highlights

First release: a coding-agent harness that enforces its policies in code — the review gate with its verdict in `log.md`, human approval with per-command permission rules, a `bwrap`-sandboxed persistent shell, and native memory with context compaction by drain. Roles and subagent dispatch are native, the analyst, judge and drafter roles can each run their own model, and every feature is switchable in YAML or JSON with disabled policies always reported. Ships the dashboard (eight responsive pages), MCP (stdio and HTTP), compatibility with Claude Code and OpenCode instructions, skills, commands and agents, and a Harbor benchmark adapter.
