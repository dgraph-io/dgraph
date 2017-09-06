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

func writePostings(dir string, postingsCh <-chan *protos.FlatPosting, prog *progress) {

	var fileNum int
	var postings []*protos.FlatPosting
	var wg sync.WaitGroup
	var sz int

	processBatch := func() {
		wg.Add(1)
		filename := filepath.Join(dir, fmt.Sprintf("%06d.bin", fileNum))
		fileNum++
		ps := postings
		postings = nil
		sz = 0
		go func() {
			sortAndWrite(filename, ps, prog)
			wg.Done()
		}()
	}

	for posting := range postingsCh {
		postings = append(postings, posting)
		sz += posting.Size()
		if sz > 256<<20 {
			processBatch()
		}
	}
	if len(postings) > 0 {
		processBatch()
	}

	wg.Wait()
}

func sortAndWrite(filename string, postings []*protos.FlatPosting, prog *progress) {
	sort.Slice(postings, func(i, j int) bool {
		return bytes.Compare(postings[i].Key, postings[j].Key) < 0
	})

	var buf proto.Buffer
	for _, posting := range postings {
		x.Check(buf.EncodeMessage(posting))
	}

	fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	x.Checkf(err, "Could not open tmp file.")
	x.Check2(fd.Write(buf.Bytes()))
	x.Check(fd.Sync())
	x.Check(fd.Close())
}
