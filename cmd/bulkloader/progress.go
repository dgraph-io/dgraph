package main

import (
	"fmt"
	"sync/atomic"
	"time"
)

type progress struct {
	rdfCount     int64
	lastRDFCount int64
	start        time.Time

	// shotdown is a uni-directional channel used to manage the stopping of the
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
	rdfCount := atomic.LoadInt64(&p.rdfCount)
	elapsed := time.Since(p.start)
	fmt.Printf("[%s] [RDF count: %d] [RDFs per second: %d]\n",
		round(elapsed).String(),
		rdfCount,
		int(float64(rdfCount)/elapsed.Seconds()),
	)
	p.lastRDFCount = rdfCount
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
