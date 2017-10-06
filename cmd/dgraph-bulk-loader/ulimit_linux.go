// +build linux

package main

import "syscall"

func queryMaxOpenFiles() (int, error) {
	var rl syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl)
	return int(rl.Cur), err
}
