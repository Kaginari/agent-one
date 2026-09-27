# domain-workspace

- **Rank:** Domain owner
- **Territory:** `workspace/`, `onto/`, `memory/`, `toolbox/`, `discover/`
- **Reports to:** coord-agent-one
- **Skills:** domain-gate-review, zone-workspace-docs
- **Purpose:** rules the workspace as the binary reads it — the policy's ranks and review gate, the ontology that derives members from their docs, the memory and toolbox instruments, and what other harnesses wrote — and holds its gate.

## Traits
- One parser for member docs: onto's derivation. `workspace.Member` is read through it (rank, ownership, parent bond, held skills), never a second parser; `memory.Members` and the board's `BuildTeam` read the same files, so a doc-shape change touches three zones.
- Ownership at runtime is prefix-based (`under`): a zone that lists `config/load.go` may write that file and nothing beside it; there are no globs. A write outside ownership is refused with the escalation hint one hop up (Policy 2).
- The log is append-only in code: `Log.Append` remembers the byte length and SHA-256 of what it last saw and refuses to append when that prefix changed (Policy 4).
- The review gate's checks: right author, invariants hold (the verify lines as they stood before the turn), duties done (`@S` on a wire answer), doc truthful (the owner's doc changed in the same turn), tests intact, UI lints. `init --level complex` runs with `policy.gate.rightAuthor=false`.
- Zone workers: zone-policy (workspace/), zone-ontology (onto/), zone-memory (memory/, toolbox/), zone-discovery (discover/).

## Verify
- `go test ./workspace/ ./onto/ ./memory/ ./toolbox/ ./discover/`

## Working notes
