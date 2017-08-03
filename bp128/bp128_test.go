package bp128

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

type intSlice reflect.Value

func (s intSlice) Len() int {
	return reflect.Value(s).Len()
}

func (s intSlice) Less(i, j int) bool {
	v := reflect.Value(s)
	return v.Index(i).Uint() < v.Index(j).Uint()
}

func (s intSlice) Swap(i, j int) {
	v := reflect.Value(s)
	ii := v.Index(i).Uint()
	jj := v.Index(j).Uint()
	v.Index(i).SetUint(jj)
	v.Index(j).SetUint(ii)
}

func testPackUnpackAsm(t *testing.T, intSize, nbits int, isDiffCode bool) bool {
	fpack := fpack32
	funpack := funpack32
	var in interface{}
	var out interface{}
	if intSize == 64 {
		fpack = fpack64
		funpack = funpack64
		in = &[]uint64{}
		out = &[]uint64{}
		if isDiffCode {
			fpack = fdpack64
			funpack = fdunpack64
		}
	} else if intSize == 32 {
		in = &[]uint32{}
		out = &[]uint32{}
		if isDiffCode {
			fpack = fdpack32
			funpack = fdunpack32
		}
	}

	max := 1 << uint(nbits)
	MakeAlignedSlice(blockSize, in)
	MakeAlignedSlice(blockSize, out)

	vin := reflect.ValueOf(in).Elem()
	vout := reflect.ValueOf(out).Elem()
	zip := makeAlignedBytes((nbits * blockSize) / 8)

	for i := 0; i < blockSize; i++ {
		if nbits < 63 {
			vin.Index(i).SetUint(uint64(rand.Intn(max)))
		} else {
			vin.Index(i).SetUint(uint64(rand.Int63()))
		}

	}
	sort.Sort(intSlice(vin))

	nslice := blockSize / intSize
	seed := makeAlignedBytes(16)
	copy(seed, convertToBytes(intSize, vin.Slice(0, nslice)))

	inAddr := vin.Pointer()
	outAddr := vout.Pointer()

	fpack[nbits](inAddr, &zip[0], 0, &seed[0])

	copy(seed, convertToBytes(intSize, vin.Slice(0, nslice)))
	funpack[nbits](&zip[0], outAddr, 0, &seed[0])

	equal := false
	for i := 0; i < blockSize; i++ {
		equal = assert.Equal(t, vin.Index(i).Uint(), vout.Index(i).Uint())
		if !equal {
			break
		}
	}

	return equal
}

func TestPackUnpackAsm32(t *testing.T) {
	for i := 1; i <= 32; i++ {
		if !testPackUnpackAsm(t, 32, i, false) {
			fmt.Printf("Pack-unpack for bit size %d failed\n", i)
			break
		}
	}
}

func TestPackUnpackAsm64(t *testing.T) {
	for i := 1; i <= 64; i++ {
		if !testPackUnpackAsm(t, 64, i, false) {
			fmt.Printf("Pack-unpack for bit size %d failed\n", i)
			break
		}
	}
}

func TestDeltaPackUnpackAsm32(t *testing.T) {
	for i := 1; i <= 32; i++ {
		if !testPackUnpackAsm(t, 32, i, true) {
			fmt.Printf("Pack-unpack for bit size %d failed\n", i)
			break
		}
	}
}

func TestDeltaPackUnpackAsm64(t *testing.T) {
	for i := 1; i <= 64; i++ {
		if !testPackUnpackAsm(t, 64, i, true) {
			fmt.Printf("Pack-unpack for bit size %d failed\n", i)
			break
		}
	}
}

var getData32 = func() func() []uint32 {
	f, err := os.Open("data/clustered100K.bin")
	if err != nil {
		panic(err)
	}

	buf := make([]byte, 4)
	_, err = f.Read(buf)
	if err != nil {
		panic(err)
	}

	ndata := binary.LittleEndian.Uint32(buf)
	data := make([]uint32, ndata)

	for i := range data {
		_, err = f.Read(buf)
		if err != nil {
			panic(err)
		}

		data[i] = binary.LittleEndian.Uint32(buf)
	}

	return func() []uint32 { return data }
}()

var getData64 = func() func() []uint64 {
	data := getData32()
	data64 := make([]uint64, len(data))
	for i, d := range data {
		data64[i] = uint64(d)
	}

	return func() []uint64 { return data64 }
}()

func TestPackUnpackZero(t *testing.T) {
	data32 := make([]uint32, 2048)
	data32b := make([]uint32, 2048+128)
	data64 := make([]uint64, 2048+128+42)

	packed32 := Pack(data32)
	packed32b := DeltaPack(data32b)
	packed64 := DeltaPack(data64)

	var out32 []uint32
	var out32b []uint32
	var out64 []uint64
	Unpack(packed32, &out32)
	Unpack(packed32b, &out32b)
	Unpack(packed64, &out64)

	assert.Equal(t, data32, out32)
	assert.Equal(t, data32b, out32b)
	assert.Equal(t, data64, data64)
}

func TestPackUnpack32(t *testing.T) {
	data := getData32()

	var out []uint32
	packed := Pack(data)
	Unpack(packed, &out)

	assert.Equal(t, data, out)
}

func TestPackUnpack64(t *testing.T) {
	data := getData64()

	var out []uint64
	packed := Pack(data)
	Unpack(packed, &out)

	assert.Equal(t, data, out)
}

func TestDeltaPackUnpack32(t *testing.T) {
	data := getData32()

	var out []uint32
	packed := DeltaPack(data)
	Unpack(packed, &out)

	assert.Equal(t, data, out)
}

func TestDeltaPackUnpack64(t *testing.T) {
	data := getData64()

	var out []uint64
	packed := DeltaPack(data)
	Unpack(packed, &out)

	assert.Equal(t, data, out)
}

func TestPackedIntsEncDec(t *testing.T) {
	data := getData32()

	packed1 := Pack(data)
	enc, _ := packed1.GobEncode()

	packed2 := &PackedInts{}
	packed2.GobDecode(enc)

	var out []uint32
	Unpack(packed2, &out)
	assert.Equal(t, data, out)
}

func TestDeltaPackedIntsEncDec(t *testing.T) {
	data := getData32()

	packed1 := DeltaPack(data)
	enc, _ := packed1.GobEncode()

	packed2 := &PackedInts{}
	packed2.GobDecode(enc)

	var out []uint32
	Unpack(packed2, &out)
	assert.Equal(t, data, out)
}
