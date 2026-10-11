---
name: review-core
description: How an agent here reviews, reads and acts as an instrument — the verdict/draft vocabulary, the test-golden conventions, the naming policy (dirs → member names, dotted → paths), and the day's heartbeat: build + go test -short. Held by domain owners at their gates and by the coordinator when speaking.
---

# review-core

- Verdict vocabulary lives in app/review.go: verdict kinds (pass / fail-bug / fail-feature / fail-gate) arrive from a Subagent to the dispatcher; revise docs in the seat that owns the fact, not the verdict text.
- Golden tests live as files under testdata/golden/ — UI shots in ui/shots.go, markdown fences in tui/markdown_test.go, init pages in app/init_render_test.go. A golden test with _no_ testdata file fails loudly, this is the mechanism to trust.
- Conventions sticky across the code: package path as a sentence ("the wire keeps a report's tail"); file names describe the one thing the file does (cli.go for command surface, never helpers.go); test funcs are single sentences in CamelCase.
- Build: `go build -o .agent-one/tmp/agent-one ./cmd/agent-one` (Makefile puts the binary in `.agent-one/tmp/`; bin/ is in .gitignore). Package tests: `go test -short ./...`. The bench suite is not `-short` — see bench-run skill for bench-legality.
- gate verdicts flow: tool act → guard pass → gate/gate.go Needs(class) → ask? → verdict stamped in wire + logged in .agent-one/log.md.
- When reading a finding, look first for the package's own README (pointers land there), then HANDOFF.md, then the log.
- Naming policy reminder: `.agent-one/<rank>/<short>/README.md` — name stems from folder (`domain/gatekeeper/` → `domain-gatekeeper`); onto owns claims extract backticked paths from the doc.

