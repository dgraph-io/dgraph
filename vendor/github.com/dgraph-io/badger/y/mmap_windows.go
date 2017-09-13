// +build windows

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
	"os"
	"syscall"
	"unsafe"
)

func Mmap(fd *os.File, write bool, size int64) ([]byte, error) {
	protect := syscall.PAGE_READONLY
	access := syscall.FILE_MAP_READ

	if write {
		protect = syscall.PAGE_READWRITE
		access = syscall.FILE_MAP_WRITE
	}
	handler, err := syscall.CreateFileMapping(syscall.Handle(fd.Fd()), nil,
		uint32(protect), uint32(size>>32), uint32(size), nil)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(handler)

	mapData, err := syscall.MapViewOfFile(handler, uint32(access), 0, 0, 0)
	if err != nil {
		return nil, err
	}

	data := (*[1 << 30]byte)(unsafe.Pointer(mapData))[:size]
	return data, nil
}

func Munmap(b []byte) error {
	return syscall.UnmapViewOfFile(uintptr(unsafe.Pointer(&b[0])))
}

func Madvise(b []byte, readahead bool) error {
	// Do Nothing. We don’t care about this setting on Windows
	return nil
}
