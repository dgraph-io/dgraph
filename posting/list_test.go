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
	l := getNew()
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
	dl := getNew()
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

func checkValue(ol *List, val string) error {
	if ol.Length() == 0 {
		return x.Errorf("List has length zero")
	}

	var p types.Posting
	ol.Get(&p, 0)
	if p.Uid() != math.MaxUint64 {
		return x.Errorf("All value uids should go to MaxUint64. Got: %v", p.Uid())
	}
	if !bytes.Equal(p.ValueBytes(), []byte(val)) {
		return x.Errorf("Expected a value. Got: [%q]", string(p.ValueBytes()))
	}
	return nil
}

func TestAddMutation_Value(t *testing.T) {
	ol := getNew()
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
	if err := checkValue(ol, "oh hey there"); err != nil {
		t.Error(err)
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
	var p types.Posting
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

func TestAddMutation_jchiu1(t *testing.T) {
	ol := getNew()
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

	// Set value to cars and merge to RocksDB.
	edge := x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	ctx := context.Background()
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if merged, err := ol.MergeIfDirty(ctx); err != nil {
		t.Error(err)
	} else if !merged {
		t.Error(x.Errorf("Unable to merge posting list."))
	}
	if err := checkValue(ol, "cars"); err != nil {
		t.Error(err)
	}

	// Set value to newcars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if err := checkValue(ol, "newcars"); err != nil {
		t.Error(err)
	}

	// Set value to someothercars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("someothercars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if err := checkValue(ol, "someothercars"); err != nil {
		t.Error(err)
	}

	// Set value back to the committed value cars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if err := checkValue(ol, "cars"); err != nil {
		t.Error(err)
	}
}

func TestAddMutation_jchiu2(t *testing.T) {
	ol := getNew()
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

	// Del a value cars and but don't merge.
	edge := x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	ctx := context.Background()
	if _, err := ol.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}

	// Set value to newcars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if err := checkValue(ol, "newcars"); err != nil {
		t.Error(err)
	}

	// Set value back to cars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}
	if err := checkValue(ol, "newcars"); err != nil {
		t.Error(err)
	}
}

func TestAddMutation_jchiu3(t *testing.T) {
	ol := getNew()
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

	// Set value to cars and merge to RocksDB.
	edge := x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	ctx := context.Background()
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if merged, err := ol.MergeIfDirty(ctx); err != nil {
		t.Error(err)
	} else if !merged {
		t.Error(x.Errorf("Unable to merge posting list."))
	}
	if err := checkValue(ol, "cars"); err != nil {
		t.Error(err)
	}

	// Del a value cars and but don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}
	if ol.Length() > 0 {
		t.Errorf("Length should be zero")
	}

	// Set value to newcars, but don't merge yet.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if err := checkValue(ol, "newcars"); err != nil {
		t.Error(err)
	}

	// Del a value othercars and but don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("othercars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}
	if ol.Length() == 0 {
		t.Errorf("Length shouldn't be zero")
	}
	var p types.Posting
	if !ol.Get(&p, 0) {
		t.Errorf("Error while retrieving posting")
	}
	if string(p.ValueBytes()) != "newcars" {
		t.Errorf("Value expected: newcars. Got: %q", p.ValueBytes())
	}

	// Del a value newcars and but don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}
	if ol.Length() > 0 {
		var p types.Posting
		if !ol.Get(&p, 0) {
			t.Errorf("Error while retrieving posting")
		}
		t.Errorf("Length should be zero. Got: %q", p.ValueBytes())
	}
}

func TestAddMutation_mrjn1(t *testing.T) {
	ol := getNew()
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

	// Set a value cars and merge.
	edge := x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	ctx := context.Background()
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if merged, err := ol.MergeIfDirty(ctx); err != nil {
		t.Error(err)
	} else if !merged {
		t.Error(x.Errorf("Unable to merge posting list."))
	}

	// Delete a non-existent value newcars. This should have no effect.
	edge = x.DirectedEdge{
		Value:     []byte("newcars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}
	if err := checkValue(ol, "cars"); err != nil {
		t.Error(err)
	}

	// Delete the previously committed value cars. But don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}
	if ol.Length() > 0 {
		t.Errorf("Length should be zero after deletion")
	}

	// Do this again to cover Del, muid == curUid, inPlist test case.
	// Delete the previously committed value cars. But don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}
	if ol.Length() > 0 {
		t.Errorf("Length should be zero after deletion")
	}

	// Set the value again to cover Set, muid == curUid, inPlist test case.
	// Set the previously committed value cars. But don't merge.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
		t.Error(err)
	}
	if err := checkValue(ol, "cars"); err != nil {
		t.Error(err)
	}

	// Delete it again, just for fun.
	edge = x.DirectedEdge{
		Value:     []byte("cars"),
		Source:    "jchiu",
		Timestamp: time.Now(),
	}
	if _, err := ol.AddMutation(ctx, edge, Del); err != nil {
		t.Error(err)
	}
	if ol.Length() > 0 {
		t.Errorf("Length should be zero after deletion")
	}
}

func TestAddMutation_checksum(t *testing.T) {
	var c1, c2, c3 string

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

	{
		ol := getNew()
		key := Key(10, "value")
		ol.init(key, ps)
		ctx := context.Background()

		edge := x.DirectedEdge{
			ValueId:   1,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
			t.Error(err)
		}
		edge = x.DirectedEdge{
			ValueId:   3,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
			t.Error(err)
		}
		if merged, err := ol.MergeIfDirty(ctx); err != nil {
			t.Error(err)
		} else if !merged {
			t.Error(x.Errorf("Unable to merge posting list."))
		}
		pl := ol.getPostingList()
		c1 = string(pl.Checksum())
	}

	{
		ol := getNew()
		key := Key(10, "value2")
		ol.init(key, ps)
		ctx := context.Background()

		// Add in reverse.
		edge := x.DirectedEdge{
			ValueId:   3,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
			t.Error(err)
		}
		edge = x.DirectedEdge{
			ValueId:   1,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
			t.Error(err)
		}

		if merged, err := ol.MergeIfDirty(ctx); err != nil {
			t.Error(err)
		} else if !merged {
			t.Error(x.Errorf("Unable to merge posting list."))
		}
		pl := ol.getPostingList()
		c2 = string(pl.Checksum())
	}

	if c1 != c2 {
		t.Errorf("Checksums should match: %v %v", c1, c2)
	}

	{
		ol := getNew()
		key := Key(10, "value3")
		ol.init(key, ps)
		ctx := context.Background()

		// Add in reverse.
		edge := x.DirectedEdge{
			ValueId:   3,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
			t.Error(err)
		}
		edge = x.DirectedEdge{
			ValueId:   1,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
			t.Error(err)
		}
		edge = x.DirectedEdge{
			ValueId:   4,
			Source:    "jchiu",
			Timestamp: time.Now(),
		}
		if _, err := ol.AddMutation(ctx, edge, Set); err != nil {
			t.Error(err)
		}

		if merged, err := ol.MergeIfDirty(ctx); err != nil {
			t.Error(err)
		} else if !merged {
			t.Error(x.Errorf("Unable to merge posting list."))
		}
		pl := ol.getPostingList()
		c3 = string(pl.Checksum())
	}

	if c3 == c1 {
		t.Errorf("Checksum should be different: %v %v", c1, c3)
	}
}

func benchmarkAddMutations(n int, b *testing.B) {
	// logrus.SetLevel(logrus.DebugLevel)
	l := getNew()
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
