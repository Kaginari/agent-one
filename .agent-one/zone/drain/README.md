# zone-drain

- **Rank:** Zone worker
- **Owns:** `compact/`, `instrument/`, `wire/`, `queue/`
- **Reports to:** domain-engine
- **Skills:** zone-go-package, zone-wire-and-journals
- **Purpose:** the ground truth of what leaves the context and how it is measured: the drain, the occupancy readings, and the wire envelope every report travels in, and the shared work queue its @$ orders are held on (queue/).

## Invariants
- Two strategies: "drain" (the default — pointerize → trimSpent → one model call for the unsaid and the working notes → episode, each pass behind its own Switch in Options.Passes) and "summary" (one summarising call, the generic fallback). Verify runs when its Switch is on and always for "summary".
- Trigger.Threshold(stress) is Trigger.Tokens (0 = the stress line), lowered to Window × Fraction when both are set — the lower wins. Drainer.Perceive lowers the reading's stress line (and re-zones it) to that threshold; Drainer.Hooks() hands the loop Perceive and Drain only.
- cutIndex keeps message 0 (the ask) always and the assistant turns of the last KeepTurns exchanges (0 = 4); fewer than one finished exchange is "nothing to drain" — an abort.
- Any failed pass restores the old messages, journals `drain-abort` with the report and returns an error — the loop then checkpoints. A landed drain journals `drain-report` and sets Session.Last to the rebuilt size, so the next reading is the drained one.
- The rebuilt head is the ask plus @NOTES, @D lines, @OPEN, @POINTERS (a pointer whose digest changed is flagged, not trusted) and @DRAINED. The working notes file is <workspace>/instruments/working-notes/<run>.md; with no model working notes a mechanical one is written. More than memory.WorkingNotesLimit notes is a stress flag in Report.Flags, never a truncation.
- Drained turns are appended to <workspace>/memory/short/<run>.drain.jsonl (numbered on from the last N) so the `drained` tool can recall them; landed writes call Homes.Episode (the workspace's log entry) when the Episode pass is on.
- Tokens(msgs) is an estimate (about four bytes a token), used only for the report's Before/After; the trigger reads the provider's usage.
- A report is recognised by its @S line (ParseReport returns ok only then); tags match tagLine `^@([A-Z]+|[?$])(?:\s+(.*))?$`, so an @-word mid-line is prose. The raw pipe form (`S|PASS`, `F|path:ln|claim`) is read for S F V ? U E T $ only. Unrecognised lines are kept in Report.Other, in order.
- UnsaidKinds are exactly policy, team, ownership; an @U whose first word is not one of them keeps Kind "" with the whole text. memory.Unsaid and compact's landing use the same three words.
- @$ `<agent>: <ask>` is an Enqueue (agent folded lower-case) — work ordered one layer down that the Subagent drains after the turn; EnqueuesOf reads the same lines from any text, so a commission can state the relay it expects. A @$ without `<agent>:` stays prose.
- @FORGE `<kind>:<name> | <text>` is a Forge draft, kind only tool or skill (folded lower), riding up for curation — the wire never registers it. A malformed @FORGE stays prose. Forge.Hash is its content address (fnv64 over kind, name and whitespace-folded text, 10 hex) — the same at every relay hop; ForgesOf reads @FORGE lines out of any text, prose included.
- DefaultCap is 2048 bytes; Commission.CapOrDefault applies it. Report.Emit(cap, dump) closes with an exact @E (it counts its own line) and, over the cap, drops lines in this order — Other, @T, @FORGE, @F, @V — never @S, @? or @U, and adds a `@CAP n: k lines cut` hole naming the dump path when set.
- Consumers outside this package: the openai provider's WireSchema/RenderWire, the loop's wire and +unsaid bounces, the workspace gate's Duties-done check, workspace/subagent.go and workspace/forge.go (Forges), app/claude.go (ForgesOf on the Stop hook's final text), app/subagent.go (Enqueues), app.go (EnqueuesOf on an ask), app's shell hooks, the dispatch tool and the TUI's report view. A tag added here is a tag added in each of them.
- queue/: Item snapshots (id q<n>, rev, op, state, rank, for, from, ask, owner, by, note, added, at) appended one JSON line per move to one file, never rewritten (Policy 4); Fold seats a line only when it is an add at rev 1 or follows its item's last folded rev on a non-terminal item, so a racing second writer reads back a Refusal and never a double claim — no lock, no database. States held → claimed → done | dropped (terminal); claim takes held work ordered to the claimant (or May over it), touch/requeue/close take what the holder holds (or May over it), held work closes by whoever queued it. Ranked: claimed, held, closed; rank 0 first, then age. CLI (`q list [--all] | show | add | claim | touch | requeue | close [--drop]`, --as, --json) exits 2 on a Refusal, 1 on an unreadable file.

## Verify
- `go test ./compact/ ./instrument/ ./wire/ ./queue/`

## Working notes
