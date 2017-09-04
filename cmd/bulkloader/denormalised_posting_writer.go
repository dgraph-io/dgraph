package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

func writeDenormalisedPostings(dir string, postingsIn <-chan *protos.DenormalisedPosting) {

	var fileNum int
	var buf proto.Buffer

	dump := func() {
		filename := filepath.Join(dir, fmt.Sprintf("%06d.bin", fileNum))
		fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		x.Checkf(err, "Could not open tmp file.")
		x.Check2(fd.Write(buf.Bytes()))
		x.Check(fd.Close())
		buf.Reset()
		fileNum++
	}

	for posting := range postingsIn {
		x.Check(buf.EncodeMessage(posting))
		if len(buf.Bytes()) > 128<<20 {
			dump()
		}
	}
	if len(buf.Bytes()) > 0 {
		dump()
	}
}
