// +build !windows

package main

import (
	"golang.org/x/sys/unix"
)

func queryMaxOpenFiles() (int, error) {
	var rl unix.Rlimit
	err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rl)
	return int(rl.Cur), err
}
