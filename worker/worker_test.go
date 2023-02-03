/*
 * Copyright 2016-2022 Dgraph Labs, Inc. and Contributors
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
	"math"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

var ts uint64

func commitTs(startTs uint64) uint64 {
	commit := timestamp()
	od := &pb.OracleDelta{
		MaxAssigned: atomic.LoadUint64(&ts),
	}
	od.Txns = append(od.Txns, &pb.TxnStatus{StartTs: startTs, CommitTs: commit})
	posting.Oracle().ProcessDelta(od)
	return commit
}

func commitTransaction(t *testing.T, edge *pb.DirectedEdge, l *posting.List) {
	startTs := timestamp()
	txn := posting.Oracle().RegisterStartTs(startTs)
	l = txn.Store(l)
	err := l.AddMutationWithIndex(context.Background(), edge, txn)
	require.NoError(t, err)

	commit := commitTs(startTs)

	txn.Update()
	writer := posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, commit))
	require.NoError(t, writer.Flush())
}

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
	err := testutil.RetryMutation(dg, mu)
	require.NoError(t, err)
}

func delClusterEdge(t *testing.T, dg *dgo.Dgraph, rdf string) {
	mu := &api.Mutation{DelNquads: []byte(rdf), CommitNow: true}
	err := testutil.RetryMutation(dg, mu)
	require.NoError(t, err)
}
func getOrCreate(key []byte) *posting.List {
	l, err := posting.GetNoStore(key, math.MaxUint64)
	x.Checkf(err, "While calling posting.Get")
	return l
}

func populateGraph(t *testing.T) {
	// Add uid edges : predicate neightbour.
	neighbour := x.GalaxyAttr("neighbour")
	edge := &pb.DirectedEdge{
		ValueId: 23,
		Attr:    neighbour,
	}
	edge.Entity = 10
	addEdge(t, edge, getOrCreate(x.DataKey(neighbour, 10)))

	edge.Entity = 11
	addEdge(t, edge, getOrCreate(x.DataKey(neighbour, 11)))

	edge.Entity = 12
	addEdge(t, edge, getOrCreate(x.DataKey(neighbour, 12)))

	edge.ValueId = 25
	addEdge(t, edge, getOrCreate(x.DataKey(neighbour, 12)))

	edge.ValueId = 26
	addEdge(t, edge, getOrCreate(x.DataKey(neighbour, 12)))

	edge.Entity = 10
	edge.ValueId = 31
	addEdge(t, edge, getOrCreate(x.DataKey(neighbour, 10)))

	edge.Entity = 12
	addEdge(t, edge, getOrCreate(x.DataKey(neighbour, 12)))

	// add value edges: friend : with name
	friend := x.GalaxyAttr("friend")
	edge.Attr = neighbour
	edge.Entity = 12
	edge.Value = []byte("photon")
	edge.ValueId = 0
	addEdge(t, edge, getOrCreate(x.DataKey(friend, 12)))

	edge.Entity = 10
	addEdge(t, edge, getOrCreate(x.DataKey(friend, 10)))
}

func populateClusterGraph(t *testing.T, dg *dgo.Dgraph) {
	data1 := [][]int{{10, 23}, {11, 23}, {12, 23}, {12, 25}, {12, 26}, {10, 31}, {12, 31}}
	for _, pair := range data1 {
		rdf := fmt.Sprintf(`<%#x> <neighbour> <%#x> .`, pair[0], pair[1])
		setClusterEdge(t, dg, rdf)
	}

	data2 := map[int]string{12: "photon", 10: "photon"}
	for key, val := range data2 {
		rdf := fmt.Sprintf(`<%#x> <friend> %q .`, key, val)
		setClusterEdge(t, dg, rdf)
	}
}

func initTest(t *testing.T, schemaStr string) {
	err := schema.ParseBytes([]byte(schemaStr), 1)
	require.NoError(t, err)
	populateGraph(t)
}

func initClusterTest(t *testing.T, schemaStr string) *dgo.Dgraph {
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	testutil.DropAll(t, dg)

	err = dg.Alter(context.Background(), &api.Operation{Schema: schemaStr})
	require.NoError(t, err)
	populateClusterGraph(t, dg)

	return dg
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

func runQuery(dg *dgo.Dgraph, attr string, uids []uint64, srcFunc []string) (*api.Response, error) {
	x.AssertTrue(uids == nil || srcFunc == nil)

	var query string
	if uids != nil {
		var uidv []string
		for _, uid := range uids {
			uidv = append(uidv, fmt.Sprintf("%#x", uid))
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

	resp, err := testutil.RetryQuery(dg, query)

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
	setClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 12, "notphotonExtra"))
	setClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 12, "notphoton"))

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
	delClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 10, "photon"))
	delClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 10, "photon"))

	// Delete followed by set.
	delClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 12, "notphoton"))
	setClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 12, "ignored"))

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
	setClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 12, "notphotonExtra"))
	setClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 12, "notphoton"))

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
	delClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 10, "photon"))
	delClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 10, "photon"))

	// Delete followed by set.
	delClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 12, "notphoton"))
	setClusterEdge(t, dg, fmt.Sprintf("<%#x> <friend> %q .", 12, "ignored"))

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
	posting.Config.CommitFraction = 0.10
	gr = new(groupi)
	gr.gid = 1
	gr.tablets = make(map[string]*pb.Tablet)
	addTablets := func(attrs []string, gid uint32, namespace uint64) {
		for _, attr := range attrs {
			gr.tablets[x.NamespaceAttr(namespace, attr)] = &pb.Tablet{GroupId: gid}
		}
	}

	addTablets([]string{"name", "name2", "age", "http://www.w3.org/2000/01/rdf-schema#range", "",
		"friend", "dgraph.type", "dgraph.graphql.xid", "dgraph.graphql.schema"},
		1, x.GalaxyNamespace)
	addTablets([]string{"friend_not_served"}, 2, x.GalaxyNamespace)
	addTablets([]string{"name"}, 1, 0x2)

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0)
	Init(ps)

	os.Exit(m.Run())
}
