// +build rocksdb,cgo

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

package rocksdb

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
)

func TestGet(t *testing.T) {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		t.Fail()
		return
	}
	defer os.RemoveAll(path)

	var s Store
	s.Init(path)
	k := []byte("mykey")
	if err := s.SetOne(k, []byte("neo")); err != nil {
		t.Error(err)
		t.Fail()
	}

	if val, err := s.Get(k); err != nil {
		t.Error(err)
		t.Fail()
	} else if string(val) != "neo" {
		t.Errorf("Expected 'neo'. Found: %s", string(val))
	}

	if err := s.SetOne(k, []byte("the one")); err != nil {
		t.Error(err)
		t.Fail()
	}

	if val, err := s.Get(k); err != nil {
		t.Error(err)
		t.Fail()
	} else if string(val) != "the one" {
		t.Errorf("Expected 'the one'. Found: %s", string(val))
	}
}

func benchmarkGet(valSize int, b *testing.B) {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		b.Error(err)
		b.Fail()
		return
	}
	defer os.RemoveAll(path)

	var s Store
	s.Init(path)
	buf := make([]byte, valSize)

	nkeys := 100
	for i := 0; i < nkeys; i++ {
		key := []byte(fmt.Sprintf("key_%d", i))
		if err := s.SetOne(key, buf); err != nil {
			b.Error(err)
			return
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := rand.Int() % nkeys
		key := []byte(fmt.Sprintf("key_%d", k))
		val, err := s.Get(key)
		if err != nil {
			b.Error(err)
		}
		if len(val) != valSize {
			b.Errorf("Value size expected: %d. Found: %d", valSize, len(val))
		}
	}
}

func BenchmarkGet_valsize1024(b *testing.B)  { benchmarkGet(1024, b) }
func BenchmarkGet_valsize10KB(b *testing.B)  { benchmarkGet(10240, b) }
func BenchmarkGet_valsize500KB(b *testing.B) { benchmarkGet(1<<19, b) }
func BenchmarkGet_valsize1MB(b *testing.B)   { benchmarkGet(1<<20, b) }

func benchmarkSet(valSize int, b *testing.B) {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		b.Error(err)
		b.Fail()
		return
	}
	defer os.RemoveAll(path)

	var s Store
	s.Init(path)
	buf := make([]byte, valSize)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key_%d", i))
		if err := s.SetOne(key, buf); err != nil {
			b.Error(err)
			return
		}
	}
}

func BenchmarkSet_valsize1024(b *testing.B)  { benchmarkSet(1024, b) }
func BenchmarkSet_valsize10KB(b *testing.B)  { benchmarkSet(10240, b) }
func BenchmarkSet_valsize500KB(b *testing.B) { benchmarkSet(1<<19, b) }
func BenchmarkSet_valsize1MB(b *testing.B)   { benchmarkSet(1<<20, b) }
