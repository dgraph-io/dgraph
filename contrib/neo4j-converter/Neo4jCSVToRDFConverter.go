package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	inputPath  = flag.String("input", "", "Please provide the input csv file.")
	outputPath = flag.String("output", "", "Where to place the output?")
)

func main() {
	flag.Parse()
	//check input path length
	if len(*inputPath) == 0 {
		log.Fatal("Please set the input argument.")
	}
	//check output path length
	if len(*outputPath) == 0 {
		log.Fatal("Please set the output argument.")
	}
	fmt.Printf("CSV to convert: %q ?[y/n]", *inputPath)

	var inputConf, outputConf string
	check2(fmt.Scanf("%s", &inputConf))

	fmt.Printf("Output directory wanted: %q ?[y/n]", *outputPath)
	check2(fmt.Scanf("%s", &outputConf))

	if inputConf != "y" || outputConf != "y" {
		fmt.Println("Please update the directories")
		return
	}

	//open the file
	ifile, err := os.Open(*inputPath)
	check(err)
	defer ifile.Close()
	//log the start time
	ts := time.Now().UnixNano()

	//create output file in append mode
	outputName := filepath.Join(*outputPath, fmt.Sprintf("converted_%d.rdf", ts))
	oFile, err := os.OpenFile(outputName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	check(err)
	defer oFile.Close()
	//process the file
	check(processNeo4jCSV(ifile, oFile))
	fmt.Printf("Finished writing %q", outputName)

}

func processNeo4jCSV(r io.Reader, w io.Writer) error {

	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)
	var text, rdfLines bytes.Buffer

	header := make(map[int]string)
	positionOfStart, startPositionOfProperty := -1, -1

	//read header
	readHeader := func() {
		h := csv.NewReader(strings.NewReader(scanner.Text()))
		line, _ := h.Read()
		//read headers
		for position, fieldName := range line {
			header[position] = fieldName

			if fieldName == "_start" {
				positionOfStart = position
			} else if fieldName == "_type" {
				startPositionOfProperty = position + 1
			}
		}
	}

	// Scan and read the header.
	scanner.Scan()
	readHeader()
	//ensure that header exists
	if positionOfStart == -1 {
		return errors.New("column '_start' is absent in file")
	}

	// Read the actual data.
	for scanner.Scan() {
		//parse csv
		text.WriteString(scanner.Text() + "\n")
		d := csv.NewReader(strings.NewReader(text.String()))
		records, err := d.ReadAll()
		check(err)

		linkStartNode := ""
		linkEndNode := ""
		linkName := ""
		facets := make(map[string]string)

		line := records[0]
		for position := 0; position < len(line); position++ {

			// This is an _id node.
			if len(line[0]) > 0 {
				bn := fmt.Sprintf("<_:k_%s>", line[0])
				if position < positionOfStart && position > 0 {
					//write non-facet data
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
					//collect facets
					facets[header[position]] = line[position]
				}
				continue
			}
		}
		//write the facets
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
					//insert a comma to separate multiple facets
					facetLine = fmt.Sprintf("%s, ", facetLine)
				}
				//write the actual facet
				facetLine = fmt.Sprintf("%s %s=%s", facetLine, facetName, facetValue)
				atleastOneFacetExists = true
			}
			if atleastOneFacetExists {
				//wrap all facets with round brackets
				facetLine = fmt.Sprintf("( %s )", facetLine)
			}
			rdfLines.WriteString(fmt.Sprintf("%s %s %s %s .\n",
				linkStartNode, linkName, linkEndNode, facetLine))
		}

		text.Reset()
		//write a chunk when ready
		if rdfLines.Len() > 100<<20 {
			// Flush the writes and reset the rdfLines
			check2(w.Write(rdfLines.Bytes()))
			rdfLines.Reset()
		}
	}
	check2(w.Write(rdfLines.Bytes()))
	return nil
}
func check2(_ interface{}, err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
