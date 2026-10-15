/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/dgraph-io/ristretto/v2/z"
)

// TestFeatureFlagsIntraMutationKeys drives the exact superflag path
// dgraph/cmd/alpha/run.go uses, so a key present in one place and misspelled in
// the other fails here rather than at an operator's startup.
//
// The superflag layer only validates that a key EXISTS in the defaults string —
// it cannot check the value, since intra-mutation-parallelism is off|auto|N|Fx
// rather than a plain int. A bad value would otherwise resolve to the zero value
// and silently disable fan-out for the life of the process.
func TestFeatureFlagsIntraMutationKeys(t *testing.T) {
	// No overrides: the shipped defaults must parse and be what we documented.
	sf := z.NewSuperFlag("").MergeAndCheckDefault(FeatureFlagsDefaults)

	require.Equal(t, int64(1), sf.GetInt64("intra-mutation-min-edges"))
	require.Equal(t, int64(256), sf.GetInt64("intra-mutation-edges-per-worker"))

	par, err := x.ParseIntraMutationParallelism(sf.GetString("intra-mutation-parallelism"))
	require.NoError(t, err)
	require.Equal(t, x.IntraMutationParallelism{PerCPU: 1.0}, par,
		"the shipped default must be auto, i.e. one worker per CPU")

	// Each documented spelling must survive a real superflag override.
	for _, tc := range []struct {
		in   string
		want x.IntraMutationParallelism
	}{
		{"off", x.IntraMutationParallelism{}},
		{"auto", x.IntraMutationParallelism{PerCPU: 1.0}},
		{"30", x.IntraMutationParallelism{Workers: 30}},
		{"2x", x.IntraMutationParallelism{PerCPU: 2}},
		{"1.5x", x.IntraMutationParallelism{PerCPU: 1.5}},
	} {
		sf := z.NewSuperFlag("intra-mutation-parallelism=" + tc.in).
			MergeAndCheckDefault(FeatureFlagsDefaults)
		got, err := x.ParseIntraMutationParallelism(sf.GetString("intra-mutation-parallelism"))
		require.NoErrorf(t, err, "value %q", tc.in)
		require.Equalf(t, tc.want, got, "value %q", tc.in)
	}

	// A bad value must surface an error rather than resolving to off.
	sf = z.NewSuperFlag("intra-mutation-parallelism=1.5").
		MergeAndCheckDefault(FeatureFlagsDefaults)
	_, err = x.ParseIntraMutationParallelism(sf.GetString("intra-mutation-parallelism"))
	require.Error(t, err)

	// The other two keys still override as plain ints.
	sf = z.NewSuperFlag("intra-mutation-min-edges=0; intra-mutation-edges-per-worker=64").
		MergeAndCheckDefault(FeatureFlagsDefaults)
	require.Equal(t, int64(0), sf.GetInt64("intra-mutation-min-edges"))
	require.Equal(t, int64(64), sf.GetInt64("intra-mutation-edges-per-worker"))
}

// TestWorkerOptionsStringShowsIntraMutation guards the gap that cost real time
// during live debugging: WorkerOptions.String() hand-formats its fields, so a
// new field is invisible in the startup "x.WorkerConfig: %+v" line unless it is
// added there by hand.
func TestWorkerOptionsStringShowsIntraMutation(t *testing.T) {
	w := x.WorkerOptions{
		IntraMutationMinEdges:       1,
		IntraMutationParallelism:    x.IntraMutationParallelism{PerCPU: 1.5},
		IntraMutationEdgesPerWorker: 256,
	}
	s := w.String()
	require.Contains(t, s, "IntraMutationMinEdges:1")
	require.Contains(t, s, "IntraMutationParallelism:1.5x")
	require.Contains(t, s, "IntraMutationEdgesPerWorker:256")
}
