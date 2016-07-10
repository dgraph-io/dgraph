/*
 * Copyright 2015 DGraph Labs, Inc.
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

package store

import (
	"fmt"
	"sync"
)

type Constructor func() Store

type storeRegistry struct {
	stores map[string]Constructor
	sync.RWMutex
}

var Registry = &storeRegistry{
	stores: make(map[string]Constructor),
}

func (r *storeRegistry) Add(name string, constructor Constructor) {
	r.Lock()
	r.stores[name] = constructor
	r.Unlock()
}

func (r *storeRegistry) Get(name string) (Store, error) {
	r.RLock()
	constructor, ok := r.stores[name]
	r.RUnlock()
	if !ok {
		return nil, fmt.Errorf("Unregistered store type: %s", name)
	}

	return constructor(), nil
}

func (r *storeRegistry) Registered() []string {
	r.RLock()
	names := make([]string, len(r.stores))
	i := 0
	for k := range r.stores {
		names[i] = k
		i++
	}
	r.RUnlock()

	return names
}
