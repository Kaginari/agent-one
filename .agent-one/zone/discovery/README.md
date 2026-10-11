# zone-discovery

- **Rank:** Zone worker
- **Owns:** `discover/`
- **Reports to:** domain-workspace
- **Skills:** zone-go-package
- **Purpose:** the ground truth of what other harnesses wrote: instruction files, skills, commands, agents and MCP imports from Claude Code and OpenCode, read and never written.

## Invariants
- Options: Root, Home, and the five switches Instructions, Skills, Commands, Agents, MCP; Sources.Claude and Sources.OpenCode are *bool, nil meaning on. Nothing here writes; the only consumer is app (app/discover.go and the MCP import in app/mcp.go), which turns the structs into prompt text, toolbox entries, slash commands and foreign agents.
- Instructions: AGENTS.md and CLAUDE.md walked up from the root (nearest first, AGENTS.md before CLAUDE.md in one directory), then ~/.claude/CLAUDE.md and ~/.config/opencode/AGENTS.md; each carries its Scope and Path.
- Skills: level 1 only (name and description from front matter); Skill.Load() reads the agent on demand; Builtin(text) wraps a skill the binary ships. SkillBases(workspaceDir) is the one registry every reader shares, in precedence: <workspace>/crafted-skills, <workspace>/base-skills (Options.WorkspaceDir), .claude/skills, .opencode/skills, .opencode/skill, .agents/skills, then the home dirs (GlobalSkillBases); SkillIndex keeps the first home of a name and turns a second home into a hole naming both (a builtin yields silently).
- Commands: .claude/commands and .opencode/command(s) markdown; Command.Expand substitutes $ARGUMENTS and $1..$9.
- Agents: .claude/agents and .opencode/agent(s), walked recursively; front matter name (else the file name), description, model, mode (default "subagent"), tools; the first file to claim a name wins, sorted by name.
- MCP: .mcp.json (Claude Code) and opencode.json(c)'s mcp block, each Server tagged with its Source; a malformed file is returned as an error, never skipped silently.

## Verify
- `go test ./discover/`

## Working notes
