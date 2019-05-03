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
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
)

func TestQuery(t *testing.T) {
	wrap := func(fn func(*testing.T, *dgo.Dgraph)) func(*testing.T) {
		return func(t *testing.T) {
			dg := z.DgraphClientWithGroot(z.SockAddr)
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
	t.Run("unmatched var assignment eval", wrap(UnmatchedVarEval))
	t.Run("hash index queries", wrap(QueryHashIndex))
	t.Run("fuzzy matching", wrap(FuzzyMatch))
	t.Run("regexp with toggled trigram index", wrap(RegexpToggleTrigramIndex))
	t.Run("groupby uid that works", wrap(GroupByUidWorks))
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
      stock: [uid] @reverse .
    `,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
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

	txn = c.NewTxn()
	for _, tc := range tests {
		resp, err := txn.Query(ctx, fmt.Sprintf(queryFmt, tc.in))
		require.NoError(t, err)
		CompareJSON(t, tc.out, string(resp.Json))
	}
}

func UnmatchedVarEval(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
      item: string @index(hash) .
      style.type: string .
      style.name: string .
      style.cool: bool .
    `,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
      _:a <item> "chair" .
      _:a <style.name> "Modern leather chair" .
      _:a <style.cool> "true" .

      _:b <item> "chair" .
      _:b <style.name> "Broken beanbag" .
      _:b <style.cool> "false"^^<xs:boolean> .

      _:c <item> "sofa" .
      _:c <style.name> "U-shape sectional" .
      _:c <style.cool> "true"^^<xs:boolean> .

      _:d <item> "table" .
      _:d <style.name> "Glass top marble table" .
      _:d <style.cool> "true"^^<xs:boolean> .
    `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	tests := []struct {
		in  string
		out string
	}{
		{
			in: `
      {
        items as var(func: eq(item, "chair")) @filter(has(style.name)) @cascade {
          is_cool as style.cool
        }
        q(func: eq(item, "chair")) @filter(eq(val(is_cool), false) AND uid(items)) {
          item
          style.name
          style.cool
          is_cool
        }
      }`,
			out: `
      {
        "q": [
          {
            "item": "chair",
            "style.cool": false,
            "style.name": "Broken beanbag"
          }
        ]
      }`,
		},
		{
			// filtering by dummy (no such pred) reduces to empty set.
			// is_cool would be assigned as default type to aid in filter eval.
			// @filter(eq(val(is_cool), false) would effectively evaluate: "" eq "false"
			in: `
      {
        items as var(func: eq(item, "chair")) @filter(has(dummy)) @cascade {
          is_cool as style.cool
        }
        q(func: eq(item, "chair")) @filter(eq(val(is_cool), false) AND uid(items)) {
          item
          style.name
          style.cool
          is_cool
        }
      }`,
			out: `{"q": []}`,
		},
	}

	txn = c.NewTxn()
	for _, tc := range tests {
		resp, err := txn.Query(ctx, tc.in)
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
    "schema": [` + x.AclPredicates + `,
      {
        "predicate": "_predicate_",
        "type": "string",
        "list": true
      },
      {
        "predicate": "dgraph.type",
        "type": "string",
        "index": true,
        "tokenizer": [
          "exact"
        ],
		"list": true
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
        "predicate": "dgraph.xid"
      },
      {
        "predicate": "dgraph.password"
      },
      {
        "predicate": "dgraph.group.acl"
      },
      {
        "predicate": "dgraph.user.group"
      },
      {
        "predicate": "_predicate_"
      },
      {
        "predicate": "friends"
      },
      {
        "predicate": "dgraph.type"
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
	res, err := http.Post("http://localhost:8180/query", "application/graphql", &bb)
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
    "schema": [` + x.AclPredicates + `,
      {
        "predicate": "_predicate_",
        "type": "string",
        "list": true
      },
      {
        "index": true,
        "predicate": "dgraph.type",
        "type": "string",
        "tokenizer": ["exact"],
		"list": true
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

func FuzzyMatch(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
      term: string @index(trigram) .
      name: string .
    `,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
      _:t0 <term> "" .
      _:t1 <term> "road" .
      _:t2 <term> "avenue" .
      _:t3 <term> "street" .
      _:t4 <term> "boulevard" .
      _:t5 <term> "drive" .
      _:t6 <term> "route" .
      _:t7 <term> "pass" .
      _:t8 <term> "pathway" .
      _:t9 <term> "lane" .
      _:ta <term> "highway" .
      _:tb <term> "parkway" .
      _:tc <term> "motorway" .
      _:td <term> "high road" .
      _:te <term> "side street" .
      _:tf <term> "dual carriageway" .
      _:n0 <name> "srfrog" .
    `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	tests := []struct {
		in, out, failure string
	}{
		{
			in:  `{q(func:match(term, drive, 8)) {term}}`,
			out: `{"q":[{"term":"drive"}]}`,
		},
		{
			in:  `{q(func:match(term, "plano", 1)) {term}}`,
			out: `{"q":[]}`,
		},
		{
			in:  `{q(func:match(term, "plano", 2)) {term}}`,
			out: `{"q":[{"term":"lane"}]}`,
		},
		{
			in:  `{q(func:match(term, "plano", 8)) {term}}`,
			out: `{"q":[{"term":"lane"}]}`,
		},
		{
			in: `{q(func:match(term, way, 8)) {term}}`,
			out: `{"q":[
        {"term": "highway"},
        {"term": "pathway"},
        {"term": "parkway"},
        {"term": "motorway"}
      ]}`,
		},
		{
			in: `{q(func:match(term, pway, 8)) {term}}`,
			out: `{"q":[
        {"term": "highway"},
        {"term": "pathway"},
        {"term": "parkway"},
        {"term": "motorway"}
      ]}`,
		},
		{
			in: `{q(func:match(term, high, 8)) {term}}`,
			out: `{"q":[
        {"term": "highway"},
        {"term": "high road"}
      ]}`,
		},
		{
			in: `{q(func:match(term, str, 8)) {term}}`,
			out: `{"q":[
        {"term": "street"},
        {"term": "side street"}
      ]}`,
		},
		{
			in: `{q(func:match(term, strip, 8)) {term}}`,
			out: `{"q":[
        {"term": "street"},
        {"term": "side street"}
      ]}`,
		},
		{
			in:  `{q(func:match(term, strip, 3)) {term}}`,
			out: `{"q":[{"term": "street"}]}`,
		},
		{
			in: `{q(func:match(term, "carigeway", 8)) {term}}`,
			out: `{"q":[
        {"term": "dual carriageway"}
      ]}`,
		},
		{
			in:  `{q(func:match(term, "carigeway", 4)) {term}}`,
			out: `{"q":[]}`,
		},
		{
			in: `{q(func:match(term, "dualway", 8)) {term}}`,
			out: `{"q":[
        {"term": "highway"},
        {"term": "pathway"},
        {"term": "parkway"},
        {"term": "motorway"}
      ]}`,
		},
		{
			in:  `{q(func:match(term, "dualway", 2)) {term}}`,
			out: `{"q":[]}`,
		},
		{
			in:      `{q(func:match(name, "someone", 8)) {name}}`,
			failure: `Attribute name is not indexed with type trigram`,
		},
	}
	for _, tc := range tests {
		resp, err := c.NewTxn().Query(ctx, tc.in)
		if tc.failure != "" {
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.failure)
			continue
		}
		require.NoError(t, err)
		CompareJSON(t, tc.out, string(resp.Json))
	}
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

func RegexpToggleTrigramIndex(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
      name: string @index(term) @lang .
    `,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
      _:x1 <name> "The luck is in the details" .
      _:x1 <name> "The art is in the details"@en .
      _:x1 <name> "L'art est dans les détails"@fr .

      _:x2 <name> "Detach yourself from ostentation" .
      _:x2 <name> "Detach yourself from artificiality"@en .
      _:x2 <name> "Desprenderse de la artificialidad"@es .
    `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	tests := []struct {
		in, out string
	}{
		{
			in:  `{q(func:has(name)) @filter(regexp(name, /art/)) {name}}`,
			out: `{"q":[]}`,
		},
		{
			in:  `{q(func:has(name)) @filter(regexp(name@es, /art/)) {name@es}}`,
			out: `{"q":[{"name@es": "Desprenderse de la artificialidad"}]}`,
		},
		{
			in:  `{q(func:has(name)) @filter(regexp(name@fr, /art/)) {name@fr}}`,
			out: `{"q":[{"name@fr": "L'art est dans les détails"}]}`,
		},
	}

	t.Log("testing without trigram index")
	for _, tc := range tests {
		resp, err := c.NewTxn().Query(ctx, tc.in)
		require.NoError(t, err)
		CompareJSON(t, tc.out, string(resp.Json))
	}

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
      name: string @index(trigram) @lang .
    `,
	}))

	t.Log("testing with trigram index")
	for _, tc := range tests {
		resp, err := c.NewTxn().Query(ctx, tc.in)
		require.NoError(t, err)
		CompareJSON(t, tc.out, string(resp.Json))
	}

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
      name: string @index(term) @lang .
    `,
	}))

	t.Log("testing without trigram index at root")
	_, err = c.NewTxn().Query(ctx, `{q(func:regexp(name, /art/)) {name}}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Attribute name does not have trigram index for regex matching.")
}

func GroupByUidWorks(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
      _:x1 <name> "horsejr" .
      _:x2 <name> "srfrog" .
      _:x3 <name> "missbug" .
    `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	tests := []struct {
		in, out string
	}{
		{
			in:  fmt.Sprintf(`{q(func:uid(%s)) @groupby(uid) {count(uid)}}`, assigned.Uids["x1"]),
			out: `{}`,
		},
		{
			in: fmt.Sprintf(`{q(func:uid(%s)) @groupby(name) {count(uid)}}`, assigned.Uids["x1"]),
			out: `{"q":[{
				"@groupby":[{
					"count": 1,
					"name": "horsejr"
				}]}]}`,
		},
		{
			in:  `{q(func:has(dummy)) @groupby(uid) {}}`,
			out: `{}`,
		},
		{
			in:  `{q(func:has(name)) @groupby(dummy) {}}`,
			out: `{}`,
		},
	}
	for _, tc := range tests {
		resp, err := c.NewTxn().Query(ctx, tc.in)
		require.NoError(t, err)
		CompareJSON(t, tc.out, string(resp.Json))
	}
}
