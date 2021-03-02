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
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMetricTxnCommits(t *testing.T) {
	metricName := "dgraph_txn_commits_total"
	mt := `
    {
	  set {
		<0x71>  <name> "Bob" .
	  }
	}
	`

	// first normal commit
	mr, err := mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr.keys, mr.preds, mr.startTs, false))

	metrics := fetchMetrics(t, metricName)

	// second normal commit
	mr, err = mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr.keys, mr.preds, mr.startTs, false))

	require.NoError(t, retryableFetchMetrics(t, map[string]int{
		metricName: metrics[metricName] + 1,
	}))
}

func TestMetricTxnDiscards(t *testing.T) {
	metricName := "dgraph_txn_discards_total"
	mt := `
    {
	  set {
		<0x71>  <name> "Bob" .
	  }
	}
	`

	// first normal commit
	mr, err := mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr.keys, mr.preds, mr.startTs, false))

	metrics := fetchMetrics(t, metricName)

	// second commit discarded
	mr, err = mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr.keys, mr.preds, mr.startTs, true))

	require.NoError(t, retryableFetchMetrics(t, map[string]int{
		metricName: metrics[metricName] + 1,
	}))
}

func TestMetricTxnAborts(t *testing.T) {
	metricName := "dgraph_txn_aborts_total"
	mt := `
    {
	  set {
		<0x71>  <name> "Bob" .
	  }
	}
	`

	mr1, err := mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	mr2, err := mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr1.keys, mr1.preds, mr1.startTs, false))
	require.Error(t, commitWithTs(mr2.keys, mr2.preds, mr2.startTs, false))

	metrics := fetchMetrics(t, metricName)

	mr1, err = mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	mr2, err = mutationWithTs(mt, "application/rdf", false, false, 0)
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr1.keys, mr1.preds, mr1.startTs, false))
	require.Error(t, commitWithTs(mr2.keys, mr2.preds, mr2.startTs, false))

	require.NoError(t, retryableFetchMetrics(t, map[string]int{
		metricName: metrics[metricName] + 1,
	}))
}

func retryableFetchMetrics(t *testing.T, expected map[string]int) error {
	metricList := make([]string, 0)
	for metric := range expected {
		metricList = append(metricList, metric)
	}

	for i := 0; i < 10; i++ {
		metrics := fetchMetrics(t, metricList...)
		found := 0
		for expMetric, expCount := range expected {
			count, ok := metrics[expMetric]
			if !ok {
				return fmt.Errorf("expected metric '%s' was not found", expMetric)
			}
			if count != expCount {
				return fmt.Errorf("expected metric '%s' count was %d instead of %d",
					expMetric, count, expCount)
			}
			found++
		}
		if found == len(metricList) {
			return nil
		}
		time.Sleep(time.Second * 2)
	}

	return fmt.Errorf("metrics were not found")
}

func fetchMetrics(t *testing.T, metrics ...string) map[string]int {
	req, err := http.NewRequest("GET", addr+"/debug/prometheus_metrics", nil)
	require.NoError(t, err)

	_, body, _, err := runRequest(req)
	require.NoError(t, err)

	metricsMap, err := extractMetrics(string(body))
	require.NoError(t, err)

	countMap := make(map[string]int)
	for _, metric := range metrics {
		if count, ok := metricsMap[metric]; ok {
			n, err := strconv.Atoi(count.(string))
			require.NoError(t, err)
			countMap[metric] = n
		} else {
			t.Fatalf("the required metric '%s' was not found", metric)
		}
	}
	return countMap
}

func TestMetrics(t *testing.T) {
	req, err := http.NewRequest("GET", addr+"/debug/prometheus_metrics", nil)
	require.NoError(t, err)

	_, body, _, err := runRequest(req)
	require.NoError(t, err)
	metricsMap, err := extractMetrics(string(body))
	require.NoError(t, err, "Unable to get the metrics map: %v", err)

	requiredMetrics := []string{
		// Go Runtime Metrics
		"go_goroutines", "go_memstats_gc_cpu_fraction", "go_memstats_heap_alloc_bytes",
		"go_memstats_heap_idle_bytes", "go_memstats_heap_inuse_bytes", "dgraph_latency_bucket",

		// Badger Metrics
		"badger_v3_disk_reads_total", "badger_v3_disk_writes_total", "badger_v3_gets_total",
		"badger_v3_memtable_gets_total", "badger_v3_puts_total", "badger_v3_read_bytes",
		"badger_v3_written_bytes",
		// The following metrics get exposed after 1 minute from Badger, so
		// they're not available in time for this test
		// "badger_v3_lsm_size_bytes", "badger_v3_vlog_size_bytes",

		// Transaction Metrics
		"dgraph_txn_aborts_total", "dgraph_txn_commits_total", "dgraph_txn_discards_total",

		// Dgraph Memory Metrics
		"dgraph_memory_idle_bytes", "dgraph_memory_inuse_bytes", "dgraph_memory_proc_bytes",
		"dgraph_memory_alloc_bytes",
		// Dgraph Activity Metrics
		"dgraph_active_mutations_total", "dgraph_pending_proposals_total",
		"dgraph_pending_queries_total",
		"dgraph_num_queries_total", "dgraph_alpha_health_status",

		// Raft metrics
		"dgraph_raft_has_leader", "dgraph_raft_is_leader", "dgraph_raft_leader_changes_total",
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
