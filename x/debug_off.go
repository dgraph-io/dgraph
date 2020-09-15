// +build !debug

package x

import (
	bpb "github.com/dgraph-io/badger/v2/pb"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/protos/pb"
)

// VerifySnapshot works in debug mode. Check out the comment in debug_on.go
func VerifySnapshot(pstore *badger.DB, readTs uint64) {

}

// VerifyPostingSplits works in debug mode. Check out the comment in debug_on.go
func VerifyPostingSplits(kvs []*bpb.KV, plist *pb.PostingList,
	parts map[uint64]*pb.PostingList, baseKey []byte) {

}
