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
	"runtime"
	"strconv"
	"strings"
	"time"
)

const redlineMemAvailKb = 1024 * 1024 // 1 GB

// NOTE: function does not return
func MonitorMemory() {
	for {
		time.Sleep(5 * time.Second)

		file, err := os.Open("/proc/meminfo")
		if err != nil {
			continue
		}

		var memAvailKb int
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemAvailable:") {
				memAvailKb, _ = strconv.Atoi(strings.Fields(line)[1])
			}
		}
		_ = file.Close()

		if memAvailKb > 0 && memAvailKb <= redlineMemAvailKb {
			fmt.Fprintf(os.Stderr, "Available memory low. Attempting to free some.")
			runtime.GC()
		}
	}
}
