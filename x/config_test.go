/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseIntraMutationParallelism covers the whole accepted grammar and, just
// as importantly, the rejections. The superflag layer stores this key as an
// opaque string, so nothing upstream validates it: a typo that parsed to the
// zero value would silently disable intra-predicate fan-out for the life of the
// process, with no error and no visible symptom other than throughput that never
// responds to the flag.
func TestParseIntraMutationParallelism(t *testing.T) {
	valid := []struct {
		in   string
		want IntraMutationParallelism
	}{
		// off, including the empty value a bare "key=" in the superflag yields.
		{"off", IntraMutationParallelism{}},
		{"", IntraMutationParallelism{}},
		{"OFF", IntraMutationParallelism{}},
		{"  off  ", IntraMutationParallelism{}},

		// auto is exactly 1x — not a separate mode.
		{"auto", IntraMutationParallelism{PerCPU: 1.0}},
		{"AUTO", IntraMutationParallelism{PerCPU: 1.0}},

		// Absolute worker counts. 0 is a legal spelling of off.
		{"0", IntraMutationParallelism{}},
		{"1", IntraMutationParallelism{Workers: 1}},
		{"30", IntraMutationParallelism{Workers: 30}},
		{"1024", IntraMutationParallelism{Workers: 1024}},

		// Per-CPU multipliers, including fractional and oversubscribed.
		{"1x", IntraMutationParallelism{PerCPU: 1}},
		{"2X", IntraMutationParallelism{PerCPU: 2}},
		{"1.5x", IntraMutationParallelism{PerCPU: 1.5}},
		{"0.5x", IntraMutationParallelism{PerCPU: 0.5}},
		{"0.25x", IntraMutationParallelism{PerCPU: 0.25}},
		{" 3x ", IntraMutationParallelism{PerCPU: 3}},
	}
	for _, tc := range valid {
		got, err := ParseIntraMutationParallelism(tc.in)
		require.NoErrorf(t, err, "input %q", tc.in)
		require.Equalf(t, tc.want, got, "input %q", tc.in)
	}

	invalid := []string{
		"x",                      // multiplier with no number
		"-1",                     // the removed AUTO sentinel must not silently mean anything
		"-1x",                    // negative multiplier
		"0x",                     // zero multiplier would mean "off", which has a spelling
		"abc",                    // not a number
		"1.5",                    // a float without the x is ambiguous: workers or multiplier?
		"1.5xx",                  // trailing junk
		"30 40",                  // two values
		"NaNx",                   // NaN must not slip through the f <= 0 comparison
		"Infx",                   // nor must +Inf
		"1e400x",                 // overflows to +Inf on parse
		"auto1x",                 // near-miss on a keyword
		"9999999999999999999999", // overflows an int
	}
	for _, in := range invalid {
		_, err := ParseIntraMutationParallelism(in)
		require.Errorf(t, err, "input %q must be rejected", in)
	}
}

// TestIntraMutationParallelismString checks the flag notation round-trips, since
// String() is what the startup WorkerConfig log line prints — an operator has to
// be able to match it against what they passed.
func TestIntraMutationParallelismString(t *testing.T) {
	for _, in := range []string{"off", "48", "1x", "1.5x", "0.25x"} {
		p, err := ParseIntraMutationParallelism(in)
		require.NoError(t, err)
		require.Equal(t, in, p.String())
	}

	// "auto" is the one input that does not round-trip verbatim: it renders as
	// the "1x" it means, which is the more informative of the two.
	p, err := ParseIntraMutationParallelism("auto")
	require.NoError(t, err)
	require.Equal(t, "1x", p.String())

	// The zero value must read as "off" rather than as an empty string.
	require.Equal(t, "off", IntraMutationParallelism{}.String())
}
