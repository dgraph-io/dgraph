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

// keyToHex turns bytes into nibbles
func keyToHex(in []byte) []byte {
	l := len(in)*2 + 1
	res := make([]byte, l)
	for i, b := range in {
		res[2*i] = b / 16
		res[2*i+1] = b % 16
	}

	// last index of nibble array is set to 16 due to the way branches are indexed
	// branch at 0...15 points to possible children, branch at 16 is the value at the branch
	res[l-1] = 16
	return res
}
