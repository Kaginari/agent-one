# coord-engine

- **Rank:** Coordinator
- **Owns:** `loop/`, `instrument/`, `provider/`, `wire/`, `compact/`, `guard/`, `gate/`, `sandbox/`, `shell/`, `tool/`
- **Reports to:** orchestrator
- **Skills:** loop-core, gate-safety, review-core
- **Purpose:** the shared skill of the run lane: plans an order across domain-engine and domain-safety, routes each slice to the domain owner whose domain owns it, and speaks the lane's verdicts back up — never authors code.

## Invariants
- The coordinator rank neither authors nor holds a gate (DefaultRanks in workspace/rank.go): workspace.Owner never resolves a path here, the ownership is the lane's routing map. On an order it writes its plan deck first (the plan tool, one line per slice, each named to its domain owner), then dispatches, in parallel when the slices are independent.
- domain-engine holds the six-beat turn engine (loop, instrument, provider, wire, compact); domain-safety holds every refusal and container (guard, gate, sandbox, shell, tool). A change to the order of a tool step touches both: plan it as one slice per domain owner and land the safety half first.
- The step order is a contract across both lanes: budgets → unknown tool → class settled (tool classify) → guard → ownership policy → permissions → the human gate → run (sandboxed) → guts reading → doom check.
- Provider retry is shared (provider/retry.go) and the model fallback lives in app — a provider change that adds or removes a retry path is a cross-lane change with coord-host.
- Every command an agent or the config can name runs through sandbox.Contain (its own process group, killed whole at the timeout); a new exec site that skips it is a hole for domain-safety.
- The guard and the classifier read commands through the same guard.CommandWords: a command named by its path, an alias escape or a wrapper is still that command.

## Verify
- `go test -count=1 ./loop/... ./instrument/... ./provider/... ./wire/... ./compact/...`
- `go test -count=1 ./guard/... ./gate/... ./sandbox/... ./shell/... ./tool/...`

## Working notes
- (none yet)
