//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/stretchr/testify/require"
)

// TestZeroAddrReconciliation verifies that when a Zero node restarts with a
// different --my address, the new address is propagated into the cluster's
// MembershipState via ConfChangeUpdateNode. This is a regression test for the
// bug where the initial bootstrap address baked into the WAL would persist
// indefinitely, causing Alphas to connect to a stale Zero address.
func TestZeroAddrReconciliation(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()

	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(true))
	require.NoError(t, c.StartAlpha(0))
	require.NoError(t, c.HealthCheck(false))

	// Verify the initial Zero address in MembershipState is the container alias.
	initialAddr, err := getZeroAddr(c, 0)
	require.NoError(t, err)
	require.Contains(t, initialAddr, "zero0")
	t.Logf("Initial Zero address: %s", initialAddr)

	// Stop Zero, change its --my to a different address. We use the container's
	// full Docker name (also a valid DNS alias on the network) with the real
	// gRPC port, so the address is reachable but differs from the initial alias.
	require.NoError(t, c.StopZero(0))
	require.NoError(t, c.StopAlpha(0))

	zeroContainerName, err := c.GetZeroContainerName(0)
	require.NoError(t, err)
	newAddr := zeroContainerName + ":5080"
	t.Logf("New --my address: %s", newAddr)
	require.NoError(t, c.SetZeroMyAddr(0, newAddr))
	require.NoError(t, c.RecreateZero(0))
	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(true))

	// The reconciliation runs on leader election and periodically. Poll until
	// the MembershipState reflects the new address.
	var reconciledAddr string
	require.Eventually(t, func() bool {
		addr, err := getZeroAddr(c, 0)
		if err != nil {
			t.Logf("Polling /state: %v", err)
			return false
		}
		reconciledAddr = addr
		t.Logf("Polling /state: Zero addr = %q (want %q)", addr, newAddr)
		return reconciledAddr == newAddr
	}, 30*time.Second, time.Second, "expected Zero addr to reconcile to %q, last seen %q", newAddr, reconciledAddr)

	t.Logf("Reconciled Zero address: %s", reconciledAddr)

	// Restart Alpha and verify it connects successfully. The Alpha's Connect
	// RPC receives the corrected address from MembershipState.
	require.NoError(t, c.StartAlpha(0))
	require.NoError(t, c.HealthCheck(false))
}

// getZeroAddr queries the Zero /state endpoint and returns the address
// recorded for the first Zero in MembershipState.
func getZeroAddr(c *dgraphtest.LocalCluster, zeroIdx int) (string, error) {
	stateURL, err := c.GetZeroStateURL(zeroIdx)
	if err != nil {
		return "", fmt.Errorf("GetZeroStateURL: %w", err)
	}

	resp, err := http.Get(stateURL)
	if err != nil {
		return "", fmt.Errorf("HTTP GET %s: %w", stateURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading body: %w", err)
	}

	var state struct {
		Zeros map[string]struct {
			Addr string `json:"addr"`
		} `json:"zeros"`
	}
	if err := json.Unmarshal(body, &state); err != nil {
		return "", fmt.Errorf("unmarshal /state: %w (body: %s)", err, string(body))
	}

	for _, z := range state.Zeros {
		return z.Addr, nil
	}
	return "", fmt.Errorf("no zero found in /state response")
}
