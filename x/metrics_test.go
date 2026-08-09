/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opencensus.io/stats/view"
)

// TestTxnsPerDeltaViewRegisteredAsDeclared pins the metric operators read to
// judge Zero's delta batching depth. The bucket layout is part of the metric's
// contract: "share of samples in buckets >= 2" is the documented reading, so a
// silent re-bucketing would change what dashboards mean without breaking them.
func TestTxnsPerDeltaViewRegisteredAsDeclared(t *testing.T) {
	v := view.Find("txns_per_delta")
	require.NotNil(t, v, "txns_per_delta view is not registered")
	require.Equal(t, TxnsPerDelta, v.Measure)
	require.Equal(t, view.AggTypeDistribution, v.Aggregation.Type)
	// Declared as Distribution(0, 1, 2, ...); OpenCensus drops non-positive
	// bounds at registration, so the effective layout starts at 1.
	require.Equal(t, []float64{1, 2, 4, 8, 16, 32, 64, 128, 256}, v.Aggregation.Buckets)
}
