package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

var (
	input  = flag.String("input", "/demos/neo4jcsv/example.csv", "Please provide the input csv file.")
	output = flag.String("output", "/demos/neo4jcsv/", "Where to place the output?")
)

var inputPath, outputPath string

func main() {
	flag.Parse()
	inputPath = *input
	outputPath = *output

	fmt.Println(inputPath)
	fmt.Println(outputPath)

	// Confirm the input/output paths
	var inpDir, outputDir string

	fmt.Println("CSV to convert: " + inputPath + " ?[y/n] ")
	_, err := fmt.Scanf("%s", &inpDir)
	if err != nil {
		log.Fatal("Error reading input path")
	}

	fmt.Println("Output directory wanted: " + outputPath + " ?[y/n] ")
	_, err = fmt.Scanf("%s", &outputDir)
	if err != nil {
		log.Fatal("Error reading input path")
	}
	if inpDir != "y" || outputDir != "y" {
		fmt.Println("Please update the directories")
		return
	}

	ts := time.Now().UnixNano()
	processCSV(inputPath, outputPath, ts)

}

func processCSV(inputFile string, outputPath string, ts int64) {
	//read the file
	file, err := os.Open(inputFile)

	if err != nil {
		log.Fatalf("failed to open")
	}

	scanner := bufio.NewScanner(file)

	scanner.Split(bufio.ScanLines)
	var text bytes.Buffer
	var rdfLines bytes.Buffer

	header := make(map[int]string)
	var positionOfStart = -1
	var startPositionOfProperty = -1
	count := 0

	lineNumber := 0

	for scanner.Scan() {
		text.WriteString(scanner.Text() + "\n")

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
		} else {
			r := csv.NewReader(strings.NewReader(text.String()))
			records, err := r.ReadAll()
			if err != nil {
				log.Fatal(err)
			}
			for _, line := range records {
				linkStartNode := ""
				linkEndNode := ""
				linkName := ""
				facets := make(map[string]string)
				for position := 0; position < len(line); position++ {

					if len(line[0]) > 0 {
						bn := fmt.Sprintf("<_:k_%s>", line[0])
						if position < positionOfStart {
							if position > 0 {
								rdfLines.WriteString(fmt.Sprintf("%s <%s> \"%s\" .\n", bn, header[position], line[position]))
							}
						}
					} else {
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
				}
				//write the link
				if len(linkName) > 0 {
					facetLine := ""
					atleastOneFacetExists := false
					for facetName, facetValue := range facets {
						if len(facetValue) == 0 {
							continue
						} else {
							//strip [ ], and assume only one value
							facetValue = strings.Replace(facetValue, "[", "", 1)
							facetValue = strings.Replace(facetValue, "]", "", 1)
						}
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

		}

	}
	rdfFileName := fmt.Sprintf("%s_%d_%d.rdf", "converted", count, ts)
	writeToFile(outputPath, rdfFileName, rdfLines.String())
	rdfLines.Reset()
	err = file.Close()

}

func writeToFile(outputPath, filename, lines string) {

	outputFile := fmt.Sprintf("%s%s", outputPath, filename)
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	_, err = f.WriteString(lines)
	if err != nil {
		panic(err)
	}
	err = f.Sync()
	if err != nil {
		panic(err)
	}
	err = f.Close()
	if err != nil {
		panic(err)
	}

	fmt.Println("Finished writing " + filename + " in directory " + outputPath)
}
