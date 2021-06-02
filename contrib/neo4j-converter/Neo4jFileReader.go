package main

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"text/scanner"
)

type Neo4jCSVContext struct{
	r *bufio.Reader
	count int
}

func (context *Neo4jCSVContext) Init(fileName string) {
	file, _ := os.Open("./exampleLineBreaks.csv")
	context.r = bufio.NewReader(file)
}


// ProvideNextLine This function processes the next line in the file
func (context *Neo4jCSVContext) ProvideNextLine() (string, bool){
	var lineBuffer bytes.Buffer

	completeLineRead := false
	continueReadingLine := false
	eofFileReached := false

	handler := func(s *scanner.Scanner, msg string) {
		//fmt.Println("ERROR")
		//fmt.Println(msg)
		if msg == "literal not terminated"{
			continueReadingLine = true
		}
	}
	for !completeLineRead {
		continueReadingLine = false
		line,_ := context.r.ReadString('\n')
		lineBuffer.WriteString(line)

		var s scanner.Scanner
		s.Init(strings.NewReader(line))
		s.Error = handler
		var tok rune
		for tok = s.Scan(); tok != scanner.EOF; tok = s.Scan() {
			//do nothing
		}
		if tok == scanner.EOF && len(line) == 0{
			eofFileReached = true
		}

		if continueReadingLine {
			completeLineRead = false
		} else{
			completeLineRead = true
		}
	}
	return lineBuffer.String(), eofFileReached
}
