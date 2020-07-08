# Change log

----

**NOTE:** This changelog is no longer maintained. Changes are now tracked in
the top level [`CHANGELOG.md`](https://github.com/apollographql/apollo-link/blob/master/CHANGELOG.md).

----

### 1.1.2

- No changes

### 1.1.1
- Added `graphql` 14 to peer and dev deps; Updated `@types/graphql` to 14  <br/>
  [@hwillson](http://github.com/hwillson) in [#789](https://github.com/apollographql/apollo-link/pull/789)
- Update types to be compatible with `@types/graphql@0.13.3`

### 1.1.0
- Pass `forward` into error handler for ErrorLink to support retrying a failed request

### 1.0.9
- Correct return type to FetchResult [#600](https://github.com/apollographql/apollo-link/pull/600)

### 1.0.8
- Update apollo-link [#559](https://github.com/apollographql/apollo-link/pull/559)

### 1.0.7
- update apollo link with zen-observable-ts [PR#515](https://github.com/apollographql/apollo-link/pull/515)

### 1.0.6
- ApolloLink upgrade

### 1.0.5
- ApolloLink upgrade

### 1.0.4
- ApolloLink upgrade

### 1.0.3
- export options as named interface [TypeScript]

### 1.0.2
- changed peer-dependency of apollo-link to actual dependency
- graphQLErrors alias networkError.result.errors on a networkError

### 1.0.1
- moved to better rollup build

### 1.0.0
- Added the operation and any data to the error handler callback
- changed graphqlErrors to be graphQLErrors to be consistent with Apollo Client
