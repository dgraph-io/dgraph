package x

import (
	cryptoRand "crypto/rand"
	"encoding/binary"
)

// NonceCounter can generate unique 128-bit numbers.  Only usable on a single thread.
type NonceCounter struct {
	lo uint64
	hi uint64 // Only this gets incremented, atomically, and it wraps around.
}

// NewNonceCounter reads from /dev/urandom (or such) and generates a new randomized NonceCounter.
func NewNonceCounter() NonceCounter {
	var buf [16]byte
	_, err := cryptoRand.Read(buf[:])
	Check(err)
	var ret NonceCounter
	ret.lo = binary.LittleEndian.Uint64(buf[0:8])
	ret.hi = binary.LittleEndian.Uint64(buf[8:16])
	return ret
}

// Generate returns a new nonce value.
func (nc *NonceCounter) Generate() [16]byte {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], nc.lo)
	nc.hi++
	binary.LittleEndian.PutUint64(buf[8:16], nc.hi)
	return buf
}
