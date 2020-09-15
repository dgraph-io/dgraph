// +build debug

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
		Check(kErr)
		if !parsedKey.IsData() {
			continue
		}
		v, vErr := i.ValueCopy(nil)
		Check(vErr)
		plist := &pb.PostingList{}
		err := plist.Unmarshal(v)
		Check(err)
		if len(plist.Splits) == 0 {
			continue
		}
		if plist.Splits[0] != uint64(1) {
			log.Panic("First split UID is not 1")
		}
		for _, uid := range plist.Splits {
			sKey, kErr := SplitKey(k, uid)
			Check(kErr)
			newTxn := pstore.NewTransactionAt(readTs, false)
			_, dbErr := newTxn.Get(sKey)
			if dbErr != nil {
				log.Panic("Snapshot verification failed: Unable to find splitKey: ",
					sKey, "\nbaseKey: ", parsedKey, "\nSplits: ", plist.Splits,
				)
			}
		}
	}
}

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
