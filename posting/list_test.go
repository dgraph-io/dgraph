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
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/manishrjain/dgraph/posting/types"
	"github.com/manishrjain/dgraph/store"
	"github.com/manishrjain/dgraph/x"
)

func checkUids(t *testing.T, l List, uids ...uint64) {
	if l.Root().PostingsLength() != len(uids) {
		t.Errorf("Length: %d", l.Root().PostingsLength())
		t.Fail()
	}
	for i := 0; i < len(uids); i++ {
		var p types.Posting
		if ok := l.Root().Postings(&p, i); !ok {
			t.Error("Unable to retrieve posting at 2nd iter")
		}
		if p.Uid() != uids[i] {
			t.Errorf("Expected: %v. Got: %v", uids[i], p.Uid())
		}
	}
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

func TestAddTriple(t *testing.T) {
	var l List
	key := store.Key("name", 1)
	pdir := NewStore(t)
	defer os.RemoveAll(pdir)
	ps := new(store.Store)
	ps.Init(pdir)

	mdir := NewStore(t)
	defer os.RemoveAll(mdir)
	ms := new(store.Store)
	ms.Init(mdir)

	l.Init(key, ps, ms)

	triple := x.Triple{
		ValueId:   9,
		Source:    "testing",
		Timestamp: time.Now(),
	}
	if err := l.AddMutation(triple, Set); err != nil {
		t.Error(err)
	}
	if err := l.Commit(); err != nil {
		t.Error(err)
	}

	if l.Root().PostingsLength() != 1 {
		t.Error("Unable to find added elements in posting list")
	}
	var p types.Posting
	if ok := l.Root().Postings(&p, 0); !ok {
		t.Error("Unable to retrieve posting at 1st iter")
		t.Fail()
	}
	if p.Uid() != 9 {
		t.Errorf("Expected 9. Got: %v", p.Uid)
	}
	if string(p.Source()) != "testing" {
		t.Errorf("Expected testing. Got: %v", string(p.Source()))
	}

	// Add another triple now.
	triple.ValueId = 81
	l.AddMutation(triple, Set)
	l.Commit()
	if l.Root().PostingsLength() != 2 {
		t.Errorf("Length: %d", l.Root().PostingsLength())
		t.Fail()
	}

	var uid uint64
	uid = 1
	for i := 0; i < l.Root().PostingsLength(); i++ {
		if ok := l.Root().Postings(&p, i); !ok {
			t.Error("Unable to retrieve posting at 2nd iter")
		}
		uid *= 9
		if p.Uid() != uid {
			t.Logf("Expected: %v. Got: %v", uid, p.Uid())
		}
	}

	// Add another triple, in between the two above.
	uids := []uint64{
		9, 49, 81,
	}
	triple.ValueId = 49
	if err := l.AddMutation(triple, Set); err != nil {
		t.Error(err)
	}
	if err := l.Commit(); err != nil {
		t.Error(err)
	}
	checkUids(t, l, uids...)

	// Delete a triple, add a triple, replace a triple
	triple.ValueId = 49
	if err := l.AddMutation(triple, Del); err != nil {
		t.Error(err)
	}

	triple.ValueId = 69
	if err := l.AddMutation(triple, Set); err != nil {
		t.Error(err)
	}

	triple.ValueId = 9
	triple.Source = "anti-testing"
	if err := l.AddMutation(triple, Set); err != nil {
		t.Error(err)
	}
	if err := l.Commit(); err != nil {
		t.Error(err)
	}

	uids = []uint64{9, 69, 81}
	checkUids(t, l, uids...)

	l.Root().Postings(&p, 0)
	if string(p.Source()) != "anti-testing" {
		t.Errorf("Expected: anti-testing. Got: %v", p.Source())
	}

	// Try reading the same data in another PostingList.
	var dl List
	dl.Init(key, ps, ms)
	checkUids(t, dl, uids...)
}
