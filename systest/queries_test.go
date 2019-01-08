/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestQuery(t *testing.T) {
	wrap := func(fn func(*testing.T, *dgo.Dgraph)) func(*testing.T) {
		return func(t *testing.T) {
			conn, err := grpc.Dial("localhost:9180", grpc.WithInsecure())
			x.Check(err)
			dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
			require.NoError(t, dg.Alter(
				context.Background(), &api.Operation{DropAll: true}))
			fn(t, dg)
		}
	}

	t.Run("schema response", wrap(SchemaQueryTest))
	t.Run("schema response http", wrap(SchemaQueryTestHTTP))
}

func SchemaQueryTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `name: string @index(exact) .`,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`_:n1 <name> "srfrog" .`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, `schema {}`)
	require.NoError(t, err)
	js := `
	{
		"schema": [
			{
				"predicate": "_predicate_",
				"type": "string",
				"list": true
			},
			{
				"predicate": "dgraph.group.acl",
				"type": "string"
			},
			{
				"predicate": "dgraph.password",
				"type": "password"
			},
			{
				"predicate": "dgraph.user.group",
				"type": "uid",
				"reverse": true
			},
			{
				"predicate": "dgraph.xid",
				"type": "string",
				"index": true,
				"tokenizer": [
					"exact"
				]
			},
			{
				"predicate": "name",
				"type": "string",
				"index": true,
				"tokenizer": [
					"exact"
				]
			}
		]
	}`
	CompareJSON(t, js, string(resp.Json))
}

func SchemaQueryTestHTTP(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `name: string @index(exact) .`,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`_:n1 <name> "srfrog" .`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	var bb bytes.Buffer
	bb.WriteString(`schema{}`)
	res, err := http.Post("http://localhost:8180/query", "application/json", &bb)
	require.NoError(t, err)
	require.NotNil(t, res)
	defer res.Body.Close()

	bb.Reset()
	_, err = bb.ReadFrom(res.Body)
	require.NoError(t, err)

	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(bb.Bytes(), &m))
	require.NotNil(t, m["extensions"])

	js := `
	{
		"schema": [
			{
				"predicate": "_predicate_",
				"type": "string",
				"list": true
			},
			{
				"predicate": "dgraph.group.acl",
				"type": "string"
			},
			{
				"predicate": "dgraph.password",
				"type": "password"
			},
			{
				"predicate": "dgraph.user.group",
				"type": "uid",
				"reverse": true
			},
			{
				"predicate": "dgraph.xid",
				"type": "string",
				"index": true,
				"tokenizer": [
					"exact"
				]
			},
			{
				"predicate": "name",
				"type": "string",
				"index": true,
				"tokenizer": [
					"exact"
				]
			}
		]
	}`
	CompareJSON(t, js, string(m["data"]))
}
