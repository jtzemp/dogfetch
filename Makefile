.PHONY: build install test test-cover golden-update lint clean version all build-all build-linux build-darwin build-windows dev release-tag

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

# testseam gates the DOGFETCH_API_URL mock-server override (internal/fetcher/testseam.go)
# so it's physically absent from binaries built without this tag.
TEST_TAGS=-tags testseam

# Run tests
test:
	$(GOTEST) $(TEST_TAGS) -v ./...

# Run tests with coverage
test-cover:
	$(GOTEST) $(TEST_TAGS) -v -cover ./...

# Regenerate golden test fixtures (e.g. internal/toon/testdata/*.toon)
golden-update:
	$(GOTEST) $(TEST_TAGS) ./internal/toon -update

# Run the linter (matches CI: golangci-lint)
lint:
	golangci-lint run

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

# Bump .claude-plugin/plugin.json to V, commit, and tag in one local
# step (usage: make release-tag V=1.2.3). Keeps the version bump and the
# tag on the same commit so release.yml can gate on a match instead of
# mutating the default branch during a tag build.
release-tag:
	@if [ -z "$(V)" ]; then echo "usage: make release-tag V=x.y.z"; exit 1; fi
	@if [ -n "$$(git status --porcelain)" ]; then echo "working tree not clean"; exit 1; fi
	bash scripts/sync-plugin-version.sh "$(V)"
	git add .claude-plugin/plugin.json
	git commit -m "chore: bump Claude plugin version to $(V)"
	git tag "v$(V)"
	@echo "Review with 'git show' and 'git log -1', then push both:"
	@echo "  git push origin HEAD && git push origin v$(V)"

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
