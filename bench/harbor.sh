#!/usr/bin/env bash
# bench/harbor.sh — the Harbor benchmark launcher: agent-one and OpenCode on the same tasks, the same
# gateway models and the same number of attempts, head to head. One Harbor job per (agent, model,
# task set), each in its own directory under bench/jobs/<run>/; every launch is appended to
# bench/runs.jsonl by record.py, which regenerates bench/RESULTS.md (with the agent × model table).
#
#   bench/harbor.sh --gateway https://gw.example/v1 --key-env GATEWAY_API_KEY --models kimi-k3,glm-5-3
#   bench/harbor.sh --dry-run ...          prints every command, runs none
#   bench/harbor.sh --run <name> ...       resumes an interrupted run (unfinished jobs only)
#
# Secrets travel by variable NAME only: the key is read from $<key-env> at run time, forwarded to
# Harbor's process and into the task container's environment, and never written to a file or
# printed. See bench/HARBOR.md.
set -euo pipefail

R=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
B="$R/bench"
DIST=${BENCH_DIST:-agent-one}

# ---- inputs (flags win over env) -----------------------------------------------------------------
GATEWAY=${BENCH_GATEWAY_URL:-}
KEY_ENV=${BENCH_KEY_ENV:-GATEWAY_API_KEY}
MODELS=${BENCH_MODELS:-}
AGENTS=${BENCH_AGENTS:-agent-one,opencode}
TASKS=${BENCH_TASKS:-}
DATASET=${BENCH_DATASET:-}
ATTEMPTS=${BENCH_ATTEMPTS:-1}
CONCURRENCY=${BENCH_CONCURRENCY:-1}
RUN=${BENCH_RUN:-}
PURPOSE=${BENCH_PURPOSE:-}
RUNNER=${BENCH_RUNNER:-auto}          # host | docker | auto
IMAGE=${HARBOR_IMAGE:-$DIST-harbor}
OPENCODE_PROVIDER=${BENCH_OPENCODE_PROVIDER:-compat}   # compat | openai
ALLOW_HOSTS=()
DRY=0
EXTRA=()

usage() {
  cat <<EOF
usage: bench/harbor.sh --gateway URL --key-env NAME --models a,b[,c] [options] [-- harbor run args]

  --gateway URL        OpenAI-compatible base URL, ending in /v1        (env BENCH_GATEWAY_URL)
  --key-env NAME       env var holding the gateway key; the NAME only   (env BENCH_KEY_ENV, default GATEWAY_API_KEY)
  --models a,b         model ids as the gateway's /v1/models lists them (env BENCH_MODELS)
  --agents a,b         agent-one, opencode, or both                     (env BENCH_AGENTS, default both)
  --tasks p1[,p2]      local task or dataset dirs, one job per entry    (env BENCH_TASKS, default bench/tasks)
  --dataset org/n@v    a registry dataset instead of --tasks            (env BENCH_DATASET)
  --attempts N         attempts per task, each agent, each model        (env BENCH_ATTEMPTS, default 1)
  --concurrency N      concurrent trials inside one Harbor job          (env BENCH_CONCURRENCY, default 1)
  --run NAME           the run's directory bench/jobs/NAME; an existing one is resumed
                                                                        (env BENCH_RUN, default a timestamp)
  --purpose TEXT       recorded with every launch                       (env BENCH_PURPOSE)
  --runner host|docker|auto   the harbor CLI on PATH, or the $IMAGE runner image (env BENCH_RUNNER)
  --opencode-provider compat|openai   how OpenCode reaches the gateway (env BENCH_OPENCODE_PROVIDER)
                       compat: an openai-compatible provider speaking /chat/completions (default)
                       openai: Harbor's built-in openai provider (OPENAI_BASE_URL / OPENAI_API_KEY)
  --allow-host H       let the agent reach H (hostname or IP/CIDR); repeatable
  --dry-run            print every command, run nothing; preflight failures become warnings
  --                   everything after it is passed to \`harbor run\` (e.g. --timeout-multiplier 2)
EOF
}

while [ $# -gt 0 ]; do
  case $1 in
    --gateway) GATEWAY=$2; shift 2 ;;
    --key-env) KEY_ENV=$2; shift 2 ;;
    --models) MODELS=$2; shift 2 ;;
    --agents) AGENTS=$2; shift 2 ;;
    --tasks) TASKS=$2; shift 2 ;;
    --dataset) DATASET=$2; shift 2 ;;
    --attempts) ATTEMPTS=$2; shift 2 ;;
    --concurrency) CONCURRENCY=$2; shift 2 ;;
    --run|--resume) RUN=$2; shift 2 ;;
    --purpose) PURPOSE=$2; shift 2 ;;
    --runner) RUNNER=$2; shift 2 ;;
    --opencode-provider) OPENCODE_PROVIDER=$2; shift 2 ;;
    --allow-host) ALLOW_HOSTS+=("$2"); shift 2 ;;
    --dry-run) DRY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    --) shift; EXTRA=("$@"); break ;;
    *) echo "harbor.sh: unknown flag $1 (see --help)" >&2; exit 2 ;;
  esac
done

# ---- helpers -------------------------------------------------------------------------------------
say() { printf '%s\n' "$*"; }
fail() { say "FAIL  $*" >&2; exit 2; }
FAILED=0
check() {  # check <ok|no> <message> — in --dry-run a failure is a warning, otherwise the exit
  local ok=$1; shift
  if [ "$ok" = ok ]; then say "ok    $*"; return 0; fi
  if [ "$DRY" = 1 ]; then say "warn  $* (dry-run: would fail)"; FAILED=1; return 0; fi
  fail "$*"
}
show() {  # show <args…> — print a command in a copy-pasteable form (the key never appears)
  local out="" a
  for a in "$@"; do
    case $a in *[!A-Za-z0-9_./:=@,+-]*|"") out+=" '${a//\'/\'\\\'\'}'" ;; *) out+=" $a" ;; esac
  done
  say "+${out}"
}
run() {  # run <args…> — print, then execute unless --dry-run
  show "$@"
  [ "$DRY" = 1 ] || "$@"
}
slug() { printf '%s' "$1" | tr '/@:' '---'; }
csv() { tr ',' '\n' <<<"$1" | sed '/^[[:space:]]*$/d'; }

# ---- argument validation -------------------------------------------------------------------------
[ -n "$GATEWAY" ] || { usage >&2; fail "--gateway is required (the OpenAI-compatible base URL, e.g. https://gw.example/v1)"; }
GATEWAY=${GATEWAY%/}
case $GATEWAY in */v1) ;; *) say "warn  --gateway does not end in /v1 ($GATEWAY); agent-one and OpenCode append /chat/completions to it" ;; esac
[[ $KEY_ENV =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || fail "--key-env must be a variable NAME (got '$KEY_ENV'); never pass the key itself"
[ -n "$MODELS" ] || { usage >&2; fail "--models is required: the ids the gateway serves, comma-separated (the example placeholders are muse-glimmer,kimi-k3,glm-5-3)"; }
mapfile -t MODEL_LIST < <(csv "$MODELS")
mapfile -t AGENT_LIST < <(csv "$AGENTS")
for a in "${AGENT_LIST[@]}"; do
  case $a in agent-one|opencode) ;; *) fail "--agents: unknown agent '$a' (agent-one, opencode)" ;; esac
done
[[ $ATTEMPTS =~ ^[1-9][0-9]*$ ]] || fail "--attempts must be a positive integer"
[[ $CONCURRENCY =~ ^[1-9][0-9]*$ ]] || fail "--concurrency must be a positive integer"
case $OPENCODE_PROVIDER in compat|openai) ;; *) fail "--opencode-provider must be compat or openai" ;; esac
case $RUNNER in host|docker|auto) ;; *) fail "--runner must be host, docker or auto" ;; esac
[ -z "$DATASET" ] || [ -z "$TASKS" ] || fail "--dataset and --tasks exclude each other"

# The task sets: each entry is one Harbor job per agent and model.
TASK_KIND=path
TASK_LIST=()
if [ -n "$DATASET" ]; then
  TASK_KIND=dataset
  TASK_LIST=("$DATASET")
else
  mapfile -t raw < <(csv "${TASKS:-$B/tasks}")
  for t in "${raw[@]}"; do
    [ -d "$t" ] || fail "--tasks: $t is not a directory (a Harbor task dir, or a dir of task dirs)"
    TASK_LIST+=("$(cd "$t" && pwd)")
  done
fi

RUN=${RUN:-$(date +%Y%m%d-%H%M%S)}
case $RUN in /*) RUN_DIR=$RUN; RUN=$(basename "$RUN") ;; *) RUN_DIR="$B/jobs/$RUN" ;; esac
PURPOSE=${PURPOSE:-"harbor $RUN: ${AGENTS} × ${MODELS} × ${ATTEMPTS} attempt(s)"}

# ---- preflight -----------------------------------------------------------------------------------
say "== preflight"
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  check ok "docker: $(docker version --format '{{.Server.Version}}' 2>/dev/null || echo daemon reachable)"
else
  check no "docker: not found or the daemon is unreachable — install Docker Engine (https://docs.docker.com/engine/install/) and make sure your user can use /var/run/docker.sock"
fi

if [ "$RUNNER" = auto ]; then
  if command -v harbor >/dev/null 2>&1; then RUNNER=host
  elif docker image inspect "$IMAGE" >/dev/null 2>&1; then RUNNER=docker
  fi
fi
case $RUNNER in
  host)
    if command -v harbor >/dev/null 2>&1; then check ok "harbor: $(command -v harbor) ($(harbor --version 2>/dev/null | head -1 || echo version unknown))"
    else check no "harbor: not on PATH — uv tool install harbor   (or: pip install harbor)   — or --runner docker with the $IMAGE image"; fi ;;
  docker)
    if docker image inspect "$IMAGE" >/dev/null 2>&1; then check ok "harbor: the $IMAGE runner image"
    else check no "harbor: the runner image $IMAGE is missing — docker build -t $IMAGE $B/harbor"; fi ;;
  auto)
    check no "harbor: not installed. Either: uv tool install harbor   (or: pip install harbor)   — or build the runner image: docker build -t $IMAGE $B/harbor" ;;
esac

command -v python3 >/dev/null 2>&1 && check ok "python3: $(python3 --version 2>&1)" || check no "python3: not found (record.py needs it)"

BIN="$R/bin/$DIST"
if printf '%s\n' "${AGENT_LIST[@]}" | grep -qx agent-one; then
  if [ -x "$BIN" ]; then
    check ok "$DIST: $BIN ($("$BIN" version 2>/dev/null | head -1 || echo version unknown))"
    if command -v ldd >/dev/null 2>&1 && ldd "$BIN" 2>/dev/null | grep -q '=>'; then
      say "warn  $BIN is dynamically linked; task images may lack its libraries — build it with CGO_ENABLED=0 GOOS=linux GOARCH=amd64"
    fi
  else
    check no "$DIST: $BIN is missing — CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/$DIST ./cmd/$DIST"
  fi
fi

KEY_VALUE=${!KEY_ENV:-}
if [ -n "$KEY_VALUE" ]; then check ok "$KEY_ENV: set (its value is never printed or written)"
else check no "$KEY_ENV: not set — export $KEY_ENV=…  (pass another name with --key-env)"; fi

if command -v curl >/dev/null 2>&1; then
  # The key goes to curl through its config on stdin, never on the command line.
  if body=$(printf 'header = "Authorization: Bearer %s"\n' "$KEY_VALUE" | curl -sS -m 15 -f -K - "$GATEWAY/models" 2>&1); then
    served=$(printf '%s' "$body" | python3 -c 'import json,sys
try: print(" ".join(m.get("id","?") for m in json.load(sys.stdin).get("data",[])))
except Exception: print("(unparsed)")' 2>/dev/null || echo "(unparsed)")
    check ok "gateway: GET $GATEWAY/models → serves: $served"
    for m in "${MODEL_LIST[@]}"; do
      case " $served " in *" $m "*) ;; *) say "warn  gateway does not list model '$m' (check --models against the ids above)" ;; esac
    done
  else
    check no "gateway: GET $GATEWAY/models failed: ${body:-no response} — check --gateway, the network, and $KEY_ENV"
  fi
else
  check no "curl: not found (needed to probe the gateway)"
fi
unset KEY_VALUE

[ "$RUNNER" = docker ] && [ -n "$DATASET" ] && say "warn  the docker runner downloads registry datasets into the container; prefer --runner host for --dataset"
# The docker runner writes Harbor's job dirs as root; the launcher only needs its own run dir writable.
parent=$RUN_DIR; while [ ! -e "$parent" ]; do parent=$(dirname "$parent"); done
if [ -w "$parent" ]; then check ok "run dir: $RUN_DIR (writable)"
else check no "run dir: cannot write under $parent (owned by $(stat -c %U "$parent" 2>/dev/null || echo ?), the docker runner's doing) — sudo chown -R $(id -un) $parent, or pass an absolute --run elsewhere"; fi
[ "$FAILED" = 0 ] || say "== preflight had failures; continuing because of --dry-run"

# ---- the run directory and the agent's config ----------------------------------------------------
say "== run $RUN → $RUN_DIR"
CONFIG="$RUN_DIR/$DIST.yaml"
render_config() {
  sed -e "s#__GATEWAY_URL__#$GATEWAY#" -e "s#__KEY_ENV__#$KEY_ENV#" -e "s#__DEFAULT_MODEL__#${MODEL_LIST[0]}#" \
    "$B/configs/gateway.yaml"
}
if [ "$DRY" = 1 ]; then
  say "+ mkdir -p $RUN_DIR && render $B/configs/gateway.yaml → $CONFIG:"
  render_config | sed 's/^/    /'
else
  mkdir -p "$RUN_DIR"
  render_config > "$CONFIG"
  say "wrote $CONFIG"
fi

# The adapter (bench/harbor/harbor_agent.py) reads these from Harbor's process environment; the
# key is forwarded by name and reaches the task container's environment only.
export PYTHONPATH="$B/harbor${PYTHONPATH:+:$PYTHONPATH}"
export BENCH_DIST="$DIST" BENCH_BIN="$BIN" BENCH_CONFIG="$CONFIG" BENCH_FORWARD_ENV="$KEY_ENV"
if [ "$OPENCODE_PROVIDER" = openai ]; then
  export OPENAI_BASE_URL="$GATEWAY"
  [ "$DRY" = 1 ] || export OPENAI_API_KEY="${!KEY_ENV:-}"
fi

# harbor <args…> through the chosen runner. With the docker runner the repository is mounted at its
# own absolute path, so the paths Harbor hands to the Docker daemon resolve on the host.
harbor_cmd() {
  if [ "$RUNNER" = docker ]; then
    local mounts=(-v /var/run/docker.sock:/var/run/docker.sock -v "$R:$R")
    for t in "${TASK_LIST[@]}" "$RUN_DIR"; do case $t in "$R"/*) ;; /*) mounts+=(-v "$t:$t") ;; esac; done
    printf '%s\n' docker run --rm "${mounts[@]}" -w "$B" --group-add "$(stat -c %g /var/run/docker.sock 2>/dev/null || echo 0)" \
      -e PYTHONPATH -e BENCH_DIST -e BENCH_BIN -e BENCH_CONFIG -e BENCH_FORWARD_ENV -e "$KEY_ENV" \
      -e OPENAI_BASE_URL -e OPENAI_API_KEY "$IMAGE"
  else
    printf '%s\n' harbor
  fi
}
mapfile -t HARBOR < <(harbor_cmd)

# write_job <file> <name> <agent> <model> <task> — one Harbor job config: agent, model, task set, attempts.
write_job() {
  local file=$1 name=$2 agent=$3 model=$4 task=$5
  {
    echo "# Harbor job config written by bench/harbor.sh — the key is referenced by name, never by value."
    echo "job_name: $name"
    echo "jobs_dir: $RUN_DIR"
    echo "n_attempts: $ATTEMPTS"
    echo "n_concurrent_trials: $CONCURRENCY"
    echo "environment: { type: docker, delete: true }"
    echo "agents:"
    case $agent in
      agent-one)
        echo "  - name: harbor_agent:HarborAgent      # bench/harbor/harbor_agent.py, PYTHONPATH=bench/harbor"
        echo "    model_name: gateway/$model" ;;
      opencode)
        echo "  - name: opencode"
        if [ "$OPENCODE_PROVIDER" = openai ]; then
          echo "    model_name: openai/$model             # OPENAI_BASE_URL and OPENAI_API_KEY from Harbor's environment"
        else
          echo "    model_name: gateway/$model"
          echo "    env: { $KEY_ENV: \"\${$KEY_ENV}\" }    # resolved by Harbor from its environment at run time"
          echo "    kwargs:"
          echo "      opencode_config:"
          echo "        provider:"
          echo "          gateway:"
          echo "            npm: \"@ai-sdk/openai-compatible\""
          echo "            name: gateway"
          echo "            options: { baseURL: \"$GATEWAY\", apiKey: \"{env:$KEY_ENV}\" }"
          echo "            models:"
          echo "              $model: { name: \"$model\" }"
        fi ;;
    esac
    echo "datasets:"
    if [ "$TASK_KIND" = dataset ]; then
      echo "  - name: ${task%@*}"
      case $task in *@*) echo "    version: \"${task##*@}\"" ;; esac
    else
      echo "  - path: $task"
    fi
  } > "$file"
}

# ---- the jobs ------------------------------------------------------------------------------------
say "== jobs: ${#AGENT_LIST[@]} agent(s) × ${#MODEL_LIST[@]} model(s) × ${#TASK_LIST[@]} task set(s), $ATTEMPTS attempt(s) each"
HARBOR_FLAGS=(-y)
for h in "${ALLOW_HOSTS[@]+"${ALLOW_HOSTS[@]}"}"; do HARBOR_FLAGS+=(--allow-agent-host "$h"); done
HARBOR_FLAGS+=("${EXTRA[@]+"${EXTRA[@]}"}")
rc=0
for task in "${TASK_LIST[@]}"; do
  for model in "${MODEL_LIST[@]}"; do
    for agent in "${AGENT_LIST[@]}"; do
      name="${agent}__$(slug "$model")__$(slug "$(basename "$task")")"
      dir="$RUN_DIR/$name"
      yaml="$RUN_DIR/$name.yaml"
      say "-- $name"
      if [ -f "$RUN_DIR/$name.recorded" ]; then   # the mark lives beside the job: its dir may be root's
        say "   already recorded, skipping"
        continue
      fi
      if [ -f "$dir/result.json" ]; then
        say "   finished but not recorded, recording"
      elif [ -f "$dir/config.json" ]; then
        say "   unfinished, resuming"
        run "${HARBOR[@]}" jobs resume -p "$dir" || { say "   harbor resume failed (rc $?)"; rc=1; continue; }
      else
        if [ "$DRY" = 1 ]; then
          say "+ write $yaml:"
          write_job /dev/stdout "$name" "$agent" "$model" "$task" | sed 's/^/    /'
        else
          write_job "$yaml" "$name" "$agent" "$model" "$task"
        fi
        run "${HARBOR[@]}" run -c "$yaml" "${HARBOR_FLAGS[@]}" || { say "   harbor run failed (rc $?); rerun with --run $RUN to resume"; rc=1; continue; }
      fi
      if [ "$DRY" = 1 ]; then
        show python3 "$B/record.py" "$dir" --run "$RUN" --purpose "$PURPOSE"
      elif python3 "$B/record.py" "$dir" --run "$RUN" --purpose "$PURPOSE"; then
        touch "$RUN_DIR/$name.recorded"
      else
        say "   record.py failed for $dir"; rc=1
      fi
    done
  done
done

say "== done: results in $RUN_DIR; the table in $B/RESULTS.md (from $B/runs.jsonl)"
[ "$rc" = 0 ] || say "== some jobs failed; rerun the same command with --run $RUN to resume them"
exit $rc
