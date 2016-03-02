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
	"fmt"
	"io/ioutil"

	"github.com/dgraph-io/dgraph/store/rocksdb"
	"github.com/dgraph-io/dgraph/x"
)

type Item struct {
	key, value []byte
	itIdx      int
	index      int
}

type PriorityQueue []*Item

var stores = flag.String("stores", "", "Folder containing rocksDB directories")
var destinationDB = flag.String("destination", "",
	"Folder to store merged rocksDB")

var glog = x.Log("rocksmerge")

func (pq PriorityQueue) Len() int { return len(pq) }
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq PriorityQueue) Less(i, j int) bool {
	return (bytes.Compare(pq[i].key, pq[j].key) <= 0)
}
func (pq *PriorityQueue) Push(y interface{}) {
	n := len(*pq)
	item := y.(*Item)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

func main() {
	flag.Parse()
	if len(*stores) == 0 {
		glog.Fatal("No Directory specified")
	}
	files, err := ioutil.ReadDir(*stores)
	if err != nil {
		glog.Fatal("Cannot open stores directory")
	}

	var opt *rocksdb.Options
	var ropt *rocksdb.ReadOptions
	var wopt *rocksdb.WriteOptions
	opt = rocksdb.NewOptions()
	opt.SetCreateIfMissing(true)
	fp := rocksdb.NewBloomFilter(16)
	opt.SetFilterPolicy(fp)

	ropt = rocksdb.NewReadOptions()
	wopt = rocksdb.NewWriteOptions()
	wopt.SetSync(true)

	var itVec []*rocksdb.Iterator
	var db *rocksdb.DB
	var lastKey, lastValue []bytes
	count := 0
	for range files {
		count++
	}
	pq := make(PriorityQueue, count)

	i := 0
	for _, f := range files {
		curDb, err := rocksdb.Open(*stores+f.Name(), opt)
		if err != nil {
			glog.WithField("filepath", *stores+f.Name()).
				Fatal("While opening store")
		}
		it := curDb.NewIterator(ropt)
		it.SeekToFirst()
		if !it.Valid() {
			continue
		}
		fmt.Println(it.Key(), it.Value())
		pq[i] = &Item{
			key:   it.Key(),
			value: it.Value(),
			itIdx: i,
			index: i,
		}
		i++
		itVec = append(itVec, it)
	}
	heap.Init(&pq)

	db, err = rocksdb.Open(*destinationDB, opt)
	if err != nil {
		glog.WithField("filepath", *destinationDB).
			Fatal("While opening store")
	}

	fmt.Println(count)
	for pq.Len() > 0 {
		top := heap.Pop(&pq).(*Item)

		if bytes.Compare(top.key, lastKey) == 0 {
			if bytes.Compare(top.value, lastValue) != 0 {
				glog.Fatalf("different value for same key %s", lastKey)
			}
		} else {
			db.Put(wopt, top.key, top.value)
			fmt.Println(top.key, top.value)
			lastKey = top.key
			lastValue = top.value
		}

		itVec[top.itIdx].Next()
		if !itVec[top.itIdx].Valid() {
			continue
		}
		item := &Item{
			key:   itVec[top.itIdx].Key(),
			value: itVec[top.itIdx].Value(),
			itIdx: top.itIdx,
		}
		heap.Push(&pq, item)
	}
	fmt.Println(count)
	db.Close()
}
