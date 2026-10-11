# The full runtime for --docker-agent: the harness binary is not in the image (it is mounted
# read-only from the host), so this is the machine the agent works in — the toolchains a coding
# agent reaches for, ready offline once pulled.
#
#   docker build -t <repo>:<tag> docker/runtime
#   agent-one --docker-agent --image <repo>:<tag>      (or registry.containers.image in config)
#
# The session runs as your host uid with HOME a fresh tmpfs, so everything a tool writes for
# itself goes under /tmp (GOPATH, npm's global prefix, pip's user base) — writable by any uid,
# gone with the container. The world itself is the bind-mounted project.
ARG BASE=debian:bookworm-slim
FROM ${BASE}

ARG GO_VERSION=1.27.1
ARG NODE_VERSION=24.21.0
ARG TARGETARCH=amd64

# the base system: shells and text tools, build essentials, python, a headless browser for
# page checks, network and archive tools
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      bash ca-certificates coreutils curl wget diffutils file findutils gawk git git-lfs grep jq less \
      make patch procps sed tar unzip zip xz-utils bzip2 gzip rsync tree tzdata locales \
      openssh-client gnupg dnsutils iputils-ping netcat-openbsd \
      build-essential pkg-config cmake autoconf automake libtool \
      libssl-dev zlib1g-dev libffi-dev libsqlite3-dev sqlite3 \
      python3 python3-dev python3-pip python3-venv pipx \
      ripgrep fd-find shellcheck yq \
      chromium fonts-dejavu-core \
 && ln -s /usr/bin/fdfind /usr/local/bin/fd \
 && ln -s /usr/bin/python3 /usr/local/bin/python \
 && rm -rf /var/lib/apt/lists/*

# Go, from the official tarball
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" | tar -xz -C /usr/local

# Node.js LTS, from the official tarball, with pnpm and yarn installed at build time (corepack
# would fetch them at run time, into a dir the session's uid cannot write)
RUN case "$TARGETARCH" in amd64) a=x64 ;; arm64) a=arm64 ;; *) a=$TARGETARCH ;; esac \
 && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${a}.tar.xz" \
    | tar -xJ -C /usr/local --strip-components=1 \
 && npm install -g --no-fund --no-audit pnpm yarn \
 && rm -rf /root/.npm

# Python's fast installer and the tools agents run on Python projects
RUN pip install --no-cache-dir --break-system-packages uv ruff pytest

# Go's everyday tools, built once into /usr/local/bin
RUN GOBIN=/usr/local/bin GOPATH=/tmp/gobuild GOCACHE=/tmp/gocache /usr/local/go/bin/go install golang.org/x/tools/gopls@latest \
 && GOBIN=/usr/local/bin GOPATH=/tmp/gobuild GOCACHE=/tmp/gocache /usr/local/go/bin/go install golang.org/x/tools/cmd/goimports@latest \
 && GOBIN=/usr/local/bin GOPATH=/tmp/gobuild GOCACHE=/tmp/gocache /usr/local/go/bin/go install honnef.co/go/tools/cmd/staticcheck@latest \
 && rm -rf /tmp/gobuild /tmp/gocache

ENV LANG=C.UTF-8 \
    GOPATH=/tmp/go \
    GOCACHE=/tmp/go-cache \
    GOMODCACHE=/tmp/go/pkg/mod \
    NPM_CONFIG_PREFIX=/tmp/npm-global \
    NPM_CONFIG_CACHE=/tmp/npm-cache \
    PYTHONUSERBASE=/tmp/py-user \
    PIP_BREAK_SYSTEM_PACKAGES=1 \
    PIP_CACHE_DIR=/tmp/pip-cache \
    UV_CACHE_DIR=/tmp/uv-cache \
    CHROME_BIN=/usr/bin/chromium \
    PATH=/tmp/npm-global/bin:/tmp/go/bin:/tmp/py-user/bin:/usr/local/go/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin

LABEL org.opencontainers.image.title="kaginari runtime" \
      org.opencontainers.image.description="The full runtime for agent-one --docker-agent: Go, Node.js, Python, build tools, a headless Chromium."
