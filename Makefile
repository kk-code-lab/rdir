.PHONY: build test test-coverage test-race clean fmt lint help run bench-fuzzy bench-fuzzy-prof pprof-fuzzy-cpu pprof-fuzzy-mem

BINARY_NAME=rdir
BUILD_DIR=build
MAIN_ENTRY=./cmd/rdir
INTERNAL_PACKAGES=./internal/...
PROFILE_DIR=$(BUILD_DIR)/profiles
CPU_PROFILE=$(PROFILE_DIR)/fuzzy.cpu.pprof
MEM_PROFILE=$(PROFILE_DIR)/fuzzy.mem.pprof

build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_ENTRY)

test:
	@echo "Running tests..."
	go test $(INTERNAL_PACKAGES)

test-coverage:
	@echo "Running tests with coverage..."
	go test $(INTERNAL_PACKAGES) -cover

test-race:
	@echo "Running tests with race detector..."
	go test $(INTERNAL_PACKAGES) -race

bench-fuzzy:
	@echo "Running fuzzy matching benchmarks..."
	go test $(INTERNAL_PACKAGES) -bench Fuzzy -benchmem

bench-fuzzy-prof:
	@echo "Running fuzzy benchmarks with CPU/memory profiles..."
	@mkdir -p $(PROFILE_DIR)
	go test $(INTERNAL_PACKAGES) -run=^$$ -bench Fuzzy -benchmem -cpuprofile $(CPU_PROFILE) -memprofile $(MEM_PROFILE)
	@echo ""
	@echo "Profiles written to:"
	@echo "  $(CPU_PROFILE)"
	@echo "  $(MEM_PROFILE)"
	@echo "Use 'make pprof-fuzzy-cpu' or 'make pprof-fuzzy-mem' to inspect."

pprof-fuzzy-cpu:
	@test -f $(CPU_PROFILE) || (echo "CPU profile not found. Run 'make bench-fuzzy-prof' first."; exit 1)
	@echo "Top hot paths (cpu):"
	go tool pprof -top $(CPU_PROFILE)

pprof-fuzzy-mem:
	@test -f $(MEM_PROFILE) || (echo "Memory profile not found. Run 'make bench-fuzzy-prof' first."; exit 1)
	@echo "Top allocators (mem):"
	go tool pprof -top $(MEM_PROFILE)

clean:
	@echo "Cleaning build artifacts..."
	rm -f $(BUILD_DIR)/$(BINARY_NAME)

fmt:
	@echo "Formatting code..."
	go fmt ./...

lint:
	@echo "Linting code (requires golangci-lint)..."
	golangci-lint run ./...

run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

help:
	@echo "rdir - Terminal file manager in Go"
	@echo ""
	@echo "Available targets:"
	@echo "  make build         - Build the binary to build/rdir"
	@echo "  make test          - Run all tests with verbose output"
	@echo "  make test-coverage - Run tests with coverage report"
	@echo "  make test-race     - Run tests with race detector"
	@echo "  make bench-fuzzy   - Run fuzzy matching benchmarks"
	@echo "  make bench-fuzzy-prof - Run fuzzy benchmarks and capture CPU/memory profiles"
	@echo "  make pprof-fuzzy-cpu  - Show hottest stack traces from the captured CPU profile"
	@echo "  make pprof-fuzzy-mem  - Show top allocators from the captured memory profile"
	@echo "  make run           - Build and run the application"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make fmt           - Format code with go fmt"
	@echo "  make lint          - Run linter (requires golangci-lint)"
	@echo "  make help          - Show this help message"
