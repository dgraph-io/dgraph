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
	"log"
	"os"
	"strings"

	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

type options struct {
	// keyfile comes from the encryption_key_file or Vault flags
	keyfile x.SensitiveByteSlice
	file    string
	output  string
}

var Decrypt x.SubCommand

func init() {
	Decrypt.Cmd = &cobra.Command{
		Use:   "decrypt",
		Short: "Run Decrypt tool",
		Long:  "A tool to decrypt an export file created by an encrypted Dgraph cluster",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
	}
	Decrypt.EnvPrefix = "DGRAPH_TOOL_DECRYPT"
	flag := Decrypt.Cmd.Flags()
	flag.StringP("file", "f", "", "Path to file to decrypt.")
	flag.StringP("out", "o", "", "Path to the decrypted file.")
	enc.RegisterFlags(flag)
}
func run() {
	opts := options{
		file:   Decrypt.Conf.GetString("file"),
		output: Decrypt.Conf.GetString("out"),
	}
	sensitiveKey, err := enc.ReadKey(Decrypt.Conf)
	x.Checkf(err, "could not read encryption key file")
	opts.keyfile = sensitiveKey
	if len(opts.keyfile) == 0 {
		log.Fatal("Error while reading encryption key: Key is empty")
	}
	f, err := os.Open(opts.file)
	if err != nil {
		log.Fatalf("Error opening file: %v\n", err)
	}
	defer f.Close()
	reader, err := enc.GetReader(opts.keyfile, f)
	x.Checkf(err, "could not open key reader")
	if strings.HasSuffix(strings.ToLower(opts.file), ".gz") {
		reader, err = gzip.NewReader(reader)
		x.Check(err)
	}
	outf, err := os.OpenFile(opts.output, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatalf("Error while opening output file: %v\n", err)
	}
	w := gzip.NewWriter(outf)
	fmt.Printf("Decrypting %s\n", opts.file)
	fmt.Printf("Writing to %v\n", opts.output)
	_, err = io.Copy(w, reader)
	if err != nil {
		log.Fatalf("Error while writing: %v\n", err)
	}
	err = w.Flush()
	x.Check(err)
	err = w.Close()
	x.Check(err)
	err = outf.Close()
	x.Check(err)
	fmt.Println("Done.")
}
