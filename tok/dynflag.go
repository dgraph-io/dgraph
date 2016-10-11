// +build !embed

package tok

// We assume ICU4C is installed to /usr/local/include and /usr/local/lib.

// #cgo CPPFLAGS: -DU_DISABLE_RENAMING=1
// #cgo !darwin LDFLAGS: -licuuc -licudata
// #cgo darwin LDFLAGS: -licuuc -licucore -licudata
import "C"
