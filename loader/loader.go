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
	"context"
	"flag"
	"io"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("loader")
var uidStore, dataStore *store.Store

var maxRoutines = flag.Int("maxroutines", 3000,
	"Maximum number of goroutines to execute concurrently")

type counters struct {
	read      uint64
	parsed    uint64
	processed uint64
	ignored   uint64
}

type state struct {
	sync.RWMutex
	input        chan string
	cnq          chan rdf.NQuad
	ctr          *counters
	instanceIdx  uint64
	numInstances uint64
	err          error
}

func Init(uidstore, datastore *store.Store) {
	uidStore = uidstore
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

// readLines reads the file and pushes them onto a channel.
// Run this in a single goroutine. This function closes s.input channel.
func (s *state) readLines(r io.Reader) {
	var buf []string
	scanner := bufio.NewScanner(r)
	// Randomize lines to avoid contention on same subject.
	for i := 0; i < 1000; i++ {
		if scanner.Scan() {
			buf = append(buf, scanner.Text())
		} else {
			break
		}
	}
	ln := len(buf)
	for scanner.Scan() {
		k := rand.Intn(ln)
		s.input <- buf[k]
		buf[k] = scanner.Text()
		atomic.AddUint64(&s.ctr.read, 1)
	}
	if err := scanner.Err(); err != nil {
		glog.WithError(err).Fatal("While reading file.")
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

// handleNQuads converts the nQuads that satisfy the modulo
// rule into posting lists.
func (s *state) handleNQuads(wg *sync.WaitGroup) {
	defer wg.Done()

	ctx := context.Background()
	for nq := range s.cnq {
		if s.Error() != nil {
			return
		}
		// Only handle this edge if the attribute satisfies the modulo rule
		if farm.Fingerprint64([]byte(nq.Predicate))%s.numInstances != s.instanceIdx {
			atomic.AddUint64(&s.ctr.ignored, 1)
			continue
		}

		edge, err := nq.ToEdge()
		for err != nil {
			// Just put in a retry loop to tackle temporary errors.
			if err == posting.E_TMP_ERROR {
				time.Sleep(time.Microsecond)

			} else {
				s.SetError(err)
				glog.WithError(err).WithField("nq", nq).
					Error("While converting to edge")
				return
			}
			edge, err = nq.ToEdge()
		}

		key := posting.Key(edge.Entity, edge.Attribute)
		plist := posting.GetOrCreate(key, dataStore)
		plist.AddMutationWithIndex(ctx, edge, posting.Set)
		atomic.AddUint64(&s.ctr.processed, 1)
	}
}

// assignUid assigns a unique integer for a given xid string.
func (s *state) assignUid(xid string) error {
	if strings.HasPrefix(xid, "_uid_:") {
		_, err := strconv.ParseUint(xid[6:], 0, 64)
		return err
	}

	_, err := uid.GetOrAssign(xid, s.instanceIdx, s.numInstances)
	for err != nil {
		// Just put in a retry loop to tackle temporary errors.
		if err == posting.E_TMP_ERROR {
			time.Sleep(time.Microsecond)
			glog.WithError(err).WithField("xid", xid).
				Debug("Temporary error")
		} else {
			glog.WithError(err).WithField("xid", xid).
				Error("While getting UID")
			return err
		}
		_, err = uid.GetOrAssign(xid, s.instanceIdx, s.numInstances)
	}
	return nil
}

// assignUidsOnly assigns uid to those entities that satisfy the
// modulo rule.
func (s *state) assignUidsOnly(wg *sync.WaitGroup) {
	defer wg.Done()

	for nq := range s.cnq {
		if s.Error() != nil {
			return
		}
		ignored := true
		if farm.Fingerprint64([]byte(nq.Subject))%s.numInstances == s.instanceIdx {
			if err := s.assignUid(nq.Subject); err != nil {
				s.SetError(err)
				glog.WithError(err).Error("While assigning Uid to subject.")
				return
			}
			ignored = false
		}

		if len(nq.ObjectId) > 0 &&
			farm.Fingerprint64([]byte(nq.ObjectId))%s.numInstances == s.instanceIdx {
			if err := s.assignUid(nq.ObjectId); err != nil {
				s.SetError(err)
				glog.WithError(err).Error("While assigning Uid to object.")
				return
			}
			ignored = false
		}

		if ignored {
			atomic.AddUint64(&s.ctr.ignored, 1)
		} else {
			atomic.AddUint64(&s.ctr.processed, 1)
		}
	}
}

// LoadEdges is called with the reader object of a file whose
// contents are to be converted to posting lists.
func LoadEdges(reader io.Reader, instanceIdx uint64,
	numInstances uint64) (uint64, error) {

	s := new(state)
	s.ctr = new(counters)
	ticker := time.NewTicker(time.Second)
	go s.printCounters(ticker)

	// Producer: Start buffering input to channel.
	s.instanceIdx = instanceIdx
	s.numInstances = numInstances
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

// AssignUids would pick up all the external ids in RDFs read,
// and assign unique integer ids for them. This function would
// not load the edges, only assign UIDs.
func AssignUids(reader io.Reader, instanceIdx uint64,
	numInstances uint64) (uint64, error) {

	s := new(state)
	s.ctr = new(counters)
	ticker := time.NewTicker(time.Second)
	go s.printCounters(ticker)

	// Producer: Start buffering input to channel.
	s.instanceIdx = instanceIdx
	s.numInstances = numInstances
	s.input = make(chan string, 10000)
	go s.readLines(reader)

	s.cnq = make(chan rdf.NQuad, 10000)
	numr := runtime.GOMAXPROCS(-1)
	var pwg sync.WaitGroup
	pwg.Add(numr)
	for i := 0; i < numr; i++ {
		go s.parseStream(&pwg) // Input --> NQuads
	}

	wg := new(sync.WaitGroup)
	nrt := *maxRoutines
	for i := 0; i < nrt; i++ {
		wg.Add(1)
		go s.assignUidsOnly(wg)
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
