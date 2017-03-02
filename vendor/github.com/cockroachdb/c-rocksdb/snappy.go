// +build !windows

package rocksdb

import (
	// explicit because these Go packages do not export any Go symbols, but they
	// do export c/c++ symbols which will be needed by the final binary
	// containing this package.
	_ "github.com/cockroachdb/c-snappy"
)

// #cgo CPPFLAGS: -DSNAPPY
// #cgo CPPFLAGS: -I../c-snappy/internal
// #cgo !strictld,darwin LDFLAGS: -Wl,-undefined -Wl,dynamic_lookup
// #cgo !strictld,!darwin LDFLAGS: -Wl,-unresolved-symbols=ignore-all
import "C"
