/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"math/rand"
	"runtime"
	"time"

	"github.com/dgraph-io/dgraph/dgraph/cmd"
	"github.com/dustin/go-humanize"
	"github.com/golang/glog"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)

	// Make sure the garbage collector is run periodically.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		minDiff := uint64(5e8)
		var lastMs runtime.MemStats
		var lastNum uint32
		var ms runtime.MemStats

		for range ticker.C {
			runtime.ReadMemStats(&ms)
			var diff uint64
			if ms.HeapAlloc > lastMs.HeapAlloc {
				diff = ms.HeapAlloc - lastMs.HeapAlloc
			} else {
				diff = lastMs.HeapAlloc - ms.HeapAlloc
			}

			if ms.NumGC >= lastNum {
				// GC was already run by the Go runtime. No need to run it again.
				lastNum = ms.NumGC
			} else if diff < minDiff {
				// Do not run the GC if the allocated memory has not shrunk or expanded by
				// more than 0.5GB since the last time the memory stats were collected.
				lastNum = ms.NumGC
			} else {
				runtime.GC()
				glog.V(2).Infof("GC: %d. InUse: %s. Idle: %s\n", ms.NumGC,
					humanize.Bytes(ms.HeapInuse),
					humanize.Bytes(ms.HeapIdle-ms.HeapReleased))
				lastNum = ms.NumGC + 1
			}
			lastMs = ms
		}
	}()

	cmd.Execute()
}
