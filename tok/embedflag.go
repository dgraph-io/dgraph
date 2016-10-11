// +build embed

package tok

//  /home/jchiu/go/src/github.com/dgraph-io/dgraph/vendor/github.com/dgraph-io/goicu/icuembed

// #cgo CPPFLAGS: -I/home/jchiu/go/src/github.com/dgraph-io/dgraph/vendor/github.com/dgraph-io/goicu/icuembed -DU_DISABLE_RENAMING=1
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
		x.Assertf(len(*icuDataFile) > 0, "ICU data file empty")
		x.Check(icuembed.Load(*icuDataFile))
	})
}
