// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadog.com/).
// Copyright 2018 Datadog, Inc.

package datadog

import (
	"bytes"
	"io"
	"sync"
	"time"

	"go.opencensus.io/trace"
)

const (
	// payloadLimit specifies the maximum payload size that the Datadog
	// agent will accept. Request bodies larger than this will be rejected.
	payloadLimit = int(1e7) // 10MB

	// defaultService specifies the default service name that will be used
	// with the registered traces. Users should normally specify a different
	// service name.
	defaultService = "opencensus-app"
)

// allows tests to override
var (
	// inChannelSize specifies the size of the buffered channel which
	// takes spans and adds them to the payload.
	inChannelSize = int(5e5) // 500K (approx 61MB memory if full)

	// flushThreshold specifies the payload's size threshold in bytes. If it
	// is exceeded, a flush will be triggered.
	flushThreshold = payloadLimit / 2

	// flushInterval specifies the interval at which the payload will
	// automatically be flushed.
	flushInterval = 2 * time.Second
)

type traceExporter struct {
	opts    Options
	payload *payload
	errors  *errorAmortizer
	sampler *prioritySampler

	// uploadFn specifies the function used for uploading.
	// Defaults to (*transport).upload; replaced in tests.
	uploadFn func(pkg *bytes.Buffer, count int) (io.ReadCloser, error)

	wg   sync.WaitGroup // counts active uploads
	in   chan *ddSpan
	exit chan struct{}
}

func newTraceExporter(o Options) *traceExporter {
	if o.Service == "" {
		o.Service = defaultService
	}
	sampler := newPrioritySampler()
	e := &traceExporter{
		opts:     o,
		payload:  newPayload(),
		errors:   newErrorAmortizer(defaultErrorFreq, o.OnError),
		sampler:  sampler,
		uploadFn: newTransport(o.TraceAddr).upload,
		in:       make(chan *ddSpan, inChannelSize),
		exit:     make(chan struct{}),
	}

	go e.loop()

	return e
}

func (e *traceExporter) exportSpan(s *trace.SpanData) {
	select {
	case e.in <- e.convertSpan(s):
		// ok
	default:
		e.errors.log(errorTypeOverflow, nil)
	}
}

func (e *traceExporter) loop() {
	defer close(e.exit)
	tick := time.NewTicker(flushInterval)
	defer tick.Stop()

	for {
		select {
		case span := <-e.in:
			e.receiveSpan(span)

		case <-tick.C:
			e.flush()

		case <-e.exit:
			e.flush()
			e.wg.Wait() // wait for uploads to finish
			e.errors.flush()
			return
		}
	}
}

func (e *traceExporter) receiveSpan(span *ddSpan) {
	if _, ok := span.Metrics[keySamplingPriority]; !ok {
		e.sampler.applyPriority(span)
	}
	if err := e.payload.add(span); err != nil {
		e.errors.log(errorTypeEncoding, err)
	}
	if e.payload.size() > flushThreshold {
		e.flush()
	}
}

func (e *traceExporter) flush() {
	n := len(e.payload.traces)
	if n == 0 {
		return
	}
	buf := e.payload.buffer()
	e.wg.Add(1)
	go func() {
		body, err := e.uploadFn(buf, n)
		if err != nil {
			e.errors.log(errorTypeTransport, err)
		} else {
			e.sampler.readRatesJSON(body) // do we care about errors?
		}
		e.wg.Done()
	}()
	e.payload.reset()
}

// stop cleanly stops the exporter, flushing any remaining spans to the transport and
// reporting any errors. Make sure to always call Stop at the end of your program in
// order to not lose any tracing data. Only call Stop once per exporter. Repeated calls
// will cause panic.
func (e *traceExporter) stop() {
loop:
	// drain the input channel to catch anything the loop might not
	for {
		select {
		case span := <-e.in:
			e.receiveSpan(span)
		default:
			break loop
		}
	}
	e.exit <- struct{}{}
	<-e.exit
}
