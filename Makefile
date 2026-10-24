# nib-git — developer tasks.
# Single source of truth for the quality gate. lefthook (pre-commit) and CI
# both call these targets; never duplicate the commands elsewhere.

GO        ?= go
BINARY    ?= nib
PKGS      := ./...

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help.
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the nib binary.
	$(GO) build -o $(BINARY) .

.PHONY: install
install: ## Install nib into $(go env GOPATH)/bin, to run it from any repository.
	$(GO) build -o "$$($(GO) env GOPATH)/bin/$(BINARY)" .

.PHONY: test
test: ## Run unit tests (fast, hermetic).
	$(GO) test $(PKGS)

.PHONY: test-integration
test-integration: ## Run integration tests against real git (build tag: integration).
	$(GO) test -tags=integration $(PKGS)

.PHONY: bench
bench: ## Benchmark synthetic indexing and coupling (1000 commits, no Git ingestion).
	$(GO) test -run '^$$' -bench . -benchmem ./internal/index

.PHONY: fmt
fmt: ## Format all Go files in place.
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any Go file is not gofmt-clean.
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet.
	$(GO) vet $(PKGS)

.PHONY: lint
lint: ## Run golangci-lint (see .golangci.yml).
	PATH="$$(go env GOPATH)/bin:$$PATH" golangci-lint run

.PHONY: check
check: fmt-check vet lint test ## Fast gate: format + vet + lint + test (pre-commit & CI).

.PHONY: cover
cover: ## Per-package coverage gate (80% on domain packages). CI.
	@./scripts/coverage.sh

.PHONY: ci
ci: check cover test-integration ## Full gate run in CI.

.PHONY: tools
tools: ## Install dev tools (golangci-lint, lefthook) and git hooks.
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	$(GO) install github.com/evilmartians/lefthook@latest
	lefthook install

.PHONY: run
run: ## Build and run nib.
	$(GO) run .
