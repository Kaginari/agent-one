# zone-loop

- **Rank:** Zone worker
- **Territory:** `loop/`
- **Reports to:** domain-engine
- **Skills:** zone-go-package
- **Purpose:** the ground truth of the turn engine: the six beats of a step, the journal, the tree snapshot that sees every write, resume.

## Traits
- A turn's writes are seen by stamping the whole tree (size + mtime) before and after — never by trusting a tool's word: `xargs touch` under a read-classified command is still a write (snapshot.go, `seen`). The walk stops at `SnapshotCap` = 200000 entries; past it the session is unwatched and the hole is named once.
- Unwatched paths: `.git`, and under the workspace dir `instruments`, `tmp`, `memory`, toolbox/registry.json, ontology/graph/unsaid.ttl, `log.md`.
- Test files are read as the turn opens (256 KiB a file, 4 MiB in all) so the gate can see deleted or skipped tests; `IsTestFile` knows Go (`_test.go`), JS/TS (`.test.`/`.spec.`) and Python (`test_*.py`, `_test.py`).
- Statuses DONE · DRY · FAIL · ESCALATE · DENIED · CHECKPOINT; a budget or wall-clock stop is CHECKPOINT with a `resume: agent-one resume <id>` hint. Defaults: 50 steps a turn, 30 minutes a session, 2 retries. Journals are JSONL `Event` maps under the engine's JournalDir, in the shape loop.js writes.
- A wire answer with no `@U` when the unsaid was asked is a hole, not a failure; an answer with no `@S` at all is a hole too. `StopMaxTokens` and `StopRefusal` become holes.
- `loop.Hooks` is the only seam (System, Perceive, Recall, Record, Budget, Decide, PreTool/PostTool, EndGate, Drain, Missing, GateRetries…); every one is optional. New behaviour enters through a hook, not through an import.

## Verify
- `go test ./loop/`

## Working notes
