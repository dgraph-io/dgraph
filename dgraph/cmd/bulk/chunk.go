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

package bulk

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type chunker interface {
	begin(r *bufio.Reader) error
	chunk(r *bufio.Reader) (*bytes.Buffer, error)
	end(r *bufio.Reader) error
	parse(chunkBuf *bytes.Buffer) ([]gql.NQuad, error)
}

type rdfChunker struct{}
type jsonChunker struct{}

const (
	rdfInput int = iota
	jsonInput
)

func newChunker(inputFormat int) chunker {
	switch inputFormat {
	case rdfInput:
		return &rdfChunker{}
	case jsonInput:
		return &jsonChunker{}
	default:
		panic("unknown loader type")
	}
}

func (_ rdfChunker) begin(r *bufio.Reader) error {
	return nil
}

func (_ rdfChunker) chunk(r *bufio.Reader) (*bytes.Buffer, error) {
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

func (_ rdfChunker) end(r *bufio.Reader) error {
	return nil
}

func (_ rdfChunker) parse(chunkBuf *bytes.Buffer) ([]gql.NQuad, error) {
	str, readErr := chunkBuf.ReadString('\n')
	if readErr != nil && readErr != io.EOF {
		x.Check(readErr)
	}

	nq, parseErr := rdf.Parse(strings.TrimSpace(str))
	if parseErr == rdf.ErrEmpty {
		return nil, readErr
	} else if parseErr != nil {
		return nil, errors.Wrapf(parseErr, "while parsing line %q", str)
	}

	return []gql.NQuad{gql.NQuad{NQuad: &nq}}, readErr
}

func slurpSpace(r *bufio.Reader) error {
	for {
		ch, _, err := r.ReadRune()
		if err != nil {
			return err
		}
		if !unicode.IsSpace(ch) {
			r.UnreadRune()
			break
		}
	}
	return nil
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
			if esc, _, err := r.ReadRune(); err != nil {
				return err
			} else {
				x.Check2(out.WriteRune(esc))
				continue
			}
		}
		if ch == '"' {
			return nil
		}
	}
}

func (_ jsonChunker) begin(r *bufio.Reader) error {
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
		return fmt.Errorf("json file must contain array. Found: %v", ch)
	}
	return nil
}

func (_ jsonChunker) chunk(r *bufio.Reader) (*bytes.Buffer, error) {
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
	if ch != '{' {
		return nil, fmt.Errorf("expected json map start. Found: %v", ch)
	}
	x.Check2(out.WriteRune(ch))

	// Just find the matching closing brace. Let the JSON-to-nquad parser in the mapper worry
	// about whether everything in between is valid JSON or not.

	depth := 1 // We already consumed one `{`, so our depth starts at one.
	for depth > 0 {
		ch, _, err = r.ReadRune()
		if err != nil {
			return nil, errors.New("malformed json")
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

func (_ jsonChunker) end(r *bufio.Reader) error {
	if slurpSpace(r) == io.EOF {
		return nil
	} else {
		return errors.New("not all of json file consumed")
	}
}

func (_ jsonChunker) parse(chunkBuf *bytes.Buffer) ([]gql.NQuad, error) {
	if chunkBuf.Len() == 0 {
		return nil, io.EOF
	}

	nqs, err := edgraph.NquadsFromJson(chunkBuf.Bytes())
	if err != nil && err != io.EOF {
		x.Check(err)
	}
	chunkBuf.Reset()

	gqlNq := make([]gql.NQuad, len(nqs))
	for i, nq := range nqs {
		gqlNq[i] = gql.NQuad{NQuad: nq}
	}
	return gqlNq, err
}
