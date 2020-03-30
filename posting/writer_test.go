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
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/x"
)

type kv struct {
	key   []byte
	value []byte
}

var KVList = []kv{}

func BenchmarkTxnWriter(b *testing.B) {

	for i := 0; i < 50; i++ {
		n := kv{key: []byte(string(i)), value: []byte("Check Value")}
		KVList = append(KVList, n)
	}

	var db *badger.DB
	w := NewTxnWriter(db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, typ := range KVList {
			k := typ.key
			v := typ.value
			x.Check(w.SetAt(k, v, BitSchemaPosting, 1))
		}
	}

}
