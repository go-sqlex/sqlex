.ONESHELL:
SHELL = /bin/sh
.SHELLFLAGS = -ec

.DEFAULT_GOAL := help

GO            := go
BASE_PACKAGE  := github.com/go-sqlex/sqlex
COVER_DIR     := cover
GOPATH_BIN    := $(shell $(GO) env GOPATH)/bin

# ============================================================
# Formatting & Linting
# ============================================================

prep: fix check ## Auto-fix + full check (run before commit)

check: fmt lint ## Format + lint check (read-only)
	@echo "All checks passed."

fix: ## Auto-fix imports and formatting
	$(GO) fmt ./...
	$(GOPATH_BIN)/goimports -w -local ${BASE_PACKAGE} .
	@echo "Formatting fixed."

fmt: ## gofmt compliance check
	gofmt -d . | tee /dev/stderr | grep -q . && exit 1 || true

lint: ## goimports + go vet + staticcheck
	$(GOPATH_BIN)/goimports -d -local ${BASE_PACKAGE} . | tee /dev/stderr | grep -q . && exit 1 || true
	$(GO) vet ./...
	$(GOPATH_BIN)/staticcheck -checks=all ./...

# ============================================================
# Testing
# ============================================================

test: ## Run all tests with race detection
	$(GO) test -v -race -count=1 ./...

test-cover: ## Tests + coverage HTML
	@mkdir -p $(COVER_DIR)
	$(GO) test -race -count=1 -coverprofile=$(COVER_DIR)/cover.out ./...
	$(GO) tool cover -html=$(COVER_DIR)/cover.out -o $(COVER_DIR)/cover.html
	@echo "Coverage report: $(COVER_DIR)/cover.html"

test-func: ## Run single test (FUNC=TestName make test-func)
	$(GO) test -v -race -count=1 -run $(FUNC) ./...

# ============================================================
# Tooling
# ============================================================

tooling: ## Install lint tools
	$(GO) install honnef.co/go/tools/cmd/staticcheck@v0.7.0
	$(GO) install golang.org/x/tools/cmd/goimports@v0.46.0

update-deps: ## Update Go dependencies
	$(GO) get -u -t -v ./...
	$(GO) mod tidy

# ============================================================
# Security
# ============================================================

vuln-check: ## Scan dependencies for CVEs
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
	awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: prep check fix fmt lint test test-cover test-func \
        tooling update-deps vuln-check help
