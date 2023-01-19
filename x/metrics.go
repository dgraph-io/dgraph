/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import (
	"context"
	"expvar"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"contrib.go.opencensus.io/exporter/jaeger"
	oc_prom "contrib.go.opencensus.io/exporter/prometheus"
	datadog "github.com/DataDog/opencensus-go-exporter-datadog"
	"github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	ostats "go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/ristretto/z"
)

var (
	// Cumulative metrics.

	// NumQueries is the total number of queries processed so far.
	NumQueries = ostats.Int64("num_queries_total",
		"Total number of queries", ostats.UnitDimensionless)
	// NumMutations is the total number of mutations processed so far.
	NumMutations = ostats.Int64("num_mutations_total",
		"Total number of mutations", ostats.UnitDimensionless)
	// NumEdges is the total number of edges created so far.
	NumEdges = ostats.Int64("num_edges_total",
		"Total number of edges created", ostats.UnitDimensionless)
	// NumBackups is the number of backups requested
	NumBackups = ostats.Int64("num_backups_total",
		"Total number of backups requested", ostats.UnitDimensionless)
	// NumBackupsSuccess is the number of backups successfully completed
	NumBackupsSuccess = ostats.Int64("num_backups_success_total",
		"Total number of backups completed", ostats.UnitDimensionless)
	// NumBackupsFailed is the number of backups failed
	NumBackupsFailed = ostats.Int64("num_backups_failed_total",
		"Total number of backups failed", ostats.UnitDimensionless)
	// LatencyMs is the latency of the various Dgraph operations.
	LatencyMs = ostats.Float64("latency",
		"Latency of the various methods", ostats.UnitMilliseconds)

	// Point-in-time metrics.

	// PendingQueries records the current number of pending queries.
	PendingQueries = ostats.Int64("pending_queries_total",
		"Number of pending queries", ostats.UnitDimensionless)
	// PendingProposals records the current number of pending RAFT proposals.
	PendingProposals = ostats.Int64("pending_proposals_total",
		"Number of pending proposals", ostats.UnitDimensionless)
	// PendingBackups records if a backup is currently in progress
	PendingBackups = ostats.Int64("pending_backups_total",
		"Number of backups", ostats.UnitDimensionless)
	// MemoryAlloc records the amount of memory allocated via jemalloc
	MemoryAlloc = ostats.Int64("memory_alloc_bytes",
		"Amount of memory allocated", ostats.UnitBytes)
	// MemoryInUse records the current amount of used memory by Dgraph.
	MemoryInUse = ostats.Int64("memory_inuse_bytes",
		"Amount of memory in use", ostats.UnitBytes)
	// MemoryIdle records the amount of memory held by the runtime but not in-use by Dgraph.
	MemoryIdle = ostats.Int64("memory_idle_bytes",
		"Amount of memory in idle spans", ostats.UnitBytes)
	// MemoryProc records the amount of memory used in processes.
	MemoryProc = ostats.Int64("memory_proc_bytes",
		"Amount of memory used in processes", ostats.UnitBytes)
	// DiskFree records the number of bytes free on the disk
	DiskFree = ostats.Int64("disk_free_bytes",
		"Total number of bytes free on disk", ostats.UnitBytes)
	// DiskUsed records the number of bytes free on the disk
	DiskUsed = ostats.Int64("disk_used_bytes",
		"Total number of bytes used on disk", ostats.UnitBytes)
	// DiskTotal records the number of bytes free on the disk
	DiskTotal = ostats.Int64("disk_total_bytes",
		"Total number of bytes on disk", ostats.UnitBytes)
	// ActiveMutations is the current number of active mutations.
	ActiveMutations = ostats.Int64("active_mutations_total",
		"Number of active mutations", ostats.UnitDimensionless)
	// AlphaHealth status records the current health of the alphas.
	AlphaHealth = ostats.Int64("alpha_health_status",
		"Status of the alphas", ostats.UnitDimensionless)
	// RaftAppliedIndex records the latest applied RAFT index.
	RaftAppliedIndex = ostats.Int64("raft_applied_index",
		"Latest applied Raft index", ostats.UnitDimensionless)
	RaftApplyCh = ostats.Int64("raft_applych_size",
		"Number of proposals in Raft apply channel", ostats.UnitDimensionless)
	RaftPendingSize = ostats.Int64("pending_proposal_bytes",
		"Size of Raft pending proposal", ostats.UnitBytes)
	// MaxAssignedTs records the latest max assigned timestamp.
	MaxAssignedTs = ostats.Int64("max_assigned_ts",
		"Latest max assigned timestamp", ostats.UnitDimensionless)
	// TxnCommits records count of committed transactions.
	TxnCommits = ostats.Int64("txn_commits_total",
		"Number of transaction commits", ostats.UnitDimensionless)
	// TxnDiscards records count of discarded transactions by the client.
	TxnDiscards = ostats.Int64("txn_discards_total",
		"Number of transaction discards by the client", ostats.UnitDimensionless)
	// TxnAborts records count of aborted transactions by the server.
	TxnAborts = ostats.Int64("txn_aborts_total",
		"Number of transaction aborts by the server", ostats.UnitDimensionless)
	// PBlockHitRatio records the hit ratio of posting store block cache.
	PBlockHitRatio = ostats.Float64("hit_ratio_postings_block",
		"Hit ratio of p store block cache", ostats.UnitDimensionless)
	// PIndexHitRatio records the hit ratio of posting store index cache.
	PIndexHitRatio = ostats.Float64("hit_ratio_postings_index",
		"Hit ratio of p store index cache", ostats.UnitDimensionless)
	// PLCacheHitRatio records the hit ratio of posting list cache.
	PLCacheHitRatio = ostats.Float64("hit_ratio_posting_cache",
		"Hit ratio of posting list cache", ostats.UnitDimensionless)
	// RaftHasLeader records whether this instance has a leader
	RaftHasLeader = ostats.Int64("raft_has_leader",
		"Whether or not a leader exists for the group", ostats.UnitDimensionless)
	// RaftIsLeader records whether this instance is the leader
	RaftIsLeader = ostats.Int64("raft_is_leader",
		"Whether or not this instance is the leader of the group", ostats.UnitDimensionless)
	// RaftLeaderChanges records the total number of leader changes seen.
	RaftLeaderChanges = ostats.Int64("raft_leader_changes_total",
		"Total number of leader changes seen", ostats.UnitDimensionless)

	// Conf holds the metrics config.
	// TODO: Request statistics, latencies, 500, timeouts
	Conf *expvar.Map

	// Tag keys.

	// KeyGroup is the tag key used to record the group for Raft metrics.
	KeyGroup, _ = tag.NewKey("group")

	// KeyStatus is the tag key used to record the status of the server.
	KeyStatus, _ = tag.NewKey("status")
	// KeyMethod is the tag key used to record the method (e.g read or mutate).
	KeyMethod, _ = tag.NewKey("method")

	// KeyDirType is the tag key used to record the group for FileSystem metrics
	KeyDirType, _ = tag.NewKey("dir")

	// Tag values.

	// TagValueStatusOK is the tag value used to signal a successful operation.
	TagValueStatusOK = "ok"
	// TagValueStatusError is the tag value used to signal an unsuccessful operation.
	TagValueStatusError = "error"

	defaultLatencyMsDistribution = view.Distribution(
		0, 0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16,
		20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500,
		650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)

	// Use this tag for the metric view if it needs status or method granularity.
	// Metrics would be viewed separately for different tag values.
	allTagKeys = []tag.Key{
		KeyStatus, KeyMethod,
	}

	allRaftKeys = []tag.Key{KeyGroup}

	allFSKeys = []tag.Key{KeyDirType}

	allViews = []*view.View{
		{
			Name:        LatencyMs.Name(),
			Measure:     LatencyMs,
			Description: LatencyMs.Description(),
			Aggregation: defaultLatencyMsDistribution,
			TagKeys:     allTagKeys,
		},
		{
			Name:        NumQueries.Name(),
			Measure:     NumQueries,
			Description: NumQueries.Description(),
			Aggregation: view.Count(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        NumEdges.Name(),
			Measure:     NumEdges,
			Description: NumEdges.Description(),
			Aggregation: view.Count(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        NumBackups.Name(),
			Measure:     NumBackups,
			Description: NumBackups.Description(),
			Aggregation: view.Count(),
			TagKeys:     nil,
		},
		{
			Name:        NumBackupsSuccess.Name(),
			Measure:     NumBackupsSuccess,
			Description: NumBackupsSuccess.Description(),
			Aggregation: view.Count(),
			TagKeys:     nil,
		},
		{
			Name:        NumBackupsFailed.Name(),
			Measure:     NumBackupsFailed,
			Description: NumBackupsFailed.Description(),
			Aggregation: view.Count(),
			TagKeys:     nil,
		},
		{
			Name:        TxnCommits.Name(),
			Measure:     TxnCommits,
			Description: TxnCommits.Description(),
			Aggregation: view.Count(),
			TagKeys:     nil,
		},
		{
			Name:        TxnDiscards.Name(),
			Measure:     TxnDiscards,
			Description: TxnDiscards.Description(),
			Aggregation: view.Count(),
			TagKeys:     nil,
		},
		{
			Name:        TxnAborts.Name(),
			Measure:     TxnAborts,
			Description: TxnAborts.Description(),
			Aggregation: view.Count(),
			TagKeys:     nil,
		},
		{
			Name:        ActiveMutations.Name(),
			Measure:     ActiveMutations,
			Description: ActiveMutations.Description(),
			Aggregation: view.Sum(),
			TagKeys:     nil,
		},

		// Last value aggregations
		{
			Name:        PendingQueries.Name(),
			Measure:     PendingQueries,
			Description: PendingQueries.Description(),
			Aggregation: view.Sum(),
			TagKeys:     nil,
		},
		{
			Name:        PendingProposals.Name(),
			Measure:     PendingProposals,
			Description: PendingProposals.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     nil,
		},
		{
			Name:        PendingBackups.Name(),
			Measure:     PendingBackups,
			Description: PendingBackups.Description(),
			Aggregation: view.Sum(),
			TagKeys:     nil,
		},
		{
			Name:        MemoryAlloc.Name(),
			Measure:     MemoryAlloc,
			Description: MemoryAlloc.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        MemoryInUse.Name(),
			Measure:     MemoryInUse,
			Description: MemoryInUse.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        MemoryIdle.Name(),
			Measure:     MemoryIdle,
			Description: MemoryIdle.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        MemoryProc.Name(),
			Measure:     MemoryProc,
			Description: MemoryProc.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        DiskFree.Name(),
			Measure:     DiskFree,
			Description: DiskFree.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allFSKeys,
		},
		{
			Name:        DiskUsed.Name(),
			Measure:     DiskUsed,
			Description: DiskUsed.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allFSKeys,
		},
		{
			Name:        DiskTotal.Name(),
			Measure:     DiskTotal,
			Description: DiskTotal.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allFSKeys,
		},
		{
			Name:        AlphaHealth.Name(),
			Measure:     AlphaHealth,
			Description: AlphaHealth.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     nil,
		},
		{
			Name:        PBlockHitRatio.Name(),
			Measure:     PBlockHitRatio,
			Description: PBlockHitRatio.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        PIndexHitRatio.Name(),
			Measure:     PIndexHitRatio,
			Description: PIndexHitRatio.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        PLCacheHitRatio.Name(),
			Measure:     PLCacheHitRatio,
			Description: PLCacheHitRatio.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        MaxAssignedTs.Name(),
			Measure:     MaxAssignedTs,
			Description: MaxAssignedTs.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		// Raft metrics
		{
			Name:        RaftAppliedIndex.Name(),
			Measure:     RaftAppliedIndex,
			Description: RaftAppliedIndex.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allRaftKeys,
		},
		{
			Name:        RaftApplyCh.Name(),
			Measure:     RaftApplyCh,
			Description: RaftApplyCh.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allRaftKeys,
		},
		{
			Name:        RaftPendingSize.Name(),
			Measure:     RaftPendingSize,
			Description: RaftPendingSize.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allRaftKeys,
		},
		{
			Name:        RaftHasLeader.Name(),
			Measure:     RaftHasLeader,
			Description: RaftHasLeader.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allRaftKeys,
		},
		{
			Name:        RaftIsLeader.Name(),
			Measure:     RaftIsLeader,
			Description: RaftIsLeader.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allRaftKeys,
		},
		{
			Name:        RaftLeaderChanges.Name(),
			Measure:     RaftLeaderChanges,
			Description: RaftLeaderChanges.Description(),
			Aggregation: view.Count(),
			TagKeys:     allRaftKeys,
		},
	}
)

func init() {
	Conf = expvar.NewMap("dgraph_config")

	ctx := MetricsContext()
	go func() {
		var v string
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			v = TagValueStatusOK
			if err := HealthCheck(); err != nil {
				v = TagValueStatusError
			}
			cctx, _ := tag.New(ctx, tag.Upsert(KeyStatus, v))
			// TODO: Do we need to set health to zero, or would this tag be sufficient to
			// indicate if Alpha is up but HealthCheck is failing.
			ostats.Record(cctx, AlphaHealth.M(1))
		}
	}()

	CheckfNoTrace(view.Register(allViews...))

	prometheus.MustRegister(NewBadgerCollector())

	pe, err := oc_prom.NewExporter(oc_prom.Options{
		// DefaultRegisterer includes a ProcessCollector for process_* metrics, a GoCollector for
		// go_* metrics, and the badger_* metrics.
		Registry:  prometheus.DefaultRegisterer.(*prometheus.Registry),
		Namespace: "dgraph",
		OnError:   func(err error) { glog.Errorf("%v", err) },
	})
	Checkf(err, "Failed to create OpenCensus Prometheus exporter: %v", err)
	view.RegisterExporter(pe)

	// Exposing metrics at /metrics, which is the usual standard, as well as at the old endpoint
	http.Handle("/metrics", pe)
	http.Handle("/debug/prometheus_metrics", pe)
}

// NewBadgerCollector returns a prometheus Collector for Badger metrics from expvar.
func NewBadgerCollector() prometheus.Collector {
	return prometheus.NewExpvarCollector(map[string]*prometheus.Desc{
		"badger_v3_disk_reads_total": prometheus.NewDesc(
			"badger_disk_reads_total",
			"Number of cumulative reads by Badger",
			nil, nil,
		),
		"badger_v3_disk_writes_total": prometheus.NewDesc(
			"badger_disk_writes_total",
			"Number of cumulative writes by Badger",
			nil, nil,
		),
		"badger_v3_read_bytes": prometheus.NewDesc(
			"badger_read_bytes",
			"Number of cumulative bytes read by Badger",
			nil, nil,
		),
		"badger_v3_written_bytes": prometheus.NewDesc(
			"badger_written_bytes",
			"Number of cumulative bytes written by Badger",
			nil, nil,
		),
		"badger_v3_lsm_level_gets_total": prometheus.NewDesc(
			"badger_lsm_level_gets_total",
			"Total number of LSM gets",
			[]string{"level"}, nil,
		),
		"badger_v3_lsm_bloom_hits_total": prometheus.NewDesc(
			"badger_lsm_bloom_hits_total",
			"Total number of LSM bloom hits",
			[]string{"level"}, nil,
		),
		"badger_v3_gets_total": prometheus.NewDesc(
			"badger_gets_total",
			"Total number of gets",
			nil, nil,
		),
		"badger_v3_puts_total": prometheus.NewDesc(
			"badger_puts_total",
			"Total number of puts",
			nil, nil,
		),
		"badger_v3_memtable_gets_total": prometheus.NewDesc(
			"badger_memtable_gets_total",
			"Total number of memtable gets",
			nil, nil,
		),
		"badger_v3_lsm_size_bytes": prometheus.NewDesc(
			"badger_lsm_size_bytes",
			"Size of the LSM in bytes",
			[]string{"dir"}, nil,
		),
		"badger_v3_vlog_size_bytes": prometheus.NewDesc(
			"badger_vlog_size_bytes",
			"Size of the value log in bytes",
			[]string{"dir"}, nil,
		),
	})
}

// MetricsContext returns a context with tags that are useful for
// distinguishing the state of the running system.
// This context will be used to derive other contexts.
func MetricsContext() context.Context {
	// At the beginning add some distinguishing information
	// to the context as tags that will be propagated when
	// collecting metrics.
	return context.Background()
}

// WithMethod returns a new updated context with the tag KeyMethod set to the given value.
func WithMethod(parent context.Context, method string) context.Context {
	ctx, err := tag.New(parent, tag.Upsert(KeyMethod, method))
	Check(err)
	return ctx
}

// SinceMs returns the time since startTime in milliseconds (as a float).
func SinceMs(startTime time.Time) float64 {
	return float64(time.Since(startTime)) / 1e6
}

// RegisterExporters sets up the services to which metrics will be exported.
func RegisterExporters(conf *viper.Viper, service string) {
	if traceFlag := conf.GetString("trace"); len(traceFlag) > 0 {
		t := z.NewSuperFlag(traceFlag).MergeAndCheckDefault(TraceDefaults)
		if collector := t.GetString("jaeger"); len(collector) > 0 {
			// Port details: https://www.jaegertracing.io/docs/getting-started/
			// Default collectorEndpointURI := "http://localhost:14268"
			je, err := jaeger.NewExporter(jaeger.Options{
				Endpoint:    collector,
				ServiceName: service,
			})
			if err != nil {
				log.Fatalf("Failed to create the Jaeger exporter: %v", err)
			}
			// And now finally register it as a Trace Exporter
			trace.RegisterExporter(je)
		}
		if collector := t.GetString("datadog"); len(collector) > 0 {
			exporter, err := datadog.NewExporter(datadog.Options{
				Service:   service,
				TraceAddr: collector,
			})
			if err != nil {
				log.Fatal(err)
			}

			trace.RegisterExporter(exporter)

			// For demoing purposes, always sample.
			trace.ApplyConfig(trace.Config{
				DefaultSampler: trace.AlwaysSample(),
			})
		}
	}

	// Exclusively for stats, metrics, etc. Not for tracing.
	// var views = append(ocgrpc.DefaultServerViews, ocgrpc.DefaultClientViews...)
	// if err := view.Register(views...); err != nil {
	// 	glog.Fatalf("Unable to register OpenCensus stats: %v", err)
	// }
}

// MonitorCacheHealth periodically monitors the cache metrics and reports if
// there is high contention in the cache.
func MonitorCacheHealth(db *badger.DB, closer *z.Closer) {
	defer closer.Done()

	record := func(ct string) {
		switch ct {
		case "pstore-block":
			metrics := db.BlockCacheMetrics()
			ostats.Record(context.Background(), PBlockHitRatio.M(metrics.Ratio()))
		case "pstore-index":
			metrics := db.IndexCacheMetrics()
			ostats.Record(context.Background(), PIndexHitRatio.M(metrics.Ratio()))
		default:
			panic("invalid cache type")
		}
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			record("pstore-block")
			record("pstore-index")
		case <-closer.HasBeenClosed():
			return
		}
	}
}

func MonitorMemoryMetrics(lc *z.Closer) {
	defer lc.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	fastTicker := time.NewTicker(time.Second)
	defer fastTicker.Stop()

	update := func() {
		// ReadMemStats stops the world which is expensive especially when the
		// heap is large. So don't call it too frequently. Calling it every
		// minute is OK.
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)

		inUse := ms.HeapInuse + ms.StackInuse
		// From runtime/mstats.go:
		// HeapIdle minus HeapReleased estimates the amount of memory
		// that could be returned to the OS, but is being retained by
		// the runtime so it can grow the heap without requesting more
		// memory from the OS. If this difference is significantly
		// larger than the heap size, it indicates there was a recent
		// transient spike in live heap size.
		idle := ms.HeapIdle - ms.HeapReleased

		ostats.Record(context.Background(),
			MemoryInUse.M(int64(inUse)),
			MemoryIdle.M(int64(idle)),
			MemoryProc.M(int64(getMemUsage())))
	}
	updateAlloc := func() {
		ostats.Record(context.Background(), MemoryAlloc.M(z.NumAllocBytes()))
	}
	// Call update immediately so that Dgraph reports memory stats without
	// having to wait for the first tick.
	update()
	updateAlloc()

	for {
		select {
		case <-lc.HasBeenClosed():
			return
		case <-fastTicker.C:
			updateAlloc()
		case <-ticker.C:
			update()
		}
	}
}

func getMemUsage() int {
	if runtime.GOOS != "linux" {
		pid := os.Getpid()
		cmd := fmt.Sprintf("ps -ao rss,pid | grep %v", pid)
		c1, err := exec.Command("bash", "-c", cmd).Output()
		if err != nil {
			// In case of error running the command, resort to go way
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			megs := ms.Alloc
			return int(megs)
		}

		rss := strings.Split(string(c1), " ")[0]
		kbs, err := strconv.Atoi(rss)
		if err != nil {
			return 0
		}

		megs := kbs << 10
		return megs
	}

	contents, err := ioutil.ReadFile("/proc/self/stat")
	if err != nil {
		glog.Errorf("Can't read the proc file. Err: %v\n", err)
		return 0
	}

	cont := strings.Split(string(contents), " ")
	// 24th entry of the file is the RSS which denotes the number of pages
	// used by the process.
	if len(cont) < 24 {
		glog.Errorln("Error in RSS from stat")
		return 0
	}

	rss, err := strconv.Atoi(cont[23])
	if err != nil {
		glog.Errorln(err)
		return 0
	}

	return rss * os.Getpagesize()
}

func JemallocHandler(w http.ResponseWriter, r *http.Request) {
	AddCorsHeaders(w)

	na := z.NumAllocBytes()
	fmt.Fprintf(w, "Num Allocated Bytes: %s [%d]\n",
		humanize.IBytes(uint64(na)), na)
	fmt.Fprintf(w, "Allocators:\n%s\n", z.Allocators())
	fmt.Fprintf(w, "%s\n", z.Leaks())
}
