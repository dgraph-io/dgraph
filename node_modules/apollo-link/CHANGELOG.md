# Change log

----

**NOTE:** This changelog is no longer maintained. Changes are now tracked in
the top level [`CHANGELOG.md`](https://github.com/apollographql/apollo-link/blob/master/CHANGELOG.md).

----

### 1.2.4

- No changes

### 1.2.3
- Added `graphql` 14 to peer and dev deps; Updated `@types/graphql` to 14  <br/>
  [@hwillson](http://github.com/hwillson) in [#789](https://github.com/apollographql/apollo-link/pull/789)

### 1.2.2
- Update apollo-link [#559](https://github.com/apollographql/apollo-link/pull/559)
- export graphql types and add @types/graphql as a regular dependency [PR#576](https://github.com/apollographql/apollo-link/pull/576)
- moved @types/node to dev dependencies in package.josn to avoid collisions with other projects. [PR#540](https://github.com/apollographql/apollo-link/pull/540)

### 1.2.1
- update apollo link with zen-observable-ts to remove import issues [PR#515](https://github.com/apollographql/apollo-link/pull/515)

### 1.2.0
- Add `fromError` Observable helper
- change import method of zen-observable for rollup compat

### 1.1.0
- Expose `#execute` on ApolloLink as static

### 1.0.7
- Update to graphql@0.12

### 1.0.6
- update rollup

### 1.0.5
- fix bug where context wasn't merged when setting it

### 1.0.4
- export link util helpers

### 1.0.3
- removed requiring query on initial execution check
- moved to move efficent rollup build

### 1.0.1, 1.0.2
<!-- never published as latest -->
- preleases for dev tool integation

### 0.8.0
- added support for `extensions` on an operation

### 0.7.0
- new operation API and start of changelog
