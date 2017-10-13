/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"flag"
	"fmt"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/x"
)

var (
	postingDir = flag.String("p", "p", "postings")
)

func main() {
	flag.Parse()
	x.Init(true)

	// All the writes to posting store should be synchronous. We use batched writers
	// for posting lists, so the cost of sync writes is amortized.
	opt := badger.DefaultOptions
	opt.Dir = *postingDir
	opt.ValueDir = *postingDir
	opt.TableLoadingMode = options.MemoryMap

	ps, err := badger.NewKV(&opt)
	x.Checkf(err, "Error while creating badger KV posting store")
	defer ps.Close()

	iterOpt := badger.DefaultIteratorOptions
	iterOpt.PrefetchValues = false
	it := ps.NewIterator(iterOpt)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		iterItem := it.Item()
		k := iterItem.Key()
		pk := x.Parse(k)
		estSize := iterItem.EstimatedSize()
		if estSize > 1e7 {
			fmt.Printf("key: %+v, len(val): %v\n", pk, estSize)
		}
	}
}
