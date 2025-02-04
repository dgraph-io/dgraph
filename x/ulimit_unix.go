//go:build !windows
// +build !windows

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"golang.org/x/sys/unix"
)

func QueryMaxOpenFiles() (int, error) {
	var rl unix.Rlimit
	err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rl)
	return int(rl.Cur), err
}
