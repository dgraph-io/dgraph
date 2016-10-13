/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package tok

// #include <stdint.h>
// #include <stdlib.h>
// #include "icuc.h"
import "C"

import (
	"bytes"
	"reflect"
	"unicode"
	"unsafe"

	// We rely on these almost standard Go libraries to do unicode normalization.
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/dgraph-io/dgraph/x"
)

const maxTokenSize = 100

var transformer transform.Transformer

// Tokenizer wraps the Tokenizer object in icuc.c.
type Tokenizer struct {
	c *C.Tokenizer

	// We do not own this. It belongs to C.Tokenizer. But we like to cache it to
	// avoid extra cgo calls.
	token *C.char
}

// normalize does unicode normalization.
func normalize(in []byte) ([]byte, error) {
	// We need a new transformer for each input as it cannot be reused.
	filter := func(r rune) bool {
		return unicode.Is(unicode.Mn, r) // Mn: nonspacing marks (to be removed)
	}
	transformer := transform.Chain(norm.NFD, transform.RemoveFunc(filter), norm.NFC)
	out, _, err := transform.Bytes(transformer, in)
	out = bytes.Map(func(r rune) rune {
		if unicode.IsPunct(r) { // Replace punctuations with spaces.
			return ' '
		}
		return unicode.ToLower(r) // Convert to lower case.
	}, out)
	return out, err
}

// NewTokenizer creates a new Tokenizer object from a given input string of bytes.
func NewTokenizer(s []byte) (*Tokenizer, error) {
	x.Assert(s != nil)
	sNorm, terr := normalize(s)
	if terr != nil {
		return nil, terr
	}
	sNorm = append(sNorm, 0) // Null-terminate this for ICU's C functions.

	var err C.UErrorCode
	c := C.NewTokenizer(byteToChar(sNorm), C.int(len(s)), maxTokenSize, &err)
	if int(err) > 0 {
		return nil, x.Errorf("ICU new tokenizer error %d", int(err))
	}
	if c == nil {
		return nil, x.Errorf("NewTokenizer returns nil")
	}
	return &Tokenizer{c, C.TokenizerToken(c)}, nil
}

// Destroy destroys the tokenizer object.
func (t *Tokenizer) Destroy() {
	C.DestroyTokenizer(t.c)
}

// Next returns the next token. It will allocate memory for the token.
func (t *Tokenizer) Next() []byte {
	for {
		n := int(C.TokenizerNext(t.c))
		if n < 0 {
			break
		}
		s := bytes.TrimSpace(charToByte(t.token, n))
		if len(s) > 0 {
			return s
		}
	}
	return nil
}

// StringTokens returns all tokens as strings. If we fail, we return nil.
func (t *Tokenizer) StringTokens() []string {
	var tokens []string
	for {
		s := t.Next()
		if s == nil {
			break
		}
		tokens = append(tokens, string(s))
	}
	return tokens
}

// byteToChar returns *C.char from byte slice.
func byteToChar(b []byte) *C.char {
	var c *C.char
	if len(b) > 0 {
		c = (*C.char)(unsafe.Pointer(&b[0]))
	}
	return c
}

// charToByte converts a *C.char to a byte slice.
func charToByte(data *C.char, l int) []byte {
	var value []byte
	sH := (*reflect.SliceHeader)(unsafe.Pointer(&value))
	sH.Cap, sH.Len, sH.Data = l, l, uintptr(unsafe.Pointer(data))
	return value
}
