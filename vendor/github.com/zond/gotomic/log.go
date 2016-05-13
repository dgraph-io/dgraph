package gotomic

/*
 Ripped from http://graphics.stanford.edu/~seander/bithacks.html#IntegerLogLookup
*/
var log_table_256 []uint32

func init() {
	log_table_256 = make([]uint32, 256)
	for i := 2; i < 256; i++ {
		log_table_256[i] = 1 + log_table_256[i/2]
	}
}
func log2(v uint32) uint32 {
	var r, tt uint32
	if tt = v >> 24; tt != 0 {
		r = 24 + log_table_256[tt]
	} else if tt = v >> 16; tt != 0 {
		r = 16 + log_table_256[tt]
	} else if tt = v >> 8; tt != 0 {
		r = 8 + log_table_256[tt]
	} else {
		r = log_table_256[v]
	}
	return r
}
