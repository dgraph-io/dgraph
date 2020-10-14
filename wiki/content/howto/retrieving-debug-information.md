+++
date = "2017-03-20T22:25:17+11:00"
title = "Retrieving Debug Information"
weight = 1
[menu.main]
    parent = "howto"
+++

Each Dgraph data node exposes profile over `/debug/pprof` endpoint and metrics over `/debug/vars` endpoint. Each Dgraph data node has it's own profiling and metrics information. Below is a list of debugging information exposed by Dgraph and the corresponding commands to retrieve them.

## Metrics Information

If you are collecting these metrics from outside the Dgraph instance you need to pass `--expose_trace=true` flag, otherwise there metrics can be collected by connecting to the instance over localhost.

```
curl http://<IP>:<HTTP_PORT>/debug/vars
```

Metrics can also be retrieved in the Prometheus format at `/debug/prometheus_metrics`. See the [Metrics]({{< relref "deploy/metrics.md" >}}) section for the full list of metrics.

## Profiling Information

Profiling information is available via the `go tool pprof` profiling tool built into Go. The ["Profiling Go programs"](https://blog.golang.org/profiling-go-programs) Go blog post will help you get started with using pprof. Each Dgraph Zero and Dgraph Alpha exposes a debug endpoint at `/debug/pprof/<profile>` via the HTTP port.

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/heap
Fetching profile from ...
Saved Profile in ...
```
The output of the command would show the location where the profile is stored.

In the interactive pprof shell, you can use commands like `top` to get a listing of the top functions in the profile, `web` to get a visual graph of the profile opened in a web browser, or `list` to display a code listing with profiling information overlaid.

### CPU Profile

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/profile
```

### Memory Profile

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/heap
```

### Block Profile

Dgraph by default doesn't collect the block profile. Dgraph must be started with `--profile_mode=block` and `--block_rate=<N>` with N > 1.

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/block
```

### Goroutine stack

The HTTP page `/debug/pprof/` is available at the HTTP port of a Dgraph Zero or Dgraph Alpha. From this page a link to the "full goroutine stack dump" is available (e.g., on a Dgraph Alpha this page would be at `http://localhost:8080/debug/pprof/goroutine?debug=2`). Looking at the full goroutine stack can be useful to understand goroutine usage at that moment.