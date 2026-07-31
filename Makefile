.PHONY: all build install run clean tidy test test-race test-e2e test-contract test-all lint lint-fix record-api install-tools perf-check perf-bench

all: build

# Get version info
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT ?= $(shell git rev-parse --short HEAD)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LOG_LEVEL ?= debug

# Linker flags to inject version info
LDFLAGS := -X "github.com/0xfig-labs/thinroute/internal/version.Version=$(VERSION)" \
           -X "github.com/0xfig-labs/thinroute/internal/version.Commit=$(COMMIT)" \
           -X "github.com/0xfig-labs/thinroute/internal/version.Date=$(DATE)"

install-tools:
	@command -v golangci-lint > /dev/null 2>&1 || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10)
	@command -v pre-commit > /dev/null 2>&1 || (echo "Installing pre-commit..." && pip install pre-commit==4.5.1)
	@echo "All tools are ready"

build:
	go build -ldflags '$(LDFLAGS)' -o bin/thinroute ./cmd/thinroute

install:
	go install -ldflags '$(LDFLAGS)' ./cmd/thinroute
# Run the application
run:
	LOG_LEVEL=$(LOG_LEVEL) go run ./cmd/thinroute

# Clean build artifacts
clean:
	rm -rf bin/

# Tidy dependencies
tidy:
	go mod tidy


# Run unit tests only
test:
	go test ./cmd/... ./internal/... ./config/... -v

# Run unit tests with race detection and coverage
test-race:
	go test -v -race -coverprofile=coverage.out ./cmd/... ./internal/... ./config/...

# Run e2e tests (uses an in-process mock LLM server)
test-e2e:
	go test -v -tags=e2e ./tests/e2e/...

# Run contract tests (validates API response structures against golden files)
test-contract:
	go test -v -tags=contract -timeout=5m ./tests/contract/...

# Run all tests including e2e and contract tests
test-all: test test-e2e test-contract

perf-check:
	go test -run '^TestHotPathPerfGuard$$' -count=1 -v ./tests/perf/...

perf-bench:
	go test -bench=. -benchmem ./tests/perf/...

# Record API responses for contract tests
# Usage: OPENAI_API_KEY=sk-xxx make record-api
record-api:
	@echo "Recording OpenAI chat completion..."
	go run ./cmd/recordapi -provider=openai -endpoint=chat \
		-output=tests/contract/testdata/openai/chat_completion.json
	@echo "Recording OpenAI models..."
	go run ./cmd/recordapi -provider=openai -endpoint=models \
		-output=tests/contract/testdata/openai/models.json
	@echo "Done! Golden files saved to tests/contract/testdata/"


# Run linter
lint:
	golangci-lint run --build-tags=e2e,integration,contract ./cmd/... ./config/... ./internal/... ./tests/...

# Run linter with auto-fix
lint-fix:
	golangci-lint run --fix ./cmd/... ./config/... ./internal/...
