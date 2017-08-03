package bp128

// maxBits128 computes the bit size of the largest element of
// the input block. seed parameter is not used.
func maxBits128_64(in uintptr, offset int, seed *byte) uint8
func maxBits128_32(in uintptr, offset int, seed *byte) uint8

// dmaxBits128 computes the bit size of the largest delta of
// the input block. seed is used to get the delta of the first
// values.
func dmaxBits128_32(in uintptr, offset int, seed *byte) uint8
func dmaxBits128_64(in uintptr, offset int, seed *byte) uint8
