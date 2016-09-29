package tok

// #include <stdint.h>
// #include <stdlib.h>
// #include "icuc.h"
import "C"

import (
	"bytes"
	"strings"
	"unicode"
	"unsafe"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/dgraph-io/dgraph/x"
)

var (
	transformer transform.Transformer
)

// Tokenizer wraps the Tokenizer object in icuc.c.
type Tokenizer struct {
	c *C.Tokenizer
}

func init() {
	// Prepare the unicode normalizer.
	filter := func(r rune) bool {
		return unicode.Is(unicode.Mn, r) // Mn: nonspacing marks (to be removed)
	}
	transformer = transform.Chain(norm.NFD, transform.RemoveFunc(filter), norm.NFC)
}

// normalize does unicode normalization.
func normalize(in []byte) ([]byte, error) {
	out, _, err := transform.Bytes(transformer, in)
	out = bytes.Map(func(r rune) rune {
		if unicode.IsPunct(r) { // Replace punctuations with spaces.
			return ' '
		}
		return unicode.ToLower(r) // Convert to lower case.
	}, out)
	return out, err
}

func NewTokenizer(s []byte) (*Tokenizer, error) {
	sNorm, terr := normalize(s)
	if terr != nil {
		return nil, terr
	}
	sNorm = append(sNorm, 0) // Null-terminate this for ICU's C functions.

	var err C.UErrorCode
	c := C.NewTokenizer(byteToChar(sNorm), C.int(len(s)), &err)
	if int(err) > 0 {
		return nil, x.Errorf("ICU new tokenizer error %d", int(err))
	}
	return &Tokenizer{c}, nil
}

func (t *Tokenizer) Destroy() {
	C.DestroyTokenizer(t.c)
}

func (t *Tokenizer) Next() *string {
	for {
		result := C.TokenizerNext(t.c) // C string.
		if result == nil {             // We are out of tokens.
			return nil
		}
		s := strings.TrimSpace(C.GoString(result))
		if len(s) > 0 {
			return &s
		}
	}
}

func (t *Tokenizer) Done() bool {
	return C.TokenizerDone(t.c) != 0
}

// byteToChar returns *C.char from byte slice.
func byteToChar(b []byte) *C.char {
	var c *C.char
	if len(b) > 0 {
		c = (*C.char)(unsafe.Pointer(&b[0]))
	}
	return c
}
