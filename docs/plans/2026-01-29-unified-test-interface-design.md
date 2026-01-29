# Unified Test Interface Design

**Date:** 2026-01-29 **Status:** Approved **Author:** Design session with Claude

## Overview

Provide a single `make test` entry point with environment variables and helper targets to run any
test type (unit, integration, integration2, upgrade, fuzz, benchmark).

## Goals

1. **Unified entry point** - `make test` handles all test types
2. **Environment variable control** - `TAGS`, `SUITE`, `PKG`, `TEST`, `FUZZ` for flexible filtering
3. **Convenience targets** - `make test-unit`, `make test-integration2`, etc. for common workflows
4. **Auto-generated help** - `make help` shows all targets from `##` comments
5. **Backwards compatible** - Default behavior runs existing test suite

## Design Decisions

### 1. Routing Logic Location

**Decision:** Top-level Makefile **Rationale:** Natural entry point, shell conditionals are simpler
than extending Go code, keeps concerns separated (Makefile handles routing, t.go handles Docker
Compose orchestration).

### 2. Primary Switch Mechanism

**Decision:** `TAGS` environment variable bypasses t/ runner and runs `go test` directly
**Rationale:** Clean interface for running integration2/upgrade tests that don't need Docker Compose
orchestration.

### 3. Default Behavior

**Decision:** Run all test suites **Rationale:** Comprehensive testing by default; developers can
use helper targets for focused testing.

### 4. Fuzz Test Discovery

**Decision:** Auto-discover packages with fuzz tests and run each sequentially **Rationale:**
Future-proof - new fuzz tests are automatically included without Makefile changes.

### 5. Validation

**Decision:** No Makefile validation - let downstream tools handle it **Rationale:** Keep Makefile
simple; t/ runner already validates suite names.

## Environment Variables

| Variable   | Purpose                     | Example                        |
| ---------- | --------------------------- | ------------------------------ |
| `SUITE`    | Select t/ runner suite      | `SUITE=systest make test`      |
| `TAGS`     | Go build tags (bypasses t/) | `TAGS=integration2 make test`  |
| `PKG`      | Limit to specific package   | `PKG=systest/export make test` |
| `TEST`     | Run specific test function  | `TEST=TestGQLSchema make test` |
| `FUZZ`     | Enable fuzz testing         | `FUZZ=1 make test`             |
| `FUZZTIME` | Fuzz duration per package   | `FUZZTIME=60s make test`       |

**Precedence:** `TAGS` > `FUZZ` > `SUITE` (first match wins)

## Helper Targets

| Target                   | Equivalent          | Description                           |
| ------------------------ | ------------------- | ------------------------------------- |
| `make test`              | (runs all)          | Run all tests (default)               |
| `make test-benchmark`    | `go test -bench=.`  | Run Go benchmarks                     |
| `make test-core`         | `SUITE=core`        | Run core tests                        |
| `make test-fuzz`         | `FUZZ=1`            | Run fuzz tests (auto-discovers)       |
| `make test-integration`  | `TAGS=integration`  | Run integration tests via t/ runner   |
| `make test-integration2` | `TAGS=integration2` | Run integration2 tests via dgraphtest |
| `make test-ldbc`         | `SUITE=ldbc`        | Run LDBC benchmark tests              |
| `make test-load`         | `SUITE=load`        | Run heavy load tests                  |
| `make test-systest`      | `SUITE=systest`     | Run system integration tests          |
| `make test-unit`         | `SUITE=unit`        | Run unit tests (no Docker)            |
| `make test-upgrade`      | `TAGS=upgrade`      | Run upgrade tests                     |
| `make test-vector`       | `SUITE=vector`      | Run vector search tests               |

## Implementation

### Makefile Routing Logic

```makefile
.PHONY: test
test: dgraph-installed local-image ## Run tests (use 'make help' for options)
ifdef TAGS
	@echo "Running tests with tags: $(TAGS)"
	go test -v --tags=$(TAGS) \
		$(if $(TEST),--run=$(TEST)) \
		$(if $(PKG),./$(PKG)/...,./...)
else ifdef FUZZ
	@echo "Discovering and running fuzz tests..."
ifdef PKG
	go test -v -fuzz=Fuzz -fuzztime=$(or $(FUZZTIME),300s) ./$(PKG)/...
else
	@grep -r "^func Fuzz" --include="*_test.go" -l . 2>/dev/null | \
		xargs -I{} dirname {} | sort -u | while read dir; do \
			echo "Fuzzing $$dir..."; \
			go test -v -fuzz=Fuzz -fuzztime=$(or $(FUZZTIME),300s) ./$$dir/...; \
		done
endif
else
	@echo "Running test suite: $(or $(SUITE),all)"
	$(MAKE) -C t test args="--suite=$(or $(SUITE),all) \
		$(if $(PKG),--pkg=$(PKG)) \
		$(if $(TEST),--test=$(TEST))"
endif
```

### Helper Target Pattern

```makefile
.PHONY: test-unit
test-unit: ## Run unit tests (no Docker)
	SUITE=unit $(MAKE) test

.PHONY: test-integration2
test-integration2: ## Run integration2 tests via dgraphtest
	TAGS=integration2 $(MAKE) test

.PHONY: test-fuzz
test-fuzz: ## Run fuzz tests (auto-discovers packages)
	FUZZ=1 $(MAKE) test
```

### Auto-generated Help

```makefile
.PHONY: help
help: ## Show available targets
	@echo "Usage: make [target] [VAR=value ...]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "Variables: SUITE, TAGS, PKG, TEST, FUZZ, FUZZTIME"
```

## Example Usage

```bash
# Simple cases - use helper targets
make test                  # Run all tests
make test-unit             # Fast feedback, no Docker
make test-integration2     # Run dgraphtest-based tests
make test-fuzz             # Run all fuzz tests

# Complex cases - use env vars directly
TAGS=integration2 PKG=systest/vector make test
TAGS=upgrade TEST=TestACLUpgrade PKG=acl make test
FUZZ=1 FUZZTIME=60s PKG=dql make test

# See all options
make help
```

## Example `make help` Output

```
Usage: make [target] [VAR=value ...]

Targets:
  help                 Show available targets
  test                 Run tests (use 'make help' for options)
  test-benchmark       Run Go benchmarks
  test-core            Run core tests
  test-fuzz            Run fuzz tests (auto-discovers packages)
  test-integration     Run integration tests via t/ runner
  test-integration2    Run integration2 tests via dgraphtest
  test-ldbc            Run LDBC benchmark tests
  test-load            Run heavy load tests
  test-systest         Run system integration tests
  test-unit            Run unit tests (no Docker)
  test-upgrade         Run upgrade tests
  test-vector          Run vector search tests

Variables: SUITE, TAGS, PKG, TEST, FUZZ, FUZZTIME
```

## Documentation Updates

Update TESTING.md to add a "Quick Start: Running Tests" section after "Prerequisites & Setup" that
documents the new interface.

## Implementation Steps

1. Update top-level `Makefile` with routing logic and helper targets
2. Update `make help` target with auto-generation from `##` comments
3. Update TESTING.md with new "Quick Start" section
4. Test all helper targets and env var combinations
5. Update CONTRIBUTING.md if needed
