+++
date = "2017-03-20T22:25:17+11:00"
title = "Debug"
weight = 19
[menu.main]
    parent = "query-language"
+++

For the purposes of debugging, you can attach a query parameter `debug=true` to a query. Attaching this parameter lets you retrieve the `uid` attribute for all the entities along with the `server_latency` and `start_ts` information under the `extensions` key of the response.

- `parsing_ns`: Latency in nanoseconds to parse the query.
- `processing_ns`: Latency in nanoseconds to process the query.
- `encoding_ns`: Latency in nanoseconds to encode the JSON response.
- `start_ts`: The logical start timestamp of the transaction.

Query with debug as a query parameter
```sh
curl -H "Content-Type: application/dql" http://localhost:8080/query?debug=true -XPOST -d $'{
  tbl(func: allofterms(name@en, "The Big Lebowski")) {
    name@en
  }
}' | python -m json.tool | less
```

Returns `uid` and `server_latency`
```
{
  "data": {
    "tbl": [
      {
        "uid": "0x41434",
        "name@en": "The Big Lebowski"
      },
      {
        "uid": "0x145834",
        "name@en": "The Big Lebowski 2"
      },
      {
        "uid": "0x2c8a40",
        "name@en": "Jeffrey \"The Big\" Lebowski"
      },
      {
        "uid": "0x3454c4",
        "name@en": "The Big Lebowski"
      }
    ],
    "extensions": {
      "server_latency": {
        "parsing_ns": 18559,
        "processing_ns": 802990982,
        "encoding_ns": 1177565
      },
      "txn": {
        "start_ts": 40010
      }
    }
  }
}
```
{{% notice "note" %}}
GraphQL+- has been renamed to Dgraph Query Language (DQL). While `application/dql`
is the preferred value for the `Content-Type` header, we will continue to support
`Content-Type: application/graphql+-` to avoid making breaking changes.
{{% /notice %}}
