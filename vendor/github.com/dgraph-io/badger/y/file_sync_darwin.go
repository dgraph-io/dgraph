// +build darwin,!go1.12

/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package y

import (
	"os"
	"syscall"
)

// FileSync calls os.File.Sync with the right parameters.
// This function can be removed once we stop supporting Go 1.11
// on MacOS.
//
// More info: https://golang.org/issue/26650.
func FileSync(f *os.File) error {
	_, _, err := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_FULLFSYNC, 0)
	if err == 0 {
		return nil
	}
	return err
}
