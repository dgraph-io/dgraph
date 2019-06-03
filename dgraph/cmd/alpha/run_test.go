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

package alpha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
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

func (c *raftServer) JoinCluster(ctx context.Context, in *pb.RaftContext) (*api.Payload, error) {
	return &api.Payload{}, nil
}

func childAttrs(sg *query.SubGraph) []string {
	var out []string
	for _, c := range sg.Children {
		out = append(out, c.Attr)
	}
	return out
}

type defaultContextKey int

const (
	mutationAllowedKey defaultContextKey = iota
)

func defaultContext() context.Context {
	return context.WithValue(context.Background(), mutationAllowedKey, true)
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
	qr := query.Request{Latency: &l, GqlQuery: &res, ReadTs: timestamp()}
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

func runGraphqlQuery(q string) (string, error) {
	output, _, err := queryWithTs(q, "application/graphqlpm", 0)
	return string(output), err
}

func runJSONQuery(q string) (string, error) {
	output, _, err := queryWithTs(q, "application/json", 0)
	return string(output), err
}

func runMutation(m string) error {
	_, _, _, err := mutationWithTs(m, "application/rdf", false, true, false, 0)
	return err
}

func runJSONMutation(m string) error {
	_, _, _, err := mutationWithTs(m, "application/json", true, true, false, 0)
	return err
}

func alterSchema(s string) error {
	_, _, err := runWithRetries("PUT", "", addr+"/alter", s)
	if err != nil {
		return fmt.Errorf("error while running request with retries: %v", err)
	}
	return nil
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
	_, _, err := runWithRetries("PUT", "", addr+"/alter", op)
	return err
}

func deletePredicate(pred string) error {
	op := `{"drop_attr": "` + pred + `"}`
	_, _, err := runWithRetries("PUT", "", addr+"/alter", op)
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
				age
				friend {
					name
				}
			}
		}
	`

	var s1 = `
	friend: [uid] @reverse .
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

	output, err := runGraphqlQuery(q1)
	require.NoError(t, err)
	var m map[string]interface{}
	err = json.Unmarshal([]byte(output), &m)
	require.NoError(t, err)
	friends := m["data"].(map[string]interface{})["user"].([]interface{})[0].(map[string]interface{})["friend"].([]interface{})
	require.Equal(t, 2, len(friends))

	output, err = runGraphqlQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"name":"Alice"},{"name":"Alice1"},{"name":"Alice2"}]}}`,
		output)

	output, err = runGraphqlQuery(q3)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"age": "13", "~friend" : [{"name":"Alice"}]}]}}`, output)

	err = deletePredicate("friend")
	require.NoError(t, err)
	err = deletePredicate("salary")
	require.NoError(t, err)

	output, err = runGraphqlQuery(`schema{}`)
	require.NoError(t, err)
	z.CompareJSON(t, `{"data":{"schema":[`+
		`{"predicate":"age","type":"default"},`+
		`{"predicate":"name","type":"string","index":true, "tokenizer":["term"]},`+
		x.AclPredicates+","+
		`{"predicate":"dgraph.type","type":"string","index":true, "tokenizer":["exact"],
			"list":true}]}}`, output)

	output, err = runGraphqlQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user": []}}`, output)

	output, err = runGraphqlQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user": [{"name":"Alice"},{"name":"Alice1"},{"name":"Alice2"}]}}`, output)

	output, err = runGraphqlQuery(q4)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"age": "13"}]}}`, output)

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
			name: string @index(term, exact) .
			alias: string @index(exact, term) .
			dob: dateTime @index(year) .
			film.film.initial_release_date: dateTime @index(year) .
			loc: geo @index(geo) .
			genre: [uid] @reverse .
			survival_rate : float .
			alive         : bool .
			age           : int .
			shadow_deep   : int .
			friend: [uid] @reverse .
			geometry: geo @index(geo) . `

	expected := S{
		Predicate: "name",
		Type:      "string",
		Index:     true,
		Tokenizer: []string{"term", "exact"},
	}

	err := alterSchemaWithRetry(m)
	require.NoError(t, err)

	output, err := runGraphqlQuery("schema {}")
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

	output, err := runGraphqlQuery("schema {}")
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
            age: uid @index .
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

func TestMutationSingleUid(t *testing.T) {
	// reset Schema
	require.NoError(t, schema.ParseBytes([]byte(""), 1))

	var s = `
            friend: uid .
	`
	require.NoError(t, alterSchema(s))

	var m = `
	{
		set {
			<0x1> <friend> <0x2> .
			<0x1> <friend> <0x3> .
		}
	}
	`
	require.Error(t, runMutation(m))
}

// Verify multiple uids are allowed after mutation.
func TestSchemaMutationUid(t *testing.T) {
	// reset Schema
	require.NoError(t, schema.ParseBytes([]byte(""), 1))

	var s1 = `
            friend: uid .
	`
	require.NoError(t, alterSchema(s1))
	var m1 = `
	{
		set {
			<0x1> <friend> <0x2> .
			<0x1> <friend> <0x3> .
		}
	}
	`
	require.Error(t, runMutation(m1))

	var s2 = `
            friend: [uid] .
	`
	require.NoError(t, alterSchema(s2))
	var m2 = `
	{
		set {
			<0x1> <friend> <0x2> .
			<0x1> <friend> <0x3> .
		}
	}
	`
	require.NoError(t, runMutation(m2))
}

// Verify a list uid predicate cannot be converted to a single-element predicate.
func TestSchemaMutationUidError1(t *testing.T) {
	// reset Schema
	require.NoError(t, schema.ParseBytes([]byte(""), 1))

	var s1 = `
            friend: [uid] .
	`
	require.NoError(t, alterSchemaWithRetry(s1))

	var s2 = `
            friend: uid .
	`
	require.Error(t, alterSchemaWithRetry(s2))
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

	output, err := runGraphqlQuery(q1)
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

	output, err := runGraphqlQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"name":"Alice"}]}}`, output)

	// remove index
	err = alterSchemaWithRetry(s2)
	require.NoError(t, err)

	_, err = runGraphqlQuery(q1)
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

	var s = `friend: [uid] @reverse .`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = alterSchemaWithRetry(s)
	require.NoError(t, err)

	output, err := runGraphqlQuery(q1)
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
            friend: [uid] @reverse .
	`

	var s2 = `
            friend: [uid] .
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add reverse edge to name
	err = alterSchemaWithRetry(s1)
	require.NoError(t, err)

	output, err := runGraphqlQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"~friend" : [{"name":"Alice"}]}]}}`, output)

	// remove reverse edge
	err = alterSchemaWithRetry(s2)
	require.NoError(t, err)

	_, err = runGraphqlQuery(q1)
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
			friend: [uid] @count .
	`

	// reset Schema
	schema.ParseBytes([]byte(""), 1)
	err := runMutation(m)
	require.NoError(t, err)

	// add index to name
	err = alterSchemaWithRetry(s)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	output, err := runGraphqlQuery(q1)
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

	err = runJSONMutation(m1)
	require.NoError(t, err)

	output, err := runGraphqlQuery(q1)
	require.NoError(t, err)
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

	err = runJSONMutation(fmt.Sprintf(m2, uid))
	require.NoError(t, err)

	output, err = runGraphqlQuery(q2)
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
	err := runJSONMutation(m1)
	require.NoError(t, err)

	output, err := runGraphqlQuery(q1)
	require.NoError(t, err)
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
      		friend: [uid] @reverse .
		name: string @index(term) .
	`
	schema.ParseBytes([]byte(""), 1)
	err := alterSchemaWithRetry(s1)
	require.NoError(t, err)

	err = runMutation(m1)
	require.NoError(t, err)

	output, err := runGraphqlQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"~friend" : [{"name":"Alice"}]}]}}`, output)

	output, err = runGraphqlQuery(q2)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user":[{"friend":[{"name":"Alice1"},{"name":"Alice2"}]}]}}`,
		output)

	err = runMutation(m2)
	require.NoError(t, err)

	output, err = runGraphqlQuery(q1)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"user": []}}`, output)

	output, err = runGraphqlQuery(q2)
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
	output, err := runGraphqlQuery(strings.Replace(q5, "<id>", "0x8", -1))
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

func TestMutationError(t *testing.T) {
	var qErr = `
 	{
 		set {
 			<0x0> <name> "Alice" .
 		}
 	}
 `
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
            age: uid .
		}
	}
	`
	err = alterSchema(m)
	require.Error(t, err)
}

// change from uid to scalar or vice versa
func TestSchemaMutation5Error(t *testing.T) {
	var m = `
            friends: [uid] .
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
            friends: string .
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
	res, err := runGraphqlQuery(q)
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
	res, err := runGraphqlQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"occupations":["Software Engineer","Pianist"]}]}}`, res)

	q = `{
			me(func: anyofterms(occupations, "Engineer")) {
				occupations
			}
	}`

	res, err = runGraphqlQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"occupations":["Software Engineer","Pianist"]}]}}`, res)

	q = `{
			me(func: allofterms(occupations, "Software Engineer")) {
				occupations
			}
	}`

	res, err = runGraphqlQuery(q)
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
	res, err = runGraphqlQuery(q)
	require.NoError(t, err)
	z.CompareJSON(t, `{"data":{"schema":[`+
		x.AclPredicates+","+
		`{"predicate":"occupations","type":"string"},`+
		`{"predicate":"dgraph.type", "type":"string", "index":true, "tokenizer": ["exact"],
			"list":true}]}}`, res)
}

func TestDeleteAllSP2(t *testing.T) {
	s := `
	type Node12345 {
		nodeType: string
		name: string
		date: dateTime
		weight: float
		weightUnit: string
		lifeLoad: int
		stressLevel: int
		plan: string
		postMortem: string
	}
	`
	require.NoError(t, dropAll())
	schema.ParseBytes([]byte(""), 1)
	err := alterSchemaWithRetry(s)
	require.NoError(t, err)

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
	    <0x12345> <dgraph.type> "Node12345" .
	  }
	}
	`
	err = runMutation(m)
	require.NoError(t, err)

	q := fmt.Sprintf(`
	{
	  me(func: uid(%s)) {
		name
	    date
	    weight
	    lifeLoad
	    stressLevel
	  }
	}`, "0x12345")

	output, err := runGraphqlQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"name":"July 3 2017","date":"2017-07-03T03:49:03+00:00","weight":"262.3","lifeLoad":"5","stressLevel":"3"}]}}`, output)

	m = fmt.Sprintf(`
		{
			delete {
				<%s> * * .
			}
		}`, "0x12345")

	err = runMutation(m)
	require.NoError(t, err)

	output, err = runGraphqlQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[]}}`, output)
}

func TestDeleteScalarValue(t *testing.T) {
	var s = `name: string .`
	require.NoError(t, schema.ParseBytes([]byte(""), 1))
	require.NoError(t, alterSchemaWithRetry(s))

	var m = `
	{
	  set {
	    <0x12345> <name> "xxx" .
	  }
	}
	`
	err := runMutation(m)
	require.NoError(t, err)

	// This test has been flaky at the step that verifies whether the triple exists
	// after the first deletion. To try to combat that, verify the triple can be
	// queried before performing the deletion.
	q := `
	{
	  me(func: uid(0x12345)) {
		name
	  }
	}`
	for i := 0; i < 5; i++ {
		output, err := runGraphqlQuery(q)
		if err != nil || !assert.JSONEq(t, output, `{"data": {"me":[{"name":"xxx"}]}}`) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}

	var d1 = `
    {
      delete {
        <0x12345> <name> "yyy" .
      }
    }
	`
	err = runMutation(d1)
	require.NoError(t, err)

	// Verify triple was not deleted because the value in the request did
	// not match the existing value.
	output, err := runGraphqlQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"name":"xxx"}]}}`, output)

	var d2 = `
	{
      delete {
        <0x12345> <name> "xxx" .
      }
    }
	`
	err = runMutation(d2)
	require.NoError(t, err)

	// Verify triple was actually deleted this time.
	output, err = runGraphqlQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[]}}`, output)
}

func TestDeleteValueLang(t *testing.T) {
	var s = `name: string @lang .`
	require.NoError(t, schema.ParseBytes([]byte(""), 1))
	require.NoError(t, alterSchemaWithRetry(s))

	var m = `
	{
	  set {
	    <0x12345> <name> "Mark"@en .
	    <0x12345> <name> "Marco"@es .
	    <0x12345> <name> "Marc"@fr .
	  }
	}
	`
	err := runMutation(m)
	require.NoError(t, err)

	q := `
	{
	  me(func: uid(0x12345)) {
		name@*
	  }
	}`
	output, err := runGraphqlQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[
		{"name@en":"Mark", "name@es":"Marco", "name@fr":"Marc"}]}}`, output)

	var d1 = `
    {
      delete {
        <0x12345> <name@fr> * .
      }
    }
	`
	err = runMutation(d1)
	require.NoError(t, err)

	// Verify only the specific tagged value was deleted.
	output, err = runGraphqlQuery(q)
	require.NoError(t, err)
	require.JSONEq(t, output, `{"data": {"me":[{"name@en":"Mark", "name@es":"Marco"}]}}`)
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

	output, err := runGraphqlQuery(q1)
	require.NoError(t, err)
	q1Result := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(output), &q1Result))
	queryResults := q1Result["data"].(map[string]interface{})["q"].([]interface{})
	name := queryResults[0].(map[string]interface{})["name"].(string)
	require.Equal(t, "Foo", name)

	err = dropAll()
	require.NoError(t, err)

	q3 := "schema{}"
	output, err = runGraphqlQuery(q3)
	require.NoError(t, err)
	z.CompareJSON(t,
		`{"data":{"schema":[`+
			x.AclPredicates+","+
			`{"predicate":"dgraph.type", "type":"string", "index":true, "tokenizer":["exact"],
				"list":true}]}}`, output)

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
	output, err = runGraphqlQuery(q5)
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"q":[]}}`, output)
}

func TestIllegalCountInQueryFn(t *testing.T) {
	s := `friend: [uid] @count .`
	require.NoError(t, alterSchemaWithRetry(s))

	q := `
	{
		q(func: eq(count(friend), 0)) {
			count
		}
	}`
	_, err := runGraphqlQuery(q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "count")
	require.Contains(t, err.Error(), "zero")
}

// This test is from Github issue #2662.
// This test couldn't like in query package because that package tries to do some extra JSON
// marshal, which causes issues for this case.
func TestJsonUnicode(t *testing.T) {
	err := runJSONMutation(`{
  "set": [
  { "uid": "0x10", "log.message": "\u001b[32mHello World 1!\u001b[39m\n" }
  ]
}`)
	require.NoError(t, err)

	output, err := runGraphqlQuery(`{ node(func: uid(0x10)) { log.message }}`)
	require.NoError(t, err)
	require.Equal(t,
		`{"data":{"node":[{"log.message":"\u001b[32mHello World 1!\u001b[39m\n"}]}}`, output)
}

func TestGrpcCompressionSupport(t *testing.T) {
	conn, err := grpc.Dial(z.SockAddr,
		grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
	)
	defer conn.Close()
	require.NoError(t, err)

	dc := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	q := `schema {}`
	tx := dc.NewTxn()
	_, err = tx.Query(context.Background(), q)
	require.NoError(t, err)
}

func TestTypeMutationAndQuery(t *testing.T) {
	var m = `
	{
		"set": [
			{
				"name": "Alice",
				"dgraph.type": "Employee"
			},
			{
				"name": "Bob",
				"dgraph.type": "Employer"
			}
		]
	}
	`

	var q = `
	{
		q(func: has(name)) @filter(type(Employee)){
			uid
			name
		}
	}
	`

	var s = `
            name: string @index(exact) .
	`

	require.NoError(t, dropAll())
	err := alterSchemaWithRetry(s)
	require.NoError(t, err)

	err = runJSONMutation(m)
	require.NoError(t, err)

	output, err := runGraphqlQuery(q)
	require.NoError(t, err)
	result := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(output), &result))
	queryResults := result["data"].(map[string]interface{})["q"].([]interface{})
	require.Equal(t, 1, len(queryResults))
	name := queryResults[0].(map[string]interface{})["name"].(string)
	require.Equal(t, "Alice", name)
}

func TestIPStringParsing(t *testing.T) {
	var addrRange []x.IPRange
	var err error

	addrRange, err = getIPsFromString("144.142.126.222:144.142.126.244")
	require.NoError(t, err)
	require.Equal(t, net.IPv4(144, 142, 126, 222), addrRange[0].Lower)
	require.Equal(t, net.IPv4(144, 142, 126, 244), addrRange[0].Upper)

	addrRange, err = getIPsFromString("144.142.126.254")
	require.NoError(t, err)
	require.Equal(t, net.IPv4(144, 142, 126, 254), addrRange[0].Lower)
	require.Equal(t, net.IPv4(144, 142, 126, 254), addrRange[0].Upper)

	addrRange, err = getIPsFromString("192.168.0.0/16")
	require.NoError(t, err)
	require.Equal(t, net.IPv4(192, 168, 0, 0), addrRange[0].Lower)
	require.Equal(t, net.IPv4(192, 168, 255, 255), addrRange[0].Upper)

	addrRange, err = getIPsFromString("example.org")
	require.NoError(t, err)
	require.NotEqual(t, net.IPv4zero, addrRange[0].Lower)

	addrRange, err = getIPsFromString("144.142.126.222:144.142.126.244,144.142.126.254" +
		",192.168.0.0/16,example.org")
	require.NoError(t, err)
	require.NotEqual(t, 0, len(addrRange))

	addrRange, err = getIPsFromString("fd03:b188:0f3c:9ec4::babe:face")
	require.NoError(t, err)
	require.NotEqual(t, net.IPv6zero, addrRange[0].Lower)
	require.Equal(t, addrRange[0].Lower, addrRange[0].Upper)

	addrRange, err = getIPsFromString("fd03:b188:0f3c:9ec4::/64")
	require.NoError(t, err)
	require.NotEqual(t, net.IPv6zero, addrRange[0].Lower)
	require.NotEqual(t, addrRange[0].Lower, addrRange[0].Upper)
}

func TestJSONQueryWithVariables(t *testing.T) {
	schema.ParseBytes([]byte(""), 1)
	m := `
			user_id: string @index(exact) @upsert .
			user_name: string @index(hash) .
			follows: [uid] @reverse .
`
	err := alterSchemaWithRetry(m)
	require.NoError(t, err)

	m = `
		{
			set {
				<0x1400> <user_id> "user1" .
				<0x1400> <user_name> "first user" .
				<0x1401> <user_id> "user2" .
				<0x1401> <user_name> "second user" .
				<0x1400> <follows> <0x1401> .
				<0x1402> <user_id> "user3" .
				<0x1402> <user_name> "third user" .
				<0x1401> <follows> <0x1402> .
				<0x1403> <user_id> "user4" .
				<0x1403> <user_name> "fourth user" .
				<0x1401> <follows> <0x1403> .
			}
		}
	`

	err = runMutation(m)
	require.NoError(t, err)

	q1 := `query all($userID: string) {
		q(func: eq(user_id, $userID)) {
			user_id
			user_name
		}
	}`
	p1 := params{
		Query: q1,
		Variables: map[string]string{
			"$userID": "user1",
		},
	}
	data, err := json.Marshal(p1)
	require.NoError(t, err)
	res, err := runJSONQuery(string(data))
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"q":[{"user_id":"user1","user_name":"first user"}]}}`, res)

	q2 := `query all($userID: string, $userName: string) {
		q(func: eq(user_id, $userID)) {
			user_id
			user_name
			follows @filter(eq(user_name, $userName)) {
				uid
				user_id
			}
		}
	}`
	p2 := params{
		Query: q2,
		Variables: map[string]string{
			"$userID":   "user2",
			"$userName": "fourth user",
		},
	}
	data, err = json.Marshal(p2)
	require.NoError(t, err)
	res, err = runJSONQuery(string(data))
	require.NoError(t, err)
	exp := `{"data":{"q":[{"user_id":"user2","user_name":"second user",` +
		`"follows":[{"uid":"0x1403","user_id":"user4"}]}]}}`
	require.JSONEq(t, exp, res)
}

var addr = "http://localhost:8180"

// the grootAccessJWT stores the access JWT extracted from the response
// of http login
var grootAccessJwt string
var grootRefreshJwt string

func TestMain(m *testing.M) {
	// Increment lease, so that mutations work.
	conn, err := grpc.Dial(z.SockAddrZero, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	zc := pb.NewZeroClient(conn)
	if _, err := zc.AssignUids(context.Background(), &pb.Num{Val: 1e6}); err != nil {
		log.Fatal(err)
	}
	grootAccessJwt, grootRefreshJwt = z.GrootHttpLogin(addr + "/login")

	r := m.Run()
	os.Exit(r)
}
