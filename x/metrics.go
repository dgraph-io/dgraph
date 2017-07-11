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
	"time"

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
	PendingProposals *expvar.Int
	LCacheSize       *expvar.Int
	LCacheLen        *expvar.Int
	LCacheCapacity   *expvar.Int
	DirtyMapSize     *expvar.Int
	NumGoRoutines    *expvar.Int
	MemoryInUse      *expvar.Int
	HeapIdle         *expvar.Int
	TotalMemory      *expvar.Int
	ActiveMutations  *expvar.Int
	ServerHealth     *expvar.Int
	MaxPlLength      *expvar.Int

	PredicateStats *expvar.Map

	MaxPlLen int64
	// TODO: Request statistics, latencies, 500, timeouts

)

func init() {
	PostingReads = expvar.NewInt("postingReads")
	PostingWrites = expvar.NewInt("postingWrites")
	PendingProposals = expvar.NewInt("pendingProposals")
	BytesRead = expvar.NewInt("bytesRead")
	BytesWrite = expvar.NewInt("bytesWrite")
	EvictedPls = expvar.NewInt("evictedPls")
	PendingQueries = expvar.NewInt("pendingQueries")
	NumQueries = expvar.NewInt("numQueries")
	ServerHealth = expvar.NewInt("serverHealth")
	DirtyMapSize = expvar.NewInt("dirtyMapSize")
	LCacheSize = expvar.NewInt("lcacheSize")
	LCacheLen = expvar.NewInt("lcacheLen")
	lCacheCapacity = expvar.NewInt("lCacheCapacity")
	NumGoRoutines = expvar.NewInt("numGoRoutines")
	MemoryInUse = expvar.NewInt("memoryInUse")
	HeapIdle = expvar.NewInt("heapIdle")
	TotalMemory = expvar.NewInt("totalMemory")
	ActiveMutations = expvar.NewInt("activeMutations")
	PredicateStats = expvar.NewMap("predicateStats")
	CacheHit = expvar.NewInt("cacheHit")
	CacheMiss = expvar.NewInt("cacheMiss")
	CacheRace = expvar.NewInt("cacheRace")
	MaxPlLength = expvar.NewInt("maxPlLength")

	ticker := time.NewTicker(5 * time.Second)

	go func() {
		var err error
		for {
			select {
			case <-ticker.C:
				if err = HealthCheck(); err == nil {
					ServerHealth.Set(1)
				} else {
					ServerHealth.Set(0)
				}
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
		"lcacheSize": prometheus.NewDesc(
			"lcacheSize",
			"lcacheSize",
			nil, nil,
		),
		"lcacheLen": prometheus.NewDesc(
			"lcacheLen",
			"lcacheLen",
			nil, nil,
		),
		"lCacheCapacity": prometheus.NewDesc(
			"lCacheCapacity",
			"lCacheCapacity",
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
	})
	prometheus.MustRegister(expvarCollector)
	http.Handle("/debug/prometheus_metrics", prometheus.Handler())
}
