// +build embed

package tok

// #cgo CPPFLAGS: -I../vendor/github.com/dgraph-io/goicu/icuembed
// #cgo darwin LDFLAGS: -Wl,-undefined -Wl,dynamic_lookup
// #cgo !darwin LDFLAGS: -Wl,-unresolved-symbols=ignore-all -lrt
import "C"

import (
	"github.com/dgraph-io/dgraph/x"
	_ "github.com/dgraph-io/goicu/icuembed"
)

func init() {
	x.AddInit(func() {
		t, err := NewTokenizer([]byte("hello world"))
		if err != nil {
			disableICU = true
			x.Printf("Disabling ICU because we fail to create tokenizer: %v", err)
			return
		}
		if t == nil || t.c == nil {
			disableICU = true
			x.Printf("Disabling ICU because tokenizer is nil")
			return
		}
		tokens := t.Tokens()
		disableICU = len(tokens) != 2 || tokens[0] != "hello" || tokens[1] != "world"
		if disableICU {
			x.Printf("Disabling ICU because tokenizer fails simple test case: %v",
				tokens)
		}
	})
}
