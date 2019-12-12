# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project will adhere to [Semantic Versioning](http://semver.org/spec/v2.0.0.html) starting v1.0.0.

## 1.1.1 - [Unreleased][Unreleased-v1.1.1]
[Unreleased-v1.1.1]: https://github.com/dgraph-io/dgraph/compare/v1.1.0...master

### Changed

- **Breaking changes for expand() queries**
  - Remove `expand(_forward_)` and `expand(_reverse_)`. ([#4119][])
  - Change `expand(_all_)` functionality to only include the predicates in the type. ([#4171][])
- Add support for Go Modules. ([#4146][])
- Simplify type definitions: type definitions no longer require the type (string, int, etc.) per field name. ([#4017][])
- Adding log lines to help troubleshoot snapshot and rollup. ([#3889][])
- Add `--http` flag to configure pprof endpoint for live loader. ([#3846][])
- Use snappy compression for internal gRPC communication. ([#3368][])
- Periodically run GC in all dgraph commands. ([#4032][], [#4075][])
- Exit early if data files given to bulk loader are empty. ([#4253][])
- Add support for first and offset directive in has function. ([#3970][])
- Pad encData to 17 bytes before decoding. ([#4066][])
- Remove usage of deprecated methods. ([#4076][])
- Show line and column numbers for errors in HTTP API responses. ([#4012][])
- Do not store non-pointer values in sync.Pool. ([#4089][])
- Verify that all the fields in a type exist in the schema. ([#4114][])
- Update badger to version v2.0.0. ([#4200][])
- Introduce StreamDone in bulk loader. ([#4297][])

Enterprise features:

- ACL: Disallow schema queries when an user has not logged in. ([#4107][])
- Block delete if predicate permission is zero. Fixes [#4265][]. ([#4349][])

### Added

- Support `@cascade` directive at subqueries. ([#4006][])
- Support `@normalize` directive for subqueries. ([#4042][])
- Support `val()` function inside upsert mutations (both RDF and JSON). ([#3877][], [#3947][])
- Support GraphQL Variables for facet values in `@facets` filters. ([#4061][])
- Support filtering by facets on values. ([#4217][])
- Add ability to query `expand(TypeName)` only on certain types. ([#3920][])
- Expose numUids metrics per query to estimate query cost. ([#4033][])
- Upsert queries now return query results in the upsert response. ([#4269][], [#4375][])
- Add support for multiple mutation blocks. ([#4210][])
- Add total time taken to process a query in result under `"total_ns"` field. ([#4312][])

Enterprise features:

- Add encryption-at-rest. ([#4351][])

### Removed

- **Breaking change**: Remove `@type` directive from query language. To filter
  an edge by a type, use `@filter(type(TypeName))` instead of `@type(TypeName)`.
  ([#4016][])
  
Enterprise features:
  
- Remove regexp ACL rules. ([#4360][])

### Fixed

- Avoid changing order if multiple versions of the same edge is found.
- Consider reverse count index keys for conflict detection in transactions. Fixes [#3893][]. ([#3932][])
- Clear the unused variable tlsCfg. ([#3937][])
- Do not require the last type declaration to have a new line. ([#3926][])
- Verify type definitions do not have duplicate fields. Fixes [#3924][]. ([#3925][])
- Fix bug in bulk loader when store_xids is true. Fixes [#3922][]. ([#3950][])
- Call cancel function only if err is not nil. Fixes [#3966][]. ([#3990][])
- Change the mapper output directory from $TMP/shards to $TMP/map_output. Fixes [#3959][]. ([#3960][])
- Return error if keywords used as alias in groupby. ([#3725][])
- Fix bug where language strings are not filtered when using custom tokenizer. Fixes [#3991][]. ([#3992][])
- Support named queries without query variables. Fixes [#3994][]. ([#4028][])
- Correctly set up client connection in x package. ([#4036][])
- Fix data race in regular expression processing. Fixes [#4030][]. ([#4065][])
- Check for n.Raft() to be nil, Fixes [#4053][]. ([#4084][])
- Fix file and directory permissions for bulk loader. ([#4088][])
- Ensure that clients can send OpenCensus spans over to the server. ([#4144][])
- Change lexer to allow unicode escape sequences. Fixes [#4157][].([#4175][])
- Handle the count(uid) subgraph correctly. Fixes [#4038][]. ([#4122][])
- Don't traverse immutable layer while calling iterate if deleteBelowTs > 0. Fixes [#4182][]. ([#4204][])
- Bulk loader allocates reserved predicates in first reduce shard. Fixes [#3968][]. ([#4202][])
- Only allow one alias per predicate. ([#4236][])
- Change member removal logic to remove members only once. ([#4254][])
- Disallow uid as a predicate name. ([#4219][])
- Drain apply channel when a snapshot is received. ([#4273][])
- Added RegExp filter to func name. Fixes [#3268][]. ([#4230][])
- Acquire read lock instead of exclusive lock for langBaseCache. ([#4279][])
- Added proper handling of int and float for math op. [#4132][]. ([#4257][])
- Don't delete group if there is no member in the group. ([#4274][])
- Sort alphabets of languages for non indexed fields. Fixes [#4005][]. ([#4260][])
- Copy xid string to reduce memory usage in bulk loader. ([#4287][])
- Adding more details for mutation error messages with scalar/uid type mismatch. ([#4317][])
- Limit UIDs per variable in upsert. Fixes [#4021][]. ([#4268][])
- Return error instead of panic when geo data is corrupted. Fixes [#3740][]. ([#4318][])
- Use txn writer to write schema postings. ([#4296][])
- Fix connection log message in dgraph alpha from "CONNECTED" to "CONNECTING" when establishing a connection to a peer. Fixes [#4298][]. ([#4303][])
- Fix segmentation fault in backup. ([#4314][])
- Close store after stoping worker. ([#4356][])
- Don't pre allocate mutation map. ([#4343][])
- Cmd: fix config file from env variable issue in subcommands. Fixes [#4311][]. ([#4344][])
- Fix segmentation fault in Alpha. Fixes [#4288][]. ([#4394][]) 
- Fix handling of depth parameter for shortest path query for numpaths=1 case. Fixes [#4169][]. ([#4347][])
- Do not return dgo.ErrAborted when client calls txn.Discard(). ([#4389][])
- Fix `has` pagination when predicate is queried with @lang. Fixes [#4282][]. ([#4331][])

Enterprise features:

- Fix bug when overriding credentials in backup request. Fixes [#4044][]. ([#4047][])
- Create restore directory when running "dgraph restore". Fixes [#4315][]. ([#4352][]) 
- Write group_id files to postings directories during restore. ([#4365][]) 

[#4119]: https://github.com/dgraph-io/dgraph/issues/4119
[#4171]: https://github.com/dgraph-io/dgraph/issues/4171
[#4146]: https://github.com/dgraph-io/dgraph/issues/4146
[#4017]: https://github.com/dgraph-io/dgraph/issues/4017
[#3889]: https://github.com/dgraph-io/dgraph/issues/3889
[#3846]: https://github.com/dgraph-io/dgraph/issues/3846
[#3368]: https://github.com/dgraph-io/dgraph/issues/3368
[#4032]: https://github.com/dgraph-io/dgraph/issues/4032
[#4075]: https://github.com/dgraph-io/dgraph/issues/4075
[#4253]: https://github.com/dgraph-io/dgraph/issues/4253
[#3970]: https://github.com/dgraph-io/dgraph/issues/3970
[#4066]: https://github.com/dgraph-io/dgraph/issues/4066
[#4076]: https://github.com/dgraph-io/dgraph/issues/4076
[#4012]: https://github.com/dgraph-io/dgraph/issues/4012
[#4030]: https://github.com/dgraph-io/dgraph/issues/4030
[#4065]: https://github.com/dgraph-io/dgraph/issues/4065
[#4089]: https://github.com/dgraph-io/dgraph/issues/4089
[#4114]: https://github.com/dgraph-io/dgraph/issues/4114
[#4107]: https://github.com/dgraph-io/dgraph/issues/4107
[#4006]: https://github.com/dgraph-io/dgraph/issues/4006
[#4042]: https://github.com/dgraph-io/dgraph/issues/4042
[#3877]: https://github.com/dgraph-io/dgraph/issues/3877
[#3947]: https://github.com/dgraph-io/dgraph/issues/3947
[#4061]: https://github.com/dgraph-io/dgraph/issues/4061
[#4217]: https://github.com/dgraph-io/dgraph/issues/4217
[#3920]: https://github.com/dgraph-io/dgraph/issues/3920
[#4033]: https://github.com/dgraph-io/dgraph/issues/4033
[#4016]: https://github.com/dgraph-io/dgraph/issues/4016
[#3893]: https://github.com/dgraph-io/dgraph/issues/3893
[#3932]: https://github.com/dgraph-io/dgraph/issues/3932
[#3937]: https://github.com/dgraph-io/dgraph/issues/3937
[#3926]: https://github.com/dgraph-io/dgraph/issues/3926
[#3924]: https://github.com/dgraph-io/dgraph/issues/3924
[#3925]: https://github.com/dgraph-io/dgraph/issues/3925
[#3922]: https://github.com/dgraph-io/dgraph/issues/3922
[#3950]: https://github.com/dgraph-io/dgraph/issues/3950
[#3966]: https://github.com/dgraph-io/dgraph/issues/3966
[#3990]: https://github.com/dgraph-io/dgraph/issues/3990
[#3959]: https://github.com/dgraph-io/dgraph/issues/3959
[#3960]: https://github.com/dgraph-io/dgraph/issues/3960
[#3725]: https://github.com/dgraph-io/dgraph/issues/3725
[#3991]: https://github.com/dgraph-io/dgraph/issues/3991
[#3992]: https://github.com/dgraph-io/dgraph/issues/3992
[#3994]: https://github.com/dgraph-io/dgraph/issues/3994
[#4028]: https://github.com/dgraph-io/dgraph/issues/4028
[#4036]: https://github.com/dgraph-io/dgraph/issues/4036
[#4053]: https://github.com/dgraph-io/dgraph/issues/4053
[#4084]: https://github.com/dgraph-io/dgraph/issues/4084
[#4088]: https://github.com/dgraph-io/dgraph/issues/4088
[#4144]: https://github.com/dgraph-io/dgraph/issues/4144
[#4157]: https://github.com/dgraph-io/dgraph/issues/4157
[#4175]: https://github.com/dgraph-io/dgraph/issues/4175
[#4038]: https://github.com/dgraph-io/dgraph/issues/4038
[#4122]: https://github.com/dgraph-io/dgraph/issues/4122
[#4182]: https://github.com/dgraph-io/dgraph/issues/4182
[#4204]: https://github.com/dgraph-io/dgraph/issues/4204
[#3968]: https://github.com/dgraph-io/dgraph/issues/3968
[#4202]: https://github.com/dgraph-io/dgraph/issues/4202
[#4236]: https://github.com/dgraph-io/dgraph/issues/4236
[#4254]: https://github.com/dgraph-io/dgraph/issues/4254
[#4219]: https://github.com/dgraph-io/dgraph/issues/4219
[#4044]: https://github.com/dgraph-io/dgraph/issues/4044
[#4047]: https://github.com/dgraph-io/dgraph/issues/4047
[#4273]: https://github.com/dgraph-io/dgraph/issues/4273
[#4230]: https://github.com/dgraph-io/dgraph/issues/4230
[#4279]: https://github.com/dgraph-io/dgraph/issues/4279
[#4257]: https://github.com/dgraph-io/dgraph/issues/4257
[#4274]: https://github.com/dgraph-io/dgraph/issues/4274
[#4200]: https://github.com/dgraph-io/dgraph/issues/4200
[#4260]: https://github.com/dgraph-io/dgraph/issues/4260
[#4269]: https://github.com/dgraph-io/dgraph/issues/4269
[#4287]: https://github.com/dgraph-io/dgraph/issues/4287
[#4303]: https://github.com/dgraph-io/dgraph/issues/4303
[#4317]: https://github.com/dgraph-io/dgraph/issues/4317
[#4210]: https://github.com/dgraph-io/dgraph/issues/4210
[#4312]: https://github.com/dgraph-io/dgraph/issues/4312
[#4268]: https://github.com/dgraph-io/dgraph/issues/4268
[#4318]: https://github.com/dgraph-io/dgraph/issues/4318
[#4297]: https://github.com/dgraph-io/dgraph/issues/4297
[#4296]: https://github.com/dgraph-io/dgraph/issues/4296
[#4314]: https://github.com/dgraph-io/dgraph/issues/4314
[#4356]: https://github.com/dgraph-io/dgraph/issues/4356
[#4343]: https://github.com/dgraph-io/dgraph/issues/4343
[#4344]: https://github.com/dgraph-io/dgraph/issues/4344
[#4351]: https://github.com/dgraph-io/dgraph/issues/4351
[#3268]: https://github.com/dgraph-io/dgraph/issues/3268
[#4132]: https://github.com/dgraph-io/dgraph/issues/4132
[#4005]: https://github.com/dgraph-io/dgraph/issues/4005
[#4298]: https://github.com/dgraph-io/dgraph/issues/4298
[#4021]: https://github.com/dgraph-io/dgraph/issues/4021
[#3740]: https://github.com/dgraph-io/dgraph/issues/3740
[#4311]: https://github.com/dgraph-io/dgraph/issues/4311
[#4047]: https://github.com/dgraph-io/dgraph/issues/4047
[#4375]: https://github.com/dgraph-io/dgraph/issues/4375
[#4394]: https://github.com/dgraph-io/dgraph/issues/4394
[#4288]: https://github.com/dgraph-io/dgraph/issues/4288
[#4360]: https://github.com/dgraph-io/dgraph/issues/4360
[#4265]: https://github.com/dgraph-io/dgraph/issues/4265
[#4349]: https://github.com/dgraph-io/dgraph/issues/4349
[#4169]: https://github.com/dgraph-io/dgraph/issues/4169
[#4347]: https://github.com/dgraph-io/dgraph/issues/4347
[#4389]: https://github.com/dgraph-io/dgraph/issues/4389
[#4352]: https://github.com/dgraph-io/dgraph/issues/4352
[#4315]: https://github.com/dgraph-io/dgraph/issues/4315
[#4365]: https://github.com/dgraph-io/dgraph/issues/4365
[#4282]: https://github.com/dgraph-io/dgraph/issues/4282
[#4331]: https://github.com/dgraph-io/dgraph/issues/4331

## [1.1.0] - 2019-09-03
[1.1.0]: https://github.com/dgraph-io/dgraph/compare/v1.0.17...v1.1.0

### Changed

- **Breaking changes**

  - **uid schema type**: The `uid` schema type now means a one-to-one relation,
    **not** a one-to-many relation as in Dgraph v1.1. To specify a one-to-many
    relation in Dgraph v1.0, use the `[uid]` schema type. ([#2895][], [#3173][], [#2921][])

  - **\_predicate\_** is removed from the query language.

  - **expand(\_all\_)** only works for nodes with attached type information via
    the type system. The type system is used to determine the predicates to expand
    out from a node. ([#3262][])

  - **S \* \* deletion** only works for nodes with attached type information via
    the type system. The type system is used to determine the predicates to
    delete from a node. For `S * *` deletions, only the predicates specified by
    the type are deleted.

  - **HTTP API**: The HTTP API has been updated to replace the custom HTTP headers
    with standard headers.
    - Change `/commit` endpoint to accept a list of preds for conflict detection. ([#3020][])
    - Remove custom HTTP Headers, cleanup API. ([#3365][])
      - The startTs path parameter is now a query parameter `startTs` for the
        `/query`, `/mutate`, and `/commit` endpoints.
      - Dgraph custom HTTP Headers `X-Dgraph-CommitNow`,
        `X-Dgraph-MutationType`, and `X-Dgraph-Vars` are now ignored.
    - Update HTTP API Content-Type headers. ([#3550][]) ([#3532][])
      - Queries over HTTP must have the Content-Type header `application/graphql+-` or `application/json`.
      - Queries over HTTP with GraphQL Variables (e.g., `query queryName($a: string) { ... }`) must use the query format via `application/json` to pass query variables.
      - Mutations over HTTP must have the Content-Type header set to `application/rdf` for RDF format or `application/json` for JSON format.
      - Commits over HTTP must have the `startTs` query parameter along with the JSON map of conflict keys and predicates.

  - **Datetime index**: Use UTC Hour, Day, Month, Year for datetime
    comparison. This is a bug fix that may result in different query results for
    existing queries involving the datetime index. ([#3251][])

  - **Blank node name generation for JSON mutations.** For JSON mutations that
    do not explicitly set the `"uid"` field, the blank name format has changed
    to contain randomly generated identifiers. This fixes a bug where two JSON
    objects within a single mutation are assigned the same blank node.
    ([#3795][])

- Improve hash index. ([#2887][])
- Use a stream connection for internal connection health checking. ([#2956][])
- Use defer statements to release locks. ([#2962][])
- VerifyUid should wait for membership information. ([#2974][])
- Switching to perfect use case of sync.Map and remove the locks. ([#2976][])
- Tablet move and group removal. ([#2880][])
- Delete tablets which don't belong after tablet move. ([#3051][])
- Alphas inform Zero about tablets in its postings directory when Alpha starts. ([3271f64e0][])
- Prevent alphas from asking zero to serve tablets during queries. ([#3091][])
- Put data before extensions in JSON response. ([#3194][])
- Always parse language tag. ([#3243][])
- Populate the StartTs for the commit gRPC call so that clients can double check the startTs still matches. ([#3228][])
- Replace MD5 with SHA-256 in `dgraph cert ls`. ([#3254][])
- Fix use of deprecated function `grpc.WithTimeout()`. ([#3253][])
- Introduce multi-part posting lists. ([#3105][])
- Fix format of the keys to support startUid for multi-part posting lists. ([#3310][])
- Access groupi.gid atomically. ([#3402][])
- Move Raft checkpoint key to w directory. ([#3444][])
- Remove list.SetForDeletion method, remnant of the global LRU cache. ([#3481][])
- Whitelist by hostname. ([#2953][])
- Use CIDR format for whitelists instead of the previous range format.
- Introduce Badger's DropPrefix API into Dgraph to simplify how predicate deletions and drop all work internally. ([#3060][])
- Replace integer compression in UID Pack with groupvarint algorithm. ([#3527][], [#3650][])
- Rebuild reverse index before count reverse. ([#3688][])
- **Breaking change**: Use one atomic variable to generate blank node ids for
  json objects. This changes the format of automatically generated blank node
  names in JSON mutations. ([#3795][])
- Print commit SHA256 when invoking "make install". ([#3786][])
- Print SHA-256 checksum of Dgraph binary in the version section logs. ([#3828][])
- Change anonynmous telemetry endpoint. ([#3872][])
- Add support for API required for multiple mutations within a single call. ([#3839][])
- Make `lru_mb` optional. ([#3898][])
- Allow glog flags to be set via config file. ([#3062][], [#3077][])

- Logging
  - Suppress logging before `flag.Parse` from glog. ([#2970][])
  - Move glog of missing value warning to verbosity level 3. ([#3092][])
  - Change time threshold for Raft.Ready warning logs. ([#3901][])
  - Add log prefix to stream used to rebuild indices. ([#3696][])
  - Add additional logs to show progress of reindexing operation. ([#3746][])

- Error messages
  - Output the line and column number in schema parsing error messages. ([#2986][])
  - Improve error of empty block queries. ([#3015][])
  - Update flag description and error messaging related to `--query_edge_limit` flag. ([#2979][])
  - Reports line-column numbers for lexer/parser errors. ([#2914][])
  - Replace fmt.Errorf with errors.Errorf ([#3627][])
  - Return GraphQL compliant `"errors"` field for HTTP requests. ([#3728][])

- Optimizations
  - Don't read posting lists from disk when mutating indices. ([#3695][], [#3713][])
  - Avoid preallocating uid slice. It was slowing down unpackBlock.
  - Reduce memory consumption in bulk loader. ([#3724][])
  - Reduce memory consumptino by reusing lexer for parsing RDF. ([#3762][])
  - Use the stream framework to rebuild indices. ([#3686][])
  - Use Stream Writer for full snapshot transfer. ([#3442][])
  - Reuse postings and avoid fmt.Sprintf to reduce mem allocations ([#3767][])
  - Speed up JSON chunker. ([#3825][])
  - Various optimizations for Geo queries. ([#3805][])

- Update various govendor dependencies
  - Add OpenCensus deps to vendor using govendor. ([#2989][])
  - Govendor in latest dgo. ([#3078][])
  - Vendor in the Jaeger and prometheus exporters from their own repos ([#3322][])
  - Vendor in Shopify/sarama to use its Kafka clients. ([#3523][])
  - Update dgo dependency in vendor. ([#3412][])
  - Update vendored dependencies. ([#3357][])
  - Bring in latest changes from badger and fix broken API calls. ([#3502][])
  - Vendor badger with the latest changes. ([#3606][])
  - Vendor in badger, dgo and regenerate protobufs. ([#3747][])
  - Vendor latest badger. ([#3784][])
  - **Breaking change**: Vendor in latest Badger with data-format changes. ([#3906][])

Dgraph Debug Tool

- When looking up a key, print if it's a multi-part list and its splits. ([#3311][])
- Diagnose Raft WAL via debug tool. ([#3319][])
- Allow truncating Raft logs via debug tool. ([#3345][])
- Allow modifying Raft snapshot and hardstate in debug tool. ([#3364][])

Dgraph Live Loader / Dgraph Bulk Loader

- Add `--format` flag to Dgraph Live Loader and Dgraph Bulk Loader to specify input data format type. ([#2991][])
- Update live loader flag help text. ([#3278][])
- Improve reporting of aborts and retries during live load. ([#3313][])
- Remove xidmap storage on disk from bulk loader.
- Optimize XidtoUID map used by live and bulk loader. ([#2998][])
- Export data contains UID literals instead of blank nodes. Using Live Loader or Bulk Loader to load exported data will result in the same UIDs as the original database. ([#3004][], [#3045][]) To preserve the previous behavior, set the `--new_uids` flag in the live or bulk loader. ([18277872f][])
- Use StreamWriter in bulk loader. ([#3542][], [#3635][], [#3649][])
- Add timestamps during bulk/live load. ([#3287][])
- Use initial schema during bulk load. ([#3333][])
- Adding the verbose flag to suppress excessive logging in live loader. ([#3560][])
- Fix user meta of schema and type entries in bulk loader. ([#3628][])
- Check that all data files passed to bulk loader exist. ([#3681][])
- Handle non-list UIDs predicates in bulk loader. [#3659][]
- Use sync.Pool for MapEntries in bulk loader. ([#3763][], [802ec4c39][])

Dgraph Increment Tool

- Add server-side and client-side latency numbers to increment tool. ([#3422][])
- Add `--retries` flag to specify number of retry requests to set up a gRPC connection. ([#3584][])
- Add TLS support to `dgraph increment` command. ([#3257][])

### Added

- Add bash and zsh shell completion. See `dgraph completion bash --help` or `dgraph completion zsh --help` for usage instructions. ([#3084][])
- Add support for ECDSA in dgraph cert. ([#3269][])
- Add support for JSON export via `/admin/export?format=json`. ([#3309][])
- Add the SQL-to-Dgraph migration tool `dgraph migrate`. ([#3295][])
- Add `assign_timestamp_ns` latency field to fix encoding_ns calculation. Fixes [#3668][]. ([#3692][], [#3711][])
- Adding draining mode to Alpha. ([#3880][])


- Enterprise features
  - Support applying a license using /enterpriseLicense endpoint in Zero. ([#3824][])
  - Don't apply license state for oss builds. ([#3847][])

Query

- Type system
  - Add `type` function to query types. ([#2933][])
  - Parser for type declaration. ([#2950][])
  - Add `@type` directive to enforce type constraints. ([#3003][])
  - Store and query types. ([#3018][])
  - Rename type predicate to dgraph.type ([#3204][])
  - Change definition of dgraph.type pred to [string]. ([#3235][])
  - Use type when available to resolve expand predicates. ([#3214][])
  - Include types in results of export operation. ([#3493][])
  - Support types in the bulk loader. ([#3506][])

- Add the `upsert` block to send "query-mutate-commit" updates as a single
  call to Dgraph. This is especially helpful to do upserts with the `@upsert`
  schema directive. Addresses [#3059][]. ([#3412][])
  - Add support for conditional mutation in Upsert Block. ([#3612][])

- Allow querying all lang values of a predicate. ([#2910][])
- Allow `regexp()` in `@filter` even for predicates without the trigram index. ([#2913][])
- Add `minweight` and `maxweight` arguments to k-shortest path algorithm. ([#2915][])
- Allow variable assignment of `count(uid)`. ([#2947][])
- Reserved predicates
  - During startup, don't upsert initial schema if it already exists. ([#3374][])
  - Use all reserved predicates in IsReservedPredicateChanged. ([#3531][])
- Fuzzy match support via the `match()` function using the trigram index. ([#2916][])
- Support for GraphQL variables in arrays. ([#2981][])
- Show total weight of path in shortest path algorithm. ([#2954][])
- Rename dgraph `--dgraph` option to `--alpha`. ([#3273][])
- Support uid variables in `from` and `to` arguments for shortest path query. Fixes [#1243][]. ([#3710][])

- Add support for `len()` function in query language. The `len()` function is
  only used in the `@if` directive for upsert blocks. `len(v)` It returns the
  length of a variable `v`. ([#3756][], [#3769][])

Mutation

- Add ability to delete triples of scalar non-list predicates. ([#2899][], [#3843][])
- Allow deletion of specific language. ([#3242][])

Alter

- Add DropData operation to delete data without deleting schema. ([#3271][])

Schema

- **Breaking change**: Add ability to set schema to a single UID schema. Fixes [#2511][]. ([#2895][], [#3173][], [#2921][])
  - If you wish to create one-to-one edges, use the schema type `uid`. The `uid` schema type in v1.0.x must be changed to `[uid]` to denote a one-to-many uid edge.
- Prevent dropping or altering reserved predicates. ([#2967][]) ([#2997][])
  - Reserved predicate names start with `dgraph.` .
- Support comments in schema. ([#3133][])
- Reserved predicates
  - Reserved predicates are prefixed with "dgraph.", e.g., `dgraph.type`.
  - Ensure reserved predicates cannot be moved. ([#3137][])
  - Allow schema updates to reserved preds if the update is the same. ([#3143][])

Enterprise feature: Access Control Lists (ACLs)

Enterprise ACLs provide read/write/admin permissions to defined users and groups
at the predicate-level.

- Enforcing ACLs for query, mutation and alter requests. ([#2862][])
- Don't create ACL predicates when the ACL feature is not turned on. ([#2924][])
- Add HTTP API for ACL commands, pinning ACL predicates to group 1. ([#2951][])
- ACL: Using type to distinguish user and group. ([#3124][])
- Reduce the value of ACL TTLs to reduce the test running time. ([#3164][])
  - Adds `--acl_cache_ttl` flag.
- Fix panic when deleting a user or group that does not exist. ([#3218][])
- ACL over TLS. ([#3207][])
- Using read-only queries for ACL refreshes. ([#3256][])
- When HttpLogin response context error, unmarshal and return the response context. ([#3275][])
- Refactor: avoid double parsing of mutation string in ACL. ([#3494][])
- Security fix: prevent the HmacSecret from being logged. ([#3734][])

Enterprise feature: Backups

Enterprise backups are Dgraph backups in a binary format designed to be restored
to a cluster of the same version and configuration. Backups can be stored on
local disk or stored directly to the cloud via AWS S3 or any Minio-compatible
backend.

- Fixed bug with backup fan-out code. ([#2973][])
- Incremental backups / partial restore. ([#2963][])
- Turn obsolete error into warning. ([#3172][])
- Add `dgraph lsbackup` command to list backups. ([#3219][])
- Add option to override credentials and use public buckets. ([#3227][])
- Add field to backup requests to force a full backup. ([#3387][])
- More refactoring of backup code. ([#3515][])
- Use gzip compression in backups. ([#3536][])
- Allow partial restores and restoring different backup series. ([#3547][])
- Store group to predicate mapping as part of the backup manifest. ([#3570][])
- Only backup the predicates belonging to a group. ([#3621][])
- Introduce backup data formats for cross-version compatibility. ([#3575][])
-  Add series and backup number information to manifest. ([#3559][])
- Use backwards-compatible formats during backup ([#3629][])
- Use manifest to only restore preds assigned to each group. ([#3648][])
- Fixes the toBackupList function by removing the loop. ([#3869][])
- Add field to backup requests to force a full backup. ([#3387][])

Dgraph Zero

- Zero server shutdown endpoint `/shutdown` at Zero's HTTP port. ([#2928][])

Dgraph Live Loader

- Support live loading JSON files or stdin streams. ([#2961][]) ([#3106][])
- Support live loading N-Quads from stdin streams. ([#3266][])

Dgraph Bulk Loader

- Add `--replace_out` option to bulk command. ([#3089][])

Tracing

- Support exporting tracing data to oc_agent, then to datadog agent. ([#3398][])
- Measure latency of Alpha's Raft loop. (63f545568)

### Removed

- **Breaking change**: Remove `_predicate_` predicate within queries. ([#3262][])
- Remove `--debug_mode` option. ([#3441][])

- Remove deprecated and unused IgnoreIndexConflict field in mutations. This functionality is superceded by the `@upsert` schema directive since v1.0.4. ([#3854][])

- Enterprise features
  - Remove `--enterprise_feature` flag. Enterprise license can be applied via /enterpriseLicense endpoint in Zero. ([#3824][])

### Fixed

- Fix `anyofterms()` query for facets from mutations in JSON format. Fixes [#2867][]. ([#2885][])
- Fixes error found by gofuzz. ([#2914][])
- Fix int/float conversion to bool. ([#2893][])
- Handling of empty string to datetime conversion. ([#2891][])
- Fix schema export with special chars. Fixes [#2925][]. ([#2929][])

- Default value should not be nil. ([#2995][])
- Sanity check for empty variables. ([#3021][])
- Panic due to nil maps. ([#3042][])
- ValidateAddress should return true if IPv6 is valid. ([#3027][])
- Throw error when @recurse queries contain nested fields. ([#3182][])
- Fix panic in fillVars. ([#3505][])

- Fix race condition in numShutDownSig in Alpha. ([#3402][])
- Fix race condition in oracle.go. ([#3417][])
- Fix tautological condition in zero.go. ([#3516][])
- Correctness fix: Block before proposing mutations and improve conflict key generation. Fixes [#3528][]. ([#3565][])

- Reject requests with predicates larger than the max size allowed (longer than 65,535 characters). ([#3052][])
- Upgrade raft lib and fix group checksum. ([#3085][])
- Check that uid is not used as function attribute. ([#3112][])
- Do not retrieve facets when max recurse depth has been reached. ([#3190][])
- Remove obsolete error message. ([#3172][])
- Remove an unnecessary warning log. ([#3216][])
- Fix bug triggered by nested expand predicates. ([#3205][])
- Empty datetime will fail when returning results. ([#3169][])
- Fix bug with pagination using `after`. ([#3149][])
- Fix tablet error handling. ([#3323][])

- Fix crash when trying to use shortest path with a password predicate. Fixes [#3657][]. ([#3662][])
- Fix crash for `@groupby` queries. Fixes [#3642][]. ([#3670][])
- Fix crash when calling drop all during a query. Fixes [#3645][]. ([#3664][])
- Fix data races in queries. Fixes [#3685][]. ([#3749][])
- Bulk Loader: Fix memory usage by JSON parser. ([#3794][])
- Fixing issues in export. Fixes #3610. ([#3682][])

- Bug Fix: Use txn.Get in addReverseMutation if needed for count index ([#3874][])
- Bug Fix: Remove Check2 at writeResponse. ([#3900][])
- Bug Fix: Do not call posting.List.release.

[#3251]: https://github.com/dgraph-io/dgraph/issues/3251
[#3020]: https://github.com/dgraph-io/dgraph/issues/3020
[#3365]: https://github.com/dgraph-io/dgraph/issues/3365
[#3550]: https://github.com/dgraph-io/dgraph/issues/3550
[#3532]: https://github.com/dgraph-io/dgraph/issues/3532
[#3526]: https://github.com/dgraph-io/dgraph/issues/3526
[#3528]: https://github.com/dgraph-io/dgraph/issues/3528
[#3565]: https://github.com/dgraph-io/dgraph/issues/3565
[#2914]: https://github.com/dgraph-io/dgraph/issues/2914
[#2887]: https://github.com/dgraph-io/dgraph/issues/2887
[#2956]: https://github.com/dgraph-io/dgraph/issues/2956
[#2962]: https://github.com/dgraph-io/dgraph/issues/2962
[#2970]: https://github.com/dgraph-io/dgraph/issues/2970
[#2974]: https://github.com/dgraph-io/dgraph/issues/2974
[#2976]: https://github.com/dgraph-io/dgraph/issues/2976
[#2989]: https://github.com/dgraph-io/dgraph/issues/2989
[#3078]: https://github.com/dgraph-io/dgraph/issues/3078
[#3322]: https://github.com/dgraph-io/dgraph/issues/3322
[#3523]: https://github.com/dgraph-io/dgraph/issues/3523
[#3412]: https://github.com/dgraph-io/dgraph/issues/3412
[#3357]: https://github.com/dgraph-io/dgraph/issues/3357
[#3502]: https://github.com/dgraph-io/dgraph/issues/3502
[#3606]: https://github.com/dgraph-io/dgraph/issues/3606
[#3784]: https://github.com/dgraph-io/dgraph/issues/3784
[#3906]: https://github.com/dgraph-io/dgraph/issues/3906
[#2986]: https://github.com/dgraph-io/dgraph/issues/2986
[#3015]: https://github.com/dgraph-io/dgraph/issues/3015
[#2979]: https://github.com/dgraph-io/dgraph/issues/2979
[#2880]: https://github.com/dgraph-io/dgraph/issues/2880
[#3051]: https://github.com/dgraph-io/dgraph/issues/3051
[#3092]: https://github.com/dgraph-io/dgraph/issues/3092
[#3091]: https://github.com/dgraph-io/dgraph/issues/3091
[#3194]: https://github.com/dgraph-io/dgraph/issues/3194
[#3243]: https://github.com/dgraph-io/dgraph/issues/3243
[#3228]: https://github.com/dgraph-io/dgraph/issues/3228
[#3254]: https://github.com/dgraph-io/dgraph/issues/3254
[#3274]: https://github.com/dgraph-io/dgraph/issues/3274
[#3253]: https://github.com/dgraph-io/dgraph/issues/3253
[#3105]: https://github.com/dgraph-io/dgraph/issues/3105
[#3310]: https://github.com/dgraph-io/dgraph/issues/3310
[#3402]: https://github.com/dgraph-io/dgraph/issues/3402
[#3442]: https://github.com/dgraph-io/dgraph/issues/3442
[#3387]: https://github.com/dgraph-io/dgraph/issues/3387
[#3444]: https://github.com/dgraph-io/dgraph/issues/3444
[#3481]: https://github.com/dgraph-io/dgraph/issues/3481
[#2953]: https://github.com/dgraph-io/dgraph/issues/2953
[#3060]: https://github.com/dgraph-io/dgraph/issues/3060
[#3527]: https://github.com/dgraph-io/dgraph/issues/3527
[#3650]: https://github.com/dgraph-io/dgraph/issues/3650
[#3627]: https://github.com/dgraph-io/dgraph/issues/3627
[#3686]: https://github.com/dgraph-io/dgraph/issues/3686
[#3688]: https://github.com/dgraph-io/dgraph/issues/3688
[#3696]: https://github.com/dgraph-io/dgraph/issues/3696
[#3682]: https://github.com/dgraph-io/dgraph/issues/3682
[#3695]: https://github.com/dgraph-io/dgraph/issues/3695
[#3713]: https://github.com/dgraph-io/dgraph/issues/3713
[#3724]: https://github.com/dgraph-io/dgraph/issues/3724
[#3747]: https://github.com/dgraph-io/dgraph/issues/3747
[#3762]: https://github.com/dgraph-io/dgraph/issues/3762
[#3767]: https://github.com/dgraph-io/dgraph/issues/3767
[#3805]: https://github.com/dgraph-io/dgraph/issues/3805
[#3795]: https://github.com/dgraph-io/dgraph/issues/3795
[#3825]: https://github.com/dgraph-io/dgraph/issues/3825
[#3746]: https://github.com/dgraph-io/dgraph/issues/3746
[#3786]: https://github.com/dgraph-io/dgraph/issues/3786
[#3828]: https://github.com/dgraph-io/dgraph/issues/3828
[#3872]: https://github.com/dgraph-io/dgraph/issues/3872
[#3839]: https://github.com/dgraph-io/dgraph/issues/3839
[#3898]: https://github.com/dgraph-io/dgraph/issues/3898
[#3901]: https://github.com/dgraph-io/dgraph/issues/3901
[#3311]: https://github.com/dgraph-io/dgraph/issues/3311
[#3319]: https://github.com/dgraph-io/dgraph/issues/3319
[#3345]: https://github.com/dgraph-io/dgraph/issues/3345
[#3364]: https://github.com/dgraph-io/dgraph/issues/3364
[#2991]: https://github.com/dgraph-io/dgraph/issues/2991
[#3278]: https://github.com/dgraph-io/dgraph/issues/3278
[#3313]: https://github.com/dgraph-io/dgraph/issues/3313
[#2998]: https://github.com/dgraph-io/dgraph/issues/2998
[#3004]: https://github.com/dgraph-io/dgraph/issues/3004
[#3045]: https://github.com/dgraph-io/dgraph/issues/3045
[#3542]: https://github.com/dgraph-io/dgraph/issues/3542
[#3635]: https://github.com/dgraph-io/dgraph/issues/3635
[#3649]: https://github.com/dgraph-io/dgraph/issues/3649
[#3287]: https://github.com/dgraph-io/dgraph/issues/3287
[#3333]: https://github.com/dgraph-io/dgraph/issues/3333
[#3560]: https://github.com/dgraph-io/dgraph/issues/3560
[#3613]: https://github.com/dgraph-io/dgraph/issues/3613
[#3560]: https://github.com/dgraph-io/dgraph/issues/3560
[#3628]: https://github.com/dgraph-io/dgraph/issues/3628
[#3681]: https://github.com/dgraph-io/dgraph/issues/3681
[#3659]: https://github.com/dgraph-io/dgraph/issues/3659
[#3763]: https://github.com/dgraph-io/dgraph/issues/3763
[#3728]: https://github.com/dgraph-io/dgraph/issues/3728
[#3422]: https://github.com/dgraph-io/dgraph/issues/3422
[#3584]: https://github.com/dgraph-io/dgraph/issues/3584
[#3084]: https://github.com/dgraph-io/dgraph/issues/3084
[#3257]: https://github.com/dgraph-io/dgraph/issues/3257
[#3269]: https://github.com/dgraph-io/dgraph/issues/3269
[#3309]: https://github.com/dgraph-io/dgraph/issues/3309
[#3295]: https://github.com/dgraph-io/dgraph/issues/3295
[#3398]: https://github.com/dgraph-io/dgraph/issues/3398
[#3824]: https://github.com/dgraph-io/dgraph/issues/3824
[#3847]: https://github.com/dgraph-io/dgraph/issues/3847
[#3880]: https://github.com/dgraph-io/dgraph/issues/3880
[#2933]: https://github.com/dgraph-io/dgraph/issues/2933
[#2950]: https://github.com/dgraph-io/dgraph/issues/2950
[#3003]: https://github.com/dgraph-io/dgraph/issues/3003
[#3018]: https://github.com/dgraph-io/dgraph/issues/3018
[#3204]: https://github.com/dgraph-io/dgraph/issues/3204
[#3235]: https://github.com/dgraph-io/dgraph/issues/3235
[#3214]: https://github.com/dgraph-io/dgraph/issues/3214
[#3493]: https://github.com/dgraph-io/dgraph/issues/3493
[#3506]: https://github.com/dgraph-io/dgraph/issues/3506
[#3059]: https://github.com/dgraph-io/dgraph/issues/3059
[#3412]: https://github.com/dgraph-io/dgraph/issues/3412
[#3612]: https://github.com/dgraph-io/dgraph/issues/3612
[#2910]: https://github.com/dgraph-io/dgraph/issues/2910
[#2913]: https://github.com/dgraph-io/dgraph/issues/2913
[#2915]: https://github.com/dgraph-io/dgraph/issues/2915
[#2947]: https://github.com/dgraph-io/dgraph/issues/2947
[#3374]: https://github.com/dgraph-io/dgraph/issues/3374
[#3531]: https://github.com/dgraph-io/dgraph/issues/3531
[#2916]: https://github.com/dgraph-io/dgraph/issues/2916
[#2981]: https://github.com/dgraph-io/dgraph/issues/2981
[#2954]: https://github.com/dgraph-io/dgraph/issues/2954
[#3273]: https://github.com/dgraph-io/dgraph/issues/3273
[#1243]: https://github.com/dgraph-io/dgraph/issues/1243
[#3710]: https://github.com/dgraph-io/dgraph/issues/3710
[#3756]: https://github.com/dgraph-io/dgraph/issues/3756
[#3769]: https://github.com/dgraph-io/dgraph/issues/3769
[#2899]: https://github.com/dgraph-io/dgraph/issues/2899
[#3843]: https://github.com/dgraph-io/dgraph/issues/3843
[#3242]: https://github.com/dgraph-io/dgraph/issues/3242
[#3271]: https://github.com/dgraph-io/dgraph/issues/3271
[#2511]: https://github.com/dgraph-io/dgraph/issues/2511
[#2895]: https://github.com/dgraph-io/dgraph/issues/2895
[#3173]: https://github.com/dgraph-io/dgraph/issues/3173
[#2921]: https://github.com/dgraph-io/dgraph/issues/2921
[#2967]: https://github.com/dgraph-io/dgraph/issues/2967
[#2997]: https://github.com/dgraph-io/dgraph/issues/2997
[#3133]: https://github.com/dgraph-io/dgraph/issues/3133
[#2862]: https://github.com/dgraph-io/dgraph/issues/2862
[#2924]: https://github.com/dgraph-io/dgraph/issues/2924
[#2951]: https://github.com/dgraph-io/dgraph/issues/2951
[#3124]: https://github.com/dgraph-io/dgraph/issues/3124
[#3141]: https://github.com/dgraph-io/dgraph/issues/3141
[#3164]: https://github.com/dgraph-io/dgraph/issues/3164
[#3218]: https://github.com/dgraph-io/dgraph/issues/3218
[#3207]: https://github.com/dgraph-io/dgraph/issues/3207
[#3256]: https://github.com/dgraph-io/dgraph/issues/3256
[#3275]: https://github.com/dgraph-io/dgraph/issues/3275
[#3494]: https://github.com/dgraph-io/dgraph/issues/3494
[#3734]: https://github.com/dgraph-io/dgraph/issues/3734
[#2973]: https://github.com/dgraph-io/dgraph/issues/2973
[#2963]: https://github.com/dgraph-io/dgraph/issues/2963
[#3172]: https://github.com/dgraph-io/dgraph/issues/3172
[#3219]: https://github.com/dgraph-io/dgraph/issues/3219
[#3227]: https://github.com/dgraph-io/dgraph/issues/3227
[#3387]: https://github.com/dgraph-io/dgraph/issues/3387
[#3515]: https://github.com/dgraph-io/dgraph/issues/3515
[#3536]: https://github.com/dgraph-io/dgraph/issues/3536
[#3547]: https://github.com/dgraph-io/dgraph/issues/3547
[#3570]: https://github.com/dgraph-io/dgraph/issues/3570
[#3621]: https://github.com/dgraph-io/dgraph/issues/3621
[#3575]: https://github.com/dgraph-io/dgraph/issues/3575
[#3559]: https://github.com/dgraph-io/dgraph/issues/3559
[#3629]: https://github.com/dgraph-io/dgraph/issues/3629
[#3648]: https://github.com/dgraph-io/dgraph/issues/3648
[#3869]: https://github.com/dgraph-io/dgraph/issues/3869
[#2928]: https://github.com/dgraph-io/dgraph/issues/2928
[#2961]: https://github.com/dgraph-io/dgraph/issues/2961
[#3106]: https://github.com/dgraph-io/dgraph/issues/3106
[#3266]: https://github.com/dgraph-io/dgraph/issues/3266
[#3089]: https://github.com/dgraph-io/dgraph/issues/3089
[#3262]: https://github.com/dgraph-io/dgraph/issues/3262
[#3441]: https://github.com/dgraph-io/dgraph/issues/3441
[#3854]: https://github.com/dgraph-io/dgraph/issues/3854
[#3824]: https://github.com/dgraph-io/dgraph/issues/3824
[#2867]: https://github.com/dgraph-io/dgraph/issues/2867
[#2885]: https://github.com/dgraph-io/dgraph/issues/2885
[#2914]: https://github.com/dgraph-io/dgraph/issues/2914
[#2893]: https://github.com/dgraph-io/dgraph/issues/2893
[#2891]: https://github.com/dgraph-io/dgraph/issues/2891
[#2925]: https://github.com/dgraph-io/dgraph/issues/2925
[#2929]: https://github.com/dgraph-io/dgraph/issues/2929
[#2995]: https://github.com/dgraph-io/dgraph/issues/2995
[#3021]: https://github.com/dgraph-io/dgraph/issues/3021
[#3042]: https://github.com/dgraph-io/dgraph/issues/3042
[#3027]: https://github.com/dgraph-io/dgraph/issues/3027
[#3182]: https://github.com/dgraph-io/dgraph/issues/3182
[#3505]: https://github.com/dgraph-io/dgraph/issues/3505
[#3402]: https://github.com/dgraph-io/dgraph/issues/3402
[#3417]: https://github.com/dgraph-io/dgraph/issues/3417
[#3516]: https://github.com/dgraph-io/dgraph/issues/3516
[#3052]: https://github.com/dgraph-io/dgraph/issues/3052
[#3062]: https://github.com/dgraph-io/dgraph/issues/3062
[#3077]: https://github.com/dgraph-io/dgraph/issues/3077
[#3085]: https://github.com/dgraph-io/dgraph/issues/3085
[#3112]: https://github.com/dgraph-io/dgraph/issues/3112
[#3190]: https://github.com/dgraph-io/dgraph/issues/3190
[#3172]: https://github.com/dgraph-io/dgraph/issues/3172
[#3216]: https://github.com/dgraph-io/dgraph/issues/3216
[#3205]: https://github.com/dgraph-io/dgraph/issues/3205
[#3169]: https://github.com/dgraph-io/dgraph/issues/3169
[#3149]: https://github.com/dgraph-io/dgraph/issues/3149
[#3323]: https://github.com/dgraph-io/dgraph/issues/3323
[#3137]: https://github.com/dgraph-io/dgraph/issues/3137
[#3143]: https://github.com/dgraph-io/dgraph/issues/3143
[#3657]: https://github.com/dgraph-io/dgraph/issues/3657
[#3662]: https://github.com/dgraph-io/dgraph/issues/3662
[#3642]: https://github.com/dgraph-io/dgraph/issues/3642
[#3670]: https://github.com/dgraph-io/dgraph/issues/3670
[#3645]: https://github.com/dgraph-io/dgraph/issues/3645
[#3664]: https://github.com/dgraph-io/dgraph/issues/3664
[#3668]: https://github.com/dgraph-io/dgraph/issues/3668
[#3692]: https://github.com/dgraph-io/dgraph/issues/3692
[#3711]: https://github.com/dgraph-io/dgraph/issues/3711
[#3685]: https://github.com/dgraph-io/dgraph/issues/3685
[#3749]: https://github.com/dgraph-io/dgraph/issues/3749
[#3794]: https://github.com/dgraph-io/dgraph/issues/3794
[#3874]: https://github.com/dgraph-io/dgraph/issues/3874
[#3900]: https://github.com/dgraph-io/dgraph/issues/3900
[3271f64e0]: https://github.com/dgraph-io/dgraph/commit/3271f64e0
[63f545568]: https://github.com/dgraph-io/dgraph/commit/63f545568
[18277872f]: https://github.com/dgraph-io/dgraph/commit/18277872f
[802ec4c39]: https://github.com/dgraph-io/dgraph/commit/802ec4c39

## 1.0.18 - [Unreleased][Unreleased-v1.0.18]
[Unreleased-v1.0.18]: https://github.com/dgraph-io/dgraph/compare/v1.0.17...release/v1.0

### Fixed

- Preserve the order of entries in a mutation if multiple versions of the same
  edge are found. This addresses the mutation re-ordering change ([#2987][]) from v1.0.15.
- Fixing the zero client in live loader to avoid using TLS. Fixes [#3919][]. ([#3936][])
- Remove query cache which is causing contention. ([#4071][]).
- Fix bug when querying with nested levels of `expand(_all_)`. Fixes [#3807][]. ([#4143][]).
- Vendor in Badger to fix a vlog bug "Unable to find log file". ([#4212][])
- Change lexer to allow unicode escape sequences. Fixes [#4157][]. ([#4252][])

[#3919]: https://github.com/dgraph-io/dgraph/issues/3919
[#3936]: https://github.com/dgraph-io/dgraph/issues/3936
[#4071]: https://github.com/dgraph-io/dgraph/issues/4071
[#3807]: https://github.com/dgraph-io/dgraph/issues/3807
[#4143]: https://github.com/dgraph-io/dgraph/issues/4143
[#4212]: https://github.com/dgraph-io/dgraph/issues/4212
[#4157]: https://github.com/dgraph-io/dgraph/issues/4157
[#4252]: https://github.com/dgraph-io/dgraph/issues/4252

## [1.0.17] - 2019-08-30
[1.0.17]: https://github.com/dgraph-io/dgraph/compare/v1.0.16...v1.0.17

### Changed

- Increase max trace logs per span in Alpha. ([#3886][])
- Include line and column numbers in lexer errors. Fixes [#2900][]. ([#3772][])
- Release binaries built with Go 1.12.7.

### Fixed

- Decrease rate of Raft heartbeat messages. ([#3708][], [#3753][])
- Fix bug when exporting a predicate name to the schema. Fixes [#3699][]. ([#3701][])
- Return error instead of asserting in handleCompareFunction. ([#3665][])
- Fix bug where aliases in a query incorrectly alias the response depending on alias order. Fixes [#3814][]. ([#3837][])
- Fix for panic in fillGroupedVars. Fixes [#3768][]. ([#3781][])

[#3886]: https://github.com/dgraph-io/dgraph/issues/3886
[#2900]: https://github.com/dgraph-io/dgraph/issues/2900
[#3772]: https://github.com/dgraph-io/dgraph/issues/3772
[#3708]: https://github.com/dgraph-io/dgraph/issues/3708
[#3753]: https://github.com/dgraph-io/dgraph/issues/3753
[#3699]: https://github.com/dgraph-io/dgraph/issues/3699
[#3701]: https://github.com/dgraph-io/dgraph/issues/3701
[#3665]: https://github.com/dgraph-io/dgraph/issues/3665
[#3814]: https://github.com/dgraph-io/dgraph/issues/3814
[#3837]: https://github.com/dgraph-io/dgraph/issues/3837
[#3768]: https://github.com/dgraph-io/dgraph/issues/3768
[#3781]: https://github.com/dgraph-io/dgraph/issues/3781

## [1.0.16] - 2019-07-11
[1.0.16]: https://github.com/dgraph-io/dgraph/compare/v1.0.15...v1.0.16

### Changed

- Vendor in prometheus/client_golang/prometheus v0.9.4. ([#3653][])

### Fixed

- Fix panic with value variables in queries. Fixes [#3470][]. ([#3554][])
- Remove unused reserved predicates in the schema. Fixes [#3535][]. ([#3557][])
- Vendor in Badger v1.6.0 for StreamWriter bug fixes. ([#3631][])

[#3470]: https://github.com/dgraph-io/dgraph/issue/3470
[#3535]: https://github.com/dgraph-io/dgraph/issue/3535
[#3554]: https://github.com/dgraph-io/dgraph/issue/3554
[#3557]: https://github.com/dgraph-io/dgraph/issue/3557
[#3631]: https://github.com/dgraph-io/dgraph/issue/3631
[#3653]: https://github.com/dgraph-io/dgraph/issue/3653

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
- Optimize mutation and delta application. **Breaking: With these changes, the mutations within a single call are rearranged. So, no assumptions must be made about the order in which they get executed.**
 ([#2987][])
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
- Fix race condition in IsPeer. (#3432)

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
