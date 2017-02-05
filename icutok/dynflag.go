// +build !embed

package icutok

// We assume ICU4C is installed to /usr/local/include and /usr/local/lib.

// #cgo !darwin LDFLAGS: -licuuc -licudata
// #cgo darwin LDFLAGS: -licuuc -licucore -licudata
import "C"
