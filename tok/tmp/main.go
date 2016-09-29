package main

// #include <stdint.h>
// #include <stdlib.h>
// #include <stdio.h>
// #include <string.h>
// #cgo CFLAGS: -I/usr/local/include -DU_DISABLE_RENAMING=1
// #cgo LDFLAGS: -L/usr/local/lib -licuuc
// #include <unicode/ustring.h>
// #include <unicode/ubrk.h>
//
// UBreakIterator* MyFunc(UBreakIteratorType type, const char* locale, const char* s, int l, UErrorCode* error) {
//  UChar* t = malloc(sizeof(UChar) * (l + 1));
//  u_uastrcpy(t, s);
//     UBreakIterator* bi = ubrk_open(type, locale, t, l, error);
//     return bi;
// }
//
//
import "C"

import "fmt"
import "unicode/utf8"

type BreakIterator struct {
	c *C.UBreakIterator
}

func OpenBreakIterator(s string) *BreakIterator {
	//var err C.int
	//c := C.MyFunc(C.CString(s), C.int(len(s)), &err)
	var err C.UErrorCode
	fmt.Printf("~~~go len = %d %d\n", len(s), utf8.RuneCountInString(s))
	c := C.MyFunc(C.UBRK_WORD, C.CString(""), C.CString(s), C.int(len(s)), &err)
	//c := ubrk_open(C.UBRK_WORD, C.CString(""), (*C.UChar)(unsafe.Pointer(C.CString(s))), C.int(len(s)), &err)
	fmt.Println(err)
	return &BreakIterator{c}
}

func main() {
	bi := OpenBreakIterator("在香港分享有60多人來參加")
	defer C.ubrk_close(bi.c)
	start := C.ubrk_first(bi.c)
	fmt.Println(start)
	next := C.ubrk_next(bi.c)
	fmt.Println(next)
	next = C.ubrk_next(bi.c)
	fmt.Println(next)
	next = C.ubrk_next(bi.c)
	fmt.Println(next)
	next = C.ubrk_next(bi.c)
	fmt.Println(next)
	next = C.ubrk_next(bi.c)
	fmt.Println(next)
	next = C.ubrk_next(bi.c)
	fmt.Println(next)
	next = C.ubrk_next(bi.c)
	fmt.Println(next)
	next = C.ubrk_next(bi.c)
	fmt.Println(next)
	next = C.ubrk_next(bi.c)
	fmt.Println(next)
	next = C.ubrk_next(bi.c)
	fmt.Println(next)
	//OpenBreakIterator("hello")
	//s := "在香港分享有60    多人來參加"
	//var status int
	//	C.ubrk_open(C.UBRK_WORD, "en_us", s, len(s), &status)
	//C.ubrk_open(1, C.CString("en_us"), C.CString(s), C.int(len(s)), &status)
	//var x int32
	//fmt.Println(C.fortytwo(&x))
}
