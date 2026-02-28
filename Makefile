# Makefile for Axon Go
# Modern Go project build targets following 2026 best practices

.PHONY: all build clean test lint lint-fix vet check fmt docs help
.DEFAULT_GOAL := help

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GORUN := $(GOCMD) run
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOTOOL := $(GOCMD) tool

# Binary name
BINARY_NAME := axon-go
OUTPUT_DIR := ./bin

# Version info (set via ldflags)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

# Build flags
BUILD_FLAGS := -trimpath
RACE_FLAGS := -race

# Coverage
COVERAGE_DIR := $(CURDIR)/coverage
COVERAGE_FILE := $(COVERAGE_DIR)/coverage.out
COVERAGE_HTML := $(COVERAGE_DIR)/index.html

# Tools
GOLANGCI_LINT := golangci-lint
GOLANGCI_LINT_VERSION := latest
STATICCHECK := staticcheck
GOVULNCHECK := govulncheck
GOTESTSUM := gotestsum
GORELEASER := goreleaser

# Go version check
GO_VERSION := $(shell $(GOCMD) version | cut -d' ' -f3 | sed 's/go//' | cut -d'.' -f2)
REQUIRED_GO_VERSION := 23

# Colors for output
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

##@ General

help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\n$(BLUE)Axon Go Build System$(NC)\n\n$(YELLOW)Usage:$(NC)\n  make $(GREEN)<target>$(NC)\n\n$(YELLOW)Targets:$(NC)\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2 } /^##@/ { printf "\n$(YELLOW)%s$(NC)\n", substr($$0, 5) }' $(MAKEFILE_LIST)

version: ## Print version information
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Go Version: $(shell $(GOCMD) version)"

check-go-version: ## Verify Go version
	@if [ "$(GO_VERSION)" -lt "$(REQUIRED_GO_VERSION)" ]; then \
		echo "$(RED)Error: Go 1.$(REQUIRED_GO_VERSION) or higher required (found 1.$(GO_VERSION))$(NC)"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ Go version check passed$(NC)"

##@ Building

build: check-go-version ## Build the axon binary
	@echo "$(BLUE)Building $(BINARY_NAME)...$(NC)"
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME) .
	@echo "$(GREEN)✓ Build complete: $(OUTPUT_DIR)/$(BINARY_NAME)$(NC)"

build-debug: check-go-version ## Build with debug symbols
	@echo "$(BLUE)Building $(BINARY_NAME) (debug)...$(NC)"
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) $(BUILD_FLAGS) -gcflags="all=-N -l" -o $(OUTPUT_DIR)/$(BINARY_NAME)-debug .
	@echo "$(GREEN)✓ Debug build complete$(NC)"

build-race: check-go-version ## Build with race detector
	@echo "$(BLUE)Building $(BINARY_NAME) (race detector)...$(NC)"
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) $(BUILD_FLAGS) $(RACE_FLAGS) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-race .
	@echo "$(GREEN)✓ Race-enabled build complete$(NC)"

build-all: build build-debug build-race ## Build all variants

install: check-go-version ## Install the binary to GOPATH/bin
	@echo "$(BLUE)Installing $(BINARY_NAME)...$(NC)"
	$(GOCMD) install $(BUILD_FLAGS) $(LDFLAGS) .
	@echo "$(GREEN)✓ Installation complete$(NC)"

clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning...$(NC)"
	@rm -rf $(OUTPUT_DIR)
	@rm -rf $(COVERAGE_DIR)
	@rm -f $(BINARY_NAME)
	@$(GOMOD) tidy
	@echo "$(GREEN)✓ Clean complete$(NC)"

##@ Testing

test: check-go-version ## Run all tests
	@echo "$(BLUE)Running tests...$(NC)"
	$(GOTEST) ./... -v -short

test-full: check-go-version ## Run all tests including integration
	@echo "$(BLUE)Running full test suite...$(NC)"
	$(GOTEST) ./... -v -tags=integration

test-race: check-go-version ## Run tests with race detector
	@echo "$(BLUE)Running tests (race detector)...$(NC)"
	$(GOTEST) ./... -race -short

test-coverage: check-go-version ## Run tests with coverage
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) ./... -coverprofile=$(COVERAGE_FILE) -covermode=atomic
	$(GOTOOL) cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "$(GREEN)✓ Coverage report: $(COVERAGE_HTML)$(NC)"

test-summary: check-go-version ## Run tests with gotestsum
	@echo "$(BLUE)Running tests (gotestsum)...$(NC)"
	$(GOTESTSUM) --format=testname -- ./... -short

bench: check-go-version ## Run benchmarks
	@echo "$(BLUE)Running benchmarks...$(NC)"
	$(GOTEST) ./... -bench=. -benchmem -run=^$$ -benchtime=1s

bench-mem: check-go-version ## Run benchmarks with memory profiling
	@echo "$(BLUE)Running benchmarks (memory profile)...$(NC)"
	$(GOTEST) ./... -bench=. -benchmem -run=^$$ -memprofile=$(COVERAGE_DIR)/bench.mem.pprof

##@ Linting & Quality

lint: check-go-version ## Run golangci-lint
	@echo "$(BLUE)Running linters...$(NC)"
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { echo "$(YELLOW)golangci-lint not found, installing...$(NC)"; $(MAKE) install-tools; }
	$(GOLANGCI_LINT) run --config .golangci.yml
	@echo "$(GREEN)✓ Linting passed$(NC)"

lint-fix: check-go-version ## Run linters with auto-fix
	@echo "$(BLUE)Running linters (auto-fix)...$(NC)"
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { echo "$(YELLOW)golangci-lint not found, installing...$(NC)"; $(MAKE) install-tools; }
	$(GOLANGCI_LINT) run --config .golangci.yml --fix
	@echo "$(GREEN)✓ Linting complete$(NC)"

lint-full: check-go-version ## Run all linters (slower)
	@echo "$(BLUE)Running full lint...$(NC)"
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { echo "$(YELLOW)golangci-lint not found, installing...$(NC)"; $(MAKE) install-tools; }
	$(GOLANGCI_LINT) run --config .golangci.yml --fast=false
	@echo "$(GREEN)✓ Full lint passed$(NC)"

vet: check-go-version ## Run go vet
	@echo "$(BLUE)Running go vet...$(NC)"
	$(GOVET) ./...
	@echo "$(GREEN)✓ go vet passed$(NC)"

staticcheck: check-go-version ## Run staticcheck
	@echo "$(BLUE)Running staticcheck...$(NC)"
	$(STATICCHECK) ./...
	@echo "$(GREEN)✓ staticcheck passed$(NC)"

cyclo: check-go-version ## Check cyclomatic complexity
	@echo "$(BLUE)Checking cyclomatic complexity...$(NC)"
	@gocyclo -over 15 $(shell find . -name '*.go' -not -path './vendor/*' -not -path './.git/*')
	@echo "$(GREEN)✓ Complexity check passed$(NC)"

govulncheck: check-go-version ## Run vulnerability check
	@echo "$(BLUE)Running govulncheck...$(NC)"
	$(GOVULNCHECK) ./...
	@echo "$(GREEN)✓ No vulnerabilities found$(NC)"

check: lint vet staticcheck govulncheck ## Run all quality checks
	@echo "$(GREEN)✓ All quality checks passed$(NC)"

fmt: ## Format all Go files
	@echo "$(BLUE)Formatting code...$(NC)"
	@goimports -w .
	@gofmt -s -w .
	@echo "$(GREEN)✓ Formatting complete$(NC)"

fmt-check: ## Check if files are formatted
	@echo "$(BLUE)Checking formatting...$(NC)"
	@diff=$$(gofmt -d .); \
	if [ -n "$$diff" ]; then \
		echo "$(RED)The following files need formatting:$(NC)"; \
		echo "$$diff"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ All files formatted$(NC)"

##@ Dependencies

deps: ## Download dependencies
	@echo "$(BLUE)Downloading dependencies...$(NC)"
	$(GOMOD) download
	@echo "$(GREEN)✓ Dependencies downloaded$(NC)"

deps-tidy: ## Tidy dependencies
	@echo "$(BLUE)Tidying dependencies...$(NC)"
	$(GOMOD) tidy
	$(GOMOD) verify
	@echo "$(GREEN)✓ Dependencies tidied$(NC)"

deps-update: ## Update dependencies
	@echo "$(YELLOW)Updating dependencies...$(NC)"
	$(GOMOD) tidy -compat=1.23
	@echo "$(GREEN)✓ Dependencies updated$(NC)"

deps-audit: ## Check for vulnerable dependencies
	@echo "$(BLUE)Checking for vulnerabilities...$(NC)"
	$(GOVULNCHECK) ./...

install-tools: ## Install development tools from tools.go
	@echo "$(BLUE)Installing development tools...$(NC)"
	$(GOMOD) download
	@$(GOBUILD) -o $(GOPATH)/bin/golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@$(GOBUILD) -o $(GOPATH)/bin/gotestsum gotest.tools/gotestsum
	@$(GOBUILD) -o $(GOPATH)/bin/govulncheck golang.org/x/vuln/cmd/govulncheck
	@$(GOBUILD) -o $(GOPATH)/bin/staticcheck honnef.co/go/tools/cmd/staticcheck
	@$(GOBUILD) -o $(GOPATH)/bin/gocyclo github.com/fzipp/gocyclo/cmd/gocyclo
	@$(GOBUILD) -o $(GOPATH)/bin/goimports golang.org/x/tools/cmd/goimports
	@echo "$(GREEN)✓ Development tools installed$(NC)"

##@ Documentation

docs: ## Generate documentation
	@echo "$(BLUE)Generating documentation...$(NC)"
	@$(GOCMD) doc -all ./... > docs/api.md
	@echo "$(GREEN)✓ Documentation generated$(NC)"

godoc-server: ## Serve godoc locally
	@echo "$(BLUE)Starting godoc server on :6060$(NC)"
	@$(GOCMD) run golang.org/x/tools/cmd/godoc -http=:6060

##@ Development

run: ## Run the application
	$(GORUN) .

run-analyze: ## Run axon analyze
	$(GORUN) analyze .

run-query: ## Run axon query
	$(GORUN) query "test"

dev: ## Run with hot reload (requires air)
	air -c .air.toml

mocks: ## Generate mocks (requires mockgen)
	@echo "$(BLUE)Generating mocks...$(NC)"
	@go generate ./...
	@echo "$(GREEN)✓ Mocks generated$(NC)"

##@ Release

release-dry: ## Dry run release
	@echo "$(BLUE)Dry run release...$(NC)"
	$(GORELEASER) release --snapshot --clean

release: ## Create release
	@echo "$(BLUE)Creating release...$(NC)"
	$(GORELEASER) release --clean

release-notes: ## Generate release notes
	@echo "$(BLUE)Generating release notes...$(NC)"
	$(GORELEASER) release-notes --next

##@ CI/CD

ci: check fmt-check lint vet test ## Run all CI checks
	@echo "$(GREEN)✓ All CI checks passed$(NC)"

ci-full: check fmt-check lint-full vet staticcheck govulncheck test-coverage ## Run full CI pipeline
	@echo "$(GREEN)✓ Full CI pipeline passed$(NC)"

##@ Utilities

todo: ## List all TODO comments
	@echo "$(BLUE)Searching for TODOs...$(NC)"
	@rg "TODO|FIXME|XXX|HACK" --glob '*.go' || true

stats: ## Show code statistics
	@echo "$(BLUE)Code Statistics:$(NC)"
	@find . -name '*.go' -not -path './vendor/*' -not -path './.git/*' | \
		xargs wc -l | tail -1
	@echo ""
	@echo "$(BLUE)Test Coverage:$(NC)"
	@$(GOTEST) ./... -cover 2>/dev/null | grep -E "^(ok|FAIL)" | awk '{print $$1, $$2}'
