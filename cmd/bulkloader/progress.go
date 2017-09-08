package main

import (
	"fmt"
	"sync/atomic"
	"time"
)

type progress struct {
	rdfCount        int64
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

	reducePhase int32 // bool
}

func newProgress() *progress {
	return &progress{
		start:    time.Now(),
		shutdown: make(chan struct{}),
	}
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
	reducePhase := atomic.LoadInt32(&p.reducePhase) != 0

	if !reducePhase {
		rdfCount := atomic.LoadInt64(&p.rdfCount)
		elapsed := time.Since(p.start)
		fmt.Printf("[MAP] [%s] [RDF count: %d] [Edge count: %d] "+
			"[RDFs per second: %d] [Edges per second: %d]\n",
			round(elapsed).String(),
			rdfCount,
			mapEdgeCount,
			int(float64(rdfCount)/elapsed.Seconds()),
			int(float64(mapEdgeCount)/elapsed.Seconds()),
		)
	} else {
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
			pct = fmt.Sprintf("[%.2f%%] ", 100*float64(reduceEdgeCount)/float64(mapEdgeCount))
		}
		fmt.Printf("[REDUCE] [%s] %s[Edge count: %d] [Edges per second: %d] "+
			"[Posting list count: %d] [Posting lists per second: %d]\n",
			round(now.Sub(p.start)).String(),
			pct,
			reduceEdgeCount,
			int(float64(reduceEdgeCount)/elapsed.Seconds()),
			reduceKeyCount,
			int(float64(reduceKeyCount)/elapsed.Seconds()),
		)
	}
}

func (p *progress) endSummary() {
	p.shutdown <- struct{}{}
	<-p.shutdown

	p.reportOnce()

	total := round(time.Since(p.start))
	fmt.Printf("Total: %v\n", total)
}

func round(d time.Duration) time.Duration {
	return d / 1e9 * 1e9
}
