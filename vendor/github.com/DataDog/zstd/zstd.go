package zstd

/*
#include "zstd.h"
*/
import "C"
import (
	"bytes"
	"errors"
	"io/ioutil"
	"unsafe"
)

// ErrorCode is an error returned by the zstd library.
type ErrorCode int

func (e ErrorCode) Error() string {
	switch int(e) {
	case -1:
		return "Error (generic)"
	case -2:
		return "Unknown frame descriptor"
	case -3:
		return "Unsupported frame parameter"
	case -4:
		return "Frame parameter unsupported in 32-bits mode"
	case -5:
		return "Context should be init first"
	case -6:
		return "Allocation error: not enough memory"
	case -7:
		return "Operation not authorized at current processing stage"
	case -8:
		return "Destination buffer is too small"
	case -9:
		return "Src size incorrect"
	case -10:
		return "Corrupted block detected"
	case -11:
		return "tableLog requires too much memory"
	case -12:
		return "Unsupported max possible Symbol Value : too large"
	case -13:
		return "Specified maxSymbolValue is too small"
	case -14:
		return "Dictionary is corrupted"
	default:
		return ""
	}
}

const (
	BestSpeed       = 1
	BestCompression = 20
)

var (
	ErrGeneric                           ErrorCode = -1
	ErrPrefixUnknown                     ErrorCode = -2
	ErrFrameParameterUnsupported         ErrorCode = -3
	ErrFrameParameterUnsupportedBy32bits ErrorCode = -4
	ErrInitMissing                       ErrorCode = -5
	ErrMemoryAllocation                  ErrorCode = -6
	ErrStageWrong                        ErrorCode = -7
	ErrDstSizeTooSmall                   ErrorCode = -8
	ErrSrcSizeWrong                      ErrorCode = -9
	ErrCorruptionDetected                ErrorCode = -10
	ErrTableLogTooLarge                  ErrorCode = -11
	ErrMaxSymbolValueTooLarge            ErrorCode = -12
	ErrMaxSymbolValueTooSmall            ErrorCode = -13
	ErrDictionaryCorrupted               ErrorCode = -14
	ErrEmptySlice                                  = errors.New("Bytes slice is empty")

	DefaultCompression = 5
)

// CompressBound returns the worst case size needed for a destination buffer,
// which can be used to preallocate a destination buffer or select a previously
// allocated buffer from a pool.
func CompressBound(srcSize int) int {
	return 512 + srcSize + (srcSize >> 7) + 12
}

// cCompressBound is a cgo call to check the go implementation above against the c code.
func cCompressBound(srcSize int) int {
	return int(C.ZSTD_compressBound(C.size_t(srcSize)))
}

// getError returns an error for the return code, or nil if it's not an error
func getError(code int) error {
	if code < 0 && code >= -14 {
		return ErrorCode(code)
	}
	return nil
}

func cIsError(code int) bool {
	return int(C.ZSTD_isError(C.size_t(code))) != 0
}

// Compress src into dst.  If you have a buffer to use, you can pass it to
// prevent allocation.  If it is too small, or if nil is passed, a new buffer
// will be allocated and returned.
func Compress(dst, src []byte) ([]byte, error) {
	return CompressLevel(dst, src, DefaultCompression)
}

// CompressLevel is the same as Compress but you can pass a compression level
func CompressLevel(dst, src []byte, level int) ([]byte, error) {
	if len(src) == 0 {
		return []byte{}, ErrEmptySlice
	}
	bound := CompressBound(len(src))
	if cap(dst) >= bound {
		dst = dst[0:bound] // Reuse dst buffer
	} else {
		dst = make([]byte, bound)
	}

	cWritten := C.ZSTD_compress(
		unsafe.Pointer(&dst[0]),
		C.size_t(len(dst)),
		unsafe.Pointer(&src[0]),
		C.size_t(len(src)),
		C.int(level))

	written := int(cWritten)
	// Check if the return is an Error code
	if err := getError(written); err != nil {
		return nil, err
	}
	return dst[:written], nil
}

// Decompress src into dst.  If you have a buffer to use, you can pass it to
// prevent allocation.  If it is too small, or if nil is passed, a new buffer
// will be allocated and returned.  By default, len(src)  * 4 is allocated.
// If the buffer is too small, it will retry with a 2x sized dst buffer up to
// 3 times before falling back to the slower stream API (ie. if the compression
// ratio is 32).
func Decompress(dst, src []byte) ([]byte, error) {
	decompress := func(dst, src []byte) ([]byte, error) {

		cWritten := C.ZSTD_decompress(
			unsafe.Pointer(&dst[0]),
			C.size_t(len(dst)),
			unsafe.Pointer(&src[0]),
			C.size_t(len(src)))

		written := int(cWritten)
		// Check error
		if err := getError(written); err != nil {
			return nil, err
		}
		return dst[:written], nil
	}

	if dst == nil {
		// x is the 95 percentile compression ratio of zstd on points.mlti payloads
		dst = make([]byte, len(src)*3)
	}
	for i := 0; i < 3; i++ { // 3 tries to allocate a bigger buffer
		result, err := decompress(dst, src)
		if err != ErrDstSizeTooSmall {
			return result, err
		}
		dst = make([]byte, len(dst)*2) // Grow buffer by 2
	}
	// We failed getting a dst buffer of correct size, use stream API
	r := NewReader(bytes.NewReader(src))
	defer r.Close()
	return ioutil.ReadAll(r)
}
