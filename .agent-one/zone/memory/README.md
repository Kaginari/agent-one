# zone-memory

- **Rank:** Zone worker
- **Territory:** `memory/`, `toolbox/`
- **Reports to:** domain-workspace
- **Skills:** zone-go-package, zone-workspace-docs
- **Purpose:** the ground truth of the two instruments ported from JS: the memory tiers and their BM25 recall, and the two-level toolbox registry.

## Traits
- Both are Go ports of JS instruments (memory.js, toolbox.js) and keep files, formats and ranking byte-compatible; `memory/js.go` reproduces JS semantics on purpose — whitespace classes, UTF-16 slicing, `toFixed` rounding, `localeCompare`, JSON key order. A "simplification" there breaks parity with the JS tool that runs against the same workspace.
- Tiers: `memory/short/` (drained context), `memory/long/index.json` (the BM25 index, `IndexV` 2, k1 1.2, b 0.75, stop words), `memory/shared/notes.jsonl` (the workspace's notes), and the machine tier ~/.agent-one/shared/notes.jsonl. Unsaid kinds: policy, team, domain. `WorkingNotesLimit` is 5 — the overload reading the policy names.
- Wire failures are `@S FAIL` plus one `@?` per hole, exit 2 (`memory.Fail`; toolbox reuses the type).
- toolbox: the registry is `toolbox/registry.json` (`RegV` 1); externals come from `toolbox/extra.jsonl` and config `registry.tools`; kinds skill · command · tool · agent · external; `BudgetDefault` 1500 tokens for the level-1 manifest (`@T` lines under `@TOOLS`); `TOK` is bytes/4, an estimate. Lanes by the holder's rank: zone → zone, domain → review, coord/service → global. Every load is journaled so `status` tells loaded from offered.
- Skill bases .opencode/skills, .opencode/skill, .claude/skills; command bases .claude/commands, .opencode/commands, .opencode/command; `Members` reads the rank dirs coord/domain/zone/service.

## Verify
- `go test ./memory/ ./toolbox/`

## Working notes
