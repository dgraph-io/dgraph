package xidmap

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
)

// Opens a badger db and runs a a test on it.
func withDB(t *testing.T, test func(db *badger.DB)) {
	dir, err := ioutil.TempDir(".", "badger-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	opt := badger.LSMOnlyOptions
	opt.Dir = dir
	opt.ValueDir = dir

	db, err := badger.Open(opt)
	require.NoError(t, err)
	defer db.Close()

	test(db)
}

func TestXidmap(t *testing.T) {
	conn, err := x.SetupConnection(z.SockAddrZero, nil, false)
	require.NoError(t, err)
	require.NotNil(t, conn)

	withDB(t, func(db *badger.DB) {
		xidmap := New(conn, db)

		uida := xidmap.AssignUid("a")
		require.Equal(t, uida, xidmap.AssignUid("a"))

		uidb := xidmap.AssignUid("b")
		require.True(t, uida != uidb)
		require.Equal(t, uidb, xidmap.AssignUid("b"))

		to := xidmap.AllocateUid() + uint64(1e6+3)
		xidmap.BumpTo(to)
		uid := xidmap.AllocateUid() // Does not have to be above the bump.
		t.Logf("bump up to: %d. allocated: %d", to, uid)

		require.NoError(t, xidmap.Flush())
		xidmap = nil

		xidmap2 := New(conn, db)
		require.Equal(t, uida, xidmap2.AssignUid("a"))
		require.Equal(t, uidb, xidmap2.AssignUid("b"))
	})
}

func TestXidmapMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping because -short=true")
	}

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
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for range ticker.C {
			printMemory()
		}
	}()

	conn, err := x.SetupConnection(z.SockAddrZero, nil, false)
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
				if i > 50e6 {
					return
				}
				xidmap.AssignUid(fmt.Sprintf("xid-%d", i))
			}
		}()
	}
	wg.Wait()
	t.Logf("Time taken: %v", time.Since(start).Round(time.Millisecond))
}

func BenchmarkXidmap(b *testing.B) {
	conn, err := x.SetupConnection(z.SockAddrZero, nil, false)
	x.Check(err)
	x.AssertTrue(conn != nil)

	var counter uint64
	xidmap := New(conn, nil)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			xid := atomic.AddUint64(&counter, 1)
			xidmap.AssignUid(fmt.Sprintf("xid-%d", xid))
		}
	})
}
