# meshcore-go — build and development tasks.

BIN     := mc
CMD     := ./cmd/mc
BINDIR  := bin

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
PKG     := github.com/meshcore-cz/meshcore-go/cmd/mc/internal/cli
BACKEND := github.com/meshcore-cz/meshcore-go/backend
LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(BACKEND).Version=$(VERSION)

.DEFAULT_GOAL := build

.PHONY: build
build: ## Build the mc binary into bin/
	go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(BIN) $(CMD)

.PHONY: install
install: ## Install mc into $GOBIN
	go install -ldflags "$(LDFLAGS)" $(CMD)

.PHONY: test
test: ## Run the unit test suite
	go test ./...

.PHONY: race
race: ## Run tests with the race detector
	go test -race ./...

.PHONY: cover
cover: ## Run tests and write a coverage profile
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format all Go source
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any Go source is not formatted
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

.PHONY: tidy
tidy: ## Tidy module dependencies
	go mod tidy

.PHONY: check
check: fmt-check vet test ## Run formatting, vet and tests

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BINDIR) coverage.out

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
