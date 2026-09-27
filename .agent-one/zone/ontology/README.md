# zone-ontology

- **Rank:** Zone worker
- **Territory:** `onto/`
- **Reports to:** domain-workspace
- **Skills:** zone-go-package, zone-workspace-docs
- **Purpose:** the ground truth of the ownership graph: the Turtle parser, the triple store, the derivation of members from their docs, the reasoner and the shapes.

## Traits
- A hand-written Turtle lexer/parser/writer over an in-memory triple store with S/P/O indexes. Asserted triples come from `ontology/schema.ttl` and `ontology/graph/*.ttl`; derived triples come from the docs on disk and from `Infer` and are never written back.
- Derivation: a member is a directory `<workspace>/<rank dir>/<short>/` with README.md (else `<short>.md`, SKILL.md, the first .md); its id is `<dir>-<short>`. Fields are read leniently (`- **Key:** value`): Reports to / Parent → the bond (zone → domain is `truth`, domain → coord is `verdict`, → orchestrator is `reports`, else `above`); Territory / Owns / Ownership → `owns`, plus EVERY back-ticked token that contains `/` and no space, anywhere in the doc; the Skills field and any skill name appearing as a whole word → `holds`.
- Skills are read from .opencode/skills, .opencode/skill, .claude/skills; a skill whose name does not start with a rank dir is `shared` and exempt from SkillHeld.
- Shapes (schema.ttl): ZoneTruth (exactly one truth edge), ZoneOwnership (owns disjoint among zone workers; overlap is equality or a `/`-prefix), SkillHeld (at least one holder unless shared), BondTarget (an `above` target has a doc). `onto check` exits 0 on PASS, 1 on findings, 2 on a load error.
- `Infer` runs to a fixpoint deterministically (sorted triples): subClassOf/subPropertyOf closure, type propagation, domain/range typing, inverses, transitives. `Visible`/`Project` give a member the plain lines it may see within a token budget (Tokens = runes/4).
- CLI: check · show · project <member> [--budget N] · assert <agent> <kind> <text>; `CLILayout` is set by the binary from the lexicon before delegating.

## Verify
- `go test ./onto/`

## Working notes
