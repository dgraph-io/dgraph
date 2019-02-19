package codec

// make byte array with length 1 << 6; used to test 2-byte length encoding
func byteArrayLen64() ([]byte) {
	b := make([]byte, 64)
	for i := 0; i < 64; i++ {
		b[i] = 0x0f
	}
	return b
}

// make byte array with length 1 << 14; used to test 4-byte length encoding
func byteArrayLen16384() ([]byte) {
	b := make([]byte, 16384)
	for i := 0; i < 16384; i++ {
		b[i] = 0xf0
	}
	return b
}