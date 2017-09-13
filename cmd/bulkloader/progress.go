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

	mappersRunning int64
	lenBatchCh     func() int

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
		fmt.Printf("MAP %s rdf_count:%s rdf_speed:%s/sec edge_count:%s edge_speed:%s/sec DEBUG(MR:%d LBC:%d)\n",
			fixedDuration(elapsed),
			niceFloat(float64(rdfCount)),
			niceFloat(float64(rdfCount)/elapsed.Seconds()),
			niceFloat(float64(mapEdgeCount)),
			niceFloat(float64(mapEdgeCount)/elapsed.Seconds()),
			atomic.LoadInt64(&p.mappersRunning),
			p.lenBatchCh(),
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
		fmt.Printf("REDUCE %s %sedge_count:%s edge_speed:%s/sec "+
			"plist_count:%s plist_speed:%s/sec\n",
			fixedDuration(now.Sub(p.start)),
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
