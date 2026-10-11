# domain-workspace

- **Rank:** Domain owner
- **Owns:** `workspace/`, `onto/`, `memory/`, `toolbox/`, `discover/`, `value/`, `rank/`
- **Reports to:** coord-agent-one
- **Skills:** domain-gate-review, zone-workspace-docs
- **Purpose:** rules the workspace as the binary reads it — the policy's ranks and review gate, the ontology that derives members from their docs, the memory and toolbox instruments, and what other harnesses wrote — and holds its gate.

## Invariants
- One parser for member docs: onto's derivation. `workspace.Member` is read through it (rank, ownership, parent bond, held skills), never a second parser; `memory.Members` and the board's `BuildTeam` read the same files, so a doc-shape change touches three zones.
- Ownership at runtime is prefix-based (`under`): a zone that lists `config/load.go` may write that file and nothing beside it; there are no globs. A write outside ownership is refused with the escalation hint one hop up (Policy 2).
- The log is append-only in code: `Log.Append` remembers the byte length and SHA-256 of what it last saw and refuses to append when that prefix changed (Policy 4).
- The review gate's checks: right author, invariants hold (the verify lines as they stood before the turn), duties done (`@S` on a wire answer), doc truthful (the owner's doc changed in the same turn), tests intact, UI lints. `init --level complex` runs with `policy.gate.rightAuthor=false`.
- The value law lives beside the workspace: `rank/` is a rank name's one spelling (Canon) and its family; `value/` the signals, scores, meters and the append-only value ledger the promotion fold, the council and the boards read.
- Zone workers: zone-policy (workspace/, value/, rank/), zone-ontology (onto/), zone-memory (memory/, toolbox/), zone-discovery (discover/).

## Verify
- `go test ./workspace/ ./onto/ ./memory/ ./toolbox/ ./discover/`

## Working notes
