// Copyright 2018 Tobias Klauser
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build dragonfly freebsd netbsd openbsd

package numcpus

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/unix"
)

func getKernelMax() (int, error) {
	return 0, fmt.Errorf("GetKernelMax is not supported on %s", runtime.GOOS)
}

func getOffline() (int, error) {
	return 0, fmt.Errorf("GetOffline is not supported on %s", runtime.GOOS)
}

func getOnline() (int, error) {
	var n uint32
	var err error
	if runtime.GOOS == "netbsd" {
		n, err = unix.SysctlUint32("hw.ncpuonline")
	} else {
		n, err = unix.SysctlUint32("hw.ncpu")
	}
	return int(n), err
}

func getPossible() (int, error) {
	n, err := unix.SysctlUint32("hw.ncpu")
	return int(n), err
}

func getPresent() (int, error) {
	n, err := unix.SysctlUint32("hw.ncpu")
	return int(n), err
}
