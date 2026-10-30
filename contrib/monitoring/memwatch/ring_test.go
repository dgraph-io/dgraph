/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 28, 18, 0, 0, 0, time.UTC)

func at(sec int, total uint64) Sample {
	return Sample{Time: t0.Add(time.Duration(sec) * time.Second), Incarnation: "a", PID: 7, Total: total,
		Anon: total / 2, HWM: total}
}

func TestRingWrapsAndKeepsOrder(t *testing.T) {
	r := NewRing(3)
	for i := 1; i <= 5; i++ {
		r.Add(at(i, uint64(i)))
	}
	got := r.Snapshot()
	if len(got) != 3 || got[0].Total != 3 || got[2].Total != 5 {
		t.Fatalf("snapshot = %+v", got)
	}
	r.Reset()
	if r.Len() != 0 || len(r.Snapshot()) != 0 {
		t.Fatal("reset did not empty the ring")
	}
}

func TestRingMinUsageAndWindowMax(t *testing.T) {
	r := NewRing(10)
	for i, v := range []uint64{10, 80, 30, 20} {
		r.Add(at(i, v))
	}
	// Usage is Anon, which at() sets to Total/2.
	if low, ok := r.MinUsageSince(t0.Add(time.Second), "a"); !ok || low != 10 {
		t.Fatalf("MinUsageSince = %d, %v", low, ok)
	}
	if _, ok := r.MinUsageSince(t0.Add(time.Hour), "a"); ok {
		t.Fatal("no samples are that recent")
	}
	if _, ok := r.MinUsageSince(t0, "other"); ok {
		t.Fatal("samples of another incarnation must be ignored")
	}
	total, anon, hwm := r.WindowMax(t0.Add(2 * time.Second))
	if total != 30 || anon != 15 || hwm != 30 {
		t.Fatalf("window max = %d %d %d", total, anon, hwm)
	}
	if total, _, _ := r.WindowMax(t0); total != 80 {
		t.Fatalf("full window max = %d", total)
	}
}

func TestWriteTimelineCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteTimelineCSV(&buf, []Sample{at(0, 100), at(1, 200)}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "time,pid,rss_bytes") ||
		lines[2] != "2026-10-28T18:00:01Z,7,200,100,0,0,200" {
		t.Fatalf("csv = %q", buf.String())
	}
}
