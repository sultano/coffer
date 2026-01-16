.PHONY: setup build test fmt vet

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X github.com/sultano/coffer/cmd.version=$(VERSION) -X github.com/sultano/coffer/cmd.commit=$(COMMIT) -X github.com/sultano/coffer/cmd.date=$(DATE)"

# Setup git hooks and dependencies
setup:
	git config core.hooksPath .githooks
	go mod download
	@echo "Setup complete"

build:
	go build $(LDFLAGS) -o coffer .

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...
