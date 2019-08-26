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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/dgraph-io/dgo/x"
	"github.com/dgraph-io/dgraph/lex"

	"github.com/pkg/errors"
)

// Chunker describes the interface to parse and process the input to the live and bulk loaders.
type Chunker interface {
	Begin(r *bufio.Reader) error
	Chunk(r *bufio.Reader) (*bytes.Buffer, error)
	End(r *bufio.Reader) error
	Parse(chunkBuf *bytes.Buffer) error
	NQuads() *NQuadBuffer
}

type rdfChunker struct {
	lexer *lex.Lexer
	nqs   *NQuadBuffer
}

func (rc *rdfChunker) NQuads() *NQuadBuffer {
	return rc.nqs
}

type jsonChunker struct {
	nqs *NQuadBuffer
}

func (jc *jsonChunker) NQuads() *NQuadBuffer {
	return jc.nqs
}

// InputFormat represents the multiple formats supported by Chunker.
type InputFormat byte

const (
	// UnknownFormat is a constant to denote a format not supported by the bulk/live loaders.
	UnknownFormat InputFormat = iota
	// RdfFormat is a constant to denote the input to the live/bulk loader is in the RDF format.
	RdfFormat
	// JsonFormat is a constant to denote the input to the live/bulk loader is in the JSON format.
	JsonFormat
)

// NewChunker returns a new chunker for the specified format.
func NewChunker(inputFormat InputFormat, batchSize int) Chunker {
	switch inputFormat {
	case RdfFormat:
		return &rdfChunker{
			nqs:   NewNQuadBuffer(batchSize),
			lexer: &lex.Lexer{},
		}
	case JsonFormat:
		return &jsonChunker{
			nqs: NewNQuadBuffer(batchSize),
		}
	default:
		panic("unknown input format")
	}
}

// RDF files don't require any special processing at the beginning of the file.
func (*rdfChunker) Begin(r *bufio.Reader) error {
	return nil
}

// Chunk reads the input line by line until one of the following 3 conditions happens
// 1) the EOF is reached
// 2) 1e5 lines have been read
// 3) some unexpected error happened
func (*rdfChunker) Chunk(r *bufio.Reader) (*bytes.Buffer, error) {
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

// Parse is not thread-safe. Only call it serially, because it reuses lexer object.
func (rc *rdfChunker) Parse(chunkBuf *bytes.Buffer) error {
	if chunkBuf == nil || chunkBuf.Len() == 0 {
		return nil
	}

	for chunkBuf.Len() > 0 {
		str, err := chunkBuf.ReadString('\n')
		if err != nil && err != io.EOF {
			x.Check(err)
		}

		nq, err := ParseRDF(str, rc.lexer)
		if err == ErrEmpty {
			continue // blank line or comment
		} else if err != nil {
			return errors.Wrapf(err, "while parsing line %q", str)
		}
		rc.nqs.Push(&nq)
	}
	return nil
}

// RDF files don't require any special processing at the end of the file.
func (*rdfChunker) End(r *bufio.Reader) error {
	return nil
}

func (*jsonChunker) Begin(r *bufio.Reader) error {
	// The JSON file to load must be an array of maps (that is, '[ { ... }, { ... }, ... ]').
	// This function must be called before calling readJSONChunk for the first time to advance
	// the Reader past the array start token ('[') so that calls to readJSONChunk can read
	// one array element at a time instead of having to read the entire array into memory.
	if err := slurpSpace(r); err != nil {
		return err
	}
	chs, err := r.Peek(1)

	if err != nil {
		return err
	}
	switch chs[0] {
	case '[':
		// consume the [ so that each Chunk call consumes a map
		_, _, err := r.ReadRune()
		if err != nil {
			return err
		}
	case '{':
		// do not consume so that the next Chunk call consumes the top level map
	default:
		return errors.Errorf("JSON file must begin with [ or {. Found: %v(%c)", chs[0], chs[0])
	}

	return nil
}

// Chunk consumes a JSON map from the reader, and it also assumes that the reader's content
// must begin with a map.
func (*jsonChunker) Chunk(r *bufio.Reader) (*bytes.Buffer, error) {
	out := new(bytes.Buffer)
	// We used to grow the buffer here just like in the RDF chunker, but
	// this caused a lot of allocations and GC cycles and impacted live loader throughput.

	if err := slurpSpace(r); err != nil {
		return nil, err
	}

	// Just find the matching closing brace. Let the JSON-to-nquad parser in the mapper worry
	// about whether everything in between is valid JSON or not.
	depth := 0
	for {
		ch, _, err := r.ReadRune()
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

		if depth <= 0 {
			break
		}
	}

	// The map should be followed by either the ',' between array elements, or the ']'
	// at the end of the array, or EOF if the map is at the root level.
	if err := slurpSpace(r); err != nil {
		return nil, err
	}

	ch, _, err := r.ReadRune()

	if err == io.EOF {
		// handles the EOF case, return out which represents the top level map
		return out, err
	} else if err != nil {
		return nil, err
	}

	switch ch {
	case ']':
		return out, io.EOF
	case ',':
		// pass
	default:
		return nil, errors.Errorf("JSON map is followed by illegal rune %v(%c)", ch, ch)
	}
	return out, nil
}

func (jc *jsonChunker) Parse(chunkBuf *bytes.Buffer) error {
	if chunkBuf == nil || chunkBuf.Len() == 0 {
		return nil
	}

	err := jc.nqs.ParseJSON(chunkBuf.Bytes(), SetNquads)
	return err
}

func (*jsonChunker) End(r *bufio.Reader) error {
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
func DataFormat(filename string, format string) InputFormat {
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
