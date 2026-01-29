# Dgraph Testing Guide

## Table of Contents

- [Overview](#overview)
- [Test Types and Build Tags](#test-types-and-build-tags)
- [Module Structure](#module-structure)
- [Prerequisites & Setup](#prerequisites--setup)
- [Quick Decision Guide](#quick-decision-guide)
- [Unit Tests](#unit-tests)
- [Integration Tests via t/ Runner](#integration-tests-via-t-runner)
- [Running Integration Tests](#running-integration-tests)
- [Integration v2 Tests (integration2 build tag)](#integration-v2-tests-integration2-build-tag)
- [Upgrade Tests (upgrade build tag)](#upgrade-tests-upgrade-build-tag)
- [Writing Tests in Go (Dgraph Conventions)](#writing-tests-in-go-dgraph-conventions)
- [Fuzz Tests](#fuzz-tests)
- [Future Improvement Ideas](#future-improvement-ideas)

---

## Overview

Dgraph employs a ~~complex~~ sophisticated testing framework with extensive test coverage. The
codebase contains >200 test files with >2,000 test functions and benchmark functions across multiple
packages and modules.

This guide helps engineers navigate testing in the Dgraph codebase.

If you're making a change, you should be able to:

- Choose the appropriate test type for your change (unit, integration, systest, upgrade, or fuzz)
- Identify where to add new tests in the repo
- Write Go tests that follow Dgraph's patterns
- Execute tests locally, single test, a whole package, or a named suite
- Debug CI failures by reproducing them on your own machine

---

## Test Types and Build Tags

The testing framework uses Go build tags to conditionally compile tests that are more costly to run.

We distinguish the following types of tests:

### 1. Unit Tests

- **Purpose:** Test individual functions and components in isolation
- **Examples:** `dql/dql_test.go`, `types/value_test.go`, `schema/parse_test.go`
- **Build tags:** No build tags - Standard Go unit tests
- Unit tests run without any cluster and are usually fast.

### 2. Integration Tests

- **Purpose:** Test component interactions and full system workflows
- **Examples:** `acl/acl_test.go`, `worker/worker_test.go`, `query/query0_test.go`
- **Build tag:** `//go:build integration`

### 3. Upgrade Tests

- **Purpose:** Test database upgrade scenarios and migrations
- **Examples:** `acl/upgrade_test.go`, `worker/upgrade_test.go`
- **Build tag:** `//go:build upgrade`

### 4. Benchmark Tests

- **Function prefix:** `Benchmark`
- **Purpose:** Performance testing and optimization
- **Examples:** `query/benchmark_test.go`, `dql/bench_test.go`

### 5. Cloud Tests (DEPRECATED)

- **Purpose:** Test cloud-specific functionality
- **Examples:** `query/cloud_test.go`, `systest/cloud/cloud_test.go`
- **Build tag:** `//go:build cloud`

Integration, Upgrade and Benchmark tests require a running Dgraph cluster (Docker) and come in two
forms: tests driven by the `t/` runner, and tests using the `dgraphtest` package, which provides
programmatic control over local Dgraph clusters. Most newer integration2 and upgrade tests rely on
`dgraphtest`.

> **Note:** The `testutil` package is being phased out. For new tests, prefer `dgraphtest` (cluster
> management) and `dgraphapi` (client operations). The `testutil` package is maintained for backward
> compatibility with existing tests only.

---

## Module Structure

The main module is `github.com/hypermodeinc/dgraph`

The codebase is organized into several key packages:

### Core Packages

| Package      | Description                                |
| ------------ | ------------------------------------------ |
| `acl`        | Access Control Lists and authentication    |
| `algo`       | Algorithms and data structures             |
| `audit`      | Audit logging functionality                |
| `backup`     | Backup and restore operations              |
| `chunker`    | Data chunking and parsing                  |
| `codec`      | Encoding/decoding utilities                |
| `conn`       | Connection management                      |
| `dgraph`     | Main Dgraph binary and commands            |
| `dgraphapi`  | Dgraph API client                          |
| `dgraphtest` | Testing utilities                          |
| `dql`        | Dgraph Query Language parser and processor |
| `edgraph`    | GraphQL endpoint                           |
| `filestore`  | File storage abstraction                   |
| `graphql`    | GraphQL implementation                     |
| `lex`        | Lexical analysis                           |
| `posting`    | Posting list management                    |
| `query`      | Query processing engine                    |
| `raftwal`    | Raft write-ahead log                       |
| `schema`     | Schema management                          |
| `systest`    | System integration tests                   |
| `testutil`   | Testing utilities                          |
| `tok`        | Tokenization and text processing           |
| `types`      | Data type definitions                      |
| `upgrade`    | Database upgrade utilities                 |
| `worker`     | Worker processes                           |
| `x`          | Common utilities                           |

---

## Prerequisites & Setup

Before running tests, ensure you have the following installed and configured.

> **TL;DR:** On a fresh checkout, just run `make install` followed by `make test`. The build system
> automatically handles OS detection, builds the correct binaries, and validates dependencies.

### Automatic Dependency Checking

The test framework includes scripts that check for required dependencies and can optionally
auto-install them:

```bash
# Check all dependencies (run from t/ directory)
cd t && make check

# Auto-install missing dependencies
AUTO_INSTALL=true make check
```

The check scripts validate:

- Go version (1.21+)
- Docker and Docker Compose versions and memory allocation
- gotestsum installation
- ack installation
- Dgraph binary existence and correct architecture

### Required Tools

> **Note:** You don't need to install these manually. Running `AUTO_INSTALL=true make check` from
> the `t/` directory (or `AUTO_INSTALL=true make test` from the repo root) automatically installs
> missing dependencies. The commands below are listed for reference.

#### 1. Go (1.21+)

```bash
go version  # Verify Go is installed
```

#### 2. Docker (20.10 or higher) & Docker Compose (v2)

```bash
docker --version
docker compose version

# Allocate sufficient memory: 4GB minimum, 8GB recommended
# Docker Desktop → Settings → Resources → Memory
```

#### 3. GOPATH Configuration

```bash
# Set GOPATH (if not already set)
export GOPATH=$(go env GOPATH)
echo $GOPATH  # Should output something like /Users/you/go

# Add to your shell profile (~/.zshrc, ~/.bashrc)
export GOPATH=$(go env GOPATH)
export PATH=$PATH:$GOPATH/bin
```

#### 4. gotestsum (Required for t/ runner)

```bash
go install gotest.tools/gotestsum@latest

# Verify installation
gotestsum --version
```

#### 5. ack (required for test framework t/)

```bash
brew install ack
```

#### 6. Dgraph Binary (Required for integration/upgrade tests)

```bash
# Build and install Dgraph binary to $GOPATH/bin
make install

# Verify installation
which dgraph  # Should show $GOPATH/bin/dgraph
dgraph version
```

> **Note:** The `t/` runner's Docker Compose files mount the dgraph binary into containers at
> startup. On macOS, binaries are read from `$GOPATH/linux_<arch>/dgraph`; on Linux, from
> `$GOPATH/bin/dgraph`. Simply run `make install` after code changes — no Docker image rebuild
> needed.

### Quick Setup

The build system now handles most setup automatically. On both Linux and macOS:

```bash
# Install dependencies (optional - auto-installs if missing)
cd t && AUTO_INSTALL=true make check && cd ..

# Build dgraph binary (automatically handles Linux binary on macOS)
make install

# Run tests (builds Docker image and runs test suite)
make test
```

That's it! The `make install` command:

- On **Linux**: Installs dgraph to `$GOPATH/bin/dgraph`
- On **macOS**: Installs native binary to `$GOPATH/bin/dgraph` AND Linux binary to
  `$GOPATH/linux_<arch>/dgraph`

The Docker Compose files automatically use the correct binary path via the `LINUX_GOBIN` environment
variable.

### macOS Notes

The build system now automatically handles cross-compilation for macOS users:

- `make install` builds both native macOS and Linux binaries automatically
- Linux binaries are stored in `$GOPATH/linux_<arch>/dgraph`
- Docker Compose files use `${LINUX_GOBIN:-$GOPATH/bin}` to find the correct binary
- No manual binary swapping required!

After code changes, simply run `make install` again — it handles everything.

### Special Case: Bulk/Live Loader Tests on macOS

**Background:** Bulk and live loader tests (`systest/bulk_live/`) execute `dgraph bulk` and
`dgraph live` commands locally on your machine (not inside Docker).

**Good news:** Since `make install` now builds both binaries on macOS, you have:

1. Native macOS binary at `$GOPATH/bin/dgraph` (used for local commands)
2. Linux binary at `$GOPATH/linux_<arch>/dgraph` (used by Docker containers)

### Verify Your Setup

#### Step 1: Verify unit tests work

Use `go test` to run one easy test on types package:

```bash
go test -v ./types/... -run TestConvert
```

**Expected output:**

```text
=== RUN   TestConvertToDefault
--- PASS: TestConvertToDefault (0.00s)
...
=== RUN   TestConvertToGeoJson_PolyError2
--- PASS: TestConvertToGeoJson_PolyError2 (0.00s)
PASS
ok      github.com/dgraph-io/dgraph/v25/types   (cached)
?       github.com/dgraph-io/dgraph/v25/types/facets    [no test files]
```

#### Step 2: Verify integration test setup

> **Note:** Start Docker Desktop before running integration or upgrade tests

```bash
cd t && go build . && ./t --test=TestGQLSchema
```

If both pass, you're ready to run all test types!

---

## Quick Decision Guide

Use this section to quickly determine what test to write and where to place it.

### Testing Philosophy

Cover as many scenarios as possible. A good PR includes tests for:

- The happy path (expected behaviour)
- Edge cases (empty inputs, boundary values, special characters)
- Error conditions (invalid inputs, failure modes)

Use a layered testing approach. Aim for broad coverage with unit tests to validate individual
functions and quickly identify failures, and complement them with integration and end-to-end tests
for cluster-dependent behavior and real-world scenarios. Each test type is important and they should
be mutually reinforcing.

### Unit Tests: Test everything you can without a running Dgraph cluster

Unit tests run without a Dgraph cluster. They test pure logic in isolation.

- Place it in the same package as the code you changed
- File name: `*_test.go` next to the source file
- No build tag needed

**Example:** Changing `worker/export.go` → add test in `worker/export_test.go`

### Integration Tests: cover scenarios requiring a running Dgraph cluster

Testing individual functions and components in isolation is usually not enough. Integration Tests
test component interactions and full system workflows. They require a running Dgraph cluster.

### What are Go build tags?

Go build tags are special comments at the top of a file (for example, `//go:build integration`) that
instruct the Go toolchain when to compile that file. When you run tests with
`go test -tags=integration`, only test files without a build tag (default) or with a matching tag
are compiled and executed.

We use build tags to exclude expensive or environment-dependent tests (like `integration`,
`integration2`, and `upgrade`) from the default `go test ./...` run, while allowing you to opt in to
them when needed.

| Build Tag            | Purpose                                                           |
| -------------------- | ----------------------------------------------------------------- |
| `integration`        | Standard integration tests requiring a Docker cluster             |
| `integration2`       | Integration tests using Docker Go client via `dgraphtest` package |
| `upgrade`            | Tests for upgrade scenarios between dgraph versions               |
| `cloud` (deprecated) | Tests running against cloud environment                           |

### Test Placement Guide

| If you're testing...                            | Test type    | Build tag      | Where to place                                 |
| ----------------------------------------------- | ------------ | -------------- | ---------------------------------------------- |
| Query or mutation logic                         | Integration  | `integration`  | Existing package or `systest/`                 |
| Backup / Restore                                | Integration  | `integration`  | `systest/backup/` or `systest/online-restore/` |
| Export                                          | Integration  | `integration`  | `systest/export/`                              |
| Live loader / Bulk loader                       | Integration  | `integration`  | `systest/bulk_live/` or `systest/loader/`      |
| Multi-tenancy / Namespaces                      | Integration  | `integration`  | `systest/multi-tenancy/`                       |
| Vector / Embeddings                             | Integration  | `integration`  | `systest/vector/`                              |
| GraphQL schema or endpoints                     | Integration  | `integration`  | `graphql/e2e/`                                 |
| ACL / Auth                                      | Integration  | `integration`  | `acl/` or `systest/acl/`                       |
| Upgrade from older version                      | Upgrade      | `upgrade`      | Same package with `//go:build upgrade`         |
| Fine-grained cluster control (start/stop nodes) | Integration2 | `integration2` | `systest/integration2/` or relevant package    |

### Quick Examples

- **I fixed a bug in query parsing (no cluster needed to fully validate)** → Unit test in
  `query/*_test.go`, no build tag

- **I fixed a bug in export that affects vector data** → Integration test in `systest/vector/`, use
  `dgraphtest.LocalCluster`, tag: `//go:build integration`

- **I changed backup behaviour** → Integration test in `systest/backup/`, tag:
  `//go:build integration`

- **I need to test behaviour after upgrading from v23 to main** → Upgrade test in relevant package,
  tag: `//go:build upgrade`

- **I changed GraphQL admin endpoint** → Integration test in `graphql/e2e/`, tag:
  `//go:build integration`

### Rules of Thumb

1. **Maximize unit test coverage.** If you can fully test it without a cluster - unit tests only. If
   it can't be tested at all without a cluster, integration tests only. Otherwise add a mix of both
   unit and integration tests – unit tests for what parts can be tested in isolation and integration
   tests for the remainder.

2. **Cover multiple scenarios.** Don't just test the happy path—include edge cases and error
   conditions.

3. **Use table-driven tests.** One test function with multiple cases beats many separate functions.

4. **No flaky tests.** Avoid `time.Sleep()`; use polling, retries, or explicit waits with timeouts.

5. **Follow existing patterns.** Look at nearby `*_test.go` files and match their style.

---

## Unit Tests

### Running Go Tests

#### Basic Command Structure

```bash
go test [flags] [package] [test-filter]
```

#### Common Flags

- `-v` (verbose): Shows detailed output for each test
- `-run <pattern>`: Run only tests matching the pattern (regex)

#### Package Paths

- `./types/`: Single package
- `./types/...`: Package and all subpackages recursively

#### Examples

```bash
# Run all tests in types package
go test ./types/

# Run all tests in types and subpackages
go test ./types/...

# Run specific test with verbose output
go test -v ./types/... -run TestConvert
```

### Identifying a unit test

- No `//go:build` tag at the top of the file = unit test
- Files with `//go:build integration` are NOT unit tests

### Where to place unit tests

Place `*_test.go` next to the code being tested:

| Code in               | Test in                    |
| --------------------- | -------------------------- |
| `types/conversion.go` | `types/conversion_test.go` |
| `dql/parser.go`       | `dql/parser_test.go`       |
| `schema/parse.go`     | `schema/parse_test.go`     |

### When to write a unit test

- Parsing logic
- Data conversions
- Utility functions
- Algorithms
- Any code that doesn't need cluster access

---

## Integration Tests via t/ Runner

The `t/` runner orchestrates Docker-based integration tests. It spins up Dgraph clusters using
Docker Compose and runs tests tagged with `integration`.

### How it works

1. Uses Dgraph binary from `$GOPATH/bin/dgraph`
2. Spins up cluster via `docker-compose.yml` (package-specific or default)
3. Runs tests with `--tags=integration`
4. Tears down cluster after completion

### Test Suites

A suite is a named group of test packages that can be run together with the `--suite` flag.

| Suite     | Purpose                               | Packages/Tests Included                                                                       |
| --------- | ------------------------------------- | --------------------------------------------------------------------------------------------- |
| `unit`    | Default suite for regular development | All packages except ldbc and load (includes query, mutation, schema, GraphQL, ACL, worker)    |
| `core`    | Core Dgraph functionality             | Query, mutation, schema, GraphQL e2e, ACL, TLS, worker (excludes systest, ldbc, vector, load) |
| `systest` | Real workflows and system-level tests | Backup/restore, export, multi-tenancy, online-restore, audit, CDC, group-delete               |
| `vector`  | Vector search functionality           | Vector index, similarity search, HNSW, vector backup/restore (`systest/vector/`)              |
| `ldbc`    | Benchmark queries                     | LDBC benchmark suite (`systest/ldbc/`)                                                        |
| `load`    | Heavy data loading scenarios          | 21million, 1million, bulk_live, bgindex, bulkloader                                           |
| `all`     | Everything                            | Runs all test suites                                                                          |

### Docker Compose Discovery

The runner looks for `docker-compose.yml`:

1. First in the test package directory (e.g., `systest/export/docker-compose.yml`)
2. Falls back to default: `dgraph/docker-compose.yml`

Tests with custom compose files run in isolated clusters.

### Common Commands

```bash
# Build the runner first
cd t && go build .

# Run a suite
./t --suite=core

# Run specific package
./t --pkg=systest/export

# Run single test
./t --test=TestExportAndLoadJson

# Keep cluster after test (for debugging)
./t --pkg=systest/export --keep

# Cleanup all test containers
./t -r
```

### Key Flags

| Flag          | Description                                                        |
| ------------- | ------------------------------------------------------------------ |
| `--suite=X`   | Select test suite(s): all, ldbc, load, unit, systest, vector, core |
| `--pkg=X`     | Run specific package                                               |
| `--test=X`    | Run specific test function                                         |
| `-j=N`        | Concurrency (default: 1)                                           |
| `--keep`      | Keep cluster running after tests                                   |
| `-r`          | Remove all test containers                                         |
| `--skip-slow` | Skip slow packages                                                 |

---

## Running Integration Tests

### Method 1: Using t/ Runner

The `t/` runner manages cluster lifecycle automatically.

```bash
# Build runner
cd t && go build .

# Run all tests in a package
./t --pkg=systest/export

# Run single test
./t --test=TestExportAndLoadJson

# Keep cluster running after tests (for debugging)
./t --pkg=systest/export --keep
```

### Method 2: Manual Cluster + go test

For fine-grained control, manually start a cluster and run tests against it.

#### Step 1: Start cluster with Docker Compose

```bash
# Start default cluster with a custom prefix
docker compose -f dgraph/docker-compose.yml -p mytest up -d

# Or start package-specific cluster
docker compose -f systest/export/docker-compose.yml -p mytest up -d
```

#### Step 2: Set environment variable and run tests

```bash
# Set the prefix (tells testutil which cluster to use)
export TEST_DOCKER_PREFIX=mytest

# Run all tests in package
go test -v --tags=integration ./systest/export/...

# Run single test
go test -v --tags=integration --run '^TestExportAndLoadJson$' ./systest/export/

# Run multiple specific tests
go test -v --tags=integration --run 'TestExport.*' ./systest/export/
```

#### Step 3: Cleanup

```bash
docker compose -f dgraph/docker-compose.yml -p mytest down -v
```

### Method 3: Use Existing Cluster with t/ Runner

```bash
# Start cluster manually first
docker compose -f dgraph/docker-compose.yml -p myprefix up -d

# Run tests against it (no cluster restart)
cd t && ./t --prefix=myprefix --pkg=systest/export

# Cluster stays running after tests
```

### Running Multiple Tests

Using `go test` regex:

```bash
export TEST_DOCKER_PREFIX=mytest

# All tests matching pattern
go test -v --tags=integration --run 'TestExport' ./systest/export/

# Multiple test names
go test -v --tags=integration --run 'TestExportAndLoad|TestExportSchema' ./systest/export/
```

Using `t/` runner:

```bash
# Run all tests in multiple packages
./t --pkg=systest/export,systest/backup/filesystem

# Run entire suite
./t --suite=systest
```

### Key Environment Variables

| Variable              | Purpose                            | Set by              |
| --------------------- | ---------------------------------- | ------------------- |
| `TEST_DOCKER_PREFIX`  | Docker Compose prefix for cluster  | t/ runner or manual |
| `TEST_DATA_DIRECTORY` | Path to test data files            | t/ runner           |
| `GOPATH`              | Required for finding dgraph binary | User                |

---

## Integration v2 Tests (integration2 build tag)

Uses `dgraphtest` package for programmatic cluster control via Docker Go client.

### Why It Exists

- **Problem:** t/ runner can't handle upgrade tests, individual node control, or version switching
- **Solution:** Full programmatic control over cluster lifecycle through Go API
- **Use when:** Testing upgrades, node failures, or needing precise cluster state control

> **Important:** `dgraphtest` and `dgraphapi` are the future direction for Dgraph testing. New tests
> should use these packages instead of `testutil`. The `testutil` package is being retired and
> maintained only for backward compatibility with existing tests.

### Key Differences from t/ Runner

| Feature                 | t/ runner      | integration2                   |
| ----------------------- | -------------- | ------------------------------ |
| Cluster management      | docker-compose | Docker Go client               |
| Version switching       | No             | Yes                            |
| Individual node control | No             | Yes (Start/Stop/Kill per node) |
| Upgrade testing         | No             | Yes                            |
| Build tag               | `integration`  | `integration2`                 |

### How to Run

```bash
# Build your local binary first
make install

# Run tests
go test -v --tags=integration2 ./systest/integration2/
go test -v --tags=integration2 --run '^TestName$' ./pkg/
```

### Version & Binary Management

**Automatic version handling:**

- Clones Dgraph repo to `/tmp/dgraph-repo-*` on first run
- Checks out requested version (tag/commit)
- Builds binary with `make dgraph` (GOOS=linux)
- Caches in `dgraphtest/binaries/dgraph_<version>`

**Version formats:**

- `"local"` - uses `$GOPATH/bin/dgraph` (default)
- `"v23.0.1"` - git tag
- `"4fc9cfd"` - commit hash

First run is slow (builds binaries), subsequent runs reuse cache.

### The dgraphapi Package

`dgraphapi` provides high-level client wrappers for interacting with Dgraph in tests.

**Two client types:**

#### GrpcClient - For DQL operations

- Wraps `dgo.Dgraph`
- Handles queries, mutations, schema operations
- Login/authentication
- Namespace operations
- UID assignment

#### HTTPClient - For admin/HTTP operations

- Backup operations (full and incremental)
- Restore operations
- Namespace management (add/delete)
- Snapshot management
- Health checks
- State endpoint queries
- GraphQL operations
- Export operations

Both clients support authentication and multi-tenancy (namespace-aware operations).

### Gotchas & Important Notes

#### 1. Prerequisites

- `$GOPATH/bin/dgraph` must exist for "local" version
- `GOPATH` environment variable must be set
- Docker with sufficient resources (4GB+ memory)

#### 2. Always cleanup

- Defer cleanup after cluster creation
- Defer client cleanup after getting clients
- Cleanup removes containers, networks, volumes

#### 3. Authentication required

- Both clients need login for ACL-enabled clusters
- Default credentials: `groot` / `password`
- Must specify namespace (typically root namespace = 0)

#### 4. Performance expectations

- First run: 3-5 minutes (clone + build)
- Subsequent runs: normal test speed (reuses binaries)
- Binary cache shared across parallel tests safely

#### 5. When NOT to use integration2

- Simple query/mutation tests → use t/ runner (faster)
- Don't need version switching → use t/ runner
- Don't need individual node control → use t/ runner

### Bonus: Using dgraphapi with Your Local Cluster

The `dgraphapi` package can work with any running Dgraph cluster. If no Docker prefix is detected
(no `TEST_DOCKER_PREFIX` env var), it falls back to localhost ports.

**Default fallback ports:**

- Alpha gRPC: `localhost:9080`
- Alpha HTTP: `localhost:8080`
- Zero gRPC: `localhost:5080`
- Zero HTTP: `localhost:6080`

**Use case:** Write quick Go scripts to interact with your local development cluster instead of
using Postman for repetitive tasks.

**Benefits:**

- Automate repetitive admin operations
- Test admin workflows quickly
- Reuse test helpers for local development
- Type-safe operations instead of manual JSON crafting

This is especially useful for testing backup/restore, namespace operations, or complex mutation
sequences during development.

**Example test:**

```go
func TestLocalCluster(t *testing.T) {
    c := dgraphtest.NewComposeCluster()

    gc, cleanup, err := c.Client()
    require.NoError(t, err)
    defer cleanup()

    require.NoError(t, gc.SetupSchema(testSchema))

    numVectors := 9
    rdfs, _ := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
    mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
    _, err = gc.Mutate(mu)
    require.NoError(t, err)
}
```

---

## Upgrade Tests (upgrade build tag)

Tests that verify Dgraph behaviour when upgrading from one version to another.

### Why Upgrade Tests Matter

- Ensure backward compatibility across versions
- Catch breaking changes in data format, schema, or behaviour
- Validate that existing data survives upgrades
- Test real-world upgrade workflows customers use

### Build Tag

```go
//go:build upgrade

package main

func TestUpgradeFromV23(t *testing.T) {
    // Start with old version
    conf := dgraphtest.NewClusterConfig().WithVersion("v23.0.1")
    // ... test upgrade to "local" ...
}
```

### Upgrade Strategies

| Strategy        | How it works                               | Use case                                 |
| --------------- | ------------------------------------------ | ---------------------------------------- |
| `BackupRestore` | Take backup on old version, restore on new | Most common customer upgrade path        |
| `InPlace`       | Stop cluster, swap binary, restart         | Fast upgrade, tests binary compatibility |
| `ExportImport`  | Export from old, import to new             | Migration across major versions          |

Specified when calling `c.Upgrade()`:

```go
c.Upgrade("local", dgraphtest.BackupRestore)
c.Upgrade("local", dgraphtest.InPlace)
c.Upgrade("local", dgraphtest.ExportImport)
```

### Version Combinations

Controlled by `DGRAPH_UPGRADE_MAIN_ONLY` environment variable:

**`DGRAPH_UPGRADE_MAIN_ONLY=true` (default):**

- Tests only latest stable → HEAD
- Example: v24.0.0 → local
- Runs in PR CI (fast)

**`DGRAPH_UPGRADE_MAIN_ONLY=false`:**

- Tests many historical versions → HEAD
- Includes v21, v22, v23, v24 releases
- Includes specific cloud commits
- Runs in scheduled CI (comprehensive but slow)

### Running Upgrade Tests

**Run all upgrade tests:**

```bash
# Build your local binary first
make install

# Run with main combos only (fast)
go test -v --tags=upgrade ./...

# Run with all version combos (slow, 30min+)
DGRAPH_UPGRADE_MAIN_ONLY=false go test -v --tags=upgrade ./...
```

**Run specific package:**

```bash
go test -v --tags=upgrade ./systest/mutations-and-queries/
go test -v --tags=upgrade ./acl/
go test -v --tags=upgrade ./worker/
```

**Run single test:**

```bash
go test -v --tags=upgrade -run '^TestUpgradeName$' ./pkg/
```

### Where Upgrade Tests Live

| Package                          | Tests                             |
| -------------------------------- | --------------------------------- |
| `systest/mutations-and-queries/` | Data preservation across upgrades |
| `systest/multi-tenancy/`         | Namespace/ACL upgrade behaviour   |
| `systest/plugin/`                | Custom plugin compatibility       |
| `acl/`                           | ACL schema and permissions        |
| `worker/`                        | Worker-level upgrade logic        |
| `query/`                         | Query behaviour consistency       |

---

## Writing Tests in Go (Dgraph Conventions)

Dgraph follows standard Go testing patterns with specific conventions.

### Test Naming

**Function names:**

- Start with `Test`: `TestParseSchema`, `TestQueryExecution`
- Use camelCase: `TestBackupAndRestore`, not `Test_Backup_And_Restore`
- Be descriptive: `TestVectorIndexRebuilding` not `TestVector`

**File names:**

- End with `_test.go`: `parser_test.go`, `backup_test.go`
- Match the source file: `schema.go` → `schema_test.go`

### Table-Driven Tests (Preferred)

Used extensively in Dgraph for testing multiple scenarios:

```go
func TestConversion(t *testing.T) {
    tests := []struct {
        name    string
        input   Val
        output  Val
        wantErr bool
    }{
        {name: "string to int", input: Val{...}, output: Val{...}},
        {name: "invalid type", input: Val{...}, wantErr: true},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got, err := Convert(tc.input, tc.output.Tid)
            if tc.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            require.Equal(t, tc.output, got)
        })
    }
}
```

**Benefits:**

- Test multiple cases in one function
- Easy to add new test cases
- Clear failure messages with `t.Run`

### Assertions: require vs assert

Dgraph uses the `testify` library:

**`require.*` (fail immediately):**

```go
require.NoError(t, err) // Stops test if err != nil
require.Equal(t, expected, actual)
require.True(t, condition)
```

When to use: Setup, critical checks, integration tests

**`assert.*` (continue on failure):**

```go
assert.NoError(t, err) // Logs error but continues
assert.Equal(t, expected, actual)
```

When to use: Rarely in Dgraph; prefer `require` for clarity

**Convention:** Use `require` by default.

### Subtests with t.Run

Creates isolated subtests with individual names:

```go
func TestCluster(t *testing.T) {
    t.Run("start nodes", func(t *testing.T) {
        // subtest 1
    })

    t.Run("health check", func(t *testing.T) {
        // subtest 2
    })
}
```

**Benefits:**

- Run specific subtest: `go test --run TestCluster/health`
- Better failure isolation
- Clearer test output

### Cleanup with t.Cleanup

Always defer cleanup operations:

```go
func TestWithCluster(t *testing.T) {
    c, err := dgraphtest.NewLocalCluster(conf)
    require.NoError(t, err)
    defer func() { c.Cleanup(t.Failed()) }() // Cleanup even if test fails

    gc, cleanup, err := c.Client()
    require.NoError(t, err)
    defer cleanup() // Close client connections
}
```

**Why:** Ensures resources are freed even on test failure.

### Helper Functions with t.Helper()

Mark helper functions so failures point to actual test line:

```go
func setupTestData(t *testing.T, gc *GrpcClient) {
    t.Helper() // Failures show caller line, not this line

    err := gc.SetupSchema(`name: string .`)
    require.NoError(t, err)
}

func TestSomething(t *testing.T) {
    setupTestData(t, gc) // If this fails, error points here
    // ...
}
```

### Anti-Patterns to Avoid

#### ❌ Don't use time.Sleep for synchronization

```go
// BAD
time.Sleep(5 * time.Second) // Flaky!

// GOOD
require.NoError(t, c.HealthCheck(false)) // Wait for actual condition
```

#### ❌ Don't share mutable state between tests

```go
// BAD
var sharedClient *Client // Tests interfere with each other

// GOOD
func TestX(t *testing.T) {
    client := newClient() // Each test gets its own
}
```

#### ❌ Don't depend on test execution order

```go
// BAD - Test2 depends on Test1 running first
func TestInsertData(t *testing.T) { /* insert */ }
func TestQueryData(t *testing.T) { /* assumes data exists */ }

// GOOD - Each test is independent
func TestQuery(t *testing.T) {
    setupData(t) // Set up what you need
    // ... test query
}
```

#### ❌ Don't ignore errors in tests

```go
// BAD
client.Mutate(mutation) // Ignoring error

// GOOD
_, err := client.Mutate(mutation)
require.NoError(t, err)
```

### Parallelization

Use with caution:

```go
func TestIndependent(t *testing.T) {
    t.Parallel() // Can run in parallel with other tests
    // Only if test doesn't share resources
}
```

**Don't use for:**

- Integration tests sharing clusters
- Tests modifying global state
- Tests using same ports/resources

### Test Suites with testify/suite

Dgraph uses `testify/suite` for tests needing shared setup/teardown across multiple test methods.

**When to use:**

- Multiple related test methods sharing the same cluster
- Need setup/teardown hooks (`SetupTest`, `TearDownTest`)
- Upgrade tests that run the same tests across version combinations
- Sharing test logic between integration and upgrade tests

**Benefits:**

- Reduces boilerplate for shared setup
- Each test method is independent (new setup/teardown)
- Same test methods run for both integration and upgrade tests
- Excellent for upgrade tests (run same tests across version combos)

**Key pattern: Shared test logic across build tags**

Dgraph uses suites to run identical test methods for both integration and upgrade tests:

**Integration suite (`//go:build integration`):**

- Creates cluster once
- Runs test methods
- Tests current version behaviour

**Upgrade suite (`//go:build upgrade`):**

- Creates cluster with old version
- Runs test methods (validates data works on old version)
- Calls `Upgrade()` method
- Runs same test methods again (validates data still works after upgrade)

**Available hooks:**

- `SetupSuite()` - once before all tests
- `SetupTest()` - before each test method
- `SetupSubTest()` - before each subtest
- `TearDownTest()` - after each test method
- `TearDownSuite()` - once after all tests

**How to run:**

```bash
# Run entire test suite (all test methods)
go test -v --tags=integration ./systest/plugin/

# Run specific test method from suite
go test -v --tags=integration --run 'TestPluginTestSuite/TestPasswordReturn' ./systest/plugin/

# Run specific subtest within a test method
go test -v --tags=integration --run 'TestPluginTestSuite/TestPasswordReturn/subtest' ./systest/plugin/

# Run same tests in upgrade mode
go test -v --tags=upgrade --run 'TestPluginTestSuite/TestPasswordReturn' ./systest/plugin/
```

**When NOT to use:**

- Simple one-off tests → use regular `func TestX(t *testing.T)`
- No shared setup needed → suites add unnecessary complexity
- Unit tests → keep simple

**Examples in Dgraph codebase:**

- `acl/integration_test.go` + `acl/acl_integration_test.go` - ACL suite
- `systest/plugin/` - Integration + Upgrade suites sharing test methods
- `systest/mutations-and-queries/` - Integration + Upgrade suites

---

## Fuzz Tests

Fuzzing tests parser and validation logic with random inputs to find edge cases.

### What is Fuzzing?

Go's native fuzzing generates random inputs to find crashes, panics, or unexpected behaviour.

### Where Fuzz Tests Live

- `dql/parser_fuzz_test.go` - DQL query parser fuzzing

### Running Fuzz Tests

```bash
# Run fuzz test for 5 minutes
go test -v ./dql -fuzz=Fuzz -fuzztime=5m

# Run with custom timeout
go test -v ./dql -fuzz=Fuzz -fuzztime=300s -fuzzminimizetime=120s
```

### CI Workflow

- `ci-dgraph-fuzz.yml` (runs on PRs)
- Runs: `go test -v ./dql -fuzz="Fuzz" -fuzztime="300s"`
- Timeout: 10 minutes
- Catches parser crashes early

### When to Write Fuzz Tests

- Parsers (DQL, GraphQL, RDF)
- Input validation
- Decoders/deserializers
- Any code accepting untrusted input

---

## Future Improvement Ideas

### ✅ Completed Improvements

The following items from the original wishlist have been implemented:

- **✅ OS detection and automatic binary handling:** The Makefile now detects the host OS at runtime
  and automatically builds the correct binaries. On macOS, `make install` builds both native and
  Linux binaries without manual intervention.

- **✅ Automatic binary path management:** The `LINUX_GOBIN` environment variable is automatically
  set based on OS. Docker Compose files use `${LINUX_GOBIN:-$GOPATH/bin}` to mount the correct
  binary.

- **✅ No manual setup scripts required:** The `make test` target now depends on `dgraph-installed`
  which automatically builds binaries if missing. Dependency checking scripts in `t/scripts/` can
  auto-install missing tools with `AUTO_INSTALL=true`.

- **✅ Prerequisites handled automatically:** Running `make test` validates dependencies and builds
  required binaries before running tests.

### Remaining Ideas

The following improvements could still enhance the developer experience:

- **Unified test interface:** A single `make test` entry point that accepts arguments to run any
  test type (unit, integration, integration2, upgrade, fuzz) without needing to navigate to
  subdirectories or know specific incantations.

- **Example commands that "just work":**

  ```bash
  make test SUITE=systest
  make test FUZZ=1 PKG=dql
  make test TAGS=upgrade PKG=acl
  make test TAGS=integration PKG=systest/plugin
  ```

- **Extend t/ runner:** Have the `t/` runner also handle unit and integration2 tests, providing a
  consistent interface for all test types.
