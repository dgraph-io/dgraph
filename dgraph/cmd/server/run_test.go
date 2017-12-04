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

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	context "golang.org/x/net/context"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
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

func prepare() (dir1, dir2 string, rerr error) {
	cmd := exec.Command("go", "install", "github.com/dgraph-io/dgraph/dgraph")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("Could not run %q: %s", cmd.Args, string(out))
	}
	zero := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"zero",
		"-w=wz",
	)
	zero.Stdout = os.Stdout
	zero.Stderr = os.Stdout
	if err := zero.Start(); err != nil {
		return "", "", err
	}

	var err error
	dir1, err = ioutil.TempDir("", "storetest_")
	if err != nil {
		return "", "", err
	}

	dir2, err = ioutil.TempDir("", "wal_")
	if err != nil {
		return dir1, "", err
	}

	edgraph.Config.PostingDir = dir1
	edgraph.Config.PostingTables = "loadtoram"
	edgraph.Config.WALDir = dir2
	edgraph.InitServerState()

	posting.Init(edgraph.State.Pstore)
	schema.Init(edgraph.State.Pstore)
	worker.Init(edgraph.State.Pstore)
	worker.Config.ZeroAddr = "localhost:7080"
	x.Config.PortOffset = 1
	worker.Config.RaftId = 1
	go worker.RunServer(false)
	worker.StartRaftNodes(edgraph.State.WALstore, false)
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
	_, _, err := mutationWithTs(m, true, false, 0)
	return err
}

func alterSchema(s string) error {
	req, err := http.NewRequest("PUT", "/alter", bytes.NewBufferString(s))
	if err != nil {
		return err
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(alterHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		return fmt.Errorf("Unexpected status code: %v", status)
	}
	var qr x.QueryResWithData
	json.Unmarshal(rr.Body.Bytes(), &qr)
	if len(qr.Errors) == 0 {
		return nil
	}
	return errors.New(qr.Errors[0].Message)
}

func alterSchemaWithRetry(s string) error {
	return alterSchema(s)
}

func dropAll() error {
	op := `{"drop_all": true}`
	req, err := http.NewRequest("PUT", "/alter", bytes.NewBufferString(op))
	if err != nil {
		return err
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(alterHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		return fmt.Errorf("Unexpected status code: %v", status)
	}
	var qr x.QueryResWithData
	json.Unmarshal(rr.Body.Bytes(), &qr)
	if len(qr.Errors) == 0 {
		return nil
	}
	return x.Errorf("Got error while trying to drop all", qr.Errors)
}

func deletePredicate(pred string) error {
	op := `{"drop_attr": "` + pred + `"}`
	req, err := http.NewRequest("PUT", "/alter", bytes.NewBufferString(op))
	if err != nil {
		return err
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(alterHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		return fmt.Errorf("Unexpected status code: %v", status)
	}
	var qr x.QueryResWithData
	json.Unmarshal(rr.Body.Bytes(), &qr)
	if len(qr.Errors) == 0 {
		return nil
	}
	return x.Errorf("Got error while trying to delete predicate", qr.Errors)
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
			user(func: uid( 0x3)) {
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

func TestSchemaMutation(t *testing.T) {
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
			geometry:geo @index(geo) .

` // reset schema
	schema.ParseBytes([]byte(""), 1)
	expected := map[string]*intern.SchemaUpdate{
		"name": {
			Predicate: "name",
			Tokenizer: []string{"term", "exact"},
			ValueType: intern.Posting_ValType(types.StringID),
			Directive: intern.SchemaUpdate_INDEX,
			Explicit:  true},
	}

	err := alterSchemaWithRetry(m)
	require.NoError(t, err)
	for k, v := range expected {
		s, ok := schema.State().Get(k)
		require.True(t, ok)
		require.Equal(t, *v, s)
	}
}

func TestSchemaMutation1(t *testing.T) {
	var m = `
	{
		set {
			<0x1234> <pred1> "12345"^^<xs:string> .
			<0x1234> <pred2> "12345" .
		}
	}

` // reset schema
	schema.ParseBytes([]byte(""), 1)
	expected := map[string]*intern.SchemaUpdate{
		"pred1": {
			ValueType: intern.Posting_ValType(types.StringID)},
		"pred2": {
			ValueType: intern.Posting_ValType(types.DefaultID)},
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
	dc := edgraph.DefaultConfig
	dc.AllottedMemory = 2048.0
	edgraph.SetConfiguration(dc)
	x.Init(true)

	dir1, dir2, err := prepare()
	if err != nil {
		log.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	// Increment lease, so that mutations work.
	_, err = worker.AssignUidsOverNetwork(context.Background(), &intern.Num{Val: 10e6})
	if err != nil {
		log.Fatal(err)
	}
	// Parse GQL into intern.query representation.
	r := m.Run()
	closeAll(dir1, dir2)
	exec.Command("killall", "-9", "dgraph").Run()
	os.RemoveAll("wz")
	os.Exit(r)
}
