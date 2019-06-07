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

package bulk

import (
	"bytes"
	"fmt"

	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/badger/y"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

func reduce(mapEntries []*pb.MapEntry) *bpb.KVList {
	var currentKey []byte
	var uids []uint64
	pl := new(pb.PostingList)

	list := &bpb.KVList{}
	appendToList := func() {
		// TODO: Bring this back.
		// atomic.AddInt64(&r.prog.reduceKeyCount, 1)

		// For a UID-only posting list, the badger value is a delta packed UID
		// list. The UserMeta indicates to treat the value as a delta packed
		// list when the value is read by dgraph.  For a value posting list,
		// the full pb.Posting type is used (which pb.y contains the
		// delta packed UID list).
		if len(uids) == 0 {
			return
		}
		pl.Pack = codec.Encode(uids, 256)
		val, err := pl.Marshal()
		x.Check(err)
		list.Kv = append(list.Kv, &bpb.KV{
			Key:      y.Copy(currentKey),
			Value:    val,
			UserMeta: []byte{posting.BitCompletePosting},
			Version:  1,
		})
		pk := x.Parse(currentKey)
		fmt.Printf("append pk: %+v\n", pk)

		uids = uids[:0]
		pl.Reset()
	}

	for _, mapEntry := range mapEntries {
		// TODO: Bring this back.
		// atomic.AddInt64(&r.prog.reduceEdgeCount, 1)

		if !bytes.Equal(mapEntry.Key, currentKey) && currentKey != nil {
			appendToList()
		}
		currentKey = mapEntry.Key

		uid := mapEntry.Uid
		if mapEntry.Posting != nil {
			uid = mapEntry.Posting.Uid
		}
		if len(uids) > 0 && uids[len(uids)-1] == uid {
			continue
		}
		uids = append(uids, uid)
		if mapEntry.Posting != nil {
			pl.Postings = append(pl.Postings, mapEntry.Posting)
		}
	}
	appendToList()
	return list

	// NumBadgerWrites.Add(1)

	// TODO: Bring this back.
	// for _, kv := range list.Kv {
	// 	pk := x.Parse(kv.Key)
	// 	fmt.Printf("pk: %+v\n", pk)
	// }
	// x.Check(job.writer.Write(list))

	// x.Check(txn.CommitAt(r.state.writeTs, func(err error) {
	// 	x.Check(err)
	// 	NumBadgerWrites.Add(-1)
	// }))
}
