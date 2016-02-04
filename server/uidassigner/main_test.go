package main

import (
	"io/ioutil"
	"math"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgryski/go-farm"
)

func TestQuery(t *testing.T) {
	var instanceIdx uint64 = 0
	var numInstances uint64 = 2

	var mod uint64 = math.MaxUint64 / numInstances
	var minIdx uint64 = instanceIdx * mod

	logrus.SetLevel(logrus.DebugLevel)

	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)
	ps := new(store.Store)
	ps.Init(dir)
	clog := commit.NewLogger(dir, "mutations", 50<<20)
	clog.Init()
	defer clog.Close()

	posting.Init(ps, clog)

	list := []string{"alice", "bob", "mallory", "ash", "man", "dgraph"}

	for _, str := range list {
		if farm.Fingerprint64([]byte(str))%numInstances != instanceIdx {
			continue
		} else {
			uid, err := rdf.GetUid(str, instanceIdx, numInstances)
			if uid < minIdx || uid > minIdx+mod-1 {
				t.Error("Not the correct UID", err)
			}
			t.Logf("Correct UID")
		}
	}
}
