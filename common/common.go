package common

// Concat concatenates two byte arrays
// used instead of append to prevent modifying the original byte array
func Concat(s1 []byte, s2 ...byte) []byte {
	r := make([]byte, len(s1)+len(s2))
	copy(r, s1)
	copy(r[len(s1):], s2)
	return r
}

// Uint16ToBytes converts a uint16 into a 2-byte slice
func Uint16ToBytes(in uint16) (out []byte) {
	out = make([]byte, 2)
	out[0] = byte(in & 0x00ff)
	out[1] = byte(in >> 8 & 0x00ff)
	return out
}