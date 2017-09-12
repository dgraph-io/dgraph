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

// +build windows

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
