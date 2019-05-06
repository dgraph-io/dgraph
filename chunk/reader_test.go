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

type position struct {
	off  int
	line int
}

func checkChunks(t *testing.T, ck Chunker, rd *Reader, chunks []position) {
	var chunk *Chunk
	var err error
	for _, expected := range chunks {
		chunk, err = ck.Chunk(rd)
		//t.Logf("ERR=%+v CHUNK@%d/%d=%+v", err, chunk.BytePos, chunk.LinePos, chunk)
		require.True(t, err == nil || err == io.EOF,
			"Chunk() unexpected error: %+v", err)
		require.Equal(t, expected.line, chunk.LinePos,
			"incorrect line number")
		require.Equal(t, expected.off, chunk.BytePos,
			"incorrect offset")
	}
	require.EqualError(t, err, io.EOF.Error())
}

func TestReaderRdf(t *testing.T) {
	ck := NewChunker(RdfFormat)
	rd, fn := NewReader("testdata/data.rdf")
	defer fn()

	// ensure small test file is read in more than one chunk
	maxRdfLines = 10

	var chunks = []position{
		{0, 0},
		{321, 10},
		{640, 20},
		{935, 30},
		{1289, 40},
	}
	checkChunks(t, ck, rd, chunks)
}

func TestReaderJsonPretty(t *testing.T) {
	ck := NewChunker(JsonFormat)
	rd, fn := NewReader("testdata/pretty.json")
	defer fn()

	var chunks = []position{
		{4, 1},
		{282, 16},
		{579, 32},
		{878, 48},
		{1164, 63},
		{1474, 79},
	}
	require.NoError(t, ck.Begin(rd))
	checkChunks(t, ck, rd, chunks)
	require.NoError(t, ck.End(rd))
}

func TestReaderJsonUgly(t *testing.T) {
	ck := NewChunker(JsonFormat)
	rd, fn := NewReader("testdata/ugly.json")
	defer fn()

	// FIXME complete test
}
