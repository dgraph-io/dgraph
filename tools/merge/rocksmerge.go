/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"io/ioutil"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store/rocksdb"
	"github.com/dgraph-io/dgraph/x"
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

func compareValue(a, b interface{}) bool {
	var x, y types.Posting
	p1 := a.(*posting.List)
	if ok := p1.Get(&x, 0); !ok {
		glog.Fatal("While retrieving entry from posting list")
	}
	p2 := b.(*posting.List)
	if ok := p2.Get(&y, 0); !ok {
		glog.Fatal("While retrieving entry from posting list")
	}

	return x.Uid() == y.Uid()
}

func MergeFolders(mergePath, destPath string) {
	files, err := ioutil.ReadDir(mergePath)
	if err != nil {
		glog.Fatal("Cannot open stores directory")
	}

	var opt *rocksdb.Options
	var ropt *rocksdb.ReadOptions
	var wopt *rocksdb.WriteOptions
	opt = rocksdb.NewOptions()
	opt.SetCreateIfMissing(true)
	ropt = rocksdb.NewReadOptions()
	wopt = rocksdb.NewWriteOptions()
	wopt.SetSync(true)

	count := 0
	for range files {
		count++
	}

	if mergePath[len(mergePath)-1] == '/' {
		mergePath = mergePath[:len(mergePath)-1]
	}

	pq := make(PriorityQueue, count)
	var itVec []*rocksdb.Iterator
	for i, f := range files {
		curDb, err := rocksdb.Open(mergePath+"/"+f.Name(), opt)
		defer curDb.Close()
		if err != nil {
			glog.WithField("filepath", mergePath+"/"+f.Name()).
				Fatal("While opening store")
		}
		it := curDb.NewIterator(ropt)
		it.SeekToFirst()
		if !it.Valid() {
			itVec = append(itVec, it)
			glog.WithField("path", mergePath+"/"+f.Name()).Info("Store empty")
			continue
		}
		pq[i] = &Item{
			key:      it.Key(),
			value:    it.Value(),
			storeIdx: i,
		}
		itVec = append(itVec, it)
	}
	heap.Init(&pq)

	var db *rocksdb.DB
	db, err = rocksdb.Open(destPath, opt)
	defer db.Close()
	if err != nil {
		glog.WithField("filepath", destPath).
			Fatal("While opening store")
	}

	var lastKey, lastValue []byte
	for pq.Len() > 0 {
		top := heap.Pop(&pq).(*Item)

		if bytes.Compare(top.key, lastKey) == 0 {
			if !compareValue(top.value, lastValue) {
				glog.WithField("key", lastKey).
					Fatal("different value for same key")
			}
		}
		db.Put(wopt, top.key, top.value)
		lastKey = top.key
		lastValue = top.value

		itVec[top.storeIdx].Next()
		if !itVec[top.storeIdx].Valid() {
			continue
		}
		item := &Item{
			key:      itVec[top.storeIdx].Key(),
			value:    itVec[top.storeIdx].Value(),
			storeIdx: top.storeIdx,
		}
		heap.Push(&pq, item)
	}
}

func main() {
	flag.Parse()
	if len(*stores) == 0 {
		glog.Fatal("No Directory specified")
	}

	MergeFolders(*stores, *destinationDB)
}
