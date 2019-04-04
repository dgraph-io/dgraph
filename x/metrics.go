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
	"net/http"
	"time"

	"github.com/golang/glog"
	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	otrace "go.opencensus.io/trace"
)

var (
	// These are cumulative
	NumQueries = stats.Int64("num_queries_total",
		"Total number of queries", stats.UnitDimensionless)
	NumMutations = stats.Int64("num_mutations_total",
		"Total number of mutations", stats.UnitDimensionless)
	NumEdges = stats.Int64("num_edges_total",
		"Total number of edges created", stats.UnitDimensionless)
	LatencyMs = stats.Float64("latency",
		"Latency of the various methods", stats.UnitMilliseconds)

	// value at particular point of time
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

	// TODO: Request statistics, latencies, 500, timeouts
	Conf *expvar.Map
)

var (
	// Tag keys here
	KeyStatus, _ = tag.NewKey("status")
	KeyError, _  = tag.NewKey("error")
	KeyMethod, _ = tag.NewKey("method")

	// Tag values here
	TagValueStatusOK    = "ok"
	TagValueStatusError = "error"
)

var defaultLatencyMsDistribution = view.Distribution(
	0, 0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16,
	20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500,
	650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)

var allTagKeys = []tag.Key{
	KeyStatus, KeyError, KeyMethod,
}

var allViews = []*view.View{
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

func SinceMs(startTime time.Time) float64 {
	return float64(time.Since(startTime)) / 1e6
}

// UpsertSpanWithMethod upserts a span using the provide name, and also upserts the name as a tag
// under the method key
func UpsertSpanWithMethod(ctx context.Context, name string) (context.Context,
	*otrace.Span) {
	span := otrace.FromContext(ctx)
	if span == nil {
		ctx, span = otrace.StartSpan(ctx, name)
	}

	ctx = WithMethod(ctx, name)
	return ctx, span
}

func WithMethod(parent context.Context, method string) context.Context {
	ctx, err := tag.New(parent, tag.Upsert(KeyMethod, method))
	Check(err)
	return ctx
}
