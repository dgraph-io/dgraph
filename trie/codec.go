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

// keyToNibbles turns bytes into nibbles
// does not rearrange the nibbles; assumes they are already ordered in LE
// if the last nibble is zero, it is removed and the length of the output is odd
func keyToNibbles(in []byte) []byte {
	if len(in) == 0 {
		return []byte{}
	} else if len(in) == 1 && in[0] == 0 {
		return []byte{0, 0}
	}

	l := len(in) * 2
	res := make([]byte, l)
	for i, b := range in {
		res[2*i] = b / 16
		res[2*i+1] = b % 16
	}

	if res[l-2] == 0 {
		res[l-2] = res[l-1]
		res = res[:l-1]
	}

	return res
}

// nibblesToKey turns a slice of nibbles w/ length k into a little endian byte array
// if the length of the input is odd, the result is [ in[1] in[0] | ... | 0000 in[k-1] ]
// otherwise, res = [ in[1] in[0] | ... | in[k-1] in[k-2] ]
func nibblesToKey(in []byte) (res []byte) {
	if len(in)%2 == 0 {
		res = make([]byte, len(in)/2)
		for i := 0; i < len(in); i += 2 {
			res[i/2] = (in[i] & 0xf) | (in[i+1] << 4 & 0xf0)
		}
	} else {
		res = make([]byte, len(in)/2+1)
		for i := 0; i < len(in); i += 2 {
			if i < len(in)-1 {
				res[i/2] = (in[i] & 0xf) | (in[i+1] << 4 & 0xf0)
			} else {
				res[i/2] = (in[i] & 0xf)
			}
		}
	}

	return res
}
