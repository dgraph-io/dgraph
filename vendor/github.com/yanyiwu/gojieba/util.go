package gojieba

/*
#include "jieba.h"
*/
import "C"
import (
	"os"
	"unsafe"
)

func isDirExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return os.IsExist(err)
	} else {
		return fi.IsDir()
	}
	return false
}

func cstrings(x **C.char) []string {
	var s []string
	eltSize := unsafe.Sizeof(*x)
	for *x != nil {
		s = append(s, C.GoString(*x))
		x = (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(x)) + eltSize))
	}
	return s
}

func convertWords(s string, words *C.Word) []Word {
	result := make([]Word, 0)
	x := words
	eltSize := unsafe.Sizeof(*x)
	start := 0
	end := 0
	for (*x).len != 0 {
		start = int((*x).offset)
		end = start + int((*x).len)
		result = append(result, Word{s[start:end], start, end})
		x = (*C.Word)(unsafe.Pointer(uintptr(unsafe.Pointer(x)) + eltSize))
	}
	return result
}

//func cwordweights(x unsafe.Pointer) []WordWeight {
//	var s []WordWeight
//	eltSize := 16
//	for (*(*C.char))(x) != nil {
//		ww := WordWeight{
//			C.GoString((*C.char))(x)),
//			(*x).weight,
//		}
//		s = append(s, ww)
//		x = (*C.struct_CWordWeight)(unsafe.Pointer(uintptr(unsafe.Pointer(x)) + eltSize))
//	}
//	return s
//}
