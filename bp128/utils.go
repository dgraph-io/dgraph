package bp128

import (
	"reflect"
	"unsafe"
)

// MakeAlignedSlice loads a 16-byte aligned slice
// to dst. dst must be a pointer to an integer slice.
func MakeAlignedSlice(length int, dst interface{}) {
	intSize := 0
	switch dst.(type) {
	case *[]int, *[]uint, *[]int64, *[]uint64:
		intSize = 64
	case *[]int32, *[]uint32:
		intSize = 32
	case *[]int16, *[]uint16:
		intSize = 16
	case *[]int8, *[]uint8:
		intSize = 8
	default:
		panic("bp128: dst is not a pointer to integer slice")
	}

	padding := (addrAlignment * 8) / intSize

	c := length + padding
	vdst := reflect.ValueOf(dst).Elem()
	vslice := reflect.MakeSlice(vdst.Type(), c, c)

	idx := 0
	addr := unsafe.Pointer(vslice.Pointer())
	for !isAligned(intSize, uintptr(addr), idx) {
		idx++
	}

	vdst.Set(vslice.Slice(idx, idx+length))
}

func isAligned(intSize int, addr uintptr, index int) bool {
	addr += uintptr(index * (intSize / 8))
	return addr&(addrAlignment-1) == 0
}

func makeAlignedBytes(length int) []byte {
	if length == 0 {
		return nil
	}

	out := []byte{}
	MakeAlignedSlice(length, &out)
	return out
}

func alignSlice(intSize int, v reflect.Value) reflect.Value {
	padding := (addrAlignment * 8) / intSize

	nslice := v
	length := v.Len() + padding
	if v.Cap() < length {
		nslice = reflect.MakeSlice(v.Type(), length, length)
	}

	idx := 0
	addr := unsafe.Pointer(nslice.Pointer())
	for !isAligned(intSize, uintptr(addr), idx) {
		idx++
	}

	return reflect.AppendSlice(nslice.Slice(idx, idx), v)
}

func convertToBytes(intSize int, v reflect.Value) []byte {
	if !v.IsValid() {
		return nil
	}

	nbytes := intSize / 8
	sh := &reflect.SliceHeader{}
	sh.Cap = v.Cap() * nbytes
	sh.Len = v.Len() * nbytes
	sh.Data = v.Pointer()
	return *(*[]uint8)(unsafe.Pointer(sh))
}

func appendBytes(intSize int, v reflect.Value, b []byte) reflect.Value {
	length := (len(b) * 8) / intSize

	sh := &reflect.SliceHeader{}
	sh.Cap = length
	sh.Len = length
	sh.Data = uintptr(unsafe.Pointer(&b[0]))
	nslice := reflect.NewAt(v.Type(), unsafe.Pointer(sh)).Elem()

	return reflect.AppendSlice(v, nslice)
}

func min(x, y int) int {
	if x < y {
		return x
	}

	return y
}
