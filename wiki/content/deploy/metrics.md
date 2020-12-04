+++
date = "2017-03-20T22:25:17+11:00"
title = "Metrics"
weight = 15
[menu.main]
    parent = "deploy"
+++


Dgraph database provides metrics on disk activity, memory usage, Dgraph instance
activity and server node health; along with built-in metrics provided by Go.
Dgraph metrics follow the
[metric and label conventions for the Prometheus](https://prometheus.io/docs/practices/naming/)
monitoring and alerting toolkit.

## Disk metrics

Disk metrics let you track the disk activity of the Dgraph process. Dgraph does
not interact directly with the filesystem. Instead it relies on
[Badger](https://github.com/dgraph-io/badger) to read from and write to disk.

 Metrics                          	 | Description
 -------                          	 | -----------
 `badger_v2_disk_reads_total`        | Total count of disk reads in Badger.
 `badger_v2_disk_writes_total`       | Total count of disk writes in Badger.
 `badger_v2_gets_total`              | Total count of calls to Badger's `get`.
 `badger_v2_memtable_gets_total`     | Total count of memtable accesses to Badger's `get`.
 `badger_v2_puts_total`              | Total count of calls to Badger's `put`.
 `badger_v2_read_bytes`              | Total bytes read from Badger.
 `badger_v2_written_bytes`           | Total bytes written to Badger.

## Memory metrics

Memory metrics let you track the memory usage of the Dgraph process. The `idle`
and `inuse` metrics give you a better sense of the active memory usage of the
Dgraph process. The process memory metric shows the memory usage as measured by
the operating system.

By looking at all three metrics you can see how much memory a Dgraph process is
holding from the operating system and how much is actively in use.

 Metrics                          | Description
 -------                          | -----------
 `dgraph_memory_idle_bytes`       | Estimated amount of memory that is being held idle that could be reclaimed by the OS.
 `dgraph_memory_inuse_bytes`      | Total memory usage in bytes (sum of heap usage and stack usage).
 `dgraph_memory_proc_bytes`       | Total memory usage in bytes of the Dgraph process. On Linux/macOS, this metric is equivalent to resident set size. On Windows, this metric is equivalent to [Go's runtime.ReadMemStats](https://golang.org/pkg/runtime/#ReadMemStats).

## Activity Metrics

Activity metrics let you track the mutations, queries, and proposals of a Dgraph
instance.

 Metrics                                            | Description
 -------                                            | -----------
 `go_goroutines`                                    | Total number of Goroutines currently running in Dgraph.
 `dgraph_active_mutations_total`                    | Total number of mutations currently running.
 `dgraph_pending_proposals_total`                   | Total pending Raft proposals.
 `dgraph_pending_queries_total`                     | Total number of queries in progress.
 `dgraph_num_queries_total{method="Server.Mutate"}` | Total number of mutations run in Dgraph.
 `dgraph_num_queries_total{method="Server.Query"}`  | Total number of queries run in Dgraph.

## Health Metrics

Health metrics let you check the health of a Dgraph Alpha server node.

 Metrics                          | Description
 -------                          | -----------
 `dgraph_alpha_health_status`     | **Only applicable to Dgraph Alpha**. Value is 1 when the Alpha node is ready to accept requests; otherwise 0.
 `dgraph_max_assigned_ts`         | **Only applicable to Dgraph Alpha**. This shows the latest max assigned timestamp. All Alpha nodes within the same Alpha group should show the same timestamp if they are in sync.
 `dgraph_txn_aborts_total`        | **Only applicable to Dgraph Alpha**. Shows the total number of transaction aborts that have occurred on the Alpha node.

## Go Metrics

Go's built-in metrics may also be useful to measure memory usage and garbage
collection time.

 Metrics                        | Description
 -------                        | -----------
 `go_memstats_gc_cpu_fraction`  | The fraction of this program's available CPU time used by the GC since the program started.
 `go_memstats_heap_idle_bytes`  | Number of heap bytes waiting to be used.
 `go_memstats_heap_inuse_bytes` | Number of heap bytes that are in use.
