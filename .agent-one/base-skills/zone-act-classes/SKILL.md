---
name: zone-act-classes
description: The class ladder every act climbs (read · write · outward · destructive), the order of the four defences, where each lives, and the rule that nothing may loosen. Wear it when touching tool, shell, sandbox, guard, gate, mcp, or the app's shelf and permissions.
---
- `tool.Class` is an ordered int: Read < Write < Outward < Destructive (`tool/tool.go`). A tool's `Class` is its floor; `Classify` may raise it per call; the caller's `class` input may raise it; nothing lowers it — a lower declaration becomes a `@?` hole (`Tool.Settle`).
- Order of defence for a command, and the file that owns each step: 1. `guard.Match` (`guard/guard.go`; refused before anything, no approval possible) → 2. `Tool.Settle` with `Env.ClassifyCommand` / `ClassifyPath` / `ClassifyGit` (`tool/classify.go`, `tool/git.go`) → 3. `config.Decide` permission rules (`config/permissions.go`; Deny > Ask > Allow; `ask` sets `Request.Force`) → 4. `gate.Ask` (`gate/gate.go`; Outward and Destructive always ask, Write asks under Strict; only a TTY yes, `--approve <class>`, or the terminal's choice block approves).
- Records — `AGENT-ONE.md`, `log.md`, `design/*.md`, `notes.jsonl` — are Destructive to overwrite, by path (`ClassifyPath`) and by shell (`>`, `sed -i`, `tee`, `cp` onto one). Appending is a Write.
- Paths outside the workspace root are Outward, whether absolute, `~`, `$HOME` or a `../` climb (`Env.OutsideWorkspace`); a symlink that escapes the root is refused by `Env.Confine`.
- MCP tools floor at Outward unless the server is marked inward (Write) or config names a class; annotations only tighten (`mcp/adapter.go`).
- The sandbox is containment, not classification: `--unshare-net` unless the gate approved outward; the environment scrubbed of `*_API_KEY|_TOKEN|_SECRET|_PASSWORD|_PASSWD` before any child (`sandbox.ScrubEnv`).
- Adding a rule: a destructive or outward pattern goes into `tool/classify.go`; a catastrophic, irreversible one goes into `guard/patterns.txt` and gets a corpus line in `guard/corpus.txt` — the corpus is proven by both `go test ./guard/` and `go test ./tool/ -run TestGuardCorpusNeedsTheHuman`.
- A permission `allow` with a wildcard-only pattern on bash, git, webfetch, websearch or `*` is refused by config as an auto-approve mode; do not work around it.

## Working notes
