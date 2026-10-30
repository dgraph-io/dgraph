/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics are memwatch's own Prometheus series. Names are stable; the bundled alert rules and dashboard use them.
type Metrics struct {
	reg *prometheus.Registry

	rss          *prometheus.GaugeVec
	windowMax    *prometheus.GaugeVec
	hwm          prometheus.Gauge
	limit        prometheus.Gauge
	targetUp     prometheus.Gauge
	captures     *prometheus.CounterVec
	captureErrs  prometheus.Counter
	restarts     prometheus.Counter
	lastCapture  prometheus.Gauge
	samplingMode *prometheus.GaugeVec
}

func NewMetrics() *Metrics {
	m := &Metrics{
		reg: prometheus.NewRegistry(),
		rss: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "memwatch_rss_bytes",
			Help: "Resident memory of the alpha process by type (total, anon, file, shmem).",
		}, []string{"type"}),
		windowMax: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "memwatch_rss_window_max_bytes",
			Help: "Peak resident memory of the alpha over the trailing --window, by type (total, anon). " +
				"Shows spikes that start and end between Prometheus scrapes.",
		}, []string{"type"}),
		hwm: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "memwatch_vm_hwm_bytes",
			Help: "Peak resident memory of the alpha since it started (VmHWM). Only set in proc mode.",
		}),
		limit: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "memwatch_limit_bytes",
			Help: "Memory limit memwatch compares against, from --limit-bytes. 0 when unset.",
		}),
		targetUp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "memwatch_target_up",
			Help: "1 if the last sample of the alpha succeeded, 0 otherwise.",
		}),
		captures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "memwatch_captures_total",
			Help: "Captures written, by reason (threshold-NN, rise, periodic, restart).",
		}, []string{"reason"}),
		captureErrs: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "memwatch_capture_errors_total",
			Help: "Capture artifacts that could not be saved.",
		}),
		restarts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "memwatch_target_restarts_total",
			Help: "Times the alpha process was seen to restart.",
		}),
		lastCapture: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "memwatch_last_capture_timestamp_seconds",
			Help: "Unix time of the most recent capture.",
		}),
		samplingMode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "memwatch_sampling_mode",
			Help: "1 for the sampling mode in use (proc or http).",
		}, []string{"mode"}),
	}
	m.reg.MustRegister(m.rss, m.windowMax, m.hwm, m.limit, m.targetUp, m.captures, m.captureErrs, m.restarts,
		m.lastCapture, m.samplingMode)
	// Pre-create the reasons so rate() works from the first capture.
	for _, r := range []string{"rise", "periodic", "restart"} {
		m.captures.WithLabelValues(r)
	}
	return m
}

func (m *Metrics) observe(s Sample, total, anon uint64, mode string) {
	m.rss.WithLabelValues("total").Set(float64(s.Total))
	m.windowMax.WithLabelValues("total").Set(float64(total))
	if mode == "proc" {
		m.rss.WithLabelValues("anon").Set(float64(s.Anon))
		m.rss.WithLabelValues("file").Set(float64(s.File))
		m.rss.WithLabelValues("shmem").Set(float64(s.Shmem))
		m.windowMax.WithLabelValues("anon").Set(float64(anon))
		m.hwm.Set(float64(s.HWM))
	}
}
