package main

import (
	"bytes"
	"flag"
	"fmt"
	"strconv"

	"github.com/Sirupsen/logrus"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/rdb"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("dlist")

var dir = flag.String("dir", "", "Directory containing posting lists")
var uid = flag.String("uid", "", "Get posting list for uid")
var attr = flag.String("attr", "", "Get posting list for attribute")
var count = flag.Bool("count", false, "Only output number of results."+
	" Useful for range scanning with attribute.")

func output(val []byte) {
	var pl types.PostingList
	x.Check(pl.Unmarshal(val))
	fmt.Printf("Found posting list of length: %v\n", len(pl.Postings))
	for i, p := range pl.Postings {
		// TODO: This needs to be modified to account for type system.
		fmt.Printf("[%v] Uid: [%#x] Value: [%s]\n",
			i, p.Uid, string(p.Value))
	}
}

func scanOverAttr(db *rdb.DB) {
	ro := rdb.NewDefaultReadOptions()
	ro.SetFillCache(false)

	prefix := []byte(*attr)
	itr := db.NewIterator(ro)
	itr.Seek(prefix)

	num := 0
	for itr = itr; itr.Valid(); itr.Next() {
		if !bytes.HasPrefix(itr.Key().Data(), prefix) {
			break
		}
		if !*count {
			fmt.Printf("\nkey: %#x\n", itr.Key())
			output(itr.Value().Data())
		}
		num += 1
	}
	if err := itr.Err(); err != nil {
		glog.WithError(err).Fatal("While iterating")
	}
	fmt.Printf("Number of keys found: %v\n", num)
}

func main() {
	x.Init()
	logrus.SetLevel(logrus.ErrorLevel)

	opt := rdb.NewDefaultOptions()
	db, err := rdb.OpenDb(opt, *dir)
	defer db.Close()

	var key []byte
	if len(*uid) > 0 && len(*attr) > 0 {
		u, rerr := strconv.ParseUint(*uid, 0, 64)
		if rerr != nil {
			glog.WithError(rerr).Fatal("While parsing uid")
		}
		key = posting.Key(u, *attr)

	} else if len(*attr) > 0 {
		scanOverAttr(db)
		return

	} else if len(*uid) > 0 {
		u, rerr := strconv.ParseUint(*uid, 0, 64)
		if rerr != nil {
			glog.WithError(rerr).Fatal("While parsing uid")
		}
		key = posting.Key(u, "_xid_")

	} else {
		glog.Fatal("Invalid request.")
	}
	fmt.Printf("key: %#x\n", key)

	ropt := rdb.NewDefaultReadOptions()
	val, err := db.Get(ropt, key)
	if err != nil {
		glog.WithError(err).Fatal("Unable to get key")
	}
	if val.Size() == 0 {
		glog.Fatal("Unable to find posting list")
	}
	output(val.Data())
}
