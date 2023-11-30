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

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

func TestACLDuplicateGrootUser(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphtest.DefaultUser, dgraphtest.DefaultPassword, x.GalaxyNamespace))

	rdfs := `_:a <dgraph.xid> "groot" .
	         _:a <dgraph.type> "dgraph.type.User"  .`

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.Error(t, err)
	require.ErrorContains(t, err, "could not insert duplicate value [groot] for predicate [dgraph.xid]")
}
