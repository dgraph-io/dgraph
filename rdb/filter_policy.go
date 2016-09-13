package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"

// FilterPolicy is a factory type that allows the RocksDB database to create a
// filter, such as a bloom filter, which will used to reduce reads.
type FilterPolicy interface {
	// keys contains a list of keys (potentially with duplicates)
	// that are ordered according to the user supplied comparator.
	CreateFilter(keys [][]byte) []byte

	// "filter" contains the data appended by a preceding call to
	// CreateFilter(). This method must return true if
	// the key was in the list of keys passed to CreateFilter().
	// This method may return true or false if the key was not on the
	// list, but it should aim to return false with a high probability.
	KeyMayMatch(key []byte, filter []byte) bool

	// Return the name of this policy.
	Name() string
}

// NewNativeFilterPolicy creates a FilterPolicy object.
func NewNativeFilterPolicy(c *C.rdb_filterpolicy_t) FilterPolicy {
	return nativeFilterPolicy{c}
}

type nativeFilterPolicy struct {
	c *C.rdb_filterpolicy_t
}

func (fp nativeFilterPolicy) CreateFilter(keys [][]byte) []byte          { return nil }
func (fp nativeFilterPolicy) KeyMayMatch(key []byte, filter []byte) bool { return false }
func (fp nativeFilterPolicy) Name() string                               { return "" }

// NewBloomFilter returns a new filter policy that uses a bloom filter with approximately
// the specified number of bits per key.  A good value for bits_per_key
// is 10, which yields a filter with ~1% false positive rate.
//
// Note: if you are using a custom comparator that ignores some parts
// of the keys being compared, you must not use NewBloomFilterPolicy()
// and must provide your own FilterPolicy that also ignores the
// corresponding parts of the keys.  For example, if the comparator
// ignores trailing spaces, it would be incorrect to use a
// FilterPolicy (like NewBloomFilterPolicy) that does not ignore
// trailing spaces in keys.
func NewBloomFilter(bitsPerKey int) FilterPolicy {
	return NewNativeFilterPolicy(C.rdb_filterpolicy_create_bloom(C.int(bitsPerKey)))
}

// Hold references to filter policies.
var filterPolicies []FilterPolicy

func registerFilterPolicy(fp FilterPolicy) int {
	filterPolicies = append(filterPolicies, fp)
	return len(filterPolicies) - 1
}

//export rdbc_filterpolicy_create_filter
func rdbc_filterpolicy_create_filter(idx int, cKeys **C.char, cKeysLen *C.size_t, cNumKeys C.int, cDstLen *C.size_t) *C.char {
	rawKeys := charSlice(cKeys, cNumKeys)
	keysLen := sizeSlice(cKeysLen, cNumKeys)
	keys := make([][]byte, int(cNumKeys))
	for i, len := range keysLen {
		keys[i] = charToByte(rawKeys[i], len)
	}

	dst := filterPolicies[idx].CreateFilter(keys)
	*cDstLen = C.size_t(len(dst))
	return cByteSlice(dst)
}

//export rdbc_filterpolicy_key_may_match
func rdbc_filterpolicy_key_may_match(idx int, cKey *C.char, cKeyLen C.size_t, cFilter *C.char, cFilterLen C.size_t) C.uchar {
	key := charToByte(cKey, cKeyLen)
	filter := charToByte(cFilter, cFilterLen)
	return boolToChar(filterPolicies[idx].KeyMayMatch(key, filter))
}

//export rdbc_filterpolicy_name
func rdbc_filterpolicy_name(idx int) *C.char {
	return stringToChar(filterPolicies[idx].Name())
}
