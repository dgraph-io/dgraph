// +build linux

/*
 * Copyright 2017 Dgraph Labs, Inc. and Contributors
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
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// ErrPurged is returned when a transaction tries to access an entry which
// has been purged.
var ErrPurged = errors.New("This version of key has been purged.")

func PunchHole(fd int, offset, len int64) error {
	return unix.Fallocate(fd, unix.FALLOC_FL_KEEP_SIZE|unix.FALLOC_FL_PUNCH_HOLE, offset, len)
}
