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

package validator

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// Validator is the sub-command for validating input files which can later be
// used with bulk/live loader to insert data into dgraph.
var Validate x.SubCommand

type options struct {
	DataFiles     string
	TmpDir        string
	NumGoroutines int
	CleanupTmp    bool
	DataFormat    string
}

func init() {
	Validate.Cmd = &cobra.Command{
		Use:   "validate",
		Short: "Validate input files",
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Validate.Conf).Stop()
			run()
		},
	}
	Validate.EnvPrefix = "VALIDATOR"

	flag := Validate.Cmd.Flags()
	flag.StringP("files", "f", "",
		"Location of *.rdf(.gz) or *.json(.gz) files(s) to validate.")
	flag.String("format", "",
		"Specify file format (rdf or json) instead of getting it from filename.")
}

func run() {
	opt := options{
		DataFiles:     Validate.Conf.GetString("files"),
		NumGoroutines: Validate.Conf.GetInt("num_go_routines"),
		DataFormat:    Validate.Conf.GetString("format"),
	}

	x.PrintVersion()
	if opt.DataFiles == "" {
		fmt.Fprint(os.Stderr, "RDF or JSON file(s) location must be specified.\n")
		os.Exit(1)
	}

	fileList := strings.Split(opt.DataFiles, ",")
	for _, file := range fileList {
		if _, err := os.Stat(file); err != nil && os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Data path(%v) does not exist.\n", file)
			os.Exit(1)
		}
	}

	allDataFiles := getFiles(opt.DataFiles)
	loaderType := getLoaderType(allDataFiles[0], opt.DataFormat)

	var validatorWg sync.WaitGroup
	validatorWg.Add(len(allDataFiles))
	for _, file := range allDataFiles {
		go validateDataFile(file, loaderType, &validatorWg)
	}

	validatorWg.Wait()
	glog.Infoln("Input file validation complete.")
}

func validateDataFile(file string, loadType chunker.InputFormat, validatorWg *sync.WaitGroup) {
	defer validatorWg.Done()
	glog.Infof("Processing file %s\n", file)

	fReader, cleanup := chunker.FileReader(file)
	defer cleanup()

	fChunker := chunker.NewChunker(loadType, 1000)
	x.Check(fChunker.Begin(fReader))
	for {
		chunkBuf, err := fChunker.Chunk(fReader)
		if chunkBuf != nil && chunkBuf.Len() > 0 {
			parseChunkbuf(fChunker, chunkBuf, file)
		}
		if err == io.EOF {
			break
		} else if err != nil {
			x.Check(err) // Most likely the specified file format is incorrect.
		}
	}
	x.Check(fChunker.End(fReader))
}

func parseChunkbuf(fChunker chunker.Chunker, chunkBuf *bytes.Buffer, file string) {
	if chunkBuf == nil || chunkBuf.Len() == 0 {
		return
	}

	lexer := &lex.Lexer{}

	for chunkBuf.Len() > 0 {
		str, err := chunkBuf.ReadString('\n')
		if err != nil && err != io.EOF {
			x.Check(err)
		}

		_, err = chunker.ParseRDF(str, lexer)
		if err == chunker.ErrEmpty {
			continue // blank line or comment
		} else if err != nil {
			glog.Errorf("Error found in file %s: %s while parsing line %q\n",
				file, err, str)
		}
	}
}

func getFiles(dataFiles string) []string {
	files := x.FindDataFiles(dataFiles, []string{".rdf", ".rdf.gz", ".json", ".json.gz"})
	if len(files) == 0 {
		x.Fatalf("No data files found in %s\n", dataFiles)
	}

	return files
}

func getLoaderType(file, dataFormat string) chunker.InputFormat {
	loaderType := chunker.DataFormat(file, dataFormat)
	if loaderType == chunker.UnknownFormat {
		// Dont't try to detect JSON input in bulk loader.
		x.Fatalf("Need --format=rdf or --format=json to load %s\n", file)
	}

	return loaderType
}
