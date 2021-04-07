# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project will adhere to [Calendar Versioning](https://calver.org/) starting v20.03.

## [20.11.3] - 2021-03-31
[20.11.3]: https://github.com/dgraph-io/dgraph/compare/v20.11.2...v20.11.3

### Fixed
- GraphQL
  - Fix(GRAPHQL): fix query rewriting for multiple order on nested field ([#7523][]) ([#7536][])
  - Fix(GRAPHQL): Added support for exact index on field having @id directive ([#7534][]) ([#7550][])
  - Fix(GraphQL): Add extra checks for deleting UpdateTypeInput ([#7595][]) ([#7600][])
  - Fix(GRAPHQL): Undo the breaking change and tag it as deprecated. ([#7607][])
  - Fix(GraphQL): Log query along with the panic ([#7638][]) ([#7645][])
  - Fix(GraphQL): Fix Execution Trace for Add and Update Mutations ([#7656][]) ([#7658][])

- Core Dgraph
  - Fix(schema): log error instead of panic if schema not found for predicate ([#7502][]) ([#7509][])
  - Chore(cmd/debuginfo) add new metrics to be collected ([#7439][]) ([#7562][])
  - Fix(vlog): Use Badger's value log threshold of 1MB ([#7415][]) ([#7474][])
  - Chore(bulk): Improve perf of bulk loader with Reuse allocator and assinging tags to allocator ([#7360][]) ([#7547][])
  - Fix(query): Fix pagination with match functions ([#7668][]) ([#7672][])

[#7523]: https://github.com/dgraph-io/dgraph/issues/7523
[#7536]: https://github.com/dgraph-io/dgraph/issues/7536
[#7534]: https://github.com/dgraph-io/dgraph/issues/7534
[#7550]: https://github.com/dgraph-io/dgraph/issues/7550
[#7595]: https://github.com/dgraph-io/dgraph/issues/7595
[#7600]: https://github.com/dgraph-io/dgraph/issues/7600
[#7607]: https://github.com/dgraph-io/dgraph/issues/7607
[#7638]: https://github.com/dgraph-io/dgraph/issues/7638
[#7645]: https://github.com/dgraph-io/dgraph/issues/7645
[#7656]: https://github.com/dgraph-io/dgraph/issues/7656
[#7658]: https://github.com/dgraph-io/dgraph/issues/7658
[#7502]: https://github.com/dgraph-io/dgraph/issues/7502
[#7509]: https://github.com/dgraph-io/dgraph/issues/7509
[#7439]: https://github.com/dgraph-io/dgraph/issues/7439
[#7562]: https://github.com/dgraph-io/dgraph/issues/7562
[#7415]: https://github.com/dgraph-io/dgraph/issues/7415
[#7474]: https://github.com/dgraph-io/dgraph/issues/7474
[#7360]: https://github.com/dgraph-io/dgraph/issues/7360
[#7547]: https://github.com/dgraph-io/dgraph/issues/7547
[#7668]: https://github.com/dgraph-io/dgraph/issues/7668
[#7672]: https://github.com/dgraph-io/dgraph/issues/7672

## [20.11.2] - 2021-02-23
[20.11.2]: https://github.com/dgraph-io/dgraph/compare/v20.11.1...v20.11.2

### Fixed
- GraphQL
  - Fix(Mutation): Deeply-nested uid facets ([#7457][])
  - Fix(GraphQL): Fix panic caused by incorrect input coercion of scalar to list ([#7405][]) ([#7428][])
  - Fix(GraphQL): Refactor Mutation Rewriter for Add and Update Mutations ([#7409][]) ([#7413][])
  - Fix(GraphQL): fix `@auth` rules evaluation in case of null values. ([#7411][])
  - Fix(GraphQL): fix interface query with auth rules ([#7408][])
  - Fix(GraphQL): Added error for case when multiple filter functions are  used in filter. ([#7368][]) ([#7384][])

- Core Dgraph
  - Fix(sort): Fix multi-sort with nils ([#7432][]) ([#7444][])
  - Fix(GC): Reduce DiscardRatio from 0.9 to 0.7 ([#7412][]) ([#7421][])

- Enterprise Features
  - Fix(export-backup): fix memory leak in backup export ([#7452][]) ([#7453][])

[#7457]: https://github.com/dgraph-io/dgraph/issues/7457
[#7405]: https://github.com/dgraph-io/dgraph/issues/7405
[#7428]: https://github.com/dgraph-io/dgraph/issues/7428
[#7409]: https://github.com/dgraph-io/dgraph/issues/7409
[#7413]: https://github.com/dgraph-io/dgraph/issues/7413
[#7411]: https://github.com/dgraph-io/dgraph/issues/7411
[#7408]: https://github.com/dgraph-io/dgraph/issues/7408
[#7368]: https://github.com/dgraph-io/dgraph/issues/7368
[#7384]: https://github.com/dgraph-io/dgraph/issues/7384
[#7432]: https://github.com/dgraph-io/dgraph/issues/7432
[#7444]: https://github.com/dgraph-io/dgraph/issues/7444
[#7412]: https://github.com/dgraph-io/dgraph/issues/7412
[#7421]: https://github.com/dgraph-io/dgraph/issues/7421
[#7452]: https://github.com/dgraph-io/dgraph/issues/7452
[#7453]: https://github.com/dgraph-io/dgraph/issues/7453

## [20.11.1] - 2021-01-27
[20.11.1]: https://github.com/dgraph-io/dgraph/compare/v20.11.0...v20.11.1

### Fixed
- GraphQL
  - Fix(subscriptions): fix subscription to use the kv with the max version ([#7349][]) ([#7355][])
  - Fix(GraphQl): fix a panic when we pass a single ID as a integer and expected type is `[ID]`.We
   now coerce that to type array of string.  ([#7325][]) ([#7353][])
  - Fix(GRAPHQL): update gqlparser release to v2.1.4 ([#7347][]) ([#7352][])
  - Fix(GraphQL): Fix graphql flaky tests which were caused by receiving extra schema updates
    ([#7329][]) ([#7348][])
  - Fix(GraphQL): This PR addd input coercion from single object to list and fix panic when we
   pass single ID in filter as a string. ([#7133][]) ([#7306][])
  - Fix(GRAPHQL): Don't generate get query on interface if it doesn't have field of type ID and
   also disallow get query on field of type `@id` in inerface. ([#7158][]) ([#7305][])
  - Fix(GraphQL): This PR fix multi cors and multi schema nodes issue by selecting one of the
   latest added nodes, and add dgraph type to cors. ([#7270][]) ([#7302][])
  - Fix(GraphQL): This PR allow to use __typename in mutation. ([#7285][]) ([#7303][])
  - Fix(GraphQL): Fix auth-token propagation for HTTP endpoints resolved through GraphQL (GRAPHQL
  -946) ([#7245][]) ([#7251][])

- Core Dgraph
  - Fix(bulk): save schemaMap after map phase ([#7188][]) ([#7351][])
  - Fix(Dgraph): check for deleteBelowTs in pIterator.valid ([#7288][]) ([#7350][])
  - Fix(indexing): use encrypted tmpDBs for index building if encryption is enabled ([#6828][]) ([#7343][])
  - Fix(bulk): Fix memory held by b+ tree in reduce phase ([#7161][]) ([#7333][])
  - Feat(bulk): Add /jemalloc HTTP endpoint. ([#7165][]) ([#7331][])
  - Fix(sort): Make sort consistent for indexed and without indexed predicates ([#7241][]) ([#7323][])
  - Fix(dgraph): Add X-Dgraph-AuthToken to list of access control allowed headers ([#7311][])
  - Fix(ludicrous): Fix logical race in concurrent execution of mutations ([#7269][]) ([#7309][])
  - Fix(ludicrous): Fix data race in executor ([#7203][]) ([#7307][])
  - Opt(rollup): change the way rollups are done ([#7253][]) ([#7277][])
  - Fix(indexing): use --tmp directory for building indexes ([#7289][]) ([#7300][])
  - Fix(dgraph): Fix dgraph crash on windows ([#7261][]) ([#7299][])
  - Fix(dgraph): making jemalloc to work with dgraph on macos ([#7247][]) ([#7282][])
  - Fix(dgraph): Fixing multiple race conditions ([#7278][])
  - Fixing unique proposal key error ([#7218][]) ([#7281][])
  - Fix(raft): Unmarshal zero snapshot into pb.ZeroSnaphot ([#7244][])
  - Fix(bulk): fixing bulk loader when encryption + mtls is enabled ([#7154][]) ([#7155][])

- Enterprise Features
  - Fix(restore): reset acl accounts once restore is done if necessary ([#7202][]) ([#7280][])
  - Fix(restore): multiple restore requests should be rejected and proposals should not be submitted ([#7118][]) ([#7276][])
  - Fix(restore): Handle MaxUid=0 appropriately ([#7258][]) ([#7265][])

[#7349]: https://github.com/dgraph-io/dgraph/issues/7349
[#7355]: https://github.com/dgraph-io/dgraph/issues/7355
[#7188]: https://github.com/dgraph-io/dgraph/issues/7188
[#7351]: https://github.com/dgraph-io/dgraph/issues/7351
[#7288]: https://github.com/dgraph-io/dgraph/issues/7288
[#7350]: https://github.com/dgraph-io/dgraph/issues/7350
[#7325]: https://github.com/dgraph-io/dgraph/issues/7325
[#7353]: https://github.com/dgraph-io/dgraph/issues/7353
[#7347]: https://github.com/dgraph-io/dgraph/issues/7347
[#7352]: https://github.com/dgraph-io/dgraph/issues/7352
[#6828]: https://github.com/dgraph-io/dgraph/issues/6828
[#7343]: https://github.com/dgraph-io/dgraph/issues/7343
[#7329]: https://github.com/dgraph-io/dgraph/issues/7329
[#7348]: https://github.com/dgraph-io/dgraph/issues/7348
[#7161]: https://github.com/dgraph-io/dgraph/issues/7161
[#7333]: https://github.com/dgraph-io/dgraph/issues/7333
[#7165]: https://github.com/dgraph-io/dgraph/issues/7165
[#7331]: https://github.com/dgraph-io/dgraph/issues/7331
[#7241]: https://github.com/dgraph-io/dgraph/issues/7241
[#7323]: https://github.com/dgraph-io/dgraph/issues/7323
[#7311]: https://github.com/dgraph-io/dgraph/issues/7311
[#7269]: https://github.com/dgraph-io/dgraph/issues/7269
[#7309]: https://github.com/dgraph-io/dgraph/issues/7309
[#7133]: https://github.com/dgraph-io/dgraph/issues/7133
[#7306]: https://github.com/dgraph-io/dgraph/issues/7306
[#7158]: https://github.com/dgraph-io/dgraph/issues/7158
[#7305]: https://github.com/dgraph-io/dgraph/issues/7305
[#7270]: https://github.com/dgraph-io/dgraph/issues/7270
[#7302]: https://github.com/dgraph-io/dgraph/issues/7302
[#7285]: https://github.com/dgraph-io/dgraph/issues/7285
[#7303]: https://github.com/dgraph-io/dgraph/issues/7303
[#7203]: https://github.com/dgraph-io/dgraph/issues/7203
[#7307]: https://github.com/dgraph-io/dgraph/issues/7307
[#7253]: https://github.com/dgraph-io/dgraph/issues/7253
[#7277]: https://github.com/dgraph-io/dgraph/issues/7277
[#7289]: https://github.com/dgraph-io/dgraph/issues/7289
[#7300]: https://github.com/dgraph-io/dgraph/issues/7300
[#7261]: https://github.com/dgraph-io/dgraph/issues/7261
[#7299]: https://github.com/dgraph-io/dgraph/issues/7299
[#7247]: https://github.com/dgraph-io/dgraph/issues/7247
[#7282]: https://github.com/dgraph-io/dgraph/issues/7282
[#7278]: https://github.com/dgraph-io/dgraph/issues/7278
[#7202]: https://github.com/dgraph-io/dgraph/issues/7202
[#7280]: https://github.com/dgraph-io/dgraph/issues/7280
[#7218]: https://github.com/dgraph-io/dgraph/issues/7218
[#7281]: https://github.com/dgraph-io/dgraph/issues/7281
[#7118]: https://github.com/dgraph-io/dgraph/issues/7118
[#7276]: https://github.com/dgraph-io/dgraph/issues/7276
[#7258]: https://github.com/dgraph-io/dgraph/issues/7258
[#7265]: https://github.com/dgraph-io/dgraph/issues/7265
[#7245]: https://github.com/dgraph-io/dgraph/issues/7245
[#7251]: https://github.com/dgraph-io/dgraph/issues/7251
[#7244]: https://github.com/dgraph-io/dgraph/issues/7244
[#7154]: https://github.com/dgraph-io/dgraph/issues/7154
[#7155]: https://github.com/dgraph-io/dgraph/issues/7155

## [20.11.0] - 2020-12-16
[20.11.0]: https://github.com/dgraph-io/dgraph/compare/v20.07.0...v20.11.0

### Changed

- [BREAKING] Feat: Use snappy compression by default. ([#6697][])
- [BREAKING] Fix(OOM): Don't unmarshal pb.Proposals until we need them ([#7059][])
- [BREAKING] Feat(Dgraph): Use Badger with new WAL format. ([#6643][])
- [BREAKING] Switch Raft WAL to use simple files ([#6572][])
- Feat(tls):  splitting tls_dir + making health point available on HTTP ([#6821][])

### Added

- GraphQL
  - Feat(GraphQL): Add Aggregation Queries at Child Level ([#7022][])
  - Feat(GraphQL): Add aggregate query at root level ([#6985][])
  - Feat(GraphQL): Mutations with Auth on interfaces should work correctly. ([#6839][])
  - Feat(GraphQL): This PR adds support for "application/dql" in content header.  ([#6849][])
  - Feat(GraphQL): Add count queries Feature at non-root levels ([#6834][])
  - Fix(GraphQL): AND/OR filters now accept an array while also accepting objects. ([#6801][])
  - Feat(GraphQL): Allow Query with Auth rules on Interfaces ([#6776][])
  - Feat(GraphQL): This PR adds auth switch in GraphQL authorization header.  ([#6779][])
  - Feat(GraphQL): Add count query Feature at root to GraphQL ([#6786][])
  - Feat(GraphQL): Add generate directive to graphql schema ([#6760][])
  - Feat(GraphQL): add support for all RSA and HMAC algorithms supported by github.com/dgrijalva/jwt-go/v4 ([#6750][])
  - Feat(GraphQL): allow duplicate XIDs if only XID value is repeated ([#6762][])
  - Feat(GraphQL): Add support for Polygon and Multi-Polygon in GraphQL ([#6618][])
  - Feat(GraphQL): add support for between filter in GraphQL ([#6651][])
  - Feat(GraphQL): Unions ([#6722][])
  - Feat(GraphQL): add support for IN filter ([#6662][])
  - Feat(GraphQL): Add support for Geo point type in Graphql. ([#6481][])
  - Feat(GraphQL): GraphQL now has lambda resolvers ([#6574][])
  - Feat(GraphQL): Support authorization with jwk_url ([#6564][])
  - Feat(GraphQL): GQL Logging MW for admin query/mutation ([#6562][])
  - Feat: add schema history to graphql ([#6324][])
  - Feat(GraphQL): Add GraphQL schema validation Endpoint. ([#6250][])
  - Feat(GraphQL): This PR adds parameterised cascade in graphql. ([#6251][])
  - Feat(GraphQL): add has filter support ([#6258][])
  - Feat(GraphQL): GraphQL now has Int64 as scalar type ([#6200][])
  - Feat(GraphQL): `@custom` HTTP body now supports hardcoded scalars ([#6157][])
  - Feat(GraphQL): Custom logic now supports DQL queries ([#6115][])
  - Feat(GraphQL): This PR allows to return errors from custom REST endpoint. ([#6604][])

- Core Dgraph
  - Feat(dgraph): Add suport for RDF query. ([#6038][])
  - perf(xidmap): Use btree with hash of keys for xidmap ([#6902][])
  - Feat(Query): Enable persistent queries in dgraph ([#6788][])
  - Feat(Dgraph): Add ability to change size of caches through the admin interface. ([#6644][])
  - Feat(query): Support for between func with count at root ([#6556][])
  - Feat(querylang): language support for term tokenization ([#6269][])
  - Feat(ludicrous): Run mutations from the same predicate concurrently in ludicrous mode ([#6060][])
  - Feat(Dgraph): Add experimental cache for posting lists ([#6245][])
  - Feat(dgraph): making all internal communications with tls configured ([#6692][])
  - Feat(dgraph): enabling TLS config in http zero ([#6691][])
  - Feat(raftwal): Add support for encryption in raftwal ([#6714][])
  - Feat(Dgraph): add utility to export backup data. ([#6550][])
  - Feature: dgraph_txn_aborts metric for prometheus ([#6171][])
  - Feat(live): added upsert in live loader ([#6057][])

- Enterprise Features
  - Feat(Dgraph): Online restores allows to restore a specific backup. ([#6411][])

### Fixed

- GraphQL
  - Fix(GraphQL): Fix internal Aliases name generation ([#7009][])
  - Fix(GraphQL): Allows repetition of fields inside implementing type in
   interface and allow to inherit field of same name of type ID  from multiple interfaces. ([#7053][])
  - Fix(GraphQL): Fix password query rewriting in release/v20.11 ([#7012][])
  - Fix(GraphQL): Fix bug with password query rewriting ([#7011][])
  - Fix(GraphQL): Use fragments on interfaces while querying other interface.  ([#6964][])
  - Fix(GraphQL): Fix multiple alias in query ([#6940][])
  - Fix(GraphQL): Add support for using auth with secret directive ([#6920][])
  - Fix(GraphQL): Fix exclusion of filters in Query generation ([#6917][])
  - Fix(GraphQL): handle filters for enum properly ([#6916][])
  - Fix(GraphQL): Fixes issue of multiple responses in a subscription for an update. ([#6868][])
  - Fix(GraphQL): Fix panic caused when trying to delete a nested object which doesn't have id/xid ([#6810][])
  - Fix(GraphQL): Fix between filter bugs ([#6822][])
  - Fix(GraphQL): Fix panic error when we give null value in filter connectives. ([#6707][])
  - Fix(GraphQL): Remove extra fields when querying interfaces ([#6596][])
  - Fix(GraphQL): disallowing field names with as ([#6579][])
  - Fix(GraphQL): Fix object Linking with `hasInverse` ([#6557][])
  - Fix(GraphQL): Fix cascade with auth query when RBAC is false ([#6444][])
  - Fix(GraphQL): Generate correct schema when no orderable field in a type ([#6456][])
  - Fix(GraphQL): Fix  restoreStatus query with query variables ([#6414][])
  - Fix(GraphQL): Fix for deletion on interfaces with no non Id field ([#6387][])
  - Fix(GraphQL): don't generate orderable enum value for list fields ([#6392][])
  - Fix(GraphQL): Fix introspection completion bug ([#6385][])
  - Fix(GraphQL): Extend int64 range to 64-bit numeric values and adds input coercing and
   validation for integers. ([#6275][])
  - Fix(GraphQL): Remove auth error from mutation. ([#6329][])
  - Fix(GraphQL): Fix query rewriting for auth delete when deleting types with inverse field. ([#6350][])
  - Fix(GraphQL): incorrect generatedSchema in updateGQLSchema ([#6349][])
  - Fix(GraphQL): Link xids properly if there are duplicate xids within the same add request. ([#6265][])
  - Fix(GraphQL): Fix internal error when doing GraphQL schema introspection after drop all ([#6268][])
  - Fix(GraphQL): Fixes unexpected fragment behaviour ([#6228][])
  - Fix(GraphQL): Fix order and offset in auth queries. ([#6221][])
  - Fix(GraphQL): Linking of xids for deep mutations ([#6172][])
  - Fix(GraphQL): Don't reserve certain queries/mutations/inputs when a type is remote. ([#6055][])
  - Fix(GraphQl): Allow case insensitive auth header for graphql subscriptions. ([#6141][])
  - Fix(GraphQl): Panic Fix when subscription expiry is not present in jwt. ([#6129][])
  - Fix(GraphQL): Fix bug in custom resolver, now body need not have all the fields. ([#6054][])
  - Fix(GraphQL):  Disallow  Subscription typename. ([#6077][])
  - Fix(GraphQL): Fixes wrong query parameter value for custom field URL ([#6074][])
  - Fix(GraphQL): Fixes panic in update mutation without set & remove ([#6073][])
  - Fix(GraphQL): Fix auth rewriting for nested queries when RBAC rule is true. ([#6053][])
  - Fix(GraphQL): Fix getType queries when id was used as a name for types other than ID ([#6130][])

- Core Dgraph
  - Fix(ludicrous mode): Handle deletes correctly ([#6773][])
  - Fix(Zero): Fix how Zero snapshots and purge works ([#7096][])
  - Fix: Check for nil ServerCloser in shutdown handler ([#7048][])
  - Fix(health): Update health only after upserting schema and types ([#7006][])
  - Fix(worker): Flush the stream writer on error (DGRAPH-2499) ([#6609][])
  - Fix(export): don't return an error if there was no GraphQL schema ([#6815][])
  - Fix pointer misalignment ([#6795][])
  - Fix(metrics): Show memory metrics for zero ([#6743][])
  - feat(Query): Allow filters in expand(_all_) queries on predicates pointing to nodes ([#6752][])
  - Fix(Ludicrous): Upserts on list type in Dgraph ([#6754][])
  - Fix(worker): Avoid panic in handleUidPostings ([#6607][])
  - Fix(config): Set glog -v flag correctly from config files. ([#6678][])
  - Fix ErrIndexingInProgress if schema update fails ([#6583][])
  - feat(bulk): Allow encrypted input with unencrypted output in bulk. ([#6541][])
  - Fix(Dgraph): Subscribe to ACL updates instead of polling. ([#6459][])
  - Fix(Alpha): Immediately take a snapshot if we don't have one ([#6458][])
  - Fix(Dgraph): Fix bug when deleting and adding to a single UID predicate in the same transaction. ([#6431][])
  - Fix(raft): Only leader should check the quorum ([#6323][])
  - Fix(Dgraph): Parse Content-Type in headers correctly ([#6370][])
  - Fix(shutdown): Force exit if CTRL-C is caught before initialization ([#6359][])
  - Fix(Query) Fix Star_All delete query when used with ACL enabled ([#6331][])
  - Fix(Alpha): MASA: Make Alpha Shutdown Again ([#6313][])
  - Fix(Dgraph): Fix how visited nodes are detected in recurse queries. ([#6272][])
  - Fix(Dgraph): make backups cancel other tasks ([#6152][])
  - Fix(Dgraph): Don't store start_ts in postings. ([#6206][])
  - Fix(Dgraph): Perform rollups more aggresively. ([#6143][])
  - Fix(rollups): rollup a batch if more than 2 seconds elapsed since last batch ([#6118][])

- Enterprise Features
  - Fix(enterprise): Set version correctly post marshalling during restore ([#7018][])
  - Add badger.compression to Dgraph restore ([#6987][])
  - Fix(backup/restore): Fixes backup and restore with DROP operations (GRAPHQL-735) ([#6844][])
  - Fix(ACL) : Disallow deleting of groot user and guardians group ([#6580][])
  - Fix: Online Restore honors credentials passed in ([#6295][])
  - Fix(ACL Query): Fixes queries which use variable at the top level ([#6290][])
  - Fix(Dgraph): race condition in EnterpriseEnabled() ([#6793][])

[#6697]: https://github.com/dgraph-io/dgraph/issues/6697
[#7059]: https://github.com/dgraph-io/dgraph/issues/7059
[#6643]: https://github.com/dgraph-io/dgraph/issues/6643
[#6572]: https://github.com/dgraph-io/dgraph/issues/6572
[#6821]: https://github.com/dgraph-io/dgraph/issues/6821
[#7022]: https://github.com/dgraph-io/dgraph/issues/7022
[#6985]: https://github.com/dgraph-io/dgraph/issues/6985
[#6839]: https://github.com/dgraph-io/dgraph/issues/6839
[#6849]: https://github.com/dgraph-io/dgraph/issues/6849
[#6834]: https://github.com/dgraph-io/dgraph/issues/6834
[#6801]: https://github.com/dgraph-io/dgraph/issues/6801
[#6776]: https://github.com/dgraph-io/dgraph/issues/6776
[#6779]: https://github.com/dgraph-io/dgraph/issues/6779
[#6786]: https://github.com/dgraph-io/dgraph/issues/6786
[#6760]: https://github.com/dgraph-io/dgraph/issues/6760
[#6750]: https://github.com/dgraph-io/dgraph/issues/6750
[#6762]: https://github.com/dgraph-io/dgraph/issues/6762
[#6618]: https://github.com/dgraph-io/dgraph/issues/6618
[#6651]: https://github.com/dgraph-io/dgraph/issues/6651
[#6722]: https://github.com/dgraph-io/dgraph/issues/6722
[#6662]: https://github.com/dgraph-io/dgraph/issues/6662
[#6481]: https://github.com/dgraph-io/dgraph/issues/6481
[#6574]: https://github.com/dgraph-io/dgraph/issues/6574
[#6564]: https://github.com/dgraph-io/dgraph/issues/6564
[#6562]: https://github.com/dgraph-io/dgraph/issues/6562
[#6324]: https://github.com/dgraph-io/dgraph/issues/6324
[#6250]: https://github.com/dgraph-io/dgraph/issues/6250
[#6251]: https://github.com/dgraph-io/dgraph/issues/6251
[#6258]: https://github.com/dgraph-io/dgraph/issues/6258
[#6200]: https://github.com/dgraph-io/dgraph/issues/6200
[#6157]: https://github.com/dgraph-io/dgraph/issues/6157
[#6038]: https://github.com/dgraph-io/dgraph/issues/6038
[#6115]: https://github.com/dgraph-io/dgraph/issues/6115
[#6604]: https://github.com/dgraph-io/dgraph/issues/6604
[#6902]: https://github.com/dgraph-io/dgraph/issues/6902
[#6788]: https://github.com/dgraph-io/dgraph/issues/6788
[#6773]: https://github.com/dgraph-io/dgraph/issues/6773
[#6644]: https://github.com/dgraph-io/dgraph/issues/6644
[#6556]: https://github.com/dgraph-io/dgraph/issues/6556
[#6269]: https://github.com/dgraph-io/dgraph/issues/6269
[#6060]: https://github.com/dgraph-io/dgraph/issues/6060
[#6245]: https://github.com/dgraph-io/dgraph/issues/6245
[#6692]: https://github.com/dgraph-io/dgraph/issues/6692
[#6691]: https://github.com/dgraph-io/dgraph/issues/6691
[#6714]: https://github.com/dgraph-io/dgraph/issues/6714
[#6550]: https://github.com/dgraph-io/dgraph/issues/6550
[#6171]: https://github.com/dgraph-io/dgraph/issues/6171
[#6057]: https://github.com/dgraph-io/dgraph/issues/6057
[#6411]: https://github.com/dgraph-io/dgraph/issues/6411
[#7009]: https://github.com/dgraph-io/dgraph/issues/7009
[#7053]: https://github.com/dgraph-io/dgraph/issues/7053
[#7012]: https://github.com/dgraph-io/dgraph/issues/7012
[#7011]: https://github.com/dgraph-io/dgraph/issues/7011
[#6964]: https://github.com/dgraph-io/dgraph/issues/6964
[#6940]: https://github.com/dgraph-io/dgraph/issues/6940
[#6920]: https://github.com/dgraph-io/dgraph/issues/6920
[#6917]: https://github.com/dgraph-io/dgraph/issues/6917
[#6916]: https://github.com/dgraph-io/dgraph/issues/6916
[#6868]: https://github.com/dgraph-io/dgraph/issues/6868
[#6810]: https://github.com/dgraph-io/dgraph/issues/6810
[#6822]: https://github.com/dgraph-io/dgraph/issues/6822
[#6707]: https://github.com/dgraph-io/dgraph/issues/6707
[#6596]: https://github.com/dgraph-io/dgraph/issues/6596
[#6579]: https://github.com/dgraph-io/dgraph/issues/6579
[#6557]: https://github.com/dgraph-io/dgraph/issues/6557
[#6444]: https://github.com/dgraph-io/dgraph/issues/6444
[#6456]: https://github.com/dgraph-io/dgraph/issues/6456
[#6414]: https://github.com/dgraph-io/dgraph/issues/6414
[#6387]: https://github.com/dgraph-io/dgraph/issues/6387
[#6392]: https://github.com/dgraph-io/dgraph/issues/6392
[#6385]: https://github.com/dgraph-io/dgraph/issues/6385
[#6275]: https://github.com/dgraph-io/dgraph/issues/6275
[#6329]: https://github.com/dgraph-io/dgraph/issues/6329
[#6350]: https://github.com/dgraph-io/dgraph/issues/6350
[#6349]: https://github.com/dgraph-io/dgraph/issues/6349
[#6265]: https://github.com/dgraph-io/dgraph/issues/6265
[#6268]: https://github.com/dgraph-io/dgraph/issues/6268
[#6228]: https://github.com/dgraph-io/dgraph/issues/6228
[#6221]: https://github.com/dgraph-io/dgraph/issues/6221
[#6172]: https://github.com/dgraph-io/dgraph/issues/6172
[#6055]: https://github.com/dgraph-io/dgraph/issues/6055
[#6141]: https://github.com/dgraph-io/dgraph/issues/6141
[#6129]: https://github.com/dgraph-io/dgraph/issues/6129
[#6054]: https://github.com/dgraph-io/dgraph/issues/6054
[#6077]: https://github.com/dgraph-io/dgraph/issues/6077
[#6074]: https://github.com/dgraph-io/dgraph/issues/6074
[#6073]: https://github.com/dgraph-io/dgraph/issues/6073
[#6053]: https://github.com/dgraph-io/dgraph/issues/6053
[#6130]: https://github.com/dgraph-io/dgraph/issues/6130
[#7096]: https://github.com/dgraph-io/dgraph/issues/7096
[#7048]: https://github.com/dgraph-io/dgraph/issues/7048
[#7006]: https://github.com/dgraph-io/dgraph/issues/7006
[#6609]: https://github.com/dgraph-io/dgraph/issues/6609
[#6815]: https://github.com/dgraph-io/dgraph/issues/6815
[#6795]: https://github.com/dgraph-io/dgraph/issues/6795
[#6743]: https://github.com/dgraph-io/dgraph/issues/6743
[#6752]: https://github.com/dgraph-io/dgraph/issues/6752
[#6754]: https://github.com/dgraph-io/dgraph/issues/6754
[#6607]: https://github.com/dgraph-io/dgraph/issues/6607
[#6678]: https://github.com/dgraph-io/dgraph/issues/6678
[#6583]: https://github.com/dgraph-io/dgraph/issues/6583
[#6541]: https://github.com/dgraph-io/dgraph/issues/6541
[#6459]: https://github.com/dgraph-io/dgraph/issues/6459
[#6458]: https://github.com/dgraph-io/dgraph/issues/6458
[#6431]: https://github.com/dgraph-io/dgraph/issues/6431
[#6323]: https://github.com/dgraph-io/dgraph/issues/6323
[#6370]: https://github.com/dgraph-io/dgraph/issues/6370
[#6359]: https://github.com/dgraph-io/dgraph/issues/6359
[#6331]: https://github.com/dgraph-io/dgraph/issues/6331
[#6313]: https://github.com/dgraph-io/dgraph/issues/6313
[#6272]: https://github.com/dgraph-io/dgraph/issues/6272
[#6152]: https://github.com/dgraph-io/dgraph/issues/6152
[#6206]: https://github.com/dgraph-io/dgraph/issues/6206
[#6143]: https://github.com/dgraph-io/dgraph/issues/6143
[#6118]: https://github.com/dgraph-io/dgraph/issues/6118
[#7018]: https://github.com/dgraph-io/dgraph/issues/7018
[#6987]: https://github.com/dgraph-io/dgraph/issues/6987
[#6844]: https://github.com/dgraph-io/dgraph/issues/6844
[#6580]: https://github.com/dgraph-io/dgraph/issues/6580
[#6295]: https://github.com/dgraph-io/dgraph/issues/6295
[#6290]: https://github.com/dgraph-io/dgraph/issues/6290
[#6793]: https://github.com/dgraph-io/dgraph/issues/6793

## [20.07.3] - 2020-12-29
[20.07.3]: https://github.com/dgraph-io/dgraph/compare/v20.07.2...v20.07.3

### Changed

- Chore(bulk): Change default compression to zstd:3. ([#6995][])
- Build(dockerfile): Set GODEBUG=madvdontneed=1. ([#6955][])
- Updating badger version 3f846b3. ([#7212][])

### Added:

- Update kubernetes links to match 20.07 config files ([#7049][])
- Fix(dgraph): giving users the option to control tls versions ([#6820][])
- Feat(dgraph): making all internal communications with tls configured ([#6876][])
- Feat(dgraph): enabling TLS config in http zero ([#6691) ([#6867][])

### Fixed:

- GraphQL
  - Fix(GraphQL): don't update cacheMb if not specified by user ([#7103][]) 
  - Fix: added comment docstring for ExportInput format ([#6991][])
  - Fix(GraphQL): fixes issue of multiple responses in a subscription for an update. ([#6868][])
  - Fix ErrIndexingInProgress if schema update fails ([#6583][]) 
  - Fix(GraphQL): fix panic error when we give null value in filter connectives. ([#6707][])
  - Fix(GraphQL): reduces polling duration of subscriptions. ([#6661][])
  - Fix(GraphQL): add enable schema cleaning in GraphQL and reduce schema update time. ([#6725][])
  - Fix(GraphQL): fixes flaky test  for subscriptions. ([#6065][]) 
- Fix(DQL): ignore ordering of indexes in schema with eq function (DGRAPH-2601) ([#6996][])
- Fix(worker): fix eq filter for non-index predicates. ([#6986][]) 
- Fix(Alpha): Immediately take a snapshot if we don't have one ([#6458][])
- Fix(Dgraph): Type names in exported schema are surrounded by brackets. ([#6679][])
- Fix(ludicrous mode): Handle deletes correctly ([#6773][])
- Fix(worker): Avoid panic in handleUidPostings ([#6607][])
- Fix(gqlParser): Handle strings with only whitespace in parseID ([#6615][])
- Fix(Ludicrous): Upserts on list type in Dgraph ([#6796][])
- Enterprise features
  - Fix(backup/restore): fixes backup and restore with DROP operations ([#6922][])
  - Generic alpha log error message for failed ACL login  ([#6848][])

[#6995]: https://github.com/dgraph-io/dgraph/issues/6995
[#6955]: https://github.com/dgraph-io/dgraph/issues/6955
[#7212]: https://github.com/dgraph-io/dgraph/issues/7212
[#7049]: https://github.com/dgraph-io/dgraph/issues/7049
[#6820]: https://github.com/dgraph-io/dgraph/issues/6820
[#6876]: https://github.com/dgraph-io/dgraph/issues/6876
[#6867]: https://github.com/dgraph-io/dgraph/issues/6867
[#7103]: https://github.com/dgraph-io/dgraph/issues/7103
[#6991]: https://github.com/dgraph-io/dgraph/issues/6991
[#6868]: https://github.com/dgraph-io/dgraph/issues/6868
[#6583]: https://github.com/dgraph-io/dgraph/issues/6583
[#6707]: https://github.com/dgraph-io/dgraph/issues/6707
[#6661]: https://github.com/dgraph-io/dgraph/issues/6661
[#6725]: https://github.com/dgraph-io/dgraph/issues/6725
[#6065]: https://github.com/dgraph-io/dgraph/issues/6065
[#6996]: https://github.com/dgraph-io/dgraph/issues/6996
[#6986]: https://github.com/dgraph-io/dgraph/issues/6986
[#6458]: https://github.com/dgraph-io/dgraph/issues/6458
[#6679]: https://github.com/dgraph-io/dgraph/issues/6679
[#6773]: https://github.com/dgraph-io/dgraph/issues/6773
[#6607]: https://github.com/dgraph-io/dgraph/issues/6607
[#6615]: https://github.com/dgraph-io/dgraph/issues/6615
[#6796]: https://github.com/dgraph-io/dgraph/issues/6796
[#6922]: https://github.com/dgraph-io/dgraph/issues/6922
[#6848]: https://github.com/dgraph-io/dgraph/issues/6848


## [20.07.2] - 2020-10-22
[20.07.2]: https://github.com/dgraph-io/dgraph/compare/v20.07.1...v20.07.2

### Changed

- Update badger to 5e3d4b9. ([#6669][])
- Makefile to build Dgraph inside docker container. ([#6601][])
- Return content length header for queries. ([#6480][])
- Use raft storage in managedmode. ([#6547][])
- Update index.md. ([#6567][])
- Changes github.com/dgraph-io/graphql-transport-ws version. ([#6529][])

### Added

- Add utility to export backup data. ([#6590][])
- Add separate compression flag for z and wal dirs. ([#6421][])

### Fixed

- GraphQL
  - Disallowing field names with as. ([#6645][])
  - Remove extra fields when querying interfaces. ([#6647][])
  - fix object Linking with `hasInverse`. ([#6648][])
  - Update gqlgen in go.mod. ([#6646][])
  - Hide info when performing mutation on id field with auth rule. ([#6534][])
  - Fix cascade with auth query when RBAC is false. ([#6535][])
  - Fix squashIntoObject so that results are correctly merged. ([#6530][])
  - Fix errors from authorization examples given in docs. ([#6522][])
  - Fix  restoreStatus query with query variables. ([#6424][])
  - Fix for deletion on interfaces with no non Id field. ([#6417][])
  - Fix internal error when doing GraphQL schema introspection after drop all. ([#6525][])
  - Link xids properly if there are duplicate xids within type. ([#6521][])
  - Fix query rewriting for auth delete when deleting types with inverse field. ([#6524][])
  - Fix order and offset in auth queries. ([#6366][])
  - Generate correct schema when no orderable field in a type. ([#6460][])
  - Don't generate orderable enum value for list fields. ([#6413][])
  - Dix introspection completion bug. ([#6389][])
- Fix Poor-man's auth for admin operations. ([#6686][])
- Break out if g.Ctx is done. ([#6675][])
- Fix wrong path response for k-shortest paths. ([#6654][])
- Update nextRaftId when starting a node with a raftId > 0. ([#6597][])
- Pagination param "after" does not work when using func: uid(v). ([#6634][])
- CID never created if Zero stops early after first init. ([#6637][])
- Pause rollups during snapshot streaming. ([#6611][])
- Use flags for cache. ([#6467][])
- Remove auth error from mutation. ([#6532][])
- Fix readTs less than minTs. ([#6517][])
- Fix bug when deleting and adding to a single UID predicate in the same transaction. ([#6449][])

[#6669]: https://github.com/dgraph-io/dgraph/issues/6669
[#6601]: https://github.com/dgraph-io/dgraph/issues/6601
[#6480]: https://github.com/dgraph-io/dgraph/issues/6480
[#6547]: https://github.com/dgraph-io/dgraph/issues/6547
[#6567]: https://github.com/dgraph-io/dgraph/issues/6567
[#6529]: https://github.com/dgraph-io/dgraph/issues/6529
[#6590]: https://github.com/dgraph-io/dgraph/issues/6590
[#6421]: https://github.com/dgraph-io/dgraph/issues/6421
[#6645]: https://github.com/dgraph-io/dgraph/issues/6645
[#6647]: https://github.com/dgraph-io/dgraph/issues/6647
[#6648]: https://github.com/dgraph-io/dgraph/issues/6648
[#6646]: https://github.com/dgraph-io/dgraph/issues/6646
[#6534]: https://github.com/dgraph-io/dgraph/issues/6534
[#6535]: https://github.com/dgraph-io/dgraph/issues/6535
[#6530]: https://github.com/dgraph-io/dgraph/issues/6530
[#6522]: https://github.com/dgraph-io/dgraph/issues/6522
[#6424]: https://github.com/dgraph-io/dgraph/issues/6424
[#6417]: https://github.com/dgraph-io/dgraph/issues/6417
[#6525]: https://github.com/dgraph-io/dgraph/issues/6525
[#6521]: https://github.com/dgraph-io/dgraph/issues/6521
[#6524]: https://github.com/dgraph-io/dgraph/issues/6524
[#6366]: https://github.com/dgraph-io/dgraph/issues/6366
[#6460]: https://github.com/dgraph-io/dgraph/issues/6460
[#6413]: https://github.com/dgraph-io/dgraph/issues/6413
[#6389]: https://github.com/dgraph-io/dgraph/issues/6389
[#6686]: https://github.com/dgraph-io/dgraph/issues/6686
[#6675]: https://github.com/dgraph-io/dgraph/issues/6675
[#6654]: https://github.com/dgraph-io/dgraph/issues/6654
[#6597]: https://github.com/dgraph-io/dgraph/issues/6597
[#6634]: https://github.com/dgraph-io/dgraph/issues/6634
[#6637]: https://github.com/dgraph-io/dgraph/issues/6637
[#6611]: https://github.com/dgraph-io/dgraph/issues/6611
[#6467]: https://github.com/dgraph-io/dgraph/issues/6467
[#6532]: https://github.com/dgraph-io/dgraph/issues/6532
[#6517]: https://github.com/dgraph-io/dgraph/issues/6517
[#6449]: https://github.com/dgraph-io/dgraph/issues/6449

## [20.07.1] - 2020-09-17
[20.07.1]: https://github.com/dgraph-io/dgraph/compare/v20.07.0...v20.07.1

### Changed

- GraphQL
  - Remove github issues link from the error messages. ([#6183][])
  - Allow case insensitive auth header for graphql subscriptions. ([#6179][])
- Add retry for schema update ([#6098][]) 
- Queue keys for rollup during mutation. ([#6151][]) 

### Added

- GraphQL
  - Adds auth for subscriptions. ([#6165][]) 
- Add --cache_mb and --cache_percentage flags. ([#6286][])
- Add flags to set table and vlog loading mode for zero. ([#6342][])
- Add flag to set up compression in zero. ([#6355][])
  
### Fixed

- GraphQL
  - Multiple queries in a single request should not share the same variables. ([#6158][])
  - Fixes panic in update mutation without set & remove. ([#6160][])
  - Fixes wrong query parameter value for custom field URL. ([#6161][])
  - Fix auth rewriting for nested queries when RBAC rule is true. ([#6167][])
  - Disallow Subscription typename. ([#6173][]) 
  - Panic fix when subscription expiry is not present in jwt. ([#6175][])
  - Fix getType queries when id was used as a name for types other than ID. ([#6180][])
  - Don't reserve certain queries/mutations/inputs when a type is remote. ([#6201][])
  - Linking of xids for deep mutations. ([#6203][])
  - Prevent empty values in fields having `id` directive. ([#6196][]) 
  - Fixes unexpected fragment behaviour. ([#6274][]) 
  - Incorrect generatedSchema in update GQLSchema. ([#6354][])
- Fix out of order issues with split keys in bulk loader. ([#6124][])
- Rollup a batch if more than 2 seconds elapsed since last batch. ([#6137][])
- Refactor: Simplify how list splits are tracked. ([#6070][])
- Fix: Don't allow idx flag to be set to 0 on dgraph zero. ([#6192][]) 
- Fix error message for idx = 0 for dgraph zero. ([#6199][])
- Stop forcing RAM mode for the write-ahead log. ([#6259][])
- Fix panicwrap parent check. ([#6299][]) 
- Sort manifests by BackupNum in file handler. ([#6279][])
- Fixes queries which use variable at the top level. ([#6290][])
- Return error on closed DB. ([#6320][])
- Optimize splits by doing binary search.  Clear the pack from the main list. ([#6332][]) 
- Proto fix needed for PR [#6331][]. ([#6346][]) 
- Sentry nil pointer check. ([#6374][]) 
- Don't store start_ts in postings. ([#6213][]) 
- Use z.Closer instead of y.Closer. ([#6399][]) 
- Make Alpha Shutdown Again. ([#6402][])
- Force exit if CTRL-C is caught before initialization. ([#6407][])
- Update advanced-queries.md.
- Batch list in bulk loader to avoid panic. ([#6446][])
- Enterprise features
  - Make backups cancel other tasks. ([#6243][]) 
  - Online Restore honors credentials passed in. ([#6302][]) 
  - Add a lock to backups to process one request at a time. ([#6339][])
  - Fix Star_All delete query when used with ACL enabled. ([#6336][])

[#6407]: https://github.com/dgraph-io/dgraph/issues/6407
[#6336]: https://github.com/dgraph-io/dgraph/issues/6336
[#6446]: https://github.com/dgraph-io/dgraph/issues/6446
[#6402]: https://github.com/dgraph-io/dgraph/issues/6402
[#6399]: https://github.com/dgraph-io/dgraph/issues/6399
[#6346]: https://github.com/dgraph-io/dgraph/issues/6346
[#6332]: https://github.com/dgraph-io/dgraph/issues/6332
[#6243]: https://github.com/dgraph-io/dgraph/issues/6243
[#6302]: https://github.com/dgraph-io/dgraph/issues/6302
[#6339]: https://github.com/dgraph-io/dgraph/issues/6339
[#6355]: https://github.com/dgraph-io/dgraph/issues/6355
[#6342]: https://github.com/dgraph-io/dgraph/issues/6342
[#6286]: https://github.com/dgraph-io/dgraph/issues/6286
[#6201]: https://github.com/dgraph-io/dgraph/issues/6201
[#6203]: https://github.com/dgraph-io/dgraph/issues/6203
[#6196]: https://github.com/dgraph-io/dgraph/issues/6196
[#6124]: https://github.com/dgraph-io/dgraph/issues/6124
[#6137]: https://github.com/dgraph-io/dgraph/issues/6137
[#6070]: https://github.com/dgraph-io/dgraph/issues/6070
[#6192]: https://github.com/dgraph-io/dgraph/issues/6192
[#6199]: https://github.com/dgraph-io/dgraph/issues/6199
[#6158]: https://github.com/dgraph-io/dgraph/issues/6158
[#6160]: https://github.com/dgraph-io/dgraph/issues/6160
[#6161]: https://github.com/dgraph-io/dgraph/issues/6161
[#6167]: https://github.com/dgraph-io/dgraph/issues/6167
[#6173]: https://github.com/dgraph-io/dgraph/issues/6173
[#6175]: https://github.com/dgraph-io/dgraph/issues/6175
[#6180]: https://github.com/dgraph-io/dgraph/issues/6180
[#6183]: https://github.com/dgraph-io/dgraph/issues/6183
[#6179]: https://github.com/dgraph-io/dgraph/issues/6179
[#6009]: https://github.com/dgraph-io/dgraph/issues/6009
[#6095]: https://github.com/dgraph-io/dgraph/issues/6095
[#6098]: https://github.com/dgraph-io/dgraph/issues/6098
[#6151]: https://github.com/dgraph-io/dgraph/issues/6151
[#6165]: https://github.com/dgraph-io/dgraph/issues/6165
[#6259]: https://github.com/dgraph-io/dgraph/issues/6259
[#6299]: https://github.com/dgraph-io/dgraph/issues/6299
[#6279]: https://github.com/dgraph-io/dgraph/issues/6279
[#6290]: https://github.com/dgraph-io/dgraph/issues/6290
[#6274]: https://github.com/dgraph-io/dgraph/issues/6274
[#6320]: https://github.com/dgraph-io/dgraph/issues/6320
[#6331]: https://github.com/dgraph-io/dgraph/issues/6331
[#6354]: https://github.com/dgraph-io/dgraph/issues/6354
[#6374]: https://github.com/dgraph-io/dgraph/issues/6374
[#6213]: https://github.com/dgraph-io/dgraph/issues/6213

## [20.03.5] - 2020-09-17
[20.03.5]: https://github.com/dgraph-io/dgraph/compare/v20.03.4...v20.03.5

### Changed

- Add retry for schema update. ([#6097][]) 
- Queue keys for rollup during mutation. ([#6150][]) 

### Added

- Add --cache_mb and --cache_percentage flags. ([#6287][])
- Add flag to set up compression in zero. ([#6356][])
- Add flags to set table and vlog loading mode for zero. ([#6343][])

### Fixed

- GraphQL
  - Prevent empty values in fields having `id` directive. ([#6197][]) 
- Fix out of order issues with split keys in bulk loader. ([#6125][])
- Rollup a batch if more than 2 seconds elapsed since last batch. ([#6138][])
- Simplify how list splits are tracked. ([#6071][])
- Perform rollups more aggresively. ([#6147][]) 
- Don't allow idx flag to be set to 0 on dgraph zero. ([#6156][]) 
- Stop forcing RAM mode for the write-ahead log. ([#6260][])
- Fix panicwrap parent check.  ([#6300][]) 
- Sort manifests by backup number. ([#6280][]) 
- Don't store start_ts in postings. ([#6214][])
- Update reverse index when updating single UID predicates. ([#6006][])
- Return error on closed DB.  ([#6321][]) 
- Optimize splits by doing binary search.  Clear the pack from the main list. ([#6333][]) 
- Sentry nil pointer check. ([#6375][])
- Use z.Closer instead of y.Closer. ([#6398][])
- Make Alpha Shutdown Again. ([#6403][])
- Force exit if CTRL-C is caught before initialization. ([#6409][])
- Batch list in bulk loader to avoid panic. ([#6445][])
- Enterprise features
  -  Make backups cancel other tasks. ([#6244][])
  - Add a lock to backups to process one request at a time. ([#6340][])

[#6409]: https://github.com/dgraph-io/dgraph/issues/6409
[#6445]: https://github.com/dgraph-io/dgraph/issues/6445
[#6398]: https://github.com/dgraph-io/dgraph/issues/6398
[#6403]: https://github.com/dgraph-io/dgraph/issues/6403
[#6260]: https://github.com/dgraph-io/dgraph/issues/6260
[#6300]: https://github.com/dgraph-io/dgraph/issues/6300
[#6280]: https://github.com/dgraph-io/dgraph/issues/6280
[#6214]: https://github.com/dgraph-io/dgraph/issues/6214
[#6006]: https://github.com/dgraph-io/dgraph/issues/6006
[#6321]: https://github.com/dgraph-io/dgraph/issues/6321
[#6244]: https://github.com/dgraph-io/dgraph/issues/6244
[#6333]: https://github.com/dgraph-io/dgraph/issues/6333
[#6340]: https://github.com/dgraph-io/dgraph/issues/6340
[#6343]: https://github.com/dgraph-io/dgraph/issues/6343
[#6197]: https://github.com/dgraph-io/dgraph/issues/6197
[#6375]: https://github.com/dgraph-io/dgraph/issues/6375
[#6287]: https://github.com/dgraph-io/dgraph/issues/6287
[#6356]: https://github.com/dgraph-io/dgraph/issues/6356
[#5988]: https://github.com/dgraph-io/dgraph/issues/5988
[#6097]: https://github.com/dgraph-io/dgraph/issues/6097
[#6094]: https://github.com/dgraph-io/dgraph/issues/6094
[#6150]: https://github.com/dgraph-io/dgraph/issues/6150
[#6125]: https://github.com/dgraph-io/dgraph/issues/6125
[#6138]: https://github.com/dgraph-io/dgraph/issues/6138
[#6071]: https://github.com/dgraph-io/dgraph/issues/6071
[#6156]: https://github.com/dgraph-io/dgraph/issues/6156
[#6147]: https://github.com/dgraph-io/dgraph/issues/6147

## [1.2.7] - 2020-09-21
[1.2.7]: https://github.com/dgraph-io/dgraph/compare/v1.2.6...v1.2.7

### Added

- Add --cache_mb and --cache_percentage flags. ([#6288][])
- Add flag to set up compression in zero. ([#6357][])
- Add flags to set table and vlog loading mode for zero. ([#6344][])

### Fixed

- Don't allow idx flag to be set to 0 on dgraph zero. ([#6193][])
- Stop forcing RAM mode for the write-ahead log. ([#6261][])
- Return error on closed DB. ([#6319][]) 
- Don't store start_ts in postings. ([#6212][]) 
- Optimize splits by doing binary search.  Clear the pack from the main list. ([#6334][]) 
- Add a lock to backups to process one request at a time. ([#6341][])
- Use z.Closer instead of y.Closer' ([#6396][])
- Force exit if CTRL-C is caught before initialization. ([#6408][])
- Fix(Alpha): MASA: Make Alpha Shutdown Again. ([#6406][])
- Enterprise features
  - Sort manifests by backup number. ([#6281][]) 
  - Skip backing up nil lists. ([#6314][]) 

[#6408]: https://github.com/dgraph-io/dgraph/issues/6408
[#6406]: https://github.com/dgraph-io/dgraph/issues/6406
[#6396]: https://github.com/dgraph-io/dgraph/issues/6396
[#6261]: https://github.com/dgraph-io/dgraph/issues/6261
[#6319]: https://github.com/dgraph-io/dgraph/issues/6319
[#6212]: https://github.com/dgraph-io/dgraph/issues/6212
[#6334]: https://github.com/dgraph-io/dgraph/issues/6334
[#6341]: https://github.com/dgraph-io/dgraph/issues/6341
[#6281]: https://github.com/dgraph-io/dgraph/issues/6281
[#6314]: https://github.com/dgraph-io/dgraph/issues/6314
[#6288]: https://github.com/dgraph-io/dgraph/issues/6288
[#6357]: https://github.com/dgraph-io/dgraph/issues/6357
[#6344]: https://github.com/dgraph-io/dgraph/issues/6344
[#5987]: https://github.com/dgraph-io/dgraph/issues/5987
[#6193]: https://github.com/dgraph-io/dgraph/issues/6193

## [20.07.0] - 2020-07-28
[20.07.0]: https://github.com/dgraph-io/dgraph/compare/v20.03.4...v20.07.0

### Changed

- GraphQL
  - Make updateGQLSchema always return the new schema. ([#5540][])
  - Allow user to define and pass arguments to fields. ([#5562][])	
  - Move alias to end of graphql pipeline. ([#5369][])	
- Return error list while validating GraphQL schema. ([#5576][])
- Send CID for sentry events. ([#5625][])
- Alpha: Enable bloom filter caching ([#5552][])
- Add support for multiple uids in uid_in function ([#5292][])
- Tag sentry events with additional version details. ([#5726][])
- Sentry opt out banner. ([#5727][]) 
- Replace shutdownCh and wait groups to a y.Closer for shutting down Alpha. ([#5560][])
- Update badger to commit [e7b6e76f96e8][]. ([#5537][])
- Update Badger ([#5661][], [#6034][])
  - Fix assert in background compression and encryption. ([dgraph-io/badger#1366][])
  - GC: Consider size of value while rewriting ([dgraph-io/badger#1357][])
  - Restore: Account for value size as well ([dgraph-io/badger#1358][])
  - Tests: Do not leave behind state goroutines ([dgraph-io/badger#1349][])
  - Support disabling conflict detection ([dgraph-io/badger#1344][])
  - Compaction: Expired keys and delete markers are never purged ([dgraph-io/badger#1354][])
  - Fix build on golang tip ([dgraph-io/badger#1355][])
  - StreamWriter: Close head writer ([dgraph-io/badger#1347][])
  - Iterator: Always add key to txn.reads ([dgraph-io/badger#1328][])
  - Add immudb to the project list ([dgraph-io/badger#1341][])
  - DefaultOptions: Set KeepL0InMemory to false ([dgraph-io/badger#1345][])
- Enterprise features
  - /health endpoint now shows Enterprise Features available. Fixes [#5234][]. ([#5293][])
  - GraphQL Changes for /health endpoint's Enterprise features info. Fixes [#5234][]. ([#5308][])
  - Use encryption in temp badger, fix compilation on 32-bit. ([#4963][])
  - Only process restore request in the current alpha if it's the leader. ([#5657][])
  - Vault: Support kv v1 and decode base64 key. ([#5725][])
  - **Breaking changes**
    - [BREAKING] GraphQL: Add camelCase for add/update mutation. Fixes [#5380][]. ([#5547][])

### Added

- GraphQL
  - Add Graphql-TouchedUids header in HTTP response. ([#5572][])
  - Introduce `@cascade` in GraphQL. Fixes [#4789][]. ([#5511][])	
  - Add authentication feature and http admin endpoints. Fixes [#4758][]. ([#5162][])	
  - Support existing gqlschema nodes without xid. ([#5457][])
  - Add custom logic feature. ([#5004][])
  - Add extensions to query response. ([#5157][])
  - Allow query of deleted nodes. ([#5949][])
  - Allow more control over custom logic header names. ([#5809][])
  - Adds Apollo tracing to GraphQL extensions. ([#5855][])
  - Turn on subscriptions and adds directive to control subscription generation. ([#5856][])
  - Add introspection headers to custom logic. ([#5858][]) 
  - GraphQL health now reported by /probe/graphql. ([#5875][])
  - Validate audience in authorization JWT and change `Dgraph.Authorization` format. ([#5980][])
- Upgrade tool for 20.07. ([#5830][]) 
- Async restore operations. ([#5704][])
- Add LogRequest variable to GraphQL config input. ([#5197][])
- Allow backup ID to be passed to restore endpoint. ([#5208][])
- Added support for application/graphQL to graphQL endpoints. ([#5125][])
- Add support for xidmap in bulkloader. Fixes [#4917][]. ([#5090][])
- Add GraphQL admin endpoint to list backups. ([#5307][]) 
- Enterprise features
  - GraphQL schema get/update, Dgraph schema query/alter and /login are now admin operations. ([#5833][])
  - Backup can take S3 credentials from IAM. ([#5387][])
  - Online restore. ([#5095][]) 
  - Retry restore proposals. ([#5765][])
  - Add support for encrypted backups in online restores. ([#5226][])
  - **Breaking changes**
    - [BREAKING] Vault Integration. ([#5402][])
 
### Fixed

- GraphQL
  - Validate JWT Claims and test JWT expiry. ([#6050][])
  - Validate subscriptions in Operation function. ([#5983][]) 	
  - Nested auth queries no longer search through all possible records. ([#5950][])	
  - Apply auth rules on type having @dgraph directive. ([#5863][])	
  - Custom Claim will be parsed as JSON if it is encoded as a string. ([#5862][])	
  - Dgraph directive with reverse edge should work smoothly with interfaces. Fixed [#5744][]. ([#5982][])
  - Fix case where Dgraph type was not generated for GraphQL interface. Fixes [#5311][]. ([#5828][])
  - Fix panic error when there is no @withSubscription directive on any type. ([#5921][])	
  - Fix OOM issue in graphql mutation rewriting. ([#5854][])	
  - Preserve GraphQL schema after drop_data. ([#5840][])	
  - Maintain Master's backward compatibility for `Dgraph.Authorization` in schema. ([#6014][])	
  - Remote schema introspection for single remote endpoint. ([#5824][])	
  - Requesting only \_\-typename now returns results. ([#5823][])	
  - Typename for types should be filled in query for schema introspection queries. Fixes [#5792][]. ([#5891][])
  - Update GraphQL schema only on Group-1 leader. ([#5829][])	
  - Add more validations for coercion of object/scalar and vice versa. ([#5534][])
  - Apply type filter for get query at root level. ([#5497][])
  - Fix mutation on predicate with special characters having dgraph directive. Fixes [#5296][]. ([#5526][])	
  - Return better error message if a type only contains ID field. ([#5531][])
  - Coerce value for scalar types correctly. ([#5487][])
  - Minor delete mutation msg fix. ([#5316][])
  - Report all errors during schema update. ([#5425][])
  - Do graphql query/mutation validation in the mock server. ([#5362][])
  - Remove custom directive from internal schema. ([#5354][])
  - Recover from panic within goroutines used for resolving custom fields. ([#5329][])
  - Start collecting and returning errors from remote remote GraphQL endpoints. ([#5328][])
  - Fix response for partial admin queries. ([#5317][])	
- Avoid assigning duplicate RAFT IDs to new nodes. Fixes [#5436][]. ([#5571][])
- Alpha: Gracefully shutdown ludicrous mode. ([#5561][])	
- Use rampMeter for Executor. ([#5503][])
- Dont set n.ops map entries to nil. Instead just delete them. ([#5551][])
- Add check on rebalance interval. ([#5544][])
- Queries or mutations shouldn't be part of generated Dgraph schema. ([#5524][])
- Sent restore proposals to all groups asyncronouosly. ([#5467][])
- Fix long lines in export.go. ([#5498][])
- Fix warnings about unkeyed literals. ([#5492][])
- Remove redundant conversions between string and []byte. ([#5478][])
- Propogate request context while handling queries. ([#5418][])
- K-Shortest path query fix. Fixes [#5426][]. ([#5410][])			
- Worker: Return nil on error. ([#5414][])
- Fix warning about issues with the cancel function. ([#5397][]). 
- Replace TxnWriter with WriteBatch. ([#5007][])
- Add a check to throw an error is a nil pointer is passed to unmarshalOrCopy. ([#5334][])
- Remove noisy logs in tablet move. ([#5333][])
- Support bulk loader use-case to import unencrypted export and encrypt the result.  ([#5209][])
- Handle Dgraph shutdown gracefully. Fixes [#3873][]. ([#5137][], [#5138][])
- If we don't have any schema updates, avoid running the indexing sequence. ([#5126][])
- Pass read timestamp to getNew. ([#5085][])
- Indicate dev environment in Sentry events. ([#5051][])	
- Replaced s2 contains point methods with go-geom. ([#5023][]
- Change tablet size calculation to not depend on the right key. Fixes [#5408][]. ([#5684][])	
- Fix alpha start in ludicrous mode. Fixes [#5601][]. ([#5912][])
- Handle schema updates correctly in ludicrous mode. ([#5970][])	
- Fix Panic because of nil map in groups.go. ([#6008][]) 	
- update reverse index when updating single UID predicates. Fixes [#5732][]. ([#6005][]), ([#6015][])	
- Fix expand(\_all\_) queries in ACL. Fixes [#5687][]. ([#5993][]) 	
- Fix val queries when ACL is enabled. Fixes [#5687][]. ([#5995][])
- Return error if server is not ready. ([#6020][]) 	
- Reduce memory consumption of the map. ([#5957][]) 	
- Cancel the context when opening connection to leader for streaming snapshot. ([#6045][])
- **Breaking changes**
  - [BREAKING] Namespace dgraph internal types/predicates with `dgraph.` Fixes [#4878][]. ([#5185][])
  - [BREAKING] Remove shorthand for store_xids in bulk loader.  ([#5148][])
  - [BREAKING] Introduce new facets format. Fixes [#4798][], [#4581][], [#4907][]. ([#5424][])
- Enterprise:
  - Backup: Change groupId from int to uint32. ([#5605][])
  - Backup: Use a sync.Pool to allocate KVs during backup. ([#5579][])
  - Backup: Fix segmentation fault when calling the /admin/backup edpoint. ([#6043][])
  - Restore: Make backupId optional in restore GraphQL interface. ([#5685][])
  - Restore: Move tablets to right group when restoring a backup. ([#5682][])
  - Restore: Only processes backups for the alpha's group. ([#5588][])
  - vault_format support for online restore and gql ([#5758][])	

[#5661]: https://github.com/dgraph-io/dgraph/issues/5661
[dgraph-io/badger#1366]: https://github.com/dgraph-io/badger/issues/1366
[dgraph-io/badger#1357]: https://github.com/dgraph-io/badger/issues/1357
[dgraph-io/badger#1358]: https://github.com/dgraph-io/badger/issues/1358
[dgraph-io/badger#1349]: https://github.com/dgraph-io/badger/issues/1349
[dgraph-io/badger#1344]: https://github.com/dgraph-io/badger/issues/1344
[dgraph-io/badger#1354]: https://github.com/dgraph-io/badger/issues/1354
[dgraph-io/badger#1355]: https://github.com/dgraph-io/badger/issues/1355
[dgraph-io/badger#1347]: https://github.com/dgraph-io/badger/issues/1347
[dgraph-io/badger#1328]: https://github.com/dgraph-io/badger/issues/1328
[dgraph-io/badger#1341]: https://github.com/dgraph-io/badger/issues/1341
[dgraph-io/badger#1345]: https://github.com/dgraph-io/badger/issues/1345
[#6050]: https://github.com/dgraph-io/dgraph/issues/6050
[#6045]: https://github.com/dgraph-io/dgraph/issues/6045
[#5725]: https://github.com/dgraph-io/dgraph/issues/5725
[#5579]: https://github.com/dgraph-io/dgraph/issues/5579
[#5685]: https://github.com/dgraph-io/dgraph/issues/5685
[#5682]: https://github.com/dgraph-io/dgraph/issues/5682
[#5572]: https://github.com/dgraph-io/dgraph/issues/5572
[#4789]: https://github.com/dgraph-io/dgraph/issues/4789
[#5511]: https://github.com/dgraph-io/dgraph/issues/5511
[#4758]: https://github.com/dgraph-io/dgraph/issues/4758
[#5162]: https://github.com/dgraph-io/dgraph/issues/5162
[#5457]: https://github.com/dgraph-io/dgraph/issues/5457
[#5004]: https://github.com/dgraph-io/dgraph/issues/5004
[#5134]: https://github.com/dgraph-io/dgraph/issues/5134
[#5157]: https://github.com/dgraph-io/dgraph/issues/5157
[#5197]: https://github.com/dgraph-io/dgraph/issues/5197
[#5387]: https://github.com/dgraph-io/dgraph/issues/5387
[#5226]: https://github.com/dgraph-io/dgraph/issues/5226
[#5208]: https://github.com/dgraph-io/dgraph/issues/5208
[#5125]: https://github.com/dgraph-io/dgraph/issues/5125
[#5095]: https://github.com/dgraph-io/dgraph/issues/5095
[#4917]: https://github.com/dgraph-io/dgraph/issues/4917
[#5090]: https://github.com/dgraph-io/dgraph/issues/5090
[#5307]: https://github.com/dgraph-io/dgraph/issues/5307
[#5402]: https://github.com/dgraph-io/dgraph/issues/5402
[#5540]: https://github.com/dgraph-io/dgraph/issues/5540
[#5576]: https://github.com/dgraph-io/dgraph/issues/5576
[#5625]: https://github.com/dgraph-io/dgraph/issues/5625
[#5562]: https://github.com/dgraph-io/dgraph/issues/5562
[#5552]: https://github.com/dgraph-io/dgraph/issues/5552
[#5369]: https://github.com/dgraph-io/dgraph/issues/5369
[#5292]: https://github.com/dgraph-io/dgraph/issues/5292
[#5234]: https://github.com/dgraph-io/dgraph/issues/5234
[#5293]: https://github.com/dgraph-io/dgraph/issues/5293
[#5234]: https://github.com/dgraph-io/dgraph/issues/5234
[#5308]: https://github.com/dgraph-io/dgraph/issues/5308
[#4963]: https://github.com/dgraph-io/dgraph/issues/4963
[#5380]: https://github.com/dgraph-io/dgraph/issues/5380
[#5547]: https://github.com/dgraph-io/dgraph/issues/5547
[#5534]: https://github.com/dgraph-io/dgraph/issues/5534
[#5497]: https://github.com/dgraph-io/dgraph/issues/5497
[#5296]: https://github.com/dgraph-io/dgraph/issues/5296
[#5526]: https://github.com/dgraph-io/dgraph/issues/5526
[#5531]: https://github.com/dgraph-io/dgraph/issues/5531
[#5487]: https://github.com/dgraph-io/dgraph/issues/5487
[#5316]: https://github.com/dgraph-io/dgraph/issues/5316
[#5425]: https://github.com/dgraph-io/dgraph/issues/5425
[#5362]: https://github.com/dgraph-io/dgraph/issues/5362
[#5354]: https://github.com/dgraph-io/dgraph/issues/5354
[#5329]: https://github.com/dgraph-io/dgraph/issues/5329
[#5328]: https://github.com/dgraph-io/dgraph/issues/5328
[#5317]: https://github.com/dgraph-io/dgraph/issues/5317
[#5588]: https://github.com/dgraph-io/dgraph/issues/5588
[#5605]: https://github.com/dgraph-io/dgraph/issues/5605
[#5571]: https://github.com/dgraph-io/dgraph/issues/5571
[#5561]: https://github.com/dgraph-io/dgraph/issues/5561
[#5503]: https://github.com/dgraph-io/dgraph/issues/5503
[#5551]: https://github.com/dgraph-io/dgraph/issues/5551
[#5544]: https://github.com/dgraph-io/dgraph/issues/5544
[#5524]: https://github.com/dgraph-io/dgraph/issues/5524
[#5467]: https://github.com/dgraph-io/dgraph/issues/5467
[#5498]: https://github.com/dgraph-io/dgraph/issues/5498
[#5492]: https://github.com/dgraph-io/dgraph/issues/5492
[#5478]: https://github.com/dgraph-io/dgraph/issues/5478
[#5418]: https://github.com/dgraph-io/dgraph/issues/5418
[#5426]: https://github.com/dgraph-io/dgraph/issues/5426
[#5410]: https://github.com/dgraph-io/dgraph/issues/5410
[#5414]: https://github.com/dgraph-io/dgraph/issues/5414
[#5397]: https://github.com/dgraph-io/dgraph/issues/5397
[#5007]: https://github.com/dgraph-io/dgraph/issues/5007
[#5334]: https://github.com/dgraph-io/dgraph/issues/5334
[#5333]: https://github.com/dgraph-io/dgraph/issues/5333
[#5209]: https://github.com/dgraph-io/dgraph/issues/5209
[#3873]: https://github.com/dgraph-io/dgraph/issues/3873
[#5138]: https://github.com/dgraph-io/dgraph/issues/5138
[#3873]: https://github.com/dgraph-io/dgraph/issues/3873
[#5137]: https://github.com/dgraph-io/dgraph/issues/5137
[#5126]: https://github.com/dgraph-io/dgraph/issues/5126
[#5085]: https://github.com/dgraph-io/dgraph/issues/5085
[#5051]: https://github.com/dgraph-io/dgraph/issues/5051
[#5023]: https://github.com/dgraph-io/dgraph/issues/5023
[#4878]: https://github.com/dgraph-io/dgraph/issues/4878
[#5185]: https://github.com/dgraph-io/dgraph/issues/5185
[#5148]: https://github.com/dgraph-io/dgraph/issues/5148
[#4798]: https://github.com/dgraph-io/dgraph/issues/4798
[#4581]: https://github.com/dgraph-io/dgraph/issues/4581
[#4907]: https://github.com/dgraph-io/dgraph/issues/4907
[#5424]: https://github.com/dgraph-io/dgraph/issues/5424
[#5436]: https://github.com/dgraph-io/dgraph/issues/5436
[#5537]: https://github.com/dgraph-io/dgraph/issues/5537
[#5657]: https://github.com/dgraph-io/dgraph/issues/5657
[#5726]: https://github.com/dgraph-io/dgraph/issues/5726
[#5727]: https://github.com/dgraph-io/dgraph/issues/5727
[#5408]: https://github.com/dgraph-io/dgraph/issues/5408
[#5684]: https://github.com/dgraph-io/dgraph/issues/5684
[e7b6e76f96e8]: https://github.com/dgraph-io/badger/commit/e7b6e76f96e8
[#5949]: https://github.com/dgraph-io/dgraph/issues/5949
[#5704]: https://github.com/dgraph-io/dgraph/issues/5704
[#5765]: https://github.com/dgraph-io/dgraph/issues/5765
[#5809]: https://github.com/dgraph-io/dgraph/issues/5809
[#5830]: https://github.com/dgraph-io/dgraph/issues/5830
[#5855]: https://github.com/dgraph-io/dgraph/issues/5855
[#5856]: https://github.com/dgraph-io/dgraph/issues/5856
[#5858]: https://github.com/dgraph-io/dgraph/issues/5858
[#5833]: https://github.com/dgraph-io/dgraph/issues/5833
[#5875]: https://github.com/dgraph-io/dgraph/issues/5875
[#5980]: https://github.com/dgraph-io/dgraph/issues/5980
[#5560]: https://github.com/dgraph-io/dgraph/issues/5560
[#5912]: https://github.com/dgraph-io/dgraph/issues/5912
[#5601]: https://github.com/dgraph-io/dgraph/issues/5601
[#5970]: https://github.com/dgraph-io/dgraph/issues/5970
[#6008]: https://github.com/dgraph-io/dgraph/issues/6008
[#6005]: https://github.com/dgraph-io/dgraph/issues/6005
[#6015]: https://github.com/dgraph-io/dgraph/issues/6015
[#5732]: https://github.com/dgraph-io/dgraph/issues/5732
[#5863]: https://github.com/dgraph-io/dgraph/issues/5863
[#5862]: https://github.com/dgraph-io/dgraph/issues/5862
[#5982]: https://github.com/dgraph-io/dgraph/issues/5982
[#5744]: https://github.com/dgraph-io/dgraph/issues/5744
[#5828]: https://github.com/dgraph-io/dgraph/issues/5828
[#5311]: https://github.com/dgraph-io/dgraph/issues/5311
[#5921]: https://github.com/dgraph-io/dgraph/issues/5921
[#5854]: https://github.com/dgraph-io/dgraph/issues/5854
[#5840]: https://github.com/dgraph-io/dgraph/issues/5840
[#5758]: https://github.com/dgraph-io/dgraph/issues/5758
[#5983]: https://github.com/dgraph-io/dgraph/issues/5983
[#5957]: https://github.com/dgraph-io/dgraph/issues/5957
[#6014]: https://github.com/dgraph-io/dgraph/issues/6014
[#5824]: https://github.com/dgraph-io/dgraph/issues/5824
[#5823]: https://github.com/dgraph-io/dgraph/issues/5823
[#5891]: https://github.com/dgraph-io/dgraph/issues/5891
[#5792]: https://github.com/dgraph-io/dgraph/issues/5792
[#5829]: https://github.com/dgraph-io/dgraph/issues/5829
[#5993]: https://github.com/dgraph-io/dgraph/issues/5993
[#5687]: https://github.com/dgraph-io/dgraph/issues/5687
[#5995]: https://github.com/dgraph-io/dgraph/issues/5995
[#5687]: https://github.com/dgraph-io/dgraph/issues/5687
[#6020]: https://github.com/dgraph-io/dgraph/issues/6020
[#5950]: https://github.com/dgraph-io/dgraph/issues/5950
[#5809]: https://github.com/dgraph-io/dgraph/issues/5809
[#6034]: https://github.com/dgraph-io/dgraph/issues/6034
[#6043]: https://github.com/dgraph-io/dgraph/issues/6043

## [20.03.4] - 2020-07-23
[20.03.4]: https://github.com/dgraph-io/dgraph/compare/v20.03.3...v20.03.4

### Changed
- Update Badger 07/13/2020. ([#5941][], [#5616][])

### Added
- Sentry opt out banner. ([#5729][]) 
- Tag sentry events with additional version details. ([#5728][])

### Fixed
- GraphQL
  - Minor delete mutation msg fix. ([#5564][])
  - Make updateGQLSchema always return the new schema. ([#5582][])
  - Fix mutation on predicate with special characters in the `@dgraph` directive. ([#5577][])
  - Updated mutation rewriting to fix OOM issue. ([#5536][])
  - Fix case where Dgraph type was not generated for GraphQL interface. Fixes [#5311][]. ([#5844][])
  - Fix interface conversion panic in v20.03 ([#5857][]) .
- Dont set n.ops map entries to nil. Instead just delete them. ([#5557][])
- Alpha: Enable bloom filter caching. ([#5555][])
- Alpha: Gracefully shutdown ludicrous mode. ([#5584][])
- Alpha Close: Wait for indexing to complete. Fixes [#3873][]. ([#5597][])
- K shortest paths queries fix. ([#5548][])
- Add check on rebalance interval. ([#5594][])
- Remove noisy logs in tablet move. ([#5591][])
- Avoid assigning duplicate RAFT IDs to new nodes. Fixes [#4536][]. ([#5604][])
- Send CID for sentry events. ([#5633][])
- Use rampMeter for Executor. ([#5503][])
- Fix snapshot calculation in ludicrous mode. ([#5636][])
- Update badger: Avoid panic in fillTables(). Fix assert in background compression and encryption. ([#5680][])
- Avoid panic in handleValuePostings. ([#5678][])
- Fix facets response with normalize. Fixes [#5241][]. ([#5691][])
- Badger iterator key copy in count index query. ([#5916][])
- Ludicrous mode mutation error. ([#5914][]) 
- Return error instead of panic. ([#5907][])
- Fix segmentation fault in draft.go. ([#5860][])
- Optimize count index. ([#5971][])
- Handle schema updates correctly in ludicrous mode. ([#5969][])
- Fix Panic because of nil map in groups.go. ([#6007][])
- Return error if server is not ready. ([#6021][])
- Enterprise features
  - Backup: Change groupId from int to uint32. ([#5614][])
  - Backup: Use a sync.Pool to allocate KVs. ([#5579][])

[#5241]: https://github.com/dgraph-io/dgraph/issues/5241
[#5691]: https://github.com/dgraph-io/dgraph/issues/5691
[#5916]: https://github.com/dgraph-io/dgraph/issues/5916
[#5914]: https://github.com/dgraph-io/dgraph/issues/5914
[#5907]: https://github.com/dgraph-io/dgraph/issues/5907
[#5860]: https://github.com/dgraph-io/dgraph/issues/5860
[#5971]: https://github.com/dgraph-io/dgraph/issues/5971
[#5311]: https://github.com/dgraph-io/dgraph/issues/5311
[#5844]: https://github.com/dgraph-io/dgraph/issues/5844
[#5857]: https://github.com/dgraph-io/dgraph/issues/5857
[#5941]: https://github.com/dgraph-io/dgraph/issues/5941
[#5729]: https://github.com/dgraph-io/dgraph/issues/5729
[#5728]: https://github.com/dgraph-io/dgraph/issues/5728
[#5616]: https://github.com/dgraph-io/dgraph/issues/5616
[#5564]: https://github.com/dgraph-io/dgraph/issues/5564
[#5582]: https://github.com/dgraph-io/dgraph/issues/5582
[#5577]: https://github.com/dgraph-io/dgraph/issues/5577
[#5536]: https://github.com/dgraph-io/dgraph/issues/5536
[#5557]: https://github.com/dgraph-io/dgraph/issues/5557
[#5555]: https://github.com/dgraph-io/dgraph/issues/5555
[#5584]: https://github.com/dgraph-io/dgraph/issues/5584
[#3873]: https://github.com/dgraph-io/dgraph/issues/3873
[#5597]: https://github.com/dgraph-io/dgraph/issues/5597
[#5548]: https://github.com/dgraph-io/dgraph/issues/5548
[#5594]: https://github.com/dgraph-io/dgraph/issues/5594
[#5591]: https://github.com/dgraph-io/dgraph/issues/5591
[#4536]: https://github.com/dgraph-io/dgraph/issues/4536
[#5604]: https://github.com/dgraph-io/dgraph/issues/5604
[#5633]: https://github.com/dgraph-io/dgraph/issues/5633
[#5503]: https://github.com/dgraph-io/dgraph/issues/5503
[#5636]: https://github.com/dgraph-io/dgraph/issues/5636
[#5680]: https://github.com/dgraph-io/dgraph/issues/5680
[#5614]: https://github.com/dgraph-io/dgraph/issues/5614
[#5579]: https://github.com/dgraph-io/dgraph/issues/5579
[#5678]: https://github.com/dgraph-io/dgraph/issues/5678
[#5969]: https://github.com/dgraph-io/dgraph/issues/5969
[#6007]: https://github.com/dgraph-io/dgraph/issues/6007
[#6021]: https://github.com/dgraph-io/dgraph/issues/6021

## [1.2.6] - 2020-07-31
[1.2.6]: https://github.com/dgraph-io/dgraph/compare/v1.2.5...v1.2.6

### Changed

- Update Badger. ([#5940][], [#5990][])
  - Fix assert in background compression and encryption. (dgraph-io/badger#1366)
  - Avoid panic in filltables() (dgraph-io/badger#1365)
  - Force KeepL0InMemory to be true when InMemory is true (dgraph-io/badger#1375)
  - Tests: Use t.Parallel in TestIteratePrefix tests (dgraph-io/badger#1377)
  - Remove second initialization of writech in Open (dgraph-io/badger#1382)
  - Increase default valueThreshold from 32B to 1KB (dgraph-io/badger#1346)
  - Pre allocate cache key for the block cache and the bloom filter cache (dgraph-io/badger#1371)
  - Rework DB.DropPrefix (dgraph-io/badger#1381)
  - Update head while replaying value log (dgraph-io/badger#1372)
  - Update ristretto to commit f66de99 (dgraph-io/badger#1391)
  - Enable cross-compiled 32bit tests on TravisCI (dgraph-io/badger#1392)
  - Avoid panic on multiple closer.Signal calls (dgraph-io/badger#1401)
  - Add a contribution guide (dgraph-io/badger#1379)
  - Add assert to check integer overflow for table size (dgraph-io/badger#1402)
  - Return error if the vlog writes exceeds more that 4GB. (dgraph-io/badger#1400)
  - Revert "add assert to check integer overflow for table size (dgraph-io/badger#1402)" (dgraph-io/badger#1406)
  - Revert "fix: Fix race condition in block.incRef (dgraph-io/badger#1337)" (dgraph-io/badger#1407)
  - Revert "Buffer pool for decompression (dgraph-io/badger#1308)" (dgraph-io/badger#1408)
  - Revert "Compress/Encrypt Blocks in the background (dgraph-io/badger#1227)" (dgraph-io/badger#1409)
  - Add missing changelog for v2.0.3 (dgraph-io/badger#1410)
  - Changelog for v20.07.0 (dgraph-io/badger#1411)

### Fixed

- Alpha: Enable bloom filter caching. ([#5554][]) 	
- K shortest paths queries fix. ([#5596][])	
- Add check on rebalance interval. ([#5595][]) 	
- Change error message in case of successful license application. ([#5593][])	
- Remove noisy logs in tablet move. ([#5592][]) 	
- Avoid assigning duplicate RAFT IDs to new nodes. Fixes [#5436][]. ([#5603][]) 	
- Update badger: Set KeepL0InMemory to false (badger default), and Set DetectConflicts to false. ([#5615][])
- Use /tmp dir to store temporary index. Fixes [#4600][]. ([#5730][])
- Split posting lists recursively. ([#4867][]) 
- Set version when rollup is called with no splits. ([#4945][])	
- Return error instead of panic (readPostingList). Fixes [#5749][]. ([#5908][]) 	
- ServeTask: Return error if server is not ready. ([#6022][])	
- Enterprise features
  - Backup: Change groupId from int to uint32. ([#5613][])
  - Backup: During backup, collapse split posting lists into a single list. ([#4682][])
  - Backup: Use a sync.Pool to allocate KVs during backup. ([#5579][])

[#5730]: https://github.com/dgraph-io/dgraph/issues/5730
[#4600]: https://github.com/dgraph-io/dgraph/issues/4600
[#4682]: https://github.com/dgraph-io/dgraph/issues/4682
[#4867]: https://github.com/dgraph-io/dgraph/issues/4867
[#5579]: https://github.com/dgraph-io/dgraph/issues/5579
[#4945]: https://github.com/dgraph-io/dgraph/issues/4945
[#5908]: https://github.com/dgraph-io/dgraph/issues/5908
[#5749]: https://github.com/dgraph-io/dgraph/issues/5749
[#6022]: https://github.com/dgraph-io/dgraph/issues/6022
[#5554]: https://github.com/dgraph-io/dgraph/issues/5554
[#5596]: https://github.com/dgraph-io/dgraph/issues/5596
[#5595]: https://github.com/dgraph-io/dgraph/issues/5595
[#5593]: https://github.com/dgraph-io/dgraph/issues/5593
[#5592]: https://github.com/dgraph-io/dgraph/issues/5592
[#5436]: https://github.com/dgraph-io/dgraph/issues/5436
[#5603]: https://github.com/dgraph-io/dgraph/issues/5603
[#5615]: https://github.com/dgraph-io/dgraph/issues/5615
[#5613]: https://github.com/dgraph-io/dgraph/issues/5613
[#5940]: https://github.com/dgraph-io/dgraph/issues/5940
[#5990]: https://github.com/dgraph-io/dgraph/issues/5613

## [20.03.3] - 2020-06-02
[20.03.3]: https://github.com/dgraph-io/dgraph/compare/v20.03.1...v20.03.3

### Changed

- Sentry Improvements: Segregate dev and prod events into their own Sentry projects. Remove Panic back-traces, Set the type of exception to the panic message. ([#5305][])
- /health endpoint now shows EE Features available and GraphQL changes. ([#5304][]) 
- Return error response if encoded response is > 4GB in size. Replace idMap with idSlice in encoder. ([#5359][]) 
- Initialize sentry at the beginning of alpha.Run(). ([#5429][])  

### Added
- Adds ludicrous mode to live loader. ([#5419][]) 
- GraphQL: adds transactions to graphql mutations ([#5485][])

### Fixed

- Export: Ignore deleted predicates from schema. Fixes [#5053][]. ([#5326][])
- GraphQL: ensure upserts don't have accidental edge removal. Fixes [#5355][]. ([#5356][])
- Fix segmentation fault in query.go. ([#5377][])
- Fix empty string checks. ([#5390][]) 
- Update group checksums when combining multiple deltas. Fixes [#5368][]. ([#5394][]) 
- Change the default ratio of traces from 1 to 0.01. ([#5405][]) 
- Fix protobuf headers check. ([#5381][])
- Stream the full set of predicates and types during a snapshot. ([#5444][])
- Support passing GraphQL schema to bulk loader. Fixes [#5235][]. ([#5521][]) 
- Export GraphQL schema to separate file. Fixes [#5235][]. ([#5528][]) 
- Fix memory leak in live loader. ([#5473][]) 
- Replace strings.Trim with strings.TrimFunc in ParseRDF. ([#5494][]) 
- Return nil instead of emptyTablet in groupi.Tablet(). ([#5469][]) 
- Use pre-allocated protobufs during backups. ([#5404][]) 
- During shutdown, generate snapshot before closing raft node. ([#5476][])
- Get lists of predicates and types before sending the snapshot. ([#5488][]) 
- Fix panic for sending on a closed channel. ([#5479][])
- Fix inconsistent bulk loader failures. Fixes [#5361][]. ([#5537][])
- GraphQL: fix password rewriting. ([#5483][]) 
- GraphQL: Fix non-unique schema issue. ([#5481][]) 
- Enterprise features
  - Print error when applying enterprise license fails. ([#5342][])
  - Apply the option enterprise_license only after the node's Raft is initialized and it is the leader. Don't apply the     trial license if a license already exists. Disallow the enterprise_license option for OSS build and bail out. Apply the option even if there is a license from a previous life of the Zero. ([#5384][])

### Security

- Use SensitiveByteSlice type for hmac secret. ([#5450][])


[#5444]: https://github.com/dgraph-io/dgraph/issues/5444
[#5305]: https://github.com/dgraph-io/dgraph/issues/5305
[#5304]: https://github.com/dgraph-io/dgraph/issues/5304
[#5359]: https://github.com/dgraph-io/dgraph/issues/5359
[#5429]: https://github.com/dgraph-io/dgraph/issues/5429
[#5342]: https://github.com/dgraph-io/dgraph/issues/5342
[#5326]: https://github.com/dgraph-io/dgraph/issues/5326
[#5356]: https://github.com/dgraph-io/dgraph/issues/5356
[#5377]: https://github.com/dgraph-io/dgraph/issues/5377
[#5384]: https://github.com/dgraph-io/dgraph/issues/5384
[#5390]: https://github.com/dgraph-io/dgraph/issues/5390
[#5394]: https://github.com/dgraph-io/dgraph/issues/5394
[#5405]: https://github.com/dgraph-io/dgraph/issues/5405
[#5053]: https://github.com/dgraph-io/dgraph/issues/5053
[#5355]: https://github.com/dgraph-io/dgraph/issues/5355
[#5368]: https://github.com/dgraph-io/dgraph/issues/5368
[#5450]: https://github.com/dgraph-io/dgraph/issues/5450
[#5381]: https://github.com/dgraph-io/dgraph/issues/5381
[#5528]: https://github.com/dgraph-io/dgraph/issues/5528
[#5473]: https://github.com/dgraph-io/dgraph/issues/5473
[#5494]: https://github.com/dgraph-io/dgraph/issues/5494
[#5469]: https://github.com/dgraph-io/dgraph/issues/5469
[#5404]: https://github.com/dgraph-io/dgraph/issues/5404
[#5476]: https://github.com/dgraph-io/dgraph/issues/5476
[#5488]: https://github.com/dgraph-io/dgraph/issues/5488
[#5483]: https://github.com/dgraph-io/dgraph/issues/5483
[#5481]: https://github.com/dgraph-io/dgraph/issues/5481
[#5481]: https://github.com/dgraph-io/dgraph/issues/5481
[#5235]: https://github.com/dgraph-io/dgraph/issues/5235
[#5419]: https://github.com/dgraph-io/dgraph/issues/5419
[#5485]: https://github.com/dgraph-io/dgraph/issues/5485
[#5479]: https://github.com/dgraph-io/dgraph/issues/5479
[#5361]: https://github.com/dgraph-io/dgraph/issues/5361
[#5537]: https://github.com/dgraph-io/dgraph/issues/5537

## [1.2.5] - 2020-06-02
[1.2.5]: https://github.com/dgraph-io/dgraph/compare/v1.2.3...v1.2.5

### Changed

- Return error response if encoded response is > 4GB in size. Replace idMap with idSlice in encoder. ([#5359][])
- Change the default ratio of traces from 1 to 0.01. ([#5405][]) 

### Fixed

- Export: Ignore deleted predicates from schema. Fixes [#5053][]. ([#5327][])
- Fix segmentation fault in query.go. ([#5377][]) 
- Update group checksums when combining multiple deltas. Fixes [#5368][]. ([#5394][]) 
- Fix empty string checks. ([#5396][])
- Fix protobuf headers check. ([#5381][])
- Stream the full set of predicates and types during a snapshot. ([#5444][])
- Use pre-allocated protobufs during backups. ([#5508][])
- Replace strings.Trim with strings.TrimFunc in ParseRDF. ([#5494][]) 
- Return nil instead of emptyTablet in groupi.Tablet(). ([#5469][]) 
- During shutdown, generate snapshot before closing raft node. ([#5476][]) 
- Get lists of predicates and types before sending the snapshot. ([#5488][]) 
- Move runVlogGC to x and use it in zero as well. ([#5468][]) 
- Fix inconsistent bulk loader failures. Fixes [#5361][]. ([#5537][])

### Security

- Use SensitiveByteSlice type for hmac secret. ([#5451][])

[#5444]: https://github.com/dgraph-io/dgraph/issues/5444
[#5359]: https://github.com/dgraph-io/dgraph/issues/5359
[#5405]: https://github.com/dgraph-io/dgraph/issues/5405
[#5327]: https://github.com/dgraph-io/dgraph/issues/5327
[#5377]: https://github.com/dgraph-io/dgraph/issues/5377
[#5394]: https://github.com/dgraph-io/dgraph/issues/5394
[#5396]: https://github.com/dgraph-io/dgraph/issues/5396
[#5053]: https://github.com/dgraph-io/dgraph/issues/5053
[#5368]: https://github.com/dgraph-io/dgraph/issues/5368
[#5451]: https://github.com/dgraph-io/dgraph/issues/5451
[#5381]: https://github.com/dgraph-io/dgraph/issues/5381
[#5327]: https://github.com/dgraph-io/dgraph/issues/5327
[#5377]: https://github.com/dgraph-io/dgraph/issues/5377
[#5508]: https://github.com/dgraph-io/dgraph/issues/5508
[#5494]: https://github.com/dgraph-io/dgraph/issues/5494
[#5469]: https://github.com/dgraph-io/dgraph/issues/5469
[#5476]: https://github.com/dgraph-io/dgraph/issues/5476
[#5488]: https://github.com/dgraph-io/dgraph/issues/5488
[#5468]: https://github.com/dgraph-io/dgraph/issues/5468
[#5361]: https://github.com/dgraph-io/dgraph/issues/5361
[#5537]: https://github.com/dgraph-io/dgraph/issues/5537

## [20.03.2] - 2020-05-15
This release was removed

## [1.2.4] - 2020-05-15
This release was removed

## [20.03.1] - 2020-04-24
[20.03.1]: https://github.com/dgraph-io/dgraph/compare/v20.03.0...v20.03.1

### Changed

- Support comma separated list of zero addresses in alpha. ([#5258][])
- Optimization: Optimize snapshot creation ([#4901][])
- Optimization: Remove isChild from fastJsonNode. ([#5184][])
- Optimization: Memory improvements in fastJsonNode. ([#5088][])
- Update badger to commit cddf7c03451c. ([#5272][])
  - Compression/encryption runs in the background (which means faster writes)
  - Separate cache for bloom filters which limits the amount of memory used by bloom filters
- Avoid crashing live loader in case the network is interrupted. ([#5268][])
- Enterprise features
  - Backup/restore: Force users to explicitly tell restore command to run without zero. ([#5206][])
  - Alpha: Expose compression_level option. ([#5280][])

### Fixed

- Implement json.Marshal just for strings. ([#4979][]) 
- Change error message in case of successful license application. Fixes [#4965][]. ([#5230][]) 
- Add OPTIONS support for /ui/keywords. Fixes [#4946][]. ([#4992][]) 
- Check uid list is empty when filling shortest path vars. ([#5152][])
- Return error for invalid UID 0x0. Fixes [#5238][]. ([#5252][]) 
- Skipping floats that cannot be marshalled (+Inf, -Inf, NaN). ([#5199][], [#5163][])
- Fix panic in Task FrameWork. Fixes [#5034][]. ([#5081][]) 
- graphql: @dgraph(pred: "...") with @search. ([#5019][]) 
- graphql: ensure @id uniqueness within a mutation. ([#4959][]) 
- Set correct posting list type while creating it in live loader. ([#5012][])
- Add support for tinyint in migrate tool. Fixes [#4674][]. ([#4842][])
- Fix bug, aggregate value var works with blank node in upsert. Fixes [#4712][]. ([#4767][])
- Always set BlockSize in encoder. Fixes [#5102][]. ([#5255][])
- Optimize uid allocation in live loader. ([#5132][]) 
- Shutdown executor goroutines. ([#5150][])
- Update RAFT checkpoint when doing a clean shutdown. ([#5097][])
- Enterprise features
  - Backup schema keys in incremental backups. Before, the schema was only stored in the full backup. ([#5158][])
  
### Added

- Return list of ongoing tasks in /health endpoint. ([#4961][])
- Propose snapshot once indexing is complete. ([#5005][])
- Add query/mutation logging in glog V=3. ([#5024][]) 
- Include the total number of touched nodes in the query metrics. ([#5073][]) 
- Flag to turn on/off sending Sentry events, default is on. ([#5169][])
- Concurrent Mutations. ([#4892][]) 
- Enterprise features
  - Support bulk loader use-case to import unencrypted export and encrypt. ([#5213][])
  - Create encrypted restore directory from encrypted backups. ([#5144][])
  - Add option "--encryption_key_file"/"-k" to debug tool for encryption support. ([#5146][])
  - Support for encrypted backups/restore. **Note**: Older backups without encryption will be incompatible with this Dgraph version. Solution is to force a full backup before creating further incremental backups. ([#5103][])
  - Add encryption support for export and import (via bulk, live loaders). ([#5155][])
  - Add Badger expvar metrics to Prometheus metrics. Fixes [#4772][]. ([#5094][])
  - Add option to apply enterprise license at zero's startup. ([#5170][])

[#4979]: https://github.com/dgraph-io/dgraph/issues/4979
[#5230]: https://github.com/dgraph-io/dgraph/issues/5230
[#4965]: https://github.com/dgraph-io/dgraph/issues/4965
[#4992]: https://github.com/dgraph-io/dgraph/issues/4992
[#4946]: https://github.com/dgraph-io/dgraph/issues/4946
[#4961]: https://github.com/dgraph-io/dgraph/issues/4961
[#5005]: https://github.com/dgraph-io/dgraph/issues/5005
[#5024]: https://github.com/dgraph-io/dgraph/issues/5024
[#5073]: https://github.com/dgraph-io/dgraph/issues/5073
[#5280]: https://github.com/dgraph-io/dgraph/issues/5280
[#5097]: https://github.com/dgraph-io/dgraph/issues/5097
[#5150]: https://github.com/dgraph-io/dgraph/issues/5150
[#5132]: https://github.com/dgraph-io/dgraph/issues/5132
[#4959]: https://github.com/dgraph-io/dgraph/issues/4959
[#5019]: https://github.com/dgraph-io/dgraph/issues/5019
[#5081]: https://github.com/dgraph-io/dgraph/issues/5081
[#5034]: https://github.com/dgraph-io/dgraph/issues/5034
[#5169]: https://github.com/dgraph-io/dgraph/issues/5169
[#5170]: https://github.com/dgraph-io/dgraph/issues/5170
[#4892]: https://github.com/dgraph-io/dgraph/issues/4892
[#5146]: https://github.com/dgraph-io/dgraph/issues/5146
[#5206]: https://github.com/dgraph-io/dgraph/issues/5206
[#5152]: https://github.com/dgraph-io/dgraph/issues/5152
[#5252]: https://github.com/dgraph-io/dgraph/issues/5252
[#5199]: https://github.com/dgraph-io/dgraph/issues/5199
[#5158]: https://github.com/dgraph-io/dgraph/issues/5158
[#5213]: https://github.com/dgraph-io/dgraph/issues/5213
[#5144]: https://github.com/dgraph-io/dgraph/issues/5144
[#5146]: https://github.com/dgraph-io/dgraph/issues/5146
[#5103]: https://github.com/dgraph-io/dgraph/issues/5103
[#5155]: https://github.com/dgraph-io/dgraph/issues/5155
[#5238]: https://github.com/dgraph-io/dgraph/issues/5238
[#5272]: https://github.com/dgraph-io/dgraph/issues/5272

## [1.2.3] - 2020-04-24
[1.2.3]: https://github.com/dgraph-io/dgraph/compare/v1.2.2...v1.2.3

### Changed

- Support comma separated list of zero addresses in alpha. ([#5258][])
- Optimization: Optimize snapshot creation. ([#4901][])
- Optimization: Remove isChild from fastJsonNode. ([#5184][])
- Optimization: Memory improvements in fastJsonNode. ([#5088][])
- Update Badger to commit cddf7c03451c33. ([#5273][])
  - Compression/encryption runs in the background (which means faster writes)
  - Separate cache for bloom filters which limits the amount of memory used by bloom filters
- Avoid crashing live loader in case the network is interrupted. ([#5268][])
- Enterprise features
  - Backup/restore: Force users to explicitly tell restore command to run without zero. ([#5206][])

### Fixed

- Check uid list is empty when filling shortest path vars. ([#5152][])
- Return error for invalid UID 0x0. Fixes [#5238][]. ([#5252][]) 
- Skipping floats that cannot be marshalled (+Inf, -Inf, NaN). ([#5199][], [#5163][])
- Set correct posting list type while creating it in live loader. ([#5012][])
- Add support for tinyint in migrate tool. Fixes [#4674][]. ([#4842][])
- Fix bug, aggregate value var works with blank node in upsert. Fixes [#4712][]. ([#4767][])
- Always set BlockSize in encoder. Fixes [#5102][]. ([#5255][])
- Enterprise features
  - Backup schema keys in incremental backups. Before, the schema was only stored in the full backup. ([#5158][])

### Added

- Add Badger expvar metrics to Prometheus metrics. Fixes [#4772][]. ([#5094][])
- Enterprise features
    - Support bulk loader use-case to import unencrypted export and encrypt. ([#5213][])
  - Create encrypted restore directory from encrypted backups. ([#5144][])
  - Add option "--encryption_key_file"/"-k" to debug tool for encryption support. ([#5146][])
  - Support for encrypted backups/restore. **Note**: Older backups without encryption will be incompatible with this Dgraph version. Solution is to force a full backup before creating further incremental backups. ([#5103][])
  - Add encryption support for export and import (via bulk, live loaders). ([#5155][])

[#5146]: https://github.com/dgraph-io/dgraph/issues/5146
[#5206]: https://github.com/dgraph-io/dgraph/issues/5206
[#5152]: https://github.com/dgraph-io/dgraph/issues/5152
[#5252]: https://github.com/dgraph-io/dgraph/issues/5252
[#5199]: https://github.com/dgraph-io/dgraph/issues/5199
[#5163]: https://github.com/dgraph-io/dgraph/issues/5163
[#5158]: https://github.com/dgraph-io/dgraph/issues/5158
[#5213]: https://github.com/dgraph-io/dgraph/issues/5213
[#5144]: https://github.com/dgraph-io/dgraph/issues/5144
[#5146]: https://github.com/dgraph-io/dgraph/issues/5146
[#5103]: https://github.com/dgraph-io/dgraph/issues/5103
[#5155]: https://github.com/dgraph-io/dgraph/issues/5155
[#5238]: https://github.com/dgraph-io/dgraph/issues/5238
[#5012]: https://github.com/dgraph-io/dgraph/issues/5012
[#4674]: https://github.com/dgraph-io/dgraph/issues/4674
[#4842]: https://github.com/dgraph-io/dgraph/issues/4842
[#5116]: https://github.com/dgraph-io/dgraph/issues/5116
[#5258]: https://github.com/dgraph-io/dgraph/issues/5258
[#4901]: https://github.com/dgraph-io/dgraph/issues/4901
[#5184]: https://github.com/dgraph-io/dgraph/issues/5184
[#5088]: https://github.com/dgraph-io/dgraph/issues/5088
[#5273]: https://github.com/dgraph-io/dgraph/issues/5273
[#5216]: https://github.com/dgraph-io/dgraph/issues/5216
[#5268]: https://github.com/dgraph-io/dgraph/issues/5268
[#5102]: https://github.com/dgraph-io/dgraph/issues/5102
[#5255]: https://github.com/dgraph-io/dgraph/issues/5255
[#4772]: https://github.com/dgraph-io/dgraph/issues/4772
[#5094]: https://github.com/dgraph-io/dgraph/issues/5094

## [20.03.0] - 2020-03-30
[20.03.0]: https://github.com/dgraph-io/dgraph/compare/v1.2.2...v20.03.0
** Note: This release requires you to export and re-import data prior to upgrading or rolling back. The underlying data format has been changed. **

### Changed

- Report GraphQL stats from alpha. ([#4607][])
- During backup, collapse split posting lists into a single list. ([#4682][]) 
- Optimize computing reverse reindexing. ([#4755][])
- Add partition key based iterator to the bulk loader. ([#4841][])
- Invert s2 loop instead of rebuilding. ([#4782][])
- Update Badger Version. ([#4935][])
- Incremental Rollup and Tablet Size Calculation. ([#4972][])
- Track internal operations and cancel when needed. ([#4916][])
- Set version when rollup is called with no splits. ([#4945][])
- Use a different stream writer id for split keys. ([#4875][])
- Split posting lists recursively. ([#4867][])
- Add support for tinyint in migrate tool. Fixes [#4674][]. ([#4842][])
- Enterprise features
  - **Breaking changes**
    - [BREAKING] Underlying schema for ACL has changed. Use the upgrade tool to migrate to the new data format. ([#4725][])

### Added

- Add GraphQL API for Dgraph accessible via the `/graphql` and `/admin` HTTP endpoints on Dgraph Alpha. ([#933][])
- Add support for sorting on multiple facets. Fixes [#3638][]. ([#4579][])
- Expose Badger Compression Level option in Bulk Loader. ([#4669][])
- GraphQL Admin API: Support Backup operation. ([#4706][])
- GraphQL Admin API: Support export, draining, shutdown and setting lrumb operations. ([#4739][])
- GraphQL Admin API: duplicate `/health` in GraphQL `/admin` ([#4768][])
- GraphQL Admin API: Add `/admin/schema` endpoint ([#4777][]) 
- Perform indexing in background. ([#4819][])
- Basic Sentry Integration - Capture manual panics with Sentry exception and runtime panics with a wrapper on panic. ([#4756][])
- Ludicrous Mode. ([#4872][])
- Enterprise features
  - ACL: Allow users to query data for their groups, username, and permissions. ([#4774][])
  - ACL: Support ACL operations using the admin GraphQL API. ([#4760][]) 
  - ACL: Add tool to upgrade ACLs. ([#5016][])

### Fixed
 
- Avoid running GC frequently. Only run for every 2GB of increase. Small optimizations in Bulk.reduce.
- Check response status when posting telemetry data. ([#4726][])
- Add support for $ in quoted string. Fixes [#4695][]. ([#4702][])
- Do not include empty nodes in the export output. Fixes [#3610][]. ([#4773][]) 
- Fix Nquad value conversion in live loader. Fixes [#4468][]. ([#4793][])
- Use `/tmp` dir to store temporary index. Fixes [#4600][]. ([#4766][])
- Properly initialize posting package in debug tool. ([#4893][])
- Fix bug, aggregate value var works with blank node in upsert. Fixes [#4712][]. ([#4767][])
- Fix count with facets filter. Fixes [#4659][]. ([#4751][])
- Change split keys to have a different prefix. Fixes [#4905][]. ([#4908][])
- Various optimizations for facets filter queries. ([#4923][])
- Throw errors returned by retrieveValuesAndFacets. Fixes [#4958][]. ([#4970][])
- Add "runInBackground" option to Alter to run indexing in background. When set to `true`, then the Alter call returns immediately. When set to `false`, the call blocks until indexing is complete. This is set to `false` by default. ([#4981][])
- Set correct posting list type while creating it in the live loader. Fixes [#4889][]. ([#5012][])
- **Breaking changes**
  - [BREAKING] Language sorting on Indexed data. Fixes [#4005][]. ([#4316][]) 

[#5016]: https://github.com/dgraph-io/dgraph/issues/5016
[#5012]: https://github.com/dgraph-io/dgraph/issues/5012
[#4889]: https://github.com/dgraph-io/dgraph/issues/4889
[#4958]: https://github.com/dgraph-io/dgraph/issues/4958
[#4905]: https://github.com/dgraph-io/dgraph/issues/4905
[#4659]: https://github.com/dgraph-io/dgraph/issues/4659
[#4712]: https://github.com/dgraph-io/dgraph/issues/4712
[#4893]: https://github.com/dgraph-io/dgraph/issues/4893
[#4767]: https://github.com/dgraph-io/dgraph/issues/4767
[#4751]: https://github.com/dgraph-io/dgraph/issues/4751
[#4908]: https://github.com/dgraph-io/dgraph/issues/4908
[#4923]: https://github.com/dgraph-io/dgraph/issues/4923
[#4970]: https://github.com/dgraph-io/dgraph/issues/4970
[#4981]: https://github.com/dgraph-io/dgraph/issues/4981
[#4841]: https://github.com/dgraph-io/dgraph/issues/4841
[#4782]: https://github.com/dgraph-io/dgraph/issues/4782
[#4935]: https://github.com/dgraph-io/dgraph/issues/4935
[#4972]: https://github.com/dgraph-io/dgraph/issues/4972
[#4916]: https://github.com/dgraph-io/dgraph/issues/4916
[#4945]: https://github.com/dgraph-io/dgraph/issues/4945
[#4875]: https://github.com/dgraph-io/dgraph/issues/4875
[#4867]: https://github.com/dgraph-io/dgraph/issues/4867
[#4872]: https://github.com/dgraph-io/dgraph/issues/4872
[#4756]: https://github.com/dgraph-io/dgraph/issues/4756
[#4819]: https://github.com/dgraph-io/dgraph/issues/4819
[#4755]: https://github.com/dgraph-io/dgraph/issues/4755
[#4600]: https://github.com/dgraph-io/dgraph/issues/4600
[#4766]: https://github.com/dgraph-io/dgraph/issues/4766
[#4468]: https://github.com/dgraph-io/dgraph/issues/4468
[#4793]: https://github.com/dgraph-io/dgraph/issues/4793
[#4777]: https://github.com/dgraph-io/dgraph/issues/4777
[#4768]: https://github.com/dgraph-io/dgraph/issues/4768
[#4760]: https://github.com/dgraph-io/dgraph/issues/4760
[#4739]: https://github.com/dgraph-io/dgraph/issues/4739
[#4706]: https://github.com/dgraph-io/dgraph/issues/4706
[#4607]: https://github.com/dgraph-io/dgraph/issues/4607
[#933]: https://github.com/dgraph-io/dgraph/issues/933
[#3638]: https://github.com/dgraph-io/dgraph/issues/3638
[#4579]: https://github.com/dgraph-io/dgraph/issues/4579
[#4682]: https://github.com/dgraph-io/dgraph/issues/4682
[#4725]: https://github.com/dgraph-io/dgraph/issues/4725
[#4669]: https://github.com/dgraph-io/dgraph/issues/4669
[#4774]: https://github.com/dgraph-io/dgraph/issues/4774
[#4726]: https://github.com/dgraph-io/dgraph/issues/4726
[#4695]: https://github.com/dgraph-io/dgraph/issues/4695
[#4702]: https://github.com/dgraph-io/dgraph/issues/4702
[#3610]: https://github.com/dgraph-io/dgraph/issues/3610
[#4773]: https://github.com/dgraph-io/dgraph/issues/4773
[#4005]: https://github.com/dgraph-io/dgraph/issues/4005
[#4316]: https://github.com/dgraph-io/dgraph/issues/4316

## [1.2.2] - 2020-03-19
[1.2.2]: https://github.com/dgraph-io/dgraph/compare/v1.2.1...v1.2.2

### Changed

- Wrap errors thrown in posting/list.go for easier debugging. ([#4880][])
- Print keys using hex encoding in error messages in list.go. ([#4891][])

### Fixed

- Do not include empty nodes in the export output. ([#4896][])
- Fix error when lexing language list. ([#4784][])
- Properly initialize posting package in debug tool. ([#4893][])
- Handle special characters in schema and type queries. Fixes [#4933][]. ([#4937][])
- Overwrite values for uid predicates.  Fixes [#4879][]. ([#4883][])
- Disable @* language queries when the predicate does not support langs. ([#4881][])
- Fix bug in exporting types with reverse predicates. Fixes [#4856][]. ([#4857][])
- Do not skip over split keys. (Trying to skip over the split keys sometimes skips over keys belonging to a different split   key. This is a fix just for this release as the actual fix requires changes to the data format.) ([#4951][])
- Fix point-in-time Prometheus metrics. Fixes [#4532][]. ([#4948][])
- Split lists in the bulk loader. ([#4967][])
- Allow remote MySQL server with dgraph migrate tool. Fixes [#4707][]. ([#4860][])
- Enterprise features
  - ACL: Allow uid access. ([#4922][])
  - Backups: Assign maxLeaseId during restore. Fixes [#4816][]. ([#4877][])
  - Backups: Verify host when default and custom credentials are used. Fixes [#4855][]. ([#4858][])
  - Backups: Split lists when restoring from backup. ([#4912][])


[#4967]: https://github.com/dgraph-io/dgraph/issues/4967
[#4951]: https://github.com/dgraph-io/dgraph/issues/4951
[#4532]: https://github.com/dgraph-io/dgraph/issues/4532
[#4948]: https://github.com/dgraph-io/dgraph/issues/4948
[#4893]: https://github.com/dgraph-io/dgraph/issues/4893
[#4784]: https://github.com/dgraph-io/dgraph/issues/4784
[#4896]: https://github.com/dgraph-io/dgraph/issues/4896
[#4856]: https://github.com/dgraph-io/dgraph/issues/4856
[#4857]: https://github.com/dgraph-io/dgraph/issues/4857
[#4881]: https://github.com/dgraph-io/dgraph/issues/4881
[#4912]: https://github.com/dgraph-io/dgraph/issues/4912
[#4855]: https://github.com/dgraph-io/dgraph/issues/4855 
[#4858]: https://github.com/dgraph-io/dgraph/issues/4858 
[#4879]: https://github.com/dgraph-io/dgraph/issues/4879 
[#4883]: https://github.com/dgraph-io/dgraph/issues/4883 
[#4933]: https://github.com/dgraph-io/dgraph/issues/4933
[#4937]: https://github.com/dgraph-io/dgraph/issues/4937
[#4891]: https://github.com/dgraph-io/dgraph/issues/4891
[#4880]: https://github.com/dgraph-io/dgraph/issues/4880
[#4816]: https://github.com/dgraph-io/dgraph/issues/4816
[#4877]: https://github.com/dgraph-io/dgraph/issues/4877
[#4922]: https://github.com/dgraph-io/dgraph/issues/4922
[#4707]: https://github.com/dgraph-io/dgraph/issues/4707
[#4860]: https://github.com/dgraph-io/dgraph/issues/4860


## [1.2.1] - 2020-02-06
[1.2.1]: https://github.com/dgraph-io/dgraph/compare/v1.2.0...v1.2.1

### Fixed

- Fix bug related to posting list split, and re-enable posting list splits. Fixes [#4733][]. ([#4742][])

[#4733]: https://github.com/dgraph-io/dgraph/issues/4733
[#4742]: https://github.com/dgraph-io/dgraph/issues/4742

## [1.2.0] - 2020-01-27
[1.2.0]: https://github.com/dgraph-io/dgraph/compare/v1.1.1...v1.2.0

### Changed

- Allow overwriting values of predicates of type uid. Fixes [#4136][]. ([#4411][])
- Algorithms to handle UidPack. ([#4321][])
- Improved latency in live loader using conflict resolution at client level. ([#4362][])
- Set ZSTD CompressionLevel to 1. ([#4572][])
- Splits are now disabled. ([#4672][])
- Disk based re-indexing: while re-indexing a predicate, the temp data is now written on disk
  instead of keeping it in memory. This improves index rebuild for large datasets. ([#4440][])
- Enterprise features
  - **Breaking changes**
    - Change default behavior to block operations with ACLs enabled. ([#4390][])
  - Remove unauthorized predicates from query instead of rejecting the query entirely. ([#4479][])

### Added

- Add `debuginfo` subcommand to dgraph. ([#4464][])
- Support filtering on non-indexed predicate. Fixes [#4305][]. ([#4531][])
- Add support for variables in recurse. Fixes [#3301][]. ([#4385][]).
- Adds `@noconflict` schema directive to prevent conflict detection. This is an experimental feature. This is not a recommended directive, but exists to help avoid conflicts for predicates which don't have high correctness requirements. Fixes [#4079][]. ([#4454][])
- Implement the state HTTP endpoint on Alpha. Login is required if ACL is enabled. ([#4435][]).
- Implement `/health?all` endpoint on Alpha nodes. ([#4535][])
- Add `/health` endpoint to Zero. ([#4405][])
- **Breaking changes**
  - Support for fetching facets from value edge list. The query response format is backwards-incompatible. Fixes [#4081][]. ([#4267][])
- Enterprise features
  - Add guardians group with full authorization. ([#4447][])

 ### Fixed
 
- Infer type of schema from JSON and RDF mutations.	Fixes [#3788][]. ([#4328][])
- Fix retrieval of facets with cascade. Fixes	[#4310][]. ([#4530][])
- Do not use type keys during tablet size calculation.	Fixes [#4473][]. ([#4517][])
- Fix Levenshtein distance calculation with match function.	Fixes [#4494][]. ([#4545][])
- Add `<xs:integer>` RDF type for int schema type. Fixes [#4460][]. ([#4465][])
- Allow `@filter` directive with expand queries. Fixes [#3904][]. ([#4404][]).
- A multi-part posting list should only be accessed via the main key. Accessing the posting list via one of the other keys was causing issues during rollup and adding spurious keys to the database. Now fixed. ([#4574][])
- Enterprise features
  - Backup types. Fixes [#4507][]. ([#4514][])

[#4440]: https://github.com/dgraph-io/dgraph/pull/4440
[#4574]: https://github.com/dgraph-io/dgraph/pull/4574
[#4672]: https://github.com/dgraph-io/dgraph/pull/4672
[#4530]: https://github.com/dgraph-io/dgraph/issues/4530
[#4310]: https://github.com/dgraph-io/dgraph/issues/4310
[#4517]: https://github.com/dgraph-io/dgraph/issues/4517
[#4473]: https://github.com/dgraph-io/dgraph/issues/4473
[#4545]: https://github.com/dgraph-io/dgraph/issues/4545
[#4494]: https://github.com/dgraph-io/dgraph/issues/4494
[#4460]: https://github.com/dgraph-io/dgraph/issues/4460
[#4465]: https://github.com/dgraph-io/dgraph/issues/4465
[#4404]: https://github.com/dgraph-io/dgraph/issues/4404
[#3904]: https://github.com/dgraph-io/dgraph/issues/3904
[#4514]: https://github.com/dgraph-io/dgraph/issues/4514
[#4507]: https://github.com/dgraph-io/dgraph/issues/4507
[#4328]: https://github.com/dgraph-io/dgraph/issues/4328
[#3788]: https://github.com/dgraph-io/dgraph/issues/3788
[#4447]: https://github.com/dgraph-io/dgraph/issues/4447
[#4411]: https://github.com/dgraph-io/dgraph/issues/4411
[#4321]: https://github.com/dgraph-io/dgraph/issues/4321
[#4362]: https://github.com/dgraph-io/dgraph/issues/4362
[#4572]: https://github.com/dgraph-io/dgraph/issues/4572
[#4390]: https://github.com/dgraph-io/dgraph/issues/4390
[#4479]: https://github.com/dgraph-io/dgraph/issues/4479
[#4136]: https://github.com/dgraph-io/dgraph/issues/4136
[#4411]: https://github.com/dgraph-io/dgraph/issues/4411
[#4464]: https://github.com/dgraph-io/dgraph/issues/4464
[#4531]: https://github.com/dgraph-io/dgraph/issues/4531
[#4305]: https://github.com/dgraph-io/dgraph/issues/4305
[#4454]: https://github.com/dgraph-io/dgraph/issues/4454
[#4079]: https://github.com/dgraph-io/dgraph/issues/4079
[#4405]: https://github.com/dgraph-io/dgraph/issues/4405
[#4267]: https://github.com/dgraph-io/dgraph/issues/4267
[#4081]: https://github.com/dgraph-io/dgraph/issues/4081
[#4447]: https://github.com/dgraph-io/dgraph/issues/4447
[#4535]: https://github.com/dgraph-io/dgraph/issues/4535
[#4385]: https://github.com/dgraph-io/dgraph/issues/4385
[#3301]: https://github.com/dgraph-io/dgraph/issues/3301
[#4435]: https://github.com/dgraph-io/dgraph/issues/4435

## [1.1.1] - 2019-12-16
[1.1.1]: https://github.com/dgraph-io/dgraph/compare/v1.1.0...v1.1.1

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
- Add support for multiple mutations blocks in upsert blocks. ([#4210][])
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
- Fix `has` pagination when predicate is queried with `@lang`. Fixes [#4282][]. ([#4331][])
- Make uid function work with value variables in upsert blocks. Fixes [#4424][]. ([#4425][])

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
[#4424]: https://github.com/dgraph-io/dgraph/issues/4424
[#4425]: https://github.com/dgraph-io/dgraph/issues/4425

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

## [1.0.18] - 2019-12-16
[1.0.18]: https://github.com/dgraph-io/dgraph/compare/v1.0.17...v1.0.18

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
- Introduction of new command `dgraph cert` to simplify initial TLS setup. See [TLS configuration docs](https://dgraph.io/docs/deploy/#tls-configuration) for more info.
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

* Dgraph adds support for distributed ACID transactions (a blog post is in works). Transactions can be done via the Go, Java or HTTP clients (JS client coming). See [docs here](https://dgraph.io/docs/clients/).
* Support for Indexing via [Custom tokenizers](https://dgraph.io/docs/query-language/#indexing-with-custom-tokenizers).
* Support for CJK languages in the full-text index.

### Changed

#### Running Dgraph

* We have consolidated all the `server`, `zero`, `live/bulk-loader` binaries into a single `dgraph` binary for convenience. Instructions for running Dgraph can be found in the [docs](https://dgraph.io/docs/get-started/).
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
Examples for [Go client](https://godoc.org/github.com/dgraph-io/dgraph/client#example-Txn-Mutate-Facets) and [HTTP](https://dgraph.io/docs/query-language/#facets-edge-attributes).
* Query latency is now returned as numeric (ns) instead of string.
* [`Recurse`](https://dgraph.io/docs/query-language/#recurse-query) is now a directive. So queries with `recurse` keyword at root won't work anymore.
* Syntax for [`count` at root](https://dgraph.io/docs/query-language/#count) has changed. You need to ask for `count(uid)`, instead of `count()`.

#### Mutations

* Mutations can only be done via `Mutate` Grpc endpoint or via [`/mutate` HTTP handler](https://dgraph.io/docs/clients/#transactions).
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
* [`Upsert`](https://dgraph.io/docs/v0.8.3/query-language/#upsert) directive and [mutation variables](https://dgraph.io/docs/v0.8.3/query-language/#variables-in-mutations) go away. Both these functionalities can now easily be achieved via transactions.

#### Schema

* `<*> <pred> <*>` operations, that is deleting a predicate can't be done via mutations anymore. They need to be done via `Alter` Grpc endpoint or via the `/alter` HTTP handler.
* Drop all is now done via `Alter`.
* Schema updates are now done via `Alter` Grpc endpoint or via `/alter` HTTP handler.

#### Go client

* `Query` Grpc endpoint returns response in JSON under `Json` field instead of protocol buffer. `client.Unmarshal` method also goes away from the Go client. Users can use `json.Unmarshal` for unmarshalling the response.
* Response for predicate of type `geo` can be unmarshalled into a struct. Example [here](https://godoc.org/github.com/dgraph-io/dgraph/client#example-package--SetObject).
* `Node` and `Edge` structs go away along with the `SetValue...` methods. We recommend using [`SetJson`](https://godoc.org/github.com/dgraph-io/dgraph/client#example-package--SetObject) and `DeleteJson` fields to do mutations.
* Examples of how to use transactions using the client can be found at https://dgraph.io/docs/clients/#go.

### Removed
- Embedded dgraph goes away. We haven’t seen much usage of this feature. And it adds unnecessary maintenance overhead to the code.
- Dgraph live no longer stores external ids. And hence the `xid` flag is gone.
