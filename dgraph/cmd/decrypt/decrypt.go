/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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

package decrypt

import (
	"compress/gzip"
	"io"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"

	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/x"
)

type options struct {
	// keyfile comes from the encryption_key_file or Vault flags
	keyfile x.Sensitive
	file    string
	output  string
}

var Decrypt x.SubCommand

func init() {
	Decrypt.Cmd = &cobra.Command{
		Use:   "decrypt",
		Short: "Run the Dgraph decryption tool",
		Long:  "A tool to decrypt an export file created by an encrypted Dgraph cluster",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
		Annotations: map[string]string{"group": "tool"},
	}
	Decrypt.EnvPrefix = "DGRAPH_TOOL_DECRYPT"
	Decrypt.Cmd.SetHelpTemplate(x.NonRootTemplate)
	flag := Decrypt.Cmd.Flags()
	flag.StringP("file", "f", "", "Path to file to decrypt.")
	flag.StringP("out", "o", "", "Path to the decrypted file.")
	ee.RegisterEncFlag(flag)
}
func run() {
	keys, err := ee.GetKeys(Decrypt.Conf)
	x.Check(err)
	if len(keys.EncKey) == 0 {
		glog.Fatal("Error while reading encryption key: Key is empty")
	}

	opts := options{
		file:    Decrypt.Conf.GetString("file"),
		output:  Decrypt.Conf.GetString("out"),
		keyfile: keys.EncKey,
	}

	f, err := os.Open(opts.file)
	if err != nil {
		glog.Fatalf("Error opening file: %v\n", err)
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
		glog.Fatalf("Error while opening output file: %v\n", err)
	}
	w := gzip.NewWriter(outf)
	glog.Infof("Decrypting %s\n", opts.file)
	glog.Infof("Writing to %v\n", opts.output)
	_, err = io.Copy(w, reader)
	if err != nil {
		glog.Fatalf("Error while writing: %v\n", err)
	}
	err = w.Flush()
	x.Check(err)
	err = w.Close()
	x.Check(err)
	err = outf.Close()
	x.Check(err)
	glog.Infof("Done.")
}
