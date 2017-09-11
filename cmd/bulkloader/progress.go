package main

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
	switch phase(atomic.LoadInt32((*int32)(&p.phase))) {
	case nothing:
	case mapPhase:
		rdfCount := atomic.LoadInt64(&p.rdfCount)
		elapsed := time.Since(p.start)
		fmt.Printf("[MAP] [%s] [RDF count: %d] [Edge count: %d] "+
			"[RDFs per second: %d] [Edges per second: %d]\n",
			fixedDuration(elapsed),
			rdfCount,
			mapEdgeCount,
			int(float64(rdfCount)/elapsed.Seconds()),
			int(float64(mapEdgeCount)/elapsed.Seconds()),
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
			pct = fmt.Sprintf("[%.2f%%] ", 100*float64(reduceEdgeCount)/float64(mapEdgeCount))
		}
		fmt.Printf("[REDUCE] [%s] %s[Edge count: %d] [Edges per second: %d] "+
			"[Posting list count: %d] [Posting lists per second: %d]\n",
			fixedDuration(now.Sub(p.start)),
			pct,
			reduceEdgeCount,
			int(float64(reduceEdgeCount)/elapsed.Seconds()),
			reduceKeyCount,
			int(float64(reduceKeyCount)/elapsed.Seconds()),
		)
	default:
		x.AssertTruef(false, "invalid phase")
	}
}

func (p *progress) endSummary() {
	p.shutdown <- struct{}{}
	<-p.shutdown

	p.reportOnce()

	total := fixedDuration(time.Since(p.start))
	fmt.Printf("Total: %v\n", total)
}

func fixedDuration(d time.Duration) string {
	str := fmt.Sprintf("%02ds", int(d.Seconds())%60)
	if d >= time.Minute {
		str = fmt.Sprintf("%02dm", int(d.Minutes())%60) + str
	}
	if d >= time.Hour {
		str = fmt.Sprintf("%02dh", int(d.Hours())) + str
	}
	return str
}
