# README

### About
This directory contains some scripts and resources which were used to perform benchmarking and
profiling of normal and `@custom` HTTP queries. `@custom` HTTP queries were benchmarked for both
SINGLE and BATCH mode over REST. Please have a look at the [discuss post](https://discuss.dgraph.io/t/graphql-query-mutation-benchmarking-result/8604/5)
to find out more about the results.

### Usage
* First, generate some data for the Restaurant schema provided with [datagen](../datagen). Follow
the datagen README on how to do that. At the end of that, you will have a `~/__data` directory
, that is all we need to get started.
* Find out the `maxTxnTs` for that data, by starting zero and alpha in that directory and sending
a GET request to `/state` endpoint of alpha. Search for `maxTxnTs` in the HTTP response, and you
will get the value. Copy that value. Set `maxTxnTs` const in [graphql_profiler.go](profiling/graphql_profiler.go)
to that value. Now, stop the alpha and zero.
* Copy that data directory `~/__data` inside [profiling](profiling) directory as `__data`.
* Copy `schema.graphql` from [datagen](../datagen) inside [profiling/__data](profiling/__data).
* Now, make sure no other dgraph instance is running on your host machine or in docker. These
scripts use the default ports, so they may conflict. Also, be sure that your system has enough
RAM, otherwise, some queries may lead to OOM and killing of alpha processes on host machine and
docker.
* Also make sure that `localhost:9000` is available, as the `dgraph_api_server` uses that port.
* Now, checkout dgraph to `abhimanyu/benchmarking` branch & do a `make install`. We will use
 dgraph binary built from that branch, as it exposes a header to measure GraphQL layer time.
* Change your current working directory to the directory containing this README file.
* `$ go build dgraph_api_server.go`
* `$ nohup ./dgraph_api_server > dgraph_api_server.out &` - nohup is useful if you are on ssh.
* `$ cd profiling`
* `$ go build graphql_profiler.go`
* `$ nohup ./graphql_profiler > graphql_profiler.out &`

The last step should start the profiler. It will keep collecting all the benchmarking and
profiling information for you. If you are on ssh, you can exit now and come back later to find
the results inside [profiling/results](profiling/results) directory. For each benchmark schema and
its corresponding queries, you will get the results inside respective sub-directories inside the
results directory. The profiler also writes a log file named `graphql_profiler.log`. You can look
at that, `graphql_profiler.out`, or `dgraph_api_server.out` to find out more about any errors that
may happen during the run.

### How does it work
There are many directories inside [profiling/benchmarks](profiling/benchmarks) directory. Each
directory contains a `schema.graphql` file and another `queries` directory, which in-turn
contains some `.query` files. Each `.query` file contains a query which is run against the
corresponding schema.

The schema file in [0-th benchmark](profiling/benchmarks/0) is a simple schema. It does not have
any custom directives. So, when queries are run against this schema, it would just collect
benchmark data for pure GraphQL layer.

The rest of i-th benchmark directories contain schemas with `@custom` directive, varying over SINGLE
and BATCH mode and also where the `@custom` is applied.

The profiler first starts dgraph zero and alpha in docker with the simple schema contained in the
`__data` directory. The docker instance serves as the final API server for `@custom` HTTP calls.
Then, for each benchmarking schema it starts a dgraph instance on host, applying that schema and
performing all the queries for that schema against the host dgraph instance. The
`dgraph_api_server` acts as the necessary middleware between the host dgraph instance and the
docker dgraph instance.

For each schema, the collected benchmarking and profiling results are saved inside a sub-directory
of results directory. This is done for each query for that schema. The main files to look for are:
* `_Stats.txt`: This contains the overall average results for all queries for a schema.
* `_Durations.txt`: This contains the actual and average results for each query.
* `*_tracing.txt`: These files contain the Errors and Extensions reported by the GraphQL layer
  for that query.
* `*heap*.prof`: Files named like this are heap profiles. There are 3 kinds of heap profiles
  saved for each query. Pre, during and post. There may be many `during` profiles as a query may
  take a long time to complete.
* `*profile.prof`: Files named like this are CPU profiles for that query.

You will need to use `go tool pprof` to analyze these CPU and heap profiles.