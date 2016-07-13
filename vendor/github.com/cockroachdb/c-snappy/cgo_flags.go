// Package snappy uses the cgo compilation facilities to build the
// Snappy C++ library.
package snappy

// #cgo CXXFLAGS: -std=c++11
// #cgo CPPFLAGS: -DHAVE_CONFIG_H -Iinternal
// #cgo !windows CPPFLAGS: -DHAVE_SYS_MMAN_H
// #cgo windows CPPFLAGS: -DHAVE_WINDOWS_H
import "C"
