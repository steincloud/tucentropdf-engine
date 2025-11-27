# Makefile para TuCentroPDF Engine V2

# Variables
BINARY_NAME=tucentropdf-engine-v2
MAIN_PATH=./cmd/server
BUILD_DIR=./build
TEST_TIMEOUT=30m
COVERAGE_DIR=./coverage

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod
GOGET=$(GOCMD) get

# Colores para output
GREEN=\033[0;32m
YELLOW=\033[1;33m
RED=\033[0;31m
NC=\033[0m # No Color

.PHONY: all build clean test test-verbose test-short test-integration test-unit test-coverage test-coverage-html deps deps-download deps-clean run run-dev docker-build docker-run help benchmark lint format check install-tools

# Default target
all: clean deps test build

## Build Commands
build: ## Build the application
	@echo "$(GREEN)Building $(BINARY_NAME)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(GREEN)Build completed: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

build-linux: ## Build for Linux
	@echo "$(GREEN)Building for Linux...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux $(MAIN_PATH)
	@echo "$(GREEN)Linux build completed$(NC)"

build-windows: ## Build for Windows
	@echo "$(GREEN)Building for Windows...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME).exe $(MAIN_PATH)
	@echo "$(GREEN)Windows build completed$(NC)"

build-all: build-linux build-windows ## Build for all platforms

## Test Commands
test: ## Run all tests
	@echo "$(GREEN)Running all tests...$(NC)"
	$(GOTEST) -timeout $(TEST_TIMEOUT) -race -v ./...

test-short: ## Run tests excluding long-running ones
	@echo "$(GREEN)Running short tests...$(NC)"
	$(GOTEST) -short -timeout 5m -race -v ./...

test-unit: ## Run only unit tests
	@echo "$(GREEN)Running unit tests...$(NC)"
	$(GOTEST) -short -timeout 5m -race -v ./internal/...

test-integration: ## Run integration tests
	@echo "$(GREEN)Running integration tests...$(NC)"
	$(GOTEST) -timeout $(TEST_TIMEOUT) -race -v -run "Integration" ./...

test-verbose: ## Run tests with verbose output
	@echo "$(GREEN)Running tests with verbose output...$(NC)"
	$(GOTEST) -timeout $(TEST_TIMEOUT) -race -v -count=1 ./...

test-coverage: ## Run tests with coverage
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) -timeout $(TEST_TIMEOUT) -race -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	$(GOCMD) tool cover -func=$(COVERAGE_DIR)/coverage.out
	@echo "$(GREEN)Coverage report saved to $(COVERAGE_DIR)/coverage.out$(NC)"

test-coverage-html: test-coverage ## Generate HTML coverage report
	@echo "$(GREEN)Generating HTML coverage report...$(NC)"
	$(GOCMD) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "$(GREEN)HTML coverage report: $(COVERAGE_DIR)/coverage.html$(NC)"

test-pdf: ## Run PDF service tests specifically
	@echo "$(GREEN)Running PDF service tests...$(NC)"
	$(GOTEST) -timeout 10m -race -v ./internal/pdf/...

test-ocr: ## Run OCR service tests specifically
	@echo "$(GREEN)Running OCR service tests...$(NC)"
	$(GOTEST) -timeout 15m -race -v ./internal/ocr/...

test-office: ## Run Office service tests specifically
	@echo "$(GREEN)Running Office service tests...$(NC)"
	$(GOTEST) -timeout 15m -race -v ./internal/office/...

test-middleware: ## Run middleware tests specifically
	@echo "$(GREEN)Running middleware tests...$(NC)"
	$(GOTEST) -timeout 5m -race -v ./internal/api/middleware/...

test-config: ## Run config tests specifically
	@echo "$(GREEN)Running config tests...$(NC)"
	$(GOTEST) -timeout 5m -race -v ./internal/config/...

## Benchmark Commands
benchmark: ## Run benchmarks
	@echo "$(GREEN)Running benchmarks...$(NC)"
	$(GOTEST) -bench=. -benchmem -timeout $(TEST_TIMEOUT) ./...

benchmark-pdf: ## Run PDF benchmarks
	@echo "$(GREEN)Running PDF benchmarks...$(NC)"
	$(GOTEST) -bench=. -benchmem -timeout 10m ./internal/pdf/...

benchmark-ocr: ## Run OCR benchmarks
	@echo "$(GREEN)Running OCR benchmarks...$(NC)"
	$(GOTEST) -bench=. -benchmem -timeout 15m ./internal/ocr/...

benchmark-office: ## Run Office benchmarks
	@echo "$(GREEN)Running Office benchmarks...$(NC)"
	$(GOTEST) -bench=. -benchmem -timeout 15m ./internal/office/...

## Dependencies
deps: ## Download dependencies
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	$(GOMOD) download
	$(GOMOD) tidy

deps-update: ## Update dependencies
	@echo "$(GREEN)Updating dependencies...$(NC)"
	$(GOGET) -u ./...
	$(GOMOD) tidy

deps-clean: ## Clean module cache
	@echo "$(GREEN)Cleaning module cache...$(NC)"
	$(GOMOD) clean -cache

deps-verify: ## Verify dependencies
	@echo "$(GREEN)Verifying dependencies...$(NC)"
	$(GOMOD) verify

## Development Commands
run: build ## Run the application
	@echo "$(GREEN)Running $(BINARY_NAME)...$(NC)"
	./$(BUILD_DIR)/$(BINARY_NAME)

run-dev: ## Run in development mode
	@echo "$(GREEN)Running in development mode...$(NC)"
	$(GOCMD) run $(MAIN_PATH)

run-watch: ## Run with file watching (requires entr)
	@echo "$(GREEN)Running with file watching...$(NC)"
	find . -name "*.go" | entr -r $(GOCMD) run $(MAIN_PATH)

## Code Quality
lint: install-tools ## Run linter
	@echo "$(GREEN)Running linter...$(NC)"
	golangci-lint run ./...

lint-fix: install-tools ## Run linter with auto-fix
	@echo "$(GREEN)Running linter with auto-fix...$(NC)"
	golangci-lint run --fix ./...

format: ## Format code
	@echo "$(GREEN)Formatting code...$(NC)"
	$(GOCMD) fmt ./...

vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(NC)"
	$(GOCMD) vet ./...

check: format vet lint test-short ## Run all code quality checks

## Tools Installation
install-tools: ## Install development tools
	@echo "$(GREEN)Installing development tools...$(NC)"
	$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOGET) github.com/stretchr/testify@latest

## Docker Commands
docker-build: ## Build Docker image
	@echo "$(GREEN)Building Docker image...$(NC)"
	docker build -t tucentropdf-engine-v2 .

docker-run: docker-build ## Run Docker container
	@echo "$(GREEN)Running Docker container...$(NC)"
	docker run -p 8080:8080 tucentropdf-engine-v2

docker-test: ## Run tests in Docker
	@echo "$(GREEN)Running tests in Docker...$(NC)"
	docker build -f Dockerfile.test -t tucentropdf-engine-v2:test .
	docker run --rm tucentropdf-engine-v2:test

## Test Data Management
testdata-generate: ## Generate test data files
	@echo "$(GREEN)Generating test data files...$(NC)"
	$(GOCMD) run testdata/generate_test_files.go
	$(GOCMD) run testdata/generate_images.go

testdata-clean: ## Clean test data
	@echo "$(GREEN)Cleaning test data...$(NC)"
	rm -rf testdata/pdf/*.pdf
	rm -rf testdata/office/*.tmp
	rm -rf testdata/ocr/**/*.png testdata/ocr/**/*.jpg
	rm -rf temp_test/

testdata-verify: ## Verify test data exists
	@echo "$(GREEN)Verifying test data...$(NC)"
	@if [ ! -f "testdata/pdf/sample1.pdf" ]; then echo "$(RED)Missing testdata/pdf/sample1.pdf$(NC)"; fi
	@if [ ! -f "testdata/pdf/sample2.pdf" ]; then echo "$(RED)Missing testdata/pdf/sample2.pdf$(NC)"; fi
	@if [ ! -f "testdata/office/sample.txt" ]; then echo "$(RED)Missing testdata/office/sample.txt$(NC)"; fi
	@echo "$(GREEN)Test data verification completed$(NC)"

## Database/Redis Commands (for integration tests)
redis-start: ## Start Redis for testing (requires Docker)
	@echo "$(GREEN)Starting Redis for testing...$(NC)"
	docker run -d --name tucentropdf-redis-test -p 6379:6379 redis:7-alpine

redis-stop: ## Stop Redis testing instance
	@echo "$(GREEN)Stopping Redis testing instance...$(NC)"
	docker stop tucentropdf-redis-test || true
	docker rm tucentropdf-redis-test || true

## Cleanup
clean: ## Clean build artifacts
	@echo "$(GREEN)Cleaning build artifacts...$(NC)"
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)
	rm -rf temp_test/

clean-all: clean testdata-clean ## Clean everything including test data
	@echo "$(GREEN)Full cleanup completed$(NC)"

## CI/CD Simulation
ci-test: deps test-coverage lint ## Run CI pipeline tests
	@echo "$(GREEN)CI pipeline completed successfully$(NC)"

ci-build: clean deps test build ## Run full CI/CD pipeline
	@echo "$(GREEN)Full CI/CD pipeline completed$(NC)"

## Performance Testing
perf-test: ## Run performance tests
	@echo "$(GREEN)Running performance tests...$(NC)"
	$(GOTEST) -timeout $(TEST_TIMEOUT) -race -v -run "Performance" ./...

stress-test: ## Run stress tests
	@echo "$(GREEN)Running stress tests...$(NC)"
	$(GOTEST) -timeout 60m -race -v -count=10 ./internal/pdf/...

## Security
security-scan: ## Run security scan (requires gosec)
	@echo "$(GREEN)Running security scan...$(NC)"
	gosec ./...

vulnerability-check: ## Check for vulnerabilities
	@echo "$(GREEN)Checking for vulnerabilities...$(NC)"
	$(GOCMD) list -json -deps ./... | nancy sleuth

## Monitoring & Debug
cpu-profile: ## Run tests with CPU profiling
	@echo "$(GREEN)Running tests with CPU profiling...$(NC)"
	$(GOTEST) -cpuprofile cpu.prof -bench=. ./internal/pdf/

memory-profile: ## Run tests with memory profiling
	@echo "$(GREEN)Running tests with memory profiling...$(NC)"
	$(GOTEST) -memprofile mem.prof -bench=. ./internal/pdf/

race-test: ## Run tests with race detection
	@echo "$(GREEN)Running tests with race detection...$(NC)"
	$(GOTEST) -race -timeout $(TEST_TIMEOUT) ./...

## Help
help: ## Show this help message
	@echo "$(GREEN)TuCentroPDF Engine V2 - Available Commands:$(NC)"
	@echo ""
	@echo "$(YELLOW)Build Commands:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && /^build/ {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Test Commands:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && /^test/ {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Development Commands:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && /^(run|deps|clean|format|lint|check)/ {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Other Commands:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && !/^(build|test|run|deps|clean|format|lint|check)/ {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(YELLOW)Usage Examples:$(NC)"
	@echo "  make test              # Run all tests"
	@echo "  make test-short        # Run quick tests only"
	@echo "  make test-coverage     # Run tests with coverage"
	@echo "  make build             # Build the application"
	@echo "  make run-dev           # Run in development mode"
	@echo "  make check             # Run all quality checks"
	@echo "  make ci-test           # Simulate CI pipeline"