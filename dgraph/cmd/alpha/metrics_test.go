/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package alpha

import (
	"bytes"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricTxnAborts(t *testing.T) {
	t.Log("a string")
  	t.Fail()

	// borrowed from integraiton/testxn/main_test.go
	// 1. create two transactions
	// 2. commit two transactions
	// 3. test metric of any number, should exist
	// 4. save result of metric
	// 5. create 10 transactions
	// 6. commit 10 transactions
	// 7. test that metric is +10 from original amount

	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	txn2 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Manish"}`, uid))
	x.Check2(txn2.Mutate(context.Background(), mu))

	require.NoError(t, txn.Commit(context.Background()))
	err = txn2.Commit(context.Background())
	x.AssertTrue(err != nil)

	txn = s.dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
}

func TestMetrics(t *testing.T) {
	req, err := http.NewRequest("GET", addr+"/debug/prometheus_metrics", nil)
	require.NoError(t, err)

	_, body, err := runRequest(req)
	require.NoError(t, err)
	metricsMap, err := extractMetrics(string(body))
	require.NoError(t, err, "Unable to get the metrics map: %v", err)

	requiredMetrics := []string{
		// Go Runtime Metrics
		"go_goroutines", "go_memstats_gc_cpu_fraction", "go_memstats_heap_alloc_bytes",
		"go_memstats_heap_idle_bytes", "go_memstats_heap_inuse_bytes", "dgraph_latency_bucket",

		// Badger Metrics
		"badger_v2_disk_reads_total", "badger_v2_disk_writes_total", "badger_v2_gets_total",
		"badger_v2_memtable_gets_total", "badger_v2_puts_total", "badger_v2_read_bytes",
		"badger_v2_written_bytes",

		// Dgraph Memory Metrics
		"dgraph_memory_idle_bytes", "dgraph_memory_inuse_bytes", "dgraph_memory_proc_bytes",
		// Dgraph Activity Metrics
		"dgraph_active_mutations_total", "dgraph_pending_proposals_total",
		"dgraph_pending_queries_total",
		"dgraph_num_queries_total", "dgraph_alpha_health_status",
	}
	for _, requiredM := range requiredMetrics {
		_, ok := metricsMap[requiredM]
		require.True(t, ok, "the required metric %s is not found", requiredM)
	}
}

func extractMetrics(metrics string) (map[string]interface{}, error) {
	lines := strings.Split(metrics, "\n")
	metricRegex, err := regexp.Compile("(^[a-z0-9_]+)")
	if err != nil {
		return nil, err
	}
	metricsMap := make(map[string]interface{})
	for _, line := range lines {
		matches := metricRegex.FindStringSubmatch(line)
		if len(matches) > 0 {
			metricsMap[matches[0]] = struct{}{}
		}
	}
	return metricsMap, nil
}
