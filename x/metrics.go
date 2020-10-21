/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"log"
	"net/http"
	"time"

	"go.opencensus.io/trace"

	"contrib.go.opencensus.io/exporter/jaeger"
	oc_prom "contrib.go.opencensus.io/exporter/prometheus"
	datadog "github.com/DataDog/opencensus-go-exporter-datadog"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	// Cumulative metrics.

	// NumQueries is the total number of queries processed so far.
	NumQueries = stats.Int64("num_queries_total",
		"Total number of queries", stats.UnitDimensionless)
	// NumMutations is the total number of mutations processed so far.
	NumMutations = stats.Int64("num_mutations_total",
		"Total number of mutations", stats.UnitDimensionless)
	// NumEdges is the total number of edges created so far.
	NumEdges = stats.Int64("num_edges_total",
		"Total number of edges created", stats.UnitDimensionless)
	// LatencyMs is the latency of the various Dgraph operations.
	LatencyMs = stats.Float64("latency",
		"Latency of the various methods", stats.UnitMilliseconds)

	// Point-in-time metrics.

	// PendingQueries records the current number of pending queries.
	PendingQueries = stats.Int64("pending_queries_total",
		"Number of pending queries", stats.UnitDimensionless)
	// PendingProposals records the current number of pending RAFT proposals.
	PendingProposals = stats.Int64("pending_proposals_total",
		"Number of pending proposals", stats.UnitDimensionless)
	// MemoryInUse records the current amount of used memory by Dgraph.
	MemoryInUse = stats.Int64("memory_inuse_bytes",
		"Amount of memory in use", stats.UnitBytes)
	// MemoryIdle records the amount of memory held by the runtime but not in-use by Dgraph.
	MemoryIdle = stats.Int64("memory_idle_bytes",
		"Amount of memory in idle spans", stats.UnitBytes)
	// MemoryProc records the amount of memory used in processes.
	MemoryProc = stats.Int64("memory_proc_bytes",
		"Amount of memory used in processes", stats.UnitBytes)
	// ActiveMutations is the current number of active mutations.
	ActiveMutations = stats.Int64("active_mutations_total",
		"Number of active mutations", stats.UnitDimensionless)
	// AlphaHealth status records the current health of the alphas.
	AlphaHealth = stats.Int64("alpha_health_status",
		"Status of the alphas", stats.UnitDimensionless)
	// RaftAppliedIndex records the latest applied RAFT index.
	RaftAppliedIndex = stats.Int64("raft_applied_index",
		"Latest applied Raft index", stats.UnitDimensionless)
	// MaxAssignedTs records the latest max assigned timestamp.
	MaxAssignedTs = stats.Int64("max_assigned_ts",
		"Latest max assigned timestamp", stats.UnitDimensionless)
	// TxnAborts records count of aborted transactions.
	TxnAborts = stats.Int64("txn_aborts_total",
		"Number of transaction aborts", stats.UnitDimensionless)
	// PBlockHitRatio records the hit ratio of posting store block cache.
	PBlockHitRatio = stats.Float64("hit_ratio_postings_block",
		"Hit ratio of p store block cache", stats.UnitDimensionless)
	// PIndexHitRatio records the hit ratio of posting store index cache.
	PIndexHitRatio = stats.Float64("hit_ratio_postings_index",
		"Hit ratio of p store index cache", stats.UnitDimensionless)
	// PLCacheHitRatio records the hit ratio of posting list cache.
	PLCacheHitRatio = stats.Float64("hit_ratio_posting_cache",
		"Hit ratio of posting list cache", stats.UnitDimensionless)

	// Conf holds the metrics config.
	// TODO: Request statistics, latencies, 500, timeouts
	Conf *expvar.Map

	// Tag keys.

	// KeyStatus is the tag key used to record the status of the server.
	KeyStatus, _ = tag.NewKey("status")
	// KeyMethod is the tag key used to record the method (e.g read or mutate).
	KeyMethod, _ = tag.NewKey("method")

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
			Name:        RaftAppliedIndex.Name(),
			Measure:     RaftAppliedIndex,
			Description: RaftAppliedIndex.Description(),
			Aggregation: view.Count(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        MaxAssignedTs.Name(),
			Measure:     MaxAssignedTs,
			Description: MaxAssignedTs.Description(),
			Aggregation: view.Count(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        TxnAborts.Name(),
			Measure:     TxnAborts,
			Description: TxnAborts.Description(),
			Aggregation: view.Count(),
			TagKeys:     allTagKeys,
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
			Name:        ActiveMutations.Name(),
			Measure:     ActiveMutations,
			Description: ActiveMutations.Description(),
			Aggregation: view.Sum(),
			TagKeys:     nil,
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
			stats.Record(cctx, AlphaHealth.M(1))
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

	http.Handle("/debug/prometheus_metrics", pe)
}

// NewBadgerCollector returns a prometheus Collector for Badger metrics from expvar.
func NewBadgerCollector() prometheus.Collector {
	return prometheus.NewExpvarCollector(map[string]*prometheus.Desc{
		"badger_v2_disk_reads_total": prometheus.NewDesc(
			"badger_v2_disk_reads_total",
			"Number of cumulative reads by Badger",
			nil, nil,
		),
		"badger_v2_disk_writes_total": prometheus.NewDesc(
			"badger_v2_disk_writes_total",
			"Number of cumulative writes by Badger",
			nil, nil,
		),
		"badger_v2_read_bytes": prometheus.NewDesc(
			"badger_v2_read_bytes",
			"Number of cumulative bytes read by Badger",
			nil, nil,
		),
		"badger_v2_written_bytes": prometheus.NewDesc(
			"badger_v2_written_bytes",
			"Number of cumulative bytes written by Badger",
			nil, nil,
		),
		"badger_v2_lsm_level_gets_total": prometheus.NewDesc(
			"badger_v2_lsm_level_gets_total",
			"Total number of LSM gets",
			[]string{"level"}, nil,
		),
		"badger_v2_lsm_bloom_hits_total": prometheus.NewDesc(
			"badger_v2_lsm_bloom_hits_total",
			"Total number of LSM bloom hits",
			[]string{"level"}, nil,
		),
		"badger_v2_gets_total": prometheus.NewDesc(
			"badger_v2_gets_total",
			"Total number of gets",
			nil, nil,
		),
		"badger_v2_puts_total": prometheus.NewDesc(
			"badger_v2_puts_total",
			"Total number of puts",
			nil, nil,
		),
		"badger_v2_memtable_gets_total": prometheus.NewDesc(
			"badger_v2_memtable_gets_total",
			"Total number of memtable gets",
			nil, nil,
		),
		"badger_v2_lsm_size": prometheus.NewDesc(
			"badger_v2_lsm_size",
			"Size of the LSM in bytes",
			[]string{"dir"}, nil,
		),
		"badger_v2_vlog_size": prometheus.NewDesc(
			"badger_v2_vlog_size",
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
	if collector := conf.GetString("jaeger.collector"); len(collector) > 0 {
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

	if collector := conf.GetString("datadog.collector"); len(collector) > 0 {
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

	// Exclusively for stats, metrics, etc. Not for tracing.
	// var views = append(ocgrpc.DefaultServerViews, ocgrpc.DefaultClientViews...)
	// if err := view.Register(views...); err != nil {
	// 	glog.Fatalf("Unable to register OpenCensus stats: %v", err)
	// }
}
