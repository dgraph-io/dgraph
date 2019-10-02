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

package ristretto

import (
	"sync"
)

// store is the interface fulfilled by all hash map implementations in this
// file. Some hash map implementations are better suited for certain data
// distributions than others, so this allows us to abstract that out for use
// in Ristretto.
//
// Every store is safe for concurrent usage.
type store interface {
	// Get returns the value associated with the key parameter.
	Get(uint64) (interface{}, bool)
	// Set adds the key-value pair to the Map or updates the value if it's
	// already present.
	Set(uint64, interface{})
	// Del deletes the key-value pair from the Map.
	Del(uint64)
	// Clear clears all contents of the store.
	Clear()
	// Update attempts to update the key with a new value and returns true if
	// successful.
	Update(uint64, interface{}) bool
}

// newStore returns the default store implementation.
func newStore() store {
	return newShardedMap()
}

const numShards uint64 = 256

type shardedMap struct {
	shards []*lockedMap
}

func newShardedMap() *shardedMap {
	sm := &shardedMap{shards: make([]*lockedMap, int(numShards))}
	for i := range sm.shards {
		sm.shards[i] = newLockedMap()
	}
	return sm
}

func (sm *shardedMap) Get(key uint64) (interface{}, bool) {
	idx := key % numShards
	return sm.shards[idx].Get(key)
}

func (sm *shardedMap) Set(key uint64, value interface{}) {
	idx := key % numShards
	sm.shards[idx].Set(key, value)
}

func (sm *shardedMap) Del(key uint64) {
	idx := key % numShards
	sm.shards[idx].Del(key)
}

func (sm *shardedMap) Clear() {
	for i := uint64(0); i < numShards; i++ {
		sm.shards[i].Clear()
	}
}

func (sm *shardedMap) Update(key uint64, value interface{}) bool {
	idx := key % numShards
	return sm.shards[idx].Update(key, value)
}

type lockedMap struct {
	sync.RWMutex
	data map[uint64]interface{}
}

func newLockedMap() *lockedMap {
	return &lockedMap{data: make(map[uint64]interface{})}
}

func (m *lockedMap) Get(key uint64) (interface{}, bool) {
	m.RLock()
	val, found := m.data[key]
	m.RUnlock()
	return val, found
}

func (m *lockedMap) Set(key uint64, value interface{}) {
	m.Lock()
	m.data[key] = value
	m.Unlock()
}

func (m *lockedMap) Del(key uint64) {
	m.Lock()
	delete(m.data, key)
	m.Unlock()
}

func (m *lockedMap) Update(key uint64, value interface{}) bool {
	m.Lock()
	defer m.Unlock()
	if _, found := m.data[key]; found {
		m.data[key] = value
		return true
	}
	return false
}

func (m *lockedMap) Clear() {
	m.Lock()
	defer m.Unlock()
	m.data = make(map[uint64]interface{})
}
