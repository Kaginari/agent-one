# Harbor benchmarks — agent-one vs OpenCode, head to head

`bench/harbor.sh` runs agent-one and [OpenCode](https://opencode.ai) on the same tasks, the same
models behind one OpenAI-compatible gateway, the same number of attempts, under
[Harbor](https://github.com/harbor-framework/harbor): one Docker container per task, the agent inside
it, the task's own tests deciding. Every launch lands in `runs.jsonl`; `RESULTS.md` is regenerated
with an agent × model table.

## Prerequisites

| what | why | check |
|---|---|---|
| Docker Engine, usable by your user | every task is a container | `docker info` |
| Harbor CLI — `uv tool install harbor` (or `pip install harbor`) | the runner | `harbor --version` |
| or the runner image instead of the CLI: `docker build -t agent-one-harbor bench/harbor` | Harbor in a container, nothing in your Python | `docker image ls agent-one-harbor` |
| the static binary: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/agent-one ./cmd/agent-one` | installed into each task container | `bin/agent-one version` |
| the gateway key in your shell: `export GATEWAY_API_KEY=…` | forwarded by **name**, never written | `bench/harbor.sh` preflight |
| `python3`, `curl` | `record.py`, the gateway probe | |

The preflight checks all of these and stops at the first failure with the fix. Task containers need
network access to install OpenCode (`npm i -g opencode-ai`) and to reach the gateway.

## Quick start

```sh
export GATEWAY_API_KEY=…      # the key, in the shell only
bench/harbor.sh --gateway https://llm-gateway.example.corp/v1 --key-env GATEWAY_API_KEY \
                --models kimi-k3,glm-5-3 --attempts 3
```

That is: both agents × two models × 3 attempts on the default suite (`bench/tasks`). Add `--dry-run`
first to see every command and file without running anything.

## Flags

Flags win over their environment variables.

| flag | env | meaning |
|---|---|---|
| `--gateway URL` | `BENCH_GATEWAY_URL` | the OpenAI-compatible base URL, ending in `/v1` (required) |
| `--key-env NAME` | `BENCH_KEY_ENV` | the variable holding the key; its **name** (default `GATEWAY_API_KEY`) |
| `--models a,b` | `BENCH_MODELS` | model ids as the gateway's `/v1/models` lists them (required) |
| `--agents a,b` | `BENCH_AGENTS` | `agent-one`, `opencode`, or both (default both) |
| `--tasks p1,p2` | `BENCH_TASKS` | local task dirs or dirs of tasks; one Harbor job per entry (default `bench/tasks`) |
| `--dataset org/name@v` | `BENCH_DATASET` | a Harbor registry dataset instead of `--tasks` (e.g. `terminal-bench@2.0`) |
| `--attempts N` | `BENCH_ATTEMPTS` | attempts per task, for every agent and model (default 1) |
| `--concurrency N` | `BENCH_CONCURRENCY` | concurrent trials inside one Harbor job (default 1) |
| `--run NAME` | `BENCH_RUN` | the run's directory, `bench/jobs/NAME` (or an absolute path); an existing one is resumed (default: timestamp) |
| `--purpose TEXT` | `BENCH_PURPOSE` | recorded with every launch |
| `--runner host\|docker\|auto` | `BENCH_RUNNER` | the `harbor` CLI on PATH, or the `agent-one-harbor` image (`HARBOR_IMAGE`) |
| `--opencode-provider compat\|openai` | `BENCH_OPENCODE_PROVIDER` | how OpenCode reaches the gateway (below) |
| `--allow-host H` | | let the agent reach a host or CIDR; repeatable |
| `--dry-run` | | print every command; preflight failures become warnings |
| `-- …` | | everything after `--` goes to `harbor run` (e.g. `-- --timeout-multiplier 2`) |

## What a run does

1. **Preflight**: docker, harbor, `bin/agent-one` (static?), `python3`, the key variable, then
   `GET <gateway>/models` — the key goes to curl through its stdin config, never on a command line —
   and warns when a `--models` id is not among the ids served.
2. **The run directory** `bench/jobs/<run>/` gets `agent-one.yaml`: `configs/gateway.yaml` with the
   gateway URL, the key's *name* and the first model filled in. The adapter
   (`harbor/harbor_agent.py`) writes it into each task container as `.agent-one/config.local.yaml`.
3. **One Harbor job per (agent, model, task set)**, named `<agent>__<model>__<tasks>`, from a job
   config `<name>.yaml` written beside it (`harbor run -c`). The key reaches the container's
   environment through Harbor: for agent-one by `BENCH_FORWARD_ENV`, for OpenCode by a
   `${NAME}` template Harbor resolves at run time.
4. **Record**: `record.py <job> --run <run> --purpose …` appends to `runs.jsonl`, regenerates
   `RESULTS.md`, and marks the job with `<name>.recorded` beside it.

### How OpenCode reaches the gateway

- `compat` (default): an OpenCode provider named `gateway` using `@ai-sdk/openai-compatible` —
  `/chat/completions`, the same protocol agent-one speaks, so both harnesses hit the same endpoint.
  The model is `gateway/<model>`, the key `{env:<NAME>}` in OpenCode's config.
- `openai`: Harbor's built-in `openai/<model>` provider with `OPENAI_BASE_URL` set to the gateway.
  Newer OpenCode routes the `openai` provider through the Responses API, which vLLM gateways may
  not serve; use it when the gateway does.

### Resuming

Rerun the same command with `--run <name>`. A job with a `<name>.recorded` mark is skipped; one
with a `result.json` is only recorded; one with a `config.json` but no result is resumed with
`harbor jobs resume`; the rest start fresh. A failed job does not stop the others; the launcher
exits non-zero at the end and says so.

## Where results land

```
bench/jobs/<run>/
├── agent-one.yaml                       # the agent's config for this run (no secrets)
├── agent-one__kimi-k3__tasks.yaml       # one Harbor job config per (agent, model, task set)
├── agent-one__kimi-k3__tasks/           # Harbor's job: config.json, result.json, trials/…
├── agent-one__kimi-k3__tasks.recorded   # the launch is in runs.jsonl
├── opencode__kimi-k3__tasks.yaml
└── opencode__kimi-k3__tasks/
bench/runs.jsonl                         # one line per job, append-only (field "run" ties them)
bench/RESULTS.md                         # generated: head to head per run + every launch
```

`bench/jobs/` is not tracked; `runs.jsonl` and `RESULTS.md` are. Per-trial logs (the agent's
`run.json`, `run.stderr`, the usage journal, OpenCode's `opencode.txt` and ATIF trajectory) sit
under each job's `trials/`; `harbor view <job dir>` browses them.

## Comparing models and agents

`RESULTS.md` opens with **Head to head**: for each of the last runs, one row per model (provider
prefix stripped, so `gateway/kimi-k3` and `openai/kimi-k3` are one row), one column per agent,
each cell `pass rate (trials) · errors · input/output tokens · cost`. Pass rate is the mean reward
weighted by trials across the run's task sets; tokens and cost show when every launch in the cell
reported them. Cost is known only where the config prices the model (`registry.models` in
`configs/gateway.yaml` for agent-one; OpenCode reports its own). Rerun a comparison under a new
`--run` name; `python3 bench/record.py --render` rebuilds the tables without adding a line.

## Troubleshooting

- **`harbor: not installed`** — `uv tool install harbor` (or `pip install harbor`), or build the
  runner image: `docker build -t agent-one-harbor bench/harbor` and pass `--runner docker`.
- **`gateway: GET …/models failed`** — check the URL (must be the base, usually ending in `/v1`),
  the network from this machine, and that `$NAME` holds a key the gateway accepts. A corporate CA:
  add `tls: { caFile: … }` under `providers.gateway` in `configs/gateway.yaml`.
- **`does not list model 'x'`** — use the ids the preflight printed; the example's
  `muse-glimmer`, `kimi-k3`, `glm-5-3` are placeholders.
- **The agent cannot reach the gateway from inside a task** — the task's `network_mode` must allow
  it (`bench/tasks/hello-file` is `public`); for a gateway on a private address pass
  `--allow-host <ip or cidr>`. With the docker runner a gateway on `localhost` is not the
  container's localhost: use the host's address on the Docker bridge (`docker network inspect bridge`).
- **`bin/agent-one is dynamically linked`** — build with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`.
- **OpenCode errors on every trial** — try `--opencode-provider openai` (or back to `compat`);
  read `trials/*/agent/opencode.txt`. Installing OpenCode needs `npm` access from the task container.
- **`docker: permission denied`** — add your user to the `docker` group, or run with a socket you can use.
- **`run dir: cannot write under bench/jobs`** — the docker runner (and `smoke.sh`) write Harbor's
  output as root; `sudo chown -R $USER bench/jobs`, or give `--run` an absolute path elsewhere.
- **Nothing runs and the dry-run shows warnings** — those are the preflight failures a real run stops on.
- **A run stopped half-way** — rerun with `--run <name>`; nothing already recorded is redone.
- **€0 plumbing check** — `bench/smoke.sh` runs the same Harbor path against the fake server
  (`bin/fakevllm`, built with `go build -o bin/fakevllm ./bench/fakevllm`).
