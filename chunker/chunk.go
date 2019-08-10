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
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/x"
	"github.com/dgraph-io/dgraph/chunker/json"

	"github.com/pkg/errors"
)

type chunk struct {
	Offset int
	End    int
}

// Chunker describes the interface to parse and process the input to the live and bulk loaders.
type Chunker interface {
	Begin(buf []byte) error
	Chunk(buf []byte) (*chunk, error)
	End(buf []byte) error
	Parse(chunkBuf []byte) ([]*api.NQuad, error)
}

//type rdfChunker struct{}
type jsonChunker struct {
	offset int
	limit  int
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
func NewChunker(inputFormat InputFormat) Chunker {
	switch inputFormat {
	/*	case RdfFormat:
		return &rdfChunker{}
	*/
	case JsonFormat:
		return &jsonChunker{}
	default:
		panic("unknown input format")
	}
}

/*
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
	//batch.Grow(1 << 20)
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

// RDF files don't require any special processing at the End of the file.
func (rdfChunker) End(r *bufio.Reader) error {
	return nil
}

*/
func (j *jsonChunker) Begin(buf []byte) error {
	j.limit = len(buf)
	return nil
}

func (j *jsonChunker) nextRune(buf []byte) (byte, error) {
	if j.offset >= j.limit {
		return 0, io.EOF
	}
	b := buf[j.offset]
	j.offset++
	return b, nil
}

func (j *jsonChunker) Chunk(buf []byte) (*chunk, error) {
	//out := new(bytes.Buffer)
	//out.Grow(1 << 20)

	// For RDF, the loader just reads the input and the mapper parses it into nquads,
	// so do the same for JSON. But since JSON is not line-oriented like RDF, it's a little
	// more complicated to ensure a complete JSON structure is read.
	j.slurpSpace(buf)

	beginOffset := j.offset
	beginCh, err := j.nextRune(buf)
	if err != nil {
		return nil, err
	}

	var enclosingCh byte
	switch beginCh {
	case '{':
		enclosingCh = '}'
	case '[':
		enclosingCh = ']'
	default:
		return nil, errors.Errorf("The JSON file must begin with { or [, found %v: chunker: %+v",
			rune(beginCh), j)
	}

	// Just find the matching closing brace. Let the JSON-to-nquad parser in the mapper worry
	// about whether everything in between is valid JSON or not.
	depth := 1 // We already consumed one `{`, so our depth starts at one.
	for depth > 0 {
		ch, err := j.nextRune(buf)
		if err != nil {
			return nil, errors.New("Malformed JSON")
		}

		switch ch {
		case beginCh:
			depth++
		case enclosingCh:
			depth--
		case '"':
			if err := j.slurpQuoted(buf); err != nil {
				return nil, err
			}
		default:
			// We just write the rune to out, and let the Go JSON parser do its job.
		}
	}

	return &chunk{
		Offset: beginOffset,
		End:    j.offset,
	}, nil
}

func (jsonChunker) Parse(chunkBuf []byte) ([]*api.NQuad, error) {
	if len(chunkBuf) == 0 {
		return nil, io.EOF
	}

	nqs, err := json.Parse(chunkBuf, json.SetNquads)
	if err != nil && err != io.EOF {
		x.Check(err)
	}

	return nqs, err
}

func (j *jsonChunker) End(buf []byte) error {
	if j.slurpSpace(buf) == io.EOF {
		return nil
	}
	return errors.New("Not all of JSON file consumed")
}

func (j *jsonChunker) slurpSpace(buf []byte) error {
	for ; j.offset < j.limit; j.offset++ {
		ch := buf[j.offset]
		if !unicode.IsSpace(rune(ch)) {
			return nil
		}
	}
	return io.EOF
}

func (j *jsonChunker) slurpQuoted(buf []byte) error {
	for {
		ch, err := j.nextRune(buf)
		if err != nil {
			return err
		}

		if ch == '\\' {
			// Pick one more rune.
			_, err := j.nextRune(buf)
			if err != nil {
				return err
			}
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
func FileReader(file string) (buf []byte, cleanup func()) {
	var f *os.File
	var err error
	if file == "-" {
		f = os.Stdin
	} else {
		f, err = os.Open(file)
	}

	x.Check(err)

	cleanup = func() { f.Close() }

	/*
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
	*/

	//return rd, cleanup
	fileInfo, err := f.Stat()
	x.Check(err)
	bytes, err := y.Mmap(f, false, fileInfo.Size())
	x.Check(err)
	return bytes, cleanup
}

/*
// IsJSONData returns true if the reader, which should be at the start of the stream, is reading
// a JSON stream, false otherwise.
func IsJSONData(buf []byte) (bool, error) {
	buf, err := r.Peek(512)
	if err != nil && err != io.EOF {
		return false, err
	}

	de := encjson.NewDecoder(bytes.NewReader(buf))
	_, err = de.Token()

	return err == nil, nil
}
*/

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
