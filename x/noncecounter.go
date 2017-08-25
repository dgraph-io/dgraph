package x

import (
	cryptoRand "crypto/rand"
	"encoding/binary"
	"sync/atomic"
)

// NonceCounter can generate unique 128-bit numbers, and it can be accessed concurrently.
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

// Generate returns a new nonce value.  It uses an atomic operation.
func (nc *NonceCounter) Generate() [16]byte {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], nc.lo)
	value := atomic.AddUint64(&nc.hi, 1)
	binary.LittleEndian.PutUint64(buf[8:16], value)
	return buf
}
