// +build embed

package tok

// #cgo CPPFLAGS: -I./icu/common -DU_DISABLE_RENAMING=1
// #cgo darwin LDFLAGS: -Wl,-undefined -Wl,dynamic_lookup
// #cgo !darwin LDFLAGS: -Wl,-unresolved-symbols=ignore-all -lrt
import "C"

import (
	_ "github.com/dgraph-io/dgraph/tok/icu/icuembed"
)
