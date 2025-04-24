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

func TestShortestPath(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	err = c.LiveLoad(dgraphtest.LiveOpts{
		DataFiles:      []string{"graph.rdf.gz"},
		SchemaFiles:    []string{"graph.schema.gz"},
		GqlSchemaFiles: []string{},
	})
	require.NoError(t, err)

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	_, err = gc.Query(`
	{
		q(func: eq(guid, "85270d10-560e-4cc8-8703-4b4c563a2f4e")) {
		 	a as uid
  		}
  		q1(func: eq(guid, "4a520068-80b6-42f2-9019-4e6ef8a02bb3")) {
			b as uid
  		}
  
 		path as shortest(from: uid(a), to: uid(b), numpaths: 5, maxfrontiersize: 10000) {
  			connected_to @facets(weight)
 		}

 		path(func: uid(path)) {
   			uid
 		}
	}
	`)
	require.NoError(t, err)
}
