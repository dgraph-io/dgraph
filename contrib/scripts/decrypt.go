package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/x"
)

type options struct {
	keyfile string
	file    string
	output  string
}

var opts options

func main() {
	flag.StringVar(&opts.keyfile, "keyfile", "", "Location of the key file to decrypt the schema "+
		"and data files")
	flag.StringVar(&opts.file, "file", "", "Path to file to decrypt")
	flag.StringVar(&opts.output, "output", "", "Path to output.")

	flag.Parse()

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
	b, err := ioutil.ReadAll(reader)
	x.Check(err)

	outf, err := os.OpenFile(opts.output, os.O_WRONLY|os.O_CREATE, 0644)
	x.CheckfNoTrace(err)

	w := gzip.NewWriter(outf)

	w.Write(b)
	err = w.Flush()
	x.Check(err)
	err = w.Close()
	x.Check(err)
	err = outf.Close()
	x.Check(err)
	fmt.Printf("Done. Outputted to %v\n", opts.output)
}
