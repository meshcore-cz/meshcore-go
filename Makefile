# meshcore-go — build and development tasks.

BIN     := mc
CMD     := ./cmd/mc
BINDIR  := bin
DISTDIR := dist

REMOTE         ?= origin
RELEASE_BRANCH ?= main

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

.PHONY: dist
dist: ## Cross-build mc for all release platforms into dist/ (run on macOS for darwin)
	@mkdir -p $(DISTDIR)
	@set -e; \
	for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
		os=$${target%/*}; arch=$${target#*/}; \
		ext=; [ "$$os" = windows ] && ext=.exe; \
		out=$(DISTDIR)/$(BIN)_$(VERSION)_$${os}_$${arch}$$ext; \
		cgo=0; cc=; \
		if [ "$$os" = darwin ]; then \
			cgo=1; \
			[ "$$arch" = amd64 ] && cc="clang -arch x86_64"; \
			[ "$$arch" = arm64 ] && cc="clang -arch arm64"; \
		fi; \
		echo "building $$out"; \
		CGO_ENABLED=$$cgo GOOS=$$os GOARCH=$$arch CC="$$cc" \
			go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o $$out $(CMD); \
	done
	@echo "darwin targets are built with cgo (CoreBluetooth) and require running this on macOS."

# release runs fmt-check/vet/build locally (deterministic on every OS) and
# leaves the full test suite to CI, which gates main before a tag is cut.
.PHONY: release
release: ## Tag and push a release, for example make release VERSION=v0.2.0
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$$' || { \
		echo "Set an explicit semver tag, for example: make release VERSION=v0.2.0"; exit 1; }
	@test "$$(git branch --show-current)" = "$(RELEASE_BRANCH)" || { \
		echo "Release must be created from the $(RELEASE_BRANCH) branch"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "Working tree is not clean"; exit 1; }
	@git fetch --quiet $(REMOTE) $(RELEASE_BRANCH) --tags
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse $(REMOTE)/$(RELEASE_BRANCH))" || { \
		echo "Local $(RELEASE_BRANCH) is not synchronized with $(REMOTE)/$(RELEASE_BRANCH)"; exit 1; }
	@! git rev-parse "$(VERSION)" >/dev/null 2>&1 || { echo "Tag $(VERSION) already exists"; exit 1; }
	$(MAKE) fmt-check vet build
	git tag -a "$(VERSION)" -m "meshcore-go $(VERSION)"
	git push $(REMOTE) "$(RELEASE_BRANCH)" "$(VERSION)"
	@echo "Released $(VERSION). The release workflow will build and publish mc binaries."

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BINDIR) $(DISTDIR) coverage.out

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
