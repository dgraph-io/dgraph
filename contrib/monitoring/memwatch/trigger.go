/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"fmt"
	"math"
	"time"
)

// Trigger decides when to take a capture. It is not safe for concurrent use; the sampling loop owns it.
type Trigger struct {
	limit      uint64
	thresholds []float64
	armed      []bool
	rearm      float64
	riseBytes  uint64
	riseWindow time.Duration
	periodic   time.Duration
	cooldown   time.Duration

	lastFired    time.Time
	nextPeriodic time.Time
}

// firstPeriodicDelay is when the first baseline capture is taken, so a heap diff has an early profile to compare
// against without waiting a full --periodic interval.
const firstPeriodicDelay = 5 * time.Minute

func NewTrigger(c *Config, now time.Time) *Trigger {
	t := &Trigger{
		limit:      c.LimitBytes,
		thresholds: c.Thresholds,
		armed:      make([]bool, len(c.Thresholds)),
		rearm:      c.Rearm,
		riseBytes:  c.RiseBytes,
		riseWindow: c.RiseWindow,
		periodic:   c.Periodic,
		cooldown:   c.Cooldown,
	}
	for i := range t.armed {
		t.armed[i] = true
	}
	if t.periodic > 0 {
		t.nextPeriodic = now.Add(min(firstPeriodicDelay, t.periodic))
	}
	return t
}

// Evaluate looks at the newest sample and returns the reason for a capture, or "" if none is due. A non-empty
// reason means the caller should capture and then call Fired. The ring must already contain cur.
func (t *Trigger) Evaluate(cur Sample, ring *Ring) string {
	now := cur.Time
	reason := t.thresholdReason(cur.Usage())
	if reason == "" {
		reason = t.riseReason(cur, ring)
	}
	if reason == "" && t.periodic > 0 && !now.Before(t.nextPeriodic) {
		reason = "periodic"
	}
	if reason == "" {
		return ""
	}
	if !t.lastFired.IsZero() && now.Sub(t.lastFired) < t.cooldown {
		// Leave the threshold armed so it fires once the cooldown ends, if memory is still above it.
		return ""
	}
	return reason
}

// Fired records that a capture for reason was started at now.
func (t *Trigger) Fired(reason string, usage uint64, now time.Time) {
	t.lastFired = now
	if t.periodic > 0 {
		// Any capture also serves as the next baseline.
		t.nextPeriodic = now.Add(t.periodic)
	}
	if t.limit == 0 {
		return
	}
	// Disarm every level at or below the current usage, so a jump from 60% to 96% gives one capture, not three.
	frac := float64(usage) / float64(t.limit)
	for i, th := range t.thresholds {
		if frac >= th {
			t.armed[i] = false
		}
	}
}

// thresholdReason re-arms levels memory has dropped well below, and returns the highest armed level crossed.
func (t *Trigger) thresholdReason(usage uint64) string {
	if t.limit == 0 {
		return ""
	}
	frac := float64(usage) / float64(t.limit)
	reason := ""
	for i, th := range t.thresholds {
		switch {
		case !t.armed[i] && frac < th-t.rearm:
			t.armed[i] = true
		case t.armed[i] && frac >= th:
			reason = fmt.Sprintf("threshold-%d", int(math.Round(th*100)))
		}
	}
	return reason
}

// riseReason fires when usage is at least riseBytes above its lowest point in the last riseWindow. Using the low
// point rather than an average rate means a jump that happens within one sample still counts in full.
func (t *Trigger) riseReason(cur Sample, ring *Ring) string {
	if t.riseBytes == 0 || t.riseWindow <= 0 {
		return ""
	}
	low, ok := ring.MinUsageSince(cur.Time.Add(-t.riseWindow), cur.Incarnation)
	if !ok || cur.Usage() <= low {
		return ""
	}
	if cur.Usage()-low >= t.riseBytes {
		return "rise"
	}
	return ""
}
