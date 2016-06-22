/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package main

import (
	"bytes"
	"container/heap"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"path"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/x"
	rocksdb "github.com/tecbot/gorocksdb"
)

type Item struct {
	key, value []byte
	storeIdx   int // index of the store among the K stores
}

type PriorityQueue []*Item

var stores = flag.String("stores", "",
	"Root directory containing rocksDB directories")
var destinationDB = flag.String("dest", "",
	"Folder to store merged rocksDB")

var glog = x.Log("rocksmerge")
var pq PriorityQueue

func (pq PriorityQueue) Len() int { return len(pq) }
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}
func (pq PriorityQueue) Less(i, j int) bool {
	return bytes.Compare(pq[i].key, pq[j].key) <= 0
}
func (pq *PriorityQueue) Push(y interface{}) {
	*pq = append(*pq, y.(*Item))
}
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

func equalValue(a, b interface{}) bool {
	var x, y types.Posting
	p1 := a.(*posting.List)
	if ok := p1.Get(&x, 0); !ok {
		glog.Fatal("While retrieving entry from posting list")
	}
	p2 := b.(*posting.List)
	if ok := p2.Get(&y, 0); !ok {
		glog.Fatal("While retrieving entry from posting list")
	}

	aUid := x.Uid()
	bUid := y.Uid()

	return (x.ValueBytes() == nil && y.ValueBytes() == nil && (aUid == bUid)) ||
		((x.Uid() == math.MaxUint64 && y.Uid() == math.MaxUint64) &&
			bytes.Compare(x.ValueBytes(), y.ValueBytes()) == 0)
}

func mergeFolders(mergePath, destPath string) {
	dirList, err := ioutil.ReadDir(mergePath)
	if err != nil {
		glog.Fatal("Cannot open stores directory")
	}

	var opt *rocksdb.Options
	var ropt *rocksdb.ReadOptions
	var wopt *rocksdb.WriteOptions
	opt = rocksdb.NewDefaultOptions()
	opt.SetCreateIfMissing(true)
	ropt = rocksdb.NewDefaultReadOptions()
	wopt = rocksdb.NewDefaultWriteOptions()
	wopt.SetSync(false)
	wb := rocksdb.NewWriteBatch()

	pq = make(PriorityQueue, 0)
	heap.Init(&pq)
	var storeIters []*rocksdb.Iterator
	for i, dir := range dirList {
		mPath := path.Join(mergePath, dir.Name())
		curDb, err := rocksdb.OpenDb(opt, mPath)
		defer curDb.Close()
		if err != nil {
			glog.WithField("filepath", mPath).
				Fatal("While opening store")
		}
		it := curDb.NewIterator(ropt)
		it.SeekToFirst()
		if !it.Valid() {
			storeIters = append(storeIters, it)
			glog.WithField("path", mPath).Info("Store empty")
			continue
		}
		item := &Item{
			key:      it.Key().Data(),
			value:    it.Value().Data(),
			storeIdx: i,
		}
		heap.Push(&pq, item)
		storeIters = append(storeIters, it)
	}

	var db *rocksdb.DB
	db, err = rocksdb.OpenDb(opt, destPath)
	defer db.Close()
	if err != nil {
		glog.WithField("filepath", destPath).
			Fatal("While opening store")
	}

	var lastKey, lastValue []byte
	count := 0
	for pq.Len() > 0 {
		top := heap.Pop(&pq).(*Item)

		if bytes.Compare(top.key, lastKey) == 0 {
			if !equalValue(top.value, lastValue) {
				glog.WithField("key", lastKey).
					Fatal("different value for same key")
			}
		}
		wb.Put(top.key, top.value)
		count++
		if count%1000 == 0 {
			db.Write(wopt, wb)
			wb.Clear()
		}
		lastKey = top.key
		lastValue = top.value

		storeIters[top.storeIdx].Next()
		if !storeIters[top.storeIdx].Valid() {
			continue
		}
		item := &Item{
			key:      storeIters[top.storeIdx].Key().Data(),
			value:    storeIters[top.storeIdx].Value().Data(),
			storeIdx: top.storeIdx,
		}
		heap.Push(&pq, item)
	}

	db.Write(wopt, wb)
	wb.Destroy()

	fmt.Println("Count : ", count)
}

func main() {
	flag.Parse()
	if len(*stores) == 0 {
		glog.Fatal("No Directory specified")
	}

	mergeFolders(*stores, *destinationDB)
}
