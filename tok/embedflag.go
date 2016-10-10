// +build embed

package tok

// #cgo CPPFLAGS: -I${SRCDIR}/icu/icuembed -DU_DISABLE_RENAMING=1
// #cgo darwin LDFLAGS: -Wl,-undefined -Wl,dynamic_lookup
// #cgo !darwin LDFLAGS: -Wl,-unresolved-symbols=ignore-all -lrt
// #include <unicode/utypes.h>
// #include <unicode/udata.h>
import "C"

import (
	"flag"
	"io/ioutil"
	"log"
	"time"
	"unsafe"

	_ "github.com/dgraph-io/dgraph/tok/icu/icuembed" // Force ICU to be linked in.
	"github.com/dgraph-io/dgraph/x"
)

var (
	icuDataFile = flag.String("icu", "",
		"Location of ICU data file such as icudt57l.dat.")
	icuData []byte // Hold a reference.
)

func init() {
	x.AddInit(func() {
		x.Assertf(len(*icuDataFile) > 0, "ICU data file empty")

		start := time.Now()
		icuData, err := ioutil.ReadFile(*icuDataFile)
		x.Check(err)

		var icuErr C.UErrorCode
		C.udata_setCommonData(unsafe.Pointer(byteToChar(icuData)), &icuErr)
		x.Assertf(int(icuErr) >= 0, "Error occurred with udata_setCommonData: %d",
			int(icuErr))

		log.Printf("Loaded ICU data from [%s] in %s", *icuDataFile, time.Since(start))
	})
}
