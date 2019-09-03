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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetrics(t *testing.T) {
	req, err := http.NewRequest("GET", addr+"/debug/prometheus_metrics", nil)
	require.NoError(t, err)

	_, body, err := runRequest(req)
	metricsMap, err := extractMetrics(string(body))
	require.NoError(t, err, "Unable to get the metrics map: %v", err)

	requiredMetrics := []string{"go_goroutines", "go_memstats_heap_alloc_bytes",
		"go_memstats_heap_idle_bytes", "go_memstats_heap_inuse_bytes", "dgraph_latency_bucket"}
	for _, requiredM := range requiredMetrics {
		_, ok := metricsMap[requiredM]
		require.True(t, ok, "the required metric %s is not found", requiredM)
	}
}

func extractMetrics(metrics string) (map[string]interface{}, error) {
	lines := strings.Split(metrics, "\n")
	metricRegex, err := regexp.Compile("(^[a-z_]+)")
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
