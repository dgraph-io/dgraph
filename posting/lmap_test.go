/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
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
	"math"
	"math/rand"
	"testing"
)

func BenchmarkGet(b *testing.B) {
	// lmap := NewMap(false)
	var key []byte
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// i := uint64(rand.Int63())
			_ = uint64(rand.Int63())
			getNew(key, nil, math.MaxUint64)
			// lmap.Get(i)
		}
	})
}

func BenchmarkGetLinear(b *testing.B) {
	var key []byte
	m := make(map[uint64]*List)
	for i := 0; i < b.N; i++ {
		k := uint64(i)
		if _, ok := m[k]; !ok {
			l, err := getNew(key, nil, math.MaxUint64)
			if err != nil {
				b.Error(err)
			}
			m[k] = l
		}
	}
}

func BenchmarkGetLinearBool(b *testing.B) {
	m := make(map[uint64]bool)
	for i := 0; i < b.N; i++ {
		k := uint64(i)
		if _, ok := m[k]; !ok {
			m[k] = true
		}
	}
}
