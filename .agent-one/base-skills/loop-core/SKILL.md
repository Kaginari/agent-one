---
name: loop-core
description: The six-beat agent run (provider call, tool act, wire read/write, drain, verify/subagent, compact+usages) — held by the members of loop/, wire/, compact/, instrument/, and app-subagent/app-engine when the run itself is the ask. Back-ticked file/module reads anchor the facts.
---

# loop-core

- A turn in this code is the same six beats everywhere: provider → tool act → wire → drain → verify/subagent → compact+usages. Each beat is one function family in one package; a seventh beat does not exist.
- Beat 1 (provider): provider/provider.go FanOut → provider/env.go env reads → provider/openai/openai.go or provider/anthropic/anthropic.go Request. The mock provider (provider/mock/mock.go Return=fixed text stream) is the test stand-in.
- Beat 2 (tool act): app/worldtools.go tools run first; missed names descend app/shelf.go (registry -> custom -> tool/missing.go doom marker). tool/dispatch.go is the walk for dispatch of a task.
- Beat 3 (wire): wire/wire.go is THE wire — `Emit` frame, `Unsaid` flag, report lines `@S/@E/@F/@?`; byte-stable literal tag constants are here, never formatted elsewhere.
- Beat 4 (drain): workspace/drain.go + compact/compact.go — Recall emplaces memory, drain drops on turn limit (config'd `TurnBudget` and `MaxInMemory`).
- Beat 5 (verify/subagent): app/subagent.go + workspace/gate.go — Subagent blocks collect per-snare doc/beats; Verdict lands in wire + bench/runs.jsonl.
- Beat 6 (compact+usages): compact/compact.go Compress + instrument/instrument.go Measure; tokens recorded to board/usage.go.
- The wire package tag constants are the contract. Never generate them by string concat — always `wire.Emit`/`wire.Unsaid`. `wire_test.go` covers multi-line frame packing; run it after any wire touch.
- Dispatch is NOT free: tool/dispatch.go spawns a Subagent (fresh context, tools cut) — skills with big context agent live there, not inline.

**Wall contract (commit-per-slice + tree-sync-before-build)** — seat of the wall-contract commission (domain-engine routing, 2026-10-04, doom-2026-10-04-dispatch-wall). Commit-per-slice: when a commission lands slices, seal each slice on landing before the next breath — never accumulate a sack of uncommitted work across slices. Zone worker ranks have no git-write seat; their slices die with their context — armor ride is required: the answering domain owner seals the zone worker's landed slices in the same turn the zone worker reports DONE. Tree-sync-before-build: no build/act that consumes the tree runs against a dirty tree unless the dirt is the commission's own mid-flight state; the acting agent seals or checkpoints the delta first. Wall-hit (checkpoint) protocol: on wall, restate deltas already sealed, name what remains uncommitted, CHECKPOINT with the delta named. The mechanical seat at tool/dispatch.go:237 (wall-fires path = loop/loop.go checkpoint trigger) remains doc-policy-only unless an honest minimal hook exists — the zone-loop reading found none without rewriting the loop's beat structure.
- The loop stands — provider stands — guard stands. Dispatch yourself a review loop in app/subagent.go to self-heal an invalid doc; loop/loop.go's close-on-invalid-doc walks you there.

