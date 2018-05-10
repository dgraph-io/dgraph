// +build !windows

/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package bulk

import (
	"golang.org/x/sys/unix"
)

func queryMaxOpenFiles() (int, error) {
	var rl unix.Rlimit
	err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rl)
	return int(rl.Cur), err
}
