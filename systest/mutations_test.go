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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
)

// TestSystem uses the externally run Dgraph cluster for testing. Most other
// tests in this package are using a suite struct system, which runs Dgraph and
// loads data with bulk and live loader.
func TestSystem(t *testing.T) {
	wrap := func(fn func(*testing.T, *dgo.Dgraph)) func(*testing.T) {
		return func(t *testing.T) {
			dg := z.DgraphClientWithGroot(z.SockAddr)
			require.NoError(t, dg.Alter(
				context.Background(), &api.Operation{DropAll: true}))
			fn(t, dg)
		}
	}

	t.Run("n-quad mutation", wrap(NQuadMutationTest))
	t.Run("list with languages", wrap(ListWithLanguagesTest))
	t.Run("delete all reverse index", wrap(DeleteAllReverseIndex))
	t.Run("normalise edge cases", wrap(NormalizeEdgeCasesTest))
	t.Run("facets with order", wrap(FacetOrderTest))
	t.Run("lang and sort bug", wrap(LangAndSortBugTest))
	t.Run("sort facets return nil", wrap(SortFacetsReturnNil))
	t.Run("check schema after deleting node", wrap(SchemaAfterDeleteNode))
	t.Run("fulltext equal", wrap(FullTextEqual))
	t.Run("json blank node", wrap(JSONBlankNode))
	t.Run("scalar to list", wrap(ScalarToList))
	t.Run("list to scalar", wrap(ListToScalar))
	t.Run("set after delete for list", wrap(SetAfterDeletionListType))
	t.Run("empty strings with exact", wrap(EmptyNamesWithExact))
	t.Run("empty strings with term", wrap(EmptyRoomsWithTermIndex))
	t.Run("delete with expand all", wrap(DeleteWithExpandAll))
	t.Run("facets using nquads", wrap(FacetsUsingNQuadsError))
	t.Run("skip empty pl for has", wrap(SkipEmptyPLForHas))
	t.Run("has with dash", wrap(HasWithDash))
	t.Run("list geo filter", wrap(ListGeoFilterTest))
	t.Run("list regex filter", wrap(ListRegexFilterTest))
	t.Run("regex query vars", wrap(RegexQueryWithVars))
	t.Run("graphql var child", wrap(GraphQLVarChild))
	t.Run("math ge", wrap(MathGe))
	t.Run("has should not have deleted edge", wrap(HasDeletedEdge))
	t.Run("has should have reverse edges", wrap(HasReverseEdge))
	t.Run("facet json input supports anyofterms query", wrap(FacetJsonInputSupportsAnyOfTerms))
	t.Run("max predicate size", wrap(MaxPredicateSize))
	t.Run("restore reserved preds", wrap(RestoreReservedPreds))
	t.Run("drop data", wrap(DropData))
	t.Run("drop data and drop all", wrap(DropDataAndDropAll))
	t.Run("drop type", wrap(DropType))
	t.Run("drop type without specified type", wrap(DropTypeNoValue))
}

func FacetJsonInputSupportsAnyOfTerms(t *testing.T, c *dgo.Dgraph) {
	type Node struct {
		UID string `json:"uid,omitempty"`
	}

	type Resource struct {
		Node
		AccessToPermission string `json:"access.to|permission,omitempty"`
		AccessToInherit    bool   `json:"access.to|inherit"`
	}

	type User struct {
		Node
		Name     string    `json:"name,omitempty"`
		AccessTo *Resource `json:"access.to,omitempty"`
	}

	in := User{
		Node: Node{
			UID: "_:a",
		},
		AccessTo: &Resource{
			Node: Node{
				UID: "_:b",
			},
			AccessToPermission: "WRITE",
			AccessToInherit:    false,
		},
	}
	js, err := json.Marshal(&in)
	require.NoError(t, err, "the marshaling should have succeeded")

	ctx := context.Background()
	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetJson:   js,
	})
	require.NoError(t, err, "the mutation should have succeeded")

	q := `{
  direct(func: uid(%s)) {
    uid
    access.to @filter(uid(%s)) @facets(anyofterms(permission, "READ WRITE")) @facets(permission,inherit) {
      uid
    }
  }
}`
	query := fmt.Sprintf(q, assigned.Uids["a"], assigned.Uids["b"])
	resp, err := c.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "the query should have succeeded")

	//var respUser User
	z.CompareJSON(t, fmt.Sprintf(`
{"direct":[
  {
    "uid":"%s",
    "access.to":[
    {"uid":"%s","access.to|inherit":false,"access.to|permission":"WRITE"}]}]}
`, assigned.Uids["a"], assigned.Uids["b"]), string(resp.GetJson()))
}

func ListWithLanguagesTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	err := c.Alter(ctx, &api.Operation{
		Schema: `pred: [string] @lang .`,
	})
	require.Error(t, err)
}

func NQuadMutationTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `xid: string @index(exact) .`,
	}))

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:breakfast <name> "" .
			_:breakfast <nil_name> "_nil_" .
			_:breakfast <xid> "breakfast" .
			_:breakfast <fruit> _:banana .
			_:breakfast <fruit> _:apple .
			_:breakfast <cereal> _:weetbix .
			_:banana <xid> "banana" .
			_:apple <xid> "apple" .
			_:weetbix <xid> "weetbix" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	const breakfastQuery = `
	{
		q(func: eq(xid, "breakfast")) {
			name
			nil_name
			extra
			fruit {
				xid
			}
			cereal {
				xid
			}
		}
	}`

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, breakfastQuery)
	require.NoError(t, err)
	z.CompareJSON(t, `{ "q": [ {
		"fruit": [
			{ "xid": "apple" },
			{ "xid": "banana" }
		],
		"cereal": [
			{ "xid": "weetbix" }
		],
		"name": "",
		"nil_name": "_nil_"
	}]}`, string(resp.Json))

	txn = c.NewTxn()
	_, err = txn.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(fmt.Sprintf(`
			<%s> <fruit>  <%s> .
			<%s> <cereal> <%s> .
			<%s> <name> * .
			<%s> <nil_name> * .`,
			assigned.Uids["breakfast"], assigned.Uids["banana"],
			assigned.Uids["breakfast"], assigned.Uids["weetbix"],
			assigned.Uids["breakfast"], assigned.Uids["breakfast"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	txn = c.NewTxn()
	resp, err = txn.Query(ctx, breakfastQuery)
	require.NoError(t, err)
	z.CompareJSON(t, `{ "q": [ {
		"fruit": [
			{ "xid": "apple" }
		]
	}]}`, string(resp.Json))
}

func DeleteAllReverseIndex(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "link: [uid] @reverse ."}))
	assignedIds, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte("_:a <link> _:b ."),
	})
	require.NoError(t, err)
	aId := assignedIds.Uids["a"]
	bId := assignedIds.Uids["b"]

	/**
	we must run a query first before the next delete transaction, the
	reason is that a mutation does not wait for the previous mutation
	to finish completely with a commitTs from zero. If we run the
	deletion directly, and the previous mutation has not received a
	commitTs from zero yet, then the deletion will skip the link, and
	essentially be treated as a no-op. As a result, when we query it
	again following the reverse link, the link would still exist, and
	the test would fail.

	Running a query would make sure that the previous mutation for
	creating the link has completed with a commitTs from zero, and the
	subsequent deletion is done *AFTER* the link creation.
	*/
	_, _ = c.NewReadOnlyTxn().Query(ctx, fmt.Sprintf("{ q(func: uid(%s)) { link { uid } }}", aId))

	_, _ = c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		DelNquads: []byte(fmt.Sprintf("<%s> <link> * .", aId)),
	})
	resp, err := c.NewTxn().Query(ctx, fmt.Sprintf("{ q(func: uid(%s)) { ~link { uid } }}", bId))
	require.NoError(t, err)
	z.CompareJSON(t, `{"q":[]}`, string(resp.Json))

	assignedIds, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf("<%s> <link> _:c .", aId)),
	})
	require.NoError(t, err)
	cId := assignedIds.Uids["c"]

	resp, err = c.NewTxn().Query(ctx, fmt.Sprintf("{ q(func: uid(%s)) { ~link { uid } }}", cId))
	require.NoError(t, err)
	z.CompareJSON(t, fmt.Sprintf(`{"q":[{"~link": [{"uid": "%s"}]}]}`, aId), string(resp.Json))
}

func NormalizeEdgeCasesTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "xid: string @index(exact) ."}))

	_, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:root <xid> "root" .
			_:root <link> _:a .
			_:root <link> _:b .
		`),
	})
	require.NoError(t, err)

	for _, test := range []struct{ query, want string }{
		{
			`{ q(func: uid(0x1)) @normalize { count(uid) } } `,
			`{ "q": [] }`,
		},
		{
			`{ q(func: uid(0x1)) @normalize { c: count(uid) } } `,
			`{ "q": [ {"c": 1} ] }`,
		},
		{
			`{ q(func: uid(0x1)) @normalize { count: count(uid) } } `,
			`{ "q": [ {"count": 1} ] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { link { count(uid) } } } `,
			`{ "q": [] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { link { c: count(uid) } } } `,
			`{ "q": [ {"c": 2} ] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { link { count: count(uid) } } } `,
			`{ "q": [ {"count": 2} ] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { count(link) } } `,
			`{ "q": [] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { c: count(link) } } `,
			`{ "q": [ {"c": 2} ] }`,
		},
		{
			`{ q(func: eq(xid, "root")) @normalize { count: count(link) } } `,
			`{ "q": [ {"count": 2} ] }`,
		},
		{
			`{ q(func: uid(0x01)) @normalize { one as math(1) uno: val(one) } } `,
			`{ "q": [ {"uno": 1} ] }`,
		},
		{
			`
			{ var(func: uid(0x1, 0x2)) { ones as math(1) }
			q() @normalize { sum(val(ones)) } }
			`,
			`{ "q": [] }`,
		},
		{
			`
			{ var(func: uid(0x1, 0x2)) { ones as math(1) }
			q() @normalize { s: sum(val(ones)) } }
			`,
			`{ "q": [ {"s": 2} ] }`,
		},
	} {
		resp, err := c.NewTxn().Query(ctx, test.query)
		require.NoError(t, err)
		require.JSONEq(t, test.want, string(resp.GetJson()))
	}
}

func FacetOrderTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `name: string @index(exact) .`,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "Alice" .
			_:a <friend> _:b (age=13,car="Honda") .
			_:b <name> "Bob" .
			_:a <friend> _:c (age=15,car="Tesla") .
			_:c <name> "Charlie" .
			_:a <friend> _:d .
			_:d <name> "Bubble" .
			_:a <friend> _:e .
			_:a <friend> _:f (age=20,car="Hyundai") .
			_:f <name> "Abc" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	const friendQuery = `
	{
		q(func: eq(name, "Alice")) {
			name
			friend(orderdesc: name) @facets {
				name
			}
		}
	}`

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, friendQuery)
	require.NoError(t, err)
	z.CompareJSON(t, `{
		  "q": [
		    {
		      "friend": [
		        {
		          "friend|age": 15,
		          "friend|car": "Tesla",
		          "name": "Charlie"
		        },
		        {
		          "name": "Bubble"
		        },
		        {
		          "friend|age": 13,
		          "friend|car": "Honda",
		          "name": "Bob"
		        },
		        {
		          "friend|age": 20,
		          "friend|car": "Hyundai",
		          "name": "Abc"
		        }
		      ],
		      "name": "Alice"
		    }
		  ]
		}`, string(resp.Json))

}

// Shows fix for issue #1918.
func LangAndSortBugTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "name: string @index(exact) @lang ."}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:michael <name> "Michael" .
			_:michael <friend> _:sang .

			_:sang <name> "상현"@ko .
			_:sang <name> "Sang Hyun"@en .
		`),
	})
	require.NoError(t, err)

	txn = c.NewTxn()
	defer txn.Discard(ctx)
	resp, err := txn.Query(ctx, `
	{
	  q(func: eq(name, "Michael")) {
	    name
	    friend (orderdesc: name@.) {
	      name@en
	    }
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `
	{"q":[{
		"name": "Michael",
		"friend": [{ "name@en": "Sang Hyun" }]
	}]}
	`, string(resp.Json))
}

func SortFacetsReturnNil(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:michael <name> "Michael" .
			_:michael <friend> _:sang (since=2012-01-02) .
			_:sang <name> "Sang Hyun" .

			_:michael <friend> _:alice (since=2014-01-02) .
			_:alice <name> "Alice" .


			_:michael <friend> _:charlie .
			_:charlie <name> "Charlie" .
			`),
	})
	require.NoError(t, err)

	michael := assigned.Uids["michael"]
	resp, err := txn.Query(ctx, `
	{
	  q(func: uid(`+michael+`)) {
	    name
	    friend @facets(orderdesc: since) {
	      name
	    }
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `
	{"q":[{"name":"Michael","friend":[{"name":"Charlie"},{"name":"Alice","friend|since":"2014-01-02T00:00:00Z"},{"name":"Sang Hyun","friend|since":"2012-01-02T00:00:00Z"}]}]}
		`, string(resp.Json))
}

func SchemaAfterDeleteNode(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "married: bool ."}))

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:michael <name> "Michael" .
			_:michael <friend> _:sang .
			`),
	})
	require.NoError(t, err)
	michael := assigned.Uids["michael"]

	resp, err := c.NewTxn().Query(ctx, `schema{}`)
	require.NoError(t, err)
	z.CompareJSON(t, asJson(`[`+
		x.AclPredicates+","+
		`{"predicate":"friend","type":"uid","list":true},`+
		`{"predicate":"married","type":"bool"},`+
		`{"predicate":"name","type":"default"},`+
		`{"predicate":"dgraph.type","type":"string","index":true, "tokenizer":["exact"],
			"list":true}]`),
		string(resp.Json))

	require.NoError(t, c.Alter(ctx, &api.Operation{DropAttr: "married"}))

	// Lets try to do a S P * deletion. Schema for married shouldn't be rederived.
	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		DelNquads: []byte(`
		    <` + michael + `> * * .
			`),
	})
	require.NoError(t, err)

	resp, err = c.NewTxn().Query(ctx, `schema{}`)
	require.NoError(t, err)
	z.CompareJSON(t, asJson(`[`+
		x.AclPredicates+","+
		`{"predicate":"friend","type":"uid","list":true},`+
		`{"predicate":"name","type":"default"},`+
		`{"predicate":"dgraph.type","type":"string","index":true, "tokenizer":["exact"],
			"list":true}]`),
		string(resp.Json))
}

func asJson(schema string) string {
	return fmt.Sprintf(`{"schema":%v}`, schema)
}

func FullTextEqual(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: "text: string @index(fulltext) ."}))

	texts := []string{"bat man", "aqua man", "bat cave", "bat", "man", "aqua", "cave"}
	var rdfs bytes.Buffer
	for i, text := range texts {
		fmt.Fprintf(&rdfs, "_:node%d <text> %q .\n", i, text)
	}
	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: rdfs.Bytes(),
	})
	require.NoError(t, err)

	for _, text := range texts {
		resp, err := c.NewTxn().Query(ctx, `
		{
			q(func: eq(text, "`+text+`")) {
				text
			}
		}`)
		require.NoError(t, err)
		require.Equal(t, `{"q":[{"text":"`+text+`"}]}`, string(resp.GetJson()))
	}

	for _, bad := range []string{"cave dweller", "bat ears", "elephant"} {
		resp, err := c.NewTxn().Query(ctx, `
		{
			q(func: eq(text, "`+bad+`")) {
				text
			}
		}`)
		require.NoError(t, err)
		require.Equal(t, `{"q":[]}`, string(resp.GetJson()))
	}
}

func JSONBlankNode(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetJson: []byte(`
			{"uid": "_:michael", "name": "Michael", "friend": [{ "uid": "_:sang", "name": "Sang Hyun"}, { "uid": "_:alice", "name": "Alice"}]}
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	require.Equal(t, 3, len(assigned.Uids))
	michael := assigned.Uids["michael"]
	alice := assigned.Uids["alice"]
	resp, err := c.NewTxn().Query(ctx, `
	{
	  q(func: uid(`+michael+`)) {
	    name
	    friend @filter(uid(`+alice+`)){
	      name
	    }
	  }
	}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"name":"Michael","friend":[{"name":"Alice"}]}]}`, string(resp.Json))
}

func ScalarToList(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: `pred: string @index(exact) .`}))

	uids, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`_:blank <pred> "first" .`),
		CommitNow: true,
	})
	require.NoError(t, err)
	uid := uids.Uids["blank"]

	q := `
	{
		me(func: eq(pred, "first")) {
			pred
		}
	}
	`
	resp, err := c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":"first"}]}`, string(resp.Json))

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: `pred: [string] @index(exact) .`}))
	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["first"]}]}`, string(resp.Json))

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`<` + uid + `> <pred> "second" .`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["second","first"]}]}`, string(resp.Json))

	q2 := `
	{
		me(func: eq(pred, "second")) {
			pred
		}
	}
	`
	resp, err = c.NewTxn().Query(ctx, q2)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["second","first"]}]}`, string(resp.Json))

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		DelNquads: []byte(`<` + uid + `> <pred> "second" .`),
		SetNquads: []byte(`<` + uid + `> <pred> "third" .`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["third","first"]}]}`, string(resp.Json))

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		DelNquads: []byte(`<` + uid + `> <pred> "first" .`),
		CommitNow: true,
	})
	require.NoError(t, err)

	q3 := `
	{
		me(func: eq(pred, "third")) {
			pred
		}
	}
	`
	resp, err = c.NewTxn().Query(ctx, q3)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"pred":["third"]}]}`, string(resp.Json))

	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[]}`, string(resp.Json))

	resp, err = c.NewTxn().Query(ctx, q2)
	require.NoError(t, err)
	require.Equal(t, `{"me":[]}`, string(resp.Json))
}

func ListToScalar(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: `pred: [string] @index(exact) .`}))

	err := c.Alter(ctx, &api.Operation{Schema: `pred: string @index(exact) .`})
	require.Error(t, err)
	require.Contains(t, err.Error(), `Type can't be changed from list to scalar for attr: [pred] without dropping it first.`)

	require.NoError(t, c.Alter(ctx, &api.Operation{DropAttr: `pred`}))
	err = c.Alter(ctx, &api.Operation{Schema: `pred: string @index(exact) .`})
	require.NoError(t, err)
}

func SetAfterDeletionListType(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: `
		property.test: [string] .
	`}))

	m1 := []byte(`
		_:alice <property.test> "initial value" .
	`)
	txn := c.NewTxn()
	assigned, err := txn.Mutate(context.Background(),
		&api.Mutation{SetNquads: m1},
	)
	require.NoError(t, err)
	alice := assigned.Uids["alice"]
	require.NoError(t, txn.Commit(ctx))

	q := `
	{
		me(func: uid(` + alice + `)) {
			property.test
		}
	}
	`
	resp, err := c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"property.test":["initial value"]}]}`, string(resp.Json))

	txn = c.NewTxn()
	_, err = txn.Mutate(ctx, &api.Mutation{
		DelNquads: []byte(`<` + alice + `> <property.test> * .`),
	})
	require.NoError(t, err)
	resp, err = txn.Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[]}`, string(resp.Json))

	_, err = txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`<` + alice + `> <property.test> "rewritten value". `),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.Equal(t, `{"me":[{"property.test":["rewritten value"]}]}`, string(resp.Json))
}

func EmptyNamesWithExact(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	err := c.Alter(ctx, &api.Operation{Schema: `name: string @index(exact) @lang .`})
	require.NoError(t, err)

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:alex <name> "Alex" .
			_:alex <name@en> "Alex" .
			_:y <name> "" .
			_:y <name@ko> "상현" .
			_:amit <name> "" .
			_:amit <name@en> "" .
			_:amit <name@hi> "अमित" .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err := c.NewTxn().Query(ctx, `{
	  names(func: has(name)) @filter(eq(name, "")) {
		count(uid)
	  }
	}`)
	require.NoError(t, err)

	require.Equal(t, `{"names":[{"count":2}]}`, string(resp.Json))
}

func EmptyRoomsWithTermIndex(t *testing.T, c *dgo.Dgraph) {
	op := &api.Operation{}
	op.Schema = `
		room: string @index(term) .
		office.room: [uid] .
	`
	ctx := context.Background()
	err := c.Alter(ctx, op)
	require.NoError(t, err)

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:o <office> "office 1" .
			_:o <office.room> _:x .
			_:o <office.room> _:y .
			_:o <office.room> _:z .
			_:x <room> "room 1" .
			_:y <room> "room 2" .
			_:z <room> "" .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err := c.NewTxn().Query(ctx, `{
		  offices(func: has(office)) {
			count(office.room @filter(eq(room, "")))
		  }
		}`)
	require.NoError(t, err)
	require.Equal(t, `{"offices":[{"count(office.room)":1}]}`, string(resp.GetJson()))
}

func DeleteWithExpandAll(t *testing.T, c *dgo.Dgraph) {
	op := &api.Operation{}
	op.Schema = `
		type Node {
			to: uid
			name: string
		}
`

	ctx := context.Background()
	err := c.Alter(ctx, op)
	require.NoError(t, err)

	ctx = context.Background()
	assigned, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <dgraph.type> "Node" .
			_:b <dgraph.type> "Node" .

			_:a <to> _:b .
			_:b <name> "b" .
			_:a <to> _:c .
			_:a <to> _:d .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)
	auid := assigned.Uids["a"]
	buid := assigned.Uids["b"]

	_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		DelNquads: []byte(`
			<` + auid + `> <to> <` + buid + `> .
			<` + buid + `> * * .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	q := `query test($id: string) {
		  me(func: uid($id)) {
			expand(_all_) {
				uid
			}
		  }
		}`

	type Root struct {
		Me []map[string]interface{} `json:"me"`
	}

	var r Root
	resp, err := c.NewTxn().QueryWithVars(ctx, q, map[string]string{"$id": auid})
	require.NoError(t, err)
	json.Unmarshal(resp.Json, &r)
	// S P O deletion shouldn't delete "to" .
	require.Equal(t, 1, len(r.Me[0]))

	// b should not have any predicates.
	resp, err = c.NewTxn().QueryWithVars(ctx, q, map[string]string{"$id": buid})
	require.NoError(t, err)
	json.Unmarshal(resp.Json, &r)
	require.Equal(t, 0, len(r.Me))
}

func testTimeValue(t *testing.T, c *dgo.Dgraph, timeBytes []byte) {
	nquads := []*api.NQuad{
		{
			Subject:   "0x01",
			Predicate: "friend",
			ObjectId:  "0x02",
			Facets: []*api.Facet{
				{
					Key:     "since",
					Value:   timeBytes,
					ValType: api.Facet_DATETIME,
				},
			},
		},
	}
	mu := &api.Mutation{Set: nquads, CommitNow: true}
	ctx := context.Background()
	_, err := c.NewTxn().Mutate(ctx, mu)
	require.NoError(t, err)

	q := `query test($id: string) {
		  me(func: uid($id)) {
			friend @facets
		  }
		}`

	resp, err := c.NewTxn().QueryWithVars(ctx, q, map[string]string{"$id": "0x1"})
	require.NoError(t, err)
	require.Contains(t, string(resp.Json), "since")
}

func FacetsUsingNQuadsError(t *testing.T, c *dgo.Dgraph) {
	// test time in go binary format
	timeBinary, err := time.Now().MarshalBinary()
	require.NoError(t, err)
	testTimeValue(t, c, timeBinary)

	// test time in full RFC3339 string format
	testTimeValue(t, c, []byte(time.Now().Format(time.RFC3339)))

	// test time in partial string formats
	testTimeValue(t, c, []byte("2018"))
	testTimeValue(t, c, []byte("2018-01"))
	testTimeValue(t, c, []byte("2018-01-01"))
}

func SkipEmptyPLForHas(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	_, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:u <__user> "true" .
			_:u <name> "u" .
			_:u1 <__user> "true" .
			_:u1 <name> "u1" .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	q := `{
		  users(func: has(__user)) {
			name
		  }
		}`
	resp, err := c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	z.CompareJSON(t, `{"users":[{"name":"u"},{"name":"u1"}]}`, string(resp.Json))

	op := &api.Operation{DropAll: true}
	err = c.Alter(ctx, op)
	require.NoError(t, err)

	q = `{
		  users(func: has(__user)) {
			uid
			name
		  }
		}`
	resp, err = c.NewTxn().Query(ctx, q)
	require.NoError(t, err)
	require.JSONEq(t, `{"users": []}`, string(resp.Json))
}

func HasWithDash(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, (c.Alter(ctx, &api.Operation{
		Schema: `name: string @index(hash) .`,
	})))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "Alice" .
			_:a <new-friend> _:b .
			_:b <name> "Bob" .
			_:a <new-friend> _:c .
			_:c <name> "Charlie" .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	const friendQuery = `
	{
		q(func: has(new-friend)) {
			new-friend {
				name
			}
		}
	}`

	txn = c.NewTxn()
	resp, err := txn.Query(ctx, friendQuery)
	require.NoError(t, err)
	z.CompareJSON(t, `{"q":[{"new-friend":[{"name":"Bob"},{"name":"Charlie"}]}]}`, string(resp.Json))
}

func ListGeoFilterTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(term) .
			loc: [geo] @index(geo) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:a <name> "read"  .
			_:b <name> "write" .
			_:c <name> "admin" .

			_:a <loc> "{'type':'Point','coordinates':[-122.4220186,37.772318]}"^^<geo:geojson> .
			_:b <loc> "{'type':'Point','coordinates':[-62.4220186,37.772318]}"^^<geo:geojson>  .
			_:c <loc> "{'type':'Point','coordinates':[-62.4220186,37.772318]}"^^<geo:geojson>  .
			_:c <loc> "{'type':'Point','coordinates':[-122.4220186,37.772318]}"^^<geo:geojson> .
		`),
	})
	check(t, err)

	resp, err := c.NewTxn().Query(context.Background(), `{
		q(func: near(loc, [-122.4220186,37.772318], 1000)) {
			name
		}
	}`)
	check(t, err)
	z.CompareJSON(t, `
	{
		"q": [
			{
				"name": "read"
			},
			{
				"name": "admin"
			}
		]
	}
	`, string(resp.GetJson()))
}

func ListRegexFilterTest(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(term) .
			per: [string] @index(trigram) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:a <name> "read"  .
			_:b <name> "write" .
			_:c <name> "admin" .

			_:a <per> "read"  .
			_:b <per> "write" .
			_:c <per> "read"  .
			_:c <per> "write" .
		`),
	})
	check(t, err)

	resp, err := c.NewTxn().Query(context.Background(), `{
		q(func: regexp(per, /^rea.*$/)) {
			name
		}
	}`)
	check(t, err)
	z.CompareJSON(t, `
	{
		"q": [
			{
				"name": "read"
			},
			{
				"name": "admin"
			}
		]
	}
	`, string(resp.GetJson()))
}

func RegexQueryWithVars(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(term) .
			per: [string] @index(trigram) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:a <name> "read"  .
			_:b <name> "write" .
			_:c <name> "admin" .

			_:a <per> "read"  .
			_:b <per> "write" .
			_:c <per> "read"  .
			_:c <per> "write" .
		`),
	})
	check(t, err)

	resp, err := c.NewTxn().QueryWithVars(context.Background(), `
		query search($term: string){
			q(func: regexp(per, $term)) {
				name
			}
		}`, map[string]string{"$term": "/^rea.*$/"})
	check(t, err)
	z.CompareJSON(t, `
	{
		"q": [
			{
				"name": "read"
			},
			{
				"name": "admin"
			}
		]
	}
	`, string(resp.GetJson()))
}

func GraphQLVarChild(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(exact) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	au, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:a <name> "Alice"  .
			_:b <name> "Bob" .
			_:c <name> "Charlie" .

			_:a <friend> _:c  .
			_:a <friend> _:b  .
		`),
	})
	check(t, err)

	a := au.Uids["a"]
	b := au.Uids["b"]
	ch := au.Uids["c"]

	// Try GraphQL variable with filter at root.
	resp, err := c.NewTxn().QueryWithVars(context.Background(), `
	query q($alice: string){
		q(func: eq(name, "Alice")) @filter(uid($alice)) {
			name
		}
	}`, map[string]string{"$alice": a})
	check(t, err)
	z.CompareJSON(t, `
	{
		"q": [
			{
				"name": "Alice"
			}
		]
	}
	`, string(resp.GetJson()))

	// Try GraphQL variable with filter at child.
	resp, err = c.NewTxn().QueryWithVars(context.Background(), `
	query q($bob: string){
		q(func: eq(name, "Alice")) {
			name
			friend @filter(uid($bob)) {
				name
			}
		}
	}`, map[string]string{"$bob": b})
	check(t, err)
	z.CompareJSON(t, `
	{
		"q": [
			{
				"name": "Alice",
				"friend": [
					{
						"name": "Bob"
					}
				]
			}
		]
	}
	`, string(resp.GetJson()))

	// Try GraphQL array variable with filter at child.
	friends := "[" + b + "," + ch + "]"
	resp, err = c.NewTxn().QueryWithVars(context.Background(), `
	query q($friends: string){
		q(func: eq(name, "Alice")) {
			name
			friend @filter(uid($friends)) {
				name
			}
		}
	}`, map[string]string{"$friends": friends})
	check(t, err)
	z.CompareJSON(t, `
	{
		"q": [
			{
				"name": "Alice",
				"friend": [
					{
						"name": "Bob"
					},
					{
						"name": "Charlie"
					}
				]
			}
		]
	}
	`, string(resp.GetJson()))

}

func MathGe(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(exact) .
		`,
	}))

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:root <name> "/" .
			_:root <container> _:c .
			_:c <name> "Foobar" .
		`),
	})
	check(t, err)

	// Try GraphQL variable with filter at root.
	resp, err := c.NewTxn().Query(context.Background(), `
	{
		q(func: eq(name, "/")) {
			containerCount as count(container)
			hasChildren: math(containerCount >= 1)
		}
	}`)
	check(t, err)
	z.CompareJSON(t, `
		{
		  "q": [
		    {
		      "count(container)": 1,
		      "hasChildren": true
		    }
		  ]
		}
	`, string(resp.GetJson()))
}

func HasDeletedEdge(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	txn := c.NewTxn()
	defer txn.Discard(ctx)

	assigned, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:a <end> "" .
			_:b <end> "" .
			_:c <end> "" .
			_:d <start> "" .
			_:d2 <start> "" .
		`),
	})
	check(t, err)

	var ids []string
	for key, uid := range assigned.Uids {
		if !strings.HasPrefix(key, "d") {
			ids = append(ids, uid)
		}
	}
	sort.Strings(ids)

	type U struct {
		Uid string `json:"uid"`
	}

	getUids := func(txn *dgo.Txn) []string {
		resp, err := txn.Query(ctx, `{
			me(func: has(end)) { uid }
			you(func: has(end)) { count(uid) }
		}`)
		check(t, err)
		t.Logf("resp: %s\n", resp.GetJson())
		m := make(map[string][]U)
		err = json.Unmarshal(resp.GetJson(), &m)
		check(t, err)
		uids := m["me"]
		var result []string
		for _, uid := range uids {
			result = append(result, uid.Uid)
		}
		return result
	}

	txn = c.NewTxn()
	defer txn.Discard(ctx)
	uids := getUids(txn)
	require.Equal(t, 3, len(uids))
	for _, uid := range uids {
		require.Contains(t, ids, uid)
	}

	deleteMu := &api.Mutation{
		CommitNow: true,
	}
	deleteMu.DelNquads = []byte(fmt.Sprintf(`
		<%s> <end> * .
	`, ids[len(ids)-1]))
	t.Logf("deleteMu: %+v\n", deleteMu)
	_, err = txn.Mutate(ctx, deleteMu)
	check(t, err)

	txn = c.NewTxn()
	defer txn.Discard(ctx)
	uids = getUids(txn)
	require.Equal(t, 2, len(uids))
	for _, uid := range uids {
		require.Contains(t, ids, uid)
	}
	// Remove the last entry from ids.
	ids = ids[:len(ids)-1]

	// We must commit mutations before we expect them to show up as results in
	// queries, involving secondary indices.
	assigned, err = txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:d <end> "" .
		`),
	})
	check(t, err)

	require.Equal(t, 1, len(assigned.Uids))
	for _, uid := range assigned.Uids {
		ids = append(ids, uid)
	}

	txn = c.NewTxn()
	defer txn.Discard(ctx)
	uids = getUids(txn)
	require.Equal(t, 3, len(uids))
	for _, uid := range uids {
		require.Contains(t, ids, uid)
	}
}

func HasReverseEdge(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	check(t, c.Alter(ctx, &api.Operation{
		Schema: `
			follow: [uid] @reverse .
		`,
	}))
	txn := c.NewTxn()
	defer txn.Discard(ctx)

	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:alice <name> "alice" .
			_:bob <name> "bob" .
			_:carol <name> "carol" .
			_:alice <follow> _:carol .
			_:bob <follow> _:carol .
		`),
	})
	check(t, err)

	type F struct {
		Name string `json:"name"`
	}

	txn = c.NewTxn()
	defer txn.Discard(ctx)
	resp, err := txn.Query(ctx, `{
		fwd(func: has(follow)) { name }
		rev(func: has(~follow)) { name }
		}`)
	check(t, err)

	t.Logf("resp: %s\n", resp.GetJson())
	m := make(map[string][]F)
	err = json.Unmarshal(resp.GetJson(), &m)
	check(t, err)

	fwds := m["fwd"]
	revs := m["rev"]

	require.Equal(t, len(fwds), 2)
	require.Contains(t, []string{"alice", "bob"}, fwds[0].Name)
	require.Contains(t, []string{"alice", "bob"}, fwds[1].Name)
	require.NotEqual(t, fwds[0].Name, fwds[1].Name)

	require.Equal(t, len(revs), 1)
	require.Equal(t, revs[0].Name, "carol")
}

func MaxPredicateSize(t *testing.T, c *dgo.Dgraph) {
	// Create a string that has more than than 2^16 chars.
	var b strings.Builder
	for i := 0; i < 10000; i++ {
		b.WriteString("abcdefg")
	}
	largePred := b.String()

	// Verify that Alter requests with predicates that are too large are rejected.
	ctx := context.Background()
	err := c.Alter(ctx, &api.Operation{
		Schema: fmt.Sprintf(`%s: uid @reverse .`, largePred),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Predicate name length cannot be bigger than 2^16")

	// Verify that Mutate requests with predicates that are too large are rejected.
	txn := c.NewTxn()
	_, err = txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`_:test <%s> "value" .`, largePred)),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Predicate name length cannot be bigger than 2^16")
	_ = txn.Discard(ctx)

	// Do the same thing as above but for the predicates in DelNquads.
	txn = c.NewTxn()
	defer txn.Discard(ctx)
	_, err = txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		DelNquads: []byte(fmt.Sprintf(`_:test <%s> "value" .`, largePred)),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Predicate name length cannot be bigger than 2^16")
}

func RestoreReservedPreds(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	err := c.Alter(ctx, &api.Operation{
		DropAll: true,
	})
	require.NoError(t, err)

	// Verify that the reserved predicates were restored to the schema.
	query := `schema(preds: dgraph.type) {predicate}`
	resp, err := c.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err)
	z.CompareJSON(t, `{"schema": [{"predicate":"dgraph.type"}]}`, string(resp.Json))
}

func DropData(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
			name: string @index(term) .
			follow: [uid] @reverse .
		`,
	}))

	txn := c.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			_:alice <name> "alice" .
			_:bob <name> "bob" .
			_:carol <name> "carol" .
			_:alice <follow> _:carol .
			_:bob <follow> _:carol .
		`),
	})
	require.NoError(t, err)

	err = c.Alter(ctx, &api.Operation{
		DropOp: api.Operation_DATA,
	})
	require.NoError(t, err)

	// Check schema is still there.
	query := `schema(preds: [name, follow]) {predicate}`
	resp, err := c.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err)
	z.CompareJSON(t, `{"schema": [{"predicate":"name"}, {"predicate":"follow"}]}`, string(resp.Json))

	// Check data is gone.
	resp, err = c.NewTxn().Query(ctx, `{
		q(func: has(name)) {
			uid
			name
		}
	}`)
	require.NoError(t, err)
	z.CompareJSON(t, `{"q": []}`, string(resp.GetJson()))
}

func DropDataAndDropAll(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	err := c.Alter(ctx, &api.Operation{
		DropAll: true,
		DropOp:  api.Operation_DATA,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only one of DropAll and DropData can be true")
}

func DropType(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{
		Schema: `
			type Person {
				name: string
			}
		`,
	}))

	// Check type has been added.
	query := `schema(type: Person) {}`
	resp, err := c.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err)
	z.CompareJSON(t, `{"types":[{"name":"Person",
		"fields":[{"name":"name", "type":"string"}]}]}`, string(resp.Json))

	require.NoError(t, c.Alter(ctx, &api.Operation{
		DropOp:    api.Operation_TYPE,
		DropValue: "Person",
	}))

	// Check type is gone.
	resp, err = c.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err)
	z.CompareJSON(t, "{}", string(resp.Json))
}

func DropTypeNoValue(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()
	err := c.Alter(ctx, &api.Operation{
		DropOp: api.Operation_TYPE,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "DropValue must not be empty")
}
