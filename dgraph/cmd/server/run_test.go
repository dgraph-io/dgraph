/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	context "golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/schema"
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

type raftServer struct {
}

func (c *raftServer) Echo(ctx context.Context, in *api.Payload) (*api.Payload, error) {
	return in, nil
}

func (c *raftServer) RaftMessage(ctx context.Context, in *api.Payload) (*api.Payload, error) {
	return &api.Payload{}, nil
}

func (c *raftServer) JoinCluster(ctx context.Context, in *intern.RaftContext) (*api.Payload, error) {
	return &api.Payload{}, nil
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

var ts uint64

func timestamp() uint64 {
	return atomic.AddUint64(&ts, 1)
}

func processToFastJSON(q string) string {
	res, err := gql.Parse(gql.Request{Str: q})
	if err != nil {
		log.Fatal(err)
	}

	var l query.Latency
	ctx := defaultContext()
	qr := query.QueryRequest{Latency: &l, GqlQuery: &res, ReadTs: timestamp()}
	err = qr.ProcessQuery(ctx)

	if err != nil {
		log.Fatal(err)
	}

	buf, err := query.ToJson(&l, qr.Subgraphs)
	if err != nil {
		log.Fatal(err)
	}
	return string(buf)
}

func runQuery(q string) (string, error) {
	output, _, err := queryWithTs(q, 0)
	return string(output), err
}

func runMutation(m string) error {
	_, _, err := mutationWithTs(m, false, true, false, 0)
	return err
}

func runJsonMutation(m string) error {
	_, _, err := mutationWithTs(m, true, true, false, 0)
	return err
}

func alterSchema(s string) error {
	req, err := http.NewRequest("PUT", addr+"/alter", bytes.NewBufferString(s))
	if err != nil {
		return err
	}
	_, _, err = runRequest(req)
	return err
}

func alterSchemaWithRetry(s string) error {
	var err error
	for i := 0; i < 3; i++ {
		if err = alterSchema(s); err == nil {
			return nil
		}
	}
	return err
}

func dropAll() error {
	op := `{"drop_all": true}`
	req, err := http.NewRequest("PUT", addr+"/alter", bytes.NewBufferString(op))
	if err != nil {
		return err
	}
	_, _, err = runRequest(req)
	return err
}

func deletePredicate(pred string) error {
	op := `{"drop_attr": "` + pred + `"}`
	req, err := http.NewRequest("PUT", addr+"/alter", bytes.NewBufferString(op))
	if err != nil {
		return err
	}
	_, _, err = runRequest(req)
	return err
}

func TestDeletePredicate(t *testing.T) {
	var m1 = `
	{
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
			user(func: uid(0x3)) {
				age
				friend {
					name
				}
			}
		}
	`

	var s1 = `
	friend: uid @reverse .
	name: string @index(term) .
	`

	var s2 = `
	friend: string @index(term) .
	`

	require.NoError(t, dropAll())
	schema.ParseBytes([]byte(""), 1)
	err := alterSchemaWithRetry(s1)
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

	err = deletePredicate("friend")
	require.NoError(t, err)
	err = deletePredicate("salary")
	require.NoError(t, err)

	output, err = runQuery(`schema{}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"schema":[{"predicate":"_predicate_","type":"string","list":true},{"predicate":"age","type":"default"},{"predicate":"name","type":"string","index":true, "tokenizer":["term"]}]}}`, output)

	output, err = runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user": []}}`, output)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user": [{"name":"Alice"},{"name":"Alice1"},{"name":"Alice2"}]}}`, output)

	output, err = runQuery(q5)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"age": "13"}]}}`, output)

	output, err = runQuery(q4)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"_predicate_":["name","age"]}]}}`, output)

	// Lets try to change the type of predicates now.
	err = alterSchemaWithRetry(s2)
	require.NoError(t, err)
}

type S struct {
	Predicate string   `json:"predicate"`
	Type      string   `json:"type"`
	Index     bool     `json:"index"`
	Tokenizer []string `json:"tokenizer"`
}

type Received struct {
	Schema []S `json:"schema"`
}

func TestSchemaMutation(t *testing.T) {
	require.NoError(t, dropAll())
	var m = `
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
			geometry:geo @index(geo) . `

	expected := S{
		Predicate: "name",
		Type:      "string",
		Index:     true,
		Tokenizer: []string{"term", "exact"},
	}

	err := alterSchemaWithRetry(m)
	require.NoError(t, err)

	output, err := runQuery("schema {}")
	require.NoError(t, err)
	got := make(map[string]Received)
	require.NoError(t, json.Unmarshal([]byte(output), &got))
	received, ok := got["data"]
	require.True(t, ok)

	var found bool
	for _, s := range received.Schema {
		if s.Predicate == "name" {
			found = true
			require.Equal(t, expected, s)
		}
	}
	require.True(t, found)
}

func TestSchemaMutation1(t *testing.T) {
	require.NoError(t, dropAll())
	var m = `
	{
		set {
			<0x1234> <pred1> "12345"^^<xs:string> .
			<0x1234> <pred2> "12345" .
		}
	}

`
	err := runMutation(m)
	require.NoError(t, err)

	output, err := runQuery("schema {}")
	require.NoError(t, err)
	got := make(map[string]Received)
	require.NoError(t, json.Unmarshal([]byte(output), &got))
	received, ok := got["data"]
	require.True(t, ok)

	var count int
	for _, s := range received.Schema {
		if s.Predicate == "pred1" {
			require.Equal(t, "string", s.Type)
			count++
		} else if s.Predicate == "pred2" {
			require.Equal(t, "default", s.Type)
			count++
		}
	}
	require.Equal(t, 2, count)
}

// reverse on scalar type
func TestSchemaMutation2Error(t *testing.T) {
	var m = `
            age:string @reverse .
	`

	err := alterSchema(m)
	require.Error(t, err)
}

// index on uid type
func TestSchemaMutation3Error(t *testing.T) {
	var m = `
            age:uid @index .
	`
	err := alterSchema(m)
	require.Error(t, err)
}

func TestMutation4Error(t *testing.T) {
	t.Skip()
	var m = `
	{
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
	{
		set {
                        # comment line should be ignored
			<0x1> <name> "Alice" .
		}
	}
	`

	var s = `
            name:string @index(term) .
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = alterSchemaWithRetry(s)
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
	{
		set {
                        # comment line should be ignored
			<0x1> <name> "Alice" .
		}
	}
	`

	var s1 = `
            name:string @index(term) .
	`
	var s2 = `
            name:string .
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	// add index to name
	err := alterSchemaWithRetry(s1)
	require.NoError(t, err)

	err = runMutation(m)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"name":"Alice"}]}}`, output)

	// remove index
	err = alterSchemaWithRetry(s2)
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
	{
		set {
                        # comment line should be ignored
			<0x1> <friend> <0x3> .
			<0x1> <name> "Alice" .
		}
	}
	`

	var s = `friend:uid @reverse .`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = alterSchemaWithRetry(s)
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
	{
		set {
                        # comment line should be ignored
			<0x1> <friend> <0x3> .
			<0x1> <name> "Alice" .
		}
	}
	`

	var s1 = `
            friend:uid @reverse .
	`

	var s2 = `
            friend:uid .
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add reverse edge to name
	err = alterSchemaWithRetry(s1)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"~friend" : [{"name":"Alice"}]}]}}`, output)

	// remove reverse edge
	err = alterSchemaWithRetry(s2)
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
	{
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
			friend:uid @count .
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = alterSchemaWithRetry(s)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"name":"Alice"}]}}`, output)
}

func TestJsonMutation(t *testing.T) {
	var q1 = `
	{
		q(func: has(name)) {
			uid
			name
		}
	}
	`
	var q2 = `
	{
		q(func: has(name)) {
			name
		}
	}
	`
	var m1 = `
	{
		"set": [
			{
				"name": "Alice"
			},
			{
				"name": "Bob"
			}
		]
	}
	`
	var m2 = `
	{
		"delete": [
			{
				"uid": "%s",
				"name": null
			}
		]
	}
	`
	var s1 = `
            name: string @index(exact) .
	`

	require.NoError(t, dropAll())
	schema.ParseBytes([]byte(""), 1)
	err := alterSchemaWithRetry(s1)
	require.NoError(t, err)

	err = runJsonMutation(m1)
	require.NoError(t, err)

	output, err := runQuery(q1)
	q1Result := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(output), &q1Result))
	queryResults := q1Result["data"].(map[string]interface{})["q"].([]interface{})
	require.Equal(t, 2, len(queryResults))

	var uid string
	count := 0
	for i := 0; i < 2; i++ {
		name := queryResults[i].(map[string]interface{})["name"].(string)
		if name == "Alice" {
			uid = queryResults[i].(map[string]interface{})["uid"].(string)
			count++
		} else {
			require.Equal(t, "Bob", name)
		}
	}
	require.Equal(t, 1, count)

	err = runJsonMutation(fmt.Sprintf(m2, uid))
	require.NoError(t, err)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"q":[{"name":"Bob"}]}}`, output)
}

func TestJsonMutationNumberParsing(t *testing.T) {
	var q1 = `
	{
		q(func: has(n1)) {
			n1
			n2
		}
	}
	`
	var m1 = `
	{
		"set": [
			{
				"n1": 9007199254740995,
				"n2": 9007199254740995.0
			}
		]
	}
	`

	require.NoError(t, dropAll())
	schema.ParseBytes([]byte(""), 1)
	err := runJsonMutation(m1)
	require.NoError(t, err)

	output, err := runQuery(q1)
	var q1Result struct {
		Data struct {
			Q []map[string]interface{} `json:"q"`
		} `json:"data"`
	}
	buffer := bytes.NewBuffer([]byte(output))
	dec := json.NewDecoder(buffer)
	dec.UseNumber()
	require.NoError(t, dec.Decode(&q1Result))
	require.Equal(t, 1, len(q1Result.Data.Q))

	n1, ok := q1Result.Data.Q[0]["n1"]
	require.True(t, ok)
	switch n1.(type) {
	case json.Number:
		n := n1.(json.Number)
		require.True(t, strings.Index(n.String(), ".") < 0)
		i, err := n.Int64()
		require.NoError(t, err)
		require.Equal(t, int64(9007199254740995), i)
	default:
		require.Fail(t, fmt.Sprintf("expected n1 of type int64, got %v (type %T)", n1, n1))
	}

	n2, ok := q1Result.Data.Q[0]["n2"]
	require.True(t, ok)
	switch n2.(type) {
	case json.Number:
		n := n2.(json.Number)
		require.True(t, strings.Index(n.String(), ".") >= 0)
		f, err := n.Float64()
		require.NoError(t, err)
		require.Equal(t, 9007199254740995.0, f)
	default:
		require.Fail(t, fmt.Sprintf("expected n2 of type float64, got %v (type %T)", n2, n2))
	}
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
	{
		delete{
			<0x1> <friend> * .
			<0x1> <name> * .
		}
	}
	`
	var m1 = `
	{
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
      		friend:uid @reverse .
		name: string @index(term) .
	`
	schema.ParseBytes([]byte(""), 1)
	err := alterSchemaWithRetry(s1)
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
	require.JSONEq(t, `{"data": {"user": []}}`, output)

	output, err = runQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user": []}}`, output)
}

func TestDeleteAllSP1(t *testing.T) {
	var m = `
	{
		delete{
			<2000> * * .
		}
	}`
	time.Sleep(20 * time.Millisecond)
	err := runMutation(m)
	require.NoError(t, err)
}

var m5 = `
	{
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
	_, err := gql.Parse(gql.Request{Str: m5})
	require.Error(t, err)
	output, err := runQuery(strings.Replace(q5, "<id>", "0x8", -1))
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user": []}}`, output)
}

var m6 = `
	{
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

//func TestSchemaConversion(t *testing.T) {
//	res, err := gql.Parse(gql.Request{Str: m6, Http: true})
//	require.NoError(t, err)
//
//	var l query.Latency
//	qr := query.QueryRequest{Latency: &l, GqlQuery: &res}
//	_, err = qr.ProcessWithMutation(defaultContext())
//
//	require.NoError(t, err)
//	output := processToFastJSON(strings.Replace(q6, "<id>", "0x6", -1))
//	require.JSONEq(t, `{"data": {"user":[{"name2":1}]}}`, output)
//
//	s, ok := schema.State().Get("name2")
//	require.True(t, ok)
//	s.ValueType = uint32(types.FloatID)
//	schema.State().Set("name2", s)
//	output = processToFastJSON(strings.Replace(q6, "<id>", "0x6", -1))
//	require.JSONEq(t, `{"data": {"user":[{"name2":1.5}]}}`, output)
//}

var qErr = `
 	{
 		set {
 			<0x0> <name> "Alice" .
 		}
 	}
 `

func TestMutationError(t *testing.T) {
	err := runMutation(qErr)
	require.Error(t, err)
}

var qm = `
	{
		set {
			<0x0a> <pred.rel> _:x .
			_:x <pred.val> "value" .
			_:x <pred.rel> _:y .
			_:y <pred.val> "value2" .
		}
	}
`

//func TestAssignUid(t *testing.T) {
//	res, err := gql.Parse(gql.Request{Str: qm, Http: true})
//	require.NoError(t, err)
//
//	var l query.Latency
//	qr := query.QueryRequest{Latency: &l, GqlQuery: &res}
//	er, err := qr.ProcessWithMutation(defaultContext())
//	require.NoError(t, err)
//
//	require.EqualValues(t, len(er.Allocations), 2, "Expected two UIDs to be allocated")
//	_, ok := er.Allocations["x"]
//	require.True(t, ok)
//	_, ok = er.Allocations["y"]
//	require.True(t, ok)
//}

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

// TODO: This might not work. Fix it later, if needed.
func BenchmarkQuery(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processToFastJSON(q1)
	}
}

func TestListPred(t *testing.T) {
	require.NoError(t, alterSchema(`{"drop_all": true}`))
	var q1 = `
	{
		listpred(func:anyofterms(name, "Alice")) {
				_predicate_
		}
	}
	`
	var m = `
	{
		set {
			<0x1> <name> "Alice" .
			<0x1> <age> "13" .
			<0x1> <friend> <0x4> .
		}
	}
	`
	var s = `
			name:string @index(term) .
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = alterSchemaWithRetry(s)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"listpred":[{"_predicate_":["name","age","friend"]}]}}`,
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
	{
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
			name:string @index(term) .
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = alterSchemaWithRetry(s)
	require.NoError(t, err)

	_, err = runQuery(q1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Repeated subgraph")
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
	{
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
			name:string @index(term) .
	`
	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = alterSchemaWithRetry(s)
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

// change from uid to scalar or vice versa
func TestSchemaMutation4Error(t *testing.T) {
	var m = `
            age:int .
	`
	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := alterSchemaWithRetry(m)
	require.NoError(t, err)

	m = `
	{
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
	err = alterSchema(m)
	require.Error(t, err)
}

// change from uid to scalar or vice versa
func TestSchemaMutation5Error(t *testing.T) {
	var m = `
            friends:uid .
	`
	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := alterSchemaWithRetry(m)
	require.NoError(t, err)

	m = `
	{
		set {
			<0x8> <friends> <0x5> .
		}
	}
	`
	err = runMutation(m)
	require.NoError(t, err)

	m = `
            friends:string .
	`
	err = alterSchema(m)
	require.Error(t, err)
}

// A basic sanity check. We will do more extensive testing for multiple values in query.
func TestMultipleValues(t *testing.T) {
	schema.ParseBytes([]byte(""), 1)
	m := `
			occupations: [string] .
`
	err := alterSchemaWithRetry(m)
	require.NoError(t, err)

	m = `
		{
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
}

func TestListTypeSchemaChange(t *testing.T) {
	require.NoError(t, dropAll())
	schema.ParseBytes([]byte(""), 1)
	m := `
			occupations: [string] @index(term) .
	`

	err := alterSchemaWithRetry(m)
	require.NoError(t, err)

	m = `
		{
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

	q = `{
			me(func: anyofterms(occupations, "Engineer")) {
				occupations
			}
	}`

	res, err = runQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"occupations":["Software Engineer","Pianist"]}]}}`, res)

	q = `{
			me(func: allofterms(occupations, "Software Engineer")) {
				occupations
			}
	}`

	res, err = runQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"occupations":["Software Engineer","Pianist"]}]}}`, res)

	m = `
				occupations: string .
	`

	// Cant change from list-type to non-list till we have data.
	err = alterSchema(m)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Schema change not allowed from [string] => string")

	err = deletePredicate("occupations")
	require.NoError(t, err)

	require.NoError(t, alterSchemaWithRetry(m))

	q = `schema{}`
	res, err = runQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"schema":[{"predicate":"_predicate_","type":"string","list":true},{"predicate":"occupations","type":"string"}]}}`, res)

}

func TestDeleteAllSP2(t *testing.T) {
	var m = `
	{
	  set {
	    <0x12345> <nodeType> "TRACKED_DAY" .
	    <0x12345> <name> "July 3 2017" .
	    <0x12345> <date> "2017-07-03T03:49:03+00:00" .
	    <0x12345> <weight> "262.3" .
	    <0x12345> <weightUnit> "pound" .
	    <0x12345> <lifeLoad> "5" .
	    <0x12345> <stressLevel> "3" .
	    <0x12345> <plan> "modest day" .
	    <0x12345> <postMortem> "win!" .
	  }
	}
	`
	err := runMutation(m)
	require.NoError(t, err)

	q := fmt.Sprintf(`
	{
	  me(func: uid(%s)) {
		_predicate_
		name
	    date
	    weight
	    lifeLoad
	    stressLevel
	  }
	}`, "0x12345")

	output, err := runQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"_predicate_":["name","date","weightUnit","postMortem","lifeLoad","weight","stressLevel","nodeType","plan"],"name":"July 3 2017","date":"2017-07-03T03:49:03+00:00","weight":"262.3","lifeLoad":"5","stressLevel":"3"}]}}`, output)

	m = fmt.Sprintf(`
		{
			delete {
				<%s> * * .
			}
		}`, "0x12345")

	err = runMutation(m)
	require.NoError(t, err)

	output, err = runQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[]}}`, output)
}

func TestDropAll(t *testing.T) {
	var m1 = `
	{
		set{
			_:foo <name> "Foo" .
		}
	}`
	var q1 = `
	{
		q(func: allofterms(name, "Foo")) {
			uid
			name
		}
	}`

	s := `name: string @index(term) .`
	err := alterSchemaWithRetry(s)
	require.NoError(t, err)

	err = runMutation(m1)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	q1Result := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(output), &q1Result))
	queryResults := q1Result["data"].(map[string]interface{})["q"].([]interface{})
	name := queryResults[0].(map[string]interface{})["name"].(string)
	require.Equal(t, "Foo", name)

	err = dropAll()
	require.NoError(t, err)

	q3 := "schema{}"
	output, err = runQuery(q3)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"data":{"schema":[{"predicate":"_predicate_","type":"string","list":true}]}}`, output)

	// Reinstate schema so that we can re-run the original query.
	err = alterSchemaWithRetry(s)
	require.NoError(t, err)

	q5 := `
	{
		q(func: allofterms(name, "Foo")) {
			uid
			name
		}
	}`
	output, err = runQuery(q5)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"q":[]}}`, output)
}

func TestRecurseExpandAll(t *testing.T) {
	var q1 = `
	{
		me(func:anyofterms(name, "Alica")) @recurse {
  			expand(_all_)
		}
	}
	`
	var m = `
	{
		set {
			<0x1> <name> "Alica" .
			<0x1> <age> "13" .
			<0x1> <friend> <0x4> .
			<0x4> <name> "bob" .
			<0x4> <age> "12" .
		}
	}
	`

	var s = `name:string @index(term) .`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	err = alterSchemaWithRetry(s)
	require.NoError(t, err)

	output, err := runQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"name":"Alica","age":"13","friend":[{"name":"bob","age":"12"}]}]}}`, output)
}

func TestIllegalCountInQueryFn(t *testing.T) {
	s := `friend: uid @count .`
	require.NoError(t, alterSchemaWithRetry(s))

	q := `
	{
		q(func: eq(count(friend), 0)) {
			count
		}
	}`
	_, err := runQuery(q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "count")
	require.Contains(t, err.Error(), "zero")
}

func TestMain(m *testing.M) {
	// Increment lease, so that mutations work.
	conn, err := grpc.Dial("localhost:5080", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	zc := intern.NewZeroClient(conn)
	if _, err := zc.AssignUids(context.Background(), &intern.Num{Val: 1e6}); err != nil {
		log.Fatal(err)
	}

	r := m.Run()
	os.Exit(r)
}
