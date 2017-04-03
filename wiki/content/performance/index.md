+++
title = "Performance"
+++

## General
### Dgraphloader
[Dgraphloader]({{< relref "deploy/index.md#bulk-data-loading" >}}) can be used to load the RDF data into Dgraph. As a reference point, it took on an average '''5 minutes to load 22M RDFs''' (from [benchmarks repository](https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz)) on an i7-6820HQ quad core Thinkpad T460 laptop with 16 GB RAM and SSD storage. The total output including the RAFT logs was 1.4 GB. Loading the same data on a ec2 [m4.large](https://aws.amazon.com/ec2/instance-types) machine took around 18 minutes.

![Dgraph loader performance on an i7 quad core Thinkpad](dgraphloader.gif)

### Queries

#### List of directors with whom Tom Hanks has worked
```
curl "localhost:8080/query?debug=true" -XPOST -d $'{
  actor(id: m.0bxtg) {
    name@en
    actor.film {
      performance.film {
        ~director.film {
          name@en
        }
      }
    }
  }
}'
```

With the data loaded above on the i7 quad core machine,it took '''8-9ms to run''' the query above the first time after server run.

```
{
    "server_latency":{
        "json":"457.894µs",
        "parsing":"110.403µs",
        "processing":"7.599935ms",
        "total":"8.170181ms"
    }
}
```

Consecutive runs of the same query took '''much lesser time (2-3ms)''', due to posting lists being available in memory.

```
{
    "server_latency":{
        "json":"1.258341ms",
        "parsing":"141.394µs",
        "processing":"1.202351ms",
        "total":"2.606949ms"
    }
}
```

#### Details of all the movies directed by Steven Spielberg like release date, actors, genre etc.

```
curl "localhost:8080/query?debug=true" -XPOST -d $'{
  director(id: m.06pj8) {
    name@en
    director.film {
      name@en
      initial_release_date
      country {
        name@en
      }
      starring {
        performance.actor {
          name@en
        }
        performance.character {
          name@en
        }
      }
      genre {
        name@en
      }
    }
  }
}'
```

On the i7 quad core machine,it took only '''87ms to run''' this pretty complicated query above the first time after server run.

```
{
    "server_latency":{
        "json":"7.534859ms",
        "parsing":"185.574µs",
        "processing":"79.298902ms",
        "total":"87.020921ms"
    }
}
```

Consecutive runs of the same query took '''much lesser time (30-35ms)'''.

```
{
    "server_latency":{
        "json":"12.513055ms",
        "parsing":"143.173µs",
        "processing":"20.898614ms",
        "total":"33.556651ms"
    }
}
```

## See Also
[Can it really scale?](https://open.dgraph.io/post/performance-throughput-latency)