// +build darwin dragonfly freebsd linux netbsd openbsd solaris

package table

import (
	"os"

	"golang.org/x/sys/unix"
)

func mmap(fd *os.File, size int64) ([]byte, error) {
	return unix.Mmap(int(fd.Fd()), 0, int(size), unix.PROT_READ, unix.MAP_SHARED)
}

func munmap(b []byte) (err error) {
	return unix.Munmap(b)
}
