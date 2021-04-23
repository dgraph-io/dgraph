package main

import (
	"bytes"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"strings"
	"testing"
)

func TestParsingHeader(t *testing.T) {
	i := strings.NewReader("my request")
	buf := new(bytes.Buffer)
	require.Error(t, processNeo4jCSV(i, buf), "column '_start' is absent in file")
}

func TestSingleLineFileString(t *testing.T) {
	header := `"_id","_labels","born","name","released","tagline"` +
		`,"title","_start","_end","_type","roles"`
	detail := `"188",":Movie","","","1999","Welcome to the Real World","The Matrix",,,,`
	fileLines := fmt.Sprintf("%s\n%s", header, detail)
	output := `<_:k_188> <_labels> ":Movie" .
<_:k_188> <born> "" .
<_:k_188> <name> "" .
<_:k_188> <released> "1999" .
<_:k_188> <tagline> "Welcome to the Real World" .
<_:k_188> <title> "The Matrix" .
`
	i := strings.NewReader(fileLines)
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	require.Equal(t, buf.String(), output)
}

func TestWholeFile(t *testing.T) {
	goldenFile := "./output.rdf"
	inBuf, _ := ioutil.ReadFile("./example.csv")
	i := strings.NewReader(string(inBuf))
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	//check id
	require.Contains(t, buf.String(), "<_:k_188> <_labels> \":Movie\" .")
	//check facets
	require.Contains(t, buf.String(),
		"<_:k_191> <ACTED_IN> <_:k_188> (  roles=\"Morpheus\" )")
	//check link w/o facets
	require.Contains(t, buf.String(), "<_:k_193> <DIRECTED> <_:k_188>")

	//check full file
	expected, err := ioutil.ReadFile(goldenFile)
	if err != nil {
		// Handle error
	}
	isSame := bytes.Equal(expected, buf.Bytes())
	if !isSame {
		fmt.Println("Printing comparison")
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(string(expected), buf.String(), true)
		fmt.Println(dmp.DiffPrettyText(diffs))
	}
	require.True(t, isSame)

}

func BenchmarkSampleFile(b *testing.B) {
	inBuf, _ := ioutil.ReadFile("./example.csv")
	i := strings.NewReader(string(inBuf))
	buf := new(bytes.Buffer)
	for k := 0; k < b.N; k++ {
		processNeo4jCSV(i, buf)
		buf.Reset()
	}
}
