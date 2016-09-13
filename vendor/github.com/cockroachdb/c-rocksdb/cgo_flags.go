// Package rocksdb uses the cgo compilation facilities to build the
// RocksDB C++ library. Note that support for bzip2 and zlib is not
// compiled in.
package rocksdb

// #cgo CPPFLAGS: -Iinternal -Iinternal/include -Iinternal/db -Iinternal/util
// #cgo CPPFLAGS: -Iinternal/utilities/merge_operators/string_append
// #cgo windows CPPFLAGS: -DOS_WIN
// #cgo !windows CPPFLAGS: -DROCKSDB_PLATFORM_POSIX -DROCKSDB_LIB_IO_POSIX
// #cgo darwin CPPFLAGS: -DOS_MACOSX
// #cgo linux CPPFLAGS: -DOS_LINUX -fno-builtin-memcmp -DROCKSDB_FALLOCATE_PRESENT -DROCKSDB_MALLOC_USABLE_SIZE
// #cgo freebsd CPPFLAGS: -DOS_FREEBSD
// #cgo dragonfly CPPFLAGS: -DOS_DRAGONFLY
// #cgo CXXFLAGS: -std=c++11 -fno-omit-frame-pointer -momit-leaf-frame-pointer
// #cgo darwin CXXFLAGS: -Wshorten-64-to-32
// #cgo freebsd CXXFLAGS: -Wshorten-64-to-32
// #cgo dragonfly CXXFLAGS: -Wshorten-64-to-32
// #cgo windows LDFLAGS: -lrpcrt4
import "C"
