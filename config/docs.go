package config

import "strings"

// What each config key does, in a line: the dashboard's config tree shows it beside the key,
// `config explain` may print it. A path's `*` stands for one segment of any name (a provider,
// a role, a rank, a server). An `enabled` switch says on/off first. The schema with its
// defaults is docs/canon/config.md; this is its short form.

var keyDocs = map[string]string{
	"$schema":    "the JSON schema URL, for editors only",
	"mode":       "build (the session writes) or plan (every write denied but the plans dir)",
	"smallModel": "the model for small chores (titles, the unsaid pass); empty = the default model",
	"logLevel":   "how much the binary logs: DEBUG, INFO, WARN, ERROR",
	"rankSet":    "extend (configured ranks join the built-in ones) or replace (only the configured ranks)",

	// models
	"models":                  "who runs on what: the session's model, then per role, rank, member and task",
	"models.default":          "the session's model — the human's choice, never routed",
	"models.default.model":    "provider/model-id of the session's model",
	"models.default.fallback": "the model tried when the main one keeps failing (never on a refusal)",
	"models.default.effort":   "the reasoning effort asked of the model (low, medium, high, max)",
	"models.roles":            "a model per role: analyst reads, judge gives verdicts, drafter drafts",
	"models.roles.*":          "this role's model",
	"models.roles.*.model":    "provider/model-id this role runs on",
	"models.roles.*.fallback": "the role's fallback model",
	"models.roles.*.effort":   "the reasoning effort asked for this role",
	"models.ranks":            "a model per rank (overrides the role for that rank)",
	"models.ranks.*":          "this rank's model",
	"models.members":          "a model for one named member",
	"models.members.*":        "this member's model",
	"models.tasks":            "a model per internal task: drain, gate, log, bench",
	"models.tasks.*":          "this task's model",

	// providers
	"providers":                             "where the models are served: one entry per endpoint",
	"providers.*":                           "one model endpoint",
	"providers.*.enabled":                   "on/off — whether this provider can be used",
	"providers.*.type":                      "the wire it speaks: anthropic, openai (any OpenAI-compatible endpoint) or mock",
	"providers.*.baseURL":                   "the endpoint's base URL",
	"providers.*.apiKeyEnv":                 "the name of the environment variable holding the key (the key itself never goes in a file)",
	"providers.*.apiKey":                    "a literal key — only for headless containers where no env var can exist",
	"providers.*.headers":                   "extra HTTP headers sent on every call (a credential here is refused)",
	"providers.*.headersEnv":                "headers whose values come from environment variables",
	"providers.*.timeout":                   "how long one call may take (\"10m\" or seconds)",
	"providers.*.maxOutputTokens":           "the most tokens one reply may produce",
	"providers.*.toolCalls":                 "native (the API's tool calls) or text (tool tags inside the text)",
	"providers.*.guidedDecoding":            "ask the server to constrain output to the tool schema",
	"providers.*.contextWindow":             "the model's window in tokens, or auto to read it from the endpoint",
	"providers.*.thinking":                  "anthropic: adaptive (thinking on every call) or off",
	"providers.*.fallbacks":                 "anthropic: default (the provider retries a refused turn itself) or off",
	"providers.*.script":                    "mock: the scripted replies file, for tests and dry runs",
	"providers.*.tls":                       "TLS options for this endpoint",
	"providers.*.tls.caFile":                "a CA bundle to trust (a corporate CA)",
	"providers.*.tls.insecureSkipVerify":    "on/off — skip certificate checks (never in production)",
	"providers.*.models":                    "the models this provider serves, with their tier, window and price",
	"providers.*.models.*":                  "one served model",
	"providers.*.models.*.id":               "the id the endpoint knows the model by, when it differs from the key",
	"providers.*.models.*.tier":             "orders the roles: a higher tier is a stronger, costlier model",
	"providers.*.models.*.contextWindow":    "this model's window in tokens",
	"providers.*.models.*.maxOutput":        "the most tokens one reply from this model may produce",
	"providers.*.models.*.reasoning":        "on/off — the model reasons before answering",
	"providers.*.models.*.price":            "USD per 1M tokens; a model with no price shows tokens, never a guessed cost",
	"providers.*.models.*.price.input":      "USD per 1M input tokens",
	"providers.*.models.*.price.output":     "USD per 1M output tokens",
	"providers.*.models.*.price.cacheRead":  "USD per 1M cached input tokens read",
	"providers.*.models.*.price.cacheWrite": "USD per 1M input tokens written to the cache",

	// tools
	"tools":                               "the builtin tools, their limits, custom tools and the bash sandbox",
	"tools.profile":                       "the toolset's shape: max, anthropic, openai or minimal",
	"tools.*":                             "one builtin tool",
	"tools.*.enabled":                     "on/off — whether the model is offered this tool",
	"tools.*.description":                 "replaces the tool's builtin description",
	"tools.*.class":                       "the act class (read, write, outward, destructive) — can only be raised",
	"tools.*.timeout":                     "how long one call may run",
	"tools.*.maxTimeout":                  "the longest timeout the model may ask for",
	"tools.*.maxLines":                    "the most lines one result returns",
	"tools.*.maxBytes":                    "the most bytes one result returns",
	"tools.*.limit":                       "the most entries one result returns",
	"tools.bash":                          "the shell tool",
	"tools.bash.shell":                    "the shell to run commands in; empty = the system's",
	"tools.bash.background":               "on/off — long commands may run as background jobs",
	"tools.bash.jobsDir":                  "where background jobs keep their output",
	"tools.bash.sandbox":                  "bwrap (Linux namespaces; Seatbelt on macOS) or none",
	"tools.bash.envAllow":                 "environment variables that survive the secret scrub",
	"tools.git":                           "the git tool, classified per subcommand like bash",
	"tools.webfetch":                      "fetch a URL",
	"tools.websearch":                     "search the web",
	"tools.websearch.backend":             "a search URL with {query} answering SearXNG-style JSON; empty = no backend",
	"tools.dispatch":                      "ordering work one rank down",
	"tools.dispatch.maxDepth":             "how many ranks deep an order may travel",
	"tools.dispatch.background":           "on/off — orders run in the background by default",
	"tools.dispatch.backgroundTimeout":    "the clock on a background order; 0 = 30m",
	"tools.dispatch.sessionBackground":    "on/off — the session's own orders run in the background, the session stays free",
	"tools.dispatch.strictFlow":           "on/off — one rank at a time, no skip-level orders",
	"tools.dispatch.lineage":              "on/off — an agent orders only the members that report to it",
	"tools.workingNotes":                  "the working-memory working notes (the todo list's seat)",
	"tools.workingNotes.limit":            "how many notes the working notes holds",
	"tools.custom":                        "your own tools: a command with parameters and a class",
	"tools.custom.*":                      "one custom tool",
	"tools.output":                        "how big tool results are handled",
	"tools.output.maxLines":               "results longer than this are cut and dumped to a file",
	"tools.output.maxBytes":               "results bigger than this are cut and dumped to a file",
	"tools.output.dumpDir":                "where cut results are dumped",
	"tools.output.keepDays":               "how long dumped results are kept",
	"tools.missing":                       "what happens when the model names a tool that does not exist",
	"tools.missing.doomLoopRepeats":       "how many repeats of a missing tool count as a loop",
	"tools.missing.proposeOnSecondNaming": "on/off — the second time a missing tool is named, propose making it",
	"tools.missing.liveReload":            "on/off — a newly added tool is offered without a restart",
	"tools.tmpRedirect":                   "scratch lives in the workspace, never at /tmp or the filesystem root",
	"tools.tmpRedirect.enabled":           "on/off — off-workspace scratch paths are redirected or refused",
	"tools.tmpRedirect.mode":              "rewrite (the path is moved into the workspace's tmp) or refuse",
	"tools.tmpRedirect.refuseCwd":         "on/off — also refuse a bash working directory outside the workspace",

	// guard
	"guard":          "the catastrophic-command denylist, refused before any approval",
	"guard.enabled":  "on/off — the denylist (built-in plus yours)",
	"guard.files":    "more denylist files, one pattern per line",
	"guard.patterns": "more denylist patterns, inline",

	// registry
	"registry":                        "where things come from: models, tools, package and container registries",
	"registry.models":                 "the models a gateway serves, joined to their provider",
	"registry.tools":                  "external tools the toolbox indexes",
	"registry.packages":               "a private package registry for npm, pip and go",
	"registry.packages.npm":           "the npm registry URL",
	"registry.packages.pip":           "the pip index URL",
	"registry.packages.go":            "the GOPROXY URL",
	"registry.packages.tokenEnv":      "the one secret let through to the shell by name (use a read-only token)",
	"registry.packages.env":           "extra variables set in every shell command",
	"registry.containers":             "the runtime image for --docker-agent",
	"registry.containers.image":       "a prebuilt runtime to pull instead of building one",
	"registry.containers.base":        "the base image the runtime is built from",
	"registry.containers.apt":         "the apt mirror the build uses",
	"registry.containers.username":    "the registry user (prefer usernameEnv)",
	"registry.containers.usernameEnv": "the variable holding the registry user",
	"registry.containers.token":       "the registry token (prefer tokenEnv)",
	"registry.containers.tokenEnv":    "the variable holding the registry token",

	// policy
	"policy":                                    "the policy as code: the gates, budgets, stress, value and evolution",
	"policy.principles":                         "the creed in every system prompt",
	"policy.principles.enabled":                 "on/off — off leaves a bare harness",
	"policy.humanGate":                          "the human's approval for outward and destructive acts",
	"policy.humanGate.enabled":                  "on/off — off only from the global or local file, and always listed as a loosening",
	"policy.humanGate.strict":                   "on/off — writes ask too",
	"policy.humanGate.approve":                  "classes pre-approved for good, e.g. [outward] — logged on every use",
	"policy.humanGate.dryRun":                   "on/off — nothing outward or destructive really runs",
	"policy.humanGate.trustContainer":           "on/off — inside its own container every class reads as approved (the guard still refuses)",
	"policy.gate":                               "the four checks at the end of a turn",
	"policy.gate.enabled":                       "on/off — the gate",
	"policy.gate.rightAuthor":                   "on/off — only a file's owner may write it",
	"policy.gate.invariantsHold":                "on/off — the owner's invariants still hold",
	"policy.gate.dutiesDone":                    "on/off — the owner's duties were done",
	"policy.gate.docTruthful":                   "on/off — the owner's doc still tells the truth",
	"policy.gate.retries":                       "how many times a failed gate goes back to the model per turn",
	"policy.gate.testsIntact":                   "on/off — a turn may not pass by deleting, skipping or narrowing tests",
	"policy.gate.ui":                            "on/off — a turn that touched the ui dir must pass the ui lints",
	"policy.gate.verifyTimeout":                 "the clock on each verify command; 0 = the engine's",
	"policy.docsAsCode":                         "the doc-truth check for workspaces without a gate rank",
	"policy.docsAsCode.enabled":                 "on/off — the doc-truth check",
	"policy.ownership":                          "an agent writes only inside its own ownership",
	"policy.ownership.enabled":                  "on/off — writes outside an agent's ownership are refused",
	"policy.ownership.orchestratorNeverAuthors": "on/off — the session itself never writes files; it orders them",
	"policy.wire":                               "the one-line reports that travel up the ranks",
	"policy.wire.enabled":                       "on/off — reports use the wire form",
	"policy.wire.cap":                           "the answer ceiling in bytes",
	"policy.wire.requireUnsaid":                 "on/off — a report must surface what was learned and not written down",
	"policy.wire.raw":                           "on/off — reports pass through raw",
	"policy.log":                                "the append-only log of verdicts",
	"policy.log.enabled":                        "on/off — off prints verdicts without recording them",
	"policy.log.path":                           "where the log is",
	"policy.escalation":                         "a failed verify retries, then rises one rank",
	"policy.escalation.enabled":                 "on/off — escalation",
	"policy.escalation.maxRetries":              "retries before it rises",
	"policy.budget":                             "the limits of one turn",
	"policy.budget.contextTokens":               "the context ceiling in tokens",
	"policy.budget.stressTokens":                "the context level that reads as stressed",
	"policy.budget.steps":                       "the most steps in one turn",
	"policy.budget.minutes":                     "the most minutes in one turn",
	"policy.budget.salvageTokens":               "tokens a subagent gets before its timeout to write down what it learned; negative = none",
	"policy.doomLoop":                           "stop an agent repeating the same failing act",
	"policy.doomLoop.enabled":                   "on/off — the loop detector",
	"policy.doomLoop.threshold":                 "repeats before it trips",
	"policy.stress":                             "the council: when an agent keeps failing, its dispatcher must answer",
	"policy.stress.escalateAt":                  "the strain at which a failing agent's report demands an answer (instruct, split or escalate); 0 = off",
	"policy.stress.maxConsults":                 "answers allowed before the run rises to the human",
	"policy.stress.relieveAt":                   "the strain at which the council relieves the agent: splits or retires its ownership; 0 = off",
	"policy.guts":                               "the gut feel of each step, read from test and tool output",
	"policy.guts.enabled":                       "on/off — the gut reading",
	"policy.guts.window":                        "how many recent acts the reading averages",
	"policy.guts.floor":                         "the reading at which a step reads as going wrong (≤ 0)",
	"policy.guts.benchThrashVisits":             "re-runs of one test bench that read as thrashing",
	"policy.guts.digestEvery":                   "every how many acts the gut is summarised",
	"policy.guts.ring":                          "how many benches the echo remembers",
	"policy.guts.benches":                       "test commands and their red/green words; empty = the built-in ones",
	"policy.guts.lexemes":                       "words that read as bad or good in output; empty = the built-in ones",
	"policy.guts.weights":                       "the weight of each signal; empty = the built-in ones",
	"policy.guts.clamp":                         "the range one act's reading is held to",
	"policy.value":                              "the one definition of success, scored per rank",
	"policy.value.enabled":                      "on/off — the value ledger and meters",
	"policy.value.window":                       "how far back the score looks (e.g. 28d)",
	"policy.value.pass":                         "outcomes that count as success",
	"policy.value.fail":                         "outcomes that count as failure",
	"policy.value.neutral":                      "outcomes that count as neither",
	"policy.value.reopenTurns":                  "a success reopened within this many turns counts as a failure",
	"policy.value.minRecord":                    "runs needed before an agent is ranked",
	"policy.value.families":                     "each family's score expression and meter",
	"policy.value.families.*":                   "one family's score and meter",
	"policy.forge":                              "skills and tools the agents propose",
	"policy.forge.demandAtSteps":                "a turn this long is asked for its @FORGE draft; 0 = off",
	"policy.forge.evolvedStatus":                "where a skill evolution registers lands: trial or active",
	"policy.evolution":                          "a stressed agent with an overflowing working notes reshapes itself",
	"policy.evolution.enabled":                  "on/off — evolution",
	"policy.evolution.mode":                     "auto (it happens) or propose (queued for `evolve apply`)",
	"policy.evolution.notesMax":                 "the working notes size past which an agent evolves; 0 = off",
	"policy.evolution.stressAt":                 "the strain that reads as stressed; 0 = policy.stress.escalateAt",
	"policy.evolution.failStreak":               "or this many failed gates in a row",
	"policy.evolution.cooldownRuns":             "runs between two evolutions of one agent",
	"policy.evolution.arms":                     "the ways it may evolve: split, mutate, rebirth",
	"policy.evolution.splitLines":               "an ownership this many lines reads as large enough to split; 0 = never",
	"policy.evolution.splitFiles":               "an ownership this many files reads as large enough to split; 0 = never",

	// memory
	"memory":                          "what the workspace remembers",
	"memory.short":                    "this session's working cache",
	"memory.short.enabled":            "on/off — the short-term cache",
	"memory.short.cacheDir":           "where it lives",
	"memory.long":                     "the indexed long-term memory",
	"memory.long.enabled":             "on/off — long-term memory",
	"memory.long.index":               "the index file",
	"memory.long.rebuildOnStale":      "on/off — a stale index is rebuilt when recalled",
	"memory.long.indexOnBoot":         "on/off — a stale index is rebuilt once at start",
	"memory.shared":                   "notes shared beyond one session",
	"memory.shared.workspace":         "notes every session in this workspace reads",
	"memory.shared.workspace.enabled": "on/off — workspace notes",
	"memory.shared.workspace.path":    "where they are",
	"memory.shared.machine":           "notes every workspace on this machine reads",
	"memory.shared.machine.enabled":   "on/off — machine notes",
	"memory.shared.machine.path":      "where they are",
	"memory.recall":                   "how memories are recalled",
	"memory.recall.k":                 "how many memories one recall returns",
	"memory.recall.relationBoost":     "on/off — related notes rank higher",

	// toolbox, ontology, instruments
	"toolbox":                        "the searchable registry of tools the model loads on demand",
	"toolbox.enabled":                "on/off — the toolbox",
	"toolbox.budgetTokens":           "the tokens the toolbox may spend in a prompt",
	"toolbox.registry":               "the registry file",
	"toolbox.extra":                  "extra tools, one JSON line each",
	"ontology":                       "the graph of who owns what",
	"ontology.enabled":               "on/off — the ontology",
	"ontology.schema":                "the schema file (Turtle)",
	"ontology.graphDir":              "where the graph is kept",
	"ontology.projectBudgetTokens":   "the tokens the graph may spend in a prompt",
	"ontology.validate":              "on/off — the graph is checked at load",
	"instruments":                    "the journals that record what happened",
	"instruments.context":            "context occupancy, from the provider's usage",
	"instruments.context.enabled":    "on/off — the context instrument",
	"instruments.loop":               "one journal per run of the loop",
	"instruments.loop.enabled":       "on/off — loop journals",
	"instruments.loop.journalDir":    "where they are written",
	"instruments.toolbox":            "the toolbox's journal",
	"instruments.toolbox.enabled":    "on/off — the toolbox journal",
	"instruments.toolbox.journalDir": "where it is written",
	"instruments.status":             "the status board",
	"instruments.status.showOff":     "on/off — list every switched-off feature (off is itself listed)",

	// permissions, rules, hooks
	"permissions":                           "ask / allow / deny per command or path",
	"permissions.enabled":                   "on/off — off leaves only the class defaults",
	"permissions.rules":                     "the rules: { match: \"tool:pattern\", action: allow | ask | deny }",
	"permissions.import":                    "take permissions from other harnesses",
	"permissions.import.claudeCode":         "Claude Code's .claude/settings.json",
	"permissions.import.claudeCode.enabled": "on/off — import them",
	"permissions.import.opencode":           "OpenCode's permission block",
	"permissions.import.opencode.enabled":   "on/off — import them",
	"rules":                                 "rules every member reads; a check runs at the gate",
	"hooks":                                 "shell hooks around tools, prompts and the session",
	"hooks.enabled":                         "on/off — every hook",
	"hooks.timeout":                         "how long one hook may run",
	"hooks.preTool":                         "run before a tool call; a non-zero exit refuses it",
	"hooks.postTool":                        "run after a tool call",
	"hooks.sessionStart":                    "run when a session starts",
	"hooks.preCompact":                      "run before the context is drained",
	"hooks.stop":                            "run when a turn ends",
	"hooks.userPrompt":                      "run before a human line becomes a turn; exit 2 refuses it",

	// mcp, discovery
	"mcp":                            "MCP servers whose tools the model can use",
	"mcp.enabled":                    "on/off — every MCP server",
	"mcp.timeout":                    "how long a server has to answer",
	"mcp.import":                     "take servers from other harnesses",
	"mcp.import.claudeCode":          "Claude Code's .mcp.json",
	"mcp.import.claudeCode.enabled":  "on/off — import it",
	"mcp.import.claudeCode.path":     "where it is",
	"mcp.import.opencode":            "OpenCode's servers",
	"mcp.import.opencode.enabled":    "on/off — import them",
	"mcp.servers":                    "your servers, by name",
	"mcp.servers.*":                  "one MCP server",
	"mcp.servers.*.enabled":          "on/off — this server",
	"mcp.servers.*.type":             "stdio (a local command) or http",
	"mcp.servers.*.command":          "the command that starts it",
	"mcp.servers.*.url":              "its URL (http)",
	"mcp.servers.*.inward":           "on/off — the server never leaves the machine (its calls may be read or write)",
	"mcp.servers.*.network":          "on/off — the server's process may use the network",
	"mcp.servers.*.tools":            "per-tool switches and classes",
	"discovery":                      "where instructions, skills, commands and agents are found",
	"discovery.instructions":         "instruction files read into every prompt",
	"discovery.instructions.enabled": "on/off — read them",
	"discovery.instructions.files":   "file names looked for in the workspace",
	"discovery.instructions.global":  "machine-wide instruction files",
	"discovery.instructions.walkUp":  "on/off — also look in parent directories",
	"discovery.skills":               "skills (Skills) the agents can hold",
	"discovery.skills.enabled":       "on/off — find them",
	"discovery.skills.paths":         "where to look (Claude Code and OpenCode dirs included)",
	"discovery.commands":             "slash commands",
	"discovery.commands.enabled":     "on/off — find them",
	"discovery.commands.paths":       "where to look",
	"discovery.agents":               "agent definitions, read as agents",
	"discovery.agents.enabled":       "on/off — find them",
	"discovery.agents.paths":         "where to look",

	// sessions, compaction, plan, undo, output, budgets, ui
	"sessions":                               "where conversations are kept",
	"sessions.enabled":                       "on/off — off persists nothing (--continue is an error)",
	"sessions.dir":                           "where they are kept",
	"sessions.keepDays":                      "how long they are kept",
	"sessions.title":                         "on/off — one small-model call names each session",
	"compaction":                             "draining the context when it fills, keeping what matters",
	"compaction.enabled":                     "on/off — the drain",
	"compaction.strategy":                    "drain (passes that keep pointers and findings) or summary (one written summary)",
	"compaction.trigger":                     "when it fires: the lower of the two",
	"compaction.trigger.tokens":              "fire at this many tokens",
	"compaction.trigger.fraction":            "fire at this fraction of the model's window",
	"compaction.keepRecentTurns":             "the newest turns always kept whole",
	"compaction.passes":                      "the drain's passes",
	"compaction.passes.pointerize":           "file reads and searches become path:range pointers",
	"compaction.passes.pointerize.enabled":   "on/off — this pass",
	"compaction.passes.trimSpent":            "finished tool outputs become their journal line",
	"compaction.passes.trimSpent.enabled":    "on/off — this pass",
	"compaction.passes.unsaid":               "one model call surfaces what was learned and not written",
	"compaction.passes.unsaid.enabled":       "on/off — this pass",
	"compaction.passes.workingNotes":         "the same call writes the working working notes",
	"compaction.passes.workingNotes.enabled": "on/off — this pass",
	"compaction.passes.episode":              "landed changes get their log entry now",
	"compaction.passes.episode.enabled":      "on/off — this pass",
	"compaction.passes.verify":               "the drain is checked (the ask kept, pointers resolve, tokens shrank) or undone",
	"compaction.passes.verify.enabled":       "on/off — this pass",
	"compaction.journal":                     "on/off — each drain is journaled",
	"plan":                                   "plan mode: every write denied but the plans dir",
	"plan.enabled":                           "on/off — plan mode is available",
	"plan.dir":                               "where plans are written",
	"undo":                                   "snapshots to undo the session's writes",
	"undo.enabled":                           "on/off — snapshots",
	"undo.dir":                               "where they are kept",
	"undo.keepDays":                          "how long they are kept",
	"output":                                 "how answers are printed",
	"output.format":                          "text, json or wire",
	"output.stream":                          "on/off — answers stream as they are written",
	"output.thinking":                        "on/off — show the model's thinking",
	"output.color":                           "auto, always or never",
	"budgets":                                "spend ceilings; 0 = none",
	"budgets.session":                        "the whole session's ceiling",
	"budgets.session.tokens":                 "tokens the session may spend",
	"budgets.session.usd":                    "USD the session may spend (priced models only)",
	"budgets.subagent":                       "each subagent's ceiling",
	"budgets.subagent.tokens":                "tokens one subagent may spend",
	"budgets.subagent.usd":                   "USD one subagent may spend",
	"ui":                                     "the terminal and the dashboard",
	"ui.theme":                               "the palette both faces hold: cyberpunk, neon-dystopia, corporate-rebellion, rain-city, synthwave, glitch, chromecore, realm",
	"ui.mode":                                "dark, light or system (follows the terminal and the browser)",
	"ui.statusLine":                          "on/off — the status line under the ask",
	"ui.announceSubagents":                   "on/off — say when a subagent starts and lands",
	"ui.board":                               "the dashboard",
	"ui.board.autostart":                     "on/off — start the dashboard with the session",
	"ui.board.port":                          "its port on 127.0.0.1",
	"ui.meters":                              "the session's meters: signal, label, scale, bands",
	"ui.meters.*":                            "one meter",

	// ranks
	"ranks":                   "the ranks of the tree: who reports to whom, what each may do",
	"ranks.*":                 "one rank",
	"ranks.*.reportsTo":       "the rank above it",
	"ranks.*.model":           "the model this rank runs on",
	"ranks.*.family":          "its value family: author, gate, planner, root, steward, auditor",
	"ranks.*.portrait":        "the picture the dashboard shows for it",
	"ranks.*.value":           "its own score, meter and record",
	"ranks.*.value.score":     "the score expression",
	"ranks.*.value.meter":     "the meter: signal and bands",
	"ranks.*.value.minRecord": "runs needed before it is ranked",

	// the registry's logins and policy
	"registry.packages.policy":        "all (packages from anywhere) or private-only (every manager kept on the URLs here; a command pointing one elsewhere is refused)",
	"registry.packages.auth":          "the login npm, pip and go present to the registry (env-first; a literal token only in config.local.yaml)",
	"registry.packages.npmAuth":       "npm's own login, over the shared one",
	"registry.packages.pipAuth":       "pip's and uv's own login, over the shared one",
	"registry.packages.goAuth":        "go's own login, over the shared one",
	"registry.packages.*.username":    "the user to log in as; empty = a bearer token (npm) or the user \"token\"",
	"registry.packages.*.usernameEnv": "the variable holding the user",
	"registry.packages.*.token":       "the token itself — the fallback where no variable can exist; never in the tracked config",
	"registry.packages.*.tokenEnv":    "the variable holding the token",

	// knobs every builtin carries, used by one tool
	"tools.*.backend":           "websearch's search URL; the other tools ignore it",
	"tools.*.background":        "bash: long commands may run in the background; dispatch: orders run in the background",
	"tools.*.backgroundTimeout": "dispatch's clock on a background order; the other tools ignore it",
	"tools.*.envAllow":          "bash's variables that survive the scrub; the other tools ignore it",
	"tools.*.jobsDir":           "bash's background job output; the other tools ignore it",
	"tools.*.lineage":           "dispatch: an agent orders only those that report to it; the other tools ignore it",
	"tools.*.maxDepth":          "dispatch's order depth; the other tools ignore it",
	"tools.*.sandbox":           "bash's sandbox: bwrap or none; the other tools ignore it",
	"tools.*.sessionBackground": "dispatch: the session's orders run in the background; the other tools ignore it",
	"tools.*.shell":             "bash's shell; the other tools ignore it",
	"tools.*.strictFlow":        "dispatch: one rank at a time; the other tools ignore it",

	"policy.guts.lexemes.negative": "words that read as going wrong (FAILED, Traceback …)",
	"policy.guts.lexemes.positive": "words that read as going right (PASSED …)",
	"policy.closingCheck":          "a headless run is asked once, at its close, whether the ask was done end to end",
	"policy.closingCheck.enabled":  "on/off — the closing check",
}

// Describe is what the key at a dotted path does, or "" when it has no line. An exact entry
// wins; then the most specific pattern, a `*` matching one segment of any name.
func Describe(path string) string {
	if d, ok := keyDocs[path]; ok {
		return d
	}
	segs := strings.Split(path, ".")
	best, bestPat, bestStars := "", "", len(segs)+1
	for pat, d := range keyDocs {
		ps := strings.Split(pat, ".")
		if len(ps) != len(segs) {
			continue
		}
		stars, ok := 0, true
		for i, p := range ps {
			if p == "*" {
				stars++
			} else if p != segs[i] {
				ok = false
				break
			}
		}
		// fewer stars is more specific; on a tie the pattern whose first star is later wins
		if ok && (stars < bestStars || (stars == bestStars && pat > bestPat)) {
			best, bestPat, bestStars = d, pat, stars
		}
	}
	return best
}

// named are the maps whose keys are names (a provider, a model id, a server): their keys read
// as `*` when a path is described — a model id with a dot in it stays one segment.
var named = map[string]bool{
	"providers": true, "providers.*.models": true, "registry.models": true, "models.roles": true,
	"models.ranks": true, "models.members": true, "models.tasks": true, "mcp.servers": true,
	"mcp.servers.*.tools": true, "tools.custom": true, "policy.value.families": true, "ui.meters": true,
	"ranks": true, "registry.packages.env": true, "mcp.servers.*.env": true, "providers.*.headers": true,
}

// Docs maps every path of a JSON-shaped config tree that has a line to it — what the
// dashboard sends beside the tree. Keys are the tree's own dotted paths; a key that is no
// config key (a Go field leaking through, capitalised) is passed over.
func Docs(tree any) map[string]string {
	out := map[string]string{}
	var walk func(v any, path, pat string)
	walk = func(v any, path, pat string) {
		if path != "" {
			if d := Describe(pat); d != "" {
				out[path] = d
			}
		}
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		for k, x := range m {
			if k == "" || (k[0] >= 'A' && k[0] <= 'Z') {
				continue
			}
			seg := k
			if named[pat] {
				seg = "*"
			}
			p, q := k, seg
			if path != "" {
				p, q = path+"."+k, pat+"."+seg
			}
			walk(x, p, q)
		}
	}
	walk(tree, "", "")
	return out
}
