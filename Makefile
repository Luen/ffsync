# ffsync – build and test
.PHONY: build test clean all

BINARY := ffsync
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/ffsync/ffsync/internal/cli.versionStr=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/ffsync

test:
	go test ./...

clean:
	rm -f $(BINARY) ffsync-*

# Cross-build for common platforms
all: build
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o ffsync-darwin-amd64 ./cmd/ffsync
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o ffsync-darwin-arm64 ./cmd/ffsync
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o ffsync-linux-amd64 ./cmd/ffsync
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o ffsync-linux-arm64 ./cmd/ffsync
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o ffsync-windows-amd64.exe ./cmd/ffsync
