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

package geo

import (
	"github.com/dgraph-io/dgraph/task"
)

// QueryKeys represents the list of keys to be used when querying
type QueryKeys struct {
	keys []string
}

// Length is the number of keys in the list
func (q QueryKeys) Length() int {
	return len(q.keys)
}

// Key returns the ith key for the given attribute
func (q QueryKeys) Key(i int, attr string) []byte {
	return []byte(q.keys[i])
}

// PostFilter returns a function to filter the uids after reading them from the index.
func (q QueryKeys) PostFilter(attr string) func(u uint64) bool {
	return nil
}

// NewQueryKeys creates a QueryKeys object for the given filter.
func NewQueryKeys(f *task.GeoFilter) (*QueryKeys, error) {
	var kg KeyGenerator
	// TODO: Support near queries
	// attr not used by IndexKeys, so we use just an empty string
	keys, err := kg.IndexKeys("", f.DataBytes())
	if err != nil {
		return nil, err
	}
	return &QueryKeys{keys}, nil
}
