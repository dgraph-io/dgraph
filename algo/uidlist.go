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

package algo

import (
	"sort"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/sroar"
	"github.com/golang/glog"
)

const jump = 32 // Jump size in InsersectWithJump.

// ApplyFilter applies a filter to our UIDList.
// TODO: ApplyFilter in this way should only happen for sorted uids. For normal
// filter, it should use Bitmap FastAnd or And.
func ApplyFilter(u *pb.List, f func(uint64, int) bool) {
	if len(u.SortedUids) > 0 {
		glog.Infof("Applying ApplyFilter on sorted uids: %d\n", len(u.SortedUids))
		out := u.SortedUids[:0]
		for i, x := range u.SortedUids {
			if f(x, i) {
				out = append(out, x)
			}
		}
		u.SortedUids = out
		return
	}

	bm := codec.FromList(u)
	var i int
	bm2 := sroar.NewBitmap()
	for itr := bm.NewIterator(); itr.HasNext(); i++ {
		uid := itr.Next()
		if f(uid, i) {
			bm2.Set(uid)
		}
	}
	// Need to think of a better way to abstract this.
	u.Bitmap = bm2.ToBuffer()
	// u.Bitmap = nil
	// u.SortedUids = out
}

// IndexOf performs a binary search on the uids slice and returns the index at
// which it finds the uid, else returns -1
func IndexOf(u *pb.List, uid uint64) int {
	bm := codec.FromList(u)
	// TODO(Ahsan): We might want bm.Rank()
	uids := bm.ToArray()
	i := sort.Search(len(uids), func(i int) bool { return uids[i] >= uid })
	if i < len(uids) && uids[i] == uid {
		return i
	}
	return -1
}
