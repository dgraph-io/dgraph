package groupvarint

import (
	"encoding/binary"
	mathbits "math/bits"
)

func Encode4(dst []byte, src []uint32) []byte {

	var bits uint8
	var n, b uint32

	offs := uint32(1)

	n = src[0]
	binary.LittleEndian.PutUint32(dst[offs:], n)
	b = 3 - uint32(mathbits.LeadingZeros32(n|1)/8)
	bits |= byte(b)
	offs += b + 1

	n = src[1]
	binary.LittleEndian.PutUint32(dst[offs:], n)
	b = 3 - uint32(mathbits.LeadingZeros32(n|1)/8)
	bits |= byte(b) << 2
	offs += b + 1

	n = src[2]
	binary.LittleEndian.PutUint32(dst[offs:], n)
	b = 3 - uint32(mathbits.LeadingZeros32(n|1)/8)
	bits |= byte(b) << 4
	offs += b + 1

	n = src[3]
	binary.LittleEndian.PutUint32(dst[offs:], n)
	b = 3 - uint32(mathbits.LeadingZeros32(n|1)/8)
	bits |= byte(b) << 6
	offs += b + 1

	dst[0] = bits

	return dst[:offs]
}
