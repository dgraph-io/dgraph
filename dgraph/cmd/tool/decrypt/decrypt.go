/*
 * Copyright 2017-2021 Dgraph Labs, Inc. and Contributors
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

package tool

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/x"
  "github.com/spf13/cobra"
)

type options struct {
	keyfile string
	file    string
	output  string
}

var opts options
var Decrypt x.SubCommand

func init() {
	Decrypt.Cmd = &cobra.Command{
		Use:   "decrypt",
		Short: "Run Decrypt tool",
		Long: `
A Decrypt tool that can be used to decrypt an export that has been taken from
an encrypted Dgraph Cluster
`,
Run: func(cmd *cobra.Command, args []string) {
			run()
		},
	}
	Decrypt.EnvPrefix = "DGRAPH_DECRYPT"
	flag := Decrypt.Cmd.Flags()
	flag.StringVarP(&opts.keyfile, "encryption_key_file", "e", "", "Location of the key file to decrypt the schema and data files.")
	flag.StringVarP(&opts.file, "file", "f", "", "Path to file to decrypt.")
	flag.StringVarP(&opts.output, "output", "o", "", "Path the the decrypted file.")
}
func run() {
		f, err := os.Open(opts.file)
	x.CheckfNoTrace(err)
	defer f.Close()
	var sensitiveKey x.SensitiveByteSlice
	sensitiveKey, err = ioutil.ReadFile(opts.keyfile)
	x.Check(err)
	reader, err := enc.GetReader(sensitiveKey, f)
	x.Check(err)
	if strings.HasSuffix(strings.ToLower(opts.file), ".gz") {
		reader, err = gzip.NewReader(reader)
		x.Check(err)
	}
	outf, err := os.OpenFile(opts.output, os.O_WRONLY|os.O_CREATE, 0644)
	x.CheckfNoTrace(err)
	w := gzip.NewWriter(outf)
	_, err = io.Copy(w, reader)
	x.Check(err)
	err = w.Flush()
	x.Check(err)
	err = w.Close()
	x.Check(err)
	err = outf.Close()
	x.Check(err)
	fmt.Printf("Done. Outputted to %v\n", opts.output)
}
