# zone-session

- **Rank:** Zone worker
- **Owns:** `app/repl.go`, `app/session.go`, `app/subagent.go`, `app/wake_held.go`, `app/wake_held_test.go`, `app/goal.go`, `app/review.go`, `app/handoff.go`, `app/usage.go`, `app/tui.go`, `app/tui_test.go`, `app/tuiboard.go`, `app/web.go`, `app/web_test.go`, `app/dash.go`, `app/board.go`, `app/board_test.go`
- **Reports to:** domain-app
- **Skills:** zone-go-package
- **Purpose:** the ground truth of the live session: the REPL and its slash commands, the session store and resume, the subagent tracker, the terminal and web hosts, the usage meter, goal, review and handoff.

## Invariants
- Sessions are JSONL under the session store (`sessions.*`); `resume <id>` replays the messages and appends a `resume` event; `sessions` lists them. `Sync` writes the loop's messages after each turn.
- Slash commands (repl.go): quit/exit/q, help, dash, agents/subagents, send, usage, status, compact, handoff, resume…; the terminal merges `builtinCommands` (tui.go) with discovered commands; `/board` opens the in-terminal board.
- `tuiHost` implements `tui.Host`: approvals come through `approve` (the choice block — yes, no with a reason, don't ask again which writes a permission rule to .agent-one/config.local.yaml), questions through `ask`; the `webBus` fans the same `tea.Msg` stream out as JSON to the dashboard (`dash`), replaying up to 4000 events; browser answers travel back through the board's token-guarded acts.
- `Meter` wraps a provider: every call is journaled to instruments/usage/<session>.jsonl with tokens and USD (price from the registry); `budgets.session` and `budgets.subagent` stop the next call once reached (`Over()`); `usage` and the board roll the journal up by agent, role, model and day.
- `Subagent` tracks live agents (name, rank, role, model, depth, state, reading), an inbox per agent (`/send`), and wake-ups when a subagent finishes. `review` runs two reviewers (judge and drafter roles) on two models in parallel and merges one shortlist; `goal` works turn after turn until `--validate` exits 0, at most 12 turns by default; `handoff` writes under <workspace>/handoffs/.
- The tmp guard leaves a pattern alone (isPattern: regex syntax a path never has, or an address in a sed/awk/perl inline script — after -e, else the first operand unless -e/-f gave the script; perl's first operand is a file); globs stay paths.
- **relay hold** (`app/subagent.go`, `app/wake_held.go`): Subagent.Finish holds a background report's `@$` orders on the workspace's shared queue (the memory order book only with no workspace). Nothing dispatches a held row mid-turn; the wake — the REPL's and the terminal's turn-done drain, and a report's wake into an idle session — claims every held row in its agent's name (heldWakeLines; For "" claims for orchestrator; a row another session claimed first is skipped) and opens the next turn with their dispatch prompts (queue id named, the agent's earlier report as thread), re-checked by the chain gate; the terminal sends EvHeld with the held count at each report and claim. An agent's report settles its oldest claim (settleHeld — a foreground dispatch's end, a background Subagent's noted): DONE closes it, any other word touches it with the exit, still claimed; a background ack settles nothing.

## Verify
- `go test ./app/ -run 'TestREPLLive|TestCLIRunAndSessions|TestRoutingAndMeter|TestWebBusReplaysAndAnswers|TestTUI|TestBoard|TestE2EHandoff|TestE2EGoal|TestE2EReview|TestRelayHold'`

## Working notes
