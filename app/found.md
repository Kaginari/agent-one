Found the team of this {{WORLD}} — you are hiring a team of experts for `{{PROJECT}}` ({{LANG}}).
The law is `{{WORLD}}/agent-one.md`; its sections "Skills & Agents" and "The workspace" are the rules for
what you create. Nothing outside `{{WORLD}}/`, `.claude/skills/` and `.claude/agents/` is written;
the project's code is read, never changed.

## 1. Read everything first

List every source file (the find/glob tool), then read them — all of them, not a sample: entry
points, packages, models, services, tests, build and CI files, the README. Note as you go:
- the domains: areas that change for their own reasons and need their own judgement (a gate);
- the vertical slices inside each domain: the smallest units someone owns the truth of;
- the conventions every slice repeats (how a new one is added), the contracts they share;
- the hazards: what can break production, leak a secret, open access, or cost money;
- how the project proves itself: the commands that build and test it, and whether they pass now.

## 2. Decide the team — by what the code needs, not by a quota

- **One coordinator** (`{{VOICE}}<name>`) — the shared skill and voice: owns the cross-domain rules and the
  project-wide files (README, the module/package manifest). A second coord only when the project is
  really two products with separate audiences.
- **Domain owners** (`{{GATE}}<domain>`) — one per domain that deserves its own gate. Two to five is usual; one
  for a small tool. Merge domains too thin to judge on their own; never make a domain owner per folder
  when the folders are layers of one feature.
- **Zone workers** (`{{TRUTH}}<zone>`) — one per vertical slice: the files that change together (e.g. a
  command, its service, its model and its tests). Each zone reports to exactly one domain (its truth
  link); its territory is those files, precisely.
- **A service owner** (`{{LEAD}}<subsystem>`) — only for a standing subsystem someone must own across
  sessions (CI and releases, deploy, infrastructure). Reports to orchestrator.
- **Skills** (skills) — know-how worth wearing: a contract or convention two or more members share
  (zone skills, worn by zone workers), a review checklist a domain owner applies (verdict skills), a pipeline or
  cross-domain practice (global skills). Three to six is usual. A skill is instructions, concrete and
  specific to this code — paths, names, commands — never generic advice.
- **Agents** (agents) — mint the coord, each domain (subagent) and each service owner (keeper, `mode: all`). Zone workers
  run as subagent when dispatched; mint one only if it will run often on its own.

Before writing, show the plan as a table: member · rank · reports to · territory · skills.

## 3. Write it, in exactly these shapes

A member doc, at `{{WORLD}}/<rank dir>/<short>/README.md` (dirs: coord `{{VOICE_DIR}}`, domain
`{{GATE_DIR}}`, zone `{{TRUTH_DIR}}`, service `{{LEAD_DIR}}`); its name is the prefix plus `<short>`:

```
# {{TRUTH}}<short>

- **Rank:** {{TRUTH_RANK}}
- **Territory:** `path/one/`, `path/file.go`
- **Reports to:** {{GATE}}<domain>
- **Skills:** <skill>, <skill>
- **Purpose:** the ground truth of … (one sentence, what it owns and why it matters)

## Traits
- durable facts about this territory a newcomer would get wrong (from the code, with names)

## Verify
- `<a command that proves this territory — run it first; keep it only if it passes now>`

## Working notes
```

A skill, at `.claude/skills/<name>/SKILL.md`: front matter `name` and `description` (one line: what
it knows and when to wear it), then the know-how as terse bullets, then an empty `## Working notes`.

A agent, at `.claude/agents/<member>.md`: front matter `name` (the member's name), `description`,
and `mode: all` for a keeper; the agent text says whom it is, which doc to read first, which skills to
wear, and what waits for the operator.

Rules that make the team true:
- Territories do not overlap between zone workers; a domain owner's territory is its zone workers' union (or omitted).
- Every verify line is run before it is written. A command that fails today is not a verify line:
  write the failure as a trait and as a finding in the log instead.
- Hazards found in reading go into the traits of the member that owns them.

## 4. Prove it

- Run `{{BIN}} onto check` and fix every finding it reports, until it passes.
- Append one entry to `{{WORLD}}/log.md`: what you founded, and the findings from the read.
- Answer with the table of the team and the findings, briefly.

A first sketch from the file tree alone (no reading, no judgement — a hint to improve on, not a plan):

{{SKETCH}}
