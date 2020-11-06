/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package codec

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

func removeDups(uids []uint64) []uint64 {
	if len(uids) == 0 {
		return uids
	}
	sort.Slice(uids, func(a, b int) bool {
		return uids[a] < uids[b]
	})

	set := uids[:1]
	set[0] = uids[0]
	for _, u := range uids {
		if set[len(set)-1] == u {
			continue
		}
		set = append(set, u)
	}
	for i := 1; i < len(set); i++ {
		if set[i] == set[i-1] {
			panic("still equal")
		}
	}
	return set
}
func TestListMap(t *testing.T) {

	test := func(te *testing.T, uids []uint64) {
		lm := NewListMap(nil)
		lm.AddMany(uids)
		// for base, bm := range lm.bitmaps {
		// 	te.Logf("%d -> %d, %d\n", base, bm.GetCardinality(), bm.GetSerializedSizeInBytes())
		// }
		out := lm.ToUids()
		uids = removeDups(uids)
		if len(out) != len(uids) {
			fmt.Printf("len out: %d uids: %d\n", len(out), len(uids))
		}
		for i, u := range uids {
			if out[i] != u {
				fmt.Printf("i: %d out: %d u: %d\n", i, out[i], u)
				break
			}
		}
		require.Equal(te, uids, lm.ToUids())

		pack := lm.ToPack()

		lm2 := NewListMap(pack)
		out = lm2.ToUids()
		require.Equal(te, uids, out)
	}

	t.Run("empty", func(t *testing.T) {
		var uids []uint64
		test(t, uids)
	})

	t.Run("random", func(t *testing.T) {
		var uids []uint64
		for i := 0; i < int(lsbBitMask); i++ {
			uids = append(uids, rand.Uint64())
		}
		test(t, uids)
	})

	t.Run("fill-block", func(t *testing.T) {
		var uids []uint64
		for i := uint64(0); i <= lsbBitMask; i++ {
			uids = append(uids, i)
		}
		test(t, uids)
	})

	t.Run("random-fill-block", func(t *testing.T) {
		var uids []uint64
		for i := uint64(0); i <= lsbBitMask; i++ {
			uids = append(uids, uint64(rand.Int63n(int64(2*lsbBitMask))))
		}
		test(t, uids)
	})
}

func BenchmarkListMap(b *testing.B) {
	N := 10240
	rd := rand.New(rand.NewSource(time.Now().UnixNano()))
	getNum := func() uint64 {
		return uint64(rd.Int63n(int64(1024 * N)))
	}

	var uids []uint64
	for i := 0; i <= N; i++ {
		uids = append(uids, getNum())
	}
	uids = removeDups(uids)
	b.ResetTimer()

	b.Run("add", func(b *testing.B) {
		lm := NewListMap(nil)
		for i := 0; i < b.N; i++ {
			lm.AddOne(uids[rd.Intn(len(uids))])
		}
	})

	b.Run("remove", func(b *testing.B) {
		lm := NewListMap(nil)
		lm.AddMany(uids)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			lm.RemoveOne(uids[rd.Intn(len(uids))])
		}
	})

	var printOnce int
	b.Run("encode", func(b *testing.B) {
		lm := NewListMap(nil)
		lm.AddMany(uids)
		var pack *pb.UidPack
		for i := 0; i < b.N; i++ {
			pack = lm.ToPack()
		}
		b.StopTimer()
		if printOnce == 0 {
			b.Logf("lm: %d pack size: %d blocks: %d\n", lm.NumUids(), pack.Size(), len(pack.Blocks))
		}
		printOnce++
	})

	printOnce = 0
	b.Run("decode", func(b *testing.B) {
		lm := NewListMap(nil)
		lm.AddMany(uids)
		pack := lm.ToPack()
		b.ResetTimer()

		var dst *ListMap
		for i := 0; i < b.N; i++ {
			dst = NewListMap(pack)
			_ = dst
			// idx := rd.Intn(len(uids))
			// dst.RemoveOne(uids[idx])
		}
		b.StopTimer()
		if printOnce == 0 {
			b.Logf("dst: %d pack size: %d blocks: %d\n", dst.NumUids(), pack.Size(), len(pack.Blocks))
		}
		printOnce++
	})
}
