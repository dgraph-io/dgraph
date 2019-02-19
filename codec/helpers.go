package codec

// make byte array with length specified; used to test byte array encoding
func byteArray(length int) ([]byte) {
	b := make([]byte, length)
	for i := 0; i < length; i++ {
		b[i] = 0xff
	}
	return b
}