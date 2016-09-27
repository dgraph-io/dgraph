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
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	flatbuffers "github.com/google/flatbuffers/go"
)

func checkShard(ps *store.Store) (int, []byte) {
	it := ps.NewIterator()
	defer it.Close()

	count := 0
	var val []byte
	prefix := []byte("test")
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		count++
		val = it.Value().Data()
	}
	return count, val
}

/*
func TestPopulateShard(t *testing.T) {
	var err error
	addrs := []string{":12345", ":12346"}

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

	// Batch writing dummy key value pairs which will be transferred to other
	// instance.
	wb := ps.NewWriteBatch()
	for i := 0; i < 100; i++ {
		wb.Put([]byte(fmt.Sprintf("test|%d", i)), []byte("test"))
	}
	if err := ps.WriteBatch(wb); err != nil {
		log.Fatal(err)
	}

	w := NewState(ps, nil, 0, 2)
	SetWorkerState(w)
	go w.Connect(addrs, ":12345")

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
	SetWorkerState(w1)
	go w1.Connect(addrs, ":12346")

	// Wait for workers to be initialized and connected.
	time.Sleep(5 * time.Second)

	// Since PredicateData reads from the global variable wo, we change it to w.
	SetWorkerState(w)
	pool := w.GetPool(0)
	if err := w1.PopulateShard(context.Background(), pool, 0); err != nil {
		t.Fatal(err)
	}

	// Getting count on number of keys written to posting list store on instance 1.
	count, val := checkShard(ps1)
	if count != 100 {
		t.Fatalf("Expected %d key value pairs. Got : %d", 100, count)
	}
	if string(val) != "test" {
		t.Fatalf("Expected last value %s. Got : %s", "test", string(val))
	}
}
*/

func TestGenerateGroup(t *testing.T) {
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

	// Batch writing dummy key value pairs which will be transferred to other
	// instance.
	for i := 0; i < 100; i++ {
		k := fmt.Sprintf("%03d", i)
		t.Logf("key: %v", k)
		list, _ := posting.GetOrCreate([]byte(k), ps)

		de := x.DirectedEdge{
			ValueId:   2,
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

	ws := NewState(ps, nil, 0, 1)
	data, err := ws.generateGroup(0)
	if err != nil {
		t.Error(err)
	}
	t.Logf("Size of data: %v", len(data))

	var g task.Group
	uo := flatbuffers.GetUOffsetT(data)
	t.Logf("Found offset: %v", uo)
	g.Init(data, uo)

	if g.KeysLength() != 100 {
		t.Errorf("There should be 100 keys. Found: %v", g.KeysLength())
		t.Fail()
	}
	for i := 0; i < 100; i++ {
		var k task.KT
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
