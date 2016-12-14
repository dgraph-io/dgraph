/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package loader

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"io"
	"log"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

var (
	glog        = x.Log("loader")
	dataStore   *store.Store
	maxRoutines = flag.Int("maxroutines", 3000,
		"Maximum number of goroutines to execute concurrently")
)

type counters struct {
	read      uint64
	parsed    uint64
	processed uint64
	ignored   uint64
}

type state struct {
	sync.RWMutex
	input     chan string
	cnq       chan rdf.NQuad
	ctr       *counters
	groupsMap map[uint32]bool
	err       error
}

func Init(datastore *store.Store) {
	dataStore = datastore
}

func (s *state) Error() error {
	s.RLock()
	defer s.RUnlock()
	return s.err
}

func (s *state) SetError(err error) {
	s.Lock()
	defer s.Unlock()
	if s.err == nil {
		s.err = err
	}
}

// printCounters prints the counter variables at intervals
// specified by ticker.
func (s *state) printCounters(ticker *time.Ticker) {
	var prev uint64
	for _ = range ticker.C {
		processed := atomic.LoadUint64(&s.ctr.processed)
		if prev == processed {
			continue
		}
		prev = processed
		parsed := atomic.LoadUint64(&s.ctr.parsed)
		ignored := atomic.LoadUint64(&s.ctr.ignored)
		pending := parsed - ignored - processed
		glog.WithFields(logrus.Fields{
			"read":      atomic.LoadUint64(&s.ctr.read),
			"processed": processed,
			"parsed":    parsed,
			"ignored":   ignored,
			"pending":   pending,
			"len_cnq":   len(s.cnq),
		}).Info("Counters")
	}
}

// readLines reads the file and pushes the nquads onto a channel.
// Run this in a single goroutine. This function closes s.input channel.
func (s *state) readLines(r io.Reader) {
	var buf []string
	var err error
	var strBuf bytes.Buffer
	bufReader := bufio.NewReader(r)
	// Randomize lines to avoid contention on same subject.
	for i := 0; i < 1000; i++ {
		err = x.ReadLine(bufReader, &strBuf)
		if err != nil {
			break
		}
		buf = append(buf, strBuf.String())
		atomic.AddUint64(&s.ctr.read, 1)
	}

	if err != nil && err != io.EOF {
		err := x.Errorf("Error while reading file: %v", err)
		log.Fatalf("%+v", err)
	}

	// If we haven't yet finished reading the file read the rest of the rows.
	for {
		err = x.ReadLine(bufReader, &strBuf)
		if err != nil {
			break
		}
		k := rand.Intn(len(buf))
		s.input <- buf[k]
		buf[k] = strBuf.String()
		atomic.AddUint64(&s.ctr.read, 1)
	}

	if err != io.EOF {
		err := x.Errorf("Error while reading file: %v", err)
		log.Fatalf("%+v", err)
	}
	for i := 0; i < len(buf); i++ {
		s.input <- buf[i]
	}
	close(s.input)
}

// parseStream consumes the lines, converts them to nquad
// and sends them into cnq channel.
func (s *state) parseStream(wg *sync.WaitGroup) {
	defer wg.Done()

	for line := range s.input {
		if s.Error() != nil {
			return
		}
		line = strings.Trim(line, " \t")
		if len(line) == 0 {
			glog.Info("Empty line.")
			continue
		}

		glog.Debugf("Got line: %q", line)
		nq, err := rdf.Parse(line)
		if err != nil {
			s.SetError(err)
			return
		}
		s.cnq <- nq
		atomic.AddUint64(&s.ctr.parsed, 1)
	}
}
func markTaken(ctx context.Context, uid uint64) {
	mu := &task.DirectedEdge{
		Entity: uid,
		Attr:   "_uid_",
		Value:  []byte("_"), // not txid
		Label:  "_loader_",
		Op:     task.DirectedEdge_SET,
	}
	key := x.DataKey("_uid_", uid)
	plist, decr := posting.GetOrCreate(key)
	plist.AddMutation(ctx, mu)
	decr()
}

// handleNQuads converts the nQuads that satisfy the modulo
// rule into posting lists.
func (s *state) handleNQuads(wg *sync.WaitGroup) {
	defer wg.Done()
	// Check if we need to mark used UIDs.
	markUids := s.groupsMap[group.BelongsTo("_uid_")]
	ctx := context.Background()
	for nq := range s.cnq {
		if s.Error() != nil {
			return
		}
		// Only handle this edge if the attribute satisfies the modulo rule
		if !s.groupsMap[group.BelongsTo(nq.Predicate)] {
			atomic.AddUint64(&s.ctr.ignored, 1)
			continue
		}

		edge, err := nq.ToEdge()
		for err != nil {
			// Just put in a retry loop to tackle temporary errors.
			if err == posting.ErrRetry {
				time.Sleep(time.Microsecond)

			} else {
				s.SetError(err)
				glog.WithError(err).WithField("nq", nq).
					Error("While converting to edge")
				return
			}
			edge, err = nq.ToEdge()
		}
		edge.Op = task.DirectedEdge_SET

		key := x.DataKey(edge.Attr, edge.Entity)

		plist, decr := posting.GetOrCreate(key)
		plist.AddMutationWithIndex(ctx, edge)
		decr() // Don't defer, just call because we're in a channel loop.

		// Mark UIDs and XIDs as taken
		if markUids {
			// Mark entity UID.
			markTaken(ctx, edge.Entity)
			// Mark the Value UID.
			if edge.ValueId != 0 {
				markTaken(ctx, edge.ValueId)
			}
		}
		atomic.AddUint64(&s.ctr.processed, 1)
	}
}

// LoadEdges is called with the reader object of a file whose
// contents are to be converted to posting lists.
func LoadEdges(reader io.Reader, groupsMap map[uint32]bool) (uint64, error) {

	s := new(state)
	s.ctr = new(counters)
	ticker := time.NewTicker(time.Second)
	go s.printCounters(ticker)

	// Producer: Start buffering input to channel.
	s.groupsMap = groupsMap
	s.input = make(chan string, 10000)
	go s.readLines(reader)

	s.cnq = make(chan rdf.NQuad, 10000)
	numr := runtime.GOMAXPROCS(-1)
	var pwg sync.WaitGroup
	pwg.Add(numr)
	for i := 0; i < numr; i++ {
		go s.parseStream(&pwg) // Input --> NQuads
	}

	nrt := *maxRoutines
	var wg sync.WaitGroup
	wg.Add(nrt)
	for i := 0; i < nrt; i++ {
		go s.handleNQuads(&wg) // NQuads --> Posting list [slow].
	}

	// Block until all parseStream goroutines are finished.
	pwg.Wait()
	close(s.cnq)
	// Okay, we've stopped input to cnq, and closed it.
	// Now wait for handleNQuads to finish.
	wg.Wait()

	ticker.Stop()
	return atomic.LoadUint64(&s.ctr.processed), s.Error()
}
