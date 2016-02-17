package main

import (
	"io/ioutil"
	"math"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
)

func TestQuery(t *testing.T) {
	var numInstances uint64 = 2
	mod := math.MaxUint64 / numInstances
	minIdx0 := 0 * mod
	minIdx1 := 1 * mod

	logrus.SetLevel(logrus.DebugLevel)
	dir, err := ioutil.TempDir("", "storetest_")
	dir1, err1 := ioutil.TempDir("", "storetest1_")
	if err != nil || err1 != nil {
		t.Error(err)
		return
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

	list := []x.DirectedEdge{
		{1, "friend", 2},
		{2, "frined", 3},
		{1, "friend", 3},
	}

	for _, obj := range list {
		if farm.Fingerprint64([]byte(obj.Attribute))%numInstances == 0 {
			key := posting.Key(edge.Entity, edge.Attribute)
			plist := posting.GetOrCreate(key, ps1)
			plist.AddMutation(edge, posting.Set)
			if uid < minIdx0 || uid > minIdx0+mod-1 {
				t.Error("Not the correct UID", err)
			}
			t.Logf("Instance-0 Correct UID", str, uid)

		} else {
			key := posting.Key(edge.Entity, edge.Attribute)
			plist := posting.GetOrCreate(key, ps1)
			plist.AddMutation(edge, posting.Set)
			if uid < minIdx1 || uid > minIdx1+mod-1 {
				t.Error("Not the correct UID", err)
			}
			t.Logf("Instance-1 Correct UID", str, uid)
		}
	}
}
