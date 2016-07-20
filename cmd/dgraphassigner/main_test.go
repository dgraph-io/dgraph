package main

import (
	"io/ioutil"
	"math"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgryski/go-farm"
)

func TestQuery(t *testing.T) {
	var numInstances uint64 = 2
	mod := math.MaxUint64 / numInstances
	minIdx0 := 0 * mod
	minIdx1 := 1 * mod

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
	stores := []*store.Store{ps}
	posting.Init(clog, stores)

	uid.Init(ps)

	list := []string{"alice", "bob", "mallory", "ash", "man", "dgraph"}
	for _, str := range list {
		if farm.Fingerprint64([]byte(str))%numInstances == 0 {
			u, err := uid.GetOrAssign(str, 0, numInstances)
			if u < minIdx0 || u > minIdx0+mod-1 {
				t.Error("Not the correct UID", err)
			}
			t.Logf("Instance-0 Correct UID", str, u)

		} else {
			u, err := uid.GetOrAssign(str, 1, numInstances)
			if u < minIdx1 || u > minIdx1+mod-1 {
				t.Error("Not the correct UID", err)
			}
			t.Logf("Instance-1 Correct UID", str, u)
		}
	}
}
