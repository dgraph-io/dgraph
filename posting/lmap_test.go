/*
 * Copyright 2015-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
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
		if _, ok := m[k]; !ok {
			l, err := getNew(key, nil)
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
