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

package chunk

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReaderRdf(t *testing.T) {
	ck := NewChunker(RdfFormat)
	rd, fn := NewReader("testdata/data.rdf")
	defer fn()

	// ensure small test file is read in more than one chunk
	maxRdfLines = 10

	var chunks = []struct{ line, pos int }{
		{0, 0},
		{10, 321},
		{20, 640},
		{30, 935},
		{40, 1289},
	}

	var chunk *Chunk
	var err error
	for _, expected := range chunks {
		chunk, err = ck.ChunkNew(rd)
		require.True(t, err == nil || err == io.EOF)
		require.Equal(t, expected.line, chunk.LinePos)
		require.Equal(t, expected.pos, chunk.BytePos)
	}
	require.EqualError(t, err, io.EOF.Error())
}
