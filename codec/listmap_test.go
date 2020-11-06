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

	"github.com/stretchr/testify/require"
)

func TestListMap(t *testing.T) {
	removeDups := func(uids []uint64) []uint64 {
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
