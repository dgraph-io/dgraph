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
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/dgraph-io/dgraph/dgraph/cmd"
	"github.com/dgraph-io/ristretto/z"
	"github.com/dustin/go-humanize"
	"github.com/golang/glog"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)
	fmt.Printf("Page Size: %d\n", os.Getpagesize())

	absDiff := func(a, b uint64) uint64 {
		if a > b {
			return a - b
		}
		return b - a
	}

	ticker := time.NewTicker(10 * time.Second)

	// Make sure the garbage collector is run periodically.
	go func() {
		minDiff := uint64(2 << 30)

		var ms runtime.MemStats
		var lastMs runtime.MemStats
		var lastNumGC uint32

		var js z.MemStats
		var lastJs z.MemStats

		for range ticker.C {
			// Read Jemalloc stats first. Print if there's a big difference.
			z.ReadMemStats(&js)
			if diff := absDiff(js.Active, lastJs.Active); diff > 256<<20 {
				glog.V(2).Infof("jemalloc: Active %s Allocated: %s Resident: %s Retained: %s\n",
					humanize.IBytes(js.Active), humanize.IBytes(js.Allocated),
					humanize.IBytes(js.Resident), humanize.IBytes(js.Retained))
				lastJs = js
				z.PrintAllocators()
			} else {
				// Don't update the lastJs here.
			}

			runtime.ReadMemStats(&ms)
			diff := absDiff(ms.HeapAlloc, lastMs.HeapAlloc)

			switch {
			case ms.NumGC > lastNumGC:
				// GC was already run by the Go runtime. No need to run it again.
				lastNumGC = ms.NumGC
				lastMs = ms

			case diff < minDiff:
				// Do not run the GC if the allocated memory has not shrunk or expanded by
				// more than 0.5GB since the last time the memory stats were collected.
				lastNumGC = ms.NumGC
				// Nobody ran a GC. Don't update lastMs.

			case ms.NumGC == lastNumGC:
				runtime.GC()
				glog.V(2).Infof("GC: %d. InUse: %s. Idle: %s. jemalloc: %s.\n", ms.NumGC,
					humanize.IBytes(ms.HeapInuse),
					humanize.IBytes(ms.HeapIdle-ms.HeapReleased),
					humanize.IBytes(js.Active))
				lastNumGC = ms.NumGC + 1
				lastMs = ms
			}
		}
	}()

	// Run the program.
	cmd.Execute()
	// Free up allocators from alloctors from allocatorPool.
	z.Done()

	ticker.Stop()
	fmt.Printf("Allocated Bytes at program end: %s\n", humanize.Bytes(uint64(z.NumAllocBytes())))
	if z.NumAllocBytes() > 0 {
		z.PrintLeaks()
	}
}
