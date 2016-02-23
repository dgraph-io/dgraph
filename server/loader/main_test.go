package main

import (
	"bufio"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgryski/go-farm"
)

func TestQuery(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	dir, err := ioutil.TempDir("", "storetest_")
	dir1, err1 := ioutil.TempDir("", "storetest1_")
	if err != nil || err1 != nil {
		t.Fail()
	}
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dir1)

	ps := new(store.Store)
	ps.Init(dir)

	ps1 := new(store.Store)
	ps1.Init(dir1)

	clog := commit.NewLogger(dir, "mutations", 50<<20)
	clog.Init()
	defer clog.Close()
	posting.Init(clog)

	uid.Init(ps)
	loader.Init(ps, ps1)

	f, err := os.Open("test_input")
	r := bufio.NewReader(f)
	count, err := loader.HandleRdfReader(r, 1, 2)
	t.Logf("count", count)

	posting.MergeLists(100)

	if farm.Fingerprint64([]byte("follows"))%2 == 1 {
		if count != 4 {
			t.Error("loader assignment not as expected")
		}
	}
}
