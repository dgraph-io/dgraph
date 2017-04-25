/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"bytes"
	"context"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var q0 = `
	{
		user(id:alice) {
			name
		}
	}
`
var m = `
	mutation {
		set {
                        # comment line should be ignored
			<alice> <name> "Alice" .
		}
	}
`

func prepare() (dir1, dir2 string, ps *store.Store, rerr error) {
	var err error
	dir1, err = ioutil.TempDir("", "storetest_")
	if err != nil {
		return "", "", nil, err
	}
	ps, err = store.NewStore(dir1)
	if err != nil {
		return "", "", nil, err
	}

	dir2, err = ioutil.TempDir("", "wal_")
	if err != nil {
		return dir1, "", nil, err
	}

	posting.Init(ps)
	group.ParseGroupConfig("groups.conf")
	schema.Init(ps)
	worker.StartRaftNodes(dir2)

	return dir1, dir2, ps, nil
}

func closeAll(dir1, dir2 string) {
	os.RemoveAll(dir2)
	os.RemoveAll(dir1)
}

func childAttrs(sg *query.SubGraph) []string {
	var out []string
	for _, c := range sg.Children {
		out = append(out, c.Attr)
	}
	return out
}

func processToFastJSON(q string) string {
	res, err := gql.Parse(gql.Request{Str: q, Http: true})
	if err != nil {
		log.Fatal(err)
	}

	var l query.Latency
	ctx := context.Background()
	sgl, err := query.ProcessQuery(ctx, res, &l)

	if err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	err = query.ToJson(&l, sgl, &buf, nil)
	if err != nil {
		log.Fatal(err)
	}
	return string(buf.Bytes())
}

func runQuery(q string) (string, error) {
	res, err := gql.Parse(gql.Request{Str: q, Http: true})
	if err != nil {
		return "", err
	}

	var l query.Latency
	ctx := context.Background()
	sgl, err := query.ProcessQuery(ctx, res, &l)

	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = query.ToJson(&l, sgl, &buf, nil)
	if err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

func runMutation(m string) error {
	res, err := gql.Parse(gql.Request{Str: m, Http: true})
	if err != nil {
		return err
	}

	ctx := context.Background()
	_, err = mutationHandler(ctx, res.Mutation)
	return err
}

func TestSchemaMutation(t *testing.T) {
	var m = `
	mutation {
		schema {
            name:string @index(term, exact) .
			alias:string @index(exact, term) .
			dob:date @index .
			film.film.initial_release_date:date @index .
			loc:geo @index .
			genre:uid @reverse .
			survival_rate : float .
			alive         : bool .
			age           : int .
			shadow_deep   : int .
			friend:uid @reverse .
			geometry:geo @index .
		}
	}

` // reset schema
	schema.ParseBytes([]byte(""), 1)
	expected := map[string]*typesp.Schema{
		"name": {
			Tokenizer: []string{"term", "exact"},
			ValueType: uint32(types.StringID),
			Directive: typesp.Schema_INDEX},
	}

	err := runMutation(m)
	require.NoError(t, err)
	for k, v := range expected {
		s, ok := schema.State().Get(k)
		require.True(t, ok)
		require.Equal(t, *v, s)
	}
}

// reverse on scalar type
func TestSchemaMutation2Error(t *testing.T) {
	var m = `
	mutation {
		schema {
            age:string @reverse .
		}
	}
	`

	err := runMutation(m)
	require.Error(t, err)
}

// index on uid type
func TestSchemaMutation3Error(t *testing.T) {
	var m = `
	mutation {
		schema {
            age:uid @index .
		}
	}
	`
	err := runMutation(m)
	require.Error(t, err)
}

// change from uid to scalar or vice versa
func TestSchemaMutation4Error(t *testing.T) {
	var m = `
	mutation {
		schema {
            age:uid .
		}
	}
	`
	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	m = `
	mutation {
		schema {
            age:string .
		}
	}
	`
	err = runMutation(m)
	require.Error(t, err)
}

// change from uid to scalar or vice versa
func TestSchemaMutation5Error(t *testing.T) {
	var m = `
	mutation {
		schema {
            age:string .
		}
	}
	`
	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	m = `
	mutation {
		schema {
            age:uid .
		}
	}
	`
	err = runMutation(m)
	require.Error(t, err)
}

// add index
func TestSchemaMutationIndexAdd(t *testing.T) {
	var q1 = `
	{
		user(func:anyofterms(name, "Alice")) {
			name
		}
	}
	`
	var m = `
	mutation {
		set {
                        # comment line should be ignored
			<alice> <name> "Alice" .
		}
	}
	`

	var s = `
	mutation {
		schema {
            name:string @index .
		}
	}
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = runMutation(s)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"user":[{"name":"Alice"}]}`, output)

}

// Remove index
func TestSchemaMutationIndexRemove(t *testing.T) {
	var q1 = `
	{
		user(func:anyofterms(name, "Alice")) {
			name
		}
	}
	`
	var m = `
	mutation {
		set {
                        # comment line should be ignored
			<alice> <name> "Alice" .
		}
	}
	`

	var s1 = `
	mutation {
		schema {
            name:string @index .
		}
	}
	`
	var s2 = `
	mutation {
		schema {
            name:string .
		}
	}
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	// add index to name
	err := runMutation(s1)
	require.NoError(t, err)

	err = runMutation(m)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"user":[{"name":"Alice"}]}`, output)

	// remove index
	err = runMutation(s2)
	require.NoError(t, err)

	output, err = runQuery(q1)
	require.Error(t, err)
}

// add reverse edge
func TestSchemaMutationReverseAdd(t *testing.T) {
	var q1 = `
	{
		user(id:alice2) {
			~friend {
				name
			}
		}
	}
	`
	var m = `
	mutation {
		set {
                        # comment line should be ignored
			<alice> <friend> <alice2> .
			<alice> <name> "Alice" .
		}
	}
	`

	var s = `
	mutation {
		schema {
            friend:uid @reverse .
		}
	}
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = runMutation(s)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"user":[{"~friend" : [{"name":"Alice"}]}]}`, output)

}

// Remove reverse edge
func TestSchemaMutationReverseRemove(t *testing.T) {
	var q1 = `
	{
		user(id:alice2) {
			~friend {
				name
			}
		}
	}
	`
	var m = `
	mutation {
		set {
                        # comment line should be ignored
			<alice> <friend> <alice2> .
			<alice> <name> "Alice" .
		}
	}
	`

	var s1 = `
	mutation {
		schema {
            friend:uid @reverse .
		}
	}
	`

	var s2 = `
	mutation {
		schema {
            friend:uid .
		}
	}
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add reverse edge to name
	err = runMutation(s1)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"user":[{"~friend" : [{"name":"Alice"}]}]}`, output)

	// remove reverse edge
	err = runMutation(s2)
	require.NoError(t, err)

	output, err = runQuery(q1)
	require.Error(t, err)
}

func TestDeleteAll(t *testing.T) {
	var q1 = `
	{
		user(id:alice2) {
			~friend {
				name
			}
		}
	}
	`
	var q2 = `
	{
		user(func: anyofterms(name, "alice")) {
			friend {
				name
			}
		}
	}
	`

	var m2 = `
	mutation{
		delete{
			<alice> <friend> * .
			<alice> <name> * .
		}
	}
	`
	var m1 = `
	mutation {
		set {
			<alice> <friend> <alice1> .
			<alice> <friend> <alice2> .
			<alice> <name> "Alice" .
			<alice1> <name> "Alice1" .
			<alice2> <name> "Alice2" .
		}
	}
	`

	var s1 = `
	mutation {
		schema {
      friend:uid @reverse .
			name: string @index .
		}
	}
	`
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(s1)
	require.NoError(t, err)

	err = runMutation(m1)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"user":[{"~friend" : [{"name":"Alice"}]}]}`, output)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"user":[{"friend":[{"name":"Alice1"},{"name":"Alice2"}]}]}`,
		output)

	err = runMutation(m2)
	require.NoError(t, err)

	output, err = runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, output)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, output)
}

func TestQuery(t *testing.T) {
	res, err := gql.Parse(gql.Request{Str: m, Http: true})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = mutationHandler(ctx, res.Mutation)

	output := processToFastJSON(q0)
	require.JSONEq(t, `{"user":[{"name":"Alice"}]}`, output)
}

var m5 = `
	mutation {
		set {
                        # comment line should be ignored
			<ram> <name> "1"^^<xs:int> .
			<shyam> <name> "abc"^^<xs:int> .
		}
	}
`

var q5 = `
	{
		user(id:<id>) {
			name
		}
	}
`

func TestSchemaValidationError(t *testing.T) {
	res, err := gql.Parse(gql.Request{Str: m5, Http: true})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = mutationHandler(ctx, res.Mutation)

	require.Error(t, err)
	output := processToFastJSON(strings.Replace(q5, "<id>", "ram", -1))
	require.JSONEq(t, `{}`, output)
}

var m6 = `
	mutation {
		set {
                        # comment line should be ignored
			<ram2> <name2> "1"^^<xs:int> .
			<shyam2> <name2> "1.5"^^<xs:float> .
		}
	}
`

var q6 = `
	{
		user(id:<id>) {
			name2
		}
	}
`

func TestSchemaConversion(t *testing.T) {
	res, err := gql.Parse(gql.Request{Str: m6, Http: true})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = mutationHandler(ctx, res.Mutation)

	require.NoError(t, err)
	output := processToFastJSON(strings.Replace(q6, "<id>", "shyam2", -1))
	require.JSONEq(t, `{"user":[{"name2":1}]}`, output)

	s, ok := schema.State().Get("name2")
	require.True(t, ok)
	s.ValueType = uint32(types.FloatID)
	schema.State().Set("name2", s)
	output = processToFastJSON(strings.Replace(q6, "<id>", "shyam2", -1))
	require.JSONEq(t, `{"user":[{"name2":1.5}]}`, output)
}

var qErr = `
	mutation {
		set {
			<0x0> <name> "Alice" .
		}
	}
`

func TestMutationError(t *testing.T) {
	res, err := gql.Parse(gql.Request{Str: qErr, Http: true})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = mutationHandler(ctx, res.Mutation)
	require.Error(t, err)

}

var qm = `
	mutation {
		set {
			<0x0a> <pred.rel> _:x .
			_:x <pred.val> "value" .
			_:x <pred.rel> _:y .
			_:y <pred.val> "value2" .
		}
	}
`

func TestAssignUid(t *testing.T) {
	res, err := gql.Parse(gql.Request{Str: qm, Http: true})
	require.NoError(t, err)

	ctx := context.Background()
	allocIds, err := mutationHandler(ctx, res.Mutation)
	require.NoError(t, err)

	require.EqualValues(t, len(allocIds), 2, "Expected two UIDs to be allocated")
	_, ok := allocIds["x"]
	require.True(t, ok)
	_, ok = allocIds["y"]
	require.True(t, ok)
}

func TestConvertToEdges(t *testing.T) {
	q1 := `<0x01> <type> <0x02> .
	       <0x01> <character> <0x03> .`
	nquads, err := convertToNQuad(context.Background(), q1)
	require.NoError(t, err)

	mr, err := convertToEdges(context.Background(), nquads)
	require.NoError(t, err)

	require.EqualValues(t, len(mr.edges), 2)
}

var q1 = `
{
	al(id: alice) {
		status
		follows {
			status
			follows {
				status
				follows {
					status
				}
			}
		}
	}
}
`

func BenchmarkQuery(b *testing.B) {
	dir1, dir2, _, err := prepare()
	if err != nil {
		b.Error(err)
		return
	}
	defer closeAll(dir1, dir2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processToFastJSON(q1)
	}
}

func TestListPred(t *testing.T) {
	var q1 = `
	{
		listpred(func:anyofterms(name, "Alice")) {
				_predicate_
		}
	}
	`
	var m = `
	mutation {
		set {
			<alice> <name> "Alice" .
			<alice> <age> "13" .
			<alice> <friend> <bob> .
		}
	}
	`
	var s = `
	mutation {
		schema {
            name:string @index .
		}
	}
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = runMutation(s)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.Equal(t, `{"listpred":[{"_predicate_":[{"_name_":"age"},{"_name_":"friend"},{"_name_":"name"}]}]}`,
		output)
}

func TestExpandPredError(t *testing.T) {
	var q1 = `
	{
		me(func:anyofterms(name, "Alice")) {
  		expand(_all_)
			name
			friend
		}
	}
	`
	var m = `
	mutation {
		set {
			<alice> <name> "Alice" .
			<alice> <age> "13" .
			<alice> <friend> <bob> .
			<bob> <name> "bob" .
			<bob> <age> "12" .
		}
	}
	`
	var s = `
	mutation {
		schema {
            name:string @index .
		}
	}
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = runMutation(s)
	require.NoError(t, err)

	_, err = runQuery(q1)
	require.Error(t, err)
}

func TestExpandPred(t *testing.T) {
	var q1 = `
	{
		me(func:anyofterms(name, "Alice")) {
			expand(_all_) {
  			expand(_all_)
			}
		}
	}
	`
	var m = `
	mutation {
		set {
			<alice> <name> "Alice" .
			<alice> <age> "13" .
			<alice> <friend> <bob> .
			<bob> <name> "bob" .
			<bob> <age> "12" .
		}
	}
	`
	var s = `
	mutation {
		schema {
            name:string @index .
		}
	}
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = runMutation(s)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"me":[{"age":"13","friend":[{"age":"12","name":"bob"}],"name":"Alice"}]}`,
		output)

}
func TestMain(m *testing.M) {
	x.Init()
	dir1, dir2, _, _ := prepare()
	defer closeAll(dir1, dir2)
	time.Sleep(5 * time.Second) // Wait for ME to become leader.

	// we need watermarks for reindexing
	x.AssertTrue(!x.IsTestRun())
	// Parse GQL into internal query representation.
	os.Exit(m.Run())
}
