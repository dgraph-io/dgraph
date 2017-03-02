// Package rocksdb uses the cgo compilation facilities to build the
// RocksDB C++ library. Note that support for bzip2 and zlib is not
// compiled in.
package rocksdb

// #cgo CPPFLAGS: -Iinternal -Iinternal/include -Iinternal/db -Iinternal/util
// #cgo CPPFLAGS: -Iinternal/utilities/merge_operators/string_append
// #cgo amd64 CPPFLAGS: -msse -msse4.2
// #cgo windows CPPFLAGS: -DOS_WIN
// #cgo !windows CPPFLAGS: -DROCKSDB_PLATFORM_POSIX -DROCKSDB_LIB_IO_POSIX
// #cgo darwin CPPFLAGS: -DOS_MACOSX -DROCKSDB_BACKTRACE
// #cgo linux CPPFLAGS: -DOS_LINUX -fno-builtin-memcmp -DROCKSDB_MALLOC_USABLE_SIZE
// #cgo freebsd CPPFLAGS: -DOS_FREEBSD
// #cgo dragonfly CPPFLAGS: -DOS_DRAGONFLY
// #cgo CXXFLAGS: -std=c++11
// #cgo windows LDFLAGS: -lrpcrt4
import "C"
