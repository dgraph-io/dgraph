/*
 * Copyright 2016-2022 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
)

func TestReserverPredicateForMutation(t *testing.T) {
	err := addTriplesToCluster(`_:x <dgraph.graphql.schema> "df"`)
	require.Error(t, err, "Cannot mutate graphql reserved predicate dgraph.graphql.schema")
}

func TestAlteringReservedTypesAndPredicatesShouldFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	op := &api.Operation{Schema: `
		type dgraph.Person {
			name: string
			age: int
		}
		name: string .
		age: int .
	`}
	err = dg.Alter(ctx, op)
	require.Error(t, err, "altering type in dgraph namespace shouldn't have succeeded")
	require.Contains(t, err.Error(), "Can't alter type `dgraph.Person` as it is prefixed with "+
		"`dgraph.` which is reserved as the namespace for dgraph's internal types/predicates.")

	op = &api.Operation{Schema: `
		type Person {
			dgraph.name
			age
		}
		dgraph.name: string .
		age: int .
	`}
	err = dg.Alter(ctx, op)
	require.Error(t, err, "altering predicate in dgraph namespace shouldn't have succeeded")
	require.Contains(t, err.Error(), "Can't alter predicate `dgraph.name` as it is prefixed with "+
		"`dgraph.` which is reserved as the namespace for dgraph's internal types/predicates.")

	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`_:new <dgraph.name> "Alice" .`),
	})
	require.Error(t, err, "storing predicate in dgraph namespace shouldn't have succeeded")
	require.Contains(t, err.Error(), "Can't store predicate `dgraph.name` as it is prefixed with "+
		"`dgraph.` which is reserved as the namespace for dgraph's internal types/predicates.")
}
