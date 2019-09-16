package zstd

/*
#include "zstd.h"
#include "zstd_buffered.h"
*/
import "C"
import (
	"bytes"
	"fmt"
	"io"
	"unsafe"
)

// Writer is an io.WriteCloser that zstd-compresses its input.
type Writer struct {
	CompressionLevel int

	ctx              *C.ZSTD_CCtx
	dict             []byte
	dstBuffer        []byte
	firstError       error
	underlyingWriter io.Writer
}

func resize(in []byte, newSize int) []byte {
	if in == nil {
		return make([]byte, newSize)
	}
	if newSize <= cap(in) {
		return in[:newSize]
	}
	toAdd := newSize - len(in)
	return append(in, make([]byte, toAdd)...)
}

// NewWriter creates a new Writer with default compression options.  Writes to
// the writer will be written in compressed form to w.
func NewWriter(w io.Writer) *Writer {
	return NewWriterLevelDict(w, DefaultCompression, nil)
}

// NewWriterLevel is like NewWriter but specifies the compression level instead
// of assuming default compression.
//
// The level can be DefaultCompression or any integer value between BestSpeed
// and BestCompression inclusive.
func NewWriterLevel(w io.Writer, level int) *Writer {
	return NewWriterLevelDict(w, level, nil)

}

// NewWriterLevelDict is like NewWriterLevel but specifies a dictionary to
// compress with.  If the dictionary is empty or nil it is ignored. The dictionary
// should not be modified until the writer is closed.
func NewWriterLevelDict(w io.Writer, level int, dict []byte) *Writer {
	var err error
	ctx := C.ZSTD_createCCtx()

	if dict == nil {
		err = getError(int(C.ZSTD_compressBegin(ctx,
			C.int(level))))
	} else {
		err = getError(int(C.ZSTD_compressBegin_usingDict(
			ctx,
			unsafe.Pointer(&dict[0]),
			C.size_t(len(dict)),
			C.int(level))))
	}

	return &Writer{
		CompressionLevel: level,
		ctx:              ctx,
		dict:             dict,
		dstBuffer:        make([]byte, CompressBound(1024)),
		firstError:       err,
		underlyingWriter: w,
	}
}

// Write writes a compressed form of p to the underlying io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	if w.firstError != nil {
		return 0, w.firstError
	}
	if len(p) == 0 {
		return 0, nil
	}
	// Check if dstBuffer is enough
	if len(w.dstBuffer) < CompressBound(len(p)) {
		w.dstBuffer = make([]byte, CompressBound(len(p)))
	}

	retCode := C.ZSTD_compressContinue(
		w.ctx,
		unsafe.Pointer(&w.dstBuffer[0]),
		C.size_t(len(w.dstBuffer)),
		unsafe.Pointer(&p[0]),
		C.size_t(len(p)))

	if err := getError(int(retCode)); err != nil {
		return 0, err
	}
	written := int(retCode)

	// Write to underlying buffer
	_, err := w.underlyingWriter.Write(w.dstBuffer[:written])

	// Same behaviour as zlib, we can't know how much data we wrote, only
	// if there was an error
	if err != nil {
		return 0, err
	}
	return len(p), err
}

// Close closes the Writer, flushing any unwritten data to the underlying
// io.Writer and freeing objects, but does not close the underlying io.Writer.
func (w *Writer) Close() error {
	retCode := C.ZSTD_compressEnd(
		w.ctx,
		unsafe.Pointer(&w.dstBuffer[0]),
		C.size_t(len(w.dstBuffer)))

	if err := getError(int(retCode)); err != nil {
		return err
	}
	written := int(retCode)
	retCode = C.ZSTD_freeCCtx(w.ctx) // Safely close buffer before writing the end

	if err := getError(int(retCode)); err != nil {
		return err
	}

	_, err := w.underlyingWriter.Write(w.dstBuffer[:written])
	if err != nil {
		return err
	}
	return nil
}

// reader is an io.ReadCloser that decompresses when read from.
type reader struct {
	ctx                 *C.ZBUFF_DCtx
	compressionBuffer   []byte
	decompressionBuffer []byte
	dict                []byte
	firstError          error
	// Reuse previous bytes from source that were not consumed
	// Hopefully because we use recommended size, we will never need to use that
	srcBuffer          bytes.Buffer
	dstBuffer          bytes.Buffer
	recommendedSrcSize int
	underlyingReader   io.Reader
}

// NewReader creates a new io.ReadCloser.  Reads from the returned ReadCloser
// read and decompress data from r.  It is the caller's responsibility to call
// Close on the ReadCloser when done.  If this is not done, underlying objects
// in the zstd library will not be freed.
func NewReader(r io.Reader) io.ReadCloser {
	return NewReaderDict(r, nil)
}

// NewReaderDict is like NewReader but uses a preset dictionary.  NewReaderDict
// ignores the dictionary if it is nil.
func NewReaderDict(r io.Reader, dict []byte) io.ReadCloser {
	var err error
	ctx := C.ZBUFF_createDCtx()
	if len(dict) == 0 {
		err = getError(int(C.ZBUFF_decompressInit(ctx)))
	} else {
		err = getError(int(C.ZBUFF_decompressInitDictionary(
			ctx,
			unsafe.Pointer(&dict[0]),
			C.size_t(len(dict)))))
	}
	cSize := int(C.ZBUFF_recommendedDInSize())
	dSize := int(C.ZBUFF_recommendedDOutSize())
	if cSize <= 0 {
		panic(fmt.Errorf("ZBUFF_recommendedDInSize() returned invalid size: %v", cSize))
	}
	if dSize <= 0 {
		panic(fmt.Errorf("ZBUFF_recommendedDOutSize() returned invalid size: %v", dSize))
	}

	compressionBuffer := make([]byte, cSize)
	decompressionBuffer := make([]byte, dSize)
	return &reader{
		ctx:                 ctx,
		dict:                dict,
		compressionBuffer:   compressionBuffer,
		decompressionBuffer: decompressionBuffer,
		firstError:          err,
		recommendedSrcSize:  cSize,
		underlyingReader:    r,
	}
}

// Close frees the allocated C objects
func (r *reader) Close() error {
	return getError(int(C.ZBUFF_freeDCtx(r.ctx)))
}

func (r *reader) Read(p []byte) (int, error) {

	// If we already have enough bytes, return
	if r.dstBuffer.Len() >= len(p) {
		return r.dstBuffer.Read(p)
	}

	for r.dstBuffer.Len() < len(p) {
		// Populate src
		src := r.compressionBuffer
		reader := r.underlyingReader
		if r.srcBuffer.Len() != 0 {
			reader = io.MultiReader(&r.srcBuffer, r.underlyingReader)
		}
		n, err := io.ReadFull(reader, src)
		if err == io.EOF {
			break
		} else if err != nil && err != io.ErrUnexpectedEOF {
			return 0, fmt.Errorf("failed to read from underlying reader: %s", err)
		}
		src = src[:n]

		// C code
		cSrcSize := C.size_t(len(src))
		cDstSize := C.size_t(len(r.decompressionBuffer))
		retCode := int(C.ZBUFF_decompressContinue(
			r.ctx,
			unsafe.Pointer(&r.decompressionBuffer[0]),
			&cDstSize,
			unsafe.Pointer(&src[0]),
			&cSrcSize))

		if err = getError(retCode); err != nil {
			return 0, fmt.Errorf("failed to decompress: %s", err)
		}

		// Put everything in buffer
		if int(cSrcSize) < len(src) { // We did not read everything, put in buffer
			toSave := src[int(cSrcSize):]
			_, err = r.srcBuffer.Write(toSave)
			if err != nil {
				return 0, fmt.Errorf("failed to store temporary src buffer: %s", err)
			}
		}
		_, err = r.dstBuffer.Write(r.decompressionBuffer[:int(cDstSize)])
		if err != nil {
			return 0, fmt.Errorf("failed to store temporary result: %s", err)
		}

		// Resize buffers
		if retCode > 0 { // Hint for next src buffer size
			r.compressionBuffer = resize(r.compressionBuffer, retCode)
		} else { // Reset to recommended size
			r.compressionBuffer = resize(r.compressionBuffer, r.recommendedSrcSize)
		}
	}
	// Write to dst
	return r.dstBuffer.Read(p)
}
