// +build embed

package icutok

// #cgo CPPFLAGS: -I../../goicu/icuembed
// #cgo darwin LDFLAGS: -Wl,-undefined -Wl,dynamic_lookup
// #cgo !darwin LDFLAGS: -Wl,-unresolved-symbols=ignore-all -lrt
import "C"

import (
	"flag"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/goicu/icuembed"
)

var (
	icuDataFile = flag.String("icu", "/usr/local/share/icudt58l.dat",
		"Location of ICU data file such as icudt58l.dat.")
)

func init() {
	x.AddInit(func() {
		if len(*icuDataFile) == 0 {
			disableICU = true
			x.Printf("ICU data file missing")
			return
		}

		if err := icuembed.Load(*icuDataFile); err != nil {
			disableICU = true
			x.Printf("Failed to load ICU data file: %v", err)
			return
		}

		// Basic testing.
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
			return
		}
		x.Printf("ICU is working fine!")
	})
}
