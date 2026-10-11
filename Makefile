BINARY  := agent-one
CMD     := ./cmd/agent-one
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin

.PHONY: build install test check

build:
	go build -o .agent-one/tmp/$(BINARY) $(CMD)

install:
	go build -o $(BINDIR)/$(BINARY) $(CMD)

test:
	go test -short ./...

check: test build
