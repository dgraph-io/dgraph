// +build amd64 !noasm

package groupvarint

import (
	"encoding/binary"
)

var mask = [4]uint32{0xff, 0xffff, 0xffffff, 0xffffffff}

//func Decode4(dst []uint32, src []byte)

func Decode4(dst []uint32, src []byte) {
	// fmt.Println("Using go decoding")
	bits := src[0]
	src = src[1:]

	b := bits & 3
	n := binary.LittleEndian.Uint32(src) & mask[b]
	src = src[1+b:]
	bits >>= 2
	dst[0] = uint32(n)

	b = bits & 3
	n = binary.LittleEndian.Uint32(src) & mask[b]
	src = src[1+b:]
	bits >>= 2
	dst[1] = uint32(n)

	b = bits & 3
	n = binary.LittleEndian.Uint32(src) & mask[b]
	src = src[1+b:]
	bits >>= 2
	dst[2] = uint32(n)

	b = bits & 3
	n = binary.LittleEndian.Uint32(src) & mask[b]
	dst[3] = uint32(n)
}
