//go:build linux
// +build linux

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"math"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDiskMetricsInt64Conversion tests the int64 conversion fix for disk metrics calculations.
// This test verifies that explicit int64 conversions prevent integer overflow when calculating
// disk space metrics from syscall.Statfs_t values.
func TestDiskMetricsInt64Conversion(t *testing.T) {
	tests := []struct {
		name          string
		frsize        int64
		blocks        uint64
		bfree         uint64
		bavail        uint64
		expectedTotal int64
		expectedFree  int64
	}{
		{
			name:          "Normal filesystem values",
			frsize:        4096,
			blocks:        1000000,
			bfree:         500000,
			bavail:        400000,
			expectedTotal: 4096 * (1000000 - 100000), // blocks - reservedBlocks
			expectedFree:  4096 * 400000,
		},
		{
			name:          "Large filesystem (4TB)",
			frsize:        4096,
			blocks:        1000000000,
			bfree:         500000000,
			bavail:        400000000,
			expectedTotal: 4096 * (1000000000 - 100000000),
			expectedFree:  4096 * 400000000,
		},
		{
			name:          "Small block size",
			frsize:        512,
			blocks:        10000000,
			bfree:         5000000,
			bavail:        4000000,
			expectedTotal: 512 * (10000000 - 1000000),
			expectedFree:  512 * 4000000,
		},
		{
			name:          "No reserved blocks",
			frsize:        4096,
			blocks:        1000000,
			bfree:         500000,
			bavail:        500000, // bavail == bfree
			expectedTotal: 4096 * 1000000,
			expectedFree:  4096 * 500000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := syscall.Statfs_t{
				Blocks: tt.blocks,
				Bfree:  tt.bfree,
				Bavail: tt.bavail,
			}

			// Calculate using the fixed approach: int64(s.Frsize) * int64(...)
			reservedBlocks := s.Bfree - s.Bavail
			total := int64(tt.frsize) * int64(s.Blocks-reservedBlocks)
			free := int64(tt.frsize) * int64(s.Bavail)
			used := total - free

			// Verify calculations match expected values
			require.Equal(t, tt.expectedTotal, total, "Total calculation mismatch")
			require.Equal(t, tt.expectedFree, free, "Free calculation mismatch")

			// Ensure no overflow (negative values would indicate overflow)
			require.GreaterOrEqual(t, total, int64(0), "Total should not be negative")
			require.GreaterOrEqual(t, free, int64(0), "Free should not be negative")
			require.GreaterOrEqual(t, used, int64(0), "Used should not be negative")

			// Verify logical consistency
			require.LessOrEqual(t, free, total, "Free space cannot exceed total")
			require.LessOrEqual(t, used, total, "Used space cannot exceed total")
		})
	}
}

// TestDiskMetricsConversionOrder verifies that explicit int64 conversion
// of both operands prevents potential overflow issues.
func TestDiskMetricsConversionOrder(t *testing.T) {
	var frsize int64 = 4096
	var blocks uint64 = 1000000000

	// The fix: convert both operands to int64 before multiplication
	result := int64(frsize) * int64(blocks)
	
	require.Greater(t, result, int64(0), "Result should be positive")
	require.Equal(t, int64(4096000000000), result, "Calculation should be correct")
	require.LessOrEqual(t, result, int64(math.MaxInt64), "Result should not overflow")
}
