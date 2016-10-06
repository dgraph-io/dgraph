/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package schema

import (
	"bytes"

	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/x"
)

// IndexKeyGenerator generates the keys used for indexing an attribute.
type IndexKeyGenerator interface {
	// IndexKeys returns the index keys to be used to index the given attribute.
	IndexKeys(attr string, data []byte) ([]string, error)
}

type exactMatchKeyGen struct{}

func (e exactMatchKeyGen) IndexKeys(attr string, data []byte) ([]string, error) {
	return []string{string(IndexKey(attr, data))}, nil
}

var (
	exactMatch exactMatchKeyGen
	geoKeyGen  geo.KeyGenerator
)

const (
	// IndexRune : Posting list keys are prefixed with this rune if it is a mutation meant for the
	// index.
	IndexRune = ':'
)

// IndexKey creates a key for indexing the term for given attribute.
func IndexKey(attr string, term []byte) []byte {
	buf := bytes.NewBuffer(make([]byte, 0, len(attr)+len(term)+2))
	_, err := buf.WriteRune(IndexRune)
	x.Check(err)
	_, err = buf.WriteString(attr)
	x.Check(err)
	_, err = buf.WriteRune('|')
	x.Check(err)
	_, err = buf.Write(term)
	x.Check(err)
	return buf.Bytes()
}
