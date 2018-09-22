// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"log"
	"os"
	"syscall"
	"unsafe"
)

func mmapFile(f *os.File) mmapData {
	st, err := f.Stat()
	if err != nil {
		log.Fatal(err)
	}
	size := st.Size()
	if int64(int(size+4095)) != size+4095 {
		log.Fatalf("%s: too large for mmap", f.Name())
	}
	if size == 0 {
		return mmapData{f, nil}
	}
	h, err := syscall.CreateFileMapping(syscall.Handle(f.Fd()), nil, syscall.PAGE_READONLY, uint32(size>>32), uint32(size), nil)
	if err != nil {
		log.Fatalf("CreateFileMapping %s: %v", f.Name(), err)
	}

	addr, err := syscall.MapViewOfFile(h, syscall.FILE_MAP_READ, 0, 0, 0)
	if err != nil {
		log.Fatalf("MapViewOfFile %s: %v", f.Name(), err)
	}
	data := (*[1 << 30]byte)(unsafe.Pointer(addr))
	return mmapData{f, data[:size]}
}
