/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"runtime"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang/glog"

	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/hypermodeinc/dgraph/v25/dgraph/cmd"
)

func main() {
	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)

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
		var lastAlloc uint64

		for range ticker.C {
			// Read Jemalloc stats first. Print if there's a big difference.
			z.ReadMemStats(&js)
			if diff := absDiff(uint64(z.NumAllocBytes()), lastAlloc); diff > 1<<30 {
				glog.V(2).Infof("NumAllocBytes: %s jemalloc: Active %s Allocated: %s"+
					" Resident: %s Retained: %s\n",
					humanize.IBytes(uint64(z.NumAllocBytes())),
					humanize.IBytes(js.Active), humanize.IBytes(js.Allocated),
					humanize.IBytes(js.Resident), humanize.IBytes(js.Retained))
				lastAlloc = uint64(z.NumAllocBytes())
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
	ticker.Stop()

	glog.V(2).Infof("Num Allocated Bytes at program end: %d", z.NumAllocBytes())
	if z.NumAllocBytes() > 0 {
		glog.Warningf("MEMORY LEAK detected of size: %s\n",
			humanize.Bytes(uint64(z.NumAllocBytes())))
		glog.Warningf("%s", z.Leaks())
	}
}
