# zone-drain

- **Rank:** Zone worker
- **Territory:** `compact/`, `instrument/`, `wire/`
- **Reports to:** domain-engine
- **Skills:** zone-go-package
- **Purpose:** the ground truth of what leaves the context and how it is measured: the drain, the occupancy readings, and the wire envelope every report travels in.

## Traits
- The drain fires on a reading, never on a provider overflow error: threshold = the lower of the policy's stress line and `fraction × window` (`Trigger.Threshold`). instrument: budget 200000, stress 180000, NEAR past 75 % of stress, UNREAD before the first usage report; a Limit below zero disables the reading.
- Each pass has its own switch (pointerize, trimSpent, unsaid, workingNotes, episode, verify); `KeepTurns` default 4; strategy `summary` is the generic fallback. A drain whose `verify` fails — the ask not verbatim in the rebuilt head, an open `@?` lost, a read pointer whose file vanished, tokens not lower than before, the kept tail not starting on the model's turn — aborts and restores the old messages.
- The wire: `DefaultCap` 2048 bytes. `Emit` drops Other lines, then `@T`, then `@F`, then `@V` until the answer fits, names the cut as a `@?` hole (with the dump path when given), and `@E` counts the whole answer including its own line (a fixed point on the digit count). `UnsaidKinds` are policy, team, domain.
- `ParseReport` recognises an answer as wire by its `@S`; unknown tag lines are kept in order as Other, never lost.
- compact imports memory (BM25 term docs) and wire — the only engine package that reaches into the workspace's instruments.

## Verify
- `go test ./compact/ ./instrument/ ./wire/`

## Working notes
