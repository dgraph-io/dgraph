/*
 * Copyright 2017-2023 Dgraph Labs, Inc. and Contributors
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
	"os"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/v24/posting"
	"github.com/dgraph-io/dgraph/v24/protos/pb"
	"github.com/dgraph-io/dgraph/v24/schema"
	"github.com/dgraph-io/dgraph/v24/x"
	"github.com/stretchr/testify/require"
)

func BenchmarkAddMutationWithIndex(b *testing.B) {
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

	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0)
	Init(ps)
	err = schema.ParseBytes([]byte("benchmarkadd: string @index(term) ."), 1)
	fmt.Println(err)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.GalaxyAttr("benchmarkadd")

	for i := 0; i < b.N; i++ {
		edge := &pb.DirectedEdge{
			Value:  []byte("david"),
			Attr:   attr,
			Entity: 1,
			Op:     pb.DirectedEdge_SET,
		}

		x.Check(runMutation(ctx, edge, txn))
	}
}

func TestRemoveDuplicates(t *testing.T) {
	toSet := func(uids []uint64) map[uint64]struct{} {
		m := make(map[uint64]struct{})
		for _, uid := range uids {
			m[uid] = struct{}{}
		}
		return m
	}

	for _, test := range []struct {
		setIn   []uint64
		setOut  []uint64
		uidsIn  []uint64
		uidsOut []uint64
	}{
		{setIn: nil, setOut: nil, uidsIn: nil, uidsOut: nil},
		{setIn: nil, setOut: []uint64{2}, uidsIn: []uint64{2}, uidsOut: []uint64{2}},
		{setIn: []uint64{2}, setOut: []uint64{2}, uidsIn: []uint64{2}, uidsOut: []uint64{}},
		{setIn: []uint64{2}, setOut: []uint64{2}, uidsIn: []uint64{2, 2}, uidsOut: []uint64{}},
		{
			setIn:   []uint64{2, 3},
			setOut:  []uint64{2, 3, 4, 5},
			uidsIn:  []uint64{3, 4, 5},
			uidsOut: []uint64{4, 5},
		},
	} {
		set := toSet(test.setIn)
		uidsOut := removeDuplicates(test.uidsIn, set)
		require.Equal(t, uidsOut, test.uidsOut)
		require.Equal(t, set, toSet(test.setOut))
	}
}
