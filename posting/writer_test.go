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

package posting

import (
	"io/ioutil"
	"math"
	"os"
	"sync"
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/golang/glog"
	"github.com/stretchr/testify/require"
)

type kv struct {
	key   []byte
	value []byte
}

func BenchmarkWriter(b *testing.B) {
	createKVList := func() []kv {
		var KVList = []kv{}
		for i := 0; i < 50000; i++ {
			n := kv{key: []byte(string(i)), value: []byte(string(i))}
			KVList = append(KVList, n)
		}
		return KVList
	}

	dbOpts := badger.DefaultOptions("").
		WithLogger(nil).
		WithSyncWrites(false).
		WithNumVersionsToKeep(math.MaxInt64).
		WithCompression(options.None)

	KVList := createKVList()

	b.Run("TxnWriter", func(b *testing.B) {
		tmpIndexDir, err := ioutil.TempDir("", "dgraph")
		require.NoError(b, err)
		defer os.RemoveAll(tmpIndexDir)

		dbOpts.Dir = tmpIndexDir
		dbOpts.ValueDir = tmpIndexDir
		var db, err2 = badger.OpenManaged(dbOpts)
		require.NoError(b, err2)
		defer db.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := NewTxnWriter(db)
			for _, typ := range KVList {
				k := typ.key
				v := typ.value
				w.SetAt(k, v, BitSchemaPosting, 1)
			}
			require.NoError(b, w.Flush())

		}
	})
	b.Run("WriteBatch", func(b *testing.B) {
		tmpIndexDir, err := ioutil.TempDir("", "dgraph")
		require.NoError(b, err)
		defer os.RemoveAll(tmpIndexDir)

		dbOpts.Dir = tmpIndexDir
		dbOpts.ValueDir = tmpIndexDir

		var db, err2 = badger.OpenManaged(dbOpts)
		require.NoError(b, err2)
		defer db.Close()

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			var entries []*badger.Entry
			// 	batch := db.NewWriteBatchAt(1)
			// 	for _, typ := range KVList {
			// 		k := typ.key
			// 		v := typ.value
			// 		batch.Set(k, v)
			// 	}
			// 	require.NoError(b, batch.Flush())
			for _, kv := range KVList {
				// var meta byte
				// var entry *badger.Entry
				// if len(kv.UserMeta) > 0 {
				// 	meta = kv.UserMeta[0]
				// }
				// switch meta {
				// case BitCompletePosting, BitEmptyPosting:
				// 	entry = (&badger.Entry{
				// 		Key:      kv.Key,
				// 		Value:    kv.Value,
				// 		UserMeta: meta,
				// 	}).WithDiscard()
				// default:
				k := kv.key
				v := kv.value
				var meta byte
				var entry *badger.Entry

				entry = &badger.Entry{
					Key:      k,
					Value:    v,
					UserMeta: meta,
				}
				// UserMeta: meta,
				// }
				entry.Key = y.KeyWithTs(entry.Key, 1) //Same version for each key
				entries = append(entries, entry)
			}

			wg.Add(1)
			err := db.BatchSetAsync(entries, func(err error) {
				defer wg.Done()
				if err != nil {
					// TODO: Handle error
					glog.Error(err)
					return
				}
			})
			require.NoError(b, err)
			wg.Wait()

		}
	})
}
