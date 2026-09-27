# zone-discovery

- **Rank:** Zone worker
- **Territory:** `discover/`
- **Reports to:** domain-workspace
- **Skills:** zone-go-package
- **Purpose:** the ground truth of what other harnesses wrote: instruction files, skills, commands, agents and MCP imports from Claude Code and OpenCode, read and never written.

## Traits
- Sources: instruction files (CLAUDE.md, AGENTS.md; nearest first), skills (front matter `name` and `description` at level 1, the body on load), slash commands (`$1`…`$9` arguments), agent definitions (front matter name/description/model/tools/mode; `mode` defaults to subagent; a file without a name is named after its file), MCP imports (`.mcp.json`, `opencode.json(c)` — comments and trailing commas tolerated). Each origin sits behind its own switch (`Sources.Claude`, `Sources.OpenCode`).
- `foreign.go` is the one file in the repository that names the convention this engine was forked from, only to keep that convention's machine-wide files (under ~/.claude and ~/.config/opencode) out of this workspace; the workspace's own files are never filtered. The leak test exempts `foreign.go` and `foreign_test.go` and nothing else — a new word of that convention anywhere else in this package fails `go test .`.
- Read-only: nothing in this package writes to disk.

## Verify
- `go test ./discover/`

## Working notes
