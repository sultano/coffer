.PHONY: setup build test fmt vet

# Setup git hooks and dependencies
setup:
	git config core.hooksPath .githooks
	go mod download
	@echo "Setup complete"

build:
	go build ./...

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...
