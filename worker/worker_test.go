/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
)

var raftIndex uint64
var ts uint64

func timestamp() uint64 {
	return atomic.AddUint64(&ts, 1)
}

func addEdge(t *testing.T, edge *pb.DirectedEdge, l *posting.List) {
	edge.Op = pb.DirectedEdge_SET
	commitTransaction(t, edge, l)
}

func delEdge(t *testing.T, edge *pb.DirectedEdge, l *posting.List) {
	edge.Op = pb.DirectedEdge_DEL
	commitTransaction(t, edge, l)
}

func setClusterEdge(t *testing.T, dg *dgo.Dgraph, rdf string) {
	mu := &api.Mutation{SetNquads: []byte(rdf), CommitNow: true}
	err := z.RetryMutation(dg, mu)
	require.NoError(t, err)
}

func delClusterEdge(t *testing.T, dg *dgo.Dgraph, rdf string) {
	mu := &api.Mutation{DelNquads: []byte(rdf), CommitNow: true}
	err := z.RetryMutation(dg, mu)
	require.NoError(t, err)
}
func getOrCreate(key []byte) *posting.List {
	l, err := posting.GetNoStore(key)
	x.Checkf(err, "While calling posting.Get")
	return l
}

func populateGraph(t *testing.T) {
	// Add uid edges : predicate neightbour.
	edge := &pb.DirectedEdge{
		ValueId: 23,
		Label:   "author0",
		Attr:    "neighbour",
	}
	edge.Entity = 10
	addEdge(t, edge, getOrCreate(x.DataKey("neighbour", 10)))

	edge.Entity = 11
	addEdge(t, edge, getOrCreate(x.DataKey("neighbour", 11)))

	edge.Entity = 12
	addEdge(t, edge, getOrCreate(x.DataKey("neighbour", 12)))

	edge.ValueId = 25
	addEdge(t, edge, getOrCreate(x.DataKey("neighbour", 12)))

	edge.ValueId = 26
	addEdge(t, edge, getOrCreate(x.DataKey("neighbour", 12)))

	edge.Entity = 10
	edge.ValueId = 31
	addEdge(t, edge, getOrCreate(x.DataKey("neighbour", 10)))

	edge.Entity = 12
	addEdge(t, edge, getOrCreate(x.DataKey("neighbour", 12)))

	// add value edges: friend : with name
	edge.Attr = "friend"
	edge.Entity = 12
	edge.Value = []byte("photon")
	edge.ValueId = 0
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 12)))

	edge.Entity = 10
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 10)))
}

func populateClusterGraph(t *testing.T, dg *dgo.Dgraph) {
	data1 := [][]int{{10, 23}, {11, 23}, {12, 23}, {12, 25}, {12, 26}, {10, 31}, {12, 31}}
	for _, pair := range data1 {
		rdf := fmt.Sprintf(`<0x%x> <neighbour> <0x%x> .`, pair[0], pair[1])
		setClusterEdge(t, dg, rdf)
	}

	data2 := map[int]string{12: "photon", 10: "photon"}
	for key, val := range data2 {
		rdf := fmt.Sprintf(`<0x%x> <friend> %q .`, key, val)
		setClusterEdge(t, dg, rdf)
	}
}

func initTest(t *testing.T, schemaStr string) {
	err := schema.ParseBytes([]byte(schemaStr), 1)
	require.NoError(t, err)
	populateGraph(t)
}

func initClusterTest(t *testing.T, schemaStr string) *dgo.Dgraph {
	dg := z.DgraphClient(z.SockAddr)
	z.DropAll(t, dg)

	err := dg.Alter(context.Background(), &api.Operation{Schema: schemaStr})
	require.NoError(t, err)
	populateClusterGraph(t, dg)

	return dg
}

func helpProcessTask(query *pb.Query, gid uint32) (*pb.Result, error) {
	qs := queryState{cache: nil}
	return qs.helpProcessTask(context.Background(), query, gid)
}

func TestProcessTask(t *testing.T) {
	dg := initClusterTest(t, `neighbour: [uid] .`)

	resp, err := runQuery(dg, "neighbour", []uint64{10, 11, 12}, nil)
	require.NoError(t, err)
	require.JSONEq(t, `{
		  "q": [
		    {
		      "neighbour": [
		        { "uid": "0x17" },
		        { "uid": "0x1f" }
		      ]
		    },
		    {
		      "neighbour": [
		        { "uid": "0x17" }
		      ]
		    },
		    {
		      "neighbour": [
		        { "uid": "0x17" },
		        { "uid": "0x19" },
		        { "uid": "0x1a" },
		        { "uid": "0x1f" }
		      ]
		    }
		  ]
		}`,
		string(resp.Json),
	)
}

// newQuery creates a Query task and returns it.
func newQuery(attr string, uids []uint64, srcFunc []string) *pb.Query {
	x.AssertTrue(uids == nil || srcFunc == nil)
	// TODO: Change later, hacky way to make the tests work
	var srcFun *pb.SrcFunction
	if len(srcFunc) > 0 {
		srcFun = new(pb.SrcFunction)
		srcFun.Name = srcFunc[0]
		srcFun.Args = append(srcFun.Args, srcFunc[2:]...)
	}
	q := &pb.Query{
		UidList: &pb.List{Uids: uids},
		SrcFunc: srcFun,
		Attr:    attr,
		ReadTs:  timestamp(),
	}
	// It will have either nothing or attr, lang
	if len(srcFunc) > 0 && srcFunc[1] != "" {
		q.Langs = []string{srcFunc[1]}
	}
	return q
}

func runQuery(dg *dgo.Dgraph, attr string, uids []uint64, srcFunc []string) (*api.Response, error) {
	x.AssertTrue(uids == nil || srcFunc == nil)

	var query string
	if uids != nil {
		var uidv []string
		for _, uid := range uids {
			uidv = append(uidv, fmt.Sprintf("0x%x", uid))
		}
		query = fmt.Sprintf(`
			{
				q(func: uid(%s)) {
					%s { uid }
				}
			}`, strings.Join(uidv, ","), attr,
		)
	} else {
		var langs, args string
		if srcFunc[1] != "" {
			langs = "@" + srcFunc[1]
		}
		args = strings.Join(srcFunc[2:], " ")
		query = fmt.Sprintf(`
			{
				q(func: %s(%s%s, %q)) {
					uid
				}
			}`, srcFunc[0], attr, langs, args)
	}

	resp, err := z.RetryQuery(dg, query)

	return resp, err
}

// Index-related test. Similar to TestProcessTaskIndex but we call MergeLists only
// at the end. In other words, everything is happening only in mutation layers,
// and not committed to BadgerDB until near the end.
func TestProcessTaskIndexMLayer(t *testing.T) {
	dg := initClusterTest(t, `friend:string @index(term) .`)

	resp, err := runQuery(dg, "friend", nil, []string{"anyofterms", "", "hey photon"})
	require.NoError(t, err)
	require.JSONEq(t, `{
		  "q": [
		    { "uid": "0xa" },
		    { "uid": "0xc" }
		  ]
		}`,
		string(resp.Json),
	)

	// Now try changing 12's friend value from "photon" to "notphotonExtra" to
	// "notphoton".
	setClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 12, "notphotonExtra"))
	setClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 12, "notphoton"))

	// Issue a similar query.
	resp, err = runQuery(dg, "friend", nil,
		[]string{"anyofterms", "", "hey photon notphoton notphotonExtra"})
	require.NoError(t, err)
	require.JSONEq(t, `{
		  "q": [
		    { "uid": "0xa" },
		    { "uid": "0xc" }
		  ]
		}`,
		string(resp.Json),
	)

	// Try redundant deletes.
	delClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 10, "photon"))
	delClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 10, "photon"))

	// Delete followed by set.
	delClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 12, "notphoton"))
	setClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 12, "ignored"))

	// Issue a similar query.
	resp, err = runQuery(dg, "friend", nil,
		[]string{"anyofterms", "", "photon notphoton ignored"})
	require.NoError(t, err)
	require.JSONEq(t, `{
		  "q": [
		    { "uid": "0xc" }
		  ]
		}`,
		string(resp.Json),
	)

	resp, err = runQuery(dg, "friend", nil,
		[]string{"anyofterms", "", "photon notphoton ignored"})
	require.NoError(t, err)
	require.JSONEq(t, `{
		  "q": [
		    { "uid": "0xc" }
		  ]
		}`,
		string(resp.Json),
	)
}

// Index-related test. Similar to TestProcessTaskIndeMLayer except we call
// MergeLists in between a lot of updates.
func TestProcessTaskIndex(t *testing.T) {
	dg := initClusterTest(t, `friend:string @index(term) .`)

	resp, err := runQuery(dg, "friend", nil, []string{"anyofterms", "", "hey photon"})
	require.NoError(t, err)
	require.JSONEq(t, `{
		  "q": [
		    { "uid": "0xa" },
		    { "uid": "0xc" }
		  ]
		}`,
		string(resp.Json),
	)

	// Now try changing 12's friend value from "photon" to "notphotonExtra" to
	// "notphoton".
	setClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 12, "notphotonExtra"))
	setClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 12, "notphoton"))

	// Issue a similar query.
	resp, err = runQuery(dg, "friend", nil,
		[]string{"anyofterms", "", "hey photon notphoton notphotonExtra"})
	require.NoError(t, err)
	require.JSONEq(t, `{
		  "q": [
		    { "uid": "0xa" },
		    { "uid": "0xc" }
		  ]
		}`,
		string(resp.Json),
	)

	// Try redundant deletes.
	delClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 10, "photon"))
	delClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 10, "photon"))

	// Delete followed by set.
	delClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 12, "notphoton"))
	setClusterEdge(t, dg, fmt.Sprintf("<0x%x> <friend> %q .", 12, "ignored"))

	// Issue a similar query.
	resp, err = runQuery(dg, "friend", nil,
		[]string{"anyofterms", "", "photon notphoton ignored"})
	require.NoError(t, err)
	require.JSONEq(t, `{
		  "q": [
		    { "uid": "0xc" }
		  ]
		}`,
		string(resp.Json),
	)
}

func TestMain(m *testing.M) {
	x.Init()
	posting.Config.AllottedMemory = 1024.0
	posting.Config.CommitFraction = 0.10
	gr = new(groupi)
	gr.gid = 1
	gr.tablets = make(map[string]*pb.Tablet)
	gr.tablets["name"] = &pb.Tablet{GroupId: 1}
	gr.tablets["name2"] = &pb.Tablet{GroupId: 1}
	gr.tablets["age"] = &pb.Tablet{GroupId: 1}
	gr.tablets["friend"] = &pb.Tablet{GroupId: 1}
	gr.tablets["http://www.w3.org/2000/01/rdf-schema#range"] = &pb.Tablet{GroupId: 1}
	gr.tablets["friend_not_served"] = &pb.Tablet{GroupId: 2}
	gr.tablets[""] = &pb.Tablet{GroupId: 1}

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps)
	Init(ps)

	os.Exit(m.Run())
}
