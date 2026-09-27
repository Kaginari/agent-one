# zone-guard

- **Rank:** Zone worker
- **Territory:** `guard/`, `gate/`
- **Reports to:** domain-safety
- **Skills:** zone-go-package, zone-act-classes
- **Purpose:** the ground truth of the two hard stops: the denylist nothing can approve, and the human gate nothing approves without a human.

## Traits
- guard: one RE2 pattern per line in the embedded `patterns.txt` (48 lines), always on; ~/.agents/hooks/dangerous-patterns.txt (`SharedPath` — the file other agents' hooks read too) and config add patterns, never remove. A match is refused before the classifier and the gate see it. It guards accidents, not a determined agent: `python -c "shutil.rmtree(...)"` slips past any regex.
- `corpus.txt` (153 lines) is the test corpus; `Guard.Test()` runs it and `agent-one guard test` reports it; the tool package reads the same file to prove every case classifies outward or destructive. `guard install --yes` writes the hook into Claude Code's settings and an OpenCode plugin; without `--yes` it only shows the change.
- gate: Decision is not-needed · approved · denied · would-ask. `Needs(class)` is true for Outward and Destructive, and for Write under Strict. `Ask` never returns Approved without a human's word: a TTY answer matching `^(?i)y(es)?$`, a pre-approval from `--approve outward,destructive` (`ParseApprove`), or the terminal UI's `Answer` func (still the operator's word); DryRun answers would-ask; no TTY and no pre-approval is a denial (`By` "no-tty").
- A permission rule that says `ask` sets `Request.Force`: the gate asks whatever the class.

## Verify
- `go test ./guard/ ./gate/`

## Working notes
