# zone-mcp

- **Rank:** Zone worker
- **Territory:** `mcp/`
- **Reports to:** domain-safety
- **Skills:** zone-go-package, zone-act-classes
- **Purpose:** the ground truth of the Model Context Protocol client: how servers are spawned, how their tools are classed, how their death is seen.

## Traits
- JSON-RPC 2.0 over stdio or streamable HTTP (POST, JSON or SSE answers, session id header), stdlib only; `ProtocolVersion` is `2025-06-18`. Stdio servers spawn under the sandbox with the scrubbed environment; `ServerConfig.Env` is added after scrubbing, `EnvAllow` lets names through, `Network` lets the server reach the network.
- Registry names are `mcp__<server>__<tool>`. The class floor is Outward ("a mouth outside the workspace") unless the server is marked inward (Write) or config names a per-tool class; annotations only tighten — `destructiveHint` → Destructive, `openWorkspaceHint` → Outward; `readOnlyHint` lowers nothing.
- A server's death is detected and reported; its tools then return the fault instead of vanishing. Per-call timeout defaults to 60 s.
- Tests use `mcp/internal/testserver` in-process and build the Go server under `mcp/testdata/server`.

## Verify
- `go test ./mcp/`

## Working notes
