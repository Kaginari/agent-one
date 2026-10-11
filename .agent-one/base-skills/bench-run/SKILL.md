---
name: bench-run
description: Running the benchmark lanes — the fake-vllm stand-in (bench/fakevllm), the terminal-bench-2 task slice (bench/tasks-tb2), the harbor lane (bench/harbor), and the shell drivers. For bench zone workers and service-release when results land in bench/RESULTS.md.
---

# bench-run

- Three lanes: smoke.sh (bench driver + fakevllm fake OpenAI server, no-cost), harbor (full terminal-bench-2 via docker), tasks-tb2 (a small set of task dirs sliced for agent-one). harbor.sh is the manual lane.
- bench/fakevllm/ is a separate module (own go.mod, module fakevllm); build inside it: `cd bench/fakevllm && go build -o fakevllm .` — the fakevllm binary is gitignored at bench/fakevllm/fakevllm.
- bench/smoke.sh is run in CI every commit; needs `bin/agent-one` prebuilt (or runs via CGO_ENABLED=0 go build -o bin/agent-one ./cmd/agent-one first — bin/ is gitignored). Outcome writes bench/runs.jsonl; smoke_config.yaml defaults to the fake server.
- bench/harbor.sh + bench/harbor/ (Dockerfile + harbor_agent.py + harbor_opencode.py) runs against real gateway models — uses docker (present on this machine, but verify lines must not depend on it — true bench runs are human-initiated).
- Tasks come from bench/tasks-tb2/ and bench/tasks/ — each is a task dir with task.toml. tb2-base holds the shared harness. Tests are graded against terminal-bench-2 rubrics.
- Rollup into results via bench/record.py + bench/RESULTS.md. RESULTS.md is human-curated; agents append via record.py, never edit RESULTS.md by hand.
- Note: -short skips bench entirely. `make check` runs tests then build; smoke.sh is separate. The verify line for bench lanes is build-only; leave actual runs to humans/CI.

