/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import (
	"expvar"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/codahale/hdrhistogram"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// These are cummulative
	PostingReads  *expvar.Int
	PostingWrites *expvar.Int
	BytesRead     *expvar.Int
	BytesWrite    *expvar.Int
	EvictedPls    *expvar.Int
	NumQueries    *expvar.Int
	CacheHit      *expvar.Int
	CacheMiss     *expvar.Int
	CacheRace     *expvar.Int

	// value at particular point of time
	PendingQueries   *expvar.Int
	PendingReads     *expvar.Int
	PendingProposals *expvar.Int
	LCacheSize       *expvar.Int
	LCacheLen        *expvar.Int
	DirtyMapSize     *expvar.Int
	NumGoRoutines    *expvar.Int
	MemoryInUse      *expvar.Int
	HeapIdle         *expvar.Int
	TotalMemory      *expvar.Int
	ProposalMemory   *expvar.Int
	ActiveMutations  *expvar.Int
	ServerHealth     *expvar.Int
	MaxPlLength      *expvar.Int

	PredicateStats *expvar.Map
	PlValuesDst    *expvar.Map

	PlValueHist *hdrhistogram.Histogram
	MaxPlLen    int64
	// TODO: Request statistics, latencies, 500, timeouts

)

func init() {
	PostingReads = expvar.NewInt("postingReads")
	PostingWrites = expvar.NewInt("postingWrites")
	PendingReads = expvar.NewInt("pendingReads")
	PendingProposals = expvar.NewInt("pendingProposals")
	BytesRead = expvar.NewInt("bytesRead")
	BytesWrite = expvar.NewInt("bytesWrite")
	EvictedPls = expvar.NewInt("evictedPls")
	PendingQueries = expvar.NewInt("pendingQueries")
	NumQueries = expvar.NewInt("numQueries")
	ServerHealth = expvar.NewInt("serverHealth")
	DirtyMapSize = expvar.NewInt("dirtyMapSize")
	LCacheSize = expvar.NewInt("lCacheSize")
	LCacheLen = expvar.NewInt("lCacheLen")
	NumGoRoutines = expvar.NewInt("numGoRoutines")
	MemoryInUse = expvar.NewInt("memoryInUse")
	HeapIdle = expvar.NewInt("heapIdle")
	TotalMemory = expvar.NewInt("totalMemory")
	ProposalMemory = expvar.NewInt("proposalMemory")
	ActiveMutations = expvar.NewInt("activeMutations")
	PredicateStats = expvar.NewMap("predicateStats")
	PlValuesDst = expvar.NewMap("plValuesDst")
	CacheHit = expvar.NewInt("cacheHit")
	CacheMiss = expvar.NewInt("cacheMiss")
	CacheRace = expvar.NewInt("cacheRace")
	MaxPlLength = expvar.NewInt("maxPlLength")

	PlValueHist = hdrhistogram.New(1, 1<<40, 4)

	ticker := time.NewTicker(5 * time.Second)

	// # hacky: Find better way later
	go func() {
		var val50 expvar.Int
		var val90 expvar.Int
		var val99 expvar.Int
		var val99_99 expvar.Int
		var valMax expvar.Int
		var err error
		for {
			select {
			case <-ticker.C:
				val50.Set(PlValueHist.ValueAtQuantile(50.0))
				PlValuesDst.Set("50", &val50)
				val90.Set(PlValueHist.ValueAtQuantile(90.0))
				PlValuesDst.Set("90", &val90)
				val99.Set(PlValueHist.ValueAtQuantile(99.0))
				PlValuesDst.Set("99", &val99)
				val99_99.Set(PlValueHist.ValueAtQuantile(99.99))
				PlValuesDst.Set("99.99", &val99_99)
				valMax.Set(atomic.LoadInt64(&MaxPlLen))
				PlValuesDst.Set("Max", &valMax)
				if err = HealthCheck(); err == nil {
					ServerHealth.Set(1)
				} else {
					ServerHealth.Set(0)
				}
				MaxPlLength.Set(atomic.LoadInt64(&MaxPlLen))
			}
		}
	}()

	expvarCollector := prometheus.NewExpvarCollector(map[string]*prometheus.Desc{
		"cacheHit": prometheus.NewDesc(
			"cacheHit",
			"cacheHit",
			nil, nil,
		),
		"cacheMiss": prometheus.NewDesc(
			"cacheMiss",
			"cacheMiss",
			nil, nil,
		),
		"cacheRace": prometheus.NewDesc(
			"cacheRace",
			"cacheRace",
			nil, nil,
		),
		"postingReads": prometheus.NewDesc(
			"posting_reads",
			"cummulative posting reads",
			nil, nil,
		),
		"postingWrites": prometheus.NewDesc(
			"posting_writes",
			"cummulative posting writes",
			nil, nil,
		),
		"pendingReads": prometheus.NewDesc(
			"pending_reads",
			"cummulative pending reads",
			nil, nil,
		),
		"maxPlLength": prometheus.NewDesc(
			"maxPlLength",
			"maxPlLength",
			nil, nil,
		),
		"pendingProposals": prometheus.NewDesc(
			"pending_proposals",
			"cummulative pending proposals",
			nil, nil,
		),
		"bytesRead": prometheus.NewDesc(
			"bytes_read",
			"cummulative bytes Read",
			nil, nil,
		),
		"bytesWrite": prometheus.NewDesc(
			"bytes_write",
			"cummulative bytes Written",
			nil, nil,
		),
		"evictedPls": prometheus.NewDesc(
			"evictedPls",
			"cummulative evictedPls",
			nil, nil,
		),
		"pendingQueries": prometheus.NewDesc(
			"pending_queries",
			"pendingQueries",
			nil, nil,
		),
		"numQueries": prometheus.NewDesc(
			"numQueries",
			"numQueries",
			nil, nil,
		),
		"serverHealth": prometheus.NewDesc(
			"serverHealth",
			"serverHealth",
			nil, nil,
		),
		"dirtyMapSize": prometheus.NewDesc(
			"dirtyMapSize",
			"dirtyMapSize",
			nil, nil,
		),
		"lCacheSize": prometheus.NewDesc(
			"lCacheSize",
			"lCacheSize",
			nil, nil,
		),
		"lCacheLen": prometheus.NewDesc(
			"lCacheLen",
			"lCacheLen",
			nil, nil,
		),
		"numGoRoutines": prometheus.NewDesc(
			"numGoRoutines",
			"numGoRoutines",
			nil, nil,
		),
		"memoryInUse": prometheus.NewDesc(
			"memoryInUse",
			"memoryInUse",
			nil, nil,
		),
		"heapIdle": prometheus.NewDesc(
			"heapIdle",
			"heapIdle",
			nil, nil,
		),
		"totalMemory": prometheus.NewDesc(
			"totalMemory",
			"totalMemory",
			nil, nil,
		),
		"proposalMemory": prometheus.NewDesc(
			"proposalMemory",
			"proposalMemory",
			nil, nil,
		),
		"activeMutations": prometheus.NewDesc(
			"activeMutations",
			"activeMutations",
			nil, nil,
		),
		"predicateStats": prometheus.NewDesc(
			"predicateStats",
			"predicateStats",
			[]string{"name"}, nil,
		),
		"plValuesDst": prometheus.NewDesc(
			"plValuesDst",
			"plValuesDst",
			[]string{"quantile"}, nil,
		),
	})
	prometheus.MustRegister(expvarCollector)
	http.Handle("/debug/prometheus_metrics", prometheus.Handler())
}
