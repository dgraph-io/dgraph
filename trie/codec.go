package trie

// keyEncodeByte swaps the two nibbles of a byte to result in 'LE'
func keyEncodeByte(b byte) byte {
	b1 := (uint(b) & 240) >> 4
	b2 := (uint(b) & 15) << 4

	return byte(b1 | b2)
}

// KeyEncode encodes the key by placing in in little endian nibble format
func KeyEncode(k []byte) []byte {
	result := make([]byte, len(k))
	// Encode each byte
	for i, b := range k {
		result[i] = keyEncodeByte(b)
	}
	return result
}
