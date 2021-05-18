package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/require"
	"io"
	"io/ioutil"
	"os"
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

func TestWholeFileWithQuotedLineBreaks(t *testing.T) {
	//goldenFile := "./output.rdf"
	inBuf, _ := ioutil.ReadFile("./exampleLineBreaks.csv")
	i := strings.NewReader(string(inBuf))
	//buf := new(bytes.Buffer)

	scanner := bufio.NewScanner(i)
	//scanner.Split(TScanLineWithLineBreaks)
	scanner.Scan()
	txt:=scanner.Text()
	fmt.Println(txt)

}

func TestParsing(t *testing.T){
	file, _ := os.Open("./exampleLineBreaks.csv")
	r := bufio.NewReader(file)
	quoteOpen:=false
	totalSizeInBytes:=0
	var csvLine bytes.Buffer
	var fullFile = bytes.Buffer{}
	for {
		if c, sz, err := r.ReadRune(); err != nil {
			if err == io.EOF {
				fmt.Println("this is a genuine end of line with byte size ", totalSizeInBytes )
				totalSizeInBytes=0
				fmt.Println(csvLine.String())
				fullFile.WriteString(csvLine.String())
				csvLine.Reset()
				break
			} else {
				fmt.Println("Error")
			}
		} else {
			totalSizeInBytes += sz
			//fmt.Println(c)
			csvLine.WriteRune(c)
			if c==10{
				if quoteOpen {
					fmt.Println("this is a new line inside  string quotes")
				}else{
					fmt.Println("this is a genuine end of line with byte size ", totalSizeInBytes - 1 )
					fmt.Println(csvLine.String())
					fullFile.WriteString(csvLine.String())
					//i := strings.NewReader(string(csvLine.String()))
					//buf := new(bytes.Buffer)
					//processNeo4jCSV(i, buf)
					//fmt.Println( buf.String())
					csvLine.Reset()
					totalSizeInBytes=0
				}
			}else if c==34{

				if !quoteOpen {
					quoteOpen = true
				}else{
					quoteOpen = false
				}
			}
		}
	}
	fmt.Println("fully read file")
	fmt.Println(fullFile.String())

}
