#!/usr/bin/env bash
# bench/tb2-base/build.sh — bake the tb2 slice images: each alexgshaw prebuilt (pulled from the
# docker proxy host) wrapped by tb2-base/Dockerfile with node+opencode and the verifier toolchain
# (curl, uv, pytest + ctrf plugin, the task's pins) — everything from Artifactory, nothing from
# the open net. Each task bakes once and tags both task dirs' images:
#
#   agent-one-tb2-agent-one/<task>:20251031   (the agent-one tree's task.toml points here)
#   agent-one-tb2-opencode/<task>:20251031
#
# Secrets in, never out: the npm token is lifted from ~/.npmrc into a one-line temp file (deleted
# on exit), the machine's pip.conf rides a BuildKit secret — no value is printed, cached or layered.
#
#   bench/tb2-base/build.sh             # bake all six
#   bench/tb2-base/build.sh fix-git …   # bake just these
set -euo pipefail

B=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DOCKER_PROXY=registry.example.com/alexgshaw
TAG=20251031
TASKS=${@:-"bn-fit-modify constraints-scheduling feal-differential-cryptanalysis filter-js-from-html fix-git gcode-to-text"}

# The verifier's per-task pins (from each tests/test.sh, minus the always-baked pytest pair).
extra_pip() { case $1 in
  bn-fit-modify) echo "pandas==2.3.2 scipy==1.16.1" ;;
  feal-differential-cryptanalysis) echo "setuptools==80.9.0" ;;
  filter-js-from-html) echo "selenium==4.38.0 bs4==0.0.2" ;;
  gcode-to-text) echo "matplotlib==3.10.7 pytesseract==0.3.13 pillow==12.0.0 opencv-python==4.12.0.88" ;;
  *) echo "" ;; esac; }
extra_apt() { case $1 in
  feal-differential-cryptanalysis) echo "gcc libc6-dev" ;;   # libc6-dev: --no-install-recommends skips gcc's
  gcode-to-text) echo "python3-opencv tesseract-ocr" ;;   # system libs cv2 needs + the tesseract binary
  *) echo "" ;; esac; }

PIPCONF=${PIPCONF:-$HOME/.config/pip/pip.conf}
[ -f "$PIPCONF" ] || { echo "build.sh: pip.conf not found at $PIPCONF (PIPConfig env override: PIPCONF)" >&2; exit 2; }
TOK=$(mktemp -t npmtok.XXXXXX); APT=$(mktemp -t aptconf.XXXXXX)
chmod 600 "$TOK" "$APT"; trap 'rm -f "$TOK" "$APT"' EXIT
sed -n 's/.*_authToken=//p' "$HOME/.npmrc" | head -1 > "$TOK"
[ -s "$TOK" ] || { echo "build.sh: no _authToken in ~/.npmrc (the Artifactory npm token)" >&2; exit 2; }
# The apt identity comes from pip.conf's inline credentials — value into the temp file only.
python3 - "$PIPCONF" "$APT" <<'EOF'
import configparser, urllib.parse, sys
c = configparser.ConfigParser(); c.read(sys.argv[1])
url = c["global"].get("index-url") or c["global"].get("index")
p = urllib.parse.urlparse(url)
open(sys.argv[2], "w").write(
    "machine %s\nlogin %s\npassword %s\n"
    % (p.hostname, urllib.parse.unquote(p.username or ""), urllib.parse.unquote(p.password or "")))
EOF
[ -s "$APT" ] || { echo "build.sh: could not derive apt.conf from pip.conf (inline credentials expected)" >&2; exit 2; }

docker image inspect agent-one-bench-base:node22-opencode-amd64 >/dev/null 2>&1 \
  || { echo "build.sh: agent-one-bench-base:node22-opencode-amd64 missing — build bench/harbor/base first" >&2; exit 2; }

# Stage the bench base's node+opencode into the build context (see tb2-base/Dockerfile for why this
# is not a second FROM). Idempotent; delete tb2-base/.stage to re-stage.
if [ ! -d "$B/tb2-base/.stage/node_modules" ]; then
  echo "== staging node+opencode from agent-one-bench-base into tb2-base/.stage"
  cid=$(docker create --platform linux/amd64 agent-one-bench-base:node22-opencode-amd64)
  mkdir -p "$B/tb2-base/.stage"
  docker cp "$cid":/usr/local/bin/node "$B/tb2-base/.stage/node"
  docker cp "$cid":/usr/local/lib/node_modules "$B/tb2-base/.stage/node_modules"
  docker rm "$cid" >/dev/null
fi

rc=0
# filter-js-from-html's verifier downloads the XSS testbed zip from github; on this bench net that
# fetch is dead. The zip is vendored (fetch it once from the host, e.g. via a git clone of
# davidwagner/html-sanitizer-testbed re-zipped with top dir html-sanitizer-testbed-master) into
# .stage-payload/testbed.zip beside the build for that one task; the test prefers the baked copy.
for t in $TASKS; do
  echo "== $t (pip: $(extra_pip "$t") | apt: $(extra_apt "$t") | tags: agent-one-tb2-{agent-one,opencode}/$t:$TAG)"
  rm -rf "$B/tb2-base/.stage-payload"; mkdir -p "$B/tb2-base/.stage-payload"
  PAYLOAD=${PAYLOAD_XSS_TESTBED:-}
  if [ "$t" = filter-js-from-html ]; then
    [ -n "$PAYLOAD" ] || PAYLOAD="$B/tb2-base/.stage-payload-store/testbed.zip"
    if [ -f "$PAYLOAD" ]; then cp "$PAYLOAD" "$B/tb2-base/.stage-payload/testbed.zip"
    else echo "  warn: no vendored testbed.zip (set \$PAYLOAD_XSS_TESTBED) — filter-js's second test needs the open net" >&2; fi
  fi
  # Per-task offline bakes (tb2-base/task-setup/<task>.sh): e.g. R packages the reference solution
  # would install from CRAN at run time, unreachable here — installed from the Artifactory CRAN
  # remote at build time instead, identity via the pip.conf secret.
  rm -rf "$B/tb2-base/.stage-task"; mkdir -p "$B/tb2-base/.stage-task"
  [ -f "$B/tb2-base/task-setup/$t.sh" ] && cp "$B/tb2-base/task-setup/$t.sh" "$B/tb2-base/.stage-task/setup.sh" || true
  docker build \
    --secret id=npmtoken,src="$TOK" \
    --secret id=pipconf,src="$PIPCONF" \
    --secret id=aptconf,src="$APT" \
    --build-arg TASK_IMAGE="$DOCKER_PROXY/$t:$TAG" \
    --build-arg EXTRA_PIP="$(extra_pip "$t")" \
    --build-arg EXTRA_APT="$(extra_apt "$t")" \
    -t "agent-one-tb2-agent-one/$t:$TAG" \
    -t "agent-one-tb2-opencode/$t:$TAG" \
    "$B/tb2-base" || rc=1
done
[ "$rc" = 0 ] && echo "== all images baked" || echo "== some bakes failed (rc $rc)" >&2
exit $rc

