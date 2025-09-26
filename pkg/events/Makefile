.PHONY: all test bench clean high-throughput concurrent worker-pool memory system-topics help

GOTEST := go test -v -timeout 30s
GOBENCH := go test -bench=. -benchtime=10s -run=^$$
PKG := .

help:
	@echo "Available targets:"
	@echo "  make high-throughput  - Run single publisher high throughput test"
	@echo "  make concurrent       - Run concurrent multi-publisher test"
	@echo "  make worker-pool      - Run worker pool saturation test"
	@echo "  make memory           - Run memory pressure test"
	@echo "  make system-topics    - Run system topics integrity test"
	@echo "  make bench            - Run benchmarks"
	@echo "  make test             - Run all tests"
	@echo "  make all              - Run all tests with coverage"
	@echo "  make clean            - Clean test cache"

all:
	@echo "🚀 Running all tests with coverage..."
	@$(GOTEST) -cover $(PKG) | grep -E "(PASS|FAIL|coverage:|---)" || true

test:
	@echo "🧪 Running all tests..."
	@$(GOTEST) $(PKG)

bench:
	@echo "⚡ Running benchmarks..."
	@$(GOBENCH) $(PKG) 2>&1 | grep -v "INFO worker" | grep -E "(Benchmark|ns/op|PASS|FAIL)" || true

high-throughput:
	@echo "📊 Testing High Throughput Single Publisher..."
	@$(GOTEST) -run TestHighThroughputSinglePublisher $(PKG) | grep -E "(Testing|Throughput|Total|Time|PASS|FAIL|---)" || true

concurrent:
	@echo "🔄 Testing Concurrent Multi-Publisher..."
	@$(GOTEST) -run TestConcurrentMultiPublisher $(PKG) | grep -E "(Testing|Throughput|Total|Time|PASS|FAIL|---)" || true

worker-pool:
	@echo "🏊 Testing Worker Pool Saturation..."
	@$(GOTEST) -run TestWorkerPoolSaturation $(PKG) | grep -E "(Testing|Worker|duration|throughput|PASS|FAIL|---)" || true

memory:
	@echo "💾 Testing Memory Pressure..."
	@$(GOTEST) -run TestMemoryPressure $(PKG) | grep -E "(Testing|Memory|Topics|allocated|PASS|FAIL|---)" || true

system-topics:
	@echo "🔧 Testing System Topics Integrity..."
	@$(GOTEST) -run TestSystemTopicsIntegrity $(PKG) | grep -E "(Testing|verified|PASS|FAIL|---)" || true

clean:
	@echo "🧹 Cleaning test cache..."
	@go clean -testcache
