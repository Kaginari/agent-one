# zone-shelf

- **Rank:** Zone worker
- **Territory:** `app/shelf.go`, `app/shelf_test.go`, `app/workspacetools.go`, `app/workspace.go`, `app/workspace_test.go`, `app/permissions.go`, `app/permissions_test.go`, `app/sandbox.go`, `app/hooks.go`, `app/mcp.go`, `app/missing.go`, `app/discover.go`, `app/providers.go`, `app/providers_test.go`, `app/testdata/`
- **Reports to:** domain-app
- **Skills:** zone-go-package, zone-act-classes
- **Purpose:** the ground truth of how the policy reaches the tools: the shelf, the workspace tools, permissions, the sandbox and hook wiring, MCP connection, provider routing and fallback.

## Traits
- Shelf order: bash, read, ls, glob, grep, write, edit, multiedit, patch, git, webfetch, websearch, ask…; workspace tools dispatch · recall · remember · toolbox · onto · skill · workingNotes; `tools.profile` (max, anthropic, openai, minimal) cuts the shelf per provider shape; a disabled tool is a hole and `MissingPolicy` proposes the exact config path to enable it (`ProposeEnable`).
- `decideHook` applies `config.Decide` after classification; `tighter` orders Deny > Ask > Allow — a rule can make an act stricter than its class default, never looser; a matching `ask` forces the gate.
- `Providers.For` builds one client per model|effort and wraps a `fallback` when configured; `retryable` matches HTTP 429/500/502/503/504/529, `finish_reason error`, connection refused, EOF, timeout, no such host — never a refusal. `Window` asks the openai client's /v1/models when `contextWindow: auto`; the Meter and the drain read that window.
- `newSandbox` fills `sandbox.Options` from `tools.bash.sandbox`, the RW paths, env drop/allow, and `registry.packages` (EnvSet); `sandboxLine` is what `status` prints.
- `ShellHooks` run the `hooks:` commands (preTool, postTool, sessionStart, preCompact, stop, userPrompt) with JSON on stdin; a preTool hook's non-empty output refuses the step; a userPrompt hook's refuses the turn.
- `ConnectMCP` imports Claude Code's `.mcp.json` and OpenCode's servers when `mcp.import` allows, spawns stdio servers under the sandbox, and exposes prompts and resources; `ranksFor` builds the rank table from config over the lexicon; `foreignAgents` turns discovered agent files into rules.

## Verify
- `go test ./app/ -run 'TestShelfJunction|TestMinimalProfileAndOpenAIShape|TestDiscoverAndHooks|TestRulesHoldOnCommandForms|TestFallbackAndScript|TestWindowAuto|TestFinishReasonErrorFallsBack|TestDispatchJunction|TestDrainJunction|TestReplaceRanks|TestAgentOneWorkspace|TestGuardPatternsFromConfig|TestRegistryToolsReachTheToolbox'`

## Working notes
