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
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricTxnAborts(t *testing.T) {
	mt := `
    {
	  set {
		<0x71>  <name> "Bob" .
	  }
	}
	`

	// Create initial 'dgraph_txn_aborts' metric
	mr1, err := mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	mr2, err := mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr1.keys, mr1.preds, mr1.startTs))
	require.Error(t, commitWithTs(mr2.keys, mr2.preds, mr2.startTs))

	// Fetch Metrics
	req, err := http.NewRequest("GET", addr+"/debug/prometheus_metrics", nil)
	require.NoError(t, err)
	_, body, err := runRequest(req)
	require.NoError(t, err)
	metricsMap, err := extractMetrics(string(body))
	requiredMetric := "dgraph_txn_aborts"
	txnAbort, ok := metricsMap[requiredMetric]
	require.True(t, ok, "the required metric '%s' is not found", requiredMetric)
	txnAbort1, _ := strconv.Atoi(txnAbort.(string))

	// Create second 'dgraph_txn_aborts' metric
	mr1, err = mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	mr2, err = mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr1.keys, mr1.preds, mr1.startTs))
	require.Error(t, commitWithTs(mr2.keys, mr2.preds, mr2.startTs))

	// Fetch Updated Metrics
	req, err = http.NewRequest("GET", addr+"/debug/prometheus_metrics", nil)
	require.NoError(t, err)
	_, body, err = runRequest(req)
	require.NoError(t, err)
	metricsMap, err = extractMetrics(string(body))
	requiredMetric = "dgraph_txn_aborts"
	txnAbort, ok = metricsMap["dgraph_txn_aborts"]
	txnAbort2, _ := strconv.Atoi(txnAbort.(string))

	require.Equal(t, txnAbort1+1, txnAbort2, "txnAbort was not incremented")
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
	metricRegex, err := regexp.Compile("(^\\w+|\\d+$)")

	if err != nil {
		return nil, err
	}
	metricsMap := make(map[string]interface{})
	for _, line := range lines {
		matches := metricRegex.FindAllString(line, -1)
		if len(matches) > 0 {
			metricsMap[matches[0]] = matches[1]
		}
	}
	return metricsMap, nil
}
