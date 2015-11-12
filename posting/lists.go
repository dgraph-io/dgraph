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

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgryski/go-farm"
)

var lmutex sync.RWMutex
var lcache map[uint64]*List
var pstore *store.Store
var clog *commit.Logger

func Init(posting *store.Store, log *commit.Logger) {
	lmutex.Lock()
	defer lmutex.Unlock()

	lcache = make(map[uint64]*List)
	pstore = posting
	clog = log
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
	list.init(key, pstore, clog)
	lcache[uid] = list
	return list
}
