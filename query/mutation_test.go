//go:build integration || upgrade

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/dgraphtest"
)

func TestReservedPredicateForMutation(t *testing.T) {
	err := addTriplesToCluster(`_:x <dgraph.graphql.schema> "df"`)
	require.Error(t, err, "Cannot mutate graphql reserved predicate dgraph.graphql.schema")
}

func TestAlteringReservedTypesAndPredicatesShouldFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	op := &api.Operation{Schema: `
		type dgraph.Person {
			name: string
			age: int
		}
		name: string .
		age: int .
	`}
	err := client.Alter(context.Background(), op)
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
	err = client.Alter(ctx, op)
	require.Error(t, err, "altering predicate in dgraph namespace shouldn't have succeeded")
	require.Contains(t, err.Error(), "Can't alter predicate `dgraph.name` as it is prefixed with "+
		"`dgraph.` which is reserved as the namespace for dgraph's internal types/predicates.")

	_, err = client.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`_:new <dgraph.name> "Alice" .`),
	})
	require.Error(t, err, "storing predicate in dgraph namespace shouldn't have succeeded")
	require.Contains(t, err.Error(), "Can't store predicate `dgraph.name` as it is prefixed with "+
		"`dgraph.` which is reserved as the namespace for dgraph's internal types/predicates.")
}

func TestUnreservedPredicateForDeletion(t *testing.T) {
	// This bug was fixed in commit 8631dab37c951b288f839789bbabac5e7088b58f
	dgraphtest.ShouldSkipTest(t, "8631dab37c951b288f839789bbabac5e7088b58f", dc.GetVersion())

	grootUserQuery := `
	{
		grootUser(func:eq(dgraph.xid, "groot")){
			uid
		}
	}`
	type userQryResp struct {
		GrootUser []struct {
			Uid string `json:"uid"`
		} `json:"grootUser"`
	}
	resp, err := client.Query(grootUserQuery)
	require.NoError(t, err, "groot user query failed")

	var userResp userQryResp
	require.NoError(t, json.Unmarshal(resp.GetJson(), &userResp))
	grootsUidStr := userResp.GrootUser[0].Uid
	grootsUidInt, err := strconv.ParseUint(grootsUidStr, 0, 64)
	require.NoError(t, err)
	require.Greater(t, uint64(grootsUidInt), uint64(0))

	const testSchema = `
		type Star {
			name
		}

		name : string @index(term, exact, trigram) @count @lang .`
	setSchema(testSchema)

	triple := fmt.Sprintf(`<%s> <dgraphcore_355> "Betelgeuse" .`, grootsUidStr)
	require.NoError(t, addTriplesToCluster(triple))
	deleteTriplesInCluster(triple)
}
