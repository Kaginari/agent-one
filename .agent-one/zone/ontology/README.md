# zone-ontology

- **Rank:** Zone worker
- **Owns:** `onto/`
- **Reports to:** domain-workspace
- **Skills:** zone-go-package, zone-workspace-docs
- **Purpose:** the ground truth of the ownership graph: the Turtle parser, the triple store, the derivation of members from their docs, the reasoner and the shapes.

## Invariants
- One prefix, `ao:` = agent-one:. schema.ttl declares the classes (Member, Orchestrator, Coordinator, Domain owner, Zone worker, Service owner, Skill, Doc, Fact with Policy/Team/Ownership), the bonds (above transitive; truth, verdict, reports subPropertyOf above; holds/heldBy, owns, knows/knownBy, doc, shared) and four shapes: ZoneTruth (exactly one truth edge), ZoneOwnership (owns disjoint), SkillHeld (≥ 1 heldBy unless shared), BondTarget (every above-target has a doc). The workspace's copy of the schema must equal onto.DefaultSchema (TestSchemaOnDiskMatchesBuiltIn).
- Overlap (validate.go `overlap`): two members asserting the same FILE is a lawful split, not a finding; the same DIRECTORY claimed twice collides, and so does a strict ancestor (src/auth vs src/auth/login). Two claims of one member never collide with each other.
- derive() never writes: member id = `<rank dir>-<folder lowercased>`; the doc is README.md, else `<folder>.md`, else SKILL.md, else the first .md; fields are read leniently from `- **Key:** value` lines (reports to / domain owner / coordinator / parent; ownership / zone / owns; skills / skill / holds / hats), first wins, and a `## Ownership` heading's backticked bullets count too. The parent falls back to the first member of the expected rank the doc names, or orchestrator.
- Claims: the structured Ownership field is the authoritative claim list — a bare entry (a top-level dir, a file name) derives; a slashed entry derives only when rooted in the seat's own name (or `.agent-one` / `src`), otherwise it is a cross-reference and registers nothing. Prose backticks derive owns only when they self-name the member (selfNamesSpan) and are rooted in its seat or `.agent-one`; a foreign file path, bare .agent-one, anything under .agent-one/tmp, URI templates and REST routes never derive.
- A retired seat (a `- **Retired:** true` line) keeps its name, bonds and Skills in the graph but derives no owns — neither its Ownership field nor its prose — so the young of a split, a promotion or a relief never overlap it. It derives `ao:retired true`; its `Heir` line derives `ao:heir`, as does a promoted's `Promoted from` line (base → promoted). HeirOf walks heir edges, through heirs that retired in turn, to the first live member ("" when the line ends); Validate's HeirLive check (code, beside the shapes) finds a living member whose one hop up is a retired seat with no live heir. OneHop is the one no-third-member-between test, shared with workspace.
- Skills come from discover.SkillBases(WorkspaceDir) — crafted-skills, base-skills, then the host dirs, first home wins; a skill whose name prefix is no rank dir is `shared`. A member holds a skill its Skills field names or whose name appears as a whole word in its doc.
- Housekeep(root, heal, spawn): check-only reports and writes nothing; heal registers a missing member home, rewires a missing chain edge, and rewires a dead bond (a `Reports to:` naming no live seat) to the rank's policy parent — a live bond, policy-shaped or not, stays the operator's. spawn seats draft homes for tracked top-level dirs nobody covers. Overlaps are reported, never rewritten. app runs heal at every workspace-touching CLI start and at init, so a heal must stay idempotent.
- CLI: `onto check` prints `@S PASS|FAIL n findings · members · skills · facts · triples` (non-zero on findings), plus show, project, owner (CLIOwner, when the binary wires it, answers with workspace.ZoneOfPath — the gate's own reading; else ownerOf's graph mirror: longest claim, authoring rank on a tie) and assert. Project(member, budget) renders @ONTO by token budget (Tokens = runes/4, rounded up); Visible walks the bonds.
- cache.go: Stamp fingerprints what LoadLayout reads (ontology turtle, rank-dir docs, skill bases; path · size · mtime, dirs included); Cached memoizes LoadLayout on it — the TUI board and the web board's team poll Cached (≈0.5 ms vs ≈13 ms a load on this repo). The cached Workspace is shared and read-only; a writer (Record, the workspace) loads its own.

## Verify
- `go test ./onto/`

## Working notes
