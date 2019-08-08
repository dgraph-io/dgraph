// MIT License

// Copyright (c) 2019 Ewan Chou

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package z

import (
	"unsafe"
)

// NanoTime returns the current time in nanoseconds from a monotonic clock.
//go:linkname NanoTime runtime.nanotime
func NanoTime() int64

// CPUTicks is a faster alternative to NanoTime to measure time duration.
//go:linkname CPUTicks runtime.cputicks
func CPUTicks() int64

type stringStruct struct {
	str unsafe.Pointer
	len int
}

//go:noescape
//go:linkname aeshash runtime.aeshash
func aeshash(p unsafe.Pointer, h, s uintptr) uintptr

// AESHash is the hash function used by map, it utilizes available hardware instructions.
// NOTE: The hash seed changes for every process. So, this cannot be used as a persistent hash.
func AESHash(data []byte) uint64 {
	ss := (*stringStruct)(unsafe.Pointer(&data))
	return uint64(aeshash(ss.str, 0, uintptr(ss.len)))
}

// AESHashString is the hash function used by map, it utilizes available hardware instructions.
// NOTE: The hash seed changes for every process. So, this cannot be used as a persistent hash.
func AESHashString(str string) uint64 {
	ss := (*stringStruct)(unsafe.Pointer(&str))
	return uint64(aeshash(ss.str, 0, uintptr(ss.len)))
}

// FastRand is a fast thread local random function.
//go:linkname FastRand runtime.fastrand
func FastRand() uint32
