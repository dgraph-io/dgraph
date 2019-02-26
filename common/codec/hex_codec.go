package hexcodec

// Encode assumes its input is an array of nibbles (4bits), and produces Hex-Encoded output.
// HexEncoded: For PK = (k_1,...,k_n), Enc_hex(PK) :=
// (0, k_1 + k_2 * 16,...) for even length
// (k_1, k_2 + k_3 * 16,...) for odd length
func Encode(in []byte) []byte {
	res := make([]byte, (len(in)/2)+1)
	if len(in) == 1 { // Single byte
		res[0] = in[0]
	} else {
		resI := 1
		var i int

		if len(in)%2 == 1 { // Odd length
			res[0] = in[0]
			i = 1 // Skip first nibble
		} else { // Even length
			res[0] = 0x0
			i = 0 // Start loop with first nibble
		}

		for ; i < len(in); i += 2 {
			res[resI] = combineNibbles(in[i+1], in[i])
			resI++
		}
	}

	return res
}

// combineNibbles concatenates two nibble to make a byte.
// Assumes nibbles are the lower 4 bits of each of the inputs
func combineNibbles(ms byte, ls byte) byte {
	return byte(ms<<4 | (ls & 0xF))
}
