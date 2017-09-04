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
	shutdown     chan struct{}

	sorting int64
	writing int64
}

func newProgress() *progress {
	return &progress{
		start:    time.Now(),
		shutdown: make(chan struct{}),
	}
}

func (p *progress) reportProgress() {
	for {
		select {
		case <-time.After(time.Second):
			p.report()
		case <-p.shutdown:
			p.shutdown <- struct{}{}
			return
		}
	}
}

func (p *progress) report() {
	rdfCount := atomic.LoadInt64(&p.rdfCount)
	elapsed := time.Since(p.start)
	fmt.Printf("[%s] [RDF count: %d] [RDFs per second: %d] [sorting: %d] [writing: %d]\n",
		round(elapsed).String(),
		rdfCount,
		int(float64(rdfCount)/elapsed.Seconds()),
		atomic.LoadInt64(&p.sorting),
		atomic.LoadInt64(&p.writing),
	)
	p.lastRDFCount = rdfCount
}

func (p *progress) endSummary() {

	p.shutdown <- struct{}{}
	<-p.shutdown

	p.report()

	total := round(time.Since(p.start))
	fmt.Printf("Total: %v\n", total)
}

func round(d time.Duration) time.Duration {
	return d / 1e9 * 1e9
}
