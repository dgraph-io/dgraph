package gotomic

/*
 Ripped from http://graphics.stanford.edu/~seander/bithacks.html#ReverseParallel
*/
func reverse(v uint32) uint32 {
	// swap odd and even bits
	v = ((v >> 1) & 0x55555555) | ((v & 0x55555555) << 1)
	// swap consecutive pairs
	v = ((v >> 2) & 0x33333333) | ((v & 0x33333333) << 2)
	// swap nibbles ... 
	v = ((v >> 4) & 0x0F0F0F0F) | ((v & 0x0F0F0F0F) << 4)
	// swap bytes
	v = ((v >> 8) & 0x00FF00FF) | ((v & 0x00FF00FF) << 8)
	// swap 2-byte long pairs
	v = (v >> 16) | (v << 16)
	return v
}
