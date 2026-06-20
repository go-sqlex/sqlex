.ONESHELL:
SHELL = /bin/sh
.SHELLFLAGS = -ec

BASE_PACKAGE := github.com/go-sqlex/sqlex

tooling:
	go install honnef.co/go/tools/cmd/staticcheck@v0.6.1
	go install golang.org/x/vuln/cmd/govulncheck@v1.1.3
	go install golang.org/x/tools/cmd/goimports@v0.24.0

has-changes:
	git diff --exit-code --quiet HEAD --

GOPATH_BIN := $(shell go env GOPATH)/bin

lint:
	go vet ./...
	$(GOPATH_BIN)/staticcheck -checks=all ./...

fmt:
	go list -f '{{.Dir}}' ./... | xargs -I {} goimports -local $(BASE_PACKAGE) -w {}

vuln-check:
	govulncheck ./...

test-race:
	go test -v -race -count=1 ./...

update-dependencies:
	go get -u -t -v ./...
	go mod tidy
