/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func checkUids(t *testing.T, l List, uids ...uint64) error {
	if l.Length() != len(uids) {
		return fmt.Errorf("Length: %d", l.Length())
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
	return nil
}

func NewStore(t *testing.T) string {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		t.Fail()
		return ""
	}
	return path
}

func TestAddMutation(t *testing.T) {
	var l List
	key := Key(1, "name")
	pdir := NewStore(t)
	defer os.RemoveAll(pdir)
	ps := new(store.Store)
	ps.Init(pdir)

	mdir := NewStore(t)
	defer os.RemoveAll(mdir)
	ms := new(store.Store)
	ms.Init(mdir)

	l.init(key, ps, ms)

	edge := x.DirectedEdge{
		ValueId:   9,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	if err := l.AddMutation(edge, Set); err != nil {
		t.Error(err)
	}
	/*
		if err := l.CommitIfDirty(); err != nil {
			t.Error(err)
		}
	*/

	if l.Length() != 1 {
		t.Error("Unable to find added elements in posting list")
	}
	var p types.Posting
	if ok := l.Get(&p, 0); !ok {
		t.Error("Unable to retrieve posting at 1st iter")
		t.Fail()
	}
	if p.Uid() != 9 {
		t.Errorf("Expected 9. Got: %v", p.Uid)
	}
	if string(p.Source()) != "testing" {
		t.Errorf("Expected testing. Got: %v", string(p.Source()))
	}
	// return // Test 1.

	// Add another edge now.
	edge.ValueId = 81
	l.AddMutation(edge, Set)
	// l.CommitIfDirty()
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
	// return // Test 2.

	// Add another edge, in between the two above.
	uids := []uint64{
		9, 49, 81,
	}
	edge.ValueId = 49
	if err := l.AddMutation(edge, Set); err != nil {
		t.Error(err)
	}
	/*
		if err := l.CommitIfDirty(); err != nil {
			t.Error(err)
		}
	*/
	if err := checkUids(t, l, uids...); err != nil {
		t.Error(err)
	}
	// return // Test 3.

	// Delete an edge, add an edge, replace an edge
	edge.ValueId = 49
	if err := l.AddMutation(edge, Del); err != nil {
		t.Error(err)
	}

	edge.ValueId = 69
	if err := l.AddMutation(edge, Set); err != nil {
		t.Error(err)
	}

	edge.ValueId = 9
	edge.Source = "anti-testing"
	if err := l.AddMutation(edge, Set); err != nil {
		t.Error(err)
	}
	/*
		if err := l.CommitIfDirty(); err != nil {
			t.Error(err)
		}
	*/

	uids = []uint64{9, 69, 81}
	if err := checkUids(t, l, uids...); err != nil {
		t.Error(err)
	}

	l.Get(&p, 0)
	if string(p.Source()) != "anti-testing" {
		t.Errorf("Expected: anti-testing. Got: %v", string(p.Source()))
	}

	/*
		if err := l.CommitIfDirty(); err != nil {
			t.Error(err)
		}
	*/
	// Try reading the same data in another PostingList.
	var dl List
	dl.init(key, ps, ms)
	if err := checkUids(t, dl, uids...); err != nil {
		t.Error(err)
	}

	if err := dl.CommitIfDirty(); err != nil {
		t.Error(err)
	}
	if err := checkUids(t, dl, uids...); err != nil {
		t.Error(err)
	}
}

func TestAddMutation_Value(t *testing.T) {
	var ol List
	key := Key(10, "value")
	pdir := NewStore(t)
	defer os.RemoveAll(pdir)
	ps := new(store.Store)
	ps.Init(pdir)

	mdir := NewStore(t)
	defer os.RemoveAll(mdir)
	ms := new(store.Store)
	ms.Init(mdir)

	ol.init(key, ps, ms)

	edge := x.DirectedEdge{
		Value:     "oh hey there",
		Source:    "new-testing",
		Timestamp: time.Now(),
	}
	if err := ol.AddMutation(edge, Set); err != nil {
		t.Error(err)
	}
	var p types.Posting
	ol.Get(&p, 0)
	if p.Uid() != math.MaxUint64 {
		t.Errorf("All value uids should go to MaxUint64. Got: %v", p.Uid())
	}
	var iout interface{}
	if err := ParseValue(&iout, p.ValueBytes()); err != nil {
		t.Error(err)
	}
	out := iout.(string)
	if out != "oh hey there" {
		t.Errorf("Expected a value. Got: [%q]", out)
	}

	// Run the same check after committing.
	if err := ol.CommitIfDirty(); err != nil {
		t.Error(err)
	}
	{
		var tp types.Posting
		if ok := ol.Get(&tp, 0); !ok {
			t.Error("While retrieving posting")
		}
		if err := ParseValue(&iout, tp.ValueBytes()); err != nil {
			t.Error(err)
		}
		out := iout.(string)
		if out != "oh hey there" {
			t.Errorf("Expected a value. Got: [%q]", out)
		}
	}

	// The value made it to the posting list. Changing it now.
	edge.Value = 119
	if err := ol.AddMutation(edge, Set); err != nil {
		t.Error(err)
	}
	if ol.Length() != 1 {
		t.Errorf("Length should be one. Got: %v", ol.Length())
	}
	if ok := ol.Get(&p, 0); !ok {
		t.Error("While retrieving posting")
	}
	if err := ParseValue(&iout, p.ValueBytes()); err != nil {
		t.Error(err)
	}
	intout := iout.(float64)
	if intout != 119 {
		t.Errorf("Expected 119. Got: %v", intout)
	}
}
