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

package posting

import "testing"

func TestHashBasic(t *testing.T) {
	a := newShardedListMap(32)
	if a.Size() != 0 {
		t.Error("Expected empty a")
		return
	}

	l1 := new(List)
	l := a.PutIfMissing(123, l1)
	if l != l1 {
		t.Error("Put failed")
		return
	}

	l2 := new(List)
	l = a.PutIfMissing(123, l2)
	if l != l1 {
		t.Error("Expected PutIfMissing to be noop and return l1")
		return
	}

	l, found := a.Get(123)
	if !found || l != l1 {
		t.Error("Missing or wrong valid for key 123")
		return
	}

	a.EachWithDelete(func(k uint64, l *List) {
		if k != 123 {
			t.Errorf("Expected to delete key 123 but got d", k)
			t.Fail()
		}
		if l != l1 {
			t.Error("Expected to delete value l1 but got something else")
			t.Fail()
		}
	})

	if a.Size() != 0 {
		t.Error("Expected empty a after EachWithDelete call")
		return
	}
}

// Good for testing for race conditions.
func TestHashConcurrent(t *testing.T) {
	a := newShardedListMap(1)
	var lists []*List
	for i := 0; i < 1000; i++ {
		lists = append(lists, new(List))
	}

	for i := 0; i < 1000; i++ {
		go func(i int) {
			a.PutIfMissing(uint64(i), lists[i])
		}(i)
	}

	for i := 0; i < 1000; i++ {
		go func(i int) {
			l, _ := a.Get(uint64(i))
			if l != lists[i] {
				t.Errorf("Expected lists[%d] but got something else", i)
				t.Fail()
			}
		}(i)
	}
}
