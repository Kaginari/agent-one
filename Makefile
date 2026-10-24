BINARY  := agent-one
CMD     := ./cmd/agent-one
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin

# The binary says what it was built from: the same three stamps GoReleaser sets on a release.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build install test check

build:
	go build -ldflags "$(LDFLAGS)" -o .agent-one/tmp/$(BINARY) $(CMD)

install:
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(BINARY) $(CMD)

test:
	go test -short ./...

check: test build
