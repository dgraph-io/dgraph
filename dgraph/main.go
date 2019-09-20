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
)

func main() {
	rand.Seed(time.Now().UnixNano())
	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)

	// Run GC periodically.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		var lastNum uint32
		var ms runtime.MemStats
		for range ticker.C {
			runtime.ReadMemStats(&ms)
			if ms.NumGC > lastNum {
				// GC was already run by the Go runtime. No need to run it again.
				lastNum = ms.NumGC
			} else {
				runtime.GC()
				lastNum = ms.NumGC + 1
			}
		}
	}()


	cmd.Execute()
}
