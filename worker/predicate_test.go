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
	"bytes"
	"context"
	"io/ioutil"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
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

func writePLs(t *testing.T, pred string, count int, vid uint64, ps *store.Store) {
	for i := 0; i < count; i++ {
		k := x.DataKey(pred, uint64(i))
		list, _ := posting.GetOrCreate(k)

		de := &task.DirectedEdge{
			ValueId: vid,
			Label:   "test",
			Op:      task.DirectedEdge_SET,
		}
		list.AddMutation(context.TODO(), de)
		if merged, err := list.CommitIfDirty(context.TODO()); err != nil {
			t.Errorf("While merging: %v", err)
		} else if !merged {
			t.Errorf("No merge happened")
		}
	}
}

// We define this function so that we have access to the server which we can
// close at the end of the test.
func newServer(port string) (*grpc.Server, net.Listener, error) {
	ln, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("While running server: %v", err)
		return nil, nil, err
	}
	log.Printf("Worker listening at address: %v", ln.Addr())

	s := grpc.NewServer()
	return s, ln, nil
}

func serve(s *grpc.Server, ln net.Listener) {
	RegisterWorkerServer(s, &grpcWorker{})
	s.Serve(ln)
}

/*
 TODO: Make PopulateShard work again!
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
	posting.Init(ps)

	writePLs(t, 100, 2, ps)

	s, ln, err := newServer(":12345")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	go serve(s, ln)

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

	s1, ln1, err := newServer(":12346")
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Stop()
	go serve(s1, ln1)

	pool := newPool("localhost:12345", 5)
	_, err = populateShard(context.Background(), pool, 0)
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

	l, _ := posting.GetOrCreate(k)
	if l.Length(0) != 1 {
		t.Error("Unable to find added elements in posting list")
	}
	var found bool
	l.Iterate(0, func(p *types.Posting) bool {
		if p.Uid() != 2 {
			t.Errorf("Expected 2. Got: %v", p.Uid())
		}
		if string(p.Source()) != "test" {
			t.Errorf("Expected testing. Got: %v", string(p.Source()))
		}
		found = true
		return false
	})

	if !found {
		t.Error("Unable to retrieve posting at 1st iter")
		t.Fail()
	}

	// We modify the ValueId in 50 PLs. So now PopulateShard should only return
	// these after checking the Checksum.
	writePLs(t, 50, 5, ps)
	count, err = populateShard(context.Background(), pool, 0)
	if err != nil {
		t.Fatal(err)
	}
	if count != 50 {
		t.Errorf("Expected PopulateShard to return %v k-v pairs. Got: %v", 50, count)
	}
}


func TestJoinCluster(t *testing.T) {
	// Requires adding functions around group(). So waiting for RAFT code to stabilize a bit.
	t.Skip()
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
	posting.Init(ps)

	writePLs(t, 100, 2, ps)

	s, ln, err := newServer(":12345")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	go serve(s, ln)

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

	s1, ln1, err := newServer(":12346")
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Stop()
	go serve(s1, ln1)

	// Getting count on number of keys written to posting list store on instance 1.
	count, k := checkShard(ps1)
	if count != 100 {
		t.Fatalf("Expected %d key value pairs. Got : %d", 100, count)
	}
	if string(k) != "099" {
		t.Fatalf("Expected key to be: %v. Got %v", "099", string(k))
	}
}
*/

func TestGenerateGroup(t *testing.T) {
	dir, err := ioutil.TempDir("", "store3")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	r := bytes.NewReader([]byte("default: fp % 3"))
	require.NoError(t, group.ParseConfig(r), "Unable to parse config.")

	ps, err := store.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	posting.Init(ps)
	Init(ps)

	require.Equal(t, uint32(0), group.BelongsTo("pred0"))
	writePLs(t, "pred0", 33, 1, ps)

	require.Equal(t, uint32(1), group.BelongsTo("p1"))
	writePLs(t, "p1", 34, 1, ps)

	require.Equal(t, uint32(2), group.BelongsTo("pr2"))
	writePLs(t, "pr2", 35, 1, ps)
	time.Sleep(time.Second)

	g, err := generateGroup(0)
	if err != nil {
		t.Error(err)
	}
	require.Equal(t, 33, len(g.Keys))
	for i, k := range g.Keys {
		require.Equal(t, x.DataKey("pred0", uint64(i)), k.Key)
	}

	g, err = generateGroup(1)
	if err != nil {
		t.Error(err)
	}
	require.Equal(t, 34, len(g.Keys))
	for i, k := range g.Keys {
		require.Equal(t, x.DataKey("p1", uint64(i)), k.Key)
	}

	g, err = generateGroup(2)
	if err != nil {
		t.Error(err)
	}
	require.Equal(t, 35, len(g.Keys))
	for i, k := range g.Keys {
		require.Equal(t, x.DataKey("pr2", uint64(i)), k.Key)
	}
}
