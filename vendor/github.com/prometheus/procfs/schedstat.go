// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package procfs

import (
	"bufio"
	"errors"
	"os"
	"regexp"
	"strconv"
)

var (
	cpuLineRE  = regexp.MustCompile(`cpu(\d+) (\d+) (\d+) (\d+) (\d+) (\d+) (\d+) (\d+) (\d+) (\d+)`)
	procLineRE = regexp.MustCompile(`(\d+) (\d+) (\d+)`)
)

// Schedstat contains scheduler statistics from /proc/schedstats
//
// See
// https://www.kernel.org/doc/Documentation/scheduler/sched-stats.txt
// for a detailed description of what these numbers mean.
type Schedstat struct {
	CPUs []*SchedstatCPU
}

// SchedstatCPU contains the values from one "cpu<N>" line
type SchedstatCPU struct {
	CPUNum string

	RunningJiffies uint64
	WaitingJiffies uint64
	RunTimeslices  uint64
}

// ProcSchedstat contains the values from /proc/<pid>/schedstat
type ProcSchedstat struct {
	RunningJiffies uint64
	WaitingJiffies uint64
	RunTimeslices  uint64
}

func (fs FS) Schedstat() (*Schedstat, error) {
	file, err := os.Open(fs.proc.Path("schedstat"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stats := &Schedstat{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		match := cpuLineRE.FindStringSubmatch(scanner.Text())
		if match != nil {
			cpu := &SchedstatCPU{}
			cpu.CPUNum = match[1]

			cpu.RunningJiffies, err = strconv.ParseUint(match[8], 10, 64)
			if err != nil {
				continue
			}

			cpu.WaitingJiffies, err = strconv.ParseUint(match[9], 10, 64)
			if err != nil {
				continue
			}

			cpu.RunTimeslices, err = strconv.ParseUint(match[10], 10, 64)
			if err != nil {
				continue
			}

			stats.CPUs = append(stats.CPUs, cpu)
		}
	}

	return stats, nil
}

func parseProcSchedstat(contents string) (stats ProcSchedstat, err error) {
	match := procLineRE.FindStringSubmatch(contents)

	if match != nil {
		stats.RunningJiffies, err = strconv.ParseUint(match[1], 10, 64)
		if err != nil {
			return
		}

		stats.WaitingJiffies, err = strconv.ParseUint(match[2], 10, 64)
		if err != nil {
			return
		}

		stats.RunTimeslices, err = strconv.ParseUint(match[3], 10, 64)
		return
	}

	err = errors.New("could not parse schedstat")
	return
}

func (stat *SchedstatCPU) RunningSeconds() float64 {
	return float64(stat.RunningJiffies) / userHZ
}

func (stat *SchedstatCPU) WaitingSeconds() float64 {
	return float64(stat.WaitingJiffies) / userHZ
}

func (stat *ProcSchedstat) RunningSeconds() float64 {
	return float64(stat.RunningJiffies) / userHZ
}

func (stat *ProcSchedstat) WaitingSeconds() float64 {
	return float64(stat.WaitingJiffies) / userHZ
}
