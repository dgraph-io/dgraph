package trie

import (
	"encoding/binary"
	"math/rand"
	"testing"
)

// Test represents a key-value pair for a test
type Test struct {
	key   []byte
	value []byte
	pk    []byte
	op    int
}

// Key returns the test key
func (t *Test) Key() []byte {
	return t.key
}

// Value returns the test value
func (t *Test) Value() []byte {
	return t.value
}

// GenerateRandomTests returns an array of random Tests
func GenerateRandomTests(t *testing.T, size int) []Test {
	rt := make([]Test, size)
	kv := make(map[string][]byte)

	for i := range rt {
		test := generateRandomTest(t, kv)
		rt[i] = test
		kv[string(test.key)] = rt[i].value
	}

	return rt
}

func generateRandomTest(t *testing.T, kv map[string][]byte) Test {
	r := *rand.New(rand.NewSource(rand.Int63()))
	test := Test{}

	for {
		n := 2 // arbitrary positive number
		size := r.Intn(510) + n
		buf := make([]byte, size)
		_, err := r.Read(buf)
		if err != nil {
			t.Fatal(err)
		}

		key := binary.LittleEndian.Uint16(buf[:2])

		if kv[string(buf)] == nil || key < 256 {
			test.key = buf

			buf = make([]byte, r.Intn(128)+n)
			_, err = r.Read(buf)
			if err != nil {
				t.Fatal(err)
			}
			test.value = buf

			return test
		}
	}
}
