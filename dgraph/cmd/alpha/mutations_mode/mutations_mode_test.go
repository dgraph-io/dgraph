/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"strings"
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
)

// Tests in this file require a cluster running with the --mutations=<mode> option.

// Since this requires three alphas they will likely always be run with docker-compose,
// so no point in trying to use z.TestSockAddr here.
const disallowModeAlpha = "localhost:9180"
const strictModeAlphaGroup1 = "localhost:9182"
const strictModeAlphaGroup2 = "localhost:9183"

func runOn(conn *grpc.ClientConn, fn func(*testing.T, *dgo.Dgraph)) func(*testing.T) {
	return func(t *testing.T) {
		dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
		fn(t, dg)
	}
}

func dropAllDisallowed(t *testing.T, dg *dgo.Dgraph) {
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})

	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "no mutations allowed")
}

func dropAllAllowed(t *testing.T, dg *dgo.Dgraph) {
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})

	require.NoError(t, err)
}

func mutateNewDisallowed(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "Alice" .
		`),
	})

	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "no mutations allowed")
}

func mutateNewDisallowed2(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "Alice" .
		`),
	})

	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "schema not defined for predicate")
}

func addPredicateDisallowed(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	err := dg.Alter(ctx, &api.Operation{
		Schema: `name: string @index(exact) .`,
	})

	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "no mutations allowed")
}

func addPredicateAllowed1(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	err := dg.Alter(ctx, &api.Operation{
		Schema: `name: string @index(exact) .`,
	})

	require.NoError(t, err)
}

func addPredicateAllowed2(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	err := dg.Alter(ctx, &api.Operation{
		Schema: `size: string @index(exact) .`,
	})

	require.NoError(t, err)
}

func mutateExistingDisallowed(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <dgraph.xid> "XID00001" .
		`),
	})

	require.NoError(t, txn.Discard(ctx))
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "no mutations allowed")
}

func mutateExistingAllowed1(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "Alice" .
		`),
	})

	require.NoError(t, txn.Commit(ctx))
	require.NoError(t, err)
}

func mutateExistingAllowed2(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:s <size> "small" .
		`),
	})

	require.NoError(t, txn.Commit(ctx))
	require.NoError(t, err)
}

func TestMutationsDisallow(t *testing.T) {
	conn, err := grpc.Dial(disallowModeAlpha, grpc.WithInsecure())
	x.Check(err)
	defer conn.Close()

	t.Run("disallow drop all in no mutations mode",
		runOn(conn, dropAllDisallowed))
	t.Run("disallow mutate new predicate in no mutations mode",
		runOn(conn, mutateNewDisallowed))
	t.Run("disallow add predicate in no mutations mode",
		runOn(conn, addPredicateDisallowed))
	t.Run("disallow mutate existing predicate in no mutations mode",
		runOn(conn, mutateExistingDisallowed))
}

func TestMutationsStrict(t *testing.T) {
	conn1, err := grpc.Dial(strictModeAlphaGroup1, grpc.WithInsecure())
	x.Check(err)
	defer conn1.Close()

	conn2, err := grpc.Dial(strictModeAlphaGroup2, grpc.WithInsecure())
	x.Check(err)
	defer conn2.Close()

	t.Run("allow group1 drop all in strict mutations mode",
		runOn(conn1, dropAllAllowed))
	t.Run("allow group2 drop all in strict mutations mode",
		runOn(conn2, dropAllAllowed))
	t.Run("disallow group1 mutate new predicate in strict mutations mode",
		runOn(conn1, mutateNewDisallowed2))
	t.Run("disallow group2 mutate new predicate in strict mutations mode",
		runOn(conn2, mutateNewDisallowed2))
	t.Run("allow group1 add predicate in strict mutations mode",
		runOn(conn1, addPredicateAllowed1))
	t.Run("allow group2 add predicate in strict mutations mode",
		runOn(conn2, addPredicateAllowed2))
	t.Run("allow group1 mutate group1 predicate in strict mutations mode",
		runOn(conn1, mutateExistingAllowed1))
	t.Run("allow group2 mutate group1 predicate in strict mutations mode",
		runOn(conn2, mutateExistingAllowed1))
	t.Run("allow group1 mutate group2 predicate in strict mutations mode",
		runOn(conn1, mutateExistingAllowed2))
	t.Run("allow group2 mutate group2 predicate in strict mutations mode",
		runOn(conn2, mutateExistingAllowed2))
}
