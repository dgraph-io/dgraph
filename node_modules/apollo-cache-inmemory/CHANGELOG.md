# CHANGELOG

----

**NOTE:** This changelog is no longer maintained. Changes are now tracked in
the top level [`CHANGELOG.md`](https://github.com/apollographql/apollo-client/blob/master/CHANGELOG.md).

----

### 1.3.0

- Optimize repeated `apollo-cache-inmemory` reads by caching partial query
  results. As a consequence, watched queries will not be rebroadcast unless the
  data have changed.
  [PR #3394](https://github.com/apollographql/apollo-client/pull/3394)

### 1.2.5

- No changes.

### 1.2.4

- No changes.

### 1.2.3

- Added optional generics to cache manipulation methods (typescript).
  [PR #3541](https://github.com/apollographql/apollo-client/pull/3541)
- Restore non-enumerability of `resultFields[ID_KEY]`.
  [PR #3544](https://github.com/apollographql/apollo-client/pull/3544)
- Cache query documents transformed by InMemoryCache.
  [PR #3553](https://github.com/apollographql/apollo-client/pull/3553)

### 1.2.2

- Fixed an issue that caused fragment only queries to sometimes fail.
  [Issue #3402](https://github.com/apollographql/apollo-client/issues/3402)
  [PR #3507](https://github.com/apollographql/apollo-client/pull/3507)
- Fixed cache invalidation for inlined mixed types in union fields within
  arrays.
  [PR #3422](https://github.com/apollographql/apollo-client/pull/3422)

### 1.2.1

- Not documented

### vNEXT
- Optimize repeated `apollo-cache-inmemory` reads by caching partial query results
  [#3394](https://github.com/apollographql/apollo-client/pull/3394)

### 1.2.0

- Various optimizations for cache read performance
  [PR #3300](https://github.com/apollographql/apollo-client/pull/3300)
- Fix typo in documentation

### 1.1.12

- Fix an edge case where fields that were unions of two types, one with an
  `id`, one without an `id`, would cause the cache to throw while saving the
  result
  [PR #3159](https://github.com/apollographql/apollo-client/pull/3159)
- Map coverage to original source
- Fixed bug with cacheRedirects not getting attached
  [PR #3016](https://github.com/apollographql/apollo-client/pull/3016)

### 1.1.9

- Added `getCacheKey` function to cacheResolver context
  [PR #2998](https://github.com/apollographql/apollo-client/pull/2998)
- Changed `cacheResolvers` to `cacheRedirects`, added deprecation warning
  [PR #3001](https://github.com/apollographql/apollo-client/pull/3001)

### 1.1.8

- Dependency updates
- Fix IntrospectionResultData type definition
  [Issue #2959](https://github.com/apollographql/apollo-client/issues/2959)

### 1.1.7

- Update to latest apollo-utilities to support directives in cache

### 1.1.6 (unpublished)

- Update to latest apollo-utilities

### 1.1.5

- Update to latest apollo-cache base
  [PR #2818](https://github.com/apollographql/apollo-client/pull/2818)

### 1.1.4

- Change access modifier for data from "private" to "protected", to allow
  InMemoryCache subclasses to access it.

### 1.1.3

- Improves performance of in memory cache

### 1.1.2

- Ensure that heuristics warnings do not fire in production
  [PR #2611](https://github.com/apollographql/apollo-client/pull/2611)

### 1.1.1

- Change some access modifiers "private" to "protected" to allow code reuse by
  InMemoryCache subclasses.
- Improved rollup builds

### 1.1.0

- Improve errors for id mismatch when writing to the store
- Make it possible to swap the cache implementation. For example, you might
  want to use a `Map` to store the normalized objects, which can be faster
  than writing by keys to an `Object`. This also allows for custom use cases,
  such as emitting events on `.set()` or `.delete()` (think Observables),
  which was otherwise impossible without the use of Proxies, that are only
  available in some browsers. Unless you passed in the `store` to one of the
  `apollo-cache-inmemory` functions, such as: `writeQueryToStore` or
  `writeResultToStore`, no changes to your code are necessary. If you did
  access the cache's functions directly, all you need to do is add a
  `.toObject()` call on the cache — review the changes to the tests for [an example](https://github.com/apollographql/apollo-client/blob/cd563bcd1c2c15b973d0cdfd63332f5ee82da309/packages/apollo-cache-inmemory/src/__tests__/writeToStore.ts#L258).
  For reasoning behind this change and more information, see
  [Issue #2293](https://github.com/apollographql/apollo-client/issues/2293).

### 1.0.0

- Don't broadcast query watchers during a transaction (for example, while
  mutation results are being processed)
  [Issue #2221](https://github.com/apollographql/apollo-client/issues/2221)
  [PR #2358](https://github.com/apollographql/apollo-client/pull/2358)
- `readQuery` and `readFragment` return now the result instead of
  `Cache.DiffResult`
  [PR #2320](https://github.com/apollographql/apollo-client/pull/2320)

### 0.2.0-rc.1

- Move to named export to be consistent with rest of apollo ecosystem

### 0.2.0-beta.6

- Rename customResolvers to cacheResolvers with backwards compat

### 0.2.0-beta.5 and lower

- Fix error when missing __typename field in result
  [PR #2225](https://github.com/apollographql/apollo-client/pull/2225)
- Refactored type usage
- Prevented logging on defered queries
- Refactored internal store from apollo-client into own package
