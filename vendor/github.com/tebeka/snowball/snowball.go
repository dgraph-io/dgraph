// Package snowball implements a stemmer
//
// Example:
//  stemmer = snowball.New("english")
//  fmt.Println(stemmer.stem("running")) // Will print "run"
package snowball

import (
	"fmt"
	"runtime"
	"unsafe"
)

/*
#include <stdlib.h>
#include "libstemmer.h"
*/
import "C"

const (
	// Version is the library version
	Version = "0.2.0"
)

// Stemmer structure
type Stemmer struct {
	lang string
	stmr *C.struct_sb_stemmer
}

// free C resources
func free(stmr *Stemmer) {
	if stmr.stmr != nil {
		C.sb_stemmer_delete(stmr.stmr)
		stmr.stmr = nil
	}
}

// New creates a new stemmer for lang
func New(lang string) (*Stemmer, error) {
	clang := C.CString(lang)
	defer C.free(unsafe.Pointer(clang))

	stmr := &Stemmer{
		lang,
		C.sb_stemmer_new(clang, nil),
	}

	if stmr.stmr == nil {
		return nil, fmt.Errorf("can't create stemmer for lang %s", lang)
	}

	runtime.SetFinalizer(stmr, free)

	return stmr, nil
}

// Lang return the stemmer language
func (stmr *Stemmer) Lang() string {
	return stmr.lang
}

// Stem returns them stem of word (e.g. running -> run)
func (stmr *Stemmer) Stem(word string) string {
	ptr := unsafe.Pointer(C.CString(word))
	defer C.free(ptr)

	w := (*C.sb_symbol)(ptr)
	res := unsafe.Pointer(C.sb_stemmer_stem(stmr.stmr, w, C.int(len(word))))
	size := C.sb_stemmer_length(stmr.stmr)

	buf := C.GoBytes(res, size)
	return string(buf)
}

// LangList returns the list of languages supported by snowball
func LangList() []string {
	return langList
}

var langList []string

func init() {
	// We don't need to free since sb_stemmer_list return pointer to static variable
	cp := uintptr(unsafe.Pointer(C.sb_stemmer_list()))
	size := unsafe.Sizeof(uintptr(0))

	for {
		name := C.GoString(*(**C.char)(unsafe.Pointer(cp)))
		if len(name) == 0 {
			break
		}
		langList = append(langList, name)
		cp += size
	}
}
