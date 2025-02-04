//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
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
	mr, err := mutationWithTs(mutationInp{body: mt, typ: "application/rdf"})
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr, false))

	metrics := fetchMetrics(t, metricName)

	// second normal commit
	mr, err = mutationWithTs(mutationInp{body: mt, typ: "application/rdf"})
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr, false))

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
	mr, err := mutationWithTs(mutationInp{body: mt, typ: "application/rdf"})
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr, false))

	metrics := fetchMetrics(t, metricName)

	// second commit discarded
	mr, err = mutationWithTs(mutationInp{body: mt, typ: "application/rdf"})
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr, true))

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

	mr1, err := mutationWithTs(mutationInp{body: mt, typ: "application/rdf"})
	require.NoError(t, err)
	mr2, err := mutationWithTs(mutationInp{body: mt, typ: "application/rdf"})
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr1, false))
	require.Error(t, commitWithTs(mr2, false))

	metrics := fetchMetrics(t, metricName)

	mr1, err = mutationWithTs(mutationInp{body: mt, typ: "application/rdf"})
	require.NoError(t, err)
	mr2, err = mutationWithTs(mutationInp{body: mt, typ: "application/rdf"})
	require.NoError(t, err)
	require.NoError(t, commitWithTs(mr1, false))
	require.Error(t, commitWithTs(mr2, false))

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
		"go_goroutines", "go_memstats_heap_alloc_bytes",
		"go_memstats_heap_idle_bytes", "go_memstats_heap_inuse_bytes", "dgraph_latency_bucket",

		// Badger Metrics
		"badger_put_num_user", "badger_write_bytes_l0", "badger_write_bytes_user",
		"badger_write_pending_num_memtable", "badger_write_num_vlog",
		"badger_read_bytes_lsm", "badger_read_num_vlog",
		"badger_compaction_current_num_lsm",
		"badger_hit_num_lsm_bloom_filter", "badger_get_num_user", "badger_size_bytes_lsm",
		"badger_write_bytes_vlog", "badger_get_with_result_num_user",
		"badger_size_bytes_vlog", "badger_iterator_num_user", "badger_read_bytes_vlog",
		// The following metrics get exposed after 1 minute from Badger, so
		// they're not available in time for this test
		// "badger_lsm_size_bytes", "badger_vlog_size_bytes",

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
	metricRegex, err := regexp.Compile(`(^\w+|\d+$)`)

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
