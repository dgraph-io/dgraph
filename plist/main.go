/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package triple

import (
	"fmt"
	"io/ioutil"

	"github.com/Sirupsen/logrus"
	"github.com/google/flatbuffers/go"
	"github.com/manishrjain/dgraph/types"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var log = logrus.WithField("package", "plist")

type Triple struct {
	Entity    uint64
	EntityEid string
	Attribute string
	Value     interface{}
	ValueId   uint64
	// Source    string
	// Timestamp time.Time
}

func AddToList(t Triple) {

}

var ldb *leveldb.DB

func main() {
	path, err := ioutil.TempDir("", "dgraphldb_")
	if err != nil {
		log.Fatal(err)
		return
	}
	opt := &opt.Options{
		Filter: filter.NewBloomFilter(10),
	}
	var err error
	ldb, err := leveldb.OpenFile(path, opt)
	if err != nil {
		log.Fatal(err)
		return
	}
	fmt.Println("Using path", path)

	batch := new(leveldb.Batch)
	b := flatbuffers.NewBuilder(0)

	types.PostingListStartIdsVector(b, 2)
	b.PlaceUint64(5)
	b.PlaceUint64(2)
	vec := b.EndVector(2)

	types.PostingListStart(b)
	types.PostingListAddIds(b, vec)
	oe := types.PostingListEnd(b)
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

	plist := types.GetRootAsPostingList(val, 0)
	fmt.Println("buffer.uid id length =", plist.IdsLength())
	for i := 0; i < plist.IdsLength(); i++ {
		fmt.Printf("[%d] [%d]\n", i, plist.Ids(i))
	}
}
