//go:build integration2

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
	"github.com/hypermodeinc/dgraph/v25/x"
)

// func addData(gc *dgraphapi.GrpcClient, pred string, start, end int) error {
// 	if err := gc.SetupSchema(fmt.Sprintf(`%v: string @index(exact) .`, pred)); err != nil {
// 		return err
// 	}

// 	rdf := ""
// 	for i := start; i <= end; i++ {
// 		rdf = rdf + fmt.Sprintf("_:a%v <%v> \"%v%v\" .	\n", i, pred, pred, i)
// 	}
// 	_, err := gc.Mutate(&api.Mutation{SetNquads: []byte(rdf), CommitNow: true})
// 	return err
// }

func commonTest(t *testing.T, existingCluster, freshCluster *dgraphtest.LocalCluster) {
	hc, err := existingCluster.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	gc, cleanup, err := existingCluster.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gc.Login(context.Background(), dgraphapi.DefaultUser, dgraphapi.DefaultPassword))

	namespaces := []uint64{0}
	require.NoError(t, dgraphtest.AddData(gc, "pred", 1, 100))
	for i := 1; i <= 5; i++ {
		ns, err := hc.AddNamespace()
		require.NoError(t, err)
		namespaces = append(namespaces, ns)
		require.NoError(t, gc.LoginIntoNamespace(context.Background(),
			dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))
		require.NoError(t, dgraphtest.AddData(gc, "pred", 1, 100+int(ns)))
	}

	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))
	require.NoError(t, hc.Backup(existingCluster, false, dgraphtest.DefaultBackupDir))

	restoreNamespaces := func(c *dgraphtest.LocalCluster) {
		hc, err := c.HTTPClient()
		require.NoError(t, err)
		require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

		for _, ns := range namespaces {
			require.NoError(t, hc.RestoreTenant(c, dgraphtest.DefaultBackupDir, "", 0, 0, ns))
			require.NoError(t, dgraphapi.WaitForRestore(c))

			gc, cleanup, err = c.Client()
			require.NoError(t, err)
			defer cleanup()

			// Only the namespace '0' should have data
			require.NoError(t, gc.LoginIntoNamespace(context.Background(),
				dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))
			const query = `{
			           all(func: has(pred)) {
			                 	count(uid)
			                }
	                   	}`
			resp, err := gc.Query(query)
			require.NoError(t, err)
			require.NoError(t, dgraphapi.CompareJSON(fmt.Sprintf(`{"all":[{"count":%v}]}`, 100+ns), string(resp.Json)))

			// other namespaces should have no data
			for _, ns2 := range namespaces[1:] {
				require.Error(t, gc.LoginIntoNamespace(context.Background(),
					dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns2))
			}
		}
	}

	t.Log("restoring on existing cluster")
	restoreNamespaces(existingCluster)

	t.Log("restoring on fresh cluster")
	restoreNamespaces(freshCluster)
}

func commonIncRestoreTest(t *testing.T, existingCluster, freshCluster *dgraphtest.LocalCluster) {
	hc, err := existingCluster.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	gc, cleanup, err := existingCluster.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gc.Login(context.Background(), dgraphapi.DefaultUser, dgraphapi.DefaultPassword))

	require.NoError(t, gc.DropAll())
	require.NoError(t, dgraphtest.AddData(gc, "pred", 1, 100))

	namespaces := []uint64{}
	for i := 1; i <= 5; i++ {
		ns, err := hc.AddNamespace()
		require.NoError(t, err)
		namespaces = append(namespaces, ns)
	}

	for j := 0; j < 5; j++ {
		for i, ns := range namespaces {
			require.NoError(t, gc.LoginIntoNamespace(context.Background(),
				dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))
			start := i*20 + 1
			end := (i + 1) * 20
			require.NoError(t, dgraphtest.AddData(gc, "pred", start, end))
		}

		require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))
		require.NoError(t, hc.Backup(existingCluster, j == 0, dgraphtest.DefaultBackupDir))
	}

	restoreNamespaces := func(c *dgraphtest.LocalCluster) {
		hc, err := c.HTTPClient()
		require.NoError(t, err)
		require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))
		for _, ns := range namespaces {
			for j := 0; j < 5; j++ {
				incrFrom := j + 1
				if incrFrom == 1 {
					incrFrom = 0
				}

				require.NoError(t, hc.RestoreTenant(c, dgraphtest.DefaultBackupDir, "", incrFrom, j+1, ns))
				require.NoError(t, dgraphapi.WaitForRestore(c))

				gc, cleanup, err = c.Client()
				require.NoError(t, err)
				defer cleanup()

				require.NoError(t, gc.Login(context.Background(), dgraphapi.DefaultUser, dgraphapi.DefaultPassword))
				const query = `{
				all(func: has(pred)) {
					count(uid)
				}
			}`
				resp, err := gc.Query(query)
				require.NoError(t, err)
				require.NoError(t, dgraphapi.CompareJSON(fmt.Sprintf(`{"all":[{"count":%v}]}`, 20*(j+1)), string(resp.Json)))
			}
		}
	}

	t.Log("restoring on fresh cluster")
	restoreNamespaces(existingCluster)

	t.Log("restoring on fresh cluster")
	restoreNamespaces(freshCluster)
}

func TestNameSpaceAwareRestoreOnSingleNode(t *testing.T) {
	baseClusterConf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(3).
		WithReplicas(3).WithACL(20 * time.Hour).WithEncryption().WithUidLease(1000)
	baseCluster, err := dgraphtest.NewLocalCluster(baseClusterConf)
	require.NoError(t, err)
	defer func() { baseCluster.Cleanup(t.Failed()) }()
	require.NoError(t, baseCluster.Start())

	freshClusterConf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(3).
		WithReplicas(3).WithACL(20*time.Hour).WithEncryption().WithUidLease(1000).
		WithAlphaVolume(baseClusterConf.GetClusterVolume(dgraphtest.DefaultBackupDir), dgraphtest.DefaultBackupDir)
	freshCluster, err := dgraphtest.NewLocalCluster(freshClusterConf)
	require.NoError(t, err)
	defer func() { freshCluster.Cleanup(t.Failed()) }()
	require.NoError(t, freshCluster.Start())

	commonTest(t, baseCluster, freshCluster)
	commonIncRestoreTest(t, baseCluster, freshCluster)
}

func TestNamespaceAwareRestoreOnMultipleGroups(t *testing.T) {
	baseClusterConf := dgraphtest.NewClusterConfig().WithNumAlphas(9).WithNumZeros(3).
		WithReplicas(3).WithACL(20 * time.Hour).WithEncryption().WithUidLease(10000)
	baseCluster, err := dgraphtest.NewLocalCluster(baseClusterConf)
	require.NoError(t, err)
	defer func() { baseCluster.Cleanup(t.Failed()) }()
	require.NoError(t, baseCluster.Start())

	freshClusterConf := dgraphtest.NewClusterConfig().WithNumAlphas(9).WithNumZeros(3).
		WithReplicas(3).WithACL(20*time.Hour).WithEncryption().WithUidLease(10000).
		WithAlphaVolume(baseClusterConf.GetClusterVolume(dgraphtest.DefaultBackupDir), dgraphtest.DefaultBackupDir)
	freshCluster, err := dgraphtest.NewLocalCluster(freshClusterConf)
	require.NoError(t, err)
	defer func() { freshCluster.Cleanup(t.Failed()) }()
	require.NoError(t, freshCluster.Start())

	commonTest(t, baseCluster, freshCluster)
	commonIncRestoreTest(t, baseCluster, freshCluster)
}
