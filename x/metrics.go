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
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var (
	// These are cumulative
	PostingReads  = stats.Int64("dgraph/posting_reads", "The number of posting reads", "1")
	PostingWrites = stats.Int64("dgraph/posting_writes", "The number of posting writes", "1")
	BytesRead     = stats.Int64("dgraph/bytes_read", "The number of bytes read", "By")
	BytesWrite    = stats.Int64("dgraph/bytes_written", "The number of bytes written", "By")
	NumQueries    = stats.Int64("dgraph/queries", "The number of queries", "By")
	LcacheHit     = stats.Int64("dgrap/lcache_hit", "The number of hits from the LRU cache", "1")
	LcacheMiss    = stats.Int64("dgrap/lcache_miss", "The number of misses from the LRU cache", "1")
	LcacheRace    = stats.Int64("dgrap/lcache_race", "The number of races from the LRU cache", "1")
	LcacheEvicts  = stats.Int64("dgrap/lcache_evicts", "The number of evictions from the LRU cache", "1")
	LatencyMs     = stats.Float64("dgrap/latency", "The latency of the various methods", "ms")

	// value at particular point of time
	PendingQueries   = stats.Int64("dgrap/queries_pending", "The number of pending queries", "1")
	PendingProposals = stats.Int64("dgrap/proposals_pending", "The number of pending proposals", "1")
	LcacheSize       = stats.Int64("dgraph/lcache_size", "The size of the LRU cache", "By")
	LcacheLen        = stats.Int64("dgraph/lcache_len", "The length of the LRU cache", "By")
	LcacheCapacity   = stats.Int64("dgraph/lcache_capacity", "The number of items in LRU cache", "1")
	DirtyMapSize     = stats.Int64("dgrap/dirtymap_size", "The number of elements in the dirty map", "1")
	NumGoRoutines    = stats.Int64("dgraph/goroutines", "The number of goroutines", "1")
	MemoryInUse      = stats.Int64("dgraph/memory_in_use", "The amount of memory in use", "By")
	MemoryIdle       = stats.Int64("dgraph/memory_idle", "The amount of memory in idle spans", "By")
	MemoryProc       = stats.Int64("dgraph/memory_proc", "The amount of memory used in processes", "By")
	ActiveMutations  = stats.Int64("dgraph/active_mutations", "The number of active mutations", "1")
	AlphaHealth      = stats.Int64("dgraph/alpha_status", "The status of the alphas", "1")
	MaxPlSize        = stats.Int64("dgraph/max_list_bytes", "The maximum value of bytes of the list", "By")
	MaxPlLength      = stats.Int64("dgraph/max_list_length", "The maximum length of the list", "1")

	PredicateStats *expvar.Map
	Conf           *expvar.Map

	MaxPlSz int64
	// TODO: Request statistics, latencies, 500, timeouts
)

var (
	// Tag keys here
	KeyOS, _        = tag.NewKey("goos")
	KeyPid, _       = tag.NewKey("pid")
	KeyArch, _      = tag.NewKey("goarch")
	KeyStatus, _    = tag.NewKey("status")
	KeyError, _     = tag.NewKey("error")
	KeyGoVersion, _ = tag.NewKey("goversion")
	KeyMethod, _    = tag.NewKey("method")
	KeyPeriod, _    = tag.NewKey("period")

	// Tag values here
	TagValueStatusOK    = "ok"
	TagValueStatusError = "error"
)

var (
	defaultBytesDistribution     = view.Distribution(0, 1024, 2048, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864, 268435456, 1073741824, 4294967296)
	defaultLatencyMsDistribution = view.Distribution(0, 0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)
)

var Views = []*view.View{

	{
		Name:        "dgraph/latency",
		Measure:     LatencyMs,
		Description: "The latency distributions of the various methods",
		Aggregation: defaultLatencyMsDistribution,
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/posting_reads",
		Measure:     PostingReads,
		Description: "The number of posting reads",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/posting_writes",
		Measure:     PostingWrites,
		Description: "The number of posting writes",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/bytes_read",
		Measure:     BytesRead,
		Description: "The number of bytes read",
		Aggregation: defaultBytesDistribution,
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/bytes_write",
		Measure:     BytesWrite,
		Description: "The number of bytes written",
		Aggregation: defaultBytesDistribution,
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/queries",
		Measure:     NumQueries,
		Description: "The number of queries",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/lcache_hit",
		Measure:     LcacheHit,
		Description: "The number of hits from the LRU cache",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/lcache_miss",
		Measure:     LcacheMiss,
		Description: "The number of misses from the LRU cache",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/lcache_evict",
		Measure:     LcacheEvicts,
		Description: "The number of evictions from the LRU cache",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/lcache_race",
		Measure:     LcacheRace,
		Description: "The number of races in the LRU cache",
		Aggregation: view.Count(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},

	// Last value aggregations
	{
		Name:        "dgraph/pending_queries",
		Measure:     PendingQueries,
		Description: "The number of pending queries",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/pending_proposals",
		Measure:     PendingProposals,
		Description: "The number of pending proposals",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/lcache_size",
		Measure:     LcacheSize,
		Description: "The size of the LRU cache",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/lcache_len",
		Measure:     LcacheLen,
		Description: "The length of the LRU cache",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/lcache_capcaity",
		Measure:     LcacheCapacity,
		Description: "The number of items in the LRU cache",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/dirtymap_size",
		Measure:     DirtyMapSize,
		Description: "The number of elements in the dirty map",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/goroutines",
		Measure:     NumGoRoutines,
		Description: "The number of goroutines",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/memory_in_use",
		Measure:     MemoryInUse,
		Description: "The amount of memory in use",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/memory_idle",
		Measure:     MemoryIdle,
		Description: "The amount of memory in idle spans",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/memory_proc",
		Measure:     MemoryProc,
		Description: "The amount of memory used in processes",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/active_mutations",
		Measure:     ActiveMutations,
		Description: "The number of active mutations",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/alpha_status",
		Measure:     AlphaHealth,
		Description: "The status of the alphas",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/max_list_bytes",
		Measure:     MaxPlSize,
		Description: "The maximum value of bytes of the list",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
	{
		Name:        "dgraph/max_list_length",
		Measure:     MaxPlLength,
		Description: "The maximum length of the list",
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{KeyPid, KeyOS, KeyArch, KeyStatus, KeyError, KeyGoVersion, KeyMethod},
	},
}

func init() {
	PredicateStats = expvar.NewMap("dgraph_predicate_stats")
	Conf = expvar.NewMap("dgraph_config")

	ctx := ObservabilityEnabledParentContext()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				var tags []tag.Mutator
				if err := HealthCheck(); err == nil {
					tags = append(tags, tag.Upsert(KeyStatus, TagValueStatusOK))
				} else {
					tags = append(tags, tag.Upsert(KeyStatus, TagValueStatusError), tag.Upsert(KeyError, err.Error()))
				}
				cctx, _ := tag.New(ctx, tags...)
				stats.Record(cctx, AlphaHealth.M(1))
			}
		}
	}()

	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "dgraph", // TODO: read this namespace from flags
		// TODO: Enable an on error that logs to Dgraph's logging output.
	})
	if err != nil {
		log.Fatalf("Failed to create OpenCensus Prometheus exporter: %v", err)
	}

	http.Handle("/debug/prometheus_metrics", pe)
}

// ObservabilityEnabledParentContext returns a context with tags that are useful for
// distinguishing the state of the running system. It contains tags such as:
// * PID
// * OS
// * Architecture
// This context will be used to derive other contexts.
func ObservabilityEnabledParentContext() context.Context {
	// At the beginning add some distinguishing information
	// to the context as tags that will be propagated when
	// collecting metrics.
	ctx, _ := tag.New(context.Background(),
		tag.Upsert(KeyPid, fmt.Sprintf("%d", os.Getpid())),
		tag.Upsert(KeyOS, runtime.GOOS),
		tag.Upsert(KeyGoVersion, runtime.Version()),
		tag.Upsert(KeyArch, runtime.GOARCH))

	return ctx
}

func ObservabilityEnabledContextWithMethod(parent context.Context, method string) context.Context {
	ctx, _ := tag.New(parent,
		tag.Upsert(KeyPid, fmt.Sprintf("%d", os.Getpid())),
		tag.Upsert(KeyOS, runtime.GOOS),
		tag.Upsert(KeyGoVersion, runtime.Version()),
		tag.Upsert(KeyArch, runtime.GOARCH),
		tag.Upsert(KeyMethod, method))
	return ctx
}

func SinceInMilliseconds(startTime time.Time) float64 {
	durNs := time.Since(startTime)
	return float64(durNs) / 1e6
}

func RegisterStatsViews() {
}
