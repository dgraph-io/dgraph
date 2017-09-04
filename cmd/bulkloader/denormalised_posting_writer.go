package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

func writeDenormalisedPostings(dir string, postingsIn <-chan *protos.DenormalisedPosting) {

	var fileNum int
	var postings []*protos.DenormalisedPosting
	var wg sync.WaitGroup

	processBatch := func() {
		wg.Add(1)
		filename := filepath.Join(dir, fmt.Sprintf("%06d.bin", fileNum))
		fileNum++
		ps := postings
		go func() {
			sortAndDump(filename, ps)
			wg.Done()
		}()
		postings = nil
	}

	for posting := range postingsIn {
		postings = append(postings, posting)
		if len(postings) > 4<<20 {
			processBatch()
		}
	}
	if len(postings) > 0 {
		processBatch()
	}

	wg.Wait()
}

func sortAndDump(filename string, postings []*protos.DenormalisedPosting) {

	sort.Slice(postings, func(i, j int) bool {
		return bytes.Compare(postings[i].PostingListKey, postings[j].PostingListKey) < 0
	})

	var buf proto.Buffer
	for _, posting := range postings {
		x.Check(buf.EncodeMessage(posting))
	}

	fmt.Printf("Writing %q: Postings: %d BufSize %d\n", filename, len(postings), len(buf.Bytes()))

	fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	x.Checkf(err, "Could not open tmp file.")
	x.Check2(fd.Write(buf.Bytes()))
	x.Check(fd.Close())
}
