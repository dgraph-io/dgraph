package main

import (
	"bufio"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgryski/go-farm"
	"github.com/stretchr/testify/require"

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

	posting.Init(ps)
	loader.Init(ps)

	var count uint64
	{
		f, err := os.Open("test_input")
		require.NoError(t, err)
		r := bufio.NewReader(f)
		count, err = loader.LoadEdges(r, 1, 2)
		require.NoError(t, err)
		t.Logf("count: %v", count)
		f.Close()
		posting.MergeLists(100)
	}

	require.EqualValues(t, farm.Fingerprint64([]byte("follows"))%2, 1)
	require.EqualValues(t, count, 4, "loader assignment not as expected")

	{
		f, err := os.Open("test_input")
		if err != nil {
			t.Error(err)
			t.Fail()
		}
		r := bufio.NewReader(f)
		count, err = loader.LoadEdges(r, 0, 2)
		t.Logf("count: %v", count)
		f.Close()
		posting.MergeLists(100)
	}

	require.EqualValues(t, farm.Fingerprint64([]byte("enemy"))%2, 0)
	require.EqualValues(t, count, 4, "loader assignment not as expected")
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}
