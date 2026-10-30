/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"encoding/csv"
	"io"
	"strconv"
	"sync"
	"time"
)

// Ring keeps the most recent samples of one alpha incarnation. It is safe for concurrent use: the sampling loop
// adds to it while a capture goroutine snapshots it.
type Ring struct {
	mu    sync.Mutex
	buf   []Sample
	start int
	n     int
}

func NewRing(size int) *Ring {
	return &Ring{buf: make([]Sample, size)}
}

func (r *Ring) Add(s Sample) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.n < len(r.buf) {
		r.buf[(r.start+r.n)%len(r.buf)] = s
		r.n++
		return
	}
	r.buf[r.start] = s
	r.start = (r.start + 1) % len(r.buf)
}

// Reset drops every sample. It is called when the alpha restarts so a new incarnation's history doesn't mix with
// the old one's.
func (r *Ring) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.start, r.n = 0, 0
}

func (r *Ring) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

// Snapshot returns the samples oldest first.
func (r *Ring) Snapshot() []Sample {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Sample, r.n)
	for i := 0; i < r.n; i++ {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}

// MinUsageSince returns the lowest Usage among samples of the given incarnation taken at or after since.
func (r *Ring) MinUsageSince(since time.Time, incarnation string) (uint64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var low uint64
	found := false
	for i := r.n - 1; i >= 0; i-- {
		s := r.buf[(r.start+i)%len(r.buf)]
		if s.Time.Before(since) {
			break
		}
		if s.Incarnation != incarnation {
			continue
		}
		if u := s.Usage(); !found || u < low {
			low, found = u, true
		}
	}
	return low, found
}

// WindowMax returns the largest Total, Anon and HWM among samples taken after since.
func (r *Ring) WindowMax(since time.Time) (total, anon, hwm uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := r.n - 1; i >= 0; i-- {
		s := r.buf[(r.start+i)%len(r.buf)]
		if s.Time.Before(since) {
			break
		}
		total, anon, hwm = max(total, s.Total), max(anon, s.Anon), max(hwm, s.HWM)
	}
	return total, anon, hwm
}

var timelineHeader = []string{"time", "pid", "rss_bytes", "anon_bytes", "file_bytes", "shmem_bytes", "hwm_bytes"}

// WriteTimelineCSV writes samples as CSV, oldest first.
func WriteTimelineCSV(w io.Writer, samples []Sample) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(timelineHeader); err != nil {
		return err
	}
	u := func(v uint64) string { return strconv.FormatUint(v, 10) }
	for _, s := range samples {
		rec := []string{
			s.Time.UTC().Format(time.RFC3339Nano), strconv.Itoa(s.PID),
			u(s.Total), u(s.Anon), u(s.File), u(s.Shmem), u(s.HWM),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
