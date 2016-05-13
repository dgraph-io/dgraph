/*
Package bitset implements bitsets, a mapping
between non-negative integers and boolean values. It should be more
efficient than map[uint] bool.

It provides methods for setting, clearing, flipping, and testing
individual integers.

But it also provides set intersection, union, difference,
complement, and symmetric operations, as well as tests to
check whether any, all, or no bits are set, and querying a
bitset's current length and number of postive bits.

BitSets are expanded to the size of the largest set bit; the
memory allocation is approximately Max bits, where Max is
the largest set bit. BitSets are never shrunk. On creation,
a hint can be given for the number of bits that will be used.

Many of the methods, including Set,Clear, and Flip, return
a BitSet pointer, which allows for chaining.

Example use:

	import "bitset"
	var b BitSet
	b.Set(10).Set(11)
	if b.Test(1000) {
		b.Clear(1000)
	}
	if B.Intersection(bitset.New(100).Set(10)).Count() > 1 {
		fmt.Println("Intersection works.")
	}

As an alternative to BitSets, one should check out the 'big' package,
which provides a (less set-theoretical) view of bitsets.

*/
package bitset

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// the wordSize of a bit set
const wordSize = uint(64)

// log2WordSize is lg(wordSize)
const log2WordSize = uint(6)

// A BitSet is a set of bits. The zero value of a BitSet is an empty set of length 0.
type BitSet struct {
	length uint
	set    []uint64
}

// A BitSetError is used to distinguish errors (panics) generated in this package.
type BitSetError string

// safeSet will fixup b.set to be non-nil and return the field value
func (b *BitSet) safeSet() []uint64 {
	if b.set == nil {
		b.set = make([]uint64, wordsNeeded(0))
	}
	return b.set
}

// wordsNeeded calculates the number of words needed for i bits
func wordsNeeded(i uint) int {
	if i > ((^uint(0)) - wordSize + 1) {
		return int((^uint(0)) >> log2WordSize)
	}
	return int((i + (wordSize - 1)) >> log2WordSize)
}

// New creates a new BitSet with a hint that length bits will be required
func New(length uint) *BitSet {
	return &BitSet{length, make([]uint64, wordsNeeded(length))}
}

// Cap returns the total possible capicity, or number of bits
func Cap() uint {
	return ^uint(0)
}

// Len returns the length of the BitSet in words
func (b *BitSet) Len() uint {
	return b.length
}

// extendSetMaybe adds additional words to incorporate new bits if needed
func (b *BitSet) extendSetMaybe(i uint) {
	if i >= b.length { // if we need more bits, make 'em
		nsize := wordsNeeded(i + 1)
		if b.set == nil {
			b.set = make([]uint64, nsize)
		} else if len(b.set) < nsize {
			newset := make([]uint64, nsize)
			copy(newset, b.set)
			b.set = newset
		}
		b.length = i + 1
	}
}

// Test whether bit i is set.
func (b *BitSet) Test(i uint) bool {
	if i >= b.length {
		return false
	}
	return b.set[i>>log2WordSize]&(1<<(i&(wordSize-1))) != 0
}

// Set bit i to 1
func (b *BitSet) Set(i uint) *BitSet {
	b.extendSetMaybe(i)
	b.set[i>>log2WordSize] |= 1 << (i & (wordSize - 1))
	return b
}

// Clear bit i to 0
func (b *BitSet) Clear(i uint) *BitSet {
	if i >= b.length {
		return b
	}
	b.set[i>>log2WordSize] &^= 1 << (i & (wordSize - 1))
	return b
}

// SetTo sets bit i to value
func (b *BitSet) SetTo(i uint, value bool) *BitSet {
	if value {
		return b.Set(i)
	}
	return b.Clear(i)
}

// Flip bit at i
func (b *BitSet) Flip(i uint) *BitSet {
	if i >= b.length {
		return b.Set(i)
	}
	b.set[i>>log2WordSize] ^= 1 << (i & (wordSize - 1))
	return b
}

// NextSet returns the next bit set from the specified index,
// including possibly the current index
// along with an error code (true = valid, false = no set bit found)
// for i,e := v.NextSet(0); e; i,e = v.NextSet(i + 1) {...}
func (b *BitSet) NextSet(i uint) (uint, bool) {
	x := int(i >> log2WordSize)
	if x >= len(b.set) {
		return 0, false
	}
	w := b.set[x]
	w = w >> (i & (wordSize - 1))
	if w != 0 {
		return i + trailingZeroes64(w), true
	}
	x = x + 1
	for x < len(b.set) {
		if b.set[x] != 0 {
			return uint(x)*wordSize + trailingZeroes64(b.set[x]), true
		}
		x = x + 1

	}
	return 0, false
}

// ClearAll clears the entire BitSet
func (b *BitSet) ClearAll() *BitSet {
	if b != nil && b.set != nil {
		for i := range b.set {
			b.set[i] = 0
		}
	}
	return b
}

// wordCount returns the number of words used in a bit set
func (b *BitSet) wordCount() int {
	return wordsNeeded(b.length)
}

// Clone this BitSet
func (b *BitSet) Clone() *BitSet {
	c := New(b.length)
	if b.set != nil { // Clone should not modify current object
		copy(c.set, b.set)
	}
	return c
}

// Copy this BitSet into a destination BitSet
// Returning the size of the destination BitSet
// like array copy
func (b *BitSet) Copy(c *BitSet) (count uint) {
	if c == nil {
		return
	}
	if b.set != nil { // Copy should not modify current object
		copy(c.set, b.set)
	}
	count = c.length
	if b.length < c.length {
		count = b.length
	}
	return
}

// Count (number of set bits)
func (b *BitSet) Count() uint {
	if b != nil && b.set != nil {
		return uint(popcntSlice(b.set))
	}
	return 0
}

var deBruijn = [...]byte{
	0, 1, 56, 2, 57, 49, 28, 3, 61, 58, 42, 50, 38, 29, 17, 4,
	62, 47, 59, 36, 45, 43, 51, 22, 53, 39, 33, 30, 24, 18, 12, 5,
	63, 55, 48, 27, 60, 41, 37, 16, 46, 35, 44, 21, 52, 32, 23, 11,
	54, 26, 40, 15, 34, 20, 31, 10, 25, 14, 19, 9, 13, 8, 7, 6,
}

func trailingZeroes64(v uint64) uint {
	return uint(deBruijn[((v&-v)*0x03f79d71b4ca8b09)>>58])
}

// Equal tests the equvalence of two BitSets.
// False if they are of different sizes, otherwise true
// only if all the same bits are set
func (b *BitSet) Equal(c *BitSet) bool {
	if c == nil {
		return false
	}
	if b.length != c.length {
		return false
	}
	if b.length == 0 { // if they have both length == 0, then could have nil set
		return true
	}
	// testing for equality shoud not transform the bitset (no call to safeSet)

	for p, v := range b.set {
		if c.set[p] != v {
			return false
		}
	}
	return true
}

func panicIfNull(b *BitSet) {
	if b == nil {
		panic(BitSetError("BitSet must not be null"))
	}
}

// Difference of base set and other set
// This is the BitSet equivalent of &^ (and not)
func (b *BitSet) Difference(compare *BitSet) (result *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	result = b.Clone() // clone b (in case b is bigger than compare)
	l := int(compare.wordCount())
	if l > int(b.wordCount()) {
		l = int(b.wordCount())
	}
	for i := 0; i < l; i++ {
		result.set[i] = b.set[i] &^ compare.set[i]
	}
	return
}

// DifferenceCardinality computes the cardinality of the differnce
func (b *BitSet) DifferenceCardinality(compare *BitSet) uint {
	panicIfNull(b)
	panicIfNull(compare)
	l := int(compare.wordCount())
	if l > int(b.wordCount()) {
		l = int(b.wordCount())
	}
	cnt := uint64(0)
	cnt += popcntMaskSlice(b.set[:l], compare.set[:l])
	cnt += popcntSlice(b.set[l:])
	return uint(cnt)
}

// InPlaceDifference computes the difference of base set and other set
// This is the BitSet equivalent of &^ (and not)
func (b *BitSet) InPlaceDifference(compare *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	l := int(compare.wordCount())
	if l > int(b.wordCount()) {
		l = int(b.wordCount())
	}
	for i := 0; i < l; i++ {
		b.set[i] &^= compare.set[i]
	}
}

// Convenience function: return two bitsets ordered by
// increasing length. Note: neither can be nil
func sortByLength(a *BitSet, b *BitSet) (ap *BitSet, bp *BitSet) {
	if a.length <= b.length {
		ap, bp = a, b
	} else {
		ap, bp = b, a
	}
	return
}

// Intersection of base set and other set
// This is the BitSet equivalent of & (and)
func (b *BitSet) Intersection(compare *BitSet) (result *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	b, compare = sortByLength(b, compare)
	result = New(b.length)
	for i, word := range b.set {
		result.set[i] = word & compare.set[i]
	}
	return
}

// IntersectionCardinality computes the cardinality of the union
func (b *BitSet) IntersectionCardinality(compare *BitSet) uint {
	panicIfNull(b)
	panicIfNull(compare)
	b, compare = sortByLength(b, compare)
	cnt := popcntAndSlice(b.set, compare.set)
	return uint(cnt)
}

// InPlaceIntersection destructively computes the intersection of
// base set and the compare set.
// This is the BitSet equivalent of & (and)
func (b *BitSet) InPlaceIntersection(compare *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	l := int(compare.wordCount())
	if l > int(b.wordCount()) {
		l = int(b.wordCount())
	}
	for i := 0; i < l; i++ {
		b.set[i] &= compare.set[i]
	}
	for i := l; i < len(b.set); i++ {
		b.set[i] = 0
	}
	if compare.length > 0 {
		b.extendSetMaybe(compare.length - 1)
	}
	return
}

// Union of base set and other set
// This is the BitSet equivalent of | (or)
func (b *BitSet) Union(compare *BitSet) (result *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	b, compare = sortByLength(b, compare)
	result = compare.Clone()
	for i, word := range b.set {
		result.set[i] = word | compare.set[i]
	}
	return
}

// UnionCardinality computes the cardinality of the uniton of the base set
// and the compare set.
func (b *BitSet) UnionCardinality(compare *BitSet) uint {
	panicIfNull(b)
	panicIfNull(compare)
	b, compare = sortByLength(b, compare)
	cnt := popcntOrSlice(b.set, compare.set)
	if len(compare.set) > len(b.set) {
		cnt += popcntSlice(compare.set[len(b.set):])
	}
	return uint(cnt)
}

// InPlaceUnion creates the destructive union of base set and compare set.
// This is the BitSet equivalent of | (or).
func (b *BitSet) InPlaceUnion(compare *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	l := int(compare.wordCount())
	if l > int(b.wordCount()) {
		l = int(b.wordCount())
	}
	if compare.length > 0 {
		b.extendSetMaybe(compare.length - 1)
	}
	for i := 0; i < l; i++ {
		b.set[i] |= compare.set[i]
	}
	if len(compare.set) > l {
		for i := l; i < len(compare.set); i++ {
			b.set[i] = compare.set[i]
		}
	}
}

// SymmetricDifference of base set and other set
// This is the BitSet equivalent of ^ (xor)
func (b *BitSet) SymmetricDifference(compare *BitSet) (result *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	b, compare = sortByLength(b, compare)
	// compare is bigger, so clone it
	result = compare.Clone()
	for i, word := range b.set {
		result.set[i] = word ^ compare.set[i]
	}
	return
}

// SymmetricDifferenceCardinality computes the cardinality of the symmetric difference
func (b *BitSet) SymmetricDifferenceCardinality(compare *BitSet) uint {
	panicIfNull(b)
	panicIfNull(compare)
	b, compare = sortByLength(b, compare)
	cnt := popcntXorSlice(b.set, compare.set)
	if len(compare.set) > len(b.set) {
		cnt += popcntSlice(compare.set[len(b.set):])
	}
	return uint(cnt)
}

// InPlaceSymmetricDifference creates the destructive SymmetricDifference of base set and other set
// This is the BitSet equivalent of ^ (xor)
func (b *BitSet) InPlaceSymmetricDifference(compare *BitSet) {
	panicIfNull(b)
	panicIfNull(compare)
	l := int(compare.wordCount())
	if l > int(b.wordCount()) {
		l = int(b.wordCount())
	}
	if compare.length > 0 {
		b.extendSetMaybe(compare.length - 1)
	}
	for i := 0; i < l; i++ {
		b.set[i] ^= compare.set[i]
	}
	if len(compare.set) > l {
		for i := l; i < len(compare.set); i++ {
			b.set[i] = compare.set[i]
		}
	}
}

// Is the length an exact multiple of word sizes?
func (b *BitSet) isEven() bool {
	return b.length%wordSize == 0
}

// Clean last word by setting unused bits to 0
func (b *BitSet) cleanLastWord() {
	if !b.isEven() {
		// Mask for cleaning last word
		const allBits uint64 = 0xffffffffffffffff
		b.set[wordsNeeded(b.length)-1] &= allBits >> (wordSize - b.length%wordSize)
	}
}

// Complement computes the (local) complement of a biset (up to length bits)
func (b *BitSet) Complement() (result *BitSet) {
	panicIfNull(b)
	result = New(b.length)
	for i, word := range b.set {
		result.set[i] = ^word
	}
	result.cleanLastWord()
	return
}

// All returns true if all bits are set, false otherwise
func (b *BitSet) All() bool {
	panicIfNull(b)
	return b.Count() == b.length
}

// None returns true if no bit is set, false otherwise
func (b *BitSet) None() bool {
	panicIfNull(b)
	if b != nil && b.set != nil {
		for _, word := range b.set {
			if word > 0 {
				return false
			}
		}
		return true
	}
	return true
}

// Any returns true if any bit is set, false otherwise
func (b *BitSet) Any() bool {
	panicIfNull(b)
	return !b.None()
}

// IsSuperSet returns true if this is a superset of the other set
func (b *BitSet) IsSuperSet(other *BitSet) bool {
	for i, e := other.NextSet(0); e; i, e = other.NextSet(i + 1) {
		if !b.Test(i) {
			return false
		}
	}
	return true
}

// IsStrictSuperSet returns true if this is a strict superset of the other set
func (b *BitSet) IsStrictSuperSet(other *BitSet) bool {
	return b.Count() > other.Count() && b.IsSuperSet(other)
}

// DumpAsBits dumps a bit set as a string of bits
func (b *BitSet) DumpAsBits() string {
	if b.set == nil {
		return "."
	}
	buffer := bytes.NewBufferString("")
	i := len(b.set) - 1
	for ; i >= 0; i-- {
		fmt.Fprintf(buffer, "%064b.", b.set[i])
	}
	return string(buffer.Bytes())
}

// BinaryStorageSize returns the binary storage requirements
func (b *BitSet) BinaryStorageSize() int {
	return binary.Size(uint64(0)) + binary.Size(b.set)
}

// WriteTo writes a BitSet to a stream
func (b *BitSet) WriteTo(stream io.Writer) (int64, error) {
	length := uint64(b.length)

	// Write length
	err := binary.Write(stream, binary.BigEndian, length)
	if err != nil {
		return 0, err
	}

	// Write set
	err = binary.Write(stream, binary.BigEndian, b.set)
	return int64(b.BinaryStorageSize()), err
}

// ReadFrom reads a BitSet from a stream written using WriteTo
func (b *BitSet) ReadFrom(stream io.Reader) (int64, error) {
	var length uint64

	// Read length first
	err := binary.Read(stream, binary.BigEndian, &length)
	if err != nil {
		return 0, err
	}
	newset := New(uint(length))

	if uint64(newset.length) != length {
		return 0, errors.New("Unmarshalling error: type mismatch")
	}

	// Read remaining bytes as set
	err = binary.Read(stream, binary.BigEndian, newset.set)
	if err != nil {
		return 0, err
	}

	*b = *newset
	return int64(b.BinaryStorageSize()), nil
}

// MarshalJSON marshals a BitSet as a JSON structure
func (b *BitSet) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBuffer(make([]byte, 0, b.BinaryStorageSize()))
	_, err := b.WriteTo(buffer)
	if err != nil {
		return nil, err
	}

	// URLEncode all bytes
	return json.Marshal(base64.URLEncoding.EncodeToString(buffer.Bytes()))
}

// UnmarshalJSON unmarshals a BitSet from JSON created using MarshalJSON
func (b *BitSet) UnmarshalJSON(data []byte) error {
	// Unmarshal as string
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	// URLDecode string
	buf, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return err
	}

	_, err = b.ReadFrom(bytes.NewReader(buf))
	return err
}
