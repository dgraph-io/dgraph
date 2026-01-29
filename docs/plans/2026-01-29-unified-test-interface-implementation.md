# Unified Test Interface Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan
> task-by-task.

**Goal:** Add a unified `make test` interface with environment variables and helper targets to run
any test type from the top-level Makefile.

**Architecture:** The top-level Makefile routes test requests based on env vars (TAGS > FUZZ >
SUITE). TAGS/FUZZ bypass the t/ runner and call `go test` directly; SUITE delegates to the existing
t/ runner. Helper targets are simple wrappers that set env vars and call `make test`.

**Tech Stack:** GNU Make, Go testing, shell scripting

---

## Task 1: Update Help Target with Auto-Generation

**Files:**

- Modify: `Makefile:119-131` (replace existing help target)

**Step 1: Read current help target**

Review the existing `help` target to understand what we're replacing.

**Step 2: Replace help target with auto-generated version**

Replace the existing help target with:

```makefile
.PHONY: help
help: ## Show available targets and variables
	@echo "Usage: make [target] [VAR=value ...]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "Variables:"
	@echo "  SUITE     Select t/ runner suite (unit, core, systest, vector, ldbc, load, all)"
	@echo "  TAGS      Go build tags - bypasses t/ runner (integration, integration2, upgrade)"
	@echo "  PKG       Limit to specific package (e.g., PKG=systest/export)"
	@echo "  TEST      Run specific test function (e.g., TEST=TestGQLSchema)"
	@echo "  FUZZ      Enable fuzz testing (FUZZ=1)"
	@echo "  FUZZTIME  Fuzz duration per package (default: 300s)"
```

**Step 3: Add ## comments to existing targets**

Update existing targets to have `## ` comments:

```makefile
.PHONY: all
all: dgraph ## Build all targets

.PHONY: dgraph
dgraph: ## Build dgraph binary
	$(MAKE) -w -C $@ all

.PHONY: install
install: ## Install dgraph binary
	...

.PHONY: version
version: ## Show build version info
	...
```

**Step 4: Run make help to verify**

Run: `make help` Expected: Sorted list of targets with descriptions

**Step 5: Commit**

```bash
git add Makefile
git commit -m "feat: add auto-generated help target with ## comments"
```

---

## Task 2: Add Routing Logic to Test Target

**Files:**

- Modify: `Makefile:76-78` (replace existing test target)

**Step 1: Read current test target**

The current test target is:

```makefile
.PHONY: test
test: dgraph-installed local-image
	@$(MAKE) -C t test
```

**Step 2: Replace with routing logic**

Replace with:

```makefile
.PHONY: test
test: dgraph-installed local-image ## Run tests (see 'make help' for options)
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
	$(MAKE) -C t test args="--suite=$(or $(SUITE),all) $(if $(PKG),--pkg=$(PKG)) $(if $(TEST),--test=$(TEST))"
endif
```

**Step 3: Test TAGS routing**

Run: `TAGS=integration2 PKG=types make test --dry-run` Expected: Shows
`go test -v --tags=integration2 ./types/...`

**Step 4: Test SUITE routing (default path)**

Run: `SUITE=unit make test --dry-run` Expected: Shows delegation to
`make -C t test args="--suite=unit"`

**Step 5: Commit**

```bash
git add Makefile
git commit -m "feat: add routing logic for TAGS, FUZZ, SUITE in test target"
```

---

## Task 3: Add Helper Targets

**Files:**

- Modify: `Makefile` (add after test target, before help target)

**Step 1: Add test-unit target**

```makefile
.PHONY: test-unit
test-unit: ## Run unit tests (no Docker required)
	@SUITE=unit $(MAKE) test
```

**Step 2: Add test-core target**

```makefile
.PHONY: test-core
test-core: ## Run core tests
	@SUITE=core $(MAKE) test
```

**Step 3: Add test-integration target**

```makefile
.PHONY: test-integration
test-integration: ## Run integration tests via t/ runner
	@TAGS=integration $(MAKE) test
```

**Step 4: Add test-integration2 target**

```makefile
.PHONY: test-integration2
test-integration2: ## Run integration2 tests via dgraphtest
	@TAGS=integration2 $(MAKE) test
```

**Step 5: Add test-upgrade target**

```makefile
.PHONY: test-upgrade
test-upgrade: ## Run upgrade tests
	@TAGS=upgrade $(MAKE) test
```

**Step 6: Add test-systest target**

```makefile
.PHONY: test-systest
test-systest: ## Run system integration tests
	@SUITE=systest $(MAKE) test
```

**Step 7: Add test-vector target**

```makefile
.PHONY: test-vector
test-vector: ## Run vector search tests
	@SUITE=vector $(MAKE) test
```

**Step 8: Add test-fuzz target**

```makefile
.PHONY: test-fuzz
test-fuzz: ## Run fuzz tests (auto-discovers packages)
	@FUZZ=1 $(MAKE) test
```

**Step 9: Add test-ldbc target**

```makefile
.PHONY: test-ldbc
test-ldbc: ## Run LDBC benchmark tests
	@SUITE=ldbc $(MAKE) test
```

**Step 10: Add test-load target**

```makefile
.PHONY: test-load
test-load: ## Run heavy load tests
	@SUITE=load $(MAKE) test
```

**Step 11: Add test-benchmark target**

```makefile
.PHONY: test-benchmark
test-benchmark: ## Run Go benchmarks
	go test -bench=. -benchmem $(if $(PKG),./$(PKG)/...,./...)
```

**Step 12: Verify make help shows all targets**

Run: `make help` Expected: All test-\* targets appear in sorted order with descriptions

**Step 13: Commit**

```bash
git add Makefile
git commit -m "feat: add helper targets for common test workflows"
```

---

## Task 4: Test the Implementation

**Files:**

- None (testing only)

**Step 1: Test make help output**

Run: `make help` Expected: Clean sorted output with all targets and variables documented

**Step 2: Test TAGS routing with dry-run**

Run:
`TAGS=integration2 PKG=systest/vector TEST=TestVector make test --dry-run 2>&1 | grep "go test"`
Expected: `go test -v --tags=integration2 --run=TestVector ./systest/vector/...`

**Step 3: Test SUITE routing with dry-run**

Run: `SUITE=core PKG=query make test --dry-run 2>&1 | grep "make -C t"` Expected: Contains
`--suite=core --pkg=query`

**Step 4: Test helper target**

Run: `make test-unit --dry-run 2>&1 | grep SUITE` Expected: `SUITE=unit`

**Step 5: Run actual unit test on small package**

Run: `PKG=types TEST=TestConvert make test-unit` Expected: Tests pass

---

## Task 5: Update TESTING.md Documentation

**Files:**

- Modify: `TESTING.md` (add new section after "Prerequisites & Setup")

**Step 1: Add Quick Start section**

After the "Prerequisites & Setup" section (around line 252), add:

````markdown
---

## Quick Start: Running Tests

### Using Make Targets

The simplest way to run tests:

```bash
# Run all tests (default)
make test

# Common shortcuts
make test-unit          # Unit tests only (no Docker)
make test-integration   # Integration tests via t/ runner
make test-integration2  # Integration2 tests via dgraphtest
make test-fuzz          # Fuzz testing (auto-discovers packages)
make test-upgrade       # Upgrade tests
```
````

Run `make help` to see all available targets.

### Using Environment Variables

For more control, use environment variables with `make test`:

| Variable   | Purpose                     | Example                        |
| ---------- | --------------------------- | ------------------------------ |
| `SUITE`    | Select t/ runner suite      | `SUITE=systest make test`      |
| `TAGS`     | Go build tags (bypasses t/) | `TAGS=integration2 make test`  |
| `PKG`      | Limit to specific package   | `PKG=systest/export make test` |
| `TEST`     | Run specific test function  | `TEST=TestGQLSchema make test` |
| `FUZZ`     | Enable fuzz testing         | `FUZZ=1 make test`             |
| `FUZZTIME` | Fuzz duration per package   | `FUZZTIME=60s make test`       |

**Precedence:** `TAGS` > `FUZZ` > `SUITE` (first match wins)

### Examples

```bash
# Run integration2 tests for vector package
TAGS=integration2 PKG=systest/vector make test

# Run upgrade tests for ACL with specific test
TAGS=upgrade TEST=TestACLUpgrade PKG=acl make test

# Run fuzz tests with custom duration
FUZZ=1 FUZZTIME=60s PKG=dql make test

# Run specific test (auto-discovers package)
TEST=TestGQLSchema make test
```

---

````

**Step 2: Verify markdown renders correctly**

Review the section for proper formatting.

**Step 3: Commit**

```bash
git add TESTING.md
git commit -m "docs: add Quick Start section for unified test interface"
````

---

## Task 6: Update Future Improvement Ideas Section

**Files:**

- Modify: `TESTING.md` (update Future Improvement Ideas section)

**Step 1: Move items to Completed Improvements**

In the "Future Improvement Ideas" section, add to "Completed Improvements":

````markdown
- **✅ Unified test interface:** A single `make test` entry point that accepts arguments to run any
  test type (unit, integration, integration2, upgrade, fuzz) with environment variables for control.

- **✅ Example commands that "just work":** The following now work as expected:
  ```bash
  make test SUITE=systest
  make test FUZZ=1 PKG=dql
  make test TAGS=upgrade PKG=acl
  make test TAGS=integration PKG=systest/plugin
  ```
````

````

**Step 2: Remove corresponding items from Remaining Ideas**

Remove the "Unified test interface" and "Example commands" items from the Remaining Ideas section since they're now implemented.

**Step 3: Commit**

```bash
git add TESTING.md
git commit -m "docs: update TESTING.md to mark unified test interface as complete"
````

---

## Task 7: Final Verification and Cleanup

**Files:**

- None (verification only)

**Step 1: Run make help and verify output**

Run: `make help` Expected: Clean, sorted output showing all targets and variables

**Step 2: Verify test-unit works**

Run: `PKG=types make test-unit` Expected: Unit tests pass

**Step 3: Verify TAGS routing works**

Run: `TAGS=integration2 PKG=types make test 2>&1 | head -5` Expected: Shows "Running tests with
tags: integration2" then runs go test

**Step 4: Review all commits**

Run: `git log --oneline -10` Expected: Clean commit history with descriptive messages

**Step 5: Push branch for PR**

Run: `git push -u origin feature/unified-test-interface`

---

## Summary

After completing all tasks, you will have:

1. **Auto-generated `make help`** with sorted targets from `##` comments
2. **Routing logic** in the `test` target handling TAGS, FUZZ, and SUITE
3. **12 helper targets** (test-unit, test-integration, test-integration2, etc.)
4. **Updated TESTING.md** with Quick Start documentation
5. **Clean commit history** ready for PR
