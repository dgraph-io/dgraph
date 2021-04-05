/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

var (
	inputPath  = flag.String("input", "", "Please provide the input csv file.")
	outputPath = flag.String("output", "", "Where to place the output?")
)

func main() {
	flag.Parse()

	if len(*inputPath) == 0 {
		log.Fatal("Please set the input argument.")
	}
	if len(*outputPath) == 0 {
		log.Fatal("Please set the output argument.")
	}
	fmt.Printf("CSV to convert: %q ?[y/n]", *inputPath)

	var inputConf, outputConf string
	x.Check2(fmt.Scanf("%s", &inputConf))

	fmt.Printf("Output directory wanted: %q ?[y/n]", *outputPath)
	x.Check2(fmt.Scanf("%s", &outputConf))

	if inputConf != "y" || outputConf != "y" {
		fmt.Println("Please update the directories")
		return
	}

	ifile, err := os.Open(*inputPath)
	x.Check(err)
	defer ifile.Close()
	ts := time.Now().UnixNano()

	outputName := filepath.Join(*outputPath, fmt.Sprintf("converted_%d.rdf", ts))
	oFile, err := os.OpenFile(outputName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	x.Check(err)
	defer func() {
		oFile.Close()
	}()
	processCSV(ifile, oFile)
	fmt.Printf("Finished writing %q", outputName)
}

// func processCSV(inputFile string, outputPath string) {
func processCSV(r io.Reader, w io.Writer) {

	// rr := csv.NewReader(r)

	// for {
	// 	record, err := rr.Read()
	// 	if err == io.EOF {
	// 		break
	// 	}
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}

	// 	fmt.Println(record)
	// }

	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)
	var text, rdfLines bytes.Buffer

	header := make(map[int]string)
	var lineNumber, count int
	positionOfStart, startPositionOfProperty := -1, -1

	readHeader := func() {
		if lineNumber == 0 {
			r := csv.NewReader(strings.NewReader(text.String()))
			line, _ := r.Read()
			//read headers
			for position, fieldName := range line {
				header[position] = fieldName

				if fieldName == "_start" {
					positionOfStart = position
				} else if fieldName == "_type" {
					startPositionOfProperty = position + 1
				}

			}

			lineNumber++
			text.Reset()
		}
	}

	// Scan and read the header.
	scanner.Scan()
	readHeader()

	// Read the actual data.
	for scanner.Scan() {
		text.WriteString(scanner.Text() + "\n")

		r := csv.NewReader(strings.NewReader(text.String()))
		records, err := r.ReadAll()
		x.Check(err)
		for _, line := range records {
			linkStartNode := ""
			linkEndNode := ""
			linkName := ""
			facets := make(map[string]string)
			for position := 0; position < len(line); position++ {

				// This is an _id node.
				if len(line[0]) > 0 {
					bn := fmt.Sprintf("<_:k_%s>", line[0])
					if position < positionOfStart && position > 0 {
						rdfLines.WriteString(fmt.Sprintf("%s <%s> \"%s\" .\n",
							bn, header[position], line[position]))
					}
					continue
				}
				// Handle relationship data.
				if position >= positionOfStart {
					if header[position] == "_start" {
						linkStartNode = fmt.Sprintf("<_:k_%s>", line[position])
					} else if header[position] == "_end" {
						linkEndNode = fmt.Sprintf("<_:k_%s>", line[position])
					} else if header[position] == "_type" {
						linkName = fmt.Sprintf("<%s>", line[position])
					} else if position >= startPositionOfProperty {
						facets[header[position]] = line[position]
					}
				}
			}
			//write the link
			if len(linkName) > 0 {
				facetLine := ""
				atleastOneFacetExists := false
				for facetName, facetValue := range facets {
					if len(facetValue) == 0 {
						continue
					}
					//strip [ ], and assume only one value
					facetValue = strings.Replace(facetValue, "[", "", 1)
					facetValue = strings.Replace(facetValue, "]", "", 1)
					if atleastOneFacetExists {
						facetLine = fmt.Sprintf("%s, ", facetLine)
					}
					facetLine = fmt.Sprintf("%s %s=%s", facetLine, facetName, facetValue)
					atleastOneFacetExists = true
				}
				if atleastOneFacetExists {
					facetLine = fmt.Sprintf("( %s )", facetLine)
				}
				rdfLines.WriteString(fmt.Sprintf("%s %s %s %s.\n", linkStartNode, linkName, linkEndNode, facetLine))
			}

			count++
			text.Reset()
		}
		lineNumber++
		if rdfLines.Len() > 100<<20 {
			// Flush the writes and reset the rdfLines
			x.Check2(w.Write(rdfLines.Bytes()))
			rdfLines.Reset()
		}
	}
	x.Check2(w.Write(rdfLines.Bytes()))
}
