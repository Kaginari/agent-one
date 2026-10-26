# zone-shelf

- **Rank:** Zone worker
- **Owns:** `app/shelf.go`, `app/shelf_test.go`, `app/workspacetools.go`, `app/workspace.go`, `app/workspace_test.go`, `app/permissions.go`, `app/permissions_test.go`, `app/sandbox.go`, `app/hooks.go`, `app/mcp.go`, `app/missing.go`, `app/discover.go`, `app/providers.go`, `app/providers_test.go`, `app/testdata/`
- **Reports to:** domain-app
- **Skills:** zone-go-package, zone-act-classes
- **Purpose:** the ground truth of how the policy reaches the tools: the shelf, the workspace tools, permissions, the sandbox and hook wiring, MCP connection, provider routing and fallback.

## Invariants
- Shelf order: bash, read, fold, ls, glob, grep, write, edit, multiedit, patch, git, webfetch, websearch, ask…; workspace tools dispatch · recall · remember · toolbox · onto · skill · workingNotes; `tools.profile` (max, anthropic, openai, minimal) cuts the shelf per provider shape; a disabled tool is a hole and `MissingPolicy` proposes the exact config path to enable it (`ProposeEnable`).
- `decideHook` applies `config.Decide` after classification; `tighter` orders Deny > Ask > Allow — a rule can make an act stricter than its class default, never looser; a matching `ask` forces the gate.
- `Providers.For` builds one client per model|effort and wraps a `fallback` when configured; `retryable` matches HTTP 429/500/502/503/504/529, `finish_reason error`, connection refused, EOF, timeout, no such host — never a refusal. `Window` asks the openai client's /v1/models when `contextWindow: auto`; the Meter and the drain read that window.
- `newSandbox` fills `sandbox.Options` from `tools.bash.sandbox`, the RW paths, env drop/allow, and `registry.packages` (EnvSet); `sandboxLine` is what `status` prints.
- `ShellHooks` run the `hooks:` commands (preTool, postTool, sessionStart, preCompact, stop, userPrompt) with JSON on stdin; a preTool hook's non-empty output refuses the step; a userPrompt hook's refuses the turn.
- `ConnectMCP` imports Claude Code's `.mcp.json` and OpenCode's servers when `mcp.import` allows, spawns stdio servers under the sandbox, and exposes prompts and resources; `ranksFor` builds the rank table from config over the lexicon; `foreignAgents` turns discovered agent files into rules.
- The working notes tool keys on the acting run: a subagent writes <workspace>/instruments/working-notes/<run>.md (tool.Env.RunID — the file the drainer and a resume read), the orchestrator the session's.
- **retry ladder** (`app/providers.go` retrying): Providers.For wraps every client, outside the fallback pair, in a ladder that re-asks the whole call after 5 s then 15 s (`ladder`, a var tests shorten) when `transient` names the fault — 429/5xx/529, a refused or reset connection, EOF, ErrStalled, `finish_reason error`, a stream ended before message_stop, a provider.StreamError whose Transient() says so (read by type, before any wording; retryable, the fallback's test, reads it the same way); never a 4xx refusal, a 404 (the fallback's), the client's own timeout (a net.Error Timeout or `timeout` text with no 5xx/429 status — the status list is read first, so a 504 or a 503 saying `timeout` is re-asked) or a cancelled context, and never past the context. It covers what provider.Retry's ~7 s per-send ladder cannot: a restart that outlasts it and a cut after the headers. The last error says `asked 3 times`; unwrap sees through it. The fallback pair joins both failures with %w so a typed fault survives it. A response body read cut by the client's own Timeout (`stream: context deadline exceeded (Client.Timeout …)`) stays terminal; only ErrStalled is a stall.
- The queue tool (tool.QueueTool over Workspace.Queue, as the acting agent) is a workspace tool on every shelf when tools.queue.enabled; switched off, Disabled names tools.queue.enabled with its origin.
- With tools.preferFileTools `hint` (default; `off` silent) and read on, bash is wrapped by withReadHint: a command tool.FileReadHint reads as a plain print of one file (cat, head, tail, `sed -n 'A,Bp'`) that succeeded, not in the background, gets one `[hint: read {"path":"f:A:N"} …]` line — never a refusal, never a class change.

## Verify
- `go test ./app/ -run 'TestShelfJunction|TestMinimalProfileAndOpenAIShape|TestDiscoverAndHooks|TestRulesHoldOnCommandForms|TestFallbackAndScript|TestRetryLadder|TestWindowAuto|TestFinishReasonErrorFallsBack|TestDispatchJunction|TestDrainJunction|TestReplaceRanks|TestAgentOneWorkspace|TestGuardPatternsFromConfig|TestRegistryToolsReachTheToolbox|TestQueueToolOnEveryShelf'`

## Working notes
