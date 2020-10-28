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
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

type Person struct {
	Name  string `json:"name,omitempty"`
	count int
}

type Data struct {
	Name   string `json:"name,omitempty"`
	Counts []int  `json:"count,omitempty"`
}
type ResponseData struct {
	All []Data `json:"all,omitempty"`
}

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

	p := Person{
		Name:  "Alice",
		count: 1,
	}
	pb, err := json.Marshal(p)
	require.NoError(t, err)

	mu := &api.Mutation{
		SetJson:   pb,
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
			  name
			  count
			}
		  }`

	txn := dg.NewTxn()
	res, err := txn.QueryWithVars(ctx, q, map[string]string{"$a": "Alice"})
	require.NoError(t, err)
	var dat ResponseData
	err = json.Unmarshal(res.Json, &dat)
	require.NoError(t, err)

	require.Equal(t, 10, len(dat.All[0].Counts))
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
			  name
			  count
			}
		  }`

	txn := dg.NewTxn()
	res, err := txn.QueryWithVars(ctx, q, map[string]string{"$a": "Alice"})
	require.NoError(t, err)
	var dat ResponseData
	err = json.Unmarshal(res.Json, &dat)
	require.NoError(t, err)

	require.Equal(t, 10, len(dat.All[0].Counts))
}
