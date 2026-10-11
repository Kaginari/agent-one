---
name: zone-wire-and-journals
description: The machine formats the agent-one binary reads and writes — the wire envelope (@S @F @V @? @U @E, commission tags), the loop journal events, the usage journal, log.md entries, and the memory/toolbox files shared with the JS instruments. Hold when a change touches any of those formats or a package that parses or renders them.
---

- Report (a Subagent's answer, a hook's stdout, `run --format wire`): first line `@S <status word>`; then `@F` findings (file:line fact), `@V` verdicts with evidence, `@?` holes (named, never guessed), `@U <policy|team|domain> text` the unsaid, `@E <n>` the size. Parsed by wire.ParseReport (ok only with @S); rendered by Report.Emit(cap, dump) — over cap it is cut and the cut becomes a hole. DefaultCap 2048.
- Commission (going down to a Subagent): `@ROOT`, `@SCOPE`, `@ASK`, `@CAP`, `@DUMP`, `@SIZE`; tool.Commission.String() renders it, workspace.Gate's Duties-done check parses it back and fails a commissioned turn whose answer carries no @S.
- On an openai-compatible provider a Subagent's report can be guided-decoded to openai.WireSchema (S, F, V, Q, U) and rendered back by RenderWire — keep the schema and the tags in step.
- Loop journal: .agent-one/instruments/loop/<run-id>.jsonl, one JSON object per beat with "t" ∈ perceive, gate, act, verify, record, hook, drain-report, drain-abort and "id" s1, s2…; loop.ReadJournal / State / Runs rebuild a run; the board's Run view and `resume` read it. Add a field, never rename one.
- Usage journal: .agent-one/instruments/usage/<session>.jsonl, app.UsageRecord per provider call (session, agent, role, rank, model, input/output/cacheRead/cacheWrite, cost); `agent-one usage` and the board's /usage roll it up (Ranges 24h 7d 30d all).
- log.md entry (workspace.Entry.Render): `### [RFC3339] <author> — <title>` then `- **Task:**`, `- **Files:**`, `- **Gate:**`, `- **Result:**`, `- **Learned:**` (one line or sub-bullets). Appended only through workspace.Log, which refuses when the remembered prefix changed (Policy 4).
- Memory files (shared with .agent-one/tools/memory.js): memory/long/index.json (IndexV 2), memory/short/<member>.jsonl, memory/shared/notes.jsonl (append-only JSONL). Toolbox files (shared with toolbox.js): toolbox/registry.json (RegV 1), toolbox/extra.jsonl, loads under instruments/toolbox. Output of `memory status|recall` and `toolbox brief|status` must equal the JS tools' byte for byte — memory/js.go exists for that.
- Working notes: .agent-one/instruments/working-notes/<session>.md, `# working notes — <body> — <date>` then `- note` lines, at most 5 before it is a stress reading.

## Working notes
