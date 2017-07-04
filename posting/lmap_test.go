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

package posting

import (
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
			getNew(key, nil)
			// lmap.Get(i)
		}
	})
}

func BenchmarkGetLinear(b *testing.B) {
	var key []byte
	m := make(map[uint64]*List)
	for i := 0; i < b.N; i++ {
		k := uint64(i)
		if l, ok := m[k]; !ok {
			l = getNew(key, nil)
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
