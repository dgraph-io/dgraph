package main

import (
	"bufio"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func TestQuery(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	require.NoError(t, err)
	defer ps.Close()

	posting.Init(ps)
	loader.Init(ps)
	require.NoError(t, group.ParseGroupConfig("tests/groups.conf"))

	var count uint64
	{
		f, err := os.Open("tests/test_input")
		require.NoError(t, err)
		r := bufio.NewReader(f)
		gm := map[uint32]bool{
			1: true,
		}
		count, err = loader.LoadEdges(r, gm)
		require.NoError(t, err)
		t.Logf("count: %v", count)
		f.Close()
		posting.MergeLists(100)
	}

	require.EqualValues(t, uint32(0), group.BelongsTo("enemy"))
	require.EqualValues(t, uint32(1), group.BelongsTo("follows"))
	require.EqualValues(t, uint64(4), count, "loader assignment not as expected")

	{
		f, err := os.Open("tests/test_input")
		require.NoError(t, err)
		r := bufio.NewReader(f)
		gm := map[uint32]bool{
			0: true,
		}
		count, err = loader.LoadEdges(r, gm)
		require.NoError(t, err)
		t.Logf("count: %v", count)
		f.Close()
		posting.MergeLists(100)
	}

	require.EqualValues(t, count, 4, "loader assignment not as expected")
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}
