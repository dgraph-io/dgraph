/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package x

import (
	"bufio"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const redlineMemAvailKb = 4 * 1024 * 1024 // 4 GB

// NOTE: function does not return
func MonitorMemory() {
	var counter int
	for {
		time.Sleep(1 * time.Second)
		counter += 1

		file, err := os.Open("/proc/meminfo")
		if err != nil {
			continue
		}

		var memTotalKb, memFreeKb, memAvailKb int
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				memTotalKb, _ = strconv.Atoi(strings.Fields(line)[1])
			} else if strings.HasPrefix(line, "MemFree:") {
				memFreeKb, _ = strconv.Atoi(strings.Fields(line)[1])
			} else if strings.HasPrefix(line, "MemAvailable:") {
				memAvailKb, _ = strconv.Atoi(strings.Fields(line)[1])
			}
		}
		_ = file.Close()

		fmt.Fprintf(os.Stderr, "MEM total_mb: %d, free_mb: %d, avail_mb: %d\n",
			memTotalKb / 1024, memFreeKb / 1024, memAvailKb / 1024)

		if memAvailKb > 0 {
			if counter%10 == 0 || memAvailKb <= redlineMemAvailKb {
				if memAvailKb <= redlineMemAvailKb {
					fmt.Fprintf(os.Stderr, "MEM attempting to release some memory\n")
					debug.FreeOSMemory()
				}
			}
		}
	}
}
