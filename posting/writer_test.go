/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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
	"fmt"
	"io/ioutil"
	"math"
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/dgraph-io/dgraph/x"
)

type kv struct {
	key   []byte
	value []byte
}

var tmpIndexDir, err = ioutil.TempDir("", "dgraph_index_")

var dbOpts = badger.DefaultOptions(tmpIndexDir).
	WithSyncWrites(false).
	WithNumVersionsToKeep(math.MaxInt64).
	WithLogger(&x.ToGlog{}).
	WithCompression(options.None).
	WithEventLogging(false).
	WithLogRotatesToFlush(10).
	WithMaxCacheSize(50) // TODO(Aman): Disable cache altogether

var db, err2 = badger.OpenManaged(dbOpts)

func createKVList() []kv {
	var KVList = []kv{}
	for i := 0; i < 50000; i++ {
		n := kv{key: []byte(string(i)), value: []byte("Check Value")}
		KVList = append(KVList, n)
	}
	return KVList
}

func BenchmarkTxnWriter(b *testing.B) {
	KVList := createKVList()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := NewTxnWriter(db)
		for _, typ := range KVList {
			k := typ.key
			v := typ.value
			x.Check(w.SetAt(k, v, BitSchemaPosting, 1))
		}
		if err := w.Flush(); err != nil {
			fmt.Printf("Got error while flushing txnwriter: %v\n", err)
		}
	}

}

func BenchmarkWriteBatch(b *testing.B) {
	KVList := createKVList()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch := db.NewWriteBatchAt(1)
		for _, typ := range KVList {
			k := typ.key
			v := typ.value
			x.Check(batch.Set(k, v))
		}
		if err := batch.Flush(); err != nil {
			fmt.Printf("Got error while flushing batch: %v\n", err)
		}
	}

}
