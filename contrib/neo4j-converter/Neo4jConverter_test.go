package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"text/scanner"
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
	require.Equal(t, output,buf.String() )
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
		"<_:k_191> <ACTED_IN> <_:k_188> (  roles=\"\\\"Morpheus\\\"\" )")
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

func TestSingleLineFileWithLineBreak(t *testing.T) {
	header := `"_id","_labels","born","name","released","tagline"` +
		`,"title","_start","_end","_type","roles"`
	detail := `"188",":Movie","","","1999","Welcome\n to the \"Real\" World","The Matrix",,,,`
	fileLines := fmt.Sprintf("%s\n%s", header, detail)
	output := `<_:k_188> <_labels> ":Movie" .
<_:k_188> <born> "" .
<_:k_188> <name> "" .
<_:k_188> <released> "1999" .
<_:k_188> <tagline> "Welcome\\n to the \\\"Real\\\" World" .
<_:k_188> <title> "The Matrix" .
`
	i := strings.NewReader(fileLines)
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	out:=buf.String()
	require.Equal(t, output,out )
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

func TestScan(t *testing.T){
	file, _ := os.Open("./exampleLineBreaks.csv")
	r := bufio.NewReader(file)
	var s scanner.Scanner
	s.Init(r)
	count := 0
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		fmt.Printf("%s: %s\n", s.Position, s.TokenText())
		if count>5 {
			break
		}
		count = count + 1
	}
}

func TestReader(t *testing.T){
	file, _ := os.Open("./exampleLineBreaks.csv")
	var r *bufio.Reader
	r = bufio.NewReader(file)
	var lineBuffer bytes.Buffer

	completeLineRead := false
	continueReadingLine := false
	for !completeLineRead {
		continueReadingLine = false
		line,_ := r.ReadString('\n')
		lineBuffer.WriteString(line)
		handler := func(s *scanner.Scanner, msg string) {
			fmt.Println("ERROR")
			fmt.Println(msg)
			if msg == "literal not terminated"{
				continueReadingLine = true
			}
		}

		var s scanner.Scanner
		s.Init(strings.NewReader(line))
		s.Error = handler
		s.Scan()

		if continueReadingLine {
			completeLineRead = false
		} else{
			completeLineRead = true
		}
		//for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		//	fmt.Printf("%s: %s\n", s.Position, s.TokenText())
		//}
	}
	fmt.Println(continueReadingLine)
	fmt.Println(lineBuffer.String())
}

func TestNeo4jFileReader(t *testing.T){
	fileName := "./exampleLineBreaks.csv"
	var nContext Neo4jCSVContext
	nContext.Init(fileName)

	eofReached := false
	nextLine := ""
	count := 0
	for !eofReached {
		nextLine, eofReached = nContext.ProvideNextLine()
		fmt.Println(nextLine)
		count=count  + 1
	}

	require.Equal(t, 2, count)
}

