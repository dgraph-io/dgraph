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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
)

var glog = x.Log("loader")

type counters struct {
	read      uint64
	parsed    uint64
	processed uint64
	ignored   uint64
}

type state struct {
	input chan string
	cnq   chan rdf.NQuad
	ctr   *counters
	mod   uint64
}

func printCounters(ticker *time.Ticker, c *counters) {
	for _ = range ticker.C {
		glog.WithFields(logrus.Fields{
			"read":      atomic.LoadUint64(&c.read),
			"parsed":    atomic.LoadUint64(&c.parsed),
			"processed": atomic.LoadUint64(&c.processed),
			"ignored":   atomic.LoadUint64(&c.ignored),
		}).Info("Counters")
	}
}

func (s *state) readLines(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		s.input <- scanner.Text()
		atomic.AddUint64(&s.ctr.read, 1)
	}
	if err := scanner.Err(); err != nil {
		glog.WithError(err).Fatal("While reading file.")
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
		if farm.Fingerprint64([]byte(nq.Subject))%s.mod != 0 {
			// Ignore due to mod sampling.
			atomic.AddUint64(&s.ctr.ignored, 1)
			continue
		}

		edge, err := nq.ToEdge()
		if err != nil {
			glog.WithError(err).WithField("nq", nq).Error("While converting to edge")
			return
		}

		key := posting.Key(edge.Entity, edge.Attribute)
		plist := posting.Get(key)
		plist.AddMutation(edge, posting.Set)
		atomic.AddUint64(&s.ctr.processed, 1)
	}
	wg.Done()
}

// Blocking function.
func HandleRdfReader(reader io.Reader, mod uint64) (uint64, error) {
	s := new(state)
	s.ctr = new(counters)
	ticker := time.NewTicker(time.Second)
	go printCounters(ticker, s.ctr)

	// Producer: Start buffering input to channel.
	s.mod = mod
	s.input = make(chan string, 10000)
	go s.readLines(reader)

	numr := runtime.GOMAXPROCS(-1)
	s.cnq = make(chan rdf.NQuad, 10000)
	done := make(chan error, numr)
	wg := new(sync.WaitGroup)
	for i := 0; i < numr; i++ {
		wg.Add(1)
		go s.parseStream(done) // Input --> NQuads
		go s.handleNQuads(wg)  // NQuads --> Posting list [slow].
	}

	// The following will block until all ParseStream goroutines finish.
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
