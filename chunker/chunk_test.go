/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package chunker

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
		{"[,]", "Illegal rune found \",\", expecting {"},
		{"[a]", "Illegal rune found \"a\", expecting {"},
		{"{}]", "JSON map is followed by an extraneous ]"},
		{"These are words.", "file is not JSON"},
		{"\x1f\x8b\x08\x08\x3e\xc7\x0a\x5c\x00\x03\x65\x6d\x70\x74\x79\x00", "file is binary"},
	}

	for _, test := range tests {
		chunker := NewChunker(JsonFormat, 1000)
		_, err := chunker.Chunk(bufioReader(test.json))
		require.True(t, err != nil && err != io.EOF, test.desc)
	}
}

func TestChunkJSONMapAndArray(t *testing.T) {
	tests := []struct {
		json   string
		chunks []string
	}{
		{`[]`, []string{"[]"}},
		{`[{}]`, []string{"[{}]"}},
		{`[{"user": "alice"}]`, []string{`[{"user":"alice"}]`}},
		{`[{"user": "alice", "age": 26}]`, []string{`[{"user":"alice","age":26}]`}},
		{`[{"user": "alice", "age": 26}, {"name": "bob"}]`, []string{`[{"user":"alice","age":26},{"name":"bob"}]`}},
	}

	for _, test := range tests {
		chunker := NewChunker(JsonFormat, 1000)
		r := bufioReader(test.json)
		var chunks []string
		for {
			chunkBuf, err := chunker.Chunk(r)
			if err != nil {
				require.Equal(t, io.EOF, err, "Received error for %s", test)
			}

			chunks = append(chunks, chunkBuf.String())

			if err == io.EOF {
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
		chunkBuf, err := chunker.Chunk(reader)
		if err == nil {
			err = chunker.Parse(chunkBuf)
			require.True(t, err != nil && err != io.EOF, test.desc)
		} else {
			require.True(t, err != io.EOF, test.desc)
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
		{"[{}]", "[{}]", "empty map"},
		{`[{"closingDelimeter":"}"}]`, `[{"closingDelimeter":"}"}]`, "quoted closing brace"},
		{`[{"company":"dgraph"}]`, `[{"company":"dgraph"}]`, "simple, compact map"},
		{
			"[\n  {\n    \"company\" : \"dgraph\"\n  }\n]\n",
			"[{\"company\":\"dgraph\"}]",
			"simple, pretty map",
		},
		{
			`[{"professor":"Alastor \"Mad-Eye\" Moody"}]`,
			`[{"professor":"Alastor \"Mad-Eye\" Moody"}]`,
			"escaped balanced quotes",
		},
		{

			`[{"something{": "}something"}]`,
			`[{"something{":"}something"}]`,
			"escape quoted brackets",
		},
		{
			`[{"height":"6'0\""}]`,
			`[{"height":"6'0\""}]`,
			"escaped unbalanced quote",
		},
		{
			`[{"house":{"Hermione":"Gryffindor","Cedric":"Hufflepuff","Luna":"Ravenclaw","Draco":"Slytherin",}}]`,
			`[{"house":{"Hermione":"Gryffindor","Cedric":"Hufflepuff","Luna":"Ravenclaw","Draco":"Slytherin",}}]`,
			"nested braces",
		},
	}
	for _, test := range tests {
		chunker := NewChunker(JsonFormat, 1000)
		reader := bufioReader(test.json)
		json, err := chunker.Chunk(reader)
		if err == io.EOF {
			// pass
		} else {
			require.NoError(t, err, test.desc)
		}
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

	var err error
	for idx = 0; err == nil; idx++ {
		desc := fmt.Sprintf("reading chunk #%d", idx+1)
		json, err = chunker.Chunk(reader)
		if err != io.EOF {
			require.NoError(t, err, desc)
			require.Equal(t, testChunks[idx], json.String(), desc)
		}
	}
	require.Equal(t, io.EOF, err, "end reading JSON document")
}

func TestFileReader(t *testing.T) {
	//create sample files
	_, thisFile, _, _ := runtime.Caller(0)
	dir := "test-files"
	require.NoError(t, os.MkdirAll(dir, os.ModePerm))
	defer deleteDirs(t, dir)
	testFilesDir := filepath.Join(filepath.Dir(thisFile), "test-files")
	var expectedOutcomes [2]string

	file_data := []struct {
		filename string
		content  string
	}{
		{"test-1", "This is test file 1."},
		{"test-2", "This is test file 2."},
	}
	for i, data := range file_data {
		filePath := filepath.Join(testFilesDir, data.filename)
		f, err := os.Create(filePath)
		require.NoError(t, err)
		defer f.Close()
		_, err = f.WriteString(data.content)
		require.NoError(t, err)
		expectedOutcomes[i] = data.content
	}
	files, err := os.ReadDir(testFilesDir)
	require.NoError(t, err)

	for i, file := range files {
		testfilename := filepath.Join(testFilesDir, file.Name())
		reader, cleanup := FileReader(testfilename, nil)
		bytes, err := io.ReadAll(reader)
		require.NoError(t, err)
		contents := string(bytes)
		//compare file content with correct string
		require.Equal(t, contents, expectedOutcomes[i])
		cleanup()
	}
}

func TestDataFormat(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := "test-files"
	require.NoError(t, os.MkdirAll(dir, os.ModePerm))
	defer deleteDirs(t, dir)
	testFilesDir := filepath.Join(filepath.Dir(thisFile), "test-files")
	expectedOutcomes := [5]InputFormat{2, 1, 0, 2, 1}
	file_data := [5]string{"test-1.json", "test-2.rdf", "test-3.txt", "test-4.json.gz", "test-5.rdf.gz"}

	for i, data := range file_data {
		filePath := filepath.Join(testFilesDir, data)
		format := DataFormat(filePath, "")
		require.Equal(t, format, expectedOutcomes[i])
	}
}

func TestRDFChunkerChunk(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := "test-files"
	require.NoError(t, os.MkdirAll(dir, os.ModePerm))
	defer deleteDirs(t, dir)
	testFilesDir := filepath.Join(filepath.Dir(thisFile), "test-files")
	dataFile := filepath.Join(filepath.Dir(thisFile), "data.rdf")
	resultData := filepath.Join(testFilesDir, "result.rdf")
	f, err := os.Create(resultData)
	require.NoError(t, err)
	chunker := NewChunker(RdfFormat, 1000)
	rd, cleanup := FileReader(dataFile, nil)
	chunkBuf, err := chunker.Chunk(rd)
	if err != io.EOF {
		require.NoError(t, err)
	}

	for chunkBuf.Len() > 0 {
		str, err := chunkBuf.ReadString('\n')
		if err != io.EOF {
			require.NoError(t, err)
		}
		_, err = f.WriteString(str)
		require.NoError(t, err)
	}

	f1, err1 := os.ReadFile(dataFile)
	require.NoError(t, err1)
	f2, err2 := os.ReadFile(resultData)
	require.NoError(t, err2)
	require.Equal(t, true, bytes.Equal(f1, f2))
	cleanup()
}

func deleteDirs(t *testing.T, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		fmt.Printf("Error removing direcotory: %s", err.Error())
	}
}
