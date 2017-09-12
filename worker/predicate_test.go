/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

import (
	"context"
	"io/ioutil"
	"log"
	"net"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

func checkShard(ps *badger.KV) (int, []byte) {
	it := ps.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	count := 0
	var item *badger.KVItem
	for it.Rewind(); it.Valid(); it.Next() {
		item = it.Item()
		count++
	}
	if item == nil {
		return 0, nil
	}
	return count, item.Key()
}

// Hacky tests change laster
func writePLs(t *testing.T, pred string, startIdx int, count int, vid uint64) {
	for i := 0; i < count; i++ {
		k := x.DataKey(pred, uint64(i+startIdx))
		list := posting.Get(k, 1)

		de := &protos.DirectedEdge{
			ValueId: vid,
			Label:   "test",
			Op:      protos.DirectedEdge_SET,
		}
		list.AddMutation(context.TODO(), de)
		// If test fails, might be due to delay in syncing to disk
		// Warning, this would write to posting pstore
		if merged, err := list.SyncIfDirty(false); err != nil {
			t.Errorf("While merging: %v", err)
		} else if !merged {
			t.Errorf("No merge happened")
		}
	}
}

func deletePLs(t *testing.T, pred string, startIdx int, count int, ps *badger.KV) {
	for i := 0; i < count; i++ {
		k := x.DataKey(pred, uint64(i+startIdx))
		ps.Delete(k)
	}
}

func writeToBadger(t *testing.T, pred string, startIdx int, count int, ps *badger.KV) {
	for i := 0; i < count; i++ {
		k := x.DataKey(pred, uint64(i+startIdx))
		pl := new(protos.PostingList)
		data, err := pl.Marshal()
		if err != nil {
			t.Errorf("Error while marshing pl")
		}
		err = ps.Set(k, data, 0x00)
		if err != nil {
			t.Errorf("Error while writing to badger")
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
	x.Printf("Worker listening at address: %v", ln.Addr())

	s := grpc.NewServer()
	return s, ln, nil
}

func serve(s *grpc.Server, ln net.Listener) {
	protos.RegisterWorkerServer(s, &grpcWorker{})
	s.Serve(ln)
}

func TestPopulateShard(t *testing.T) {
	x.SetTestRun()
	group.ParseGroupConfig("")
	var err error
	dir, err := ioutil.TempDir("", "store0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir
	psLeader, err := badger.NewKV(&opt)
	if err != nil {
		t.Fatal(err)
	}
	defer psLeader.Close()
	posting.Init(psLeader)
	Init(psLeader)

	writePLs(t, "name", 0, 100, 2)

	dir1, err := ioutil.TempDir("", "store1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir1)

	opt = badger.DefaultOptions
	opt.Dir = dir1
	opt.ValueDir = dir1
	psFollower, err := badger.NewKV(&opt)
	if err != nil {
		t.Fatal(err)
	}
	defer psFollower.Close()

	s1, ln1, err := newServer(":12346")
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Stop()
	go serve(s1, ln1)

	pool, err := conn.NewPool("localhost:12346")
	if err != nil {
		t.Fatal(err)
	}
	_, err = populateShard(context.Background(), psFollower, pool, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Getting count on number of keys written to posting list store on instance 1.
	count, k := checkShard(psFollower)
	if count != 100 {
		t.Fatalf("Expected %d key value pairs. Got : %d", 100, count)
	}
	if x.Parse([]byte(k)).Uid != 99 {
		t.Fatalf("Expected key to be: %v. Got %v", "099", string(k))
	}

	l := posting.Get(k, 1)
	if l.Length(0) != 1 {
		t.Error("Unable to find added elements in posting list")
	}
	var found bool
	l.Iterate(0, func(p *protos.Posting) bool {
		if p.Uid != 2 {
			t.Errorf("Expected 2. Got: %v", p.Uid)
		}
		if string(p.Label) != "test" {
			t.Errorf("Expected testing. Got: %v", string(p.Label))
		}
		found = true
		return false
	})

	if !found {
		t.Error("Unable to retrieve posting at 1st iter")
		t.Fail()
	}

	// Everything is same in both stores, so no diff
	count, err = populateShard(context.Background(), psFollower, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("Expected PopulateShard to return %v k-v pairs. Got: %v", 0, count)
	}

	// We modify the ValueId in 40 PLs. So now PopulateShard should only return
	// these after checking the Checksum.
	writePLs(t, "name", 0, 40, 5)
	count, err = populateShard(context.Background(), psFollower, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 40 {
		t.Errorf("Expected PopulateShard to return %v k-v pairs. Got: %v", 40, count)
	}

	var item badger.KVItem
	err = psFollower.Get(x.DataKey("name", 1), &item)
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, item.Value(func(val []byte) {
		if len(val) == 0 {
			t.Errorf("value for uid 1 predicate name not found\n")
		}
	}))
	deletePLs(t, "name", 0, 5, psLeader) // delete in leader, should be deleted in follower also
	deletePLs(t, "name", 94, 5, psLeader)
	deletePLs(t, "name", 47, 5, psLeader)
	writePLs(t, "name2", 0, 10, 2)
	writePLs(t, "name", 100, 10, 2)               // Write extra in leader
	writeToBadger(t, "name", 110, 10, psFollower) // write extra in follower should be deleted
	count, k = checkShard(psFollower)
	if count != 110 {
		t.Fatalf("Expected %d key value pairs. Got : %d", 110, count)
	}
	if x.Parse([]byte(k)).Uid != 119 {
		t.Fatalf("Expected key to be: %v. Got %v", "119", string(k))
	}
	count, err = populateShard(context.Background(), psFollower, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 45 {
		t.Errorf("Expected PopulateShard to return %v k-v pairs. Got: %v", 45, count)
	}
	err = psFollower.Get(x.DataKey("name", 1), &item)
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, item.Value(func(val []byte) {
		if len(val) != 0 {
			t.Errorf("value for uid 1 predicate name shouldn't be present\n")
		}
	}))
	err = psFollower.Get(x.DataKey("name", 110), &item)
	if err != nil {
		t.Fatal(err)
	}
	require.NoError(t, item.Value(func(val []byte) {
		if len(val) != 0 {
			t.Errorf("value for uid 1 predicate name shouldn't be present\n")
		}
	}))

	// We have delete and added new pl's
	// Nothing is present for group2
	count, err = populateShard(context.Background(), psFollower, pool, 2)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("Expected PopulateShard to return %v k-v pairs. Got: %v", 0, count)
	}
}

/*
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

/*
 * TODO: Fix this when we find a way to mock grpc.
 *func TestGenerateGroup(t *testing.T) {
 *  dir, err := ioutil.TempDir("", "store3")
 *  if err != nil {
 *    t.Fatal(err)
 *  }
 *  defer os.RemoveAll(dir)
 *
 *  r := bytes.NewReader([]byte("default: fp % 3"))
 *  require.NoError(t, group.ParseConfig(r), "Unable to parse config.")
 *
 *  ps, err := store.NewStore(dir)
 *  if err != nil {
 *    t.Fatal(err)
 *  }
 *  defer ps.Close()
 *  posting.Init(ps)
 *  Init(ps)
 *
 *  require.Equal(t, uint32(0), group.BelongsTo("pred0"))
 *  writePLs(t, "pred0", 33, 1, ps)
 *
 *  require.Equal(t, uint32(1), group.BelongsTo("p1"))
 *  writePLs(t, "p1", 34, 1, ps)
 *
 *  require.Equal(t, uint32(2), group.BelongsTo("pr2"))
 *  writePLs(t, "pr2", 35, 1, ps)
 *  time.Sleep(time.Second)
 *
 *  g, err := generateGroup(0)
 *  if err != nil {
 *    t.Error(err)
 *  }
 *  require.Equal(t, 33, len(g.Keys))
 *  for i, k := range g.Keys {
 *    require.Equal(t, x.DataKey("pred0", uint64(i)), k.Key)
 *  }
 *
 *  g, err = generateGroup(1)
 *  if err != nil {
 *    t.Error(err)
 *  }
 *  require.Equal(t, 34, len(g.Keys))
 *  for i, k := range g.Keys {
 *    require.Equal(t, x.DataKey("p1", uint64(i)), k.Key)
 *  }
 *
 *  g, err = generateGroup(2)
 *  if err != nil {
 *    t.Error(err)
 *  }
 *  require.Equal(t, 35, len(g.Keys))
 *  for i, k := range g.Keys {
 *    require.Equal(t, x.DataKey("pr2", uint64(i)), k.Key)
 *  }
 *}
 */
