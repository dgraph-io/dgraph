package dlist

import (
	"flag"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("dlist")

var dir = flag.String("dir", "", "Directory containing ")
var xid = flag.String("xid", "", "Get posting list for xid")
var uid = flag.String("uid", "", "Get posting list for uid")
var attr = flag.String("attr", "", "Get posting list for attribute")

func main() {
	flag.Parse()
	var s store.Store
	s.Init(*dir)
	defer s.Close()

	var key []byte
	if len(*uid) > 0 && len(*attr) > 0 {
		key = posting.Key(*uid, *attr)

	} else if len(*attr) > 0 {
		glog.Fatal("Not handling this yet.")

	} else if len(*uid) > 0 {
		key = posting.Key(*uid, "_xid_")
	}
}
