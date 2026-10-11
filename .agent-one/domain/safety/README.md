# domain-safety

- **Rank:** Domain owner
- **Owns:** `tool/`, `shell/`, `sandbox/`, `guard/`, `gate/`, `mcp/`
- **Reports to:** coord-agent-one
- **Skills:** domain-gate-review, zone-act-classes
- **Purpose:** rules every act's reach — its class, its containment, the human's word — and holds the gate: no change loosens a class, widens a bind, scrubs less, or approves without a human, without this owner's pass.

## Invariants
- Order of defence for a shell command: `guard.Match` (refused, no approval can run it) → `Tool.Settle` (declared floor, classifier, the caller's own declaration — each only tightens) → `config.Decide` (permission rules allow · ask · deny) → `gate.Ask` (outward and destructive always ask; Strict makes writes ask). Only the operator's word or an explicit `--approve <class>` approves.
- `guard/corpus.txt` (153 lines) is read by two tests: the guard's own and `TestGuardCorpusNeedsTheHuman` in the tool package — every corpus case must classify outward or destructive, never a silent write.
- The sandbox scrubs the environment before any child sees it (names ending API_KEY, TOKEN, SECRET, PASSWORD, PASSWD; EnvDrop globs; EnvAllow wins); MCP stdio servers are spawned through the same scrub.
- Record paths — AGENT-ONE.md, log.md, design/*.md, notes.jsonl — are destructive to overwrite, by file write and by shell redirect, sed -i, tee or cp (Policy 4).
- The fence: bwrap gives every command its own pid, ipc and uts namespaces and masks the home's credential paths (~/.ssh, ~/.gnupg, ~/.aws, ~/.kube, ~/.docker, ~/.netrc, gcloud, gh, keyrings, browser profiles); on macOS the same fence is a Seatbelt profile through sandbox-exec. A timeout kills the command's whole process group.
- A clone cannot loosen the policy: an allow of a gated class, or the human gate switched off, in the tracked `.agent-one/config.yaml` is refused — it lives in config.local.yaml or the global config. The gate runs no model-written command: verify lines are read as they stood before the turn.
- Zone workers: zone-tools (tool/), zone-shell (shell/, sandbox/), zone-guard (guard/, gate/), zone-mcp (mcp/).

## Verify
- `go test ./tool/ ./shell/ ./sandbox/ ./guard/ ./gate/ ./mcp/`

## Working notes
