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
)

var (
	// These are cumulative
	PostingReads = stats.Int64("dgraph/posting_reads",
		"The number of posting reads", stats.UnitDimensionless)
	PostingWrites = stats.Int64("dgraph/posting_writes",
		"The number of posting writes", stats.UnitDimensionless)
	BytesRead = stats.Int64("dgraph/bytes_read",
		"The number of bytes read", stats.UnitBytes)
	BytesWrite = stats.Int64("dgraph/bytes_written",
		"The number of bytes written", stats.UnitBytes)
	NumQueries = stats.Int64("dgraph/queries",
		"The number of queries", stats.UnitDimensionless)
	NumMutations = stats.Int64("dgraph/mutations",
		"The number of mutations", stats.UnitDimensionless)
	LatencyMs = stats.Float64("dgraph/latency",
		"The latency of the various methods", stats.UnitMilliseconds)

	// value at particular point of time
	PendingQueries = stats.Int64("dgraph/queries_pending",
		"The number of pending queries", stats.UnitDimensionless)
	PendingProposals = stats.Int64("dgraph/proposals_pending",
		"The number of pending proposals", stats.UnitDimensionless)
	DirtyMapSize = stats.Int64("dgraph/dirtymap_size",
		"The number of elements in the dirty map", stats.UnitDimensionless)
	NumGoRoutines = stats.Int64("dgraph/goroutines",
		"The number of goroutines", stats.UnitDimensionless)
	MemoryInUse = stats.Int64("dgraph/memory_in_use",
		"The amount of memory in use", stats.UnitBytes)
	MemoryIdle = stats.Int64("dgraph/memory_idle",
		"The amount of memory in idle spans", stats.UnitBytes)
	MemoryProc = stats.Int64("dgraph/memory_proc",
		"The amount of memory used in processes", stats.UnitBytes)
	ActiveMutations = stats.Int64("dgraph/active_mutations",
		"The number of active mutations", stats.UnitDimensionless)
	AlphaHealth = stats.Int64("dgraph/alpha_status",
		"The status of the alphas", stats.UnitDimensionless)
	MaxPlSize = stats.Int64("dgraph/max_list_bytes",
		"The maximum value of bytes of the list", stats.UnitBytes)
	MaxPlLength = stats.Int64("dgraph/max_list_length",
		"The maximum length of the list", stats.UnitDimensionless)

	// TODO: Request statistics, latencies, 500, timeouts
	Conf *expvar.Map
)

var (
	// Tag keys here
	KeyPid, _    = tag.NewKey("pid")
	KeyStatus, _ = tag.NewKey("status")
	KeyError, _  = tag.NewKey("error")
	KeyMethod, _ = tag.NewKey("method")
	KeyPeriod, _ = tag.NewKey("period")

	// Tag values here
	TagValueStatusOK    = "ok"
	TagValueStatusError = "error"
)

var (
	defaultBytesDistribution = view.Distribution(
		0, 1024, 2048, 4096, 16384, 65536, 262144, 1048576, 4194304,
		16777216, 67108864, 268435456, 1073741824, 4294967296)

	defaultLatencyMsDistribution = view.Distribution(
		0, 0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16,
		20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500,
		650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)
)

var allTagKeys = []tag.Key{
	KeyPid, KeyStatus, KeyError, KeyMethod,
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
		Name:        PostingReads.Name(),
		Measure:     PostingReads,
		Description: PostingReads.Description(),
		Aggregation: view.Count(),
		TagKeys:     allTagKeys,
	},
	{
		Name:        PostingWrites.Name(),
		Measure:     PostingWrites,
		Description: PostingWrites.Description(),
		Aggregation: view.Count(),
		TagKeys:     allTagKeys,
	},
	{
		Name:        BytesRead.Name(),
		Measure:     BytesRead,
		Description: BytesRead.Description(),
		Aggregation: defaultBytesDistribution,
		TagKeys:     allTagKeys,
	},
	{
		Name:        BytesWrite.Name(),
		Measure:     BytesWrite,
		Description: BytesWrite.Description(),
		Aggregation: defaultBytesDistribution,
		TagKeys:     allTagKeys,
	},
	{
		Name:        NumQueries.Name(),
		Measure:     NumQueries,
		Description: NumQueries.Description(),
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
		Name:        DirtyMapSize.Name(),
		Measure:     DirtyMapSize,
		Description: DirtyMapSize.Description(),
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
	{
		Name:        MaxPlSize.Name(),
		Measure:     MaxPlSize,
		Description: MaxPlSize.Description(),
		Aggregation: view.LastValue(),
		TagKeys:     allTagKeys,
	},
	{
		Name:        MaxPlLength.Name(),
		Measure:     MaxPlLength,
		Description: MaxPlLength.Description(),
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

func WithMethod(parent context.Context, method string) context.Context {
	ctx, err := tag.New(parent, tag.Upsert(KeyMethod, method))
	Check(err)
	return ctx
}

func SinceInMilliseconds(startTime time.Time) float64 {
	return float64(time.Since(startTime)) / 1e6
}
