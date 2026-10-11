# Changelog

All notable changes to Agent-One are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org).

## [Unreleased]

### Added

- **The 3D net is a brain.** The orchestrator sits at the centre and each rank is a shell further out
  (coordinators inner, domain owners middle, zone workers on the outer cortex); each lane is a region of the brain, tinted
  by a faint glow and switchable to its four weeks of work (heat) or its runs' pass rate (health).
  A seeded force layout on two hemispheres keeps a member near its parent and pulls related
  members together: the team's new `relates` (code coupling between ownerships from their
  imports — Go, Python, JS/TS — and skills both hold, in `board/relate.go`), days worked together and
  runs commissioned together. The skills are a crown above the orchestrator, their `holds` filaments
  falling in the skill's colour, glowing, with particles travelling down while the holder works.
  Every rank is a lit solid, one instanced draw per shape — a star crystal in orbit rings for the
  orchestrator, slender bipyramids for coordinators, hexagonal prisms for domain owners, soft wobbling blobs for zone workers, a
  tetrahedron for the service owner, a crown ring on a promoted rank, a torus for a skill — with a key in
  the lexicon's words. Synapses taper from the parent; a dispatch lights each synapse it crosses
  as a pulse with a trail and the report comes back, a firing neuron flashes and rings; dendrites,
  slow motes and a bloom pass give depth. Hovering a relates bond says why it holds, and "replay"
  scrubs through the last turn's signals. Still under reduced motion, and the idle frame budget
  holds.

- **The terminal UI has depth.** On a truecolor terminal the frame is layered planes drawn with
  colour and the block elements: the borders on the terminal's own ground; the conversation, the
  sidebar and the status bar each a plane of their own tint; the ask and each answer raised as
  slabs with a spine (magenta, the primary), a lit top face, a shaded edge and a soft drop shadow;
  tools and Subagents as smaller chips in their class's or rank's hue; the welcome as the hero slab on
  a primary → magenta gradient; the sidebar's sections as stacked tiles; the input sunk in a well;
  toasts, the ask card and the palette floating highest. One tonal scale stepped from the
  terminal's background, inverted on a light theme (white slabs on a grey ground, soft grey
  shadows); every text role holds 4.5:1 on every tint. A selection copies text, never the
  drawing. 256-colour and colourless terminals keep the flat frame.

- **The dashboard is the board's one page, at the root.** `http://127.0.0.1:<port>/` opens the
  cockpit; the tabs are Overview · Console · Neural net · Metrics · Subagent · Memory · Toolbox ·
  Relations · Tree · Config, and a deep link opens one (`/#memory`, `/#config`, `/#subagent/<run>`).
  Four tabs are new, in the realm HUD: **Subagent** — every journaled run, filtered by agent, outcome
  or words, and one run whole (the commission, each step with the gate's class and reason, the
  holes); **Memory** — the three tiers as instruments, a read-only recall, the working notes and notes, the
  memory's review on demand; **Toolbox** — the registry by kind and state with costs, offers and
  loads, the skills the session can hold, the registered agents, the loads journal; **Config** — the
  resolved config as a tree with each value's layer, credentials shown as set but never their
  value, and the off-list. Ten tabs fit any width: what does not fit folds into a "more" menu,
  arrows walk the tabs. New read-only APIs: `/api/runs`, `/api/run`, `/api/memory`,
  `/api/memory/recall`, `/api/memory/review`, `/api/toolbox`, `/api/config` (loopback Host only).

- **A native sandbox on macOS: Seatbelt.** On a Mac every shell command, verify line, hook,
  custom tool and stdio MCP server now runs under `/usr/bin/sandbox-exec` with a profile that
  mirrors bwrap's fence: writes only to the workspace, its read-write paths and the temp dirs; no
  network but loopback and unix sockets unless the act passed the gate as outward; the home's
  credentials (plus the keychains, cookies and browser profiles) unreadable; other processes not
  inspectable or signalable; Apple Events, LaunchServices and the clipboard closed. Paths cross as
  `-D` parameters, never as profile text. `status` prints `sandbox: seatbelt (sandbox-exec)`, or
  `sandbox: none — sandbox-exec refused: …` when a macOS refuses the profile;
  `tools.bash.sandbox: none` opts out. README's Security section has the table and the limits (no
  pid namespace; allow-default with mach IPC open); `go test ./sandbox/ -run Seatbelt -v` on a Mac
  proves it there.

- **The terminal UI is one frame.** A window of 60×26 or more is a single box-drawn grid under the
  headroom row: a header (the brand on its gradient, the workspace, the model, the session and its
  state), the conversation titled in its edge, the input inside the main column only (never under
  the sidebar), a full-height sidebar, and a status bar (the context meter, tokens, cost, live
  agents, key hints) — shared edges with real junctions, every row the window's width at every
  size. The sidebar is ruled sections (vitals beside the sprite, subagents with their state as a
  chip, the plan deck in its role's lane, the files as a tree) that fold on a click or
  `alt+c`/`alt+p`/`alt+f`; `alt+s` or `/sidebar` hides it. The welcome is a compact card — the
  sprite beside the name, the lane legend as chips, a label/value grid with the roles folded to
  one line (`/roles` has them whole) and a row of where to start. The human's ask holds a
  magenta bar, the answer a `✦` in the gutter. The slash menu, completions and the `?` sheet rise
  over the conversation instead of reflowing it; `/settings` is a huh form for the theme, the
  mouse and the sidebar. The palette follows the dashboard's neon HUD — cyan primary, magenta for
  the human, borders quieter than text, deep-ink twins on light terminals, every text role at
  4.5:1. Clicks and drags read the frame's own geometry.

- **The dashboard's overview is a cockpit.** The neural net is the hero, the console docks beside
  it (drag the splitter to resize, fold it to a rail — both remembered), and a timeline of Subagents
  and verdicts sits under the net (`/api/activity` now carries the newest runs, their wire headers
  dropped); a row focuses its neuron. The header gains the plan's progress. The folder tree leaves
  the grid for a drawer over every view — the files button, `t`, or Alt+T from the prompt; Esc or a
  click away closes it — and hovering a path lights the neuron that owns it. Every action pulses
  on its own neuron in 2D and 3D: reads ripple softly, writes pulse in the rank's colour, outward or
  destructive steps and a waiting gate ring amber, verdicts flare green or red; the orchestrator breathes
  while the session runs.

- **The dashboard holds "realm", a neon HUD.** A near-black ground, neon cyan and magenta,
  hairline panels with corner brackets, Orbitron display type and tracked capital labels over
  Rajdhani body text and JetBrains Mono numbers (all SIL OFL, vendored and served by the board), a
  faint slow scanline and a one-time glitch on the brand — still under reduced motion. Dark is the
  default; light is a real light mode in the same language; the theme switch cycles dark, light
  and system. One palette of named roles (ground, panel, border, text, muted, primary, accent,
  ok/warn/danger, a hue per rank), every text colour at 4.5:1 or better in both themes. Metrics
  gains a leaderboard of the members by their work. `ui check` lets a page's own `theme.css`
  hold literal values beside the foundation's token files.

- **The cockpit, reviewed.** The net fills exactly the box its panel gives it with the legend in
  view; each subtree under the orchestrator reads as a lane band, the service owner and a coordinator with no domain owners sit
  as satellites just under the orchestrator, every upper layer is centred, rows and tiles tighten before
  the zone workers wrap, and no label is cut below six ems; the skills become a strip of brains under the
  layers. A "now" strip over the net carries the session's state, its ask, the live agents with
  their context use and a wait on you. The timeline keeps its history (a journaled run folds into
  its live row only when it is the same run) and names the orchestrator by the lexicon. A plain turn's
  end pings calmly in the rank's colour — green and red are for real verdicts — and a burst of
  steps is one breathing neuron. The 3D stage stands as layers (spaced planes, rank captions,
  depth fog, smaller halos on the light theme), fills the panel, walks the tree from the keyboard,
  has a "reset view" and a first-use hint, and idles at 12 frames a second, its turn stopping
  after an idle minute. The tree drawer is a modal dialog; the phone header is two rows; sideways
  rows fade at the edge that has more. `/api/team` answers an empty workspace with lists, never
  null; `/api/activity`'s recent runs carry their id.

- **The neural net, rebuilt.** The dashboard's net draws the hierarchy as layers — the orchestrator, then
  each rank in the order work flows down, the skills beside — every neuron a rank-coloured tile
  whose 28 cells are its last four weeks of work (runs and tokens from the journals; blank where no
  instrument saw any), every tree bond weighted by the commissions it carried. Hovering or focusing
  a neuron lights its path to the orchestrator and its subtree; arrow keys walk the tree; the side card
  reads its doc, ownership, skills, bonds and work. A Subagent's dispatch travels down the tree as a
  spark, its report comes back up (green when DONE, amber or red otherwise), and a running agent
  breathes. A **3D** stage (hand-written WebGL2, no library) stands the same tree as glowing planes
  with particles drifting down the bonds; drag orbits, the wheel or a pinch zooms, a click focuses.
  It animates only while seen, honours reduced motion, follows the theme's tokens, and falls back
  to 2D without WebGL2. Narrow screens get the tree as an outline.

- **Select and copy in the TUI's panes.** A drag in the chat pane selects (clamped to the chat,
  scrolling at its edges; a double click takes a word, a triple a line) and the release copies
  the plain text — OSC 52, the primary selection, and off SSH the desktop's `wl-copy`/`xclip`/
  `xsel`/`pbcopy` — with a toast. `ctrl+y` and `/copy` copy the last answer's markdown; `/mouse`
  hands the mouse to the terminal (a `mouse: terminal` footer badge) and back.

- **The policy inside Claude Code.** `agent-one install --claude` wires `agent-one hook` into Claude Code's
  hooks: the principles and the log's last entries at every session start (and after `/clear` and
  compaction), recall on every prompt, the guard and the human gate before every tool (it only
  tightens), and the gate when a turn ends — an owned path changed without its doc sends the turn
  back with the reasons, and the verdict lands in `log.md`. Claude Code runs the model on your plan;
  the hooks never call one and need no key.

- **The TUI rail scrolls and folds, and the footer counts its tokens.** The side rail gains
  its own scroll offset — the wheel over the rail column walks it clamped to the content,
  with ▲▼ edge markers when more rides off-screen (f2ce295) — and the plan/subagents/files rail
  cards collapse to a one-line `+<glyph> <title> (<count>)` header via alt+p/c/f or a click
  on the header border, the session card exempt; all three bindings join the Shortcuts
  overlay (0fdfb9f). The footer reads `ctx ▮▮▮▯ 42% · 84k/200k`: FooterView gains
  `CtxTokens`/`CtxLimit`, the humanized context token counts gated at width ≥ 88 while the
  meter keeps its ≥ 72 gate, and zero values render byte-identical to before (cbb181d);
  `app/tui.go` hoists `readingOf` out of the footer closure and fills both fields from
  `r.Tokens`/`r.Limit` (4d08c4c).

- **The web answers gates in every run mode (webBus.answerable from provisioning).** The browser
  dashboard now registers a choice id for every gate ask whatever the front-end — dash-only,
  TUI with the board up, or the container — so approvals answer from the web everywhere.
  First answer wins across surfaces: whichever of the terminal or the browser answers first
  lands, and the loser gets the existing "answered elsewhere" refusal instead of a hang.

- **Open plans block a clean close (plan conformance, the closing check's second leg).** The loop
  gains `PlanOpen`, wired to the agent's deck: the closing check's re-ask names the open item, and an
  agent that closes anyway gets the hole on the report — `@? closed with an open plan: …`.

- **The closing check (`policy.closingCheck`).** When enabled, an agent's run that closes with no tool
  call is sent back once ("is the ask applied end-to-end?") before it may close — the bench-class
  failure it catches is a mid-plan close ("let me merge…" with the merge never run). Off by
  default; the bench config enables it (`bench/configs/gateway.yaml`).

- **The forge (`@FORGE` + `/forge`).** A run that proves a repeating need may carry its draft on
  the report's wire — `@FORGE tool:<name> | <summary> | <argv>` (argv holes `{{param}}` declare
  as string params) or `@FORGE skill:<name> | <text>`. Drafts ride up unminted and land as
  shared notes tagged `forge` on the record beat; the working notes lists them (`/forge`) and registers by
  index (`/forge register <n>`) — a skill lands as a workspace note, a tool lands as a `tools.custom`
  patch that reloads live. Registered drafts mark themselves (`forge-registered`); nothing registers twice,
  and an existing `tools.custom` name is never overwritten.

- **The container's trust (policy.humanGate.trustContainer).** Inside the harness's own runtime
  container — `<PREFIX>DOCKER_AGENT=1`, set only by the harness's own container argv, spoofable by
  no agent — every action class reads as pre-approved at the Gate: the sandbox is the membrane,
  so outward and destructive stop asking. The guard denylist still refuses first; the workspace dir
  mounts read-write inside the sandbox or out.

- **The orchestrator stays free (tools.dispatch.sessionBackground).** With `tools.dispatch.background`
  on, the session's own dispatches default to background — orchestrator keeps talking while Subagents
  ride on; each report wakes the session. A commission keeps its word: `background: false`
  still rides the foreground. One layer down, a Subagent's own dispatches stay foreground by design.

- **The agents pane pipes (ctrl+t → i).** An agent selected in the pane takes a typed line into
  its inbox — it lands at the agent's next turn boundary (the FIFO contract, never mid-turn) and
  a done agent refuses: its context died with its task. esc cancels a pipe in the making.

- **The council (policy.stress).** Governance strain is now counted and answered: the loop's
  turn-end surface carries `Strains` (failed acts standing + gate re-asks + wire re-asks), and
  the dispatcher keeps a strain ledger per agent. A Subagent that never lands — loop Fail or
  Checkpoint, or its own wire verdict reads FAIL/ESCALATE — accrues strain, and past
  `policy.stress.escalateAt` (default 5) its report carries a **council warrant** the dispatcher
  answers as its manager: `@INSTRUCT` folds context into the re-commissioned ask, `@SPLIT`
  narrows to one commission per slice, `@ESCALATE` rides the hole up — sideways counsel is
  named lawful in the warrant's text. The warrants an agent carries are bounded by
  `policy.stress.maxConsults` (default 3); past the bound the report hardens to `@S ESCALATE` and
  rises to the human. A clean landing clears the ledger. The agent's own wire verdict drives —
  a subagent that lands its run cleanly but says `@S FAIL` is counted as failing (the Policy-9
  reading, coded end-to-end now: the journal's status is lifecycle, the agent's @S is verdict).

- **The model resolver's pin policy.** A pin set in config now FORCES: `config.ResolveModel`
  resolves member → rank → role → task → default (was: role ahead of rank), so
  `models.ranks.<rank>` or `ranks.<rank>.model` outrules the role hat the commission
  named. Only an unpinned agent takes the lens of the ask — and the binary's own tasks
  (drain/gate/log/bench) sit below roles, routable per task. Doubly-locked by
  `TestModelsResolution`'s collision row: a pinned rank rides its pin even on a differently
  lensed commission.

- **Nested dispatch steps land their Subagents, coded.** A dispatch step's start seeds the
  Subagent card's ask and its end lands the card with the wire report at EVERY depth — a Subagent
  dispatched by a Subagent is no longer invisible to the card flow: the host sees the child's
  steps only as its parent's, so `tuiHost.observe` carries the seed/land handling at the top,
  before the session-only branch. The parent's own log lines ride along unchanged.

- **Subagent-card exit-verdict policy (Leg 2).** A subagent card's face now prints the run journal's
  exit verdict — `FAIL · 2m35s` under a failed journal — never the bare lifecycle word `done`.
  `LiveAgent.Exit` is populated at `Subagent.Finish` from the wire report's first `@S` token; the
  dispatch tool re-stamps `rep.Status` as the payload's first `@S` line so a renderer that
  reads only the returned string can never show a collapsed "done" over a FAILed journal. The
  lifecycle word moves to the ask-line tooltip. (`nil` derailed the 2026-09-28 session: Policy 9.)

### Security

- **The sandbox hides the host's processes and your credentials.** bwrap now gives every command its
  own pid, ipc and uts namespaces, and masks the home's credential paths (`~/.ssh`, `~/.gnupg`,
  `~/.aws`, `~/.kube`, `~/.docker`, `~/.netrc`, `~/.config/gcloud`, `~/.config/gh`, keyrings, browser
  profiles …) — they read as empty inside, while the workspace stays writable. README's new Security
  section says what the sandbox fences, what it does not, and how to fence a Mac.
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

- **`--containered` is now `--docker-agent`** — the flag that runs the whole binary inside a Docker
  container; the old name still works and says what it is called now. The marker inside the
  container is `<PREFIX>DOCKER_AGENT`.
- **Promotion happens.** The principal coordinator and the principal domain owner promote by default on a long, clean record —
  `reports >= 20 && passRate >= 90% && holeRate <= 10%` (`ranks.principal-coordinator.promotesWhen`,
  `ranks.principal-domain-owner.promotesWhen`; `""` turns it off). A promoted rank lives in its own dir and
  prefix (`.agent-one/principal-domain/<name>/`, `principal-domain-owner-<name>`; agent-one `principal-domain-`), so the
  register no longer collides with its base. The agent rises whole: its doc, ownership, invariants and
  verify are carried into the new seat, the base's seat retires, and the members that reported to
  it now report to its promoted, which commands them. Each promotion is appended to `log.md` with
  the metrics that earned it and announced on the dispatcher's report; a failed register is logged and
  reaches the dispatcher as `@?`. The tally is an append-only ledger
  (`.agent-one/instruments/promotion.jsonl`) replayed at start, so a record counts across sessions,
  and an agent's own `@S FAIL` is never counted as a pass.
- **Dispatch follows the line.** A domain owner orders only its own zone workers, a coordinator only its own domain owners —
  the members whose `Reports to` chain reaches it; a commission to another line's member is
  refused naming the parent that commissions it. `tools.dispatch.lineage: false` restores the
  rank-only policy.
- **The service owner holds no dispatch.** A rank no other rank reports to (and not sideways) is never
  handed the dispatch tool, whatever its tools list says.
- **A rank's job reaches its agent.** The `job` line rides the member card in the system prompt.
  `ranks.<r>.agent` is gone from the rank table — every agent runs as a Subagent — and a configured
  value is reported as an ignored key.

- **The policy reads the same on every host.** `AGENT-ONE.md` names instruments (the context reading,
  memory, toolbox, the loop) and maps them once to each host — the binary's commands, or the JS
  tools and `/don`·`/register` of a Claude Code or OpenCode workspace. Three contradictions resolved: the
  working notes lives in the held Skill, or the member's own doc when it wore none; the call to the
  configured model is Egress control's one standing outward act; Policy 2 names the system's
  Coordinator ⇄ Coordinator channel. The board's drawing rules moved to `design/binary.md`; memory tiers, the loop
  and the toolbox keep their policy here and point to their design for the spec.
- **`init --level complex` checks the model first**: with none reachable it founds the light workspace,
  names the missing key and the rerun, and does not start a founding session it cannot finish.

### Fixed

- **The domain owner's verify check reads the zone worker's doc fresh.** A verify line the domain owner wrote earlier in
  the same turn now counts when it dispatches the zone worker.

- **The dashboard's folder tree loads again.** It read the root level's entries under `nodes`
  while `/api/tree/children` answers `children`, so the tree always said "no tree source".

- **The neural net draws the orchestrator and its tree.** `/api/team` always carries the orchestrator node
  (the session agent, under the lexicon's word for it) and names every member's `parent`; a
  member with no bond up to an agent on disk hangs under the orchestrator by an `implied` reports edge
  and a note, so the tree is connected and the gap is said. The ontology reads the workspace's own
  `crafted-skills/` and `base-skills/` as skills, so a member's Skills field becomes `holds` bonds.
  `/api/activity` serves each agent's last 28 days (runs from the loop journals, tokens from the
  usage journal) — what the net's activity tiles and bond weights read.

- **The TUI's input and keys.** A capital `C` (or `c` on a pinned stale deck) no longer vanishes
  from a typed word: the plan snap moved to `alt+n`. The input box grows with a long line's wraps,
  not only its newlines. `ctrl+c` clears a draft first and interrupts a busy turn without arming
  the exit; `esc` and `ctrl+c` keep a cleared draft one ↑ away. Under the panes `ctrl+u`/`ctrl+d`
  are the input's again — `pgup`/`pgdn` (now matched), `shift+↑/↓` and the wheel scroll the chat —
  and the shortcuts list says so. The agents view's pipe gives the draft back.
- **The TUI stays smooth on a long session.** Each block renders once per width and theme
  (cached; a new block costs its own render, not the transcript's), the chat viewport is fed only
  when the transcript or the live rows changed, and the rail's plan decks load in a `tea.Cmd`,
  read again only when a deck file's size or mtime (or a live agent's role) moved — no disk
  read on the UI goroutine five times a second.
- **The TUI's transcript.** Quitting in pane mode prints the conversation to the scrollback, so it
  stays in the terminal after exit. A block that finished under the board prints once, not twice.
  `<` and `>` in an answer survive Glamour (`Vec<T>`, `providers.<name>.script`); long errors wrap.

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
- **The zombie Subagent card.** A Subagent that closed "done" without its wire report ever reaching
  the model (dispatched by a Subagent, a drained `@%`, an unparseable or failed report) kept its
  card in the live area forever, the clock recomputed on every frame — a domain owner card read
  `DONE · 15m0s` when its own budget clock had axed it. Coded at the seams, never prompted:
  `Render` stops recomputing a done Subagent's elapsed; the turn's end sweeps every
  done-but-unlanded card into exactly one tombstone; and a Subagent event closes the agent pane's
  state too, so no pane outlives its card reading "thinking".

- `TestREPLLive`, `workspace.TestDispatch`, and `tui.TestBlocksGolden` walk again — the goldens
  now carry the new face stamps; the dispatch asserts expect the stamped `@S DONE`.

- **Per-section local config.** The local layer mirrors one-file-per-section: beside (or
  instead of) `.agent-one/config.local.yaml`, a `<section>.local.yaml` (`.yml`/`.json`) holds one
  top-level section's value, loads after `config.local.yaml`, wins there, and keeps its own
  file for `config explain`; two local files for one section are a load error, mirroring the
  section-file rule. Machine-local by design: `*.local.yaml` belongs in the workspace's gitignore.
- **`agent-one config scaffold [--check]`** — the local-layer register: writes (never overwrites)
  `config.local.yaml` and one commented, default-valued `<section>.local.yaml` per section
  into the workspace dir, appends `*.local.yaml` to the workspace's `.gitignore`; `--check` writes
  nothing and answers on the wire what drifted: unknown local sections/keys as `@?`, defaults
  sections with no local file yet as `@F`. File names and keys come from the running binary's
  own defaults, so re-running the command syncs the scaffold when design moves.

- **The plan deck is native.** A new workspace tool, `plan`: every planner-level agent (orchestrator, coordinator,
  domain owner, service owner, promoted) keeps a todo deck under the plan dir — one line per slice with the rank or
  zone it belongs to (`who`), exactly one `in_progress`; `tick` moves a line, `read` reads another
  agent's deck, all machine-local under tmp. The coordinator's rank text now codes the chain: write the
  deck, map lines to owning domains, dispatch — domain owners in parallel (`background: true`) when the
  slices are independent; the domain owner's text codes the verify-first: the slice's checks are written
  before its zone worker is ordered.

### Removed

- **The board pages and Bootstrap.** The server-rendered pages (Overview, Subagent, Usage, Team,
  Memory, Toolbox, Log, Config), their templates, `static/` and the embedded Bootstrap are gone,
  and with them the dashboard's "board pages" link. `/dash` redirects to `/` (301); `/subagent`,
  `/memory`, `/toolbox`, `/config`, `/usage` and `/team` redirect to their tab; `/log` is gone
  (the log stays in `log.md` and the memory's episodic tier).

## [0.3.0] - 2026-09-28

### Highlights

**The workspace versions and defends its own seat.** A workspace root bearing `.agent-one/system.yaml`
outside any work-tree is versioned `git init -b main` once at open — a workspace is provisioned under git, by
the harness itself. And the gate now attributes the orchestrator's own seat files (the system manifest
and the policy doc) to Orchestrator: before, a gate-holder whose ownership reached `.agent-one/` silently owned
them through Owner's fallback, so Orchestrator's lawful relocation of the manifest failed right-author and
Docs-as-code — demanding a domain owner's README for the orchestrator's own file. The exemption mirrors the member
doc rule and polices non-orchestrator agents exactly as before.

### Added

- **Web UIs from one token system.** A built-in `ui` Skill (method + design craft, offline), and
  `agent-one ui init|scan|check`: a foundation (tokens in three tiers, 13 OKLCH palettes checked for
  WCAG AA, layout primitives, cascade layers, two example components), a derived legend under
  `.agent-one/ui-assets/` (manifest, legend.md, a live catalogue), lints (tokens only, declared
  classes, defined tokens, true headers) and screenshots at 360/768/1280 in both themes with
  sideways-scroll detection. The gate runs the lints on a turn that touched the ui dir
  (`policy.gate.ui`, on by default). The container image ships Chromium.
- **The dashboard** at `/dash` on the board, and `agent-one dash` to run a session in the browser: a
  console like the CLI (streamed answers, thinking, tool and Subagent cards with diffs, approvals),
  the workspace as a live neural net with knowledge cards, metrics, and the relations matrix with its
  systems — built on the ui system and held to `ui check`. Acts carry a per-run token and a
  loopback/same-origin check.

- **Three ways to found a workspace**: `init --level light|medium|complex` (a terminal asks). Medium
  sketches a team from the tree with no model; complex runs a founding session that reads all the
  code and hires the team — coordinators, domain owners, zone workers, a service owner, skills and agents, relations, verify lines
  that pass — until `onto check` is clean.
- **System self-init.** `agent-one/workspace/system.go`: `LoadSystem` parses
  `.agent-one/system.yaml`, and `GitSelfInit` versions a manifest-bearing root outside any
  work-tree (`git init -b main`, once, at open) — malformed manifests are a note, never a failed
  open; the harness never commits nor pushes.

### Fixed

- **Orchestrator-seat attribution at the gate.** The gate's right-author and Docs-as-code checks exempt the
  orchestrator's seat files (`.agent-one/system.yaml`, `.agent-one/AGENT-ONE.md`) when the orchestrator authors them —
  Owner's gate-holder fallback no longer lets a domain owner whose ownership is `.agent-one/` claim the orchestrator's
  own files. A non-orchestrator agent writing them still fails right-author.

### Removed

- The SSH board (`board --ssh`): the board is local only — 127.0.0.1, or the host's loopback from
  `--containered`.

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
