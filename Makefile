.ONESHELL:
SHELL = /bin/sh
.SHELLFLAGS = -ec

BASE_PACKAGE := github.com/go-sqlex/sqlex

tooling:
	go install honnef.co/go/tools/cmd/staticcheck@v0.7.0

has-changes:
	git diff --exit-code --quiet HEAD --

GOPATH_BIN := $(shell go env GOPATH)/bin

lint:
	go vet ./...
	$(GOPATH_BIN)/staticcheck -checks=all ./...


# Run before committing: auto-fix + check.
prep: fix check

# Check formatting + lint (does not modify files).
check: fmt lint
	@echo "All checks passed."

# Auto-fix formatting issues.
fix:
	gofmt -w .
	@echo "Formatting fixed."

fmt:
	gofmt -d . | tee /dev/stderr | grep -q . && exit 1 || true

vuln-check:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

test-race:
	go test -v -race -count=1 ./...

update-dependencies:
	go get -u -t -v ./...
	go mod tidy
