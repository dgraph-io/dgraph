package xidmap

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/stretchr/testify/require"
)

// Opens a badger db and runs a a test on it.
func withDB(t *testing.T, test func(db *badger.DB)) {
	dir, err := ioutil.TempDir(".", "badger-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	opt := badger.LSMOnlyOptions(dir)
	db, err := badger.Open(opt)
	require.NoError(t, err)
	defer db.Close()

	test(db)
}

func TestXidmap(t *testing.T) {
	conn, err := x.SetupConnection(testutil.SockAddrZero, nil, false)
	require.NoError(t, err)
	require.NotNil(t, conn)

	withDB(t, func(db *badger.DB) {
		xidmap := New(conn, db)

		uida, isNew := xidmap.AssignUid("a")
		require.True(t, isNew)
		uidaNew, isNew := xidmap.AssignUid("a")
		require.Equal(t, uida, uidaNew)
		require.False(t, isNew)

		uidb, isNew := xidmap.AssignUid("b")
		require.True(t, uida != uidb)
		require.True(t, isNew)
		uidbnew, isNew := xidmap.AssignUid("b")
		require.Equal(t, uidb, uidbnew)
		require.False(t, isNew)

		to := xidmap.AllocateUid() + uint64(1e6+3)
		xidmap.BumpTo(to)
		uid := xidmap.AllocateUid() // Does not have to be above the bump.
		t.Logf("bump up to: %d. allocated: %d", to, uid)

		require.NoError(t, xidmap.Flush())
		xidmap = nil

		xidmap2 := New(conn, db)
		uida2, isNew := xidmap2.AssignUid("a")
		require.Equal(t, uida, uida2)
		require.False(t, isNew)
		uidb2, isNew := xidmap2.AssignUid("b")
		require.Equal(t, uidb, uidb2)
		require.False(t, isNew)
	})
}

func TestXidmapMemory(t *testing.T) {
	var loop uint32
	bToMb := func(b uint64) uint64 {
		return b / 1024 / 1024
	}
	printMemory := func() {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		// For info on each, see: https://golang.org/pkg/runtime/#MemStats
		fmt.Printf(" Heap = %v M", bToMb(m.HeapInuse))
		fmt.Printf(" Alloc = %v M", bToMb(m.Alloc))
		fmt.Printf(" Sys = %v M", bToMb(m.Sys))
		fmt.Printf(" Loop = %.2fM", float64(atomic.LoadUint32(&loop))/1e6)
		fmt.Printf(" NumGC = %v\n", m.NumGC)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			printMemory()
		}
	}()

	conn, err := x.SetupConnection(testutil.SockAddrZero, nil, false)
	require.NoError(t, err)
	require.NotNil(t, conn)

	xidmap := New(conn, nil)

	start := time.Now()
	var wg sync.WaitGroup
	for numGo := 0; numGo < 32; numGo++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := atomic.AddUint32(&loop, 1)
				if i > 10e6 {
					return
				}
				xidmap.AssignUid(fmt.Sprintf("xid-%d", i))
			}
		}()
	}
	wg.Wait()
	t.Logf("Time taken: %v", time.Since(start).Round(time.Millisecond))
}

// Benchmarks using Map
// BenchmarkXidmapWrites-32    	 4435590	       278 ns/op
// BenchmarkXidmapReads-32     	33248678	        34.1 ns/op
//
// Benchmarks using Trie
// BenchmarkXidmapWrites-32    	16202346	       375 ns/op
// BenchmarkXidmapReads-32     	139261450	        44.8 ns/op
func BenchmarkXidmapWrites(b *testing.B) {
	conn, err := x.SetupConnection(testutil.SockAddrZero, nil, false)
	if err != nil {
		b.Fatalf("Error setting up connection: %s", err.Error())
	}

	var counter int64
	xidmap := New(conn, nil)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			xid := atomic.AddInt64(&counter, 1)
			xidmap.AssignUid("xid-" + strconv.Itoa(int(xid)))
		}
	})
}

func BenchmarkXidmapReads(b *testing.B) {
	conn, err := x.SetupConnection(testutil.SockAddrZero, nil, false)
	if err != nil {
		b.Fatalf("Error setting up connection: %s", err.Error())
	}

	var N = 1000000
	xidmap := New(conn, nil)
	for i := 0; i < N; i++ {
		xidmap.AssignUid("xid-" + strconv.Itoa(i))
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			xid := int(z.FastRand()) % N
			xidmap.AssignUid("xid-" + strconv.Itoa(xid))
		}
	})
}
