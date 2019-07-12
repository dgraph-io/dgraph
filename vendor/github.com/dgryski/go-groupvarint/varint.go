package groupvarint

func Encode1(dst []byte, n uint32) []byte {
	var offs int

	for n >= 0x80 {
		b := byte(n) | 0x80
		dst[offs] = b
		offs++
		n >>= 7
	}

	dst[offs] = byte(n)
	offs++

	return dst[:offs]
}

func Decode1(dst *uint32, src []byte) int {
	*dst = uint32(src[0]) &^ 0x80
	if src[0] < 128 {
		return 1
	}
	*dst = (uint32(src[1]&^0x80) << 7) | *dst
	if src[1] < 128 {
		return 2
	}
	*dst = (uint32(src[2]&^0x80) << 14) | *dst
	if src[2] < 128 {
		return 3
	}
	*dst = (uint32(src[3]&^0x80) << 21) | *dst
	if src[3] < 128 {
		return 4
	}
	*dst = (uint32(src[4]&^0x80) << 28) | *dst
	return 5
}
