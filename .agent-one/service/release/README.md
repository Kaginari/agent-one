# service-release

- **Rank:** Service owner
- **Territory:** `.github/`, `.goreleaser.yaml`, `Dockerfile.release`, `install.sh`, `RELEASING.md`, `CHANGELOG.md`, `bench/`, `.gitignore`
- **Reports to:** orchestrator
- **Skills:** service-release-flow
- **Purpose:** owns CI, releases and the benchmark harness end to end, across sessions; anything that tags, publishes, pushes an image or spends on a gateway waits for the operator.

## Traits
- `ci.yml` on push to main and PRs: gofmt clean, `go vet`, bubblewrap installed and `kernel.apparmor_restrict_unprivileged_userns=0`, `go test -race ./...`, `go run ./cmd/agent-one selftest`, then the tree must be clean (`git status --porcelain` empty — a test that writes a tracked file fails CI); a linux/darwin × amd64/arm64 build matrix; `goreleaser check`; and bench-smoke (Harbor in Docker against the fake vLLM: right must score 1.0, wrong 0.0).
- `release.yml` on `v*` tags: tests, then the tag's `## [X.Y.Z]` section of CHANGELOG.md must exist or the release fails; GoReleaser builds tar.gz archives named `agent-one_<os>_<arch>` (no version, so `releases/latest/download` resolves), `checksums.txt`, the GHCR image `ghcr.io/kaginari/agent-one` tagged `<version>` and `latest` (never `latest` on a pre-release), and Sigstore attestations only when the repository is public.
- `install.sh` downloads the archive and `checksums.txt` from `releases/latest` (or `VERSION=vX.Y.Z`) and verifies sha256 before installing to `~/.local/bin`.
- bench: `runs.jsonl` is append-only and `RESULTS.md` is generated from it by `record.py` ("do not edit"); `record.py --expect` asserts a mean reward; `harbor.sh` runs real models on a gateway (cost) and forwards the key by NAME only — never run without the operator. `jobs/`, `.smoke-config.yaml`, `.fakevllm-*.log` and `__pycache__/` are gitignored. `bench/fakevllm` is its own Go module, built with `go build -C bench/fakevllm`.
- The Harbor adapter (`harbor_agent.py`) installs the static binary into the task container and seeds `.<dist>/config.local.yaml` from BENCH_CONFIG; `configs/fake-vllm.yaml` switches the human gate and the bash sandbox off because nobody answers inside a task container — that file is never a template for a real workspace.
- `bench/harbor/__pycache__/` and `bench/__pycache__/` are left behind by runs — the first is root-owned, written by Harbor's Docker runner — so `python3 -m py_compile` fails on this tree; syntax is checked with `ast.parse` instead, which writes nothing.
- On this machine: docker, python3, bwrap and chromium are present; goreleaser and the harbor CLI are not, so `goreleaser check` and a smoke run are CI's readings, not local ones.

## Verify
- `test -z "$(gofmt -l .)"`
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /dev/null ./cmd/agent-one`
- `go build -C bench/fakevllm -o /dev/null .`
- `bash -n bench/smoke.sh bench/harbor.sh install.sh`
- `python3 -c 'import ast,sys; [ast.parse(open(f).read()) for f in sys.argv[1:]]' bench/record.py bench/harbor/harbor_agent.py`

## Working notes
