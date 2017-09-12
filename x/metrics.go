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
	LcacheSize       *expvar.Int
	LcacheLen        *expvar.Int
	LcacheCapacity   *expvar.Int
	DirtyMapSize     *expvar.Int
	NumGoRoutines    *expvar.Int
	MemoryInUse      *expvar.Int
	HeapIdle         *expvar.Int
	TotalMemory      *expvar.Int
	TotalOSMemory    *expvar.Int
	ActiveMutations  *expvar.Int
	ServerHealth     *expvar.Int
	MaxPlSize        *expvar.Int
	MaxPlLength      *expvar.Int

	PredicateStats *expvar.Map

	MaxPlSz int64
	// TODO: Request statistics, latencies, 500, timeouts

)

func init() {
	PostingReads = expvar.NewInt("dgraph_posting_reads_total")
	PostingWrites = expvar.NewInt("dgraph_posting_writes_total")
	PendingProposals = expvar.NewInt("dgraph_pending_proposals_total")
	BytesRead = expvar.NewInt("dgraph_read_bytes_total")
	BytesWrite = expvar.NewInt("dgraph_written_bytes_total")
	EvictedPls = expvar.NewInt("dgraph_evicted_lists_total")
	PendingQueries = expvar.NewInt("dgraph_pending_queries_total")
	NumQueries = expvar.NewInt("dgraph_num_queries_total")
	ServerHealth = expvar.NewInt("dgraph_server_health_status")
	DirtyMapSize = expvar.NewInt("dgraph_dirtymap_keys_total")
	LcacheSize = expvar.NewInt("dgraph_lcache_size_bytes")
	LcacheLen = expvar.NewInt("dgraph_lcache_keys_total")
	LcacheCapacity = expvar.NewInt("dgraph_lcache_capacity_bytes")
	NumGoRoutines = expvar.NewInt("dgraph_goroutines_total")
	MemoryInUse = expvar.NewInt("dgraph_memory_inuse_bytes")
	HeapIdle = expvar.NewInt("dgraph_heap_idle_bytes")
	TotalOSMemory = expvar.NewInt("dgraph_proc_memory_bytes")
	ActiveMutations = expvar.NewInt("dgraph_active_mutations_total")
	PredicateStats = expvar.NewMap("dgraph_predicate_stats")
	CacheHit = expvar.NewInt("dgraph_cache_hits_total")
	CacheMiss = expvar.NewInt("dgraph_cache_miss_total")
	CacheRace = expvar.NewInt("dgraph_cache_race_total")
	MaxPlSize = expvar.NewInt("dgraph_max_list_bytes")
	MaxPlLength = expvar.NewInt("dgraph_max_list_length")

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := HealthCheck(); err == nil {
					ServerHealth.Set(1)
				} else {
					ServerHealth.Set(0)
				}
			}
		}
	}()

	expvarCollector := prometheus.NewExpvarCollector(map[string]*prometheus.Desc{
		"dgraph_cache_hits_total": prometheus.NewDesc(
			"dgraph_cache_hits_total",
			"dgraph_cache_hits_total",
			nil, nil,
		),
		"dgraph_cache_miss_total": prometheus.NewDesc(
			"dgraph_cache_miss_total",
			"dgraph_cache_miss_total",
			nil, nil,
		),
		"dgraph_cache_race_total": prometheus.NewDesc(
			"dgraph_cache_race_total",
			"dgraph_cache_race_total",
			nil, nil,
		),
		"dgraph_posting_reads_total": prometheus.NewDesc(
			"dgraph_posting_reads_total",
			"dgraph_posting_reads_total",
			nil, nil,
		),
		"dgraph_posting_writes_total": prometheus.NewDesc(
			"dgraph_posting_writes_total",
			"dgraph_posting_writes_total",
			nil, nil,
		),
		"dgraph_max_list_bytes": prometheus.NewDesc(
			"dgraph_max_list_bytes",
			"dgraph_max_list_bytes",
			nil, nil,
		),
		"dgraph_max_list_length": prometheus.NewDesc(
			"dgraph_max_list_length",
			"dgraph_max_list_length",
			nil, nil,
		),
		"dgraph_pending_proposals_total": prometheus.NewDesc(
			"dgraph_pending_proposals_total",
			"dgraph_pending_proposals_total",
			nil, nil,
		),
		"dgraph_read_bytes_total": prometheus.NewDesc(
			"dgraph_read_bytes_total",
			"dgraph_read_bytes_total",
			nil, nil,
		),
		"dgraph_written_bytes_total": prometheus.NewDesc(
			"dgraph_written_bytes_total",
			"dgraph_written_bytes_total",
			nil, nil,
		),
		"dgraph_evicted_lists_total": prometheus.NewDesc(
			"dgraph_evicted_lists_total",
			"dgraph_evicted_lists_total",
			nil, nil,
		),
		"dgraph_pending_queries_total": prometheus.NewDesc(
			"dgraph_pending_queries_total",
			"dgraph_pending_queries_total",
			nil, nil,
		),
		"dgraph_num_queries_total": prometheus.NewDesc(
			"dgraph_num_queries_total",
			"dgraph_num_queries_total",
			nil, nil,
		),
		"dgraph_server_health_status": prometheus.NewDesc(
			"dgraph_server_health_status",
			"dgraph_server_health_status",
			nil, nil,
		),
		"dgraph_dirtymap_keys_total": prometheus.NewDesc(
			"dgraph_dirtymap_keys_total",
			"dgraph_dirtymap_keys_total",
			nil, nil,
		),
		"dgraph_lcache_size_bytes": prometheus.NewDesc(
			"dgraph_lcache_size_bytes",
			"dgraph_lcache_size_bytes",
			nil, nil,
		),
		"dgraph_lcache_keys_total": prometheus.NewDesc(
			"dgraph_lcache_keys_total",
			"dgraph_lcache_keys_total",
			nil, nil,
		),
		"dgraph_lcache_capacity_bytes": prometheus.NewDesc(
			"dgraph_lcache_capacity_bytes",
			"dgraph_lcache_capacity_bytes",
			nil, nil,
		),
		"dgraph_goroutines_total": prometheus.NewDesc(
			"dgraph_goroutines_total",
			"dgraph_goroutines_total",
			nil, nil,
		),
		"dgraph_memory_inuse_bytes": prometheus.NewDesc(
			"dgraph_memory_inuse_bytes",
			"dgraph_memory_inuse_bytes",
			nil, nil,
		),
		"dgraph_heap_idle_bytes": prometheus.NewDesc(
			"dgraph_heap_idle_bytes",
			"dgraph_heap_idle_bytes",
			nil, nil,
		),
		"dgraph_proc_memory_bytes": prometheus.NewDesc(
			"dgraph_proc_memory_bytes",
			"dgraph_proc_memory_bytes",
			nil, nil,
		),
		"dgraph_active_mutations_total": prometheus.NewDesc(
			"dgraph_active_mutations_total",
			"dgraph_active_mutations_total",
			nil, nil,
		),
		"dgraph_predicate_stats": prometheus.NewDesc(
			"dgraph_predicate_stats",
			"dgraph_predicate_stats",
			[]string{"name"}, nil,
		),
		"badger_disk_reads_total": prometheus.NewDesc(
			"badger_disk_reads_total",
			"badger_disk_reads_total",
			nil, nil,
		),
		"badger_disk_writes_total": prometheus.NewDesc(
			"badger_disk_writes_total",
			"badger_disk_writes_total",
			nil, nil,
		),
		"badger_read_bytes": prometheus.NewDesc(
			"badger_read_bytes",
			"badger_read_bytes",
			nil, nil,
		),
		"badger_written_bytes": prometheus.NewDesc(
			"badger_written_bytes",
			"badger_written_bytes",
			nil, nil,
		),
		"badger_lsm_level_gets_total": prometheus.NewDesc(
			"badger_lsm_level_gets_total",
			"badger_lsm_level_gets_total",
			[]string{"level"}, nil,
		),
		"badger_lsm_bloom_hits_total": prometheus.NewDesc(
			"badger_lsm_bloom_hits_total",
			"badger_lsm_bloom_hits_total",
			[]string{"level"}, nil,
		),
		"badger_gets_total": prometheus.NewDesc(
			"badger_gets_total",
			"badger_gets_total",
			nil, nil,
		),
		"badger_puts_total": prometheus.NewDesc(
			"badger_puts_total",
			"badger_puts_total",
			nil, nil,
		),
		"badger_memtable_gets_total": prometheus.NewDesc(
			"badger_memtable_gets_total",
			"badger_memtable_gets_total",
			nil, nil,
		),
		"badger_lsm_size": prometheus.NewDesc(
			"badger_lsm_size",
			"badger_lsm_size",
			[]string{"dir"}, nil,
		),
		"badger_vlog_size": prometheus.NewDesc(
			"badger_vlog_size",
			"badger_vlog_size",
			[]string{"dir"}, nil,
		),
		"badger_write_ch_len": prometheus.NewDesc(
			"badger_write_ch_len",
			"badger_write_ch_len",
			[]string{"dir"}, nil,
		),
	})
	prometheus.MustRegister(expvarCollector)
	http.Handle("/debug/prometheus_metrics", prometheus.Handler())
}
