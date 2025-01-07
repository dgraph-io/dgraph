//go:build debug
// +build debug

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package x

import (
	"bytes"
	"log"
	"sort"

	"github.com/golang/glog"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/badger/v4"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/hypermodeinc/dgraph/v24/protos/pb"
)

func PrintRollup(plist *pb.PostingList, parts map[uint64]*pb.PostingList, baseKey []byte, ts uint64) {
	k, _ := Parse(baseKey)
	glog.V(2).Infof("[TXNLOG] DOING ROLLUP for key: %+v at timestamp: %v", k, ts)
}

func PrintMutationEdge(plist *pb.DirectedEdge, key ParsedKey, startTs uint64) {
	glog.V(2).Infof("[TXNLOG] ADDING MUTATION at TS: %v, key: %v, value: %v", startTs, key.String(), plist)
}

func PrintOracleDelta(delta *pb.OracleDelta) {
	for _, status := range delta.Txns {
		glog.V(2).Infof("[TXNLOG] COMMITING: startTs: %v, commitTs: %v", status.StartTs, status.CommitTs)
	}
}

// VerifyPack checks that the Pack should not be nil if the postings exist.
func VerifyPack(plist *pb.PostingList) {
	if plist.Pack == nil && len(plist.Postings) > 0 {
		log.Panic("UID Pack verification failed: Pack is nil for posting list: %+v", plist)
	}
}

// VerifySnapshot iterates over all the keys in badger. For all data keys it checks
// if key is a split key and it verifies if all part are present in badger as well.
func VerifySnapshot(pstore *badger.DB, readTs uint64) {
	stream := pstore.NewStreamAt(readTs)
	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		for ; itr.Valid(); itr.Next() {
			item := itr.Item()
			if item.IsDeletedOrExpired() {
				break
			}
			if !bytes.Equal(key, item.Key()) {
				// Break out on the first encounter with another key.
				break
			}

			k := item.Key()
			parsedKey, kErr := Parse(k)
			Checkf(kErr, "Error parsing key: %v, version: %d", k, item.Version())
			if !parsedKey.IsData() {
				continue
			}

			err := item.Value(func(v []byte) error {
				plist := &pb.PostingList{}
				Check(proto.Unmarshal(v, plist))
				VerifyPack(plist)
				if len(plist.Splits) == 0 {
					return nil
				}
				if plist.Splits[0] != uint64(1) {
					log.Panic("First split UID is not 1 baseKey: ", k,
						" version ", item.Version())
				}
				for _, uid := range plist.Splits {
					sKey, kErr := SplitKey(k, uid)
					Checkf(kErr,
						"Error creating split key from base key: %v, version: %d", k,
						item.Version())
					newTxn := pstore.NewTransactionAt(readTs, false)
					_, dbErr := newTxn.Get(sKey)
					if dbErr != nil {
						log.Panic("Snapshot verification failed: Unable to find splitKey: ",
							sKey, "\nbaseKey: ", " version: ", item.Version(),
							parsedKey, "\nSplits: ", plist.Splits,
						)
					}
				}
				return nil
			})
			Checkf(err, "Error getting value of key: %v version: %v", k, item.Version())

			if item.DiscardEarlierVersions() {
				break
			}
		}
		return nil, nil
	}
}

// VerifyPostingSplits checks if all the keys from parts are
// present in kvs. Parts is a map of split keys -> postinglist.
func VerifyPostingSplits(kvs []*bpb.KV, plist *pb.PostingList,
	parts map[uint64]*pb.PostingList, baseKey []byte) {
	if len(plist.Splits) == 0 {
		return
	}

	if plist.Splits[0] != uint64(1) {
		log.Panic("Posting split verification failed: First uid of split ",
			plist.Splits[0], " is not 1\nPosting: ", plist)
	}
	for _, uid := range plist.Splits {
		if _, ok := parts[uid]; !ok {
			log.Panic(uid, " split uid is not present")
		}

		partKey, kErr := SplitKey(baseKey, uid)
		if kErr != nil {
			log.Panic("Error while generating splitKey. baseKey: ",
				baseKey, " startUid: ", uid)
		}
		keyIdx := sort.Search(len(kvs), func(i int) bool {
			return bytes.Compare(kvs[i].Key, partKey) >= 0
		})

		if keyIdx == len(kvs) {
			log.Panic("Posting split verification failed: ", partKey,
				" split key not found\nbaseKey: ", baseKey, "\nPosting: ", plist)
		}
	}
}
