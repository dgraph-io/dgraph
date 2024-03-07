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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

type Node struct {
	Uid       string    `json:"uid"`
	Namespace string    `json:"namespace"`
	Vtest     []float32 `json:"vtest"`
}

// var testVectors = [][]float32{
// 	{1.9, 4.5, 3.3, 2.1},
// 	{4.4, 3.2, 2.3, 1.7},
// 	{1.6, 4.2, 3.9, 2.3},
// 	{2.3, 1.8, 4.7, 3.2},
// 	{4.2, 3, 2, 1.6},
// 	{2.3, 3.5, 1.6, 4},
// 	{1.5, 2, 3, 4.5},
// 	{4.6, 3.9, 2.1, 1.4},
// 	{4.3, 2.7, 1.9, 3.8},
// 	{4.2, 2.5, 2.7, 1.5},
// 	{2.1, 3.7, 1.8, 4.3},
// 	{1.8, 4.2, 3.1, 2.5},
// 	{2.6, 3.8, 1.5, 4.1},
// 	{3.7, 1.9, 4.5, 2.4},
// 	{2, 3.1, 1.7, 4.6},
// 	{3.9, 1.6, 4, 2.2},
// 	{3.3, 1.5, 4.4, 2.8},
// 	{3.5, 1.6, 4.2, 2.9},
// 	{1.7, 4.4, 3.8, 2},
// 	{3.2, 4.1, 2.6, 1.9},
// }

func TestLiveLoadAndExportRDFFormat(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	testExportAndLiveLoad(t, c, "rdf")
}

func TestLiveLoadAndExportJsonFormat(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	testExportAndLiveLoad(t, c, "json")
}

func testExportAndLiveLoad(t *testing.T, c *dgraphtest.LocalCluster, exportFormat string) {
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

	numVectors := 100
	pred := "project_discription_v"
	rdfs, vectors := dgraphtest.GenerateRandomVectors(0, numVectors, 10, pred)

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	t.Log("taking export \n")

	require.NoError(t, hc.Export(dgraphtest.DefaultExportDir, exportFormat, -1))

	require.NoError(t, gc.DropAll())

	query := `{
		vector(func: has(project_discription_v)) {
			   count(uid)
			}
	}`

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, 0), string(result.GetJson()))

	require.NoError(t, c.LiveLoadFromExport(dgraphtest.DefaultExportDir))

	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace))

	result, err = gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)
}

func TestBulkLoader(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(2).WithNumZeros(1).
		WithACL(time.Hour).WithReplicas(1).WithBulkLoadOutDir(t.TempDir())
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()

	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(true))

	baseDir := t.TempDir()
	gqlSchemaFile := filepath.Join(baseDir, "gql.schema")
	require.NoError(t, os.WriteFile(gqlSchemaFile, []byte(testSchema), os.ModePerm))
	dataFile := filepath.Join(baseDir, "data.rdf")
	numvectors := 100
	pred := "project_discription_v"
	rdfs, vectors := dgraphtest.GenerateRandomVectors(0, numvectors, 10, pred)
	require.NoError(t, os.WriteFile(dataFile, []byte(rdfs), os.ModePerm))

	opts := dgraphtest.BulkOpts{
		DataFiles:   []string{dataFile},
		SchemaFiles: []string{gqlSchemaFile},
	}
	require.NoError(t, c.BulkLoad(opts))

	// start Alphas
	require.NoError(t, c.Start())

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphtest.DefaultUser,
		dgraphtest.DefaultPassword, x.GalaxyNamespace))
	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace))

	query := fmt.Sprintf(`{
			vector(func: has(%v)) {
				   count(uid)
				}
		}`, pred)

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numvectors), string(result.GetJson()))

	for _, vector := range vectors {
		similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, numvectors)
		require.NoError(t, err)
		for _, similarVector := range similarVectors {
			require.Contains(t, vectors, similarVector)
		}
	}
}
