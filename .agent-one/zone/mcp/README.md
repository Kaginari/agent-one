# zone-mcp

- **Rank:** Zone worker
- **Owns:** `mcp/`
- **Reports to:** domain-safety
- **Skills:** zone-go-package, zone-act-classes
- **Purpose:** the ground truth of the Model Context Protocol client: how servers are spawned, how their tools are classed, how their death is seen.

## Invariants
- JSON-RPC 2.0 over the standard library; ProtocolVersion 2025-06-18; Connect picks stdio when a Command is set (spawned under the sandbox with a scrubbed environment) else http (POST, JSON or SSE answers, session id header); then initialize → notifications/initialized; default Timeout 60 s per call.
- A server's death is detected (setFault, ErrDead) and reported: Alive() false, and its tools then return the fault instead of vanishing from the shelf.
- adapter.go: ToolName is mcp__<server>__<tool>; Classify's floor is Outward ("an endpoint outside the workspace") unless ServerOptions.Inward, a per-tool class from config replaces the floor, and destructiveHint / openWorkspaceHint only tighten. Adapt wraps a ToolInfo as a tool.Tool whose Run calls the server.
- Server definitions come from config (mcp.servers, checkMCP: credentials only through headersEnv / envAllow) and from discover's imports; app/mcp.go is the one caller that connects them and puts the adapted tools on the shelf.
- Test fixtures: internal/testserver (an in-process server) and testdata/server (a binary). The app package keeps its own fixture at app/testdata/mcpserver for its junction tests.

## Verify
- `go test ./mcp/`

## Working notes
