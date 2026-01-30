# Test Results Summary

**Date:** 2026-01-30 **Branch:** test-env-setup-tweaks

## Results Table

| #   | Command                  | Status    | Failure Summary                                                                                                  |
| --- | ------------------------ | --------- | ---------------------------------------------------------------------------------------------------------------- |
| 1   | `make test-all`          | ❌ FAILED | systest/1million: missing schema file "1million-noindex.schema"                                                  |
| 2   | `make test-benchmark`    | ❌ FAILED | Hung on BenchmarkSortQuickSort (1M elements); also: dql parser errors, graphql connection refused, posting panic |
| 3   | `make test-core`         | ❌ FAILED | worker/TestExportFormat: connection refused to localhost:8080                                                    |
| 4   | `make test-fuzz`         | ✅ PASSED | FuzzTestParser ran 5 minutes, 24.9M executions                                                                   |
| 5   | `make test-integration`  | ❌ FAILED | Multiple connection refused errors to Docker services (9080, 5080, 9001)                                         |
| 6   | `make test-integration2` | ❌ FAILED | Exit code 2 - make target failed despite most individual tests passing                                           |
| 7   | `make test-ldbc`         | ❌ FAILED | TestQueries/IC07: connection refused to 0.0.0.0:9080 (alpha not ready after bulk upload)                         |
| 8   | `make test-load`         | ❌ FAILED | systest/1million: alpha health check failed after 60 attempts                                                    |
| 9   | `make test-systest`      | ❌ FAILED | TestBackupOfOldRestore in systest/backup/filesystem                                                              |
| 10  | `make test-unit`         | ❌ FAILED | TestRestoreOfOldBackup/backup_of_20.11 in systest/backup/filesystem                                              |
| 11  | `make test-upgrade`      | ❌ FAILED | TestMultitenancySuite, TestPluginTestSuite, TestCountReverseIndex; container exits with code 255                 |
| 12  | `make test-vector`       | ❌ FAILED | TestVectorIncrBackupRestore in systest/vector                                                                    |
| 13  | `make test`              | ❌ FAILED | systest/1million: missing schema file "1million-noindex.schema"                                                  |

## Summary

**Total:** 1 passed, 12 failed

### Common Failure Patterns

1. **Connection Refused Errors** - Many tests fail with "connection refused" to Docker-hosted
   services (dgraph alpha on 9080, dgraph zero on 5080, minio on 9001). This suggests Docker
   infrastructure isn't reliably starting or exposing ports.

2. **Missing Schema Files** - The systest/1million package consistently fails because the
   1million-noindex.schema file doesn't exist at the expected temp path.

3. **Health Check Timeouts** - Alpha containers fail health checks repeatedly (60+ attempts) before
   tests give up.

4. **Backup/Restore Tests** - Multiple backup-related tests fail (TestBackupOfOldRestore,
   TestRestoreOfOldBackup) in systest/backup/filesystem.

### Environment Notes

- Platform: macOS (Darwin 25.2.0)
- Architecture: arm64
- Docker Desktop with Linux containers (rosetta emulation)
- Tests use Docker Compose for multi-container test clusters
