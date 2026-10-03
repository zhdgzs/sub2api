.PHONY: build generate test test-unit test-integration test-e2e

VERSION ?= $(shell ./scripts/resolve-version.sh)
LDFLAGS ?= -s -w -X main.Version=$(VERSION)

build: generate
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/server ./cmd/server

generate:
	go generate ./ent
	go generate -tags wireinject ./cmd/server

test: generate
	go test ./...
	golangci-lint run ./...

test-unit: generate
	go test -tags=unit ./...

test-integration: generate
	go test -tags=integration ./...

test-e2e: generate
	./scripts/e2e-test.sh

test-e2e-local: generate
	go test -tags=e2e -v -timeout=300s ./internal/integration/...
