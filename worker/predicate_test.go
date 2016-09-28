/*
* Copyright 2016 DGraph Labs, Inc.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*         http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */
package worker

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	flatbuffers "github.com/google/flatbuffers/go"
)

func checkShard(ps *store.Store) (int, []byte) {
	it := ps.NewIterator()
	defer it.Close()

	count := 0
	for it.SeekToFirst(); it.Valid(); it.Next() {
		count++
	}
	return count, it.Key().Data()
}

func writePLs(t *testing.T, count int, vid uint64, ps *store.Store) {
	for i := 0; i < count; i++ {
		k := fmt.Sprintf("%03d", i)
		t.Logf("key: %v", k)
		list, _ := posting.GetOrCreate([]byte(k), ps)

		de := x.DirectedEdge{
			ValueId:   vid,
			Source:    "test",
			Timestamp: time.Now(),
		}
		list.AddMutation(context.TODO(), de, posting.Set)
		if merged, err := list.MergeIfDirty(context.TODO()); err != nil {
			t.Errorf("While merging: %v", err)
		} else if !merged {
			t.Errorf("No merge happened")
		}
	}
}

func initNode(id uint64, my string) {
	thisNode = newNode(id, my)
}

func TestPopulateShard(t *testing.T) {
	var err error
	dir, err := ioutil.TempDir("", "store0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	posting.Init()

	writePLs(t, 100, 2, ps)
	w := NewState(ps, nil, 0, 1)
	SetWorkerState(w)
	go RunServer(":12345")

	dir1, err := ioutil.TempDir("", "store1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir1)

	ps1, err := store.NewStore(dir1)
	if err != nil {
		t.Fatal(err)
	}
	defer ps1.Close()

	w1 := NewState(ps1, nil, 1, 2)
	go RunServer(":12346")
	SetWorkerState(w)
	pool := NewPool("localhost:12345", 5)
	_, err = w1.PopulateShard(context.Background(), pool, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Getting count on number of keys written to posting list store on instance 1.
	count, k := checkShard(ps1)
	if count != 100 {
		t.Fatalf("Expected %d key value pairs. Got : %d", 100, count)
	}
	if string(k) != "099" {
		t.Fatalf("Expected key to be: %v. Got %v", "099", string(k))
	}

	l, _ := posting.GetOrCreate(k, ps)
	if l.Length() != 1 {
		t.Error("Unable to find added elements in posting list")
	}
	var p types.Posting
	if ok := l.Get(&p, 0); !ok {
		t.Error("Unable to retrieve posting at 1st iter")
		t.Fail()
	}
	if p.Uid() != 2 {
		t.Errorf("Expected 2. Got: %v", p.Uid())
	}
	if string(p.Source()) != "test" {
		t.Errorf("Expected testing. Got: %v", string(p.Source()))
	}

	// We modify the ValueId in 50 PLs. So now PopulateShard should only return these after checking the Checksum.
	writePLs(t, 50, 5, ps)
	count, err = w1.PopulateShard(context.Background(), pool, 0)
	if err != nil {
		t.Fatal(err)
	}
	if count != 50 {
		t.Errorf("Expected PopulateShard to return %v k-v pairs. Got: %v", 50, count)
	}
}

func TestJoinCluster(t *testing.T) {
	// initNode(1, "localhost:12345")
	// n1 := GetNode()
	// n1.StartNode("1:localhost:12345")

	// initNode(2, "localhost:12346")
	// n2 := GetNode()
	// n2.StartNode("")
	// thisNode = n1
}

func TestGenerateGroup(t *testing.T) {
	dir, err := ioutil.TempDir("", "store3")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	posting.Init()

	writePLs(t, 100, 2, ps)
	ws := NewState(ps, nil, 0, 1)
	data, err := ws.generateGroup(0)
	if err != nil {
		t.Error(err)
	}
	t.Logf("Size of data: %v", len(data))

	var g task.GroupKeys
	uo := flatbuffers.GetUOffsetT(data)
	t.Logf("Found offset: %v", uo)
	g.Init(data, uo)

	if g.KeysLength() != 100 {
		t.Errorf("There should be 100 keys. Found: %v", g.KeysLength())
		t.Fail()
	}
	for i := 0; i < 100; i++ {
		var k task.KC
		if ok := g.Keys(&k, i); !ok {
			t.Errorf("Unable to parse key at index: %v", i)
		}
		expected := fmt.Sprintf("%03d", i)
		found := string(k.KeyBytes())
		if expected != found {
			t.Errorf("Key expected:[%q], found:[%q]", expected, found)
		}
		t.Logf("Checksum: %q", k.ChecksumBytes())
	}
}
