package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

func writeDenormalisedPostings(dir string, postingsIn <-chan *protos.DenormalisedPosting) {

	var fileNum int
	var buf proto.Buffer
	var postings []*protos.DenormalisedPosting

	dump := func() {
		sort.Slice(postings, func(i, j int) bool {
			return bytes.Compare(postings[i].PostingListKey, postings[j].PostingListKey) < 0
		})
		for _, posting := range postings {
			x.Check(buf.EncodeMessage(posting))
		}

		filename := filepath.Join(dir, fmt.Sprintf("%06d.bin", fileNum))
		fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		x.Checkf(err, "Could not open tmp file.")
		x.Check2(fd.Write(buf.Bytes()))
		x.Check(fd.Close())

		fileNum++
		buf.Reset()
		postings = postings[:0]
	}

	for posting := range postingsIn {
		postings = append(postings, posting)
		if len(postings) > 4<<20 {
			dump()
		}
	}
	if len(buf.Bytes()) > 0 {
		dump()
	}
}
