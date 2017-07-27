// Copyright 2015 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package x

import (
	"sync"
	"time"

	"github.com/codahale/hdrhistogram"
)

const (
	// The number of histograms to keep in rolling window.
	histWrapNum = 2
)

// A slidingHistogram is a wrapper around an
// hdrhistogram.WindowedHistogram. The caller must enforce proper
// synchronization.
type slidingHistogram struct {
	windowed *hdrhistogram.WindowedHistogram
	nextT    time.Time
	duration time.Duration
}

// newSlidingHistogram creates a new windowed HDRHistogram with the given
// parameters. Data is kept in the active window for approximately the given
// duration. See the documentation for hdrhistogram.WindowedHistogram for
// details.
func newSlidingHistogram(duration time.Duration, maxVal int64, sigFigs int) *slidingHistogram {
	if duration <= 0 {
		panic("cannot create a sliding histogram with nonpositive duration")
	}
	return &slidingHistogram{
		nextT:    time.Now(),
		duration: duration,
		windowed: hdrhistogram.NewWindowed(histWrapNum, 0, maxVal, sigFigs),
	}
}

func (h *slidingHistogram) tick() {
	h.nextT = h.nextT.Add(h.duration / histWrapNum)
	h.windowed.Rotate()
}

func (h *slidingHistogram) nextTick() time.Time {
	return h.nextT
}

func (h *slidingHistogram) Current() *hdrhistogram.Histogram {
	for h.nextTick().Before(time.Now()) {
		h.tick()
	}
	return h.windowed.Merge()
}

func (h *slidingHistogram) RecordValue(v int64) error {
	return h.windowed.Current.RecordValue(v)
}

// A Histogram collects observed values by keeping bucketed counts. For
// convenience, internally two sets of buckets are kept: A cumulative set (i.e.
// data is never evicted) and a windowed set (which keeps only recently
// collected samples).
//
// Top-level methods generally apply to the cumulative buckets; the windowed
// variant is exposed through the Windowed method.
type Histogram struct {
	sync.Mutex
	cumulative *hdrhistogram.Histogram
	sliding    *slidingHistogram
	maxVal     int64
}

// NewHistogram initializes a given Histogram. The contained windowed histogram
// rotates every 'duration'; both the windowed and the cumulative histogram
// track nonnegative values up to 'maxVal' with 'sigFigs' decimal points of
// precision.
func NewHistogram(duration time.Duration, maxVal int64, sigFigs int) *Histogram {
	dHist := newSlidingHistogram(duration, maxVal, sigFigs)
	h := &Histogram{}
	h.cumulative = hdrhistogram.New(0, maxVal, sigFigs)
	h.sliding = dHist
	h.maxVal = maxVal
	return h
}

// RecordValue adds the given value to the histogram. Recording a value in
// excess of the configured maximum value for that histogram results in
// recording the maximum value instead.
func (h *Histogram) RecordValue(v int64) {
	h.Lock()
	defer h.Unlock()

	if h.sliding.RecordValue(v) != nil {
		_ = h.sliding.RecordValue(h.maxVal)
	}
	if h.cumulative.RecordValue(v) != nil {
		_ = h.cumulative.RecordValue(h.maxVal)
	}
}

func (h *Histogram) Stats() {

}
