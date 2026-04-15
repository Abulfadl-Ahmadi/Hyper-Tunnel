.PHONY: all build test lint vet clean help baseline

# Makefile for Hybrid Tunnel Integration Project
# Provides repeatable CI commands for both MasterDNS and Spoof-Tunnel

MASTERDNS_DIR := MasterDNS
SPOOF_TUNNEL_DIR := spoof-tunnel

# Default target
all: build test lint vet

# ==============================================================================
# Build Targets
# ==============================================================================

build: build-masterdns build-spoof

build-masterdns:
	@echo "=== Building MasterDNS ==="
	cd $(MASTERDNS_DIR) && go build -o masterdnsvpn-client ./cmd/client
	cd $(MASTERDNS_DIR) && go build -o masterdnsvpn-server ./cmd/server
	@echo "✅ MasterDNS binaries built"

build-spoof:
	@echo "=== Building Spoof-Tunnel ==="
	cd $(SPOOF_TUNNEL_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o spoof ./cmd/spoof/
	@echo "✅ Spoof-Tunnel binary built"

# ==============================================================================
# Test Targets
# ==============================================================================

test: test-masterdns test-spoof
	@echo "✅ All tests passed"

test-masterdns:
	@echo "=== Testing MasterDNS ==="
	cd $(MASTERDNS_DIR) && go test ./...
	@echo "✅ MasterDNS tests passed"

test-spoof:
	@echo "=== Testing Spoof-Tunnel ==="
	cd $(SPOOF_TUNNEL_DIR) && go test -v -race -count=1 ./...
	@echo "✅ Spoof-Tunnel tests passed"

# ==============================================================================
# Lint and Static Analysis Targets
# ==============================================================================

lint: lint-masterdns lint-spoof
	@echo "✅ Lint checks passed"

lint-masterdns:
	@echo "=== Linting MasterDNS ==="
	cd $(MASTERDNS_DIR) && go vet ./...
	@echo "✅ MasterDNS vet passed"

lint-spoof:
	@echo "=== Linting Spoof-Tunnel ==="
	cd $(SPOOF_TUNNEL_DIR) && go vet ./...
	@echo "✅ Spoof-Tunnel vet passed"

vet: lint
	@echo "✅ All vet checks passed"

# ==============================================================================
# Baseline and Performance Targets
# ==============================================================================

baseline: build test
	@echo "=== Baseline Report ==="
	@echo "Baseline metrics can be found in: docs/testing/baseline.md"
	@echo ""
	@echo "To populate baseline measurements:"
	@echo "  1. Run build and test targets (completed above)"
	@echo "  2. Execute custom benchmark harnesses (to be implemented in Phase 1)"
	@echo "  3. Update docs/testing/baseline.md with [TBD] values"
	@echo ""
	@echo "For loss tolerance testing, use:"
	@echo "  tc qdisc add dev lo root netem loss X%"
	@echo "  # Run tests"
	@echo "  tc qdisc del dev lo root"

# ==============================================================================
# Cleanup Targets
# ==============================================================================

clean: clean-masterdns clean-spoof
	@echo "✅ All build artifacts cleaned"

clean-masterdns:
	@echo "=== Cleaning MasterDNS ==="
	cd $(MASTERDNS_DIR) && go clean
	rm -f $(MASTERDNS_DIR)/masterdnsvpn-client $(MASTERDNS_DIR)/masterdnsvpn-server
	@echo "✅ MasterDNS cleaned"

clean-spoof:
	@echo "=== Cleaning Spoof-Tunnel ==="
	cd $(SPOOF_TUNNEL_DIR) && go clean
	rm -f $(SPOOF_TUNNEL_DIR)/spoof
	@echo "✅ Spoof-Tunnel cleaned"

# ==============================================================================
# Documentation and Info Targets
# ==============================================================================

help:
	@echo "Hybrid Tunnel Integration Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available Targets:"
	@echo "  all              - Build, test, lint, and vet (default)"
	@echo "  build            - Build both MasterDNS and Spoof-Tunnel"
	@echo "  build-masterdns  - Build MasterDNS only"
	@echo "  build-spoof      - Build Spoof-Tunnel only"
	@echo "  test             - Run all unit tests"
	@echo "  test-masterdns   - Test MasterDNS only"
	@echo "  test-spoof       - Test Spoof-Tunnel only"
	@echo "  lint             - Run linters (vet) on all projects"
	@echo "  lint-masterdns   - Lint MasterDNS only"
	@echo "  lint-spoof       - Lint Spoof-Tunnel only"
	@echo "  vet              - Run go vet (alias for lint)"
	@echo "  baseline         - Display baseline metrics (see docs/testing/baseline.md)"
	@echo "  clean            - Remove all build artifacts"
	@echo "  clean-masterdns  - Clean MasterDNS artifacts only"
	@echo "  clean-spoof      - Clean Spoof-Tunnel artifacts only"
	@echo "  help             - Display this help message"
	@echo ""
	@echo "Documentation:"
	@echo "  - Phase progress: PHASE_NOTES.md"
	@echo "  - Decisions log: DECISIONS.md"
	@echo "  - Architecture: docs/architecture/current-state.md, target-state.md"
	@echo "  - Test matrix: docs/testing/test-matrix.md"
	@echo "  - Baseline: docs/testing/baseline.md"
	@echo ""
	@echo "Examples:"
	@echo "  make               # Full build, test, lint"
	@echo "  make build test    # Build and test only"
	@echo "  make clean         # Clean all artifacts"
