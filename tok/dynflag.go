// +build !embed

package tok

// We assume ICU4C is installed to /usr/local/include and /usr/local/lib.

// #cgo CPPFLAGS: -I/usr/local/include -DU_DISABLE_RENAMING=1
// #cgo LDFLAGS: -L/usr/local/lib -licuuc -libicudata
import "C"
