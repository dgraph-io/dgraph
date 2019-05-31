# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project will adhere to [Semantic Versioning](http://semver.org/spec/v2.0.0.html) starting v1.0.0.

## [1.0.15] - 2019-05-30
[1.0.15]: https://github.com/dgraph-io/dgraph/compare/v1.0.14...v1.0.15

### Fixed

- Fix bug that can cause a Dgraph cluster to get stuck in infinite leader election. ([#3391][])
- Fix bug in bulk loader that prevented loading data from JSON files. ([#3464][])
- Fix bug with a potential deadlock by breaking circular lock acquisition. ([#3393][])
- Properly escape strings containing Unicode control characters for data exports. Fixes [#3383]. ([#3429][])
- Initialize tablets map when creating a group. ([#3360][])
- Fix queries with `offset` not working with multiple `orderasc` or `orderdesc` statements. Fixes [#3366][]. ([#3455][])
- Vendor in bug fixes from badger. ([#3348][], [#3371][], [#3460][])

### Changed

- Use Go v1.12.5 to build Dgraph release binaries.
- Truncate Raft logs even when no txn commits are happening. ([3be380b8a][])
- Reduce memory usage by setting a limit on the size of committed entries that can be served per Ready. ([#3308][])
- Reduce memory usage of pending txns by only keeping deltas in memory. ([#3349][])
- Reduce memory usage by limiting the number of pending proposals in apply channel. ([#3340][])
- Reduce memory usage when calculating snapshots by retrieving entries in batches. ([#3409][])
- Allow snapshot calculations during snapshot streaming. ([ecb454754][])
- Allow quick recovery from partitions by shortening the deadline of sending Raft messages to 10s. ([77b52aca1][])
- Take snapshots less frequently so straggling Alpha followers can catch up to the leader. Snapshot frequency is configurable via a flag (see Added section). ([#3367][])
- Allow partial snapshot streams to reduce the amount of data needed to be transferred between Alphas. ([#3454][])
- Use Badger's StreamWriter to improve write speeds during snapshot streaming. ([#3457][]) ([#3442][])
- Call file sync explicitly at the end of TxnWriter to improve performance. ([#3418][])
- Optimize mutation and delta application. ([#2987][])
- Add logs to show Dgraph config options. ([#3337][])
- Add `-v=3` logs for reporting Raft communication for debugging. These logs start with `RaftComm:`. ([9cd628f6f][])

### Added

- Add Alpha flag `--snapshot_after` (default: 10000) to configure the number of Raft entries to keep before taking a snapshot. ([#3367][])
- Add Alpha flag `--abort_older_than` (default: 5m) to configure the amount of time since a pending txn's last mutation until it is aborted. ([#3367][])
- Add Alpha flag `--normalize_node_limit` (default: 10000) to configure the limit for the maximum number of nodes that can be returned in a query that uses the `@normalize` directive. Fixes [#3335][]. ([#3467][])
- Add Prometheus metrics for latest Raft applied index (`dgraph_raft_applied_index`) and the max assigned txn timestamp (`dgraph_max_assigned_ts`). These are useful to track cluster progress. ([#3338][])
- Add Raft checkpoint index to WAL for quicker recovery after restart. ([#3444][])

### Removed

- Remove size calculation in posting list. ([0716dc4e1][])
- Remove a `-v=2` log which can be too noisy during Raft replay. ([2377d9f56][]).
- Remove `dgraph_conf` from /debug/vars. Dgraph config options are available via logs. ([#3337][])

[#3337]: https://github.com/dgraph-io/dgraph/pull/3337
[#3391]: https://github.com/dgraph-io/dgraph/pull/3391
[#3400]: https://github.com/dgraph-io/dgraph/pull/3400
[#3464]: https://github.com/dgraph-io/dgraph/pull/3464
[#2987]: https://github.com/dgraph-io/dgraph/pull/2987
[#3349]: https://github.com/dgraph-io/dgraph/pull/3349
[#3393]: https://github.com/dgraph-io/dgraph/pull/3393
[#3429]: https://github.com/dgraph-io/dgraph/pull/3429
[#3383]: https://github.com/dgraph-io/dgraph/pull/3383
[#3455]: https://github.com/dgraph-io/dgraph/pull/3455
[#3366]: https://github.com/dgraph-io/dgraph/pull/3366
[#3308]: https://github.com/dgraph-io/dgraph/pull/3308
[#3340]: https://github.com/dgraph-io/dgraph/pull/3340
[#3348]: https://github.com/dgraph-io/dgraph/pull/3348
[#3371]: https://github.com/dgraph-io/dgraph/pull/3371
[#3460]: https://github.com/dgraph-io/dgraph/pull/3460
[#3360]: https://github.com/dgraph-io/dgraph/pull/3360
[#3335]: https://github.com/dgraph-io/dgraph/pull/3335
[#3367]: https://github.com/dgraph-io/dgraph/pull/3367
[#3409]: https://github.com/dgraph-io/dgraph/pull/3409
[#3418]: https://github.com/dgraph-io/dgraph/pull/3418
[#3454]: https://github.com/dgraph-io/dgraph/pull/3454
[#3457]: https://github.com/dgraph-io/dgraph/pull/3457
[#3442]: https://github.com/dgraph-io/dgraph/pull/3442
[#3467]: https://github.com/dgraph-io/dgraph/pull/3467
[#3338]: https://github.com/dgraph-io/dgraph/pull/3338
[#3444]: https://github.com/dgraph-io/dgraph/pull/3444
[3be380b8a]: https://github.com/dgraph-io/dgraph/commit/3be380b8a
[ecb454754]: https://github.com/dgraph-io/dgraph/commit/ecb454754
[77b52aca1]: https://github.com/dgraph-io/dgraph/commit/77b52aca1
[9cd628f6f]: https://github.com/dgraph-io/dgraph/commit/9cd628f6f
[0716dc4e1]: https://github.com/dgraph-io/dgraph/commit/0716dc4e1
[2377d9f56]: https://github.com/dgraph-io/dgraph/commit/2377d9f56

## [1.0.14] - 2019-04-12
[1.0.14]: https://github.com/dgraph-io/dgraph/compare/v1.0.13...v1.0.14

### Fixed

- Fix bugs related to best-effort queries. ([#3125][])
- Stream Raft Messages and Fix Check Quorum. ([#3138][])
- Fix lin reads timeouts and AssignUid recursion in Zero. ([#3203][])
- Fix panic when running `@groupby(uid)` which is not allowed and other logic fixes. ([#3232][])
- Fix a StartTs Mismatch bug which happens when running multiple best effort queries using the same txn. Reuse the same timestamp instead of allocating a new one. ([#3187][]) ([#3246][])
- Shutdown extra connections. ([#3280][])
- Fix bug for queries with `@recurse` and `expand(_all_)`. ([#3179][])
- Fix assorted cases of goroutine leaks. ([#3074][])
- Increment tool: Fix best-effort flag name so best-effort queries run as intended from the tool. ([d386fa5][])

[#3125]: https://github.com/dgraph-io/dgraph/pull/3125
[#3138]: https://github.com/dgraph-io/dgraph/pull/3138
[#3203]: https://github.com/dgraph-io/dgraph/pull/3203
[#3232]: https://github.com/dgraph-io/dgraph/pull/3232
[#3187]: https://github.com/dgraph-io/dgraph/pull/3187
[#3246]: https://github.com/dgraph-io/dgraph/pull/3246
[#3280]: https://github.com/dgraph-io/dgraph/pull/3280
[#3179]: https://github.com/dgraph-io/dgraph/pull/3179
[#3074]: https://github.com/dgraph-io/dgraph/pull/3074
[d386fa5]: https://github.com/dgraph-io/dgraph/commit/d386fa5

### Added

- Add timeout option while running queries over HTTP. Setting the `timeout` query parameter `/query?timeout=60s` will timeout queries after 1 minute. ([#3238][])
- Add `badger` tool to release binaries and Docker image.

[#3238]: https://github.com/dgraph-io/dgraph/pull/3238

## [1.0.13] - 2019-03-10
[1.0.13]: https://github.com/dgraph-io/dgraph/compare/v1.0.12...v1.0.13

**Note: This release supersedes v1.0.12 with bug fixes. If you're running v1.0.12, please upgrade to v1.0.13. It is safe to upgrade in-place without a data export and import.**

### Fixed

- Fix Raft panic. ([8cb69ea](https://github.com/dgraph-io/dgraph/commit/8cb69ea))
- Log an error instead of an assertion check for SrcUIDs being nil. ([691b3b3](https://github.com/dgraph-io/dgraph/commit/691b3b3))

## [1.0.12] - 2019-03-05
[1.0.12]: https://github.com/dgraph-io/dgraph/compare/v1.0.11...v1.0.12

**Note: This release requires you to export and re-import data prior to
upgrading or rolling back. The underlying data format has been changed.**

### Added

- Support gzip compression for gRPC and HTTP requests.
  ([#2843](https://github.com/dgraph-io/dgraph/issues/2843))
- Restore is available from a full binary backup. This is an enterprise
  feature licensed under the Dgraph Community License.
- Strict schema mode via `--mutations` flag. By default `--mutations=allow` is
  set to allow all mutations; `--mutations=disallow` disables all mutations;
  `--mutations=strict` allows mutations only for predicates which are defined in
  the schema. Fixes [#2277](https://github.com/dgraph-io/dgraph/issues/2277).
- Add `dgraph increment` tool for debugging and testing. The increment tool
  queries for the specified predicate (default: `counter.val`), increments its
  integer counter value, and mutates the result back to Dgraph. Useful for
  testing end-to-end txns to verify cluster health.
  ([#2955](https://github.com/dgraph-io/dgraph/issues/2955))
- Support best-effort queries. This would relax the requirement of linearizible
  reads. For best-effort queries, Alpha would request timestamps from memory
  instead of making an outbound request to Zero.
  ([#3071](https://github.com/dgraph-io/dgraph/issues/3071))

### Changed

- Use the new Stream API from Badger instead of Dgraph's Stream framework. ([#2852](https://github.com/dgraph-io/dgraph/issues/2852))
- Discard earlier versions of posting lists. ([#2859](https://github.com/dgraph-io/dgraph/issues/2859))
- Make HTTP JSON response encoding more efficient by operating on a bytes buffer
  directly. ([ae1d9f3](https://github.com/dgraph-io/dgraph/commit/ae1d9f3))
- Optimize and refactor facet filtering. ([#2829](https://github.com/dgraph-io/dgraph/issues/2829))
- Show badger.Item meta information in `dgraph debug` output.
- Add new option to `dgraph debug` tool to get a histogram of key and value sizes. ([#2844](https://github.com/dgraph-io/dgraph/issues/2844))
- Add new option to `dgraph debug` tool to get info from a particular read timestamp.
- Refactor rebuild index logic. ([#2851](https://github.com/dgraph-io/dgraph/issues/2851), [#2866](https://github.com/dgraph-io/dgraph/issues/2866))
- For gRPC clients, schema queries are returned in the Json field. The Schema proto field is deprecated.
- Simplify design and make tablet moves robust. ([#2800](https://github.com/dgraph-io/dgraph/issues/2800))
- Switch all node IDs to hex in logs (e.g., ID 0xa instead of ID 10), so they are consistent with Raft logs.
- Refactor reindexing code to only reindex specific tokenizers. ([#2948](https://github.com/dgraph-io/dgraph/issues/2948))
- Introduce group checksums. ([#2964](https://github.com/dgraph-io/dgraph/issues/2964), [#3085](https://github.com/dgraph-io/dgraph/issues/3085))
- Return aborted error if commit ts is 0.
- Reduce number of "ClusterInfoOnly" requests to Zero by making VerifyUid wait for membership information. ([#2974](https://github.com/dgraph-io/dgraph/issues/2974))
- Simplify Raft WAL storage caching. ([#3102](https://github.com/dgraph-io/dgraph/issues/3102))
- Build release binary with Go version 1.11.5.

### Removed

- **Remove LRU cache from Alpha for big wins in query latency reduction (5-10x)
  and mutation throughput (live loading 1.7x faster).** Setting `--lru_mb` is
  still required but will not have any effect since the cache is removed. The
  flag will be used later version when LRU cache is introduced within Badger and
  configurable from Dgraph.
- Remove `--nomutations` flag. Its functionality has moved into strict schema
  mode with the `--mutations` flag (see Added section).

### Fixed

- Use json.Marshal for strings and blobs. Fixes [#2662](https://github.com/dgraph-io/dgraph/issues/2662).
- Let eq use string "uid" as value. Fixes [#2827](https://github.com/dgraph-io/dgraph/issues/2827).
- Skip empty posting lists in `has` function.
- Fix Rollup to pick max update commit ts.
- Fix a race condition when processing concurrent queries. Fixes [#2849](https://github.com/dgraph-io/dgraph/issues/2849).
- Show an error when running multiple mutation blocks. Fixes [#2815](https://github.com/dgraph-io/dgraph/issues/2815).
- Bring in optimizations and bug fixes over from Badger.
- Bulk Loader for multi-group (sharded data) clusters writes out per-group
  schema with only the predicates owned by the group instead of all predicates
  in the cluster. This fixes an issue where queries made to one group may not
  return data served by other groups.
  ([#3065](https://github.com/dgraph-io/dgraph/issues/3065))
- Remove the assert failure in raftwal/storage.go.

## [1.0.11] - 2018-12-17
[1.0.11]: https://github.com/dgraph-io/dgraph/compare/v1.0.10...v1.0.11

### Added

- Integrate OpenCensus in Dgraph. ([#2739](https://github.com/dgraph-io/dgraph/issues/2739))
- Add Dgraph Community License for proprietary features.
- Feature: Full binary backups. This is an enterprise feature licensed under the Dgraph Community License. ([#2710](https://github.com/dgraph-io/dgraph/issues/2710))
- Add `--enterprise_features` flag to enable enterprise features. By enabling enterprise features, you accept the terms of the Dgraph Community License.
- Add minio dep and its deps in govendor. ([94daeaf7](https://github.com/dgraph-io/dgraph/commit/94daeaf7), [35a73e81](https://github.com/dgraph-io/dgraph/commit/35a73e81))
- Add network partitioning tests with blockade tool. ([./contrib/blockade](https://github.com/dgraph-io/dgraph/tree/v1.0.11/contrib/blockade))
- Add Zero endpoints `/assign?what=uids&num=10` and `/assign?what=timestamps&num=10` to assign UIDs or transaction timestamp leases.
- Adding the acl subcommand to support acl features (still work-in-progress). ([#2795](https://github.com/dgraph-io/dgraph/issues/2795))
- Support custom tokenizer in bulk loader ([#2820](https://github.com/dgraph-io/dgraph/issues/2820))
- Support JSON data with Dgraph Bulk Loader. ([#2799](https://github.com/dgraph-io/dgraph/issues/2799))

### Changed

- Make posting list memory rollup happen right after disk. ([#2731](https://github.com/dgraph-io/dgraph/issues/2731))
- Do not retry proposal if already found in CommittedEntries. ([#2740](https://github.com/dgraph-io/dgraph/issues/2740))
- Remove ExportPayload from protos. Export returns Status and ExportRequest. ([#2741](https://github.com/dgraph-io/dgraph/issues/2741))
- Allow more escape runes to be skipped over when parsing string literal. ([#2734](https://github.com/dgraph-io/dgraph/issues/2734))
- Clarify message of overloaded pending proposals for live loader. ([#2732](https://github.com/dgraph-io/dgraph/issues/2732))
- Posting List Evictions. (e2bcfdad)
- Log when removing a tablet. ([#2746](https://github.com/dgraph-io/dgraph/issues/2746))
- Deal better with network partitions in leaders. ([#2749](https://github.com/dgraph-io/dgraph/issues/2749))
- Keep maxDelay during timestamp req to 1s.
- Updates to the version output info.
  - Print the go version used to build Dgraph when running `dgraph version` and in the logs when Dgraph runs. ([#2768](https://github.com/dgraph-io/dgraph/issues/2768))
  - Print the Dgraph version when running live or bulk loader. ([#2736](https://github.com/dgraph-io/dgraph/issues/2736))
- Checking nil values in the equal function ([#2769](https://github.com/dgraph-io/dgraph/issues/2769))
- Optimize query: UID expansion. ([#2772](https://github.com/dgraph-io/dgraph/issues/2772))
- Split membership sync endpoints and remove PurgeTs endpoint. ([#2773](https://github.com/dgraph-io/dgraph/issues/2773))
- Set the Prefix option during iteration. ([#2780](https://github.com/dgraph-io/dgraph/issues/2780))
- Replace Zero's `/assignIds?num=10` endpoint with `/assign?what=uids&num=10` (see Added section).

### Removed

- Remove type hinting for JSON and RDF schema-less types. ([#2742](https://github.com/dgraph-io/dgraph/issues/2742))
- Remove deprecated logic that was found using vet. ([#2758](https://github.com/dgraph-io/dgraph/issues/2758))
- Remove assert for zero-length posting lists. ([#2763](https://github.com/dgraph-io/dgraph/issues/2763))

### Fixed

- Restore schema states on error. ([#2730](https://github.com/dgraph-io/dgraph/issues/2730))
- Refactor bleve tokenizer usage ([#2738](https://github.com/dgraph-io/dgraph/issues/2738)). Fixes [#2622](https://github.com/dgraph-io/dgraph/issues/2622) and [#2601](https://github.com/dgraph-io/dgraph/issues/2601).
- Switch to Badger's Watermark library, which has a memory leak fix. (0cd9d82e)
- Fix tiny typo. ([#2761](https://github.com/dgraph-io/dgraph/issues/2761))
- Fix Test: TestMillion.
- Fix Jepsen bank test. ([#2764](https://github.com/dgraph-io/dgraph/issues/2764))
- Fix link to help_wanted. ([#2774](https://github.com/dgraph-io/dgraph/issues/2774))
- Fix invalid division by zero error. Fixes [#2733](https://github.com/dgraph-io/dgraph/issues/2733).
- Fix missing predicates after export and bulk load. Fixes [#2616](https://github.com/dgraph-io/dgraph/issues/2616).
- Handle various edge cases around cluster memberships. ([#2791](https://github.com/dgraph-io/dgraph/issues/2791))
- Change Encrypt to not re-encrypt password values. Fixes [#2765](https://github.com/dgraph-io/dgraph/issues/2765).
- Correctly parse facet types for both JSON and RDF formats. Previously the
  parsing was handled differently depending on the input format. ([#2797](https://github.com/dgraph-io/dgraph/issues/2797))

## [1.0.10] - 2018-11-05
[1.0.10]: https://github.com/dgraph-io/dgraph/compare/v1.0.9...v1.0.10

**Note: This release requires you to export and re-import data. We have changed the underlying storage format.**

### Added

- The Alter endpoint can be protected by an auth token that is set on the Dgraph Alphas via the `--auth_token` option. This can help prevent accidental schema updates and drop all operations. ([#2692](https://github.com/dgraph-io/dgraph/issues/2692))
- Optimize has function ([#2724](https://github.com/dgraph-io/dgraph/issues/2724))
- Expose the health check API via gRPC. ([#2721](https://github.com/dgraph-io/dgraph/issues/2721))

### Changed

- Dgraph is relicensed to Apache 2.0. ([#2652](https://github.com/dgraph-io/dgraph/issues/2652))
- **Breaking change**. Rename Dgraph Server to Dgraph Alpha to clarify discussions of the Dgraph cluster. The top-level command `dgraph server` is now `dgraph alpha`. ([#2667](https://github.com/dgraph-io/dgraph/issues/2667))
- Prometheus metrics have been renamed for consistency for alpha, memory, and lru cache metrics. ([#2636](https://github.com/dgraph-io/dgraph/issues/2636), [#2670](https://github.com/dgraph-io/dgraph/issues/2670), [#2714](https://github.com/dgraph-io/dgraph/issues/2714))
- The `dgraph-converter` command is available as the subcommand `dgraph conv`. ([#2635](https://github.com/dgraph-io/dgraph/issues/2635))
- Updating protobuf version. ([#2639](https://github.com/dgraph-io/dgraph/issues/2639))
- Allow checkpwd to be aliased ([#2641](https://github.com/dgraph-io/dgraph/issues/2641))
- Better control excessive traffic to Dgraph ([#2678](https://github.com/dgraph-io/dgraph/issues/2678))
- Export format now exports on the Alpha receiving the export request. The naming scheme of the export files has been simplified.
- Improvements to the `dgraph debug` tool that can be used to inspect the contents of the posting lists directory.
- Bring in Badger updates ([#2697](https://github.com/dgraph-io/dgraph/issues/2697))

### Fixed

- Make raft leader resume probing after snapshot crash ([#2707](https://github.com/dgraph-io/dgraph/issues/2707))
- **Breaking change:** Create a lot simpler sorted uint64 codec ([#2716](https://github.com/dgraph-io/dgraph/issues/2716))
- Increase the size of applyCh, to give Raft some breathing space. Otherwise, it fails to maintain quorum health.
- Zero should stream last commit update
- Send commit timestamps in order ([#2687](https://github.com/dgraph-io/dgraph/issues/2687))
- Query blocks with the same name are no longer allowed.
- Fix out-of-range values in query parser. ([#2690](https://github.com/dgraph-io/dgraph/issues/2690))

## [1.0.9] - 2018-10-02
[1.0.9]: https://github.com/dgraph-io/dgraph/compare/v1.0.8...v1.0.9

### Added

- This version switches Badger Options to reasonable settings for p and w directories. This removes the need to expose `--badger.options` option and removes the `none` option from `--badger.vlog`. ([#2605](https://github.com/dgraph-io/dgraph/issues/2605))
- Add support for ignoring parse errors in bulk loader with the option `--ignore_error`. ([#2599](https://github.com/dgraph-io/dgraph/issues/2599))
- Introduction of new command `dgraph cert` to simplify initial TLS setup. See [TLS configuration docs](https://docs.dgraph.io/deploy/#tls-configuration) for more info.
- Add `expand(_forward_)` and `expand(_reverse_)` to GraphQL+- query language. If `_forward_` is passed as an argument to `expand()`, all predicates at that level (minus any reverse predicates) are retrieved.
If `_reverse_` is passed as an argument to `expand()`, only the reverse predicates are retrieved.

### Changed

- Rename intern pkg to pb ([#2608](https://github.com/dgraph-io/dgraph/issues/2608))

### Fixed

- Remove LinRead map logic from Dgraph ([#2570](https://github.com/dgraph-io/dgraph/issues/2570))
- Sanity length check for facets mostly.
- Make has function correct w.r.t. transactions ([#2585](https://github.com/dgraph-io/dgraph/issues/2585))
- Increase the snapshot calculation interval, while decreasing the min number of entries required; so we take snapshots even when there's little activity.
- Convert an assert during DropAll to inf retry. ([#2578](https://github.com/dgraph-io/dgraph/issues/2578))
- Fix a bug which caused all transactions to abort if `--expand_edge` was set to false. Fixes [#2547](https://github.com/dgraph-io/dgraph/issues/2547).
- Set the Applied index in Raft directly, so it does not pick up an index older than the snapshot. Ensure that it is in sync with the Applied watermark. Fixes [#2581](https://github.com/dgraph-io/dgraph/issues/2581).
- Pull in Badger updates. This also fixes the Unable to find log file, retry error.
- Improve efficiency of readonly transactions by reusing the same read ts ([#2604](https://github.com/dgraph-io/dgraph/issues/2604))
- Fix a bug in Raft.Run loop. ([#2606](https://github.com/dgraph-io/dgraph/issues/2606))
- Fix a few issues regarding snapshot.Index for raft.Cfg.Applied. Do not overwrite any existing data when apply txn commits. Do not let CreateSnapshot fail.
- Consider all future versions of the key as well, when deciding whether to write a key or not during txn commits. Otherwise, we'll end up in an endless loop of trying to write a stale key but failing to do so.
- When testing inequality value vars with non-matching values, the response was sent as an error although it should return empty result if the query has correct syntax. ([#2611](https://github.com/dgraph-io/dgraph/issues/2611))
- Switch traces to glogs in worker/export.go ([#2614](https://github.com/dgraph-io/dgraph/issues/2614))
- Improve error handling for `dgraph live` for errors when processing RDF and schema files. ([#2596](https://github.com/dgraph-io/dgraph/issues/2596))
- Fix task conversion from bool to int that used uint32 ([#2621](https://github.com/dgraph-io/dgraph/issues/2621))
- Fix `expand(_all_)` in recurse queries ([#2600](https://github.com/dgraph-io/dgraph/issues/2600)).
- Add language aliases for broader support for full text indices. ([#2602](https://github.com/dgraph-io/dgraph/issues/2602))

## [1.0.8] - 2018-08-29
[1.0.8]: https://github.com/dgraph-io/dgraph/compare/v1.0.7...v1.0.8

### Added

- Introduce a new /assignIds HTTP endpoint in Zero, so users can allocate UIDs to nodes externally.
- Add a new tool which retrieves and increments a counter by 1 transactionally. This can be used to test the sanity of Dgraph cluster.

### Changed

- This version introduces tracking of a few anonymous metrics to measure Dgraph adoption ([#2554](https://github.com/dgraph-io/dgraph/issues/2554)). These metrics do not contain any specifically identifying information about the user, so most users can leave it on. This can be turned off by setting `--telemetry=false` flag if needed in Dgraph Zero.

### Fixed

- Correctly handle a list of type geo in json ([#2482](https://github.com/dgraph-io/dgraph/issues/2482), [#2485](https://github.com/dgraph-io/dgraph/issues/2485)).
- Fix the graceful shutdown of Dgraph server, so a single Ctrl+C would now suffice to stop it.
- Fix various deadlocks in Dgraph and set ConfState in Raft correctly ([#2548](https://github.com/dgraph-io/dgraph/issues/2548)).
- Significantly decrease the number of transaction aborts by using SPO as key for entity to entity connections. ([#2556](https://github.com/dgraph-io/dgraph/issues/2556)).
- Do not print error while sending Raft message by default. No action needs to be taken by the user, so it is set to V(3) level.

## [1.0.7] - 2018-08-10
[1.0.7]: https://github.com/dgraph-io/dgraph/compare/v1.0.6...v1.0.7

### Changed

- Set the `--conc` flag in live loader default to 1, as a temporary fix to avoid tons of aborts.

### Fixed

- All Oracle delta streams are applied via Raft proposals. This deals better with network partition like edge-cases. [#2463](https://github.com/dgraph-io/dgraph/issues/2463)
- Fix deadlock in 10-node cluster convergence. Fixes [#2286](https://github.com/dgraph-io/dgraph/issues/2286).
- Make ReadIndex work safely. [#2469](https://github.com/dgraph-io/dgraph/issues/2469)
- Simplify snapshots, leader now calculates and proposes snapshots to the group. [#2475](https://github.com/dgraph-io/dgraph/issues/2475).
- Make snapshot streaming more robust. [#2487](https://github.com/dgraph-io/dgraph/issues/2487)
- Consolidate all txn tracking logic into Oracle, remove inSnapshot logic. [#2480](https://github.com/dgraph-io/dgraph/issues/2480).
- Bug fix in Badger, to stop panics when exporting.
- Use PreVote to avoid leader change on a node join.
- Fix a long-standing bug where `raft.Step` was being called via goroutines. It is now called serially.
- Fix context deadline issues with proposals. [#2501](https://github.com/dgraph-io/dgraph/issues/2501).

## [1.0.6] - 2018-06-20
[1.0.6]: https://github.com/dgraph-io/dgraph/compare/v1.0.5...v1.0.6

### Added

* Support GraphQL vars as args for Regexp function. [#2353](https://github.com/dgraph-io/dgraph/issues/2353)
* Support GraphQL vars with filters. [#2359](https://github.com/dgraph-io/dgraph/issues/2359)
* Add JSON mutations to raw HTTP. [#2396](https://github.com/dgraph-io/dgraph/issues/2396)

### Fixed

* Fix math >= evaluation. [#2365](https://github.com/dgraph-io/dgraph/issues/2365)
* Avoid race condition between mutation commit and predicate move. [#2392](https://github.com/dgraph-io/dgraph/issues/2392)
* Ability to correctly distinguish float from int in JSON. [#2398](https://github.com/dgraph-io/dgraph/issues/2398)
* Remove _dummy_ data key. [#2401](https://github.com/dgraph-io/dgraph/issues/2401)
* Serialize applying of Raft proposals. Concurrent application was complex and
    cause of multiple bugs. [#2428](https://github.com/dgraph-io/dgraph/issues/2428).
* Improve Zero connections.
* Fix bugs in snapshot move, refactor code and improve performance significantly. [#2440](https://github.com/dgraph-io/dgraph/issues/2440), [#2442](https://github.com/dgraph-io/dgraph/issues/2442)
* Add error handling to GetNoStore. Fixes [#2373](https://github.com/dgraph-io/dgraph/issues/2373).
* Fix bugs in Bulk loader. [#2449](https://github.com/dgraph-io/dgraph/issues/2449)
* Posting List and Raft bug fixes. [#2457](https://github.com/dgraph-io/dgraph/issues/2457)

### Changed

* Pull in Badger v1.5.2.
* Raft storage is now done entirely via Badger. This reduces RAM
    consumption by previously used MemoryStorage. [#2433](https://github.com/dgraph-io/dgraph/issues/2433)
* Trace how node.Run loop performs.
* Allow tweaking Badger options.

**Note:** This change modifies some flag names. In particular, Badger options
are now exposed via flags named with `--badger.` prefix.

## [1.0.5] - 2018-04-20
[1.0.5]: https://github.com/dgraph-io/dgraph/compare/v1.0.4...v1.0.5

### Added

* Option to have server side sequencing.
* Ability to specify whitelisted IP addresses for admin actions.


### Fixed

* Fix bug where predicate with string type sometimes appeared as `_:uidffffffffffffffff` in exports.
* Validate facet value should be according to the facet type supplied when mutating using N-Quads ([#2074](https://github.com/dgraph-io/dgraph/issues/2074)).
* Use `time.Equal` function for comparing predicates with `datetime`([#2219](https://github.com/dgraph-io/dgraph/issues/2219)).
* Skip `BitEmptyPosting` for `has` queries.
* Return error from query if we don't serve the group for the attribute instead of crashing ([#2227](https://github.com/dgraph-io/dgraph/issues/2227)).
* Send `maxpending` in connection state to server ([#2236](https://github.com/dgraph-io/dgraph/issues/2236)).
* Fix bug in SP* transactions ([#2148](https://github.com/dgraph-io/dgraph/issues/2148)).
* Batch and send during snapshot to make snapshots faster.
* Don't skip schema keys while calculating tablets served.
* Fix the issue which could lead to snapshot getting blocked for a cluster with replicas ([#2266](https://github.com/dgraph-io/dgraph/issues/2266)).
* Dgraph server retries indefinitely to connect to Zero.
* Allow filtering and regex queries for list types with lossy tokenizers.
* Dgraph server segfault in worker package ([#2322](https://github.com/dgraph-io/dgraph/issues/2322)).
* Node crashes can lead to the loss of inserted triples ([#2290](https://github.com/dgraph-io/dgraph/issues/2290)).


### Changed

* Cancel pending transactions for a predicate when predicate move is initiated.
* Move Go client to its own repo at `dgraph-io/dgo`.
* Make `expand(_all_)` return value and uid facets.
* Add an option to specify a `@lang` directive in schema for predicates with lang tags.
* Flag `memory_mb` has been changed to `lru_mb`. The default recommended value for `lru_mb` is
  one-third of the total RAM available on the server.

## [1.0.4] - 2018-03-09
[1.0.4]: https://github.com/dgraph-io/dgraph/compare/v1.0.3...v1.0.4

### Added

* Support for empty strings in query attributes.
* Support GraphQL vars in first, offset and after at root.
* Add support for query_edge_limit flag which can be used to limit number of results for shortest
  path, recurse queries.
* Make rebalance interval a flag in Zero.
* Return latency information for mutation operations.
* Support @upsert directive in schema.

### Fixed

* Issues with predicate deletion in a cluster.
* Handle errors from posting.Get.
* Correctly update commitTs while committing and startTs == deleteTs.
* Error handling in abort http handler.
* Get latest membership state from Zero if uid in mutation > maxLeaseId.
* Fix bug in Mutate where mutated keys were not filled.
* Update membership state if we can't find a leader while doing snapshot retrieval.
* Make snapshotting more frequent, also try aborting long pending transactions.
* Trim null character from end of strings before exporting.
* Sort facets after parsing RDF's using bulk loader.
* Fig bug in SyncIfDirty.
* Fix fatal error due to TxnTooBig error.
* Fix bug in dgraph live where some batches could be skipped on conflict error.
* Fix a bug related to expand(_all_) queries.
* Run cleanPredicate and proposeKeyValues sequentially.
* Serialize connect requests in Zero.

### Changed

* Retry snapshot retrieval and join cluster indefinitely.
* Make client directory optional in dgraph live.
* Do snapshot in Zero in a goroutine so that Run loop isn't blocked.


## [1.0.3] - 2018-02-08
[1.0.3]: https://github.com/dgraph-io/dgraph/compare/v1.0.2...v1.0.3

### Added

* Support for specifying blank nodes as part of JSON mutation.
* `dgraph version` command to check current version.
* `curl` to Docker image.
* `moveTablet` endpoint to Zero to allow initiating a predicate move.

### Fixed

* Out of range error while doing `eq` query.
* Reduce `maxBackOffDelay` to 10 sec so that leader election is faster after restart.
* Fix bugs with predicate move where some data was not sent and schema not loaded properly on
  replicas.
* Fix the total number of RDF's processed when live loader ends.
* Reindex data when schema is changed to list type to fix adding and deleting new data.
* Correctly upate uidMatrix when facetOrder is supplied.
* Inequality operator(`gt` and `lt`) result for non lossy tokenizers.

### Changed

* `--zero_addr` flag changed to `--zero` for `dgraph bulk` command.
* Default ports for Zero have been changed `7080` => `5080`(grpc) and `8080` => `6080`(http).
* Update badger version and how purging is done to fix CPU spiking when Dgraph is idle.
* Print predicate name as part of the warning about long term for exact index.

## [1.0.2] - 2018-01-17
[1.0.2]: https://github.com/dgraph-io/dgraph/compare/v1.0.1...v1.0.2

### Fixed

* Always return predicates of list type in an array.
* Edges without facet values are also returned when performing sort on facet.
* Don't derive schema while deleting edges.
* Better error checking when accessing posting lists. Fixes bug where parts of
  queries are sometimes omitted when system is under heavy load.
* Fix missing error check in mutation handling when using CommitNow (gave incorrect error).
* Fix bug where eq didn't work correctly for the fulltext index.
* Fix race because of which `replicas` flag was not respected.
* Fix bug with key copy during predicate move.
* Fix race in merging keys keys from btree and badger iterator.
* Fix snapshot retrieval for new nodes by retrieving it before joining the cluster.
* Write schema at timestamp 1 in bulk loader.
* Fix unexpected meta fatal error.
* Fix groupby result incase the child being grouped open has multiple parents.

### Changed

* Remove StartTs field from `api.Operation`.
* Print error message in live loader if its not ErrAborted. Also, stop using membership state and
instead use the address given by user.
* Only send keys corresponding to data that was mutated.

## [1.0.1] - 2017-12-20
[1.0.1]: https://github.com/dgraph-io/dgraph/compare/v1.0.0...v1.0.1

### Fixed

* Wait for background goroutines to finish in posting package on shutdown.
* Return error if we cant parse the uid given in json input for mutations.
* Don't remove `_predicate_` schema from disk during drop all.
* Fix panic in expand(_all_)

### Changed

* Make sure at least one field is set while doing Alter.

## [1.0.0] - 2017-12-18
[1.0.0]: https://github.com/dgraph-io/dgraph/compare/v0.9.3...v1.0.0

### Added

* Allow doing Mutate and Alter Operations using dgraph UI.
* Provide option to user to ignore conflicts on index keys.

### Fixed

* Language tag parsing in queries now accepts digits (in line with RDF parsing).
* Ensure that GraphQL variables are declared before use.
* Export now uses correct blank node syntax.
* Membership stream doesn't get stuck if node steps down as leader.
* Fix issue where sets were not being returned after doing a S P * deletion when part of same
  transaction.
* Empty string values are stored as it is and no strings have special meaning now.
* Correctly update order of facetMatrix when orderdesc/orderasc is applied.
* Allow live and bulk loaders to work with multiple zeros.
* Fix sorting with for predicates with multiple language tags.
* Fix alias edge cases in normalize directive.
* Allow reading new index key mutated as part of same transaction.
* Fix bug in value log GC in badger.
* SIGINT now forces a shutdown after 5 seconds when there are pending RPCs.

### Changed

* `DropAttr` now also removes the schema for the attribute (previously it just removed the edges).
*  Tablet metadata is removed from zero after deletion of predicate.
*  LRU size is changed dynamically now based on `max_memory_mb`
*  Call RunValueLogGC for every GB increase in size of value logs. Upgrade vendored version of
   Badger.
*  Prohibit string to password schema change.
*  Make purging less aggressive.
*  Check if GraphQL Variable is defined before using.

## [0.9.3] - 2017-12-01
[0.9.3]: https://github.com/dgraph-io/dgraph/compare/v0.9.2...v0.9.3

### Added

* Support for alias while asking for facets.
* Support for general configuration via environment variables and configuration files.
* `IgnoreIndexConflict` field in Txn which allows ignoring conflicts on index keys.

### Fixed

* `expand(_all_)` now correctly gives all language variants of a string.
* Indexes now correctly maintained when deleting via `S * *` and `S P *`.
* `expand(_all_)` now follows reverse edges.
* Don't return uid for nodes without any children when requested through debug flag.
* GraphQL variables for HTTP endpoints. Variable map can be set as a JSON
  object using the `X-Dgraph-Vars` header.
* Abort if CommitNow flag is set and the mutation fails.
* Live loader treats subjects/predicates that look like UIDs as existing nodes
  rather than new nodes.
* Fix bug in `@groupby` queries where predicate was converted to lower case in queries.

### Changed

* When showing a predicate with list type, only values without a language tag are shown. To get the values of the predicate that are tagged with a language, query the predicate with that language explicitly.
* Validate the address advertised by dgraph nodes.
* Store/Restore peer map on snapshot.
* Fix rdfs per second reporting in live loader.
* Fix bug in lru eviction.
* Proto definitions are split into intern and api.

## [0.9.2] - 2017-11-20
[0.9.2]: https://github.com/dgraph-io/dgraph/compare/v0.9.1...v0.9.2

### Added

* Support for removing dead node from quorum.
* Support for alias in groupby queries.
* Add DeleteEdges helper function for Go client.

### Changed

* Dgraph tries to abort long running/abandoned transactions.
* Fix TLS flag parsing for Dgraph server and live loader.
* Reduce dependencies for Go client.
* `depth` and `loop` arguments should be inside @recurse().
* Base36 encode keys that are part of TxnContext and are sent to the client.
* Fix race condition in expand(_all_) queries.
* Fix (--ui) flag not being parsed properly.

## [0.9.1] - 2017-11-15
[0.9.1]: https://github.com/dgraph-io/dgraph/compare/v0.9.0...v0.9.1

### Changed

* Transaction HTTP API has been modified slightly. `start_ts` is now a path parameter instead of a header.
For `/commit` API, keys are passed in the body.

## [0.9.0] - 2017-11-14
[0.9.0]: https://github.com/dgraph-io/dgraph/compare/v0.8.3...v0.9.0

**The latest release has a lot of breaking changes but also brings powerful features like Transactions, support for CJK and custom tokenization.**

### Added

* Dgraph adds support for distributed ACID transactions (a blog post is in works). Transactions can be done via the Go, Java or HTTP clients (JS client coming). See [docs here](https://docs.dgraph.io/clients/).
* Support for Indexing via [Custom tokenizers](https://docs.dgraph.io/query-language/#indexing-with-custom-tokenizers).
* Support for CJK languages in the full-text index.

### Changed

#### Running Dgraph

* We have consolidated all the `server`, `zero`, `live/bulk-loader` binaries into a single `dgraph` binary for convenience. Instructions for running Dgraph can be found in the [docs](https://docs.dgraph.io/get-started/).
* For Dgraph server, Raft ids can be assigned automatically. A user can optionally still specify an ID, via `--idx` flag.
* `--peer` flag which was used to specify another Zero instance’s IP address is being replaced by `--zero` flag to indicate the address corresponds to Dgraph zero.
* `port`, `grpc_port` and `worker_port` flags have been removed from Dgraph server and Zero. The ports are:

- Internal Grpc: 7080
- HTTP: 8080
- External Grpc: 9080 (Dgraph server only)

Users can set `port_offset` flag, to modify these fixed ports.

#### Queries

* Queries, mutations and schema updates are done through separate endpoints. **Queries can no longer have a mutation block.**
* Queries can be done via `Query` Grpc endpoint (it was called `Run` before) or the `/query` HTTP handler.
* `_uid_` is renamed to `uid`. So queries now need to request for `uid`. Example
```
{
  bladerunner(func: eq(name@en, "Blade Runner")) {
    uid
    name@en
  }
}
```
* Facets response structure has been modified and is a lot flatter. Facet key is now `predicate|facet_name`.
Examples for [Go client](https://godoc.org/github.com/dgraph-io/dgraph/client#example-Txn-Mutate-Facets) and [HTTP](https://docs.dgraph.io/query-language/#facets-edge-attributes).
* Query latency is now returned as numeric (ns) instead of string.
* [`Recurse`](https://docs.dgraph.io/query-language/#recurse-query) is now a directive. So queries with `recurse` keyword at root won't work anymore.
* Syntax for [`count` at root](https://docs.dgraph.io/query-language/#count) has changed. You need to ask for `count(uid)`, instead of `count()`.

#### Mutations

* Mutations can only be done via `Mutate` Grpc endpoint or via [`/mutate` HTTP handler](https://docs.dgraph.io/clients/#transactions).
* `Mutate` Grpc endpoint can be used to set/ delete JSON, or set/ delete a list of N-Quads and set/ delete raw RDF strings.
* Mutation blocks don't require the mutation keyword anymore. Here is an example of the new syntax.
```
{
  set {
    <name> <is> <something> .
    <hometown> <is> "San Francisco" .
  }
}
```
* [`Upsert`](https://docs.dgraph.io/v0.8.3/query-language/#upsert) directive and [mutation variables](https://docs.dgraph.io/v0.8.3/query-language/#variables-in-mutations) go away. Both these functionalities can now easily be achieved via transactions.

#### Schema

* `<*> <pred> <*>` operations, that is deleting a predicate can't be done via mutations anymore. They need to be done via `Alter` Grpc endpoint or via the `/alter` HTTP handler.
* Drop all is now done via `Alter`.
* Schema updates are now done via `Alter` Grpc endpoint or via `/alter` HTTP handler.

#### Go client

* `Query` Grpc endpoint returns response in JSON under `Json` field instead of protocol buffer. `client.Unmarshal` method also goes away from the Go client. Users can use `json.Unmarshal` for unmarshalling the response.
* Response for predicate of type `geo` can be unmarshalled into a struct. Example [here](https://godoc.org/github.com/dgraph-io/dgraph/client#example-package--SetObject).
* `Node` and `Edge` structs go away along with the `SetValue...` methods. We recommend using [`SetJson`](https://godoc.org/github.com/dgraph-io/dgraph/client#example-package--SetObject) and `DeleteJson` fields to do mutations.
* Examples of how to use transactions using the client can be found at https://docs.dgraph.io/clients/#go.

### Removed
- Embedded dgraph goes away. We haven’t seen much usage of this feature. And it adds unnecessary maintenance overhead to the code.
- Dgraph live no longer stores external ids. And hence the `xid` flag is gone.
