# zone-memory

- **Rank:** Zone worker
- **Owns:** `memory/`, `toolbox/`
- **Reports to:** domain-workspace
- **Skills:** zone-go-package, zone-workspace-docs
- **Purpose:** the ground truth of the two instruments ported from JS: the memory tiers and their BM25 recall, and the two-level toolbox registry.

## Invariants
- IndexV 2, WorkingNotesLimit 5, PipeBuf 4096. Files under the workspace dir: memory/long/index.json (derived, rebuildable), memory/short/<member>.jsonl (recall cache, disposable), memory/shared/notes.jsonl (workspace-shared, append-only, tracked) and the machine tier ~/.agent-one/shared/notes.jsonl. WriteAtomic and AppendLine are the only writers.
- NotesOf is the one working notes reading: a dated bullet or ### line under `## Working notes` starts an entry, its following lines ride it, each with byte offsets (Start, End) so a writer cuts entries and leaves the rest; WorkingNotesSpan is the working notes agent; CountNotes and Working notes read it.
- Sources indexed (sources()): log.md (episodic); Skills from SkillBases(dir) — discover.SkillBases, the one registry: crafted-skills and base-skills under the workspace dir, then .claude/skills, .opencode/skills, .opencode/skill, .agents/skills, commands from CommandBases and the workspace's tools dir (procedural); each member's `notes/*.md` (procedural); the policy, member docs of Ranks coordinator, domain owner, zone worker, service owner, design and the root README (semantic).
- Ranking is local and model-free: BM25 with the stop list and Tokens tokenizer, cosine over IDF-weighted vectors, recency, then RelationBoost — kin first: the asker's own member +0.25 (else same rank +0.10), its name +0.15, parent and a child +0.10, a held skill or its zone +0.08, its own shared note +0.05. An agent's own doc and notes must outrank the chronicle (TestRecallKinFirst). RecallOpts.Window (the workspace's recall beat and the recall tool; never the CLI, MCP or board, which keep the JS ranking) shapes the K seats in rank order with at most windowSeats (2) per source (seatK), and a cross-lane twin of a seated memory (twins: different sources, one of them a shared note, ≥ 4/5 of the shorter's content words — a note's from its snippet, never its author or tag — in the longer) gives its seat up; its cache key carries `/window`.
- Status reports silent tiers as holes (no index, HEAD moved since the index was built, no transcript); a stale index is a `@?`, and the workspace's Recall hook rebuilds it unless memory.long.rebuildOnStale is off.
- js.go reproduces JavaScript semantics on purpose (UTF16Len/UTF16Slice, JSTrim, LocaleCompare, ToFixed via big.Rat, JSNum) so status and recall output equal memory.js's byte for byte; a "simplification" here is a divergence. The interop tests skip when node or the JS tool is absent.
- review.go: `memory review` reads four tiers through their own lens — short (stems asked across agents), shared (lessons restated), long (staleness, procedural thinness, recurring friction), value (today's journals, gate fails) — and routes each finding to [provisioning], [skill], [policy] or [harness]. Review never writes.
- scrub.go: `memory clean` runs the cure deck (ScrubRules) over log.md, both notes files and the short caches. Dry-run touches nothing; `--apply` backs each file up byte-exact under the workspace's tmp/scrub, rewrites atomically, and reseeds the long index. Records (log, notes) normalise spans in place and never lose lines; only a cache line whose query is left with no content tokens drops.
- remember --kind policy|team|domain are the unsaid kinds (Unsaid, IsUnsaid); Forget clears the short tier for a member. RememberOpts.Extra rides extra fields after text (the forge's run and hash). CLI verbs: status, index, recall, remember, forget, clean, review, selftest; ParseArgs is the JS argv parser.
- RegV 1, BudgetDefault 1500 tokens, TOK(s) = ceil(len/4); Kinds are skill, command, tool, agent, external; laneOfRank maps a skill's name prefix to its lane (zone worker → zone, domain owner → verdict, coordinator and service owner → global) — a skill with no rank prefix is listed as a shared host tool.
- Sources are memory's lists, never a second copy: Skills from memory.SkillBases(dir) (discover.SkillBases: the workspace's crafted-skills and base-skills, then .claude/skills, .opencode/skills, .opencode/skill, .agents/skills), commands from memory.CommandBases, Agents from .claude/agents and .opencode/agent(s), externals from extra.jsonl plus config's registry.tools (Workspace.Extra).
- Frontmatter() reads the YAML-ish front matter (name, description, triggers as a list or string); triggers are weighted frontmatter .8, quoted .8, name .5, sentence .4 (triggerW) when picking for an ask; FitWords 2.
- Files under the workspace dir: toolbox/registry.json (derived, rebuildable), toolbox/extra.jsonl (hand-kept externals), instruments/toolbox/loads.jsonl (the loads journal). LoadRegistry reports Live or stale; IsStale triggers a rebuild in the workspace's Recall hook.
- Brief(q, PickOpts{As, Budget}) returns level-1 `@T` lines under the budget with holes ("registry older", "no registry", "registry empty" name the workspace's state, not faults — the empty-registry hole names the workspace dir); Load(name, LoadOpts) returns an agent whole or by section anchor and journals it; Status reports loads against offers; Explain shows one entry's pricing.
- CLI: index, brief (alias pick), load, status, explain, selftest — the same words and output as toolbox.js; the interop test skips when node or the JS tool is absent.

## Verify
- `go test ./memory/ ./toolbox/`

## Working notes
