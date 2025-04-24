//go:build integration2

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"testing"
	"time"

	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
	"github.com/hypermodeinc/dgraph/v25/x"
	"github.com/stretchr/testify/require"
)

func TestSnapshotTranferAfterNewNodeJoins(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(1).
		WithACL(time.Hour).WithReplicas(3).WithSnapshotConfig(11, time.Second)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()

	// start zero
	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(true))
	require.NoError(t, c.StartAlpha(0))
	require.NoError(t, c.HealthCheck(false))

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	prevSnapshotTs, err := hc.GetCurrentSnapshotTs(1)
	require.NoError(t, err)

	dgraphtest.AddData(gc, "name", 1, 20)

	_, err = hc.WaitForSnapshot(1, prevSnapshotTs)
	require.NoError(t, err)

	require.NoError(t, c.StartAlpha(1))
	require.NoError(t, c.StartAlpha(2))

	// Wait for the other alpha nodes to receive the snapshot from the leader alpha.
	// If they are healthy, there should be no issues with the snapshot streaming
	time.Sleep(time.Second)
	require.NoError(t, c.HealthCheck(false))
}
