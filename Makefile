VERSION ?= dev
COMMIT ?= $(shell git rev-parse --verify HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE)
.PHONY: fmt lint test test-race coverage vuln build docker-build run
fmt:
	gofmt -w cmd internal
lint:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
test:
	go test ./...
test-race:
	go test -race ./...
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/bot ./cmd/bot
docker-build:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t btc-route-bot:local .
run:
	go run ./cmd/bot
