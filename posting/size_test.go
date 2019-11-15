/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package posting

import (
	"testing"

	_ "net/http/pprof"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

var (
	list *List

	plist *pb.PostingList

	pack *pb.UidPack

	block *pb.UidBlock

	posting *pb.Posting

	facet *api.Facet
)

func BenchmarkPostingList(b *testing.B) {
	for i := 0; i < b.N; i++ {
		list = &List{}
		list.mutationMap = make(map[uint64]*pb.PostingList)
	}
}

func BenchmarkUidPack(b *testing.B) {
	for i := 0; i < b.N; i++ {
		pack = &pb.UidPack{}
	}
}

func BenchmarkUidBlock(b *testing.B) {
	for i := 0; i < b.N; i++ {
		block = &pb.UidBlock{}
	}
}

func BenchmarkPosting(b *testing.B) {
	for i := 0; i < b.N; i++ {
		posting = &pb.Posting{}
	}
}

func BenchmarkFacet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		facet = &api.Facet{}
	}
}

func TestPostingListCalculation(t *testing.T) {
	list = &List{}
	list.mutationMap = make(map[uint64]*pb.PostingList)
	// 144 is obtained from BenchmarkPostingList
	require.Equal(t, int(144), list.DeepSize())
}

func TestUidPackCalculation(t *testing.T) {
	pack = &pb.UidPack{}
	// 64 is obtained from BenchmarkUidPack
	require.Equal(t, int(64), calculatePackSize(pack))
}

func TestUidBlockCalculation(t *testing.T) {
	block = &pb.UidBlock{}
	// 80 is obtained from BenchmarkUidBlock
	require.Equal(t, int(80), calculateUIDBlock(block))
}

func TestPostingCalculation(t *testing.T) {
	posting = &pb.Posting{}
	// 160 is obtained from BenchmarkPosting
	require.Equal(t, int(160), calculatePostingSize(posting))
}

func TestFacetCalculation(t *testing.T) {
	facet = &api.Facet{}
	// 128 is obtained from BenchmarkFacet
	require.Equal(t, int(128), calcuateFacet(facet))
}

// run this test manually for the verfication.
// func PopulateList(l *List, t *testing.T) {
// 	kvOpt := badger.DefaultOptions("/home/schoolboy/src/github.com/dgraph-io/dgraph/dgraph/out/0/p")
// 	ps, err := badger.OpenManaged(kvOpt)
// 	require.NoError(t, err)
// 	txn := ps.NewTransactionAt(math.MaxUint64, false)
// 	defer txn.Discard()
// 	iopts := badger.DefaultIteratorOptions
// 	iopts.AllVersions = true
// 	iopts.PrefetchValues = false
// 	itr := txn.NewIterator(iopts)
// 	defer itr.Close()
// 	var i uint64
// 	for itr.Rewind(); itr.Valid(); itr.Next() {
// 		item := itr.Item()
// 		if item.ValueSize() < 512 || item.UserMeta() == BitSchemaPosting {
// 			continue
// 		}
// 		pl, err := ReadPostingList(item.Key(), itr)
// 		require.NoError(t, err)
// 		l.mutationMap[i] = pl.plist
// 		i++
// 	}
// }
// func Test21MillionDataSet(t *testing.T) {
// 	l := &List{}
// 	l.mutationMap = make(map[uint64]*pb.PostingList)
// 	PopulateList(l, t)
// 	runtime.GC()

// 	fp, _ := os.Create("mem.out")
// 	pprof.WriteHeapProfile(fp)
// 	fp.Sync()
// 	fp.Close()
// 	fmt.Println(l.DeepSize())
// }
