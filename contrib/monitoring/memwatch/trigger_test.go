/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"
	"time"
)

const gib = 1 << 30

func frac(f float64) uint64 { return uint64(f * gib) }

func testTrigger(t *testing.T, args ...string) (*Trigger, *Ring) {
	t.Helper()
	cfg, err := parseConfig(args, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	return NewTrigger(cfg, t0), NewRing(cfg.RingSize)
}

// step adds a sample with the given anon usage and evaluates it, recording a capture if one is due.
func step(tr *Trigger, r *Ring, sec int, anon uint64) string {
	s := Sample{Time: t0.Add(time.Duration(sec) * time.Second), Incarnation: "a", Total: anon + 100<<20,
		Anon: anon}
	r.Add(s)
	reason := tr.Evaluate(s, r)
	if reason != "" {
		tr.Fired(reason, s.Usage(), s.Time)
	}
	return reason
}

func TestTriggerThresholdsWithHysteresis(t *testing.T) {
	tr, r := testTrigger(t, "--limit-bytes=1Gi", "--rise-bytes=0", "--periodic=0", "--cooldown=0s")
	if got := step(tr, r, 0, frac(0.5)); got != "" {
		t.Fatalf("below thresholds fired %q", got)
	}
	if got := step(tr, r, 1, frac(0.72)); got != "threshold-70" {
		t.Fatalf("got %q, want threshold-70", got)
	}
	if got := step(tr, r, 2, frac(0.74)); got != "" {
		t.Fatalf("disarmed level fired again: %q", got)
	}
	// 0.67 is within the 5-point re-arm band, so the level stays disarmed.
	step(tr, r, 3, frac(0.67))
	if got := step(tr, r, 4, frac(0.71)); got != "" {
		t.Fatalf("level re-armed too early: %q", got)
	}
	// Dropping below 0.65 re-arms it.
	step(tr, r, 5, frac(0.6))
	if got := step(tr, r, 6, frac(0.71)); got != "threshold-70" {
		t.Fatalf("got %q after re-arm, want threshold-70", got)
	}
}

func TestTriggerJumpFiresOnceAtHighestLevel(t *testing.T) {
	tr, r := testTrigger(t, "--limit-bytes=1Gi", "--rise-bytes=0", "--periodic=0", "--cooldown=0s")
	if got := step(tr, r, 0, frac(0.96)); got != "threshold-95" {
		t.Fatalf("got %q, want threshold-95", got)
	}
	if got := step(tr, r, 1, frac(0.97)); got != "" {
		t.Fatalf("lower levels should be disarmed too, got %q", got)
	}
}

func TestTriggerCooldownDefersThreshold(t *testing.T) {
	tr, r := testTrigger(t, "--limit-bytes=1Gi", "--rise-bytes=0", "--periodic=0", "--cooldown=60s")
	if got := step(tr, r, 0, frac(0.72)); got != "threshold-70" {
		t.Fatalf("got %q", got)
	}
	if got := step(tr, r, 10, frac(0.86)); got != "" {
		t.Fatalf("cooldown not respected: %q", got)
	}
	if got := step(tr, r, 61, frac(0.86)); got != "threshold-85" {
		t.Fatalf("deferred threshold should fire after cooldown, got %q", got)
	}
}

func TestTriggerRiseBytes(t *testing.T) {
	tr, r := testTrigger(t, "--rise-bytes=200Mi", "--rise-window=5s", "--periodic=0", "--cooldown=0s")
	// Ordinary GC sawtooth: up and down by 60MiB. Never fires.
	for sec := 0; sec <= 10; sec++ {
		if got := step(tr, r, sec, uint64(300+60*(sec%2))<<20); got != "" {
			t.Fatalf("sawtooth fired %q at %ds", got, sec)
		}
	}
	// A 250MiB jump within one sample fires, even though it averages out to 50MiB/s over the window.
	if got := step(tr, r, 11, 550<<20); got != "rise" {
		t.Fatalf("one-sample jump: got %q, want rise", got)
	}
	// Slow growth of 30MiB/s never builds up 200MiB inside a 5s window.
	for sec := 20; sec <= 40; sec++ {
		if got := step(tr, r, sec, uint64(600+30*(sec-20))<<20); got != "" {
			t.Fatalf("slow growth fired %q at %ds", got, sec)
		}
	}
}

func TestTriggerRiseIgnoresNewIncarnation(t *testing.T) {
	tr, r := testTrigger(t, "--rise-bytes=1Mi", "--rise-window=5s", "--periodic=0")
	r.Add(Sample{Time: t0, Incarnation: "old", Anon: 1})
	cur := Sample{Time: t0.Add(2 * time.Second), Incarnation: "new", Anon: gib}
	r.Add(cur)
	if got := tr.Evaluate(cur, r); got != "" {
		t.Fatalf("compared across incarnations: %q", got)
	}
}

func TestTriggerPeriodic(t *testing.T) {
	tr, r := testTrigger(t, "--periodic=30m", "--rise-bytes=0", "--cooldown=2m")
	if got := step(tr, r, 60, gib/4); got != "" {
		t.Fatalf("periodic fired before the first delay: %q", got)
	}
	if got := step(tr, r, int(firstPeriodicDelay.Seconds()), gib/4); got != "periodic" {
		t.Fatalf("got %q, want first periodic capture", got)
	}
	next := int((firstPeriodicDelay + 30*time.Minute).Seconds())
	if got := step(tr, r, next-1, gib/4); got != "" {
		t.Fatalf("periodic fired early: %q", got)
	}
	if got := step(tr, r, next, gib/4); got != "periodic" {
		t.Fatalf("got %q, want periodic", got)
	}
}

func TestTriggerNoLimitDisablesThresholds(t *testing.T) {
	tr, r := testTrigger(t, "--rise-bytes=0", "--periodic=0")
	if got := step(tr, r, 0, 100*gib); got != "" {
		t.Fatalf("thresholds without a limit fired %q", got)
	}
}
