# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project will adhere to [Semantic Versioning](http://semver.org/spec/v2.0.0.html) starting v1.0.0.

## [Unreleased]

## [1.0.6] - 2018-06-20

### Added

* Support GraphQL vars as args for Regexp function. #2353
* Support GraphQL vars with filters. #2359
* Add JSON mutations to raw HTTP. #2396

### Fixed

* Fix math >= evaluation. #2365
* Avoid race condition between mutation commit and predicate move. #2392
* Ability to correctly distinguish float from int in JSON. #2398
* Remove _dummy_ data key. #2401
* Serialize applying of Raft proposals. Concurrent application was complex and
    cause of multiple bugs. #2428.
* Improve Zero connections.
* Fix bugs in snapshot move, refactor code and improve performance significantly. #2440, #2442
* Add error handling to GetNoStore. Fixes #2373.
* Fix bugs in Bulk loader. #2449

### Changed

* Pull in Badger v1.5.2.
* Raft storage is now done entirely via Badger. This reduces RAM
    consumption by previously used MemoryStorage. #2433
* Trace how node.Run loop performs.
* Allow tweaking Badger options.

*Warning: This change modifies some flag names. In particular, Badger options
are now exposed via flags named with `--badger.` prefix.

## [1.0.5] - 2018-04-20

### Added

* Option to have server side sequencing.
* Ability to specify whitelisted IP addresses for admin actions.


### Fixed

* Fix bug where predicate with string type sometimes appeared as `_:uidffffffffffffffff` in exports.
* Validate facet value should be according to the facet type supplied when mutating using NQuads (#2074).
* Use `time.Equal` function for comparing predicates with `datetime`(#2219).
* Skip `BitEmptyPosting` for `has` queries.
* Return error from query if we don't serve the group for the attribute instead of crashing (#2227).
* Send `maxpending` in connection state to server (#2236).
* Fix bug in SP* transactions (#2148).
* Batch and send during snapshot to make snapshots faster.
* Don't skip schema keys while calculating tablets served.
* Fix the issue which could lead to snapshot getting blocked for a cluster with replicas (#2266).
* Dgraph server retries indefinitely to connect to Zero.
* Allow filtering and regex queries for list types with lossy tokenizers.
* Dgraph server segfault in worker package (#2322).
* Node crashes can lead to the loss of inserted triples (#2290).


### Changed

* Cancel pending transactions for a predicate when predicate move is initiated.
* Move Go client to its own repo at `dgraph-io/dgo`.
* Make `expand(_all_)` return value and uid facets.
* Add an option to specify a `@lang` directive in schema for predicates with lang tags.
* Flag `memory_mb` has been changed to `lru_mb`. The default recommended value for `lru_mb` is
  one-third of the total RAM available on the server.

## [1.0.4] - 2018-03-09

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

### Fixed

* Wait for background goroutines to finish in posting package on shutdown.
* Return error if we cant parse the uid given in json input for mutations.
* Don't remove `_predicate_` schema from disk during drop all.
* Fix panic in expand(_all_)

### Changed

* Make sure at least one field is set while doing Alter.

## [1.0.0] - 2017-12-18

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

### Changed

* Transaction HTTP API has been modified slightly. `start_ts` is now a path parameter instead of a header.
For `/commit` API, keys are passed in the body.

## [0.9.0] - 2017-11-14

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
* `Mutate` Grpc endpoint can be used to set/ delete JSON, or set/ delete a list of NQuads and set/ delete raw RDF strings.
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
