.PHONY: build install test clean version

# Binary name
BINARY_NAME=dogfetch

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOINSTALL=$(GOCMD) install
GOCLEAN=$(GOCMD) clean

# Linker flags to inject version info
LDFLAGS=-ldflags "\
	-X github.com/jtzemp/dogfetch/internal/version.Version=$(VERSION) \
	-X github.com/jtzemp/dogfetch/internal/version.Commit=$(COMMIT) \
	-X github.com/jtzemp/dogfetch/internal/version.Date=$(DATE)"

# Default target
all: test build

# Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .

# Install the binary to GOPATH/bin
install:
	$(GOINSTALL) $(LDFLAGS) .

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-cover:
	$(GOTEST) -v -cover ./...

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

# Print version information
version:
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"
	@echo "Date:    $(DATE)"

# Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 .

build-darwin:
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 .

build-windows:
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe .

# Development build (no version injection)
dev:
	$(GOBUILD) -o $(BINARY_NAME) .
