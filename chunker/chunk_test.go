/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func bufioReader(str string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(str))
}

// Test that problems at the start of the JSON document are caught.
func TestJSONLoadStart(t *testing.T) {
	var tests = []struct {
		json string
		desc string
	}{
		{"", "file is empty"},
		{"  \t   ", "file is white space"},
		{"These are words.", "file is not JSON"},
		{"\x1f\x8b\x08\x08\x3e\xc7\x0a\x5c\x00\x03\x65\x6d\x70\x74\x79\x00", "file is binary"},
	}

	for _, test := range tests {
		chunker := NewChunker(JsonFormat, 1000)
		require.Error(t, chunker.Begin(bufioReader(test.json)), test.desc)
	}
}

func TestChunkJSONMapAndArray(t *testing.T) {
	tests := []struct {
		json   string
		chunks []string
	}{
		{`[]`, []string{"<nil>"}},
		{`[{}]`, []string{"{}"}},
		{`[{"user": "alice"}]`, []string{`{"user": "alice"}`}},
		{`[{"user": "alice", "age", 26}]`, []string{`{"user": "alice", "age", 26}`}},
		{`[{"user": "alice", "age", 26}, {"name": "bob"}]`, []string{`{"user": "alice", "age", 26}`,
			`{"name": "bob"}`}},
	}

	for _, test := range tests {
		chunker := NewChunker(JsonFormat, 1000)
		r := bufioReader(test.json)
		require.NoError(t, chunker.Begin(r))

		var chunks []string
		for {
			chunkBuf, err := chunker.Chunk(r)
			if err != nil && err != io.EOF {
				t.FailNow()
			}

			chunks = append(chunks, chunkBuf.String())

			if err == io.EOF {
				require.NoError(t, chunker.End(r), "Got an error while validating end of reader")
				break
			}
		}

		require.Equal(t, test.chunks, chunks, "Got different chunks")
	}
}

// Test that problems at the start of the next chunk are caught.
func TestJSONLoadReadNext(t *testing.T) {
	var tests = []struct {
		json string
		desc string
	}{
		{"[,]", "no start of JSON map 1"},
		{"[ this is not really a json array ]", "no start of JSON map 2"},
		{"[{]", "malformed map"},
		{"[{}", "malformed array"},
	}
	for _, test := range tests {
		chunker := NewChunker(JsonFormat, 1000)
		reader := bufioReader(test.json)
		require.NoError(t, chunker.Begin(reader), test.desc)

		chunkBuf, err := chunker.Chunk(reader)
		if err == nil {
			err = chunker.Parse(chunkBuf)
			require.Error(t, err, test.desc)
		} else {
			require.Error(t, err, test.desc)
		}
	}
}

// Test that loading first chunk succeeds. No need to test that loaded chunk is valid.
func TestJSONLoadSuccessFirst(t *testing.T) {
	var tests = []struct {
		json string
		expt string
		desc string
	}{
		{"[{}]", "{}", "empty map"},
		{`[{"closingDelimeter":"}"}]`, `{"closingDelimeter":"}"}`, "quoted closing brace"},
		{`[{"company":"dgraph"}]`, `{"company":"dgraph"}`, "simple, compact map"},
		{
			"[\n  {\n    \"company\" : \"dgraph\"\n  }\n]\n",
			"{\n    \"company\" : \"dgraph\"\n  }",
			"simple, pretty map",
		},
		{
			`[{"professor":"Alastor \"Mad-Eye\" Moody"}]`,
			`{"professor":"Alastor \"Mad-Eye\" Moody"}`,
			"escaped balanced quotes",
		},
		{

			`[{"something{": "}something"}]`,
			`{"something{": "}something"}`,
			"escape quoted brackets",
		},
		{
			`[{"height":"6'0\""}]`,
			`{"height":"6'0\""}`,
			"escaped unbalanced quote",
		},
		{
			`[{"house":{"Hermione":"Gryffindor","Cedric":"Hufflepuff","Luna":"Ravenclaw","Draco":"Slytherin",}}]`,
			`{"house":{"Hermione":"Gryffindor","Cedric":"Hufflepuff","Luna":"Ravenclaw","Draco":"Slytherin",}}`,
			"nested braces",
		},
	}
	for _, test := range tests {
		chunker := NewChunker(JsonFormat, 1000)
		reader := bufioReader(test.json)
		require.NoError(t, chunker.Begin(reader), test.desc)

		json, err := chunker.Chunk(reader)
		if err == io.EOF {
			// pass
		} else {
			require.NoError(t, err, test.desc)
		}
		//fmt.Fprintf(os.Stderr, "err = %v, json = %v\n", err, json)
		require.Equal(t, test.expt, json.String(), test.desc)
	}
}

// Test that loading all chunks succeeds. No need to test that loaded chunk is valid.
func TestJSONLoadSuccessAll(t *testing.T) {
	var testDoc = `
[
	{},
	{
		"closingDelimeter" : "}"
	},
	{
		"company" : "dgraph",
		"age": 3
	},
	{
		"professor" : "Alastor \"Mad-Eye\" Moody",
		"height"    : "6'0\""
	},
	{
		"house" : {
			"Hermione" : "Gryffindor",
			"Cedric"   : "Hufflepuff",
			"Luna"     : "Ravenclaw",
			"Draco"    : "Slytherin"
		}
	}
]`
	var testChunks = []string{
		`{}`,
		`{
		"closingDelimeter" : "}"
	}`,
		`{
		"company" : "dgraph",
		"age": 3
	}`,
		`{
		"professor" : "Alastor \"Mad-Eye\" Moody",
		"height"    : "6'0\""
	}`,
		`{
		"house" : {
			"Hermione" : "Gryffindor",
			"Cedric"   : "Hufflepuff",
			"Luna"     : "Ravenclaw",
			"Draco"    : "Slytherin"
		}
	}`,
	}

	chunker := NewChunker(JsonFormat, 1000)
	reader := bufioReader(testDoc)

	var json *bytes.Buffer
	var idx int

	err := chunker.Begin(reader)
	require.NoError(t, err, "begin reading JSON document")
	for idx = 0; err == nil; idx++ {
		desc := fmt.Sprintf("reading chunk #%d", idx+1)
		json, err = chunker.Chunk(reader)
		//fmt.Fprintf(os.Stderr, "err = %v, json = %v\n", err, json)
		if err != io.EOF {
			require.NoError(t, err, desc)
			require.Equal(t, testChunks[idx], json.String(), desc)
		}
	}
	err = chunker.End(reader)
	require.NoError(t, err, "end reading JSON document")
}
