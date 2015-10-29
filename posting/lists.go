/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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

package posting

import (
	"sync"

	"github.com/dgryski/go-farm"
	"github.com/dgraph-io/dgraph/store"
)

var lmutex sync.RWMutex
var lcache map[uint64]*List
var pstore *store.Store
var mstore *store.Store

func Init(posting *store.Store, mutation *store.Store) {
	lmutex.Lock()
	defer lmutex.Unlock()

	lcache = make(map[uint64]*List)
	pstore = posting
	mstore = mutation
}

func Get(key []byte) *List {
	// Acquire read lock and check if list is available.
	lmutex.RLock()
	uid := farm.Fingerprint64(key)
	if list, ok := lcache[uid]; ok {
		lmutex.RUnlock()
		return list
	}
	lmutex.RUnlock()

	// Couldn't find it. Acquire write lock.
	lmutex.Lock()
	defer lmutex.Unlock()
	// Check again after acquiring write lock.
	if list, ok := lcache[uid]; ok {
		return list
	}

	list := new(List)
	list.init(key, pstore, mstore)
	lcache[uid] = list
	return list
}
