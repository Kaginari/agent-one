Found the team of this {{WORLD}} — you are hiring a team of experts for `{{PROJECT}}` ({{LANG}}).
The policy is `{{WORLD}}/AGENT-ONE.md`; its sections "Skills & Agents" and "The workspace" are the rules for
what you create. Nothing outside `{{WORLD}}/` and `{{WORLD}}/base-skills/` is written;
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
  command, its service, its model and its tests). Each zone worker reports to exactly one domain owner (its truth
  link); its ownership is those files, precisely.
- **Zone width policy** — size an ownership to what its persistent agent actually works: not so wide that every
  new file's creation lands on one owner (that persistent agent becomes the bottleneck and its edges go
  stale), not so narrow that one change always crosses zones. A single-prefix zone (one whole dir)
  can only be narrowed later by splitting the tree under it — prefer several precise claims
  (sub-dirs, or file sets for a flat dir) over one wide dir when the area is big.
- **A service owner** (`{{LEAD}}<subsystem>`) — only for a standing subsystem someone must own across
  sessions (CI and releases, deploy, infrastructure). Reports to orchestrator.
- **Skills** (skills) — know-how worth wearing: a contract or convention two or more members share
  (zone skills, worn by zone workers), a review checklist a domain owner applies (verdict skills), a pipeline or
  cross-domain practice (global skills). Three to six is usual. A skill is instructions, concrete and
  specific to this code — paths, names, commands — never generic advice.
- **Agents** — no extra card: each member doc you write IS the agent. Zone worker docs likewise —
  a zone worker runs as subagent when dispatched; nothing extra is registered for it.

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

A skill, at `{{WORLD}}/base-skills/<name>/SKILL.md`: front matter `name` and `description` (one line: what
it knows and when to hold it), then the know-how as terse bullets, then an empty `## Working notes`.

The agent needs no card of its own: the member doc IS the agent — `{{WORLD}}/<rank dir>/<member>/README.md`
carries the member's name, purpose and skills. Nothing is written outside the workspace; a persistent agent is the
member whose rank is Persistent agent (its doc says so), not a file with `mode: all`.

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
