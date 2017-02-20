/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package algo

import (
	"fmt"
	"time"

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func DataKeysForPrefix(prefix string, store *store.Store) map[string]*x.ParsedKey {
	fmt.Printf("[DataKeysForPrefix] prefix: %v\n", prefix)
	startTm := time.Now()
	dict := make(map[string]*x.ParsedKey)
	// Iterate over
	it := store.NewIterator()
	defer it.Close()
	pk := x.Parse(x.DataKey(prefix, 0))
	dataPrefix := pk.DataPrefix()
	fmt.Printf("data prefix: %v\n", dataPrefix)

	it.Seek(dataPrefix)
	for it.ValidForPrefix(dataPrefix) {
		byt := it.Key().Data()
		k := x.Parse(byt)
		x.AssertTrue(k != nil)
		x.AssertTrue(k.IsData())
		dict[string(byt)] = k
		it.Next()
	}
	fmt.Printf("Search From db time usage: %v\n", time.Since(startTm))
	return dict
}
