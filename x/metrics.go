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
	"contrib.go.opencensus.io/exporter/prometheus"
	datadog "github.com/DataDog/opencensus-go-exporter-datadog"
	"github.com/golang/glog"
	"github.com/spf13/viper"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	// Cumulative metrics.
	NumQueries = stats.Int64("num_queries_total",
		"Total number of queries", stats.UnitDimensionless)
	NumMutations = stats.Int64("num_mutations_total",
		"Total number of mutations", stats.UnitDimensionless)
	NumEdges = stats.Int64("num_edges_total",
		"Total number of edges created", stats.UnitDimensionless)
	LatencyMs = stats.Float64("latency",
		"Latency of the various methods", stats.UnitMilliseconds)

	// Point-in-time metrics.
	PendingQueries = stats.Int64("pending_queries_total",
		"Number of pending queries", stats.UnitDimensionless)
	PendingProposals = stats.Int64("pending_proposals_total",
		"Number of pending proposals", stats.UnitDimensionless)
	NumGoRoutines = stats.Int64("goroutines_total",
		"Number of goroutines", stats.UnitDimensionless)
	MemoryInUse = stats.Int64("memory_inuse_bytes",
		"Amount of memory in use", stats.UnitBytes)
	MemoryIdle = stats.Int64("memory_idle_bytes",
		"Amount of memory in idle spans", stats.UnitBytes)
	MemoryProc = stats.Int64("memory_proc_bytes",
		"Amount of memory used in processes", stats.UnitBytes)
	ActiveMutations = stats.Int64("active_mutations_total",
		"Number of active mutations", stats.UnitDimensionless)
	AlphaHealth = stats.Int64("alpha_health_status",
		"Status of the alphas", stats.UnitDimensionless)
	RaftAppliedIndex = stats.Int64("raft_applied_index",
		"Latest applied Raft index", stats.UnitDimensionless)
	MaxAssignedTs = stats.Int64("max_assigned_ts",
		"Latest max assigned timestamp", stats.UnitDimensionless)

	// Conf holds the metrics config.
	// TODO: Request statistics, latencies, 500, timeouts
	Conf *expvar.Map

	// Tag keys here
	KeyStatus, _ = tag.NewKey("status")
	KeyError, _  = tag.NewKey("error")
	KeyMethod, _ = tag.NewKey("method")

	// Tag values here
	TagValueStatusOK    = "ok"
	TagValueStatusError = "error"

	defaultLatencyMsDistribution = view.Distribution(
		0, 0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16,
		20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500,
		650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)

	allTagKeys = []tag.Key{
		KeyStatus, KeyError, KeyMethod,
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

		// Last value aggregations
		{
			Name:        PendingQueries.Name(),
			Measure:     PendingQueries,
			Description: PendingQueries.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        PendingProposals.Name(),
			Measure:     PendingProposals,
			Description: PendingProposals.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        NumGoRoutines.Name(),
			Measure:     NumGoRoutines,
			Description: NumGoRoutines.Description(),
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
			Name:        ActiveMutations.Name(),
			Measure:     ActiveMutations,
			Description: ActiveMutations.Description(),
			Aggregation: view.LastValue(),
			TagKeys:     allTagKeys,
		},
		{
			Name:        AlphaHealth.Name(),
			Measure:     AlphaHealth,
			Description: AlphaHealth.Description(),
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
		for {
			select {
			case <-ticker.C:
				v = TagValueStatusOK
				if err := HealthCheck(); err != nil {
					v = TagValueStatusError
				}
				cctx, _ := tag.New(ctx, tag.Upsert(KeyStatus, v))
				// TODO: Do we need to set health to zero, or would this tag be sufficient to
				// indicate if Alpha is up but HealthCheck is failing.
				stats.Record(cctx, AlphaHealth.M(1))
			}
		}
	}()

	CheckfNoTrace(view.Register(allViews...))

	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "dgraph",
		OnError:   func(err error) { glog.Errorf("%v", err) },
	})
	Checkf(err, "Failed to create OpenCensus Prometheus exporter: %v", err)
	view.RegisterExporter(pe)

	http.Handle("/debug/prometheus_metrics", pe)
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
