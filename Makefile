BIN     := bin/gitlab-tui
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build test lint fmt run tidy hooks confidential

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/gitlab-tui

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

run: build
	./$(BIN) $(ARGS)

tidy:
	go mod tidy

# Git hooks that block confidential data (see CLAUDE.md, "Конфиденциальность").
hooks:
	git config core.hooksPath scripts/githooks
	@test -f .confidential-patterns || echo "create .confidential-patterns (gitignored), see scripts/githooks/confidential.sh"

# Scan every tracked file with the same checks as the hooks.
confidential:
	scripts/githooks/confidential.sh tree
