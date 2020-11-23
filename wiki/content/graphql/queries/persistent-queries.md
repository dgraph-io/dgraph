+++
title = "Persistent Queries"
weight = 3
[menu.main]
    parent = "graphql-queries"
+++

Dgraph supports Persistent Queries. When a client uses persistent queries, the client only sends the hash of a query to the server. The server has a list of known hashes and uses the associated query accordingly.

Persistent queries significantly improve the performance and the security of an application since the smaller hash signature reduces bandwidth utilization and speeds up client loading times.

### Persisted Query logic

The execution of Persistent Queries follows this logic:

- If the `extensions` key is not provided in the `GET` request, Dgraph will process the request as usual
- If a `persistedQuery ` exists under the `extensions` key, Dgraph will try to process a Persisted Query:
	 - if no `sha256` hash is provided, process the query without persisting
	 - if the `sha256` hash is provided, try to retrieve the persisted query

Example:

```json
{
   "persistedQuery":{
      "sha256Hash":"b952c19b894e1aa89dc05b7d53e15ab34ee0b3a3f11cdf3486acef4f0fe85c52"
   }
}
```

### Create

To create a Persistent Query, both `query` and `sha256` must be provided.

Dgraph will verify the hash and perform a lookup. If the query doesn't exist, Dgraph will store the query, provided that the `sha256` of the query is correct. Finally, Dgraph will process the query and return the results.

Example: 

```sh
curl -g 'http://localhost:8080/graphql/?query={sample_query}&extensions={"persistedQuery":{"sha256Hash":"b952c19b894e1aa89dc05b7d53e15ab34ee0b3a3f11cdf3486acef4f0fe85c52"}}'
```

### Lookup

If only a `sha256` is provided, Dgraph will do a look-up, and process the query if found. Otherwise you'll get a `PersistedQueryNotFound` error.

Example: 

```sh
curl -g 'http://localhost:8080/graphql/?extensions={"persistedQuery":{"sha256Hash":"b952c19b894e1aa89dc05b7d53e15ab34ee0b3a3f11cdf3486acef4f0fe85c52"}}'
```

### Usage with Apollo

You can create an [Apollo GraphQL](https://www.apollographql.com/) client with persisted queries enabled. In the background, Apollo will send the same requests like the ones previously shown.

For example:

```go
import { createPersistedQueryLink } from "apollo-link-persisted-queries";
import { createHttpLink } from "apollo-link-http";
import { InMemoryCache } from "apollo-cache-inmemory";
import ApolloClient from "apollo-client";
const link = createPersistedQueryLink().concat(createHttpLink({ uri: "/graphql" }));
const client = new ApolloClient({
  cache: new InMemoryCache(),
  link: link,
});
```
