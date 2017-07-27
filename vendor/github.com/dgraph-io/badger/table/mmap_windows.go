package table

import (
	"os"
	"syscall"
	"unsafe"
)

func mmap(fd *os.File, size int64) ([]byte, error) {
	handler, err := syscall.CreateFileMapping(syscall.Handle(fd.Fd()), nil,
		syscall.PAGE_READONLY, uint32(size>>32), uint32(size), nil)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(handler)

	mapData, err := syscall.MapViewOfFile(handler, syscall.FILE_MAP_READ, 0, 0, 0)
	if err != nil {
		return nil, err
	}

	data := (*[1 << 30]byte)(unsafe.Pointer(mapData))[:size]
	return data, nil
}

func munmap(b []byte) error {
	return syscall.UnmapViewOfFile(uintptr(unsafe.Pointer(&b[0])))
}
