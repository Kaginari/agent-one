---
name: gate-safety
description: Egress control mechanics of the safety lane — worm (ask first) and bwrap (class gate on filesystem writes) for gate/guard/sandbox/shell-bearing members; instrument for the reading; discovery for where things leave. Registered for gatekeeper domain owners and their zone workers.
---

# gate-safety

- Egress control is two mechanisms: `worm` package (gate/gate.go — ask first; verdict recorded on disk) and `bwrap` (sandbox/sandbox.go — bubblewrap confinement for spawned processes on Linux).
- shell/shell.go is the only exec path. `bash` tool routes here; any new outward path must enter through shell to inherit TTY + sandbox + gate.
- instrument/instrument.go measures; it reads from `.agent-one/instruments/*.jsonl` keyed by caller-id.
- gate/gate.go's `Needs` function lists classes — passing the gate requires declaring a `class` that does NOT weaken the default (read → write → outward → destructive).
- guard/guard.go denylist is the gate's teeth: no `git push`, no `kubectl/oc` mutation, no `rm -rf` on workspace roots. Edit via guard/cli.go's corpus merge.
- sandbox/sandbox.go mode `none` skips confinement but NOT the gate; on macOS bwrap is absent, auto-fallbacks to `none` (see sandbox_test.go). Darwins rely on guard+gate strictly.
- Permissions are persistent allow-lists in `.agent-one/` at `approvals.jsonl`. `guard/guard_test.go` (`TestTheCorpus`) freezes the corpus syntax over `guard/corpus.txt`.
- sandbox+".agent-one" is the trust boundary: anything the sandbox respects is hooked there first.

