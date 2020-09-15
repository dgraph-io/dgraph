// +build debug

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

package x

import (
	"bytes"
	"log"
	"sort"

	"github.com/dgraph-io/badger/v2"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/dgraph/protos/pb"
)

// VerifySnapshot iterates over all the keys in badger. For all data keys it checks
// if key is a split key and it verifies if all part are present in badger as well.
func VerifySnapshot(pstore *badger.DB, readTs uint64) {
	iOpt := badger.DefaultIteratorOptions
	iOpt.AllVersions = true
	txn := pstore.NewTransactionAt(readTs, false)
	it := txn.NewIterator(iOpt)
	for it.Rewind(); it.Valid(); it.Next() {
		i := it.Item()
		k := i.Key()
		parsedKey, kErr := Parse(k)
		Checkf(kErr, "Error parsing key: %v, version: %d", k, i.Version())
		if !parsedKey.IsData() {
			continue
		}
		v, vErr := i.ValueCopy(nil)
		Checkf(vErr, "Error getting value for key: %v, version: %d", k, i.Version())
		plist := &pb.PostingList{}
		Check(plist.Unmarshal(v))
		if len(plist.Splits) == 0 {
			continue
		}
		if plist.Splits[0] != uint64(1) {
			log.Panic("First split UID is not 1 baseKey: ", k, " version ", i.Version())
		}
		for _, uid := range plist.Splits {
			sKey, kErr := SplitKey(k, uid)
			Checkf(kErr, "Error creating split key from base key: %v, version: %d", k, i.Version())
			newTxn := pstore.NewTransactionAt(readTs, false)
			_, dbErr := newTxn.Get(sKey)
			if dbErr != nil {
				log.Panic("Snapshot verification failed: Unable to find splitKey: ",
					sKey, "\nbaseKey: ", " version: ", i.Version(),
					parsedKey, "\nSplits: ", plist.Splits,
				)
			}
		}
	}
}

// VerifyPostingSplits checks if all the keys from parts are
// present in kvs. parts is a map of split keys -> postinglist
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
