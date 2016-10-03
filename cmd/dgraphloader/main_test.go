package main

import (
	"bufio"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
)

func TestQuery(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Fail()
	}
	defer os.RemoveAll(dir)

	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		t.Fail()
	}

	posting.Init()

	uid.Init(ps)
	loader.Init(ps)
	posting.InitIndex(ps)

	var count uint64
	{
		f, err := os.Open("test_input")
		if err != nil {
			t.Error(err)
			t.Fail()
		}
		r := bufio.NewReader(f)
		count, err = loader.LoadEdges(r, 1, 2)
		t.Logf("count: %v", count)
		f.Close()
		posting.MergeLists(100)
	}

	if farm.Fingerprint64([]byte("follows"))%2 != 1 {
		t.Error("Expected fp to be 1.")
		t.Fail()
	}
	if count != 4 {
		t.Error("loader assignment not as expected")
	}

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

	if farm.Fingerprint64([]byte("enemy"))%2 != 0 {
		t.Error("Expected fp to be 0.")
		t.Fail()
	}
	if count != 4 {
		t.Error("loader assignment not as expected")
	}
}
