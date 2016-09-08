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

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"context"

	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func checkUids(t *testing.T, l *List, uids ...uint64) error {
	if l.Length() != len(uids) {
		return fmt.Errorf("Expected: %d. Length: %d", len(uids), l.Length())
	}
	for i := 0; i < len(uids); i++ {
		var p types.Posting
		if ok := l.Get(&p, i); !ok {
			return fmt.Errorf("Unable to retrieve posting.")
		}
		if p.Uid() != uids[i] {
			return fmt.Errorf("Expected: %v. Got: %v", uids[i], p.Uid())
		}
	}
	if len(uids) >= 3 {
		opts := ListOptions{1, 2, 0}
		ruids := l.Uids(opts)
		if len(ruids) != 2 {
			return fmt.Errorf("Expected result of length: 2. Got: %v", len(ruids))
		}

		for i := 0; i < len(ruids); i++ {
			if ruids[i] != uids[1+i] {
				return fmt.Errorf("Uids expected: %v. Got: %v", uids[1+i], ruids[i])
			}
		}

		opts = ListOptions{1, -2, 0}
		ruids = l.Uids(opts) // offset should be ignored.
		ulen := len(uids)
		if ulen > 2 && len(ruids) != 2 {
			return fmt.Errorf("Expected result of length: 2. Got: %v", len(ruids))
		}

		for i := 0; i < len(ruids); i++ {
			if ruids[i] != uids[ulen-2+i] {
				return fmt.Errorf("Uids neg count expected: %v. Got: %v",
					uids[ulen-2+i], ruids[i])
			}
		}

		// Tests for "after"
		opts = ListOptions{0, 2, 10}
		ruids = l.Uids(opts)
		if len(ruids) != 2 {
			return fmt.Errorf("Expected result of length: 2. Got: %v", len(ruids))
		}
		for i := 0; i < len(ruids); i++ {
			if ruids[i] != uids[1+i] {
				return fmt.Errorf("Uids expected: %v. Got: %v", uids[1+i], ruids[i])
			}
		}

		opts = ListOptions{0, 2, 80}
		ruids = l.Uids(opts)
		if len(ruids) != 1 {
			return fmt.Errorf("Expected result of length: 1. Got: %v", len(ruids))
		}
		if ruids[0] != 81 {
			return fmt.Errorf("Uids expected: %v. Got: %v", uids[2], ruids[0])
		}

		opts = ListOptions{0, 2, 82}
		ruids = l.Uids(opts)
		if len(ruids) != 0 {
			return fmt.Errorf("Expected result of length: 0. Got: %v", len(ruids))
		}
	}

	return nil
}

func TestAddMutation(t *testing.T) {
	l := NewList()
	key := Key(1, "name")
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}

	defer os.RemoveAll(dir)
	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		return
	}

	l.init(key, ps)

	edge := x.DirectedEdge{
		ValueId:   9,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	ctx := context.Background()
	if _, err := l.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}

	if l.Length() != 1 {
		t.Error("Unable to find added elements in posting list")
	}
	var p types.Posting
	if ok := l.Get(&p, 0); !ok {
		t.Error("Unable to retrieve posting at 1st iter")
		t.Fail()
	}
	if p.Uid() != 9 {
		t.Errorf("Expected 9. Got: %v", p.Uid())
	}
	if string(p.Source()) != "testing" {
		t.Errorf("Expected testing. Got: %v", string(p.Source()))
	}

	// Add another edge now.
	edge.ValueId = 81
	l.AddMutation(ctx, edge, Set)
	if l.Length() != 2 {
		t.Errorf("Length: %d", l.Length())
		t.Fail()
	}

	var uid uint64
	uid = 1
	for i := 0; i < l.Length(); i++ {
		if ok := l.Get(&p, i); !ok {
			t.Error("Unable to retrieve posting at 2nd iter")
		}
		uid *= 9
		if p.Uid() != uid {
			t.Logf("Expected: %v. Got: %v", uid, p.Uid())
		}
	}

	// Add another edge, in between the two above.
	uids := []uint64{
		9, 49, 81,
	}
	edge.ValueId = 49
	if _, err := l.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}

	if err := checkUids(t, l, uids...); err != nil {
		t.Error(err)
	}

	// Delete an edge, add an edge, replace an edge
	edge.ValueId = 49
	if _, err := l.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}

	edge.ValueId = 69
	if _, err := l.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}

	edge.ValueId = 9
	edge.Source = "anti-testing"
	if _, err := l.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}

	uids = []uint64{9, 69, 81}
	if err := checkUids(t, l, uids...); err != nil {
		t.Error(err)
	}

	l.Get(&p, 0)
	if string(p.Source()) != "anti-testing" {
		t.Errorf("Expected: anti-testing. Got: %v", string(p.Source()))
	}
	l.MergeIfDirty(ctx)

	// Try reading the same data in another PostingList.
	dl := NewList()
	dl.init(key, ps)
	if err := checkUids(t, dl, uids...); err != nil {
		t.Error(err)
	}

	if _, err := dl.MergeIfDirty(ctx); err != nil {
		t.Error(err)
	}
	if err := checkUids(t, dl, uids...); err != nil {
		t.Error(err)
	}
}

func TestAddMutation_Value(t *testing.T) {
	ol := NewList()
	key := Key(10, "value")
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}

	defer os.RemoveAll(dir)
	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		return
	}

	ol.init(key, ps)
	log.Println("Init successful.")

	edge := x.DirectedEdge{
		Value:     []byte("oh hey there"),
		Source:    "new-testing",
		Timestamp: time.Now(),
	}
	ctx := context.Background()
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	var p types.Posting
	ol.Get(&p, 0)
	if p.Uid() != math.MaxUint64 {
		t.Errorf("All value uids should go to MaxUint64. Got: %v", p.Uid())
	}
	if !bytes.Equal(p.ValueBytes(), []byte("oh hey there")) {
		t.Errorf("Expected a value. Got: [%q]", string(p.ValueBytes()))
	}

	// Run the same check after committing.
	if _, err := ol.MergeIfDirty(ctx); err != nil {
		t.Error(err)
	}
	{
		var tp types.Posting
		if ok := ol.Get(&tp, 0); !ok {
			t.Error("While retrieving posting")
		}
		if !bytes.Equal(tp.ValueBytes(), []byte("oh hey there")) {
			t.Errorf("Expected a value. Got: [%q]", string(tp.ValueBytes()))
		}
	}

	// The value made it to the posting list. Changing it now.
	edge.Value = []byte(strconv.Itoa(119))
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if ol.Length() != 1 {
		t.Errorf("Length should be one. Got: %v", ol.Length())
	}
	if ok := ol.Get(&p, 0); !ok {
		t.Error("While retrieving posting")
	}
	var intout int
	if intout, err = strconv.Atoi(string(p.ValueBytes())); err != nil {
		t.Error(err)
	}
	if intout != 119 {
		t.Errorf("Expected 119. Got: %v", intout)
	}
}

func benchmarkAddMutations(n int, b *testing.B) {
	// logrus.SetLevel(logrus.DebugLevel)
	l := NewList()
	key := Key(1, "name")
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		b.Error(err)
		return
	}

	defer os.RemoveAll(dir)
	ps, err := store.NewStore(dir)
	if err != nil {
		b.Error(err)
		return
	}

	l.init(key, ps)
	b.ResetTimer()

	ts := time.Now()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		edge := x.DirectedEdge{
			ValueId:   uint64(rand.Intn(b.N) + 1),
			Source:    "testing",
			Timestamp: ts.Add(time.Microsecond),
		}
		if _, err := l.AddMutation(ctx, edge, Set); err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkAddMutations_SyncEveryLogEntry(b *testing.B) {
	benchmarkAddMutations(0, b)
}

func BenchmarkAddMutations_SyncEvery10LogEntry(b *testing.B) {
	benchmarkAddMutations(10, b)
}

func BenchmarkAddMutations_SyncEvery100LogEntry(b *testing.B) {
	benchmarkAddMutations(100, b)
}

func BenchmarkAddMutations_SyncEvery1000LogEntry(b *testing.B) {
	benchmarkAddMutations(1000, b)
}
