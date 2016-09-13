// +build embed

package rdb

// #cgo CXXFLAGS: -std=c++11
// #cgo CPPFLAGS: -I${SRCDIR}/../vendor/github.com/cockroachdb/c-lz4/internal/lib
// #cgo CPPFLAGS: -I${SRCDIR}/../vendor/github.com/cockroachdb/c-rocksdb/internal/include
// #cgo CPPFLAGS: -I${SRCDIR}/../vendor/github.com/cockroachdb/c-snappy/internal
// #cgo LDFLAGS: -lstdc++
// #cgo darwin LDFLAGS: -Wl,-undefined -Wl,dynamic_lookup
// #cgo !darwin LDFLAGS: -Wl,-unresolved-symbols=ignore-all -lrt
import "C"

import (
	_ "github.com/cockroachdb/c-lz4"
	_ "github.com/cockroachdb/c-rocksdb"
	_ "github.com/cockroachdb/c-snappy"
)
