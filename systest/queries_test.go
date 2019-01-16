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
			require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropAll: true}))
			fn(t, dg)
		}
	}

	t.Run("schema response", wrap(SchemaQueryTest))
	t.Run("schema response http", wrap(SchemaQueryTestHTTP))
	t.Run("schema predicate names", wrap(SchemaQueryTestPredicate1))
	t.Run("schema specific predicate fields", wrap(SchemaQueryTestPredicate2))
	t.Run("schema specific predicate field", wrap(SchemaQueryTestPredicate3))
	t.Run("hash index queries", wrap(QueryHashIndex))
	t.Run("cleanup", wrap(SchemaQueryCleanup))
}

func SchemaQueryCleanup(t *testing.T, c *dgo.Dgraph) {
	require.NoError(t, c.Alter(context.Background(), &api.Operation{DropAll: true}))
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

func QueryHashIndex(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
      name: string @index(hash) @lang .
    `,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
      _:p0 <name> "" .
      _:p1 <name> "0" .
      _:p2 <name> "srfrog" .
      _:p3 <name> "Lorem ipsum" .
      _:p4 <name> "Lorem ipsum dolor sit amet" .
      _:p5 <name> "Lorem ipsum dolor sit amet, consectetur adipiscing elit" .
      _:p6 <name> "Lorem ipsum"@en .
      _:p7 <name> "Lorem ipsum dolor sit amet"@en .
      _:p8 <name> "Lorem ipsum dolor sit amet, consectetur adipiscing elit"@en .
      _:p9 <name> "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed varius tellus ut sem bibendum, eu tristique augue congue. Praesent eget odio tincidunt, pellentesque ante sit amet, tempus sem. Donec et tellus et diam facilisis egestas ut ac risus. Proin feugiat risus tristique erat condimentum placerat. Nulla eget ligula tempus, blandit leo vel, accumsan tortor. Phasellus et felis in diam ultricies porta nec in ipsum. Phasellus id leo sagittis, bibendum enim ut, pretium lectus. Quisque ac ex viverra, suscipit turpis sed, scelerisque metus. Sed non dui facilisis, viverra leo eget, vulputate erat. Etiam nec enim sed nisi imperdiet cursus. Suspendisse sed ligula non nisi pharetra varius." .
      _:pa <name> ""@fr .
    `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	tests := []struct {
		in, out string
	}{
		{
			in: `schema(pred: [name]) {}`,
			out: `
      {
        "schema": [
          {
            "index": true,
            "lang": true,
            "predicate": "name",
            "tokenizer": [
              "hash"
            ],
            "type": "string"
          }
        ]
      }`,
		},
		{
			in:  `{q(func:eq(name,"")){name}}`,
			out: `{"q": [{"name":""}]}`,
		},
		{
			in:  `{q(func:eq(name,"0")){name}}`,
			out: `{"q": [{"name":"0"}]}`,
		},
		{
			in:  `{q(func:eq(name,"srfrog")){name}}`,
			out: `{"q": [{"name":"srfrog"}]}`,
		},
		{
			in:  `{q(func:eq(name,"Lorem ipsum")){name}}`,
			out: `{"q": [{"name":"Lorem ipsum"}]}`,
		},
		{
			in:  `{q(func:eq(name,"Lorem ipsum dolor sit amet")){name}}`,
			out: `{"q": [{"name":"Lorem ipsum dolor sit amet"}]}`,
		},
		{
			in:  `{q(func:eq(name@en,"Lorem ipsum")){name@en}}`,
			out: `{"q": [{"name@en":"Lorem ipsum"}]}`,
		},
		{
			in:  `{q(func:eq(name@.,"Lorem ipsum dolor sit amet")){name@en}}`,
			out: `{"q": [{"name@en":"Lorem ipsum dolor sit amet"}]}`,
		},
		{
			in:  `{q(func:eq(name,["srfrog"])){name}}`,
			out: `{"q": [{"name":"srfrog"}]}`,
		},
		{
			in:  `{q(func:eq(name,["srfrog","srf","srfrogg","sr","s"])){name}}`,
			out: `{"q": [{"name":"srfrog"}]}`,
		},
		{
			in:  `{q(func:eq(name,["Lorem ipsum","Lorem ipsum dolor sit amet, consectetur adipiscing elit",""])){name}}`,
			out: `{"q": [{"name":""},{"name":"Lorem ipsum"},{"name":"Lorem ipsum dolor sit amet, consectetur adipiscing elit"}]}`,
		},
		{
			in:  `{q(func:eq(name,["Lorem ipsum","Lorem ipsum","Lorem ipsum","Lorem ipsum","Lorem ipsum"])){name}}`,
			out: `{"q": [{"name":"Lorem ipsum"}]}`,
		},
		{
			in:  `{q(func:eq(name@en,["Lorem ipsum","Lorem ipsum dolor sit amet, consectetur adipiscing elit",""])){name@en}}`,
			out: `{"q": [{"name@en":"Lorem ipsum"},{"name@en":"Lorem ipsum dolor sit amet, consectetur adipiscing elit"}]}`,
		},
		{
			in:  `{q(func:eq(name@en,["Lorem ipsum","Lorem ipsum","Lorem ipsum","Lorem ipsum","Lorem ipsum"])){name@en}}`,
			out: `{"q": [{"name@en":"Lorem ipsum"}]}`,
		},
		{
			in:  `{q(func:eq(name@.,"")){name@fr}}`,
			out: `{"q": [{"name@fr":""}]}`,
		},
	}

	for _, tc := range tests {
		resp, err := c.NewTxn().Query(ctx, tc.in)
		require.NoError(t, err)
		CompareJSON(t, tc.out, string(resp.Json))
	}
}
