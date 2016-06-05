package main

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/store/rocksdb"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgryski/go-farm"
)

func TestMergeFolders(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	rootDir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(rootDir)

	dir1, err := ioutil.TempDir(rootDir, "dir_")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir1)

	dir2, err := ioutil.TempDir(rootDir, "dir_")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir2)

	destDir, err := ioutil.TempDir("", "dest_")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(destDir)

	ps1 := new(store.Store)
	ps1.Init(dir1)

	ps2 := new(store.Store)
	ps2.Init(dir2)

	list := []string{"alice", "bob", "mallory", "ash", "man", "dgraph",
		"ash", "alice"}
	var numInstances uint64 = 2
	posting.Init(nil)
	uid.Init(ps1)
	loader.Init(nil, ps1)
	for _, str := range list {
		if farm.Fingerprint64([]byte(str))%numInstances == 0 {
			_, err := uid.GetOrAssign(str, 0, numInstances)
			if err != nil {
				t.Error("error while assigning uid")
			}
		}
	}
	uid.Init(ps2)
	loader.Init(nil, ps2)
	for _, str := range list {
		if farm.Fingerprint64([]byte(str))%numInstances == 1 {
			uid.Init(ps2)
			_, err := uid.GetOrAssign(str, 1, numInstances)
			if err != nil {
				t.Error("error while assigning uid")
			}

		}
	}
	posting.MergeLists(100)
	ps1.Close()
	ps2.Close()

	mergeFolders(rootDir, destDir)

	var opt *rocksdb.Options
	var ropt *rocksdb.ReadOptions
	opt = rocksdb.NewOptions()
	ropt = rocksdb.NewReadOptions()
	db, err := rocksdb.Open(destDir, opt)
	it := db.NewIterator(ropt)

	count := 0
	for it.SeekToFirst(); it.Valid(); it.Next() {
		count++
	}

	if count != 6 { // There are totally 6 unique strings
		t.Error("Not all the items have been assigned uid")
	}
}
