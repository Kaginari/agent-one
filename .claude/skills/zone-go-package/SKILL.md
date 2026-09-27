---
name: zone-go-package
description: How a package of agent-one is changed and proven — the package-scoped test line, gofmt and vet, the doc-comment contract, the wire in CLIs, embeds, and the leak test every file must pass. Wear it for any Go change in this repository.
---
- Prove the package you touched, not the world: `go test ./<pkg>/` (add `/...` for a package with sub-packages: `./provider/...`, `./config/...`). The whole suite is `go test ./...`; e2e tests in `app/` build the binary and are the slow ones — run a `-run` subset while iterating.
- Before you report: `gofmt -l .` must print nothing and `go vet ./...` must pass — CI fails on either. Then `go test .` from the root: the leak test scans every text file, including `.agent-one/` and `.claude/`, for the forked convention's words; a hit anywhere fails the module.
- Every package opens with a `// Package <name> …` comment that states its contract in the policy's words (what it guards, what it never does); a behaviour change edits that comment in the same change.
- Instrument CLIs (memory, toolbox, onto, config) answer on the wire: `@S PASS|OK|FAIL`, one `@?` per hole, `@E <bytes>`; exit 0 ok, 1 fail, 2 usage or load error. Keep new output in that shape.
- Embedded files (`//go:embed`): `guard/patterns.txt`, `guard/corpus.txt`, `onto/schema.ttl`, `app/policy/AGENT-ONE.md`, `app/found.md`, `app/container.Dockerfile`, `ui/assets/*`, `board/assets|static|templates|web`. Changing one is a code change: the tests that read it run again.
- Third-party code is the charm.land v2 family, chroma, colorprofile and harmonica only; HTTP, JSON-RPC, YAML and Turtle are hand-written on the stdlib. Do not add a dependency for what the stdlib already does here.
- `bench/fakevllm` is a separate module: build it with `go build -C bench/fakevllm -o /dev/null .`; the root's `./...` never sees it.
- Table tests and `t.Run` names in prose; `-race` runs in CI, so no unsynchronised shared state in tests either.

## Working notes
