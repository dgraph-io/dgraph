/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

func InitData(t *testing.T) {
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	testutil.DropAll(t, dg)
	schema := `
		name: string @index(exact) .
		count: [int]  .
	`

	err = dg.Alter(context.Background(), &api.Operation{Schema: schema})
	require.NoError(t, err)

	mu := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "Alice" .
			_:a <count> "1" .
		`),
		CommitNow: true,
	}
	txn := dg.NewTxn()
	ctx := context.Background()
	defer txn.Discard(ctx)

	_, err = txn.Mutate(ctx, mu)
	require.NoError(t, err)
}

func TestConcurrentUpdate(t *testing.T) {
	// wait for server to be ready
	time.Sleep(2 * time.Second)
	InitData(t)
	ctx := context.Background()
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	count := 10
	var wg sync.WaitGroup
	wg.Add(count)
	mutation := func(i int) {
		defer wg.Done()
		query := `query {
			user as var(func: eq(name, "Alice"))
		}`
		mu := &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`uid(user) <count> "%d" .`, i)),
		}
		req := &api.Request{
			Query:     query,
			Mutations: []*api.Mutation{mu},
			CommitNow: true,
		}
		_, err := dg.NewTxn().Do(ctx, req)
		require.NoError(t, err)
	}
	for i := 0; i < count; i++ {
		go mutation(i)
	}
	wg.Wait()
	// eventual consistency
	time.Sleep(3 * time.Second)

	q := `query all($a: string) {
			all(func: eq(name, $a)) {
			  count
			}
		  }`

	txn := dg.NewTxn()
	res, err := txn.QueryWithVars(ctx, q, map[string]string{"$a": "Alice"})
	require.NoError(t, err)
	require.JSONEq(t, `{"all":[{"count":[0,4,5,2,8,1,3,9,6,7]}]}`, string(res.GetJson()))
}

func TestSequentialUpdate(t *testing.T) {
	// wait for server to be ready
	time.Sleep(2 * time.Second)
	InitData(t)
	ctx := context.Background()
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	count := 10
	mutation := func(i int) {
		query := `query {
			user as var(func: eq(name, "Alice"))
			}`
		mu := &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`uid(user) <count> "%d" .`, i)),
		}
		req := &api.Request{
			Query:     query,
			Mutations: []*api.Mutation{mu},
			CommitNow: true,
		}
		_, err := dg.NewTxn().Do(ctx, req)
		require.NoError(t, err)

	}
	for i := 0; i < count; i++ {
		mutation(i)
	}

	// eventual consistency
	time.Sleep(3 * time.Second)

	q := `query all($a: string) {
			all(func: eq(name, $a)) {
			  count
			}
		  }`

	txn := dg.NewTxn()
	res, err := txn.QueryWithVars(ctx, q, map[string]string{"$a": "Alice"})
	require.NoError(t, err)
	require.JSONEq(t, `{"all":[{"count":[0,4,5,2,8,1,3,9,6,7]}]}`, string(res.GetJson()))
}

func TestDelete(t *testing.T) {
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	testutil.DropAll(t, dg)
	schema := `
		name: string .
		xid: string .
		type MyType {
		  name
		  xid
		}
	`

	err = dg.Alter(context.Background(), &api.Operation{Schema: schema})
	require.NoError(t, err)

	mu := &api.Mutation{
		SetNquads: []byte(`
			_:n <dgraph.type> "MyType" .
			_:n <name> "Alice" .
			_:n <xid> "10" .
		`),

		CommitNow: true,
	}
	txn := dg.NewTxn()
	ctx := context.Background()
	defer txn.Discard(ctx)

	res, err := txn.Mutate(ctx, mu)
	require.NoError(t, err)
	uid := res.Uids["n"]

	ctx = context.Background()
	query := func() string {
		q := `{ q(func: uid(` + uid + `)){ dgraph.type name xid } }`
		res, err := dg.NewTxn().Query(ctx, q)
		require.NoError(t, err)
		return string(res.GetJson())
	}
	time.Sleep(time.Second)
	expected := `{"q":[{"dgraph.type":["MyType"],"name":"Alice","xid":"10"}]}`
	require.JSONEq(t, expected, query())

	mu = &api.Mutation{
		DelNquads: []byte(`<` + uid + `> * * .`),
		CommitNow: true,
	}
	txn = dg.NewTxn()
	ctx = context.Background()
	defer txn.Discard(ctx)

	_, err = txn.Mutate(ctx, mu)
	require.NoError(t, err)
	time.Sleep(time.Second)
	require.JSONEq(t, `{"q":[]}`, query())
}
