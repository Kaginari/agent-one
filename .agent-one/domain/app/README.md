# domain-app

- **Rank:** Domain owner
- **Territory:** `app/`, `cmd/`
- **Reports to:** coord-agent-one
- **Skills:** domain-gate-review
- **Purpose:** rules the binary's wiring — the CLI, the live session and its hosts, the tool shelf and the policy hooks — and holds its gate; app is the one package that imports every other.

## Traits
- `app.Main(dist, args, io, version)` is the whole entry; `cmd/agent-one/main.go` is one call plus the GoReleaser ldflags (`main.version`, `main.commit`, `main.date`). Subcommands are the `commands` table in cli.go; `--containered` re-executes the binary inside Docker before anything else runs.
- `app/policy/AGENT-ONE.md` (the embedded policy) belongs to coord-agent-one, not to this domain: it must stay byte-equal to the root copy.
- `TestE2E*` in e2e_test.go build the binary and a Go MCP server from `app/testdata/mcpserver` — the slowest tests in the module; run a named subset while iterating.
- `init --level complex` sets `policy.gate.rightAuthor=false` for the founding session and hands the model `app/found.md` with a sketch from the tree.
- Zone workers: zone-cli (entry, commands, init, container, selftest), zone-session (REPL, sessions, subagents, hosts, usage), zone-shelf (tools, permissions, sandbox, hooks, MCP, providers wiring).

## Verify
- `go test ./app/ ./cmd/...`

## Working notes
