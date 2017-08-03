package bp128

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaxBits128_32(t *testing.T) {
	in := []uint32{}
	MakeAlignedSlice(128, &in)

	vin := reflect.ValueOf(in)
	for i := 0; i < 32; i++ {
		in[127] = 1 << uint(i)
		bs := maxBits128_32(vin.Pointer(), 0, new(byte))

		if !assert.EqualValues(t, i+1, bs) {
			break
		}
	}
}

func TestMaxBits128_64(t *testing.T) {
	in := []uint64{}
	MakeAlignedSlice(128, &in)

	vin := reflect.ValueOf(in)
	for i := 0; i < 64; i++ {
		in[127] = 1 << uint(i)
		bs := maxBits128_64(vin.Pointer(), 0, new(byte))

		if !assert.EqualValues(t, i+1, bs) {
			break
		}
	}
}

func TestDMaxBits128_32(t *testing.T) {
	offset := 3
	in := []uint32{}
	MakeAlignedSlice(128, &in)

	vin := reflect.ValueOf(in)
	for i := 0; i < 32-offset+1; i++ {
		delta := 1 << uint(i)

		v := 0
		for i := 0; i < 128; i++ {
			v += delta
			in[i] = uint32(v)
		}

		seed := makeAlignedBytes(16)
		copy(seed, convertToBytes(32, vin.Slice(0, 4)))

		bs := dmaxBits128_32(vin.Pointer(), 0, &seed[0])
		if !assert.EqualValues(t, i+offset, bs) {
			break
		}
	}
}

func TestDMaxBits128_64(t *testing.T) {
	offset := 2
	in := []uint64{}
	MakeAlignedSlice(128, &in)

	vin := reflect.ValueOf(in)
	for i := 0; i < 64-offset+1; i++ {
		delta := 1 << uint(i)

		v := 0
		for i := 0; i < 128; i++ {
			v += delta
			in[i] = uint64(v)
		}

		seed := makeAlignedBytes(16)
		copy(seed, convertToBytes(64, vin.Slice(0, 2)))

		bs := dmaxBits128_64(vin.Pointer(), 0, &seed[0])
		if !assert.EqualValues(t, i+offset, bs) {
			break
		}
	}
}
