/*
 * Copyright 2016 DGraph Labs, Inc.
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

package x

import (
	"container/heap"
	"testing"
)

func TestPush(t *testing.T) {
	h := &Uint64Heap{}
	heap.Init(h)

	e := Elem{Uid: 5}
	heap.Push(h, e)
	e.Uid = 3
	heap.Push(h, e)
	e.Uid = 4
	heap.Push(h, e)

	if h.Len() != 3 {
		t.Errorf("Expected len 3. Found: %v", h.Len())
	}
	if (*h)[0].Uid != 3 {
		t.Errorf("Expected min 3. Found: %+v", (*h)[0])
	}
	e.Uid = 10
	(*h)[0] = e
	heap.Fix(h, 0)
	if (*h)[0].Uid != 4 {
		t.Errorf("Expected min 4. Found: %+v", (*h)[0])
	}
	e.Uid = 11
	(*h)[0] = e
	heap.Fix(h, 0)
	if (*h)[0].Uid != 5 {
		t.Errorf("Expected min 5. Found: %+v", (*h)[0])
	}

	e = heap.Pop(h).(Elem)
	if e.Uid != 5 {
		t.Errorf("Expected min 5. Found %+v", e)
	}

	e = heap.Pop(h).(Elem)
	if e.Uid != 10 {
		t.Errorf("Expected min 10. Found: %+v", e)
	}
	e = heap.Pop(h).(Elem)
	if e.Uid != 11 {
		t.Errorf("Expected min 11. Found: %+v", e)
	}

	if h.Len() != 0 {
		t.Errorf("Expected len 0. Found: %v, values: %+v", h.Len(), h)
	}
}
