package main

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/google/flatbuffers/go"
	"github.com/manishrjain/dgraph/types"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var log = logrus.WithField("package", "plist")

type Triple struct {
	Entity    string
	Attribute string
	Value     interface{}
	ValueId   string
	Source    string
	Timestamp time.Time
}

/*
func addTriple(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		x.SetStatus(w, x.E_INVALID_METHOD, "Should be POST")
		return
	}

	var t Triple
	if ok := x.ParseRequest(w, r, &t); !ok {
		return
	}

	log.Debug(t)
}
*/

func main() {
	path, err := ioutil.TempDir("", "dgraphldb_")
	if err != nil {
		log.Fatal(err)
		return
	}
	opt := &opt.Options{
		Filter: filter.NewBloomFilter(10),
	}
	db, err := leveldb.OpenFile(path, opt)
	if err != nil {
		log.Fatal(err)
		return
	}
	fmt.Println("Using path", path)

	batch := new(leveldb.Batch)
	b := flatbuffers.NewBuilder(0)
	oi := b.CreateString("mrjn is a smart kid")
	on := b.CreateString("His name is jain")
	types.UidStart(b)
	types.UidAddId(b, oi)
	types.UidAddName(b, on)
	oe := types.UidEnd(b)
	b.Finish(oe)
	fmt.Println("Value byte size:", len(b.Bytes))

	key := "Some long id"
	batch.Put([]byte(key), b.Bytes[b.Head():])
	if err := db.Write(batch, nil); err != nil {
		log.Fatal(err)
		return
	}
	fmt.Println("Wrote key value out to leveldb. Reading back")
	if err := db.Close(); err != nil {
		log.Fatal(err)
		return
	}

	db, err = leveldb.OpenFile(path, opt)
	if err != nil {
		log.Fatal(err)
		return
	}

	val, err := db.Get([]byte(key), nil)
	if err != nil {
		log.Fatal(err)
		return
	}
	fmt.Println("Value byte size from Leveldb:", len(val))

	uid := types.GetRootAsUid(val, 0)
	fmt.Println("buffer.uid id =", string(uid.Id()))
	fmt.Println("buffer.uid name =", string(uid.Name()))
	// http.HandleFunc("/add", addTriple)
	// http.ListenAndServe(":8080", nil)

}
