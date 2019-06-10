/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
)

var (
	ctxb = context.Background()

	countQuery = `
query countAnswers($num: int) {
  me(func: eq(count(answer), $num)) {
    uid
    count(answer)
  }
}
`
)

func init() {
	assignUids(200)
}

func TestCountIndexConcurrentTxns(t *testing.T) {
	dg := z.DgraphClientWithGroot(z.SockAddr) //dgraphClient()
	z.DropAll(t, dg)
	alterSchema(dg, "answer: [uid] @count .")

	// Expected edge count of 0x100: 1
	txn0 := dg.NewTxn()
	mu := api.Mutation{SetNquads: []byte("<0x100> <answer> <0x200> .")}
	_, err := txn0.Mutate(ctxb, &mu)
	check(err)
	err = txn0.Commit(ctxb)
	check(err)

	// Expected edge count of 0x1: 2
	// This should NOT appear in the query result
	// The following two mutations are in separate interleaved txns.
	txn1 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x2> .")}
	_, err = txn1.Mutate(ctxb, &mu)
	check(err)

	txn2 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x3> .")}
	_, err = txn2.Mutate(ctxb, &mu)
	check(err)

	err = txn1.Commit(ctxb)
	check(err)
	err = txn2.Commit(ctxb)
	require.Error(t, err,
		"the txn2 should be aborted due to concurrent update on the count index of	<0x01>")

	// retry the mutatiton
	txn3 := dg.NewTxn()
	_, err = txn3.Mutate(ctxb, &mu)
	check(err)
	err = txn3.Commit(ctxb)
	check(err)

	// Verify count queries
	txn := dg.NewReadOnlyTxn()
	vars := map[string]string{"$num": "1"}
	resp, err := txn.QueryWithVars(ctxb, countQuery, vars)
	check(err)
	js := string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 1, "uid": "0x100"}]}`,
		js)
	txn = dg.NewReadOnlyTxn()
	vars = map[string]string{"$num": "2"}
	resp, err = txn.QueryWithVars(ctxb, countQuery, vars)
	check(err)
	js = string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 2, "uid": "0x1"}]}`,
		js)
}

func TestCountIndexSerialTxns(t *testing.T) {
	dg := z.DgraphClientWithGroot(z.SockAddr)
	z.DropAll(t, dg)
	alterSchema(dg, "answer: [uid] @count .")

	// Expected Edge count of 0x100: 1
	txn0 := dg.NewTxn()
	mu := api.Mutation{SetNquads: []byte("<0x100> <answer> <0x200> .")}
	_, err := txn0.Mutate(ctxb, &mu)
	check(err)
	err = txn0.Commit(ctxb)
	check(err)

	// Expected edge count of 0x1: 2
	// This should NOT appear in the query result
	// The following two mutations are in serial txns.
	txn1 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x2> .")}
	_, err = txn1.Mutate(ctxb, &mu)
	check(err)
	err = txn1.Commit(ctxb)
	check(err)

	txn2 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x3> .")}
	_, err = txn2.Mutate(ctxb, &mu)
	check(err)
	err = txn2.Commit(ctxb)
	check(err)

	// Verify query
	txn := dg.NewReadOnlyTxn()
	vars := map[string]string{"$num": "1"}
	resp, err := txn.QueryWithVars(ctxb, countQuery, vars)
	check(err)
	js := string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 1, "uid": "0x100"}]}`,
		js)
	txn = dg.NewReadOnlyTxn()
	vars = map[string]string{"$num": "2"}
	resp, err = txn.QueryWithVars(ctxb, countQuery, vars)
	check(err)
	js = string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 2, "uid": "0x1"}]}`,
		js)
}

func TestCountIndexSameTxn(t *testing.T) {
	dg := z.DgraphClientWithGroot(z.SockAddr)
	z.DropAll(t, dg)
	alterSchema(dg, "answer: [uid] @count .")

	// Expected Edge count of 0x100: 1
	txn0 := dg.NewTxn()
	mu := api.Mutation{SetNquads: []byte("<0x100> <answer> <0x200> .")}
	_, err := txn0.Mutate(ctxb, &mu)
	check(err)
	err = txn0.Commit(ctxb)
	check(err)

	// Expected edge count of 0x1: 2
	// This should NOT appear in the query result
	// The following two mutations are in the same txn.
	txn1 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x2> .")}
	_, err = txn1.Mutate(ctxb, &mu)
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x3> .")}
	_, err = txn1.Mutate(ctxb, &mu)
	check(err)
	err = txn1.Commit(ctxb)
	check(err)

	// Verify query
	txn := dg.NewReadOnlyTxn()
	vars := map[string]string{"$num": "1"}
	resp, err := txn.QueryWithVars(ctxb, countQuery, vars)
	check(err)
	js := string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 1, "uid": "0x100"}]}`,
		js)
	txn = dg.NewReadOnlyTxn()
	vars = map[string]string{"$num": "2"}
	resp, err = txn.QueryWithVars(ctxb, countQuery, vars)
	check(err)
	js = string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 2, "uid": "0x1"}]}`,
		js)
}

func alterSchema(dg *dgo.Dgraph, schema string) {
	op := api.Operation{}
	op.Schema = schema
	err := dg.Alter(ctxb, &op)
	check(err)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
