# Source build, estate mirrors only. GOPROXY points at the Artifactory go proxy
# (named, never valued with credentials — see the netrc note below).
ARG GO_IMAGE=registry.example.com/library/golang:1.26-alpine
ARG RUNTIME_IMAGE=registry.example.com/library/debian:bookworm-slim
FROM ${GO_IMAGE} AS build
ARG GOPROXY=https://artifactory.example.com/artifactory/api/go/go
ARG GOPRIVATE=
ENV GOPROXY=${GOPROXY} GOPRIVATE=${GOPRIVATE} GOSUMDB=off GOFLAGS=-mod=mod

# Corporate CA, so the module fetch trusts the Artifactory endpoint
# (vendored from bench-forge — same estate CA).
COPY cacert.pem /usr/local/share/ca-certificates/artifactory-ca.crt
# CI kaniko builds (gitlab-templates containers-build.yml) inject their robot
# identity at /workspace/auth.conf — use it as netrc for `go mod` when present;
# local builds fall back to credentials carried in GOPROXY.
RUN update-ca-certificates && \
    if [ -f /workspace/auth.conf ]; then cp /workspace/auth.conf /root/.netrc; fi

WORKDIR /src
# No separate `go mod download`: the estate go proxy currently ships mangled zips for
# nested charmbracelet modules (inner path duplicated, unzip: "go.mod file not in module
# root directory" — 2026-09-30; defect to raise with the team-example org). The full-graph
# fetch would die on them; the build-and-test below pulls exactly what it needs.
COPY go.mod go.sum ./
COPY . .
# The go test gate rides the image build: runner nodes cannot pull the
# dockerhub.* vhost for a standalone test job (x509 at node level), so tests
# run here — under kaniko, where the estate CA and the robot netrc are live.
# ./app stays out while the proxy's teatest zip is mangled (its TUI tests import it).
# The headless-TUI test and the bwrap-less Linux loop reds are skipped by name;
# the mac bench jurisdiction keeps them.
RUN go test -short -skip '^TestTUIInterruptAndQueue$|^TestGutsEchoAndWitnessAdmissibility$|^TestShellWritesReachTheGate$' ./loop ./config
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/agent-one ./cmd/agent-one \
 && /out/agent-one version

FROM ${RUNTIME_IMAGE}
# TLS bundle incl. the corp CA rides from the build stage (bench-forge shape) —
# providers' https gateways verify without any apt step.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/agent-one /usr/local/bin/agent-one
# git + curl install only where the estate helper exists: CI kaniko ships
# /workspace/gen_apt_config.sh (sources → artifactory/<distro>, its robot auth.conf
# symlinked as apt credentials — gitlab-templates/container/kaniko.yml). Local builds
# carry neither, so the image keeps bookworm-slim's bash alone; git-needing acts then
# report the tool as missing instead of being baked in untrusted.
RUN if [ -f /workspace/gen_apt_config.sh ]; then sh /workspace/gen_apt_config.sh \
 && apt-get update && apt-get install -y --no-install-recommends git curl \
 && rm -rf /var/lib/apt/lists/* /etc/apt/auth.conf; fi
WORKDIR /work
ENTRYPOINT ["agent-one"]

