#!/usr/bin/env python3
"""Append one Harbor job to bench/runs.jsonl and regenerate bench/RESULTS.md.

    bench/record.py <jobs/JOB_DIR> --purpose "<why this launch>" [--expect <mean>] [--run <name>]
    bench/record.py --render          regenerate RESULTS.md from runs.jsonl, appending nothing

--expect makes it an assertion: a mean reward other than the expected one exits 1 (used by the
smoke test). --run names the launch's run (bench/harbor.sh passes its run id): launches sharing a
run are set side by side in RESULTS.md, agent × model. runs.jsonl is append-only; RESULTS.md is
generated from it and never hand-edited.
"""

import argparse
import json
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

BENCH = Path(__file__).resolve().parent
HEAD_TO_HEAD_RUNS = 5  # the most recent runs get an agent × model table


def git(*args: str) -> str:
    try:
        return subprocess.run(["git", *args], cwd=BENCH, capture_output=True, text=True, check=True).stdout.strip()
    except (subprocess.CalledProcessError, FileNotFoundError):
        return "unknown"


def summarize(job_dir: Path, purpose: str, run: str | None) -> dict:
    r = json.loads((job_dir / "result.json").read_text())
    (name, ev), *more = r["stats"]["evals"].items()
    if more:
        raise SystemExit(f"{job_dir}: one agent/dataset per job expected, got {len(more) + 1}")
    parts = name.split("__")  # agent__dataset, or agent__model__dataset when a model is set
    agent, dataset = parts[0], parts[-1] if len(parts) > 1 else ""
    model = None
    cfg = job_dir / "config.json"
    if cfg.exists():
        c = json.loads(cfg.read_text())
        agents = c.get("agents") or []
        if agents:
            model = agents[0].get("model_name")
    s = r["stats"]
    line = {
        "ts": r["started_at"],
        "job": job_dir.name,
        "repo": git("rev-parse", "--short", "HEAD"),
        "dirty": bool(git("status", "--porcelain")),
        "agent": agent,
        "model": model,
        "dataset": dataset or "adhoc",
        "trials": r["n_total_trials"],
        "errors": ev.get("n_errors", 0),
        "mean_reward": (ev.get("metrics") or [{}])[0].get("mean"),
        "n_input_tokens": s.get("n_input_tokens"),
        "n_cache_tokens": s.get("n_cache_tokens"),
        "n_output_tokens": s.get("n_output_tokens"),
        "cost_usd": s.get("cost_usd"),
        "purpose": purpose,
    }
    if run:
        line["run"] = run
    return line


def bare_model(model: str | None) -> str:
    """The model id without its provider prefix: gateway/kimi-k3 and openai/kimi-k3 are one model."""
    if not model:
        return "—"
    return model.split("/", 1)[1] if "/" in model else model


def _tok(n: int | None) -> str:
    if n is None:
        return "—"
    return f"{n / 1000:.1f}k" if n >= 1000 else str(n)


def head_to_head(runs: list[dict]) -> list[str]:
    """One agent × model table per run id (most recent first), pass rate weighted by trials."""
    by_run: dict[str, list[dict]] = defaultdict(list)
    for r in runs:
        if r.get("run"):
            by_run[r["run"]].append(r)
    if not by_run:
        return []
    order = sorted(by_run, key=lambda k: max(x.get("ts", "") for x in by_run[k]), reverse=True)
    out: list[str] = []
    for run in order[:HEAD_TO_HEAD_RUNS]:
        lines = by_run[run]
        agents = sorted({x["agent"] for x in lines})
        models = sorted({bare_model(x.get("model")) for x in lines})
        cells: dict[tuple[str, str], dict] = {}
        for x in lines:
            key = (bare_model(x.get("model")), x["agent"])
            c = cells.setdefault(key, {"trials": 0, "reward": 0.0, "errors": 0, "in": 0, "out": 0, "cost": 0.0,
                                       "in_known": True, "out_known": True, "cost_known": True, "reward_known": True})
            t = x.get("trials") or 0
            c["trials"] += t
            c["errors"] += x.get("errors") or 0
            if x.get("mean_reward") is None:
                c["reward_known"] = False
            else:
                c["reward"] += x["mean_reward"] * t
            for src, dst in (("n_input_tokens", "in"), ("n_output_tokens", "out"), ("cost_usd", "cost")):
                if x.get(src) is None:
                    c[dst + "_known"] = False
                else:
                    c[dst] += x[src]
        purpose = next((x.get("purpose") for x in lines if x.get("purpose")), "")
        out += [f"### Run `{run}`", "", f"{purpose}  ", "",
                "Each cell: pass rate (trials) · errors · input/output tokens · cost $. Tokens and cost show only when every launch in the cell reported them.",
                "", "| model | " + " | ".join(agents) + " |", "|---|" + "---|" * len(agents)]
        for m in models:
            row = [f"`{m}`"]
            for a in agents:
                c = cells.get((m, a))
                if not c:
                    row.append("—")
                    continue
                rate = f"{c['reward'] / c['trials']:.2f}" if c["reward_known"] and c["trials"] else "—"
                toks = f"{_tok(c['in'])}/{_tok(c['out'])}" if c["in_known"] and c["out_known"] else "—"
                cost = f"{c['cost']:.4f}" if c["cost_known"] else "—"
                row.append(f"**{rate}** ({c['trials']}) · {c['errors']} err · {toks} · {cost}")
            out.append("| " + " | ".join(row) + " |")
        out.append("")
    return out


def render(runs: list[dict]) -> str:
    def cell(v):
        return "—" if v is None else (f"{v:.4f}" if isinstance(v, float) else str(v))

    out = ["# Benchmark results", "", "Generated from `runs.jsonl` by `record.py` — do not edit.", ""]
    h2h = head_to_head(runs)
    if h2h:
        out += ["## Head to head", "", *h2h]
    out += ["## Every launch", "",
            "| when | run | agent | model | dataset | trials | mean reward | errors | input tok | output tok | cost $ | commit | purpose |",
            "|---|---|---|---|---|---|---|---|---|---|---|---|---|"]
    for r in reversed(runs):
        commit = r.get("repo", "?") + ("+dirty" if r.get("dirty") else "")
        out.append("| " + " | ".join(cell(x) for x in [
            r.get("ts", "")[:16].replace("T", " "), r.get("run"), r.get("agent"), r.get("model"), r.get("dataset"),
            r.get("trials"), r.get("mean_reward"), r.get("errors"), r.get("n_input_tokens"),
            r.get("n_output_tokens"), r.get("cost_usd"), commit, r.get("purpose")]) + " |")
    return "\n".join(out) + "\n"


def load_runs(runs_path: Path) -> list[dict]:
    if not runs_path.exists():
        return []
    return [json.loads(x) for x in runs_path.read_text().splitlines() if x.strip()]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("job_dir", type=Path, nargs="?")
    ap.add_argument("--purpose")
    ap.add_argument("--expect", type=float)
    ap.add_argument("--run", help="the run this launch belongs to (bench/harbor.sh's run id)")
    ap.add_argument("--render", action="store_true", help="regenerate RESULTS.md only")
    a = ap.parse_args()
    runs_path = BENCH / "runs.jsonl"
    if a.render:
        if a.job_dir:
            ap.error("--render takes no job directory")
        (BENCH / "RESULTS.md").write_text(render(load_runs(runs_path)))
        return 0
    if not a.job_dir or not a.purpose:
        ap.error("a job directory and --purpose are required (or --render)")
    line = summarize(a.job_dir, a.purpose, a.run)
    with runs_path.open("a") as f:
        f.write(json.dumps(line) + "\n")
    (BENCH / "RESULTS.md").write_text(render(load_runs(runs_path)))
    print(json.dumps(line))
    if a.expect is not None and line["mean_reward"] != a.expect:
        print(f"expected mean reward {a.expect}, got {line['mean_reward']}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
