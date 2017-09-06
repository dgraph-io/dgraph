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

	start       time.Time
	startReduce time.Time

	// shutdown is a bidirectional channel used to manage the stopping of the
	// report goroutine. It handles both the request to stop the report
	// goroutine, as well as the message back to say that the goroutine has
	// stopped. The channel MUST be unbuffered for this to work.
	shutdown chan struct{}
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
	reduceEdgeCount := atomic.LoadInt64(&p.reduceEdgeCount)

	if reduceEdgeCount == 0 {
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
		reduceEdgeCount := atomic.LoadInt64(&p.reduceEdgeCount)
		fmt.Printf("[REDUCE] [%s] [%.2f%%] [Edge count: %d] [Edges per second: %d]\n",
			round(now.Sub(p.start)).String(),
			100*float64(reduceEdgeCount)/float64(mapEdgeCount),
			reduceEdgeCount,
			int(float64(reduceEdgeCount)/elapsed.Seconds()),
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
