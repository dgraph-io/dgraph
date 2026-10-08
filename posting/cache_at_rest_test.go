package posting

import (
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/dgraph/v25/codec"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

func commitPostingLists(tb testing.TB, attr string, n int, commitTs uint64) [][]byte {
	keys := make([][]byte, n)
	uids := make([]uint64, 100)
	txn := ps.NewTransactionAt(commitTs, true)
	for i := range keys {
		for j := range uids {
			uids[j] = uint64(i*1000 + j + 1)
		}
		keys[i] = x.DataKey(x.NamespaceAttr(0, attr), uint64(i+1))
		val, err := proto.Marshal(&pb.PostingList{Pack: codec.Encode(uids, 256)})
		require.NoError(tb, err)
		require.NoError(tb, txn.SetEntry(badger.NewEntry(keys[i], val).WithMeta(BitCompletePosting)))
	}
	require.NoError(tb, txn.CommitAt(commitTs, nil))
	return keys
}

func TestCacheServesListsThatDidNotChange(t *testing.T) {
	ml := initMemoryLayer(64<<20, false)
	key := commitPostingLists(t, "cache.at.rest.test", 1, 5)[0]

	for _, readTs := range []uint64{5, 10, 1000} {
		_, err := ml.ReadData(key, ps, readTs, false)
		require.NoError(t, err)
		ml.cache.data.Wait()
		require.NotNil(t, ml.readFromCache(key, readTs, false), "readTs %d", readTs)
	}
}

func BenchmarkReadDataFromCache(b *testing.B) {
	const commitTs, lists = 5, 1000
	ml := initMemoryLayer(256<<20, false)
	keys := commitPostingLists(b, "cache.at.rest.bench", lists, commitTs)

	for _, c := range []struct {
		name   string
		readTs uint64
	}{{"atCommitTs", commitTs}, {"afterCommit", commitTs + 1}} {
		b.Run(c.name, func(b *testing.B) {
			read := func(key []byte) {
				_, err := ml.ReadData(key, ps, c.readTs, false)
				require.NoError(b, err)
			}
			for _, key := range keys {
				read(key)
			}
			ml.cache.data.Wait()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				read(keys[i%lists])
			}
		})
	}
}
