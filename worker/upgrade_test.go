//go:build upgrade

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

package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/dgraphtest"
	"github.com/dgraph-io/dgraph/v24/testutil"
	"github.com/dgraph-io/dgraph/v24/x"
	"github.com/stretchr/testify/require"
)

var client *dgraphapi.GrpcClient
var (
	schemaIndexed    = `friend: [uid] @count @reverse .`
	schemaNonIndexed = `friend: [uid] .`
)

func setClusterEdge(t *testing.T, rdf string) error {
	mu := &api.Mutation{SetNquads: []byte(rdf), CommitNow: true}
	return testutil.RetryMutation(client.Dgraph, mu)
}

func delClusterEdge(t *testing.T, rdf string) error {
	mu := &api.Mutation{DelNquads: []byte(rdf), CommitNow: true}
	return testutil.RetryMutation(client.Dgraph, mu)
}

func setSchema(schema string) {
	var err error
	for retry := 0; retry < 60; retry++ {
		err = client.Alter(context.Background(), &api.Operation{Schema: schema})
		if err == nil {
			return
		}
		time.Sleep(time.Second)
	}
	panic(fmt.Sprintf("Could not alter schema. Got error %v", err.Error()))
}

func populateCluster(t *testing.T, dc dgraphapi.Cluster) {
	x.Panic(client.Alter(context.Background(), &api.Operation{DropAll: true}))
	x.Panic(dc.AssignUids(client.Dgraph, 65536))

	setSchema(schemaIndexed)

	require.NoError(t, delClusterEdge(t, fmt.Sprintf("<%#x> <friend> <%#x> .", 1, 2)))
}

func TestCountReverseIndex(t *testing.T) {
	uc := &dgraphtest.UpgradeCombo{
		Before:   "v24.0.4",
		After:    "local",
		Strategy: dgraphtest.InPlace,
	}

	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).
		WithReplicas(1).WithACL(time.Hour).WithVersion(uc.Before)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	fmt.Println(c)

	var code = 2

	defer func() { c.Cleanup(code != 0) }()
	x.Panic(c.Start())

	dg, cleanup, err := c.Client()
	client = dg
	x.Panic(err)
	defer cleanup()
	x.Panic(dg.LoginIntoNamespace(context.Background(), dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.GalaxyNamespace))

	populateCluster(t, c)

	err = delClusterEdge(t, fmt.Sprintf("<%#x> <friend> <%#x> .", 1, 2))
	require.Contains(t, err.Error(), "Transaction is too old")

	x.Panic(c.Upgrade(uc.After, uc.Strategy))

	dg, cleanup, err = c.Client()
	client = dg
	x.Panic(err)
	defer cleanup()
	x.Panic(dg.LoginIntoNamespace(context.Background(), dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.GalaxyNamespace))

	setSchema(schemaNonIndexed)
	setSchema(schemaIndexed)

	require.NoError(t, delClusterEdge(t, fmt.Sprintf("<%#x> <friend> <%#x> .", 1, 2)))
	require.NoError(t, delClusterEdge(t, fmt.Sprintf("<%#x> <friend> <%#x> .", 1, 2)))
	require.NoError(t, delClusterEdge(t, fmt.Sprintf("<%#x> <friend> <%#x> .", 1, 2)))
	require.NoError(t, delClusterEdge(t, fmt.Sprintf("<%#x> <friend> <%#x> .", 1, 2)))

	code = 0
}
