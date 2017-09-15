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
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/dgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var q0 = `
	{
		user(func: uid(0x1)) {
			name
		}
	}
`

var m = `
	mutation {
		set {
                        # comment line should be ignored
			<0x1> <name> "Alice" .
		}
	}
`

func prepare() (dir1, dir2 string, rerr error) {
	var err error
	dir1, err = ioutil.TempDir("", "storetest_")
	if err != nil {
		return "", "", err
	}

	dir2, err = ioutil.TempDir("", "wal_")
	if err != nil {
		return dir1, "", err
	}

	dgraph.Config.PostingDir = dir1
	dgraph.Config.PostingTables = "loadtoram"
	dgraph.Config.WALDir = dir2
	dgraph.State = dgraph.NewServerState()

	posting.Init(dgraph.State.Pstore)
	x.Check(group.ParseGroupConfig(""))
	schema.Init(dgraph.State.Pstore)
	worker.Init(dgraph.State.Pstore)
	worker.StartRaftNodes(dgraph.State.WALstore, false)
	return dir1, dir2, nil
}

func closeAll(dir1, dir2 string) {
	os.RemoveAll(dir1)
	os.RemoveAll(dir2)
}

func childAttrs(sg *query.SubGraph) []string {
	var out []string
	for _, c := range sg.Children {
		out = append(out, c.Attr)
	}
	return out
}

func defaultContext() context.Context {
	return context.WithValue(context.Background(), "mutation_allowed", true)
}

func processToFastJSON(q string) string {
	res, err := gql.Parse(gql.Request{Str: q, Http: true})
	if err != nil {
		log.Fatal(err)
	}

	var l query.Latency
	ctx := defaultContext()
	qr := query.QueryRequest{Latency: &l, GqlQuery: &res}
	err = qr.ProcessQuery(ctx)

	if err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	err = query.ToJson(&l, qr.Subgraphs, &buf, nil, false)
	if err != nil {
		log.Fatal(err)
	}
	return string(buf.Bytes())
}

func runQuery(q string) (string, error) {
	req, err := http.NewRequest("POST", "/query", bytes.NewBufferString(q))
	if err != nil {
		return "", err
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(queryHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		return "", fmt.Errorf("Unexpected status code: %v", status)
	}

	var qr x.QueryResWithData
	json.Unmarshal(rr.Body.Bytes(), &qr)
	if len(qr.Errors) > 0 {
		return "", errors.New(qr.Errors[0].Message)
	}
	return rr.Body.String(), nil
}

func runMutation(m string) error {
	res, err := gql.Parse(gql.Request{Str: m, Http: true})
	if err != nil {
		return err
	}

	var l query.Latency
	qr := query.QueryRequest{Latency: &l, GqlQuery: &res}
	_, err = qr.ProcessWithMutation(defaultContext())
	return err
}

func TestDeletePredicate(t *testing.T) {
	var m2 = `
	mutation {
		delete {
			* <friend> * .
			* <name> * .
			* <salary> * .
		}
	}
	`

	var m1 = `
	mutation {
		set {
			<0x1> <friend> <0x2> .
			<0x1> <friend> <0x3> .
			<0x1> <name> "Alice" .
			<0x2> <name> "Alice1" .
			<0x3> <name> "Alice2" .
			<0x3> <age> "13" .
			<0x11> <salary> "100000" . # should be deleted from schema after we delete the predicate
		}
	}
	`

	var q1 = `
	{
		user(func: anyofterms(name, "alice")) {
			friend {
				name
			}
		}
	}
	`
	var q2 = `
	{
		user(func: uid(0x1, 0x2, 0x3)) {
			name
		}
	}
	`
	var q3 = `
	{
		user(func: uid(0x3)) {
			age
			~friend {
				name
			}
		}
	}
	`

	var q4 = `
		{
			user(func: uid(0x3)) {
				_predicate_
			}
		}
	`

	var q5 = `
		{
			user(func: uid( 0x3)) {
				age
				friend {
					name
				}
			}
		}
		`

	var s1 = `
	mutation {
		schema {
			friend: uid @reverse .
			name: string @index(term) .
		}
	}
	`

	var s2 = `
		mutation {
			schema {
				friend: string @index(term) .
				name: uid @reverse .
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
	var m map[string]interface{}
	err = json.Unmarshal([]byte(output), &m)
	require.NoError(t, err)
	friends := m["data"].(map[string]interface{})["user"].([]interface{})[0].(map[string]interface{})["friend"].([]interface{})
	require.Equal(t, 2, len(friends))

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"name":"Alice"},{"name":"Alice1"},{"name":"Alice2"}]}}`,
		output)

	output, err = runQuery(q3)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"age": "13", "~friend" : [{"name":"Alice"}]}]}}`, output)

	output, err = runQuery(q4)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"_predicate_":["name","age"]}]}}`, output)

	err = runMutation(m2)
	require.NoError(t, err)

	output, err = runQuery(`schema{}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"schema":[{"predicate":"_predicate_","type":"string","list":true},{"predicate":"age","type":"default"},{"predicate":"friend","type":"uid","reverse":true},{"predicate":"name","type":"string","index":true,"tokenizer":["term"]}]}}`, output)

	output, err = runQuery(q1)
	require.JSONEq(t, `{"data": {}}`, output)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {}}`, output)

	output, err = runQuery(q5)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"age": "13"}]}}`, output)

	output, err = runQuery(q4)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"_predicate_":["name","age"]}]}}`, output)

	// Lets try to change the type of predicates now.
	err = runMutation(s2)
	require.NoError(t, err)
}

func TestSchemaMutation(t *testing.T) {
	var m = `
	mutation {
		schema {
			name:string @index(term, exact) .
			alias:string @index(exact, term) .
			dob:dateTime @index(year) .
			film.film.initial_release_date:dateTime @index(year) .
			loc:geo @index(geo) .
			genre:uid @reverse .
			survival_rate : float .
			alive         : bool .
			age           : int .
			shadow_deep   : int .
			friend:uid @reverse .
			geometry:geo @index(geo) .
		}
	}

` // reset schema
	schema.ParseBytes([]byte(""), 1)
	expected := map[string]*protos.SchemaUpdate{
		"name": {
			Tokenizer: []string{"term", "exact"},
			ValueType: uint32(types.StringID),
			Directive: protos.SchemaUpdate_INDEX,
			Explicit:  true},
	}

	err := runMutation(m)
	require.NoError(t, err)
	for k, v := range expected {
		s, ok := schema.State().Get(k)
		require.True(t, ok)
		require.Equal(t, *v, s)
	}
}

func TestSchemaMutation1(t *testing.T) {
	var m = `
	mutation {
		set {
			<0x1234> <pred1> "12345"^^<xs:string> .
			<0x1234> <pred2> "12345" .
		}
	}

` // reset schema
	schema.ParseBytes([]byte(""), 1)
	expected := map[string]*protos.SchemaUpdate{
		"pred1": {
			ValueType: uint32(types.StringID)},
		"pred2": {
			ValueType: uint32(types.DefaultID)},
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

// index on uid type
func TestMutation4Error(t *testing.T) {
	var m = `
	mutation {
		set {
      <1> <_age_> "5" .
		}
	}
	`
	err := runMutation(m)
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
			<0x1> <name> "Alice" .
		}
	}
	`

	var s = `
	mutation {
		schema {
            name:string @index(term) .
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
	require.JSONEq(t, `{"data": {"user":[{"name":"Alice"}]}}`, output)

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
			<0x1> <name> "Alice" .
		}
	}
	`

	var s1 = `
	mutation {
		schema {
            name:string @index(term) .
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
	require.JSONEq(t, `{"data": {"user":[{"name":"Alice"}]}}`, output)

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
		user(func: uid(0x3)) {
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
			<0x1> <friend> <0x3> .
			<0x1> <name> "Alice" .
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
	require.JSONEq(t, `{"data": {"user":[{"~friend" : [{"name":"Alice"}]}]}}`, output)

}

// Remove reverse edge
func TestSchemaMutationReverseRemove(t *testing.T) {
	var q1 = `
	{
		user(func: uid(0x3)) {
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
			<0x1> <friend> <0x3> .
			<0x1> <name> "Alice" .
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
	require.JSONEq(t, `{"data": {"user":[{"~friend" : [{"name":"Alice"}]}]}}`, output)

	// remove reverse edge
	err = runMutation(s2)
	require.NoError(t, err)

	output, err = runQuery(q1)
	require.Error(t, err)
}

// add count edges
func TestSchemaMutationCountAdd(t *testing.T) {
	var q1 = `
	{
		user(func:eq(count(friend),4)) {
			name
		}
	}
	`
	var m = `
	mutation {
		set {
                        # comment line should be ignored
			<0x1> <name> "Alice" .
			<0x01> <friend> <0x02> .
			<0x01> <friend> <0x03> .
			<0x01> <friend> <0x04> .
			<0x01> <friend> <0x05> .
		}
	}
	`

	var s = `
	mutation {
		schema {
			friend:uid @count .
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

	time.Sleep(10 * time.Millisecond)
	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"name":"Alice"}]}}`, output)

}

func TestDeleteAll(t *testing.T) {
	var q1 = `
	{
		user(func: uid(0x3)) {
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
			<0x1> <friend> * .
			<0x1> <name> * .
		}
	}
	`
	var m1 = `
	mutation {
		set {
			<0x1> <friend> <0x2> .
			<0x1> <friend> <0x3> .
			<0x1> <name> "Alice" .
			<0x2> <name> "Alice1" .
			<0x3> <name> "Alice2" .
		}
	}
	`

	var s1 = `
	mutation {
		schema {
      friend:uid @reverse .
			name: string @index(term) .
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
	require.JSONEq(t, `{"data": {"user":[{"~friend" : [{"name":"Alice"}]}]}}`, output)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"friend":[{"name":"Alice1"},{"name":"Alice2"}]}]}}`,
		output)

	err = runMutation(m2)
	require.NoError(t, err)

	output, err = runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {}}`, output)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {}}`, output)
}

func TestDeleteAllSP(t *testing.T) {
	var q1 = `
	{
		user(func: uid(0x3)) {
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
	var q3 = `
	{
		user(func: uid( 0x1)) {
			_predicate_
		}
	}
	`
	var q4 = `
	{
		user(func: uid( 0x1)) {
			count(_predicate_)
		}
	}
	`
	var q5 = `
	{
		user(func: uid( 0x1)) {
			pred_count: count(_predicate_)
		}
	}
	`

	var m2 = `
	mutation{
		delete{
			uid(a) * * .
		}
	}

	{
		a as var(func: uid(1))
	}
	`
	var m1 = `
	mutation {
		set {
			<0x1> <friend> <0x2> .
			<0x1> <friend> <0x3> .
			<0x1> <name> "Alice" .
			<0x2> <name> "Alice1" .
			<0x3> <name> "Alice2" .
		}
	}
	`

	var s1 = `
	mutation {
		schema {
			friend:uid @reverse .
			name: string @index(term) .
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
	require.JSONEq(t, `{"data": {"user":[{"~friend" : [{"name":"Alice"}]}]}}`, output)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"friend":[{"name":"Alice1"},{"name":"Alice2"}]}]}}`,
		output)

	output, err = runQuery(q3)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"_predicate_":["name","friend"]}]}}`,
		output)

	output, err = runQuery(q4)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"count(_predicate_)":2}]}}`,
		output)

	output, err = runQuery(q5)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"pred_count":2}]}}`,
		output)

	err = runMutation(m2)
	require.NoError(t, err)

	output, err = runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {}}`, output)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {}}`, output)

	output, err = runQuery(q3)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {}}`,
		output)
}

func TestDeleteAllSP1(t *testing.T) {
	var m = `
	mutation{
		delete{
			<2000> * * .
		}
	}`
	time.Sleep(20 * time.Millisecond)
	err := runMutation(m)
	require.NoError(t, err)
}

func TestQuery(t *testing.T) {
	res, err := gql.Parse(gql.Request{Str: m, Http: true})
	require.NoError(t, err)

	var l query.Latency
	qr := query.QueryRequest{Latency: &l, GqlQuery: &res}
	_, err = qr.ProcessWithMutation(defaultContext())

	output := processToFastJSON(q0)
	require.JSONEq(t, `{"data": {"user":[{"name":"Alice"}]}}`, output)
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
		user(func: uid(<id>)) {
			name
		}
	}
`

func TestSchemaValidationError(t *testing.T) {
	_, err := gql.Parse(gql.Request{Str: m5, Http: true})
	require.Error(t, err)
	output := processToFastJSON(strings.Replace(q5, "<id>", "0x8", -1))
	require.JSONEq(t, `{"data": {}}`, output)
}

var m6 = `
	mutation {
		set {
                        # comment line should be ignored
			<0x5> <name2> "1"^^<xs:int> .
			<0x6> <name2> "1.5"^^<xs:float> .
		}
	}
`

var q6 = `
	{
		user(func: uid(<id>)) {
			name2
		}
	}
`

func TestSchemaConversion(t *testing.T) {
	res, err := gql.Parse(gql.Request{Str: m6, Http: true})
	require.NoError(t, err)

	var l query.Latency
	qr := query.QueryRequest{Latency: &l, GqlQuery: &res}
	_, err = qr.ProcessWithMutation(defaultContext())

	require.NoError(t, err)
	output := processToFastJSON(strings.Replace(q6, "<id>", "0x6", -1))
	require.JSONEq(t, `{"data": {"user":[{"name2":1}]}}`, output)

	s, ok := schema.State().Get("name2")
	require.True(t, ok)
	s.ValueType = uint32(types.FloatID)
	schema.State().Set("name2", s)
	output = processToFastJSON(strings.Replace(q6, "<id>", "0x6", -1))
	require.JSONEq(t, `{"data": {"user":[{"name2":1.5}]}}`, output)
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

	var l query.Latency
	qr := query.QueryRequest{Latency: &l, GqlQuery: &res}
	_, err = qr.ProcessWithMutation(defaultContext())
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

	var l query.Latency
	qr := query.QueryRequest{Latency: &l, GqlQuery: &res}
	er, err := qr.ProcessWithMutation(defaultContext())
	require.NoError(t, err)

	require.EqualValues(t, len(er.Allocations), 2, "Expected two UIDs to be allocated")
	_, ok := er.Allocations["x"]
	require.True(t, ok)
	_, ok = er.Allocations["y"]
	require.True(t, ok)
}

var q1 = `
{
	al(func: uid( 0x1)) {
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
	dir1, dir2, err := prepare()
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
			<0x1> <name> "Alice" .
			<0x1> <age> "13" .
			<0x1> <friend> <0x4> .
		}
	}
	`
	var s = `
	mutation {
		schema {
			name:string @index(term) .
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
	require.Equal(t, `{"data": {"listpred":[{"_predicate_":["name","age","friend"]}]}}`,
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
			<0x1> <name> "Alice" .
			<0x1> <age> "13" .
			<0x1> <friend> <0x4> .
			<0x4> <name> "bob" .
			<0x4> <age> "12" .
		}
	}
	`
	var s = `
	mutation {
		schema {
			name:string @index(term) .
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
		me(func: uid(0x11)) {
			expand(_all_) {
				expand(_all_)
			}
		}
	}
	`
	var m = `
	mutation {
		set {
			<0x11> <name> "Alice" .
			<0x11> <age> "13" .
			<0x11> <friend> <0x4> .
			<0x4> <name> "bob" .
			<0x4> <age> "12" .
		}
	}
	`
	var s = `
	mutation {
		schema {
			name:string @index(term) .
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
	require.JSONEq(t, `{"data": {"me":[{"age":"13","friend":[{"age":"12","name":"bob"}],"name":"Alice"}]}}`,
		output)
}

var threeNiceFriends = `{
	"data": {
	  "me": [
	    {
	      "friend": [
	        {
	          "nice": "true"
	        },
	        {
	          "nice": "true"
	        },
	        {
	          "nice": "true"
	        }
	      ]
	    }
	  ]
	}
}`

func TestMutationSubjectVariables(t *testing.T) {
	m1 := `
		mutation {
			set {
                <0x500>    <friend>   <_:alice> .
                <0x500>    <friend>   <_:bob> .
                <0x500>    <friend>   <_:chris> .
			}
		}
    `
	err := runMutation(m1)
	require.NoError(t, err)

	m2 := `
        mutation {
			set {
				uid(myfriend) <nice> "true" .
			}
		}
		{
			me(func: uid( 0x500)) {
				myfriend as friend
			}
		}`

	parsed, err := gql.Parse(gql.Request{Str: m2, Http: true})
	require.NoError(t, err)

	var l query.Latency
	qr := query.QueryRequest{Latency: &l, GqlQuery: &parsed}
	_, err = qr.ProcessWithMutation(defaultContext())
	require.NoError(t, err)

	q1 := `
		{
			me(func: uid( 0x500)) {
				friend  {
					nice
				}
			}
		}
    `
	r, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, threeNiceFriends, r)
}

func TestMutationSubjectVariablesSingleMutation(t *testing.T) {
	m1 := `
		mutation {
			set {
                <0x700>          <friend>   <_:alice> .
                <0x700>          <friend>   <_:bob> .
                <0x700>          <friend>   <_:chris> .
				uid(myfriend) <nice>     "true" .
			}
		}
		{
			me(func: uid( 0x700)) {
				myfriend as friend
			}
		}
    `

	parsed, err := gql.Parse(gql.Request{Str: m1, Http: true})
	require.NoError(t, err)

	var l query.Latency
	qr := query.QueryRequest{Latency: &l, GqlQuery: &parsed}
	_, err = qr.ProcessWithMutation(defaultContext())
	require.NoError(t, err)

	q1 := `
		{
			me(func: uid( 0x700)) {
				friend  {
					nice
				}
			}
		}
    `
	r, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, threeNiceFriends, r)
}

func TestMutationObjectVariables(t *testing.T) {
	m1 := `
		mutation {
			set {
                <0x600>    <friend>   <0x501> .
                <0x600>    <friend>   <0x502> .
                <0x600>    <friend>   <0x503> .
				<0x600>    <likes>    uid(myfriend) .
			}
		}
		{
			me(func: uid( 0x600)) {
				myfriend as friend
			}
		}
    `

	parsed, err := gql.Parse(gql.Request{Str: m1, Http: true})
	require.NoError(t, err)

	var l query.Latency
	qr := query.QueryRequest{Latency: &l, GqlQuery: &parsed}
	_, err = qr.ProcessWithMutation(defaultContext())

	require.NoError(t, err)

	q1 := `
		{
			me(func: uid( 0x600)) {
				count(likes)
            }
		}
    `
	r, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"count(likes)":3}]}}`, r)
}

func TestMutationSubjectObjectVariables(t *testing.T) {
	m1 := `
		mutation {
			set {
				<0x601>    <friend>   <0x501> .
				<0x601>    <friend>   <0x502> .
				<0x601>    <friend>   <0x503> .
				uid(user)    <likes>    uid(myfriend) .
			}
		}
		{
			user as var(func: uid(0x601))
			me(func: uid(0x601)) {
				myfriend as friend
			}
		}
    `

	parsed, err := gql.Parse(gql.Request{Str: m1, Http: true})
	require.NoError(t, err)

	var l query.Latency
	qr := query.QueryRequest{Latency: &l, GqlQuery: &parsed}
	_, err = qr.ProcessWithMutation(defaultContext())

	require.NoError(t, err)

	q1 := `
		{
			me(func: uid(0x601)) {
				count(likes)
            }
		}
    `
	r, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"count(likes)":3}]}}`, r)
}

func TestMutationObjectVariablesError(t *testing.T) {
	m1 := `
		mutation {
			set {
                <0x600>    <friend>   <0x501> .
                <0x600>    <friend>   <0x502> .
                <0x600>    <friend>   <0x503> .
				<0x600>    <likes>    val(myfriend) .
			}
		}
		{
			me(func: uid(0x600)) {
				myfriend as friend
			}
		}
    `

	_, err := gql.Parse(gql.Request{Str: m1, Http: true})
	require.Error(t, err)
}

// change from uid to scalar or vice versa
func TestSchemaMutation4Error(t *testing.T) {
	var m = `
	mutation {
		schema {
            age:int .
		}
	}
	`
	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	m = `
	mutation {
		set {
			<0x9> <age> "13" .
		}
	}
	`
	err = runMutation(m)
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

// change from uid to scalar or vice versa
func TestSchemaMutation5Error(t *testing.T) {
	var m = `
	mutation {
		schema {
            friends:uid .
		}
	}
	`
	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	m = `
	mutation {
		set {
			<0x8> <friends> <0x5> .
		}
	}
	`
	err = runMutation(m)
	require.NoError(t, err)

	m = `
	mutation {
		schema {
            friends:string .
		}
	}
	`
	err = runMutation(m)
	require.Error(t, err)
}

// A basic sanity check. We will do more extensive testing for multiple values in query.
func TestMultipleValues(t *testing.T) {
	schema.ParseBytes([]byte(""), 1)
	m := `
	mutation {
		schema {
			occupations: [string] .
		}
	}`

	err := runMutation(m)
	require.NoError(t, err)

	m = `
		mutation {
			set {
				<0x88> <occupations> "Pianist" .
				<0x88> <occupations> "Software Engineer" .
			}
		}
	`

	err = runMutation(m)
	require.NoError(t, err)

	q := `{
			me(func: uid(0x88)) {
				occupations
			}
		}`
	res, err := runQuery(q)
	require.NoError(t, err)
	require.Equal(t, `{"data": {"me":[{"occupations":["Software Engineer","Pianist"]}]}}`, res)
}

func TestListTypeSchemaChange(t *testing.T) {
	schema.ParseBytes([]byte(""), 1)
	m := `
	mutation {
		schema {
			occupations: [string] .
		}
	}`

	err := runMutation(m)
	require.NoError(t, err)

	m = `
		mutation {
			set {
				<0x88> <occupations> "Pianist" .
				<0x88> <occupations> "Software Engineer" .
			}
		}
	`

	err = runMutation(m)
	require.NoError(t, err)

	q := `{
			me(func: uid(0x88)) {
				occupations
			}
		}`
	res, err := runQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"occupations":["Software Engineer","Pianist"]}]}}`, res)

	m = `
		mutation {
			schema {
				occupations: string .
			}
		}
	`

	// Cant change from list-type to non-list till we have data.
	err = runMutation(m)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Schema change not allowed from [string] => string")

	sm := `
		mutation {
			delete {
				* <occupations> * .
			}
		}
	`
	err = runMutation(sm)
	require.NoError(t, err)

	require.NoError(t, runMutation(m))

	q = `schema{}`
	res, err = runQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"schema":[{"predicate":"_predicate_","type":"string","list":true},{"predicate":"occupations","type":"string"}]}}`, res)

}

func TestMain(m *testing.M) {
	dc := dgraph.DefaultConfig
	dc.AllottedMemory = 2048.0
	dgraph.SetConfiguration(dc)
	x.Init()

	dir1, dir2, _ := prepare()

	// Parse GQL into internal query representation.
	r := m.Run()
	closeAll(dir1, dir2)
	os.Exit(r)
}
