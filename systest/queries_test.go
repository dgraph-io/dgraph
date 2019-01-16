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
	"fmt"
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
			require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropAll: true}))
			fn(t, dg)
		}
	}

	t.Run("schema response", wrap(SchemaQueryTest))
	t.Run("schema response http", wrap(SchemaQueryTestHTTP))
	t.Run("schema predicate names", wrap(SchemaQueryTestPredicate1))
	t.Run("schema specific predicate fields", wrap(SchemaQueryTestPredicate2))
	t.Run("schema specific predicate field", wrap(SchemaQueryTestPredicate3))
	t.Run("multiple block eval", wrap(MultipleBlockEval))
	t.Run("cleanup", wrap(SchemaQueryCleanup))
}

func SchemaQueryCleanup(t *testing.T, c *dgo.Dgraph) {
	require.NoError(t, c.Alter(context.Background(), &api.Operation{DropAll: true}))
}

func MultipleBlockEval(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
      entity: string @index(exact) .
      stock: uid @reverse .
    `,
	}))

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
      _:e1 <entity> "672E1D63-4921-420C-90A8-5B39DD77B89F" .
      _:e1 <entity.type> "chair" .
      _:e1 <entity.price> "2999.99"^^<xs:float> .

      _:e2 <entity> "03B9CB73-7424-4BE5-AE39-97CC4D2F3A21" .
      _:e2 <entity.type> "sofa" .
      _:e2 <entity.price> "899.0"^^<xs:float> .

      _:e1 <combo> _:e2 .
      _:e2 <combo> _:e1 .

      _:e3 <entity> "BDFD64A3-5CA8-41F0-98D6-E65A9DAE092D" .
      _:e3 <entity.type> "desk" .
      _:e3 <entity.price> "578.99"^^<xs:float> .

      _:e4 <entity> "AE1D1A85-9C26-4A1D-9B0B-00A8BBCFDDA0" .
      _:e4 <entity.type> "lamp" .
      _:e4 <entity.price> "199.99"^^<xs:float> .

      _:e3 <combo> _:e4 .
      _:e4 <combo> _:e3 .

      _:e5 <entity> "9021E504-65B7-4939-8C02-F73CFD5635F6" .
      _:e5 <entity.type> "table" .
      _:e5 <entity.price> "1899.98"^^<xs:float> .

      # table has no combo

      _:s1 <stock> _:e1 .
      _:s1 <stock.in> "100"^^<xs:int> .
      _:s1 <stock.note> "Over-stocked" .
      _:e1 <stock> _:s1 .

      _:s2 <stock> _:e2 .
      _:s2 <stock.in> "20"^^<xs:int> .
      _:s2 <stock.note> "Running low, order more" .
      _:e2 <stock> _:s2 .

      _:s3 <stock> _:e3 .
      _:s3 <stock.in> "25"^^<xs:int> .
      _:s3 <stock.note> "Delicate needs insurance" .
      _:e3 <stock> _:s3 .

      _:s4 <stock> _:e4 .
      _:s4 <stock.out> "true"^^<xs:boolean> .
      _:s4 <stock.note> "Out of stock" .
      _:e4 <stock> _:s4 .

      _:s5 <stock> _:e5 .
      _:s5 <stock.out> "true"^^<xs:boolean> .
      _:s5 <stock.note> "Out of stock" .
      _:e5 <stock> _:s5 .
    `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	txn = c.NewTxn()

	tests := []struct {
		in  string
		out string
	}{
		{in: "672E1D63-4921-420C-90A8-5B39DD77B89F",
			out: `{"q": [{
        "notes": [{
          "stock.note": "Over-stocked"
        }],
        "sku": "672E1D63-4921-420C-90A8-5B39DD77B89F",
        "type": "chair",
        "combos": 1
      }]}`},
		{in: "03B9CB73-7424-4BE5-AE39-97CC4D2F3A21",
			out: `{"q": [{
        "notes": [{
          "stock.note": "Running low, order more"
        }],
        "sku": "03B9CB73-7424-4BE5-AE39-97CC4D2F3A21",
        "type": "sofa",
        "combos": 1
      }]}`},
		{in: "BDFD64A3-5CA8-41F0-98D6-E65A9DAE092D",
			out: `{"q": [{
        "notes": [{
          "stock.note": "Delicate needs insurance"
        }],
        "sku": "BDFD64A3-5CA8-41F0-98D6-E65A9DAE092D",
        "type": "desk",
        "combos": 1
      }]}`},
		{in: "AE1D1A85-9C26-4A1D-9B0B-00A8BBCFDDA0",
			out: `{"q": [{
      "combos": 1,
      "notes": [{
          "stock.note": "Out of stock"
      }],
      "sku": "AE1D1A85-9C26-4A1D-9B0B-00A8BBCFDDA0",
      "type": "lamp"
    }]}`},
		{in: "9021E504-65B7-4939-8C02-F73CFD5635F6",
			out: `{"q":[]}`},
	}
	if assigned == nil {
	}

	queryFmt := `
  {
    filter_uids as var(func: eq(entity, "%s"))
      @filter(has(entity.type) and not(has(stock.out)) and (has(combo)))

    entity_uids as var (func: uid(filter_uids)) @filter()

    var(func: uid(entity_uids)) {
      stock_uid as stock
    }

    var(func: uid(entity_uids)) {
      stock {
        available as stock @filter(has(entity.price) and not(has(stock.out)))
      }
    }

    var(func: uid(available)) @cascade {
      combo {
        cnt_combos as math(1)
        combo {
          ~stock {
            ~stock @filter(has(combo))
          }
        }
      }
      available_combos as sum(val(cnt_combos))
    }

    q(func: uid(available)) {
      sku: entity
      type: entity.type
      notes: ~stock @filter(uid(stock_uid)) {
        stock.note
      }
      combos: val(available_combos)
    }
  }`

	for _, tc := range tests {
		resp, err := txn.Query(ctx, fmt.Sprintf(queryFmt, tc.in))
		require.NoError(t, err)
		CompareJSON(t, tc.out, string(resp.Json))
	}
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
        "reverse": true,
        "list": true
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

func SchemaQueryTestPredicate1(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
      name: string @index(exact) .
      age: int .
    `,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
      _:p1 <name> "srfrog" .
      _:p1 <age> "25"^^<xs:int> .
      _:p2 <name> "mary" .
      _:p2 <age> "22"^^<xs:int> .
      _:p1 <friends> _:p2 .
    `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, `schema {name}`)
	require.NoError(t, err)
	js := `
  {
    "schema": [
      {
        "predicate": "_predicate_"
      },
      {
        "predicate": "dgraph.group.acl"
      },
      {
        "predicate": "dgraph.password"
      },
      {
        "predicate": "dgraph.user.group"
      },
      {
        "predicate": "dgraph.xid"
      },
      {
        "predicate": "friends"
      },
      {
        "predicate": "name"
      },
      {
        "predicate": "age"
      }
    ]
  }`
	CompareJSON(t, js, string(resp.Json))
}

func SchemaQueryTestPredicate2(t *testing.T, c *dgo.Dgraph) {
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
	resp, err := txn.Query(ctx, `schema(pred: [name]) {}`)
	require.NoError(t, err)
	js := `
  {
    "schema": [
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

func SchemaQueryTestPredicate3(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
      name: string @index(exact) .
      age: int .
    `,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
      _:p1 <name> "srfrog" .
      _:p1 <age> "25"^^<xs:int> .
      _:p2 <name> "mary" .
      _:p2 <age> "22"^^<xs:int> .
      _:p1 <friends> _:p2 .
    `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, `
    schema(pred: [age]) {
      name
      type
    }
  `)
	require.NoError(t, err)
	js := `
  {
    "schema": [
      {
        "predicate": "age",
        "type": "int"
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
        "reverse": true,
        "list": true
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
