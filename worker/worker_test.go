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

package worker

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

var raftIndex uint64

func addEdge(t *testing.T, edge *protos.DirectedEdge, l *posting.List) {
	edge.Op = protos.DirectedEdge_SET
	raftIndex++
	rv := x.RaftValue{Group: 1, Index: raftIndex}
	ctx := context.WithValue(context.Background(), "raft", rv)
	require.NoError(t, l.AddMutationWithIndex(ctx, edge))
}

func delEdge(t *testing.T, edge *protos.DirectedEdge, l *posting.List) {
	edge.Op = protos.DirectedEdge_DEL
	raftIndex++
	rv := x.RaftValue{Group: 1, Index: raftIndex}
	ctx := context.WithValue(context.Background(), "raft", rv)
	require.NoError(t, l.AddMutationWithIndex(ctx, edge))
}

func getOrCreate(key []byte) *posting.List {
	l := posting.Get(key, 1)
	return l
}

func populateGraph(t *testing.T) {
	// Add uid edges : predicate neightbour.
	edge := &protos.DirectedEdge{
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

func taskValues(t *testing.T, v []*protos.TaskValue) []string {
	out := make([]string, len(v))
	for i, tv := range v {
		out[i] = string(tv.Val)
	}
	return out
}

func initTest(t *testing.T, schemaStr string) (string, *badger.KV) {
	err := schema.ParseBytes([]byte(schemaStr), 1)
	require.NoError(t, err)

	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)

	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir
	ps, err := badger.NewKV(&opt)
	x.Check(err)
	pstore = ps

	posting.Init(ps)
	populateGraph(t)

	return dir, ps
}

func TestProcessTask(t *testing.T) {
	dir, ps := initTest(t, `neighbour: uid .`)
	defer os.RemoveAll(dir)
	defer ps.Close()

	query := newQuery("neighbour", []uint64{10, 11, 12}, nil)
	r, err := helpProcessTask(context.Background(), query, 1)
	require.NoError(t, err)
	require.EqualValues(t,
		[][]uint64{
			{23, 31},
			{23},
			{23, 25, 26, 31},
		}, algo.ToUintsListForTest(r.UidMatrix))
}

// newQuery creates a Query task and returns it.
func newQuery(attr string, uids []uint64, srcFunc []string) *protos.Query {
	x.AssertTrue(uids == nil || srcFunc == nil)
	// TODO: Change later, hacky way to make the tests work
	var srcFun *protos.SrcFunction
	if len(srcFunc) > 0 {
		srcFun = new(protos.SrcFunction)
		srcFun.Name = srcFunc[0]
		srcFun.Args = append(srcFun.Args, srcFunc[2:]...)
	}
	q := &protos.Query{
		UidList: &protos.List{uids},
		SrcFunc: srcFun,
		Attr:    attr,
	}
	// It will have either nothing or attr, lang
	if len(srcFunc) > 0 && srcFunc[1] != "" {
		q.Langs = []string{srcFunc[1]}
	}
	return q
}

// Index-related test. Similar to TestProcessTaskIndex but we call MergeLists only
// at the end. In other words, everything is happening only in mutation layers,
// and not committed to RocksDB until near the end.
func TestProcessTaskIndexMLayer(t *testing.T) {
	dir, ps := initTest(t, `friend:string @index(term) .`)
	defer os.RemoveAll(dir)
	defer ps.Close()

	query := newQuery("friend", nil, []string{"anyofterms", "", "hey photon"})
	r, err := helpProcessTask(context.Background(), query, 1)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{},
		{10, 12},
	}, algo.ToUintsListForTest(r.UidMatrix))

	// Now try changing 12's friend value from "photon" to "notphotonExtra" to
	// "notphoton".
	edge := &protos.DirectedEdge{
		Value:  []byte("notphotonExtra"),
		Label:  "author0",
		Attr:   "friend",
		Entity: 12,
	}
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 12)))
	edge.Value = []byte("notphoton")
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 12)))

	// Issue a similar query.
	query = newQuery("friend", nil, []string{"anyofterms", "", "hey photon notphoton notphotonExtra"})
	r, err = helpProcessTask(context.Background(), query, 1)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{},
		{12},
		{},
		{10},
	}, algo.ToUintsListForTest(r.UidMatrix))

	// Try deleting.
	edge = &protos.DirectedEdge{
		Value:  []byte("photon"),
		Label:  "author0",
		Attr:   "friend",
		Entity: 10,
	}
	// Redundant deletes.
	delEdge(t, edge, getOrCreate(x.DataKey("friend", 10)))
	delEdge(t, edge, getOrCreate(x.DataKey("friend", 10)))

	// Delete followed by set.
	edge.Entity = 12
	edge.Value = []byte("notphoton")
	delEdge(t, edge, getOrCreate(x.DataKey("friend", 12)))
	edge.Value = []byte("ignored")
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 12)))

	// Issue a similar query.
	query = newQuery("friend", nil, []string{"anyofterms", "", "photon notphoton ignored"})
	r, err = helpProcessTask(context.Background(), query, 1)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{12},
		{},
		{},
	}, algo.ToUintsListForTest(r.UidMatrix))

	query = newQuery("friend", nil, []string{"anyofterms", "", "photon notphoton ignored"})
	r, err = helpProcessTask(context.Background(), query, 1)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{12},
		{},
		{},
	}, algo.ToUintsListForTest(r.UidMatrix))
}

// Index-related test. Similar to TestProcessTaskIndeMLayer except we call
// MergeLists in between a lot of updates.
func TestProcessTaskIndex(t *testing.T) {
	dir, ps := initTest(t, `friend:string @index(term) .`)
	defer os.RemoveAll(dir)
	defer ps.Close()

	query := newQuery("friend", nil, []string{"anyofterms", "", "hey photon"})
	r, err := helpProcessTask(context.Background(), query, 1)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{},
		{10, 12},
	}, algo.ToUintsListForTest(r.UidMatrix))

	// Now try changing 12's friend value from "photon" to "notphotonExtra" to
	// "notphoton".
	edge := &protos.DirectedEdge{
		Value:  []byte("notphotonExtra"),
		Label:  "author0",
		Attr:   "friend",
		Entity: 12,
	}
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 12)))
	edge.Value = []byte("notphoton")
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 12)))

	// Issue a similar query.
	query = newQuery("friend", nil, []string{"anyofterms", "", "hey photon notphoton notphotonExtra"})
	r, err = helpProcessTask(context.Background(), query, 1)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{},
		{12},
		{},
		{10},
	}, algo.ToUintsListForTest(r.UidMatrix))

	// Try deleting.
	edge = &protos.DirectedEdge{
		Value:  []byte("photon"),
		Label:  "author0",
		Attr:   "friend",
		Entity: 10,
	}
	// Redundant deletes.
	delEdge(t, edge, getOrCreate(x.DataKey("friend", 10)))
	delEdge(t, edge, getOrCreate(x.DataKey("friend", 10)))

	// Delete followed by set.
	edge.Entity = 12
	edge.Value = []byte("notphoton")
	delEdge(t, edge, getOrCreate(x.DataKey("friend", 12)))
	edge.Value = []byte("ignored")
	addEdge(t, edge, getOrCreate(x.DataKey("friend", 12)))

	// Issue a similar query.
	query = newQuery("friend", nil, []string{"anyofterms", "", "photon notphoton ignored"})
	r, err = helpProcessTask(context.Background(), query, 1)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{12},
		{},
		{},
	}, algo.ToUintsListForTest(r.UidMatrix))
}

/*
func populateGraphForSort(t *testing.T, ps store.Store) {
	edge := &protos.DirectedEdge{
		Label: "author1",
		Attr:  "dob",
	}

	dobs := []string{
		"1980-05-05", // 10 (1980)
		"1980-04-05", // 11
		"1979-05-05", // 12 (1979)
		"1979-02-05", // 13
		"1979-03-05", // 14
		"1965-05-05", // 15 (1965)
		"1965-04-05", // 16
		"1965-03-05", // 17
		"1970-05-05", // 18 (1970)
		"1970-04-05", // 19
		"1970-01-05", // 20
		"1970-02-05", // 21
	}
	// The sorted UIDs are: (17 16 15) (20 21 19 18) (13 14 12) (11 10)

	for i, dob := range dobs {
		edge.Entity = uint64(i + 10)
		edge.Value = []byte(dob)
		addEdge(t, edge,
			getOrCreate(x.DataKey(edge.Attr, edge.Entity)))
	}
	time.Sleep(200 * time.Millisecond) // Let indexing finish.
}

// newSort creates a protos.Sort for sorting.
func newSort(uids [][]uint64, offset, count int) *protos.Sort {
	x.AssertTrue(uids != nil)
	uidMatrix := make([]*protos.List, len(uids))
	for i, l := range uids {
		uidMatrix[i] = &protos.List{Uids: l}
	}
	return &protos.Sort{
		Attr:      "dob",
		Offset:    int32(offset),
		Count:     int32(count),
		UidMatrix: uidMatrix,
	}
}

func TestProcessSort(t *testing.T) {
	dir, ps := initTest(t, `dob:date @index .`)
	defer os.RemoveAll(dir)
	defer ps.Close()
	populateGraphForSort(t, ps)

	sort := newSort([][]uint64{
		{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21},
		{10, 11, 12, 13, 14, 21},
		{16, 17, 18, 19, 20, 21},
	}, 0, 1000)
	r, err := processSort(sort)
	require.NoError(t, err)

	// The sorted UIDs are: (17 16 15) (20 21 19 18) (13 14 12) (11 10)
	require.EqualValues(t, [][]uint64{
		{17, 16, 15, 20, 21, 19, 18, 13, 14, 12, 11, 10},
		{21, 13, 14, 12, 11, 10},
		{17, 16, 20, 21, 19, 18}},
		algo.ToUintsListForTest(r.UidMatrix))
}

func TestProcessSortOffset(t *testing.T) {
	dir, ps := initTest(t, `dob:date @index .`)
	defer os.RemoveAll(dir)
	defer ps.Close()
	populateGraphForSort(t, ps)

	input := [][]uint64{
		{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21},
		{10, 11, 12, 13, 14, 21},
		{16, 17, 18, 19, 20, 21}}

	// Offset 1.
	sort := newSort(input, 1, 1000)
	r, err := processSort(sort)
	require.NoError(t, err)
	require.EqualValues(t, [][]uint64{
		{16, 15, 20, 21, 19, 18, 13, 14, 12, 11, 10},
		{13, 14, 12, 11, 10},
		{16, 20, 21, 19, 18}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 2.
	sort = newSort(input, 2, 1000)
	r, err = processSort(sort)
	require.NoError(t, err)
	require.EqualValues(t, [][]uint64{
		{15, 20, 21, 19, 18, 13, 14, 12, 11, 10},
		{14, 12, 11, 10},
		{20, 21, 19, 18}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 5.
	sort = newSort(input, 5, 1000)
	r, err = processSort(sort)
	require.NoError(t, err)
	require.EqualValues(t, [][]uint64{
		{19, 18, 13, 14, 12, 11, 10},
		{10},
		{18}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 6.
	sort = newSort(input, 6, 1000)
	r, err = processSort(sort)
	require.NoError(t, err)
	require.EqualValues(t, [][]uint64{
		{18, 13, 14, 12, 11, 10},
		{},
		{}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 7.
	sort = newSort(input, 7, 1000)
	r, err = processSort(sort)
	require.NoError(t, err)
	require.EqualValues(t, [][]uint64{
		{13, 14, 12, 11, 10},
		{},
		{}},
		algo.ToUintsListForTest(r.UidMatrix))
}

func TestProcessSortCount(t *testing.T) {
	dir, ps := initTest(t, `dob:date @index .`)
	defer os.RemoveAll(dir)
	defer ps.Close()
	populateGraphForSort(t, ps)

	input := [][]uint64{
		{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21},
		{10, 11, 12, 13, 14, 21},
		{16, 17, 18, 19, 20, 21}}

	// Count 1.
	sort := newSort(input, 0, 1)
	r, err := processSort(sort)
	require.NoError(t, err)
	require.EqualValues(t, [][]uint64{
		{17},
		{21},
		{17}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Count 2.
	sort = newSort(input, 0, 2)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.NotNil(t, r)
	require.EqualValues(t, [][]uint64{
		{17, 16},
		{21, 13},
		{17, 16}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Count 5.
	sort = newSort(input, 0, 5)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.NotNil(t, r)
	require.EqualValues(t, [][]uint64{
		{17, 16, 15, 20, 21},
		{21, 13, 14, 12, 11},
		{17, 16, 20, 21, 19}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Count 6.
	sort = newSort(input, 0, 6)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.NotNil(t, r)
	require.EqualValues(t, [][]uint64{
		{17, 16, 15, 20, 21, 19},
		{21, 13, 14, 12, 11, 10},
		{17, 16, 20, 21, 19, 18}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Count 7.
	sort = newSort(input, 0, 7)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.NotNil(t, r)
	require.EqualValues(t, [][]uint64{
		{17, 16, 15, 20, 21, 19, 18},
		{21, 13, 14, 12, 11, 10},
		{17, 16, 20, 21, 19, 18}},
		algo.ToUintsListForTest(r.UidMatrix))
}

func TestProcessSortOffsetCount(t *testing.T) {
	dir, ps := initTest(t, `dob:date @index .`)
	defer os.RemoveAll(dir)
	defer ps.Close()
	populateGraphForSort(t, ps)

	input := [][]uint64{
		{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21},
		{10, 11, 12, 13, 14, 21},
		{16, 17, 18, 19, 20, 21}}

	// Offset 1. Count 1.
	sort := newSort(input, 1, 1)
	r, err := processSort(sort)
	require.NoError(t, err)
	require.EqualValues(t, [][]uint64{
		{16},
		{13},
		{16}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 1. Count 2.
	sort = newSort(input, 1, 2)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{16, 15},
		{13, 14},
		{16, 20}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 1. Count 3.
	sort = newSort(input, 1, 3)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{16, 15, 20},
		{13, 14, 12},
		{16, 20, 21}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 1. Count 1000.
	sort = newSort(input, 1, 1000)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{16, 15, 20, 21, 19, 18, 13, 14, 12, 11, 10},
		{13, 14, 12, 11, 10},
		{16, 20, 21, 19, 18}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 5. Count 1.
	sort = newSort(input, 5, 1)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{19},
		{10},
		{18}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 5. Count 2.
	sort = newSort(input, 5, 2)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{19, 18},
		{10},
		{18}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 5. Count 3.
	sort = newSort(input, 5, 3)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{19, 18, 13},
		{10},
		{18}},
		algo.ToUintsListForTest(r.UidMatrix))

	// Offset 100. Count 100.
	sort = newSort(input, 100, 100)
	r, err = processSort(sort)
	require.NoError(t, err)

	require.EqualValues(t, [][]uint64{
		{},
		{},
		{}},
		algo.ToUintsListForTest(r.UidMatrix))
}
*/

func TestMain(m *testing.M) {
	x.Init()
	posting.Config.AllottedMemory = 1024.0
	posting.Config.CommitFraction = 0.10
	os.Exit(m.Run())
}
