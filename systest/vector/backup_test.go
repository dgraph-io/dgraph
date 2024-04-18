//go:build !oss && integration

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func TestVectorIncrBackupRestore(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace))

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphtest.DefaultUser,
		dgraphtest.DefaultPassword, x.GalaxyNamespace))

	require.NoError(t, gc.SetupSchema(testSchema))

	numVectors := 500
	pred := "project_discription_v"
	allVectors := make([][][]float32, 0, 5)
	allRdfs := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		var rdfs string
		var vectors [][]float32
		rdfs, vectors = dgraphtest.GenerateRandomVectors(numVectors*(i-1), numVectors*i, 1, pred)
		allVectors = append(allVectors, vectors)
		allRdfs = append(allRdfs, rdfs)
		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err := gc.Mutate(mu)
		require.NoError(t, err)

		t.Logf("taking backup #%v\n", i)
		require.NoError(t, hc.Backup(c, i == 1, dgraphtest.DefaultBackupDir))
	}

	for i := 0; i < 5; i++ {
		t.Logf("restoring backup #%v\n", i)

		incrFrom := i - 1
		require.NoError(t, hc.Restore(c, dgraphtest.DefaultBackupDir, "", incrFrom, i))
		require.NoError(t, dgraphtest.WaitForRestore(c))

		// rebuild index
		require.NoError(t, gc.SetupSchema(testSchemaWithoutIndex))
		time.Sleep(2 * time.Minute)
		require.NoError(t, gc.SetupSchema(testSchema))
		time.Sleep(2 * time.Minute)

		query := `{
			vector(func: has(project_discription_v)) {
				   count(uid)
				}
		}`

		result, err := gc.Query(query)
		require.NoError(t, err)

		if i == 0 {
			require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors*5), string(result.GetJson()))
			var a [][]float32
			for _, vecArr := range allVectors {
				a = append(a, vecArr...)
			}

			for p, vector := range allVectors[i] {
				triple := strings.Split(allRdfs[i], "\n")[p]
				uid := strings.Split(triple, " ")[0]
				queriedVector, err := gc.QuerySingleVectorsUsingUid(uid, pred)
				require.NoError(t, err)
				require.Equal(t, allVectors[i][p], queriedVector[0])

				similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, numVectors)
				require.NoError(t, err)
				require.Equal(t, numVectors, len(similarVectors))

				for _, similarVector := range similarVectors {
					require.Contains(t, a, similarVector)
				}
			}

		} else {
			require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))
			for p, vector := range allVectors[i-1] {
				triple := strings.Split(allRdfs[i-1], "\n")[p]
				uid := strings.Split(triple, " ")[0]
				queriedVector, err := gc.QuerySingleVectorsUsingUid(uid, pred)
				require.NoError(t, err)

				require.Equal(t, allVectors[i-1][p], queriedVector[0])

				similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, numVectors)
				require.NoError(t, err)
				require.Equal(t, numVectors, len(similarVectors))

				for _, similarVector := range similarVectors {
					require.Contains(t, allVectors[i-1], similarVector)
				}
			}
		}
	}
}

func TestVectorBackupRestore(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace))

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphtest.DefaultUser,
		dgraphtest.DefaultPassword, x.GalaxyNamespace))

	require.NoError(t, gc.SetupSchema(testSchema))

	numVectors := 1000
	pred := "project_discription_v"
	rdfs, vectors := dgraphtest.GenerateRandomVectors(0, numVectors, 10, pred)

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	t.Log("taking backup \n")
	require.NoError(t, hc.Backup(c, false, dgraphtest.DefaultBackupDir))

	t.Log("restoring backup \n")
	require.NoError(t, hc.Restore(c, dgraphtest.DefaultBackupDir, "", 0, 0))
	require.NoError(t, dgraphtest.WaitForRestore(c))

	// rebuild index
	require.NoError(t, gc.SetupSchema(testSchemaWithoutIndex))
	time.Sleep(2 * time.Minute)
	require.NoError(t, gc.SetupSchema(testSchema))
	time.Sleep(2 * time.Minute)

	testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)
}
