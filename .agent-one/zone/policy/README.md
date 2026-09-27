# zone-policy

- **Rank:** Zone worker
- **Territory:** `workspace/`
- **Reports to:** domain-workspace
- **Skills:** zone-go-package, zone-workspace-docs
- **Purpose:** the ground truth of the policy in code: ranks, ownership, the review gate, the append-only log, dispatch down the ranks, recall and record.

## Traits
- `DefaultRanks`: coord (drafter, ephemeral), domain (judge, holds the gate, sideways), zone (analyst, authors), service (judge, authors and holds the gate, persistent), principal_coordinator, principal_domain_owner, auditor (persistent, read-only shelf). `Ranks.Validate`: every rank reaches orchestrator, an authoring rank has a gate at or above it, prefix = dir + "-".
- The gate reads verify lines as they stood before the turn, so an agent cannot rewrite the check it is judged by in the same turn; a line changed this turn is a hole the gate holder confirms. Per-command timeout 120 s; commands run with `bash -c` in the workspace root. Verdict words: pass · fail · n/a · off.
- `testsIntact` fails a turn that deleted a test file holding tests, lost test declarations, or gained skip/only markers — Go, JS/TS, Python.
- `VerifyOf` reads `- **Verify:** cmd` lines and the bullets under a `## Verify` heading; exit codes, not prose, are the invariants' reading.
- Dispatch goes down the ranks only: a member may not dispatch a rank above or beside it (except domain ⇄ domain); `Depth` increments per hop; a subagent's writes are gated on its own account and subtracted from the parent's (`ownWrites`).
- `Log.Append` remembers length and SHA-256 and refuses when the prefix changed (Policy 4). `Lexicon.Default` names the rank dirs coord/domain/zone/service/auditor and feeds onto and the board through `Layout()`.
- `LoadPolicy` splits AGENT-ONE.md into sections for the `policy` tool; `LoadInstructions` reads CLAUDE.md/AGENTS.md nearest first.

## Verify
- `go test ./workspace/`

## Working notes
