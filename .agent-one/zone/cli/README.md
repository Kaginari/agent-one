# zone-cli

- **Rank:** Zone worker
- **Owns:** `cmd/`, `app/cli.go`, `app/queue.go`, `app/queue_test.go`, `app/app.go`, `app/app_helpers.go`, `app/embed.go`, `app/found.go`, `app/found.md`, `app/found_test.go`, `app/setup.go`, `app/selftest.go`, `app/logger.go`, `app/container.go`, `app/container.Dockerfile`, `app/container_test.go`, `app/ui.go`, `app/guard.go`, `app/bench.go`, `app/mockscript.go`, `app/cli_test.go`, `app/e2e_test.go`
- **Reports to:** domain-app
- **Skills:** zone-go-package, zone-workspace-docs
- **Purpose:** the ground truth of the binary's entry: the command table, `init` and its three levels, the setup, the container re-exec, the selftest and the in-binary bench.

## Invariants
- `init` levels: light (the policy, `log.md`, `name`, the instrument and memory dirs, ontology/schema.ttl, a `.gitignore` for the derived files), medium (a team sketched from the tree with no model: a domain per top-level source dir with ≥ 3 files, at most 6, zones per sub-dir, `domain-core` for the small areas, `service-release` when CI exists), complex (light, then the founding session with `found.md` and the sketch). An existing file is never touched; `--plain` skips the setup questions.
- setup (setup.go) asks on a TTY only, unless `--plain` or `AGENT_ONE_NO_SETUP`; presets openrouter (free models), anthropic (claude-sonnet-5), openai, ollama, custom (vLLM at 127.0.0.1:8000, key `VLLM_API_KEY`).
- `--containered`: `docker run --network host`, the binary mounted read-only at /usr/local/bin/agent-one, the workspace root read-write, host paths read-only at the same path, secrets passed by NAME (`-e KEY` — docker reads the value, it never lands in argv), `AGENT_ONE_CONTAINERED=1` marks the inner process so the flag is a no-op inside. The runtime image is built from the embedded `container.Dockerfile` (debian bookworm-slim with bash, git, jq, python3, ripgrep, chromium…), from `registry.containers.base`/`apt` mirrors, or pulled as `registry.containers.image`.
- `selftest` prints `@S PASS <n> checks` over every package's selftest plus the app's — 195 today.
- `bench` (bench.go) is the in-binary bench: scripted mock tasks judged by checks, then the same asks on every real provider whose key env is set — real calls cost money; the Harbor bench under `bench/` is service-release's, not this zone's.
- `TestEmbeddedLawsMatchTheWorkspace` ties `app/policy/AGENT-ONE.md` to the root file; e2e_test.go builds the binary and app/testdata/mcpserver — the module's slowest tests.
- **queue** (`app/queue.go`): `agent-one q list|show|add|claim|touch|requeue|close` is queue.CLI over the workspace's shared queue (Workspace.QueuePath) as the human — the lexicon's human word, every move allowed (the human stands over every agent); `--as <body>` acts for an agent, `--json` (output.format json) prints the items, a refusal exits 2.

- **onto owner** (`app/cli.go` cmdInstrument → zoneOwner): wires onto.CLIOwner to a workspace opened under the config, so `agent-one onto owner <path>` answers Workspace.ZoneOfPath — the gate's own reading — instead of the graph's mirror.
- **fold sweep** (`app/app.go` New): once the workspace opens, tool.FoldSweep drops every run's fold drawer (and any loose file of the old shared shelf) untouched past tools.output.foldKeepDays (default 7; 0 keeps them) — TestAppSweepsTheFoldShelf.
- **fallback roots** (`cmd/agent-one/main.go`): the binary blank-imports golang.org/x/crypto/x509roots/fallback — a static (CGO_ENABLED=0) build trusts the host's roots, and a minimal container (no ca-certificates, SSL_CERT_FILE/SSL_CERT_DIR empty) has none; the embedded bundle is used only then (x509.SetFallbackRoots never overrides real system roots). TestFallbackRootsWhenTheHostHasNone reads the pool in a child process with both pointed at an empty dir.

## Verify
- `go test ./app/ -run 'TestEmbeddedLawsMatchTheWorkspace|TestInitBench|TestBenchOnMock|TestRunIgnoresAnOpenStdin|TestContainer|TestSetup|TestSketchTeam|TestMediumWritesOnceAndFoundingBrief|TestNormLevel|TestE2E|TestQueueCommand'`
- `go vet ./cmd/...`
- `go test -count=1 -run TestFallbackRootsWhenTheHostHasNone ./cmd/...`

## Working notes
