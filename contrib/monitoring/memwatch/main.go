/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// memwatch is a flight recorder for Dgraph alphas. It runs next to an alpha (as a Kubernetes sidecar, or a
// Compose service sharing the alpha's PID namespace), samples the alpha's resident memory every second, and saves
// heap profiles, goroutine dumps and the recent memory timeline when memory crosses a threshold, grows quickly, or
// the alpha restarts. It also exports the peak memory seen between Prometheus scrapes. It talks to the alpha only
// through HTTP endpoints present in both v24 and v25. See contrib/monitoring/README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "memwatch:", err)
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *Config, log *slog.Logger) error {
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return err
	}
	m := NewMetrics()
	m.limit.Set(float64(cfg.LimitBytes))

	mux := newMux(m)
	if cfg.ServeCaptures {
		registerCaptureHandlers(mux, cfg.OutDir)
	}
	srv := &http.Server{Addr: cfg.Listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server failed", "err", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	sampler, err := chooseSampler(ctx, cfg, log)
	if err != nil {
		return err
	}
	m.samplingMode.WithLabelValues(sampler.Mode()).Set(1)
	interval := cfg.Interval
	if sampler.Mode() == "http" {
		interval = cfg.HTTPInterval
	}
	if cfg.LimitBytes == 0 {
		log.Warn("no --limit-bytes set; threshold captures are disabled")
	}
	log.Info("memwatch started", "mode", sampler.Mode(), "interval", interval, "limit_bytes", cfg.LimitBytes,
		"thresholds", cfg.Thresholds, "out_dir", cfg.OutDir)

	w := &watcher{
		cfg:      cfg,
		log:      log,
		metrics:  m,
		sampler:  sampler,
		ring:     NewRing(cfg.RingSize),
		trigger:  NewTrigger(cfg, time.Now()),
		capturer: NewCapturer(cfg, sampler.Mode(), m.captureErrs.Inc),
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		w.tick(ctx)
		select {
		case <-ctx.Done():
			w.wg.Wait()
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func newMux(m *Metrics) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

// chooseSampler resolves --mode=auto: proc mode if an alpha process becomes visible within --discover-timeout,
// http mode otherwise.
func chooseSampler(ctx context.Context, cfg *Config, log *slog.Logger) (Sampler, error) {
	switch cfg.Mode {
	case "proc":
		return newProcSampler(cfg.ProcRoot, cfg.ProcMatch), nil
	case "http":
		return newHTTPSampler(cfg.AlphaURL, cfg.FetchTimeout), nil
	}
	if _, err := os.Stat(filepath.Join(cfg.ProcRoot, "self")); err != nil {
		log.Info("no procfs, using http mode", "proc_root", cfg.ProcRoot)
		return newHTTPSampler(cfg.AlphaURL, cfg.FetchTimeout), nil
	}
	ps := newProcSampler(cfg.ProcRoot, cfg.ProcMatch)
	deadline := time.Now().Add(cfg.DiscoverTimeout)
	for {
		if _, err := ps.find(); err == nil {
			return ps, nil
		}
		if time.Now().After(deadline) {
			log.Info("no alpha process visible, using http mode; share the PID namespace to use proc mode",
				"proc_match", cfg.ProcMatch)
			return newHTTPSampler(cfg.AlphaURL, cfg.FetchTimeout), nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// watcher is the sampling loop's state.
type watcher struct {
	cfg      *Config
	log      *slog.Logger
	metrics  *Metrics
	sampler  Sampler
	ring     *Ring
	trigger  *Trigger
	capturer *Capturer

	last     Sample // newest sample of the current incarnation; zero while the alpha is down
	inFlight atomic.Bool
	wg       sync.WaitGroup
}

func (w *watcher) tick(ctx context.Context) {
	cur, err := w.sampler.Sample(ctx)
	if err != nil {
		w.metrics.targetUp.Set(0)
		if errors.Is(err, errNotFound) {
			if w.last.Incarnation != "" {
				w.restarted(time.Now(), "alpha process gone")
			}
		} else {
			w.log.Warn("sampling failed", "err", err)
		}
		return
	}
	w.metrics.targetUp.Set(1)
	if w.last.Incarnation != "" && cur.Incarnation != w.last.Incarnation {
		w.restarted(cur.Time, "alpha process replaced")
	}
	w.last = cur
	w.ring.Add(cur)

	total, anon, _ := w.ring.WindowMax(cur.Time.Add(-w.cfg.Window))
	w.metrics.observe(cur, total, anon, w.sampler.Mode())

	reason := w.trigger.Evaluate(cur, w.ring)
	if reason == "" || w.inFlight.Load() {
		return
	}
	w.trigger.Fired(reason, cur.Usage(), cur.Time)
	w.inFlight.Store(true)
	timeline := w.ring.Snapshot()
	w.wg.Go(func() {
		defer w.inFlight.Store(false)
		dir, err := w.capturer.Capture(ctx, reason, cur, timeline)
		if err != nil {
			w.log.Error("capture failed", "reason", reason, "dir", dir, "err", err)
			return
		}
		w.metrics.captures.WithLabelValues(reason).Inc()
		w.metrics.lastCapture.Set(float64(time.Now().Unix()))
		w.log.Info("capture written", "reason", reason, "dir", dir, "usage_bytes", cur.Usage())
	})
}

// restarted saves the timeline of the incarnation that just ended and starts a fresh one.
func (w *watcher) restarted(now time.Time, why string) {
	w.metrics.restarts.Inc()
	last := w.last
	timeline := w.ring.Snapshot()
	w.ring.Reset()
	w.last = Sample{}
	w.log.Warn(why, "pid", last.PID, "last_rss_bytes", last.Total, "hwm_bytes", last.HWM)
	if len(timeline) == 0 {
		return
	}
	dir, err := w.capturer.RestartDump(last, timeline, now)
	if err != nil {
		w.log.Error("restart dump failed", "dir", dir, "err", err)
		return
	}
	w.metrics.captures.WithLabelValues("restart").Inc()
	w.metrics.lastCapture.Set(float64(now.Unix()))
	w.log.Info("restart timeline written", "dir", dir)
}
