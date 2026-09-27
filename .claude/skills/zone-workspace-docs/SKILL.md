---
name: zone-workspace-docs
description: The member-doc shape the ontology derives the team from — which fields become edges, why every back-ticked path is ownership, how skills are held, how verify lines are read — and the check that proves it. Wear it when writing or parsing docs under .agent-one/ or code that reads them.
---
- A member is a directory `.agent-one/<rank dir>/<short>/` with `README.md`; its id is `<rank dir>-<short>` (`coord-`, `domain-`, `zone-`, `service-`). The derivation is `onto/workspace.go` (`derive`); `workspace.Member`, `memory.Members` and the board's `BuildTeam` all read through it.
- Fields are `- **Key:** value` lines, read leniently. `Reports to` (also Parent, Domain owner, Coordinator) makes the bond: zone → domain is `truth`, domain → coord is `verdict`, → orchestrator is `reports`, anything else `above`. Without the field, the first name of the expected rank in the text is taken.
- Ownership is the `Territory` (also Owns, Ownership, Zone) field PLUS every back-ticked token in the whole doc that contains `/` and no space. So a trait that back-ticks `provider/x.go` in zone-loop's doc makes zone-loop own it and trips ZoneOwnership against zone-providers. Name a file outside your territory without back-ticks, or with the package word only.
- Ownership at runtime is prefix-based (`workspace.under`): no globs. Zones that share a directory list their files one by one.
- Skills are held by naming them in the `Skills` field or anywhere as a whole word; only names that start with a rank dir (`zone-…`, `domain-…`, `coord-…`, `service-…`) are member skills — any other name is `shared` (a host tool) and needs no holder.
- Verify lines: `- **Verify:** cmd` or the bullets under `## Verify`; they run with `bash -c` from the workspace root, 120 s each, as they stood BEFORE the turn. A line that fails today is written as a trait and a log finding, never as a verify line.
- Shapes checked by `agent-one onto check`: ZoneTruth (exactly one parent), ZoneOwnership (no overlap between zone workers — equality or a `/`-prefix), SkillHeld, BondTarget. Exit 0 PASS · 1 findings (`@F` lines with file:line) · 2 load error.
- The log is `### [RFC3339] author — title` then `- **Task/Files/Gate/Result/Learned:**` lines; the board's `ParseLog` and `Log.Append` both expect that head. Append only.

## Working notes
