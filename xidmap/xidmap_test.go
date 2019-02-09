package xidmap

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync/atomic"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

// Opens a badger db and runs a a test on it.
func runTest(t *testing.T, test func(t *testing.T, xidmap *XidMap)) {
	dir, err := ioutil.TempDir(".", "badger-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	opt := badger.LSMOnlyOptions
	opt.Dir = dir
	opt.ValueDir = dir

	db, err := badger.Open(opt)
	require.NoError(t, err)
	defer db.Close()

	conn, err := x.SetupConnection("localhost:5080", nil, false)
	require.NoError(t, err)
	require.NotNil(t, conn)

	xidmap := New(db, conn)
	test(t, xidmap)
}

func TestXidmap(t *testing.T) {
	runTest(t, func(t *testing.T, xidmap *XidMap) {
		uida := xidmap.AssignUid("a")
		require.Equal(t, uida, xidmap.AssignUid("a"))

		uidb := xidmap.AssignUid("b")
		require.True(t, uida != uidb)
		require.Equal(t, uidb, xidmap.AssignUid("b"))
	})
}

func BenchmarkXidmap(b *testing.B) {
	dir, err := ioutil.TempDir(".", "badger-test")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.LSMOnlyOptions
	opt.Dir = dir
	opt.ValueDir = dir

	db, err := badger.Open(opt)
	x.Check(err)
	defer db.Close()

	conn, err := x.SetupConnection("localhost:5080", nil, false)
	x.Check(err)
	x.AssertTrue(conn != nil)

	var counter uint64
	xidmap := New(db, conn)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			xid := atomic.AddUint64(&counter, 1)
			xidmap.AssignUid(fmt.Sprintf("xid-%d", xid))
		}
	})
}
