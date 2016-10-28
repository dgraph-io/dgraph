// +build embed

package tok

// govendor get  github.com/dgraph-io/goicu/icuembed
// govendor get  github.com/dgraph-io/goicu/icuembed/unicode

// #cgo CPPFLAGS: -I../vendor/github.com/dgraph-io/goicu/icuembed
// #cgo darwin LDFLAGS: -Wl,-undefined -Wl,dynamic_lookup
// #cgo !darwin LDFLAGS: -Wl,-unresolved-symbols=ignore-all -lrt
import "C"

import (
	"flag"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/goicu/icuembed"
)

var (
	icuDataFile = flag.String("icu", "",
		"Location of ICU data file such as icudt57l.dat.")
	icuData []byte // Hold a reference.
)

func init() {
	x.AddInit(func() {
		disableICU = true
		if len(*icuDataFile) == 0 {
			x.Printf("WARNING: ICU data file empty")
			return
		}
		if err := icuembed.Load(*icuDataFile); err != nil {
			x.Printf("WARNING: Error loading ICU datafile: %s %v",
				*icuDataFile, err)
			return
		}
		// Everything well. Re-enable ICU.
		disableICU = false
	})
}
