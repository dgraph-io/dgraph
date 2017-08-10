package bp128

// dmaxBits128 computes the bit size of the largest delta of
// the input block. seed is used to get the delta of the first
// values.
func maxBits128(in *uint64, seed *uint64) uint8
func maxBits256(in *uint64, seed *uint64) uint8
