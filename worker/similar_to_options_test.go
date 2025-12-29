/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSimilarToOptions_ValidEf(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"ef", "64"}, fc)
	require.NoError(t, err)
	require.Equal(t, 64, fc.vsEfOverride)
	require.Nil(t, fc.vsDistanceThreshold)
}

func TestParseSimilarToOptions_ValidDistanceThreshold(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"distance_threshold", "0.45"}, fc)
	require.NoError(t, err)
	require.Equal(t, 0, fc.vsEfOverride)
	require.NotNil(t, fc.vsDistanceThreshold)
	require.Equal(t, 0.45, *fc.vsDistanceThreshold)
}

func TestParseSimilarToOptions_BothOptions(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"ef", "32", "distance_threshold", "1.5"}, fc)
	require.NoError(t, err)
	require.Equal(t, 32, fc.vsEfOverride)
	require.NotNil(t, fc.vsDistanceThreshold)
	require.Equal(t, 1.5, *fc.vsDistanceThreshold)
}

func TestParseSimilarToOptions_EmptyArgs(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions(nil, fc)
	require.NoError(t, err)
	require.Equal(t, 0, fc.vsEfOverride)
	require.Nil(t, fc.vsDistanceThreshold)
}

func TestParseSimilarToOptions_DuplicateKey(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"ef", "10", "ef", "20"}, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Duplicate key in similar_to options")
	require.Contains(t, err.Error(), "ef")
}

func TestParseSimilarToOptions_UnknownKey(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"unknown_key", "123"}, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unknown option in similar_to")
	require.Contains(t, err.Error(), "unknown_key")
}

func TestParseSimilarToOptions_MalformedOption_OddArgs(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"ef"}, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Malformed option in similar_to")
}

func TestParseSimilarToOptions_InvalidEfValue(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"ef", "abc"}, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid value for 'ef'")
}

func TestParseSimilarToOptions_NegativeEf(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"ef", "-5"}, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Value for 'ef' must be positive")
}

func TestParseSimilarToOptions_ZeroEf(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"ef", "0"}, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Value for 'ef' must be positive")
}

func TestParseSimilarToOptions_InvalidDistanceThreshold(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"distance_threshold", "notanumber"}, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid value for 'distance_threshold'")
}

func TestParseSimilarToOptions_NegativeDistanceThreshold(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"distance_threshold", "-0.5"}, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Value for 'distance_threshold' must be non-negative")
}

func TestParseSimilarToOptions_WhitespaceHandling(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"  ef  ", " 64 ", " distance_threshold ", " 0.5  "}, fc)
	require.NoError(t, err)
	require.Equal(t, 64, fc.vsEfOverride)
	require.NotNil(t, fc.vsDistanceThreshold)
	require.Equal(t, 0.5, *fc.vsDistanceThreshold)
}

func TestParseSimilarToOptions_QuotedValues(t *testing.T) {
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"ef", `"64"`}, fc)
	require.NoError(t, err)
	require.Equal(t, 64, fc.vsEfOverride)
}

func TestParseSimilarToOptions_MisspelledKey(t *testing.T) {
	// Ensure typos like "distanc_threshold" are caught
	fc := &functionContext{}
	err := parseSimilarToOptions([]string{"distanc_threshold", "0.5"}, fc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unknown option in similar_to")
	require.Contains(t, err.Error(), "distanc_threshold")
}
