BINARY      := mbr
MSR_BINARY  := msr
MODULE      := github.com/angsak/mbr
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -s -w \
               -X $(MODULE)/internal/version.Version=$(VERSION) \
               -X $(MODULE)/internal/version.Commit=$(COMMIT) \
               -X $(MODULE)/internal/version.Date=$(BUILD_DATE)
GOPATH_BIN  := $(shell go env GOPATH)/bin

.PHONY: build build-msr run run-msr test lint clean cross install install-msr help

## build: Compile mbr (AWS) for the current OS/arch → bin/mbr
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/mbr

## build-msr: Compile msr (Azure) for the current OS/arch → bin/msr
build-msr:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(MSR_BINARY) ./cmd/msr

## build-all: Build both mbr and msr
build-all: build build-msr

## run: Build and launch the AWS TUI
run: build
	./bin/$(BINARY)

## run-msr: Build and launch the Azure TUI
run-msr: build-msr
	./bin/$(MSR_BINARY)

## test: Run all unit tests with race detector
test:
	go test -race -count=1 ./...

## lint: Run golangci-lint (install: brew install golangci-lint)
lint:
	golangci-lint run ./...

## cross: Cross-compile both tools for all supported platforms → dist/
cross:
	@mkdir -p dist
	GOOS=linux   GOARCH=amd64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_linux_amd64            ./cmd/mbr
	GOOS=linux   GOARCH=arm64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_linux_arm64            ./cmd/mbr
	GOOS=darwin  GOARCH=amd64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_darwin_amd64           ./cmd/mbr
	GOOS=darwin  GOARCH=arm64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_darwin_arm64           ./cmd/mbr
	GOOS=windows GOARCH=amd64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_windows_amd64.exe     ./cmd/mbr
	GOOS=linux   GOARCH=amd64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(MSR_BINARY)_linux_amd64       ./cmd/msr
	GOOS=linux   GOARCH=arm64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(MSR_BINARY)_linux_arm64       ./cmd/msr
	GOOS=darwin  GOARCH=amd64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(MSR_BINARY)_darwin_amd64      ./cmd/msr
	GOOS=darwin  GOARCH=arm64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(MSR_BINARY)_darwin_arm64      ./cmd/msr
	GOOS=windows GOARCH=amd64  CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(MSR_BINARY)_windows_amd64.exe ./cmd/msr
	@echo "Built binaries:"
	@ls -lh dist/

## install: Install mbr to GOPATH/bin
install:
	CGO_ENABLED=0 go install -ldflags "$(LDFLAGS)" ./cmd/mbr

## install-msr: Install msr to GOPATH/bin
install-msr:
	CGO_ENABLED=0 go install -ldflags "$(LDFLAGS)" ./cmd/msr

## release: Run GoReleaser (requires GITHUB_TOKEN env var)
release:
	goreleaser release --clean

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/

## help: Print this help message
help:
	@grep -E '^## ' Makefile | sed 's/^## /  /'
