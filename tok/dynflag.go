// +build !embed

package tok

// #cgo CFLAGS: -I/usr/local/include -DU_DISABLE_RENAMING=1
// #cgo LDFLAGS: -L/usr/local/lib -licuuc
import "C"
