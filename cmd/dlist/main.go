package main

import (
	"bytes"
	"flag"
	"fmt"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
	rocksdb "github.com/tecbot/gorocksdb"
)

var glog = x.Log("dlist")

var dir = flag.String("dir", "", "Directory containing ")
var xid = flag.String("xid", "", "Get posting list for xid")
var suid = flag.String("uid", "", "Get posting list for uid")
var attr = flag.String("attr", "", "Get posting list for attribute")
var count = flag.Bool("count", false, "Only output number of results."+
	" Useful for range scanning with attribute.")

func output(val []byte) {
	pl := types.GetRootAsPostingList(val, 0)
	fmt.Printf("Found posting list of length: %v\n", pl.PostingsLength())
	var p types.Posting
	for i := 0; i < pl.PostingsLength(); i++ {
		if !pl.Postings(&p, i) {
			glog.WithField("i", i).Fatal("Unable to get posting")
		}
		fmt.Printf("[%v] Uid: [%#x] Value: [%s]\n",
			i, p.Uid(), string(p.ValueBytes()))
	}
}

func scanOverAttr(db *rocksdb.DB) {
	ro := rocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)

	prefix := []byte(*attr)
	itr := db.NewIterator(ro)
	itr.Seek(prefix)

	num := 0
	for ; itr.Valid(); itr.Next() {
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
	logrus.SetLevel(logrus.ErrorLevel)

	flag.Parse()
	opt := rocksdb.NewDefaultOptions()
	db, err := rocksdb.OpenDb(opt, *dir)
	defer db.Close()

	var key []byte
	if len(*suid) > 0 && len(*attr) > 0 {
		u, rerr := strconv.ParseUint(*suid, 0, 64)
		if rerr != nil {
			glog.WithError(rerr).Fatal("While parsing uid")
		}
		key = posting.Key(u, *attr)

	} else if len(*attr) > 0 {
		scanOverAttr(db)
		return

	} else if len(*suid) > 0 {
		u, rerr := strconv.ParseUint(*suid, 0, 64)
		if rerr != nil {
			glog.WithError(rerr).Fatal("While parsing uid")
		}
		key = posting.Key(u, "_xid_")

	} else if len(*xid) > 0 {
		key = uid.StringKey(*xid)

	} else {
		glog.Fatal("Invalid request.")
	}
	fmt.Printf("key: %#x\n", key)

	ropt := rocksdb.NewDefaultReadOptions()
	val, err := db.Get(ropt, key)
	if err != nil {
		glog.WithError(err).Fatal("Unable to get key")
	}
	if val.Size() == 0 {
		glog.Fatal("Unable to find posting list")
	}
	output(val.Data())
}
