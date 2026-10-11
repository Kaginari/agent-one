BINARY  := agent-one
CMD     := ./cmd/agent-one
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin

.PHONY: build install test check docker-image docker-dash

build:
	go build -o .agent-one/tmp/$(BINARY) $(CMD)

install:
	go build -o $(BINDIR)/$(BINARY) $(CMD)

test:
	go test -short ./...

check: test build

# The container lane. GOPROXY carries its estate credentials: export it or register ./.env
# (chmod 600, gitignored).
docker-image:
	@set -a; [ -f ./.env ] && . ./.env || true; set +a; \
	test -n "$$GOPROXY" || { echo "GOPROXY unset — export it (estate creds) or register ./.env"; exit 1; }; \
	docker build --build-arg GOPROXY="$$GOPROXY" -t $(BINARY):local .

# The dashboard session through Docker: the workspace mounts at /work (a .agent-one/config.local.yaml
# apiKey literal resolves inside it — no -e needed); host networking because the dash binds
# 127.0.0.1 by design. → http://127.0.0.1:7411/dash  (ctrl-c stops)
docker-dash: docker-image
	docker run --rm --network host -v "$$PWD":/work -w /work $(BINARY):local dash --open

