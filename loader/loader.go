/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"io"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
)

var glog = x.Log("loader")
var uidStore, dataStore *store.Store

type counters struct {
	read      uint64
	parsed    uint64
	processed uint64
	ignored   uint64
}

type state struct {
	input        chan string
	cnq          chan rdf.NQuad
	ctr          *counters
	instanceIdx  uint64
	numInstances uint64
}

func Init(uidstore, datastore *store.Store) {
	uidStore = uidstore
	dataStore = datastore
}

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

func (s *state) parseStream(done chan error) {
	for line := range s.input {
		line = strings.Trim(line, " \t")
		if len(line) == 0 {
			glog.Info("Empty line.")
			continue
		}

		glog.Debugf("Got line: %q", line)
		nq, err := rdf.Parse(line)
		if err != nil {
			glog.WithError(err).Errorf("While parsing: %q", line)
			done <- err
			return
		}
		s.cnq <- nq
		atomic.AddUint64(&s.ctr.parsed, 1)
	}
	done <- nil
}

func (s *state) handleNQuads(wg *sync.WaitGroup) {
	for nq := range s.cnq {
		edge, err := nq.ToEdge(s.instanceIdx, s.numInstances)
		for err != nil {
			// Just put in a retry loop to tackle temporary errors.
			if err == posting.E_TMP_ERROR {
				time.Sleep(time.Microsecond)

			} else {
				glog.WithError(err).WithField("nq", nq).
					Error("While converting to edge")
				return
			}
			edge, err = nq.ToEdge(s.instanceIdx, s.numInstances)
		}

		// Only handle this edge if the attribute satisfies the modulo rule
		if farm.Fingerprint64([]byte(edge.Attribute))%s.numInstances ==
			s.instanceIdx {
			key := posting.Key(edge.Entity, edge.Attribute)
			plist := posting.GetOrCreate(key, dataStore)
			plist.AddMutation(edge, posting.Set)
			atomic.AddUint64(&s.ctr.processed, 1)
		} else {
			atomic.AddUint64(&s.ctr.ignored, 1)
		}

	}
	wg.Done()
}

func (s *state) getUidForString(str string) {
	_, err := rdf.GetUid(str, s.instanceIdx, s.numInstances)
	for err != nil {
		// Just put in a retry loop to tackle temporary errors.
		if err == posting.E_TMP_ERROR {
			time.Sleep(time.Microsecond)
			glog.WithError(err).WithField("nq.Subject", str).
				Error("Temporary error")
		} else {
			glog.WithError(err).WithField("nq.Subject", str).
				Error("While getting UID")
			return
		}
		_, err = rdf.GetUid(str, s.instanceIdx, s.numInstances)
	}
}

func (s *state) handleNQuadsWhileAssign(wg *sync.WaitGroup) {
	for nq := range s.cnq {
		if farm.Fingerprint64([]byte(nq.Subject))%s.numInstances != s.instanceIdx {
			// This instance shouldnt assign UID to this string
			atomic.AddUint64(&s.ctr.ignored, 1)
		} else {
			s.getUidForString(nq.Subject)
		}

		if len(nq.ObjectId) == 0 ||
			farm.Fingerprint64([]byte(nq.ObjectId))%s.numInstances != s.instanceIdx {
			// This instance shouldnt or cant assign UID to this string
			atomic.AddUint64(&s.ctr.ignored, 1)
		} else {
			s.getUidForString(nq.ObjectId)
		}
	}

	wg.Done()
}

// Blocking function.
func HandleRdfReader(reader io.Reader, instanceIdx uint64,
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
	done := make(chan error, numr)
	for i := 0; i < numr; i++ {
		go s.parseStream(done) // Input --> NQuads
	}

	wg := new(sync.WaitGroup)
	for i := 0; i < 3000; i++ {
		wg.Add(1)
		go s.handleNQuads(wg) // NQuads --> Posting list [slow].
	}

	// Block until all parseStream goroutines are finished.
	for i := 0; i < numr; i++ {
		if err := <-done; err != nil {
			glog.WithError(err).Fatal("While reading input.")
		}
	}

	close(s.cnq)
	// Okay, we've stopped input to cnq, and closed it.
	// Now wait for handleNQuads to finish.
	wg.Wait()

	ticker.Stop()
	return atomic.LoadUint64(&s.ctr.processed), nil
}

// Blocking function.
func HandleRdfReaderWhileAssign(reader io.Reader, instanceIdx uint64,
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
	done := make(chan error, numr)
	for i := 0; i < numr; i++ {
		go s.parseStream(done) // Input --> NQuads
	}

	wg := new(sync.WaitGroup)
	for i := 0; i < 3000; i++ {
		wg.Add(1)
		go s.handleNQuadsWhileAssign(wg) //Different compared to HandleRdfReader
	}

	// Block until all parseStream goroutines are finished.
	for i := 0; i < numr; i++ {
		if err := <-done; err != nil {
			glog.WithError(err).Fatal("While reading input.")
		}
	}

	close(s.cnq)
	// Okay, we've stopped input to cnq, and closed it.
	// Now wait for handleNQuads to finish.
	wg.Wait()

	ticker.Stop()
	return atomic.LoadUint64(&s.ctr.processed), nil
}
