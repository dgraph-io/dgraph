package main

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"strings"
	"testing"
)

func TestParsingHeader(t *testing.T) {
	i := strings.NewReader("my request")
	buf := new(bytes.Buffer)
	err := processNeo4jCSV(i, buf)
	require.Contains(t, err.Error(), "column '_start' is absent")
	//log output
	fmt.Println(buf.String())
}


func TestSingleLineFileString(t *testing.T) {
	header:=`"_id","_labels","born","name","released","tagline","title","_start","_end","_type","roles"`
	detail := `"188",":Movie","","","1999","Welcome to the Real World","The Matrix",,,,`
	fileLines := fmt.Sprintf("%s\n%s",header, detail)

	i := strings.NewReader(fileLines)
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	require.Contains(t, buf.String(), "<_:k_188> <_labels> \":Movie\" .")
}

func TestWholeFile(t *testing.T){
	inBuf,_:= ioutil.ReadFile("./example.csv")
	i := strings.NewReader(string(inBuf))
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	//check id
	require.Contains(t, buf.String(), "<_:k_188> <_labels> \":Movie\" .")
	//check facets
	require.Contains(t, buf.String(),"<_:k_191> <ACTED_IN> <_:k_188> (  roles=\"Morpheus\" )")
	//check link w/o facets
	require.Contains(t, buf.String(),"<_:k_193> <DIRECTED> <_:k_188>")
}

func BenchmarkSampleFile(b *testing.B) {
	inBuf,_:= ioutil.ReadFile("./example.csv")
	i := strings.NewReader(string(inBuf))
	for k := 0; k < b.N; k++ {
		buf := new(bytes.Buffer)
		processNeo4jCSV(i, buf)
	}
}
