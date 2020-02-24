/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package bulk

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

type phase int32

const (
	nothing phase = iota
	mapPhase
	reducePhase
)

type progress struct {
	nquadCount      int64
	errCount        int64
	mapEdgeCount    int64
	reduceEdgeCount int64
	reduceKeyCount  int64

	start       time.Time
	startReduce time.Time

	// shutdown is a bidirectional channel used to manage the stopping of the
	// report goroutine. It handles both the request to stop the report
	// goroutine, as well as the message back to say that the goroutine has
	// stopped. The channel MUST be unbuffered for this to work.
	shutdown chan struct{}

	phase phase
}

func newProgress() *progress {
	return &progress{
		start:    time.Now(),
		shutdown: make(chan struct{}),
	}
}

func (p *progress) setPhase(ph phase) {
	atomic.StoreInt32((*int32)(&p.phase), int32(ph))
}

func (p *progress) report() {
	for {
		select {
		case <-time.After(time.Second):
			p.reportOnce()
		case <-p.shutdown:
			p.shutdown <- struct{}{}
			return
		}
	}
}

func (p *progress) reportOnce() {
	mapEdgeCount := atomic.LoadInt64(&p.mapEdgeCount)
	timestamp := time.Now().Format("15:04:05Z0700")
	switch phase(atomic.LoadInt32((*int32)(&p.phase))) {
	case nothing:
	case mapPhase:
		rdfCount := atomic.LoadInt64(&p.nquadCount)
		errCount := atomic.LoadInt64(&p.errCount)
		elapsed := time.Since(p.start)
		fmt.Printf("[%s] MAP %s nquad_count:%s err_count:%s nquad_speed:%s/sec "+
			"edge_count:%s edge_speed:%s/sec\n",
			timestamp,
			x.FixedDuration(elapsed),
			niceFloat(float64(rdfCount)),
			niceFloat(float64(errCount)),
			niceFloat(float64(rdfCount)/elapsed.Seconds()),
			niceFloat(float64(mapEdgeCount)),
			niceFloat(float64(mapEdgeCount)/elapsed.Seconds()),
		)
	case reducePhase:
		now := time.Now()
		elapsed := time.Since(p.startReduce)
		if p.startReduce.IsZero() {
			p.startReduce = time.Now()
			elapsed = time.Second
		}
		reduceKeyCount := atomic.LoadInt64(&p.reduceKeyCount)
		reduceEdgeCount := atomic.LoadInt64(&p.reduceEdgeCount)
		pct := ""
		if mapEdgeCount != 0 {
			pct = fmt.Sprintf("%.2f%% ", 100*float64(reduceEdgeCount)/float64(mapEdgeCount))
		}
		fmt.Printf("[%s] REDUCE %s %sedge_count:%s edge_speed:%s/sec "+
			"plist_count:%s plist_speed:%s/sec\n",
			timestamp,
			x.FixedDuration(now.Sub(p.start)),
			pct,
			niceFloat(float64(reduceEdgeCount)),
			niceFloat(float64(reduceEdgeCount)/elapsed.Seconds()),
			niceFloat(float64(reduceKeyCount)),
			niceFloat(float64(reduceKeyCount)/elapsed.Seconds()),
		)
	default:
		x.AssertTruef(false, "invalid phase")
	}
}

func (p *progress) endSummary() {
	p.shutdown <- struct{}{}
	<-p.shutdown

	p.reportOnce()

	total := x.FixedDuration(time.Since(p.start))
	fmt.Printf("Total: %v\n", total)
}

var suffixes = [...]string{"", "k", "M", "G", "T"}

func niceFloat(f float64) string {
	idx := 0
	for f >= 1000 {
		f /= 1000
		idx++
	}
	if idx >= len(suffixes) {
		return fmt.Sprintf("%f", f)
	}
	suf := suffixes[idx]
	switch {
	case f >= 100:
		return fmt.Sprintf("%.1f%s", f, suf)
	case f >= 10:
		return fmt.Sprintf("%.2f%s", f, suf)
	default:
		return fmt.Sprintf("%.3f%s", f, suf)
	}
}
