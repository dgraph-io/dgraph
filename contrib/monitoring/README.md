# Dgraph memory monitoring

Alert rules, a Grafana dashboard, and a flight-recorder sidecar (`memwatch`) for diagnosing alpha memory problems:
slow growth that eventually hits the container limit, and sudden spikes that get an alpha OOM-killed. Everything
here works with Dgraph v24 and v25, because it only relies on metric names and HTTP endpoints that both versions
share.

| Path                                    | What it is                                                                          |
| --------------------------------------- | ----------------------------------------------------------------------------------- |
| `prometheus/dgraph-rules.yaml`          | Recording and alerting rules (plain Prometheus rules file)                          |
| `prometheus/dgraph-prometheusrule.yaml` | The same rules as a `PrometheusRule` for the Prometheus Operator (generated)        |
| `prometheus/podmonitor.yaml`            | Scrapes the alpha's metrics and memwatch's metrics                                  |
| `prometheus/tests/`                     | `promtool` unit tests for every alert                                               |
| `grafana/dgraph-memory.json`            | Dashboard: memory breakdown, OOM kills, captures, queries, writes, Raft, storage    |
| `memwatch/`                             | The sidecar (Go, no Dgraph imports) and its Dockerfile                              |
| `k8s/alpha-memwatch-patch.yaml`         | Patch that adds memwatch to an alpha StatefulSet                                    |
| `compose/`                              | Local stack (zero, alpha, memwatch, cAdvisor, Prometheus, Grafana) and a spike test |

## Why a sidecar

Prometheus plus the standard Kubernetes exporters already cover most of this. cAdvisor reports container memory
and OOM events, and kube-state-metrics reports limits, restarts and termination reasons. The rules and dashboard
here build on those. Three things are missing:

- **Spikes between scrapes.** An alpha can go from 800 MiB to its 2 GiB limit and get killed between two 30-second
  scrapes, so Prometheus never sees the spike.
- **Profiles from the moment of the spike.** By the time someone runs `dgraph debuginfo`, the alpha has restarted
  and the evidence is gone.
- **The memory trajectory into an OOM kill.** Nothing records it.

memwatch fills those gaps. It reads the alpha's `/proc/<pid>/status` every second. When memory crosses a threshold
or jumps quickly, it saves a heap profile, a goroutine dump and the recent timeline. When the alpha dies, it writes
out the timeline leading into the death. It also exports the peak it saw over the last minute, so Prometheus sees
spikes it would otherwise miss.

## Setup on Kubernetes

These steps assume the [Prometheus Operator](https://github.com/prometheus-operator/kube-prometheus), for example
via kube-prometheus-stack, which also runs cAdvisor scraping and kube-state-metrics.

1. **Build and push memwatch** from the repository root:

   ```bash
   docker build -f contrib/monitoring/memwatch/Dockerfile -t <registry>/memwatch:<tag> .
   docker push <registry>/memwatch:<tag>
   ```

2. **Add the sidecar.** Edit `contrib/monitoring/k8s/alpha-memwatch-patch.yaml`: set the image, and replace
   `ALPHA_STATEFULSET` and `ALPHA_CONTAINER` with your alpha StatefulSet's and container's names. Then patch the
   StatefulSet, or list the file under `patches:` in a kustomization.

   ```bash
   kubectl -n dgraph patch statefulset <alpha-statefulset> \
     --patch-file contrib/monitoring/k8s/alpha-memwatch-patch.yaml
   ```

   With the default `RollingUpdate` strategy, Kubernetes then restarts the alpha pods one at a time. With
   `OnDelete`, or a rolling-update `partition`, pods keep the old template until you delete them yourself
   (`kubectl -n dgraph delete pod <pod>`), one at a time so the group keeps quorum.

   The patch sets `shareProcessNamespace: true` so memwatch can see the alpha process. It passes the alpha's memory
   limit through the downward API and stores captures in an `emptyDir`. That survives alpha restarts, including OOM
   kills, but not pod deletion.

3. **Scrape and alert.** Edit the `release` label, selector, namespace and port name in
   `contrib/monitoring/prometheus/podmonitor.yaml` to match your setup, then apply it along with the rules:

   ```bash
   kubectl -n dgraph apply -f contrib/monitoring/prometheus/podmonitor.yaml
   kubectl -n dgraph apply -f contrib/monitoring/prometheus/dgraph-prometheusrule.yaml
   ```

   The rules find alpha containers with `container=~".*alpha.*"`. If yours is named differently, change that matcher
   in `contrib/monitoring/prometheus/dgraph-rules.yaml` and regenerate the PrometheusRule:

   ```bash
   contrib/monitoring/prometheus/gen-prometheusrule.sh > contrib/monitoring/prometheus/dgraph-prometheusrule.yaml
   ```

4. **Import the dashboard.** In Grafana, go to Dashboards, then Import, and upload
   `contrib/monitoring/grafana/dgraph-memory.json`.

## Try it locally

`compose/` runs a single-alpha cluster with a 1 GiB limit, plus memwatch, cAdvisor, Prometheus and Grafana. The
Prometheus config relabels series so the same rules file works unchanged.

```bash
cd contrib/monitoring/compose
docker compose up -d --build                                    # Dgraph v24.0.5 by default
DGRAPH_IMAGE=dgraph/dgraph:local docker compose up -d --build   # or a local build (make image-local)

N=40 ./spike.sh   # a query that triggers a memwatch capture without killing the alpha
N=60 ./spike.sh   # one that gets the alpha OOM-killed
ls captures/
```

Prometheus is at <http://localhost:9090> and Grafana at <http://localhost:3000>. `spike.sh` loads a layered graph
where every node links to every node in the next layer, then queries it N^(DEPTH+1) nodes deep. Dgraph builds the
whole result in memory before sending it, so the result size controls the spike.

On Docker Desktop, cAdvisor needs the Docker socket mount the compose file already includes. Without it, only
memwatch's and Dgraph's own metrics show up.

## memwatch

### Sampling modes

- **proc** (preferred): reads `/proc/<pid>/status` every second, giving total, anonymous, file-backed and shared
  RSS, plus `VmHWM`, the peak RSS since the alpha started. It needs a shared PID namespace: `shareProcessNamespace:
  true` in Kubernetes, or `pid: "service:<owner>"` in Compose. The alpha must not own the namespace, because when a
  namespace's first process exits, the kernel kills everything else in it. The compose stack uses a pause
  container for this, like a Kubernetes pod does.
- **http** (fallback): scrapes `process_resident_memory_bytes` from the alpha every 5 seconds. Use it when the PID
  namespace can't be shared, for example on VMs. It only sees total RSS.

The default, `--mode=auto`, uses proc mode if an alpha process shows up in `/proc` within `--discover-timeout`.
Otherwise it falls back to http mode.

Dgraph v24 runs as two processes with the same command line. A small parent (panicwrap, from the Sentry
integration) re-executes the binary and only watches the child for panics. memwatch skips any matching process
whose child also matches, so it always tracks the one doing the work. v25 runs as a single process.

### Triggers

A capture starts when any of these happen, subject to a global `--cooldown` and at most one capture at a time:

| Reason         | When                                                                                                 |
| -------------- | ---------------------------------------------------------------------------------------------------- |
| `threshold-NN` | Memory crosses NN% of `--limit-bytes` (default 70, 85, 95). A level re-arms after memory drops 5 points below it. |
| `rise`         | Memory is `--rise-bytes` above its lowest point in the last `--rise-window` (default 20% of the limit within 5s). |
| `periodic`     | Every `--periodic` (default 30m; the first one after 5m). These give a baseline for heap diffs.       |
| `restart`      | The alpha process went away or was replaced. Only the timeline is written.                            |

"Memory" means anonymous RSS in proc mode, which is what the kernel can't reclaim, and total RSS in http mode.

### What a capture contains

Each capture is a directory named `<UTC time>-<reason>` in `--out-dir`:

| File                    | Contents                                                                          |
| ----------------------- | --------------------------------------------------------------------------------- |
| `heap.pb.gz`            | `/debug/pprof/heap?gc=1`. The forced GC makes the in-use view current.             |
| `goroutine.txt`         | `/debug/pprof/goroutine?debug=2`: every goroutine's stack, including running queries |
| `jemalloc.txt`          | `/debug/jemalloc`: memory Badger allocates outside the Go heap                     |
| `metrics.txt`           | The alpha's full Prometheus metrics at that moment                                 |
| `health.json`           | `/health`, including the Dgraph version                                            |
| `proc-status.txt`       | `/proc/<pid>/status` (proc mode)                                                   |
| `proc-smaps_rollup.txt` | `/proc/<pid>/smaps_rollup`, if memwatch is allowed to read it                      |
| `timeline.csv`          | The last `--ring-size` samples (default 600, about 10 minutes at 1s)               |
| `meta.json`             | Reason, sample, limit, version, and any artifacts that failed                      |

Old captures are deleted once there are more than `--max-captures` (30) or they use more than `--max-bytes` (1Gi).
Restart dumps are deleted last.

### Getting captures

The memwatch image has no shell or `tar`, so `kubectl cp` can't copy files out of it. memwatch serves the captures
on its own port instead:

| Endpoint                            | Returns                                                     |
| ----------------------------------- | ----------------------------------------------------------- |
| `/captures`                         | One line per capture: name, size in bytes, time written     |
| `/captures.tar.gz`                  | Every capture as a gzipped tar                              |
| `/captures.tar.gz?name=<capture>`   | One capture                                                 |

On Kubernetes, port-forward to the pod and download:

```bash
kubectl -n <namespace> port-forward pod/<pod> 9095 &
curl -s localhost:9095/captures                          # see what's there
curl -sO -J localhost:9095/captures.tar.gz               # everything, saved as memwatch-captures.tar.gz
curl -sO -J "localhost:9095/captures.tar.gz?name=<capture>"
tar xzf memwatch-captures.tar.gz
```

With the Compose stack, captures are already on your machine in `compose/captures/`.

Captures hold internal details such as stack traces and metric values, but no data from the database. Treat them
like `dgraph debuginfo` output. Anyone who can reach port 9095 can download them, so restrict it with a
NetworkPolicy if that matters in your cluster, or turn the endpoints off with `--serve-captures=false`.

### Reading a capture

Start with `meta.json`, then go to whichever file answers your question:

| File            | How to read it                                                                                       |
| --------------- | ---------------------------------------------------------------------------------------------------- |
| `meta.json`     | Why the capture was taken, memory at that moment, the Dgraph version, and any files that failed       |
| `timeline.csv`  | `column -s, -t < timeline.csv`, or open it in a spreadsheet: memory second by second                  |
| `heap.pb.gz`    | `go tool pprof -http :9999 heap.pb.gz` opens it in a browser. `inuse_space` is what's held; `alloc_space` is what was allocated |
| `goroutine.txt` | Plain text. Search for `query.` or `processQuery` to find queries that were running                   |
| `jemalloc.txt`  | Plain text. A large `OutputNode.Encoder` figure means a query response was being built                |
| `metrics.txt`   | Every alpha metric at that moment. Grep it for the value you need                                    |

### Flags

Every flag can also be set as an environment variable `MEMWATCH_<FLAG>`, with dashes turned into underscores, for
example `MEMWATCH_LIMIT_BYTES`. A flag on the command line wins.

| Flag                 | Default                 | Meaning                                                                      |
| -------------------- | ----------------------- | ---------------------------------------------------------------------------- |
| `--alpha-url`        | `http://localhost:8080` | The alpha's HTTP port                                                        |
| `--limit-bytes`      | none                    | Container memory limit. Without it, threshold captures are off.              |
| `--thresholds`       | `0.70,0.85,0.95`        | Fractions of the limit that trigger captures                                 |
| `--rise-bytes`       | `auto`                  | Growth within `--rise-window` that triggers a capture. `0` disables.         |
| `--rise-window`      | `5s`                    |                                                                              |
| `--periodic`         | `30m`                   | Baseline capture interval. `0` disables.                                     |
| `--cooldown`         | `2m`                    | Minimum time between captures                                                |
| `--out-dir`          | `/captures`             |                                                                              |
| `--max-captures`     | `30`                    |                                                                              |
| `--max-bytes`        | `1Gi`                   |                                                                              |
| `--mode`             | `auto`                  | `auto`, `proc` or `http`                                                     |
| `--interval`         | `1s`                    | Sampling interval in proc mode                                               |
| `--http-interval`    | `5s`                    | Sampling interval in http mode                                               |
| `--window`           | `1m`                    | Window for `memwatch_rss_window_max_bytes`. Keep it above the scrape interval. |
| `--ring-size`        | `600`                   | Samples kept for `timeline.csv`                                              |
| `--proc-match`       | `dgraph,alpha`          | Words that identify the alpha's command line                                 |
| `--listen`           | `:9095`                 | Serves `/metrics`, `/healthz` and the capture downloads                      |
| `--serve-captures`   | `true`                  | Serve `/captures` and `/captures.tar.gz`                                     |

### Metrics

| Metric                                    | Meaning                                                           |
| ----------------------------------------- | ----------------------------------------------------------------- |
| `memwatch_rss_bytes{type}`                | Current RSS: `total`, plus `anon`, `file` and `shmem` in proc mode |
| `memwatch_rss_window_max_bytes{type}`     | Peak `total` or `anon` RSS over `--window`                        |
| `memwatch_vm_hwm_bytes`                   | Peak RSS since the alpha started (proc mode)                      |
| `memwatch_limit_bytes`                    | The configured limit                                              |
| `memwatch_captures_total{reason}`         | Captures written                                                  |
| `memwatch_capture_errors_total`           | Artifacts that couldn't be saved                                  |
| `memwatch_target_restarts_total`          | Alpha restarts seen                                               |
| `memwatch_target_up`                      | Whether the last sample succeeded                                 |
| `memwatch_last_capture_timestamp_seconds` | Time of the last capture                                          |

## Triage

Start from the alert, and check the dashboard's memory panels to see which kind of memory is growing. Then
download the captures (see [Getting captures](#getting-captures)) and work through them as below.

**An alpha was OOM-killed.** Open the newest `*-restart` directory. Its `timeline.csv` shows whether memory climbed
for minutes or jumped in a second or two. Any `threshold-*` or `rise` capture from just before it was taken during
the climb. Then check the node's kernel log for the kill itself:

```bash
journalctl -k | grep -i "killed process"   # or dmesg -T on the node
```

The line lists `anon-rss` and `file-rss` for the killed process. High `anon-rss` means heap or off-heap
allocations. [node-problem-detector](https://github.com/kubernetes/node-problem-detector) can turn those lines into
Kubernetes events if you want them collected automatically.

**A short spike.** This is usually one expensive query. `goroutine.txt` shows what was running; look for goroutines
in `query.(*SubGraph)` and `edgraph.processQuery`. Much of a large query's memory is outside the Go heap. Dgraph
encodes the JSON response in memory it allocates itself, so `jemalloc.txt` shows it under the `OutputNode.Encoder`
tag. A large figure there means a response was being built. The heap profile shows the Go-heap side:

```bash
go tool pprof -top -sample_index=inuse_space captures/<dir>/heap.pb.gz
```

To see what was allocated between two captures, for example the last periodic one and the spike, diff the
cumulative allocations:

```bash
go tool pprof -top -sample_index=alloc_space -diff_base captures/<earlier>/heap.pb.gz captures/<spike>/heap.pb.gz
```

**Slow growth.** Compare two periodic captures hours apart, looking at what's still live:

```bash
go tool pprof -top -sample_index=inuse_space -diff_base captures/<early>/heap.pb.gz captures/<late>/heap.pb.gz
```

If the Go heap is flat but RSS keeps rising, compare `jemalloc.txt` across the same captures. That shows growth
outside the Go heap.

### Finding the query behind a spike

What Dgraph can log depends on the version:

- **v25.0.0 and later** can log every request with its query text when it starts, so the culprit is logged even if
  the alpha dies before finishing it. Turn it on at runtime:

  ```graphql
  mutation { config(input: { logDQLRequest: true }) { response { code } } }
  ```

- **v25.3.0 and later** can also log slow queries with timings, via `--feature-flags
  "log-slow-query-threshold=2s"`. These lines are written when a query finishes, so a query that kills the alpha
  never gets one.
- **v24** doesn't log query text: the `logDQLRequest` line only says "Finished a query that started at". Log on
  the client side instead. Every GraphQL response includes `extensions.touched_uids`, the number of UIDs processed.
  Log it per request with the operation name, variables and response size, and log requests that fail with a
  connection error, since the request that kills an alpha never gets a response.

## Known memory traps

- **v25's default cache preallocates about 800 MiB.** The default `--cache "size-mb=4096"` sizes the cache's
  bookkeeping structures up front, so a v25 alpha in a 1 GiB container sits near its limit before serving a single
  query. v24's default was `size-mb=1024`. Size `--cache` to fit the container.
- **`GOMEMLIMIT` at or above the container limit.** It only bounds the Go heap, so the container gets killed before
  the garbage collector reacts. `DgraphGoMemLimitAboveContainerLimit` catches this.
- **Page cache counted as usage.** See below.

## Page cache

Badger memory-maps its SST, value-log and memtable files, so an alpha's container usage includes page cache for
whatever it has read recently. That page cache:

- differs between replicas, depending on which queries and compactions each one ran
- counts toward `container_memory_working_set_bytes` and the cgroup's `memory.peak`
- is reclaimed by the kernel before an OOM kill when it's clean, which nearly all of Badger's read-side pages are

Neither Dgraph nor Badger has a setting to limit it, and that isn't needed. The rules alert on anonymous memory
(`DgraphAnonMemoryNearLimit`) as the real OOM risk. `DgraphWorkingSetNearLimit` only fires when the working set is
high and anonymous memory is not.

## Alerts

| Alert                                  | Fires when                                                                  |
| -------------------------------------- | --------------------------------------------------------------------------- |
| `DgraphAlphaOOMKilled`                 | cAdvisor reports an OOM event, or kube-state-metrics an `OOMKilled` restart |
| `DgraphAnonMemoryNearLimit`            | Anonymous memory above 80% of the limit for 2m                              |
| `DgraphWorkingSetNearLimit`            | Working set above 90% for 10m while anonymous memory is at most 80%         |
| `DgraphMemorySpike`                    | memwatch's window peak above 90% of the limit                               |
| `DgraphMemoryGrowthTrend`              | Anonymous memory on a 12h trend to exceed the limit within 48h, for 1h      |
| `DgraphGoMemLimitAboveContainerLimit`  | `GOMEMLIMIT` is set at or above the container limit                         |
| `DgraphFrequentRestarts`               | More than 3 restarts in an hour                                             |
| `DgraphPendingQueriesHigh`             | More than 100 requests in flight for 5m                                     |
| `DgraphQueryLatencyHigh`               | p99 query latency above 5s for 10m                                          |
| `DgraphNoRaftLeader`                   | The alpha sees no leader for its group for 2m                               |
| `MemwatchCaptureTaken`                 | memwatch saved a capture other than a periodic one (info)                   |
| `MemwatchTargetDown`                   | memwatch hasn't been able to sample the alpha for 10m (info)                |

## Development

```bash
go test ./contrib/monitoring/memwatch/...

cd contrib/monitoring/prometheus
promtool check rules dgraph-rules.yaml
promtool test rules tests/dgraph-rules.test.yaml
./gen-prometheusrule.sh > dgraph-prometheusrule.yaml   # after editing the rules
```

Without a local `promtool`, prefix those commands with
`docker run --rm -v "$PWD":/w -w /w --entrypoint promtool prom/prometheus`.
