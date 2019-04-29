/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"bytes"
	"compress/gzip"
	encjson "encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/x"
	"github.com/dgraph-io/dgraph/chunker/json"
	"github.com/dgraph-io/dgraph/chunker/rdf"

	"github.com/pkg/errors"
)

type Chunker interface {
	Begin(r *bufio.Reader) error
	Chunk(r *bufio.Reader) (*bytes.Buffer, error)
	End(r *bufio.Reader) error
	Parse(chunkBuf *bytes.Buffer) ([]*api.NQuad, error)
}

type rdfChunker struct{}
type jsonChunker struct{}

const (
	UnknownFormat int = iota
	RdfFormat
	JsonFormat
)

func NewChunker(inputFormat int) Chunker {
	switch inputFormat {
	case RdfFormat:
		return &rdfChunker{}
	case JsonFormat:
		return &jsonChunker{}
	default:
		panic("unknown input format")
	}
}

// RDF files don't require any special processing at the beginning of the file.
func (rdfChunker) Begin(r *bufio.Reader) error {
	return nil
}

// Chunk reads the input line by line until one of the following 3 conditions happens
// 1) the EOF is reached
// 2) 1e5 lines have been read
// 3) some unexpected error happened
func (rdfChunker) Chunk(r *bufio.Reader) (*bytes.Buffer, error) {
	batch := new(bytes.Buffer)
	batch.Grow(1 << 20)
	for lineCount := 0; lineCount < 1e5; lineCount++ {
		slc, err := r.ReadSlice('\n')
		if err == io.EOF {
			batch.Write(slc)
			return batch, err
		}
		if err == bufio.ErrBufferFull {
			// This should only happen infrequently.
			batch.Write(slc)
			var str string
			str, err = r.ReadString('\n')
			if err == io.EOF {
				batch.WriteString(str)
				return batch, err
			}
			if err != nil {
				return nil, err
			}
			batch.WriteString(str)
			continue
		}
		if err != nil {
			return nil, err
		}
		batch.Write(slc)
	}
	return batch, nil
}

func (rdfChunker) Parse(chunkBuf *bytes.Buffer) ([]*api.NQuad, error) {
	if chunkBuf.Len() == 0 {
		return nil, io.EOF
	}

	nqs := make([]*api.NQuad, 0)
	for chunkBuf.Len() > 0 {
		str, err := chunkBuf.ReadString('\n')
		if err != nil && err != io.EOF {
			x.Check(err)
		}

		nq, err := rdf.Parse(str)
		if err == rdf.ErrEmpty {
			continue // blank line or comment
		} else if err != nil {
			return nil, errors.Wrapf(err, "while parsing line %q", str)
		}
		nqs = append(nqs, &nq)
	}

	return nqs, nil
}

// RDF files don't require any special processing at the end of the file.
func (rdfChunker) End(r *bufio.Reader) error {
	return nil
}

func (jsonChunker) Begin(r *bufio.Reader) error {
	// The JSON file to load must be an array of maps (that is, '[ { ... }, { ... }, ... ]').
	// This function must be called before calling readJSONChunk for the first time to advance
	// the Reader past the array start token ('[') so that calls to readJSONChunk can read
	// one array element at a time instead of having to read the entire array into memory.
	if err := slurpSpace(r); err != nil {
		return err
	}
	ch, _, err := r.ReadRune()
	if err != nil {
		return err
	}
	if ch != '[' {
		return fmt.Errorf("JSON file must contain array. Found: %v", ch)
	}
	return nil
}

func (jsonChunker) Chunk(r *bufio.Reader) (*bytes.Buffer, error) {
	out := new(bytes.Buffer)
	out.Grow(1 << 20)

	// For RDF, the loader just reads the input and the mapper parses it into nquads,
	// so do the same for JSON. But since JSON is not line-oriented like RDF, it's a little
	// more complicated to ensure a complete JSON structure is read.
	if err := slurpSpace(r); err != nil {
		return out, err
	}
	ch, _, err := r.ReadRune()
	if err != nil {
		return out, err
	}
	if ch == ']' {
		// Handle loading an empty JSON array ("[]") without error.
		return nil, io.EOF
	} else if ch != '{' {
		return nil, fmt.Errorf("Expected JSON map start. Found: %v", string(ch))
	}
	x.Check2(out.WriteRune(ch))

	// Just find the matching closing brace. Let the JSON-to-nquad parser in the mapper worry
	// about whether everything in between is valid JSON or not.
	depth := 1 // We already consumed one `{`, so our depth starts at one.
	for depth > 0 {
		ch, _, err = r.ReadRune()
		if err != nil {
			return nil, errors.New("Malformed JSON")
		}
		x.Check2(out.WriteRune(ch))

		switch ch {
		case '{':
			depth++
		case '}':
			depth--
		case '"':
			if err := slurpQuoted(r, out); err != nil {
				return nil, err
			}
		default:
			// We just write the rune to out, and let the Go JSON parser do its job.
		}
	}

	// The map should be followed by either the ',' between array elements, or the ']'
	// at the end of the array.
	if err := slurpSpace(r); err != nil {
		return nil, err
	}
	ch, _, err = r.ReadRune()
	if err != nil {
		return nil, err
	}
	switch ch {
	case ']':
		return out, io.EOF
	case ',':
		// pass
	default:
		// Let next call to this function report the error.
		x.Check(r.UnreadRune())
	}
	return out, nil
}

func (jsonChunker) Parse(chunkBuf *bytes.Buffer) ([]*api.NQuad, error) {
	if chunkBuf.Len() == 0 {
		return nil, io.EOF
	}

	nqs, err := json.Parse(chunkBuf.Bytes(), json.SetNquads)
	if err != nil && err != io.EOF {
		x.Check(err)
	}
	chunkBuf.Reset()

	return nqs, err
}

func (jsonChunker) End(r *bufio.Reader) error {
	if slurpSpace(r) == io.EOF {
		return nil
	}
	return errors.New("Not all of JSON file consumed")
}

func slurpSpace(r *bufio.Reader) error {
	for {
		ch, _, err := r.ReadRune()
		if err != nil {
			return err
		}
		if !unicode.IsSpace(ch) {
			x.Check(r.UnreadRune())
			return nil
		}
	}
}

func slurpQuoted(r *bufio.Reader, out *bytes.Buffer) error {
	for {
		ch, _, err := r.ReadRune()
		if err != nil {
			return err
		}
		x.Check2(out.WriteRune(ch))

		if ch == '\\' {
			// Pick one more rune.
			esc, _, err := r.ReadRune()
			if err != nil {
				return err
			}
			x.Check2(out.WriteRune(esc))
			continue
		}
		if ch == '"' {
			return nil
		}
	}
}

// FileReader returns an open reader and file on the given file. Gzip-compressed input is detected
// and decompressed automatically even without the gz extension. The caller is responsible for
// calling the returned cleanup function when done with the reader.
func FileReader(file string) (rd *bufio.Reader, cleanup func()) {
	var f *os.File
	var err error
	if file == "-" {
		f = os.Stdin
	} else {
		f, err = os.Open(file)
	}

	x.Check(err)

	cleanup = func() { f.Close() }

	if filepath.Ext(file) == ".gz" {
		gzr, err := gzip.NewReader(f)
		x.Check(err)
		rd = bufio.NewReader(gzr)
		cleanup = func() { f.Close(); gzr.Close() }
	} else {
		rd = bufio.NewReader(f)
		buf, _ := rd.Peek(512)

		typ := http.DetectContentType(buf)
		if typ == "application/x-gzip" {
			gzr, err := gzip.NewReader(rd)
			x.Check(err)
			rd = bufio.NewReader(gzr)
			cleanup = func() { f.Close(); gzr.Close() }
		}
	}

	return rd, cleanup
}

// IsJSONData returns true if the reader, which should be at the start of the stream, is reading
// a JSON stream, false otherwise.
func IsJSONData(r *bufio.Reader) (bool, error) {
	buf, err := r.Peek(512)
	if err != nil && err != io.EOF {
		return false, err
	}

	de := encjson.NewDecoder(bytes.NewReader(buf))
	_, err = de.Token()

	return err == nil, nil
}

// DataFormat returns a file's data format (RDF, JSON, or unknown) based on the filename
// or the user-provided format option. The file extension has precedence.
func DataFormat(filename string, format string) int {
	format = strings.ToLower(format)
	filename = strings.TrimSuffix(strings.ToLower(filename), ".gz")
	switch {
	case strings.HasSuffix(filename, ".rdf") || format == "rdf":
		return RdfFormat
	case strings.HasSuffix(filename, ".json") || format == "json":
		return JsonFormat
	default:
		return UnknownFormat
	}
}
