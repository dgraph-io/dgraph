//go:build integration2

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors *
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

package main

import (
	"context"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/dgraphtest"
	"github.com/dgraph-io/dgraph/v24/x"
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
	hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

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
