/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package chunker

import (
	"bufio"
	"compress/gzip"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dgraph-io/dgraph/x"
)

// chunk.Reader wraps a bufio.Reader to hold additional information
// about the file being read.
// XXX need to check how reliable offset value is
type Reader struct {
	reader     *bufio.Reader
	offset     uint64 // start of file is at offset 0
	line       uint32 // first line is number 0
	compressed bool
	filename   string

	// these are used to handle UnreadRune
	prevOffset uint64
	prevLine   uint32
}

//
// TODO Unexport names?
//

// NewReader returns an open reader and file on the given file. Gzip-compressed input is detected
// and decompressed automatically even without the gz extension. The caller is responsible for
// calling the returned cleanup function when done with the reader.
func NewReader(file string) (*Reader, func()) {
	var f *os.File
	var err error
	if file == "-" {
		f, file = os.Stdin, "/dev/stdin"
	} else {
		f, err = os.Open(file)
	}

	x.Check(err)

	var rd = Reader{filename: file}
	var cleanup = func() { f.Close() }

	if filepath.Ext(file) == ".gz" {
		gzr, err := gzip.NewReader(f)
		x.CheckfNoTrace(err)
		rd.reader = bufio.NewReader(gzr)
		rd.compressed = true
		cleanup = func() { f.Close(); gzr.Close() }
	} else {
		rd.reader = bufio.NewReader(f)
		buf, _ := rd.reader.Peek(512)

		typ := http.DetectContentType(buf)
		if typ == "application/x-gzip" {
			gzr, err := gzip.NewReader(rd.reader)
			x.Check(err)
			rd.reader = bufio.NewReader(gzr)
			rd.compressed = true
			cleanup = func() { f.Close(); gzr.Close() }
		}
	}

	return &rd, cleanup
}

// Offset returns the current position of the reader in the file or stream. Or alternatively,
// returns the number of bytes that have been read.
func (r *Reader) Offset() uint64 {
	return r.offset
}

// LineNumber returns the number of newlines that have been read.
func (r *Reader) LineNumber() uint32 {
	return r.line
}

//
// TODO check for corner cases in reader functions
//

func (r *Reader) ReadSlice(delim byte) ([]byte, error) {
	r.prevOffset, r.prevLine = r.offset, r.line

	line, err := r.reader.ReadSlice(delim)
	r.offset += uint64(len(line))
	r.line += 1

	return line, err
}

func (r *Reader) ReadString(delim byte) (string, error) {
	r.prevOffset, r.prevLine = r.offset, r.line

	str, err := r.reader.ReadString(delim)
	r.offset += uint64(len(str))
	r.line += 1

	return str, err
}

func (r *Reader) ReadRune() (rune, int, error) {
	r.prevOffset, r.prevLine = r.offset, r.line

	char, size, err := r.reader.ReadRune()
	r.offset += uint64(size)
	if char == '\n' {
		r.line += 1
	}
	return char, size, err
}

func (r *Reader) UnreadRune() error {
	if r.prevOffset == 0 {
		return bufio.ErrInvalidUnreadRune
	}

	r.offset, r.line = r.prevOffset, r.prevLine
	r.prevOffset, r.prevLine = 0, 0
	return r.reader.UnreadRune()
}
