package x

import (
	//"bytes"
	//	"fmt"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	testCases := []uint64{123, 123 << 8, 123 << 16, 56546546}
	for _, v := range testCases {
		b, err := EncodeUint64(v)
		if err != nil {
			t.Error(err)
			return
		}
		w, err := DecodeUint64(b)
		if err != nil {
			t.Error(err)
			return
		}
		if w != v {
			t.Errorf("Encode . Decode not identity %d %v %d", v, b, w)
			return
		}
	}
}

func TestEncodeDecodeOrdered(t *testing.T) {
	testCases := []uint64{123, 123 << 8, 123 << 16, 56546546}
	for _, v := range testCases {
		b, err := EncodeUint64Ordered(v)
		if err != nil {
			t.Error(err)
			return
		}
		w, err := DecodeUint64Ordered(b)
		if err != nil {
			t.Error(err)
			return
		}
		if w != v {
			t.Error("Encode . Decode not identity %d %v %d", v, b, w)
			return
		}
	}
}

func TestSortedBytes(t *testing.T) {
	v := []uint64{1, 3243, 45656546, 33657547547}
	var s []string
	for _, z := range v {
		b, err := EncodeUint64Ordered(z)
		if err != nil {
			t.Error(err)
			return
		}
		s = append(s, string(b))
	}
	for i := 0; i+1 < len(v); i++ {
		if s[i] >= s[i+1] {
			t.Errorf("Strings not ordered %d %d", v[i], v[i+1])
			return
		}
	}
}
