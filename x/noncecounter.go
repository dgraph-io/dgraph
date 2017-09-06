package x

import (
	cryptoRand "crypto/rand"
	"encoding/binary"
)

// NonceCounter can generate unique 128-bit numbers.  Only usable on a single thread.  (All it
// would take to be multithreaded is to increment hi atomically.)
type NonceCounter struct {
	initialLo uint64
	initialHi uint64
	counter   uint64
}

// NewNonceCounter reads from /dev/urandom (or such) and generates a new randomized NonceCounter.
func NewNonceCounter() NonceCounter {
	var buf [16]byte
	_, err := cryptoRand.Read(buf[:])
	Check(err)
	var ret NonceCounter
	ret.initialLo = binary.LittleEndian.Uint64(buf[0:8])
	ret.initialHi = binary.LittleEndian.Uint64(buf[8:16])
	ret.counter = 0
	return ret
}

// Generate returns a new nonce value.
func (nc *NonceCounter) Generate() [16]byte {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], nc.initialLo)
	hi := nc.counter + nc.initialHi
	nc.counter++
	binary.LittleEndian.PutUint64(buf[8:16], hi)
	return buf
}

// GenerationNumber returns N+C when buf was produced by the Nth call to Generate(), for some
// global constant C.
func (nc *NonceCounter) GenerationNumber(buf []byte) uint64 {
	hi := binary.LittleEndian.Uint64(buf[8:16])
	return hi - nc.initialHi
}
