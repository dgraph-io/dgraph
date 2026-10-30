/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"
)

func TestMetricsSeedEveryCaptureReason(t *testing.T) {
	m := NewMetrics([]float64{0.7, 0.85})
	families, err := m.reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]float64{}
	for _, f := range families {
		if f.GetName() != "memwatch_captures_total" {
			continue
		}
		for _, metric := range f.GetMetric() {
			for _, l := range metric.GetLabel() {
				if l.GetName() == "reason" {
					seen[l.GetValue()] = metric.GetCounter().GetValue()
				}
			}
		}
	}
	for _, r := range []string{"rise", "periodic", "restart", "threshold-70", "threshold-85"} {
		v, ok := seen[r]
		if !ok || v != 0 {
			t.Errorf("reason %q: present=%v value=%v, want a zero series", r, ok, v)
		}
	}
}
