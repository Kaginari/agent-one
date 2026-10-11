# zone-guard

- **Rank:** Zone worker
- **Owns:** `guard/`, `gate/`
- **Reports to:** domain-safety
- **Skills:** zone-go-package, zone-act-classes
- **Purpose:** the ground truth of the two hard stops: the denylist nothing can approve, and the human gate nothing approves without a human.

## Invariants
- patterns.txt is embedded and always on: one POSIX ERE (RE2) pattern per line, `#` comments; it blocks only the catastrophic and irreversible — rm -rf of /, a top-level dir or home, any sudo rm, dd/mkfs/diskutil onto a device, raw disk redirects, the fork bomb, curl|sh, git push --force / --delete / :ref / +ref, reflog expire and gc --prune=now, chmod 777 /, chown -R … /, GitHub repo and secret deletions, secret-store reads. Recoverable local commands stay allowed on purpose.
- Sources add, never remove: Default() is the built-in list; Load adds files (a missing file is no finding, an unreadable one or a bad pattern is a Note, never a crash); ForHome adds the machine-wide ~/.agents/hooks/dangerous-patterns.txt (SharedPath) and a rule for the home by its own path; AddPatterns takes config's guard.patterns. Match returns the first Rule with its Source; Refusal renders the message the model reads.
- Match tests each rule against the raw command AND against CommandWords(cmd): command words at a command position (start, after ; & | ( ` $( or a quote) lose their path or alias escape — `/usr/bin/git push --force`, `\rm -rf ~` — and wrappers are seen through (xargs, exec, command, nohup, nice, env, time, sudo, timeout, with their flags and assignments). Arguments stay: `cat /usr/bin/rm` is allowed. tool.ClassifyCommand reads commands through the same CommandWords.
- corpus.txt is the test: `block <cmd>` / `allow <cmd>` lines (155 cases), `\n` unescaped by Cases(); Guard.Test returns passed and wrong. The same file is read by tool's TestGuardCorpusNeedsTheHuman, so a new block line must also classify ≥ outward without the guard.
- Hook(in, out, errw, cursor) answers a pre-tool hook from stdin (.tool_input.command, .toolInput.command or .command): blocked is exit 2 with the reason on stderr, or a deny JSON on stdout under cursor. ClaudeSettings patches a settings JSON to add it; OpenCodePlugin(binary) emits the JS plugin. `agent-one guard install --yes` writes both; without --yes it only shows the change.
- It guards against accidents, not a determined agent: `python -c "shutil.rmtree(...)"` slips past any regex. The sandbox and the gate remain the Egress control.
- Decisions: not-needed, approved, denied, would-ask, suspended. Gate.Needs(c) is `c >= Outward || (Strict && c == Write)`; Request.Force (a permission rule said ask) makes the gate ask whatever the class.
- Ask order: not needed → pre-approved by class (Approve map, skipped under Force; By "pre-approved") → DryRun (would-ask, By "dry-run") → Gate.Answer (the TUI's choice block; anything but Approved or Suspended becomes Denied; By "tui" when unset; a false second value falls through) → the TTY prompt (yes regex `^(?i)y(es)?$`; By "tty") → no TTY: Denied with the hint `pass <Flag> <class> to pre-approve` (By "no-tty"). Nothing in this list approves on its own.
- Suspended is not a refusal: the host parked the question durably; the loop ends the turn SUSPENDED and Resume revives it at that step.
- Request carries ID, Tool, Class, Why, Summary, Force, Agent and Input (the tool's JSON, which a rule proposal reads its subject from).
- Flag is the name shown in a denial ("--approve" by default) so agent-one can name its own; Prompt replaces DefaultPrompt; In/Out default to stdin/stderr; IsTTY is injectable for tests.
- ParseApprove reads "outward,destructive" into the class set and rejects unknown names — a typo is an error, never a silent no-op.
- Standing pre-approvals are the human's written word: config refuses them from env, and `config check` lists every one as a loosening.

## Verify
- `go test ./guard/ ./gate/`

## Working notes
