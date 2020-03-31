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
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/stretchr/testify/require"
)

type kv struct {
	key   []byte
	value []byte
}

func BenchmarkWriter(b *testing.B) {
	createKVList := func() []kv {
		var KVList = []kv{}
		for i := 0; i < 5000000; i++ {
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
	validate := false

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
				err = w.SetAt(k, v, BitSchemaPosting, 1)
				if validate {
					require.NoError(b, err)
				}
			}
			err := w.Flush()
			if validate {
				require.NoError(b, err)
			}
		}
	})
	b.Run("Write batch", func(b *testing.B) {
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
			batch := db.NewWriteBatchAt(1)
			for _, typ := range KVList {
				k := typ.key
				v := typ.value
				err = batch.Set(k, v)
				if validate {
					require.NoError(b, err)
				}
			}
			err := batch.Flush()
			if validate {
				require.NoError(b, err)
			}
		}
	})
}
