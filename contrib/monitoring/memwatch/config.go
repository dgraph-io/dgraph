/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Config holds every memwatch setting. Each flag can also be set through an environment variable named
// MEMWATCH_<FLAG>, with dashes turned into underscores (e.g. --limit-bytes → MEMWATCH_LIMIT_BYTES). A flag given
// on the command line wins over the environment.
type Config struct {
	Mode            string // auto, proc or http
	AlphaURL        string
	ProcRoot        string
	ProcMatch       []string
	DiscoverTimeout time.Duration

	Interval     time.Duration
	HTTPInterval time.Duration
	RingSize     int
	Window       time.Duration

	LimitBytes uint64
	Thresholds []float64
	Rearm      float64
	RiseBytes  uint64 // growth within RiseWindow that triggers a capture; 0 disables
	RiseWindow time.Duration
	Periodic   time.Duration
	Cooldown   time.Duration

	OutDir         string
	MaxCaptures    int
	MaxBytes       uint64
	FetchTimeout   time.Duration
	ProfileTimeout time.Duration

	Listen        string
	ServeCaptures bool
}

func parseConfig(args []string, getenv func(string) string) (*Config, error) {
	fs := flag.NewFlagSet("memwatch", flag.ContinueOnError)
	var (
		c                                            Config
		procMatch, limit, thresholds, rise, maxBytes string
	)
	fs.StringVar(&c.Mode, "mode", "auto",
		"How to sample memory: proc (read /proc/<pid>/status, needs a shared PID namespace), http (scrape the "+
			"alpha's Prometheus endpoint), or auto (proc if an alpha process is visible, else http).")
	fs.StringVar(&c.AlphaURL, "alpha-url", "http://localhost:8080", "Base URL of the alpha's HTTP port.")
	fs.StringVar(&c.ProcRoot, "proc-root", "/proc", "Mount point of procfs.")
	fs.StringVar(&procMatch, "proc-match", "dgraph,alpha",
		"Comma-separated words that must all appear in a process's command line for it to count as the alpha. "+
			"The first word is matched against the basename of argv[0].")
	fs.DurationVar(&c.DiscoverTimeout, "discover-timeout", 30*time.Second,
		"In auto mode, how long to look for an alpha process before falling back to http mode.")
	fs.DurationVar(&c.Interval, "interval", time.Second, "Sampling interval in proc mode.")
	fs.DurationVar(&c.HTTPInterval, "http-interval", 5*time.Second, "Sampling interval in http mode.")
	fs.IntVar(&c.RingSize, "ring-size", 600, "Number of recent samples kept and written to timeline.csv.")
	fs.DurationVar(&c.Window, "window", time.Minute,
		"Window for memwatch_rss_window_max_bytes, the peak RSS seen recently. Keep it longer than the "+
			"Prometheus scrape interval so short spikes are never missed.")
	fs.StringVar(&limit, "limit-bytes", "",
		"Memory limit of the alpha container (bytes, or with a Ki/Mi/Gi suffix). Usually injected through the "+
			"downward API. Without it, threshold captures are disabled.")
	fs.StringVar(&thresholds, "thresholds", "0.70,0.85,0.95",
		"Fractions of --limit-bytes that trigger a capture when memory crosses them.")
	fs.Float64Var(&c.Rearm, "rearm", 0.05,
		"A threshold re-arms once memory drops this fraction of the limit below it.")
	fs.StringVar(&rise, "rise-bytes", "auto",
		"Trigger a capture when memory grows by this much (bytes, or with a Ki/Mi/Gi suffix) above its lowest "+
			"point in the last --rise-window. auto is 20% of --limit-bytes, or 256Mi without a limit. 0 disables.")
	fs.DurationVar(&c.RiseWindow, "rise-window", 5*time.Second, "Window for --rise-bytes.")
	fs.DurationVar(&c.Periodic, "periodic", 30*time.Minute,
		"Take a baseline capture this often, for heap diffs over time. 0 disables.")
	fs.DurationVar(&c.Cooldown, "cooldown", 2*time.Minute, "Minimum time between two triggered captures.")
	fs.StringVar(&c.OutDir, "out-dir", "/captures", "Directory captures are written to.")
	fs.IntVar(&c.MaxCaptures, "max-captures", 30, "Maximum number of capture directories to keep.")
	fs.StringVar(&maxBytes, "max-bytes", "1Gi", "Maximum total size of the capture directory.")
	fs.DurationVar(&c.FetchTimeout, "fetch-timeout", 10*time.Second, "Timeout for each small debug endpoint.")
	fs.DurationVar(&c.ProfileTimeout, "profile-timeout", 30*time.Second,
		"Timeout for the heap profile and goroutine dump.")
	fs.StringVar(&c.Listen, "listen", ":9095", "Address serving /metrics, /healthz and the capture downloads.")
	fs.BoolVar(&c.ServeCaptures, "serve-captures", true,
		"Serve /captures (a listing) and /captures.tar.gz (a download) on --listen. The memwatch image has no "+
			"tar, so `kubectl cp` can't copy captures out of it.")

	var envErr error
	fs.VisitAll(func(f *flag.Flag) {
		name := "MEMWATCH_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if v := getenv(name); v != "" && envErr == nil {
			if err := fs.Set(f.Name, v); err != nil {
				envErr = fmt.Errorf("%s: %w", name, err)
			}
		}
	})
	if envErr != nil {
		return nil, envErr
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	var err error
	switch c.Mode {
	case "auto", "proc", "http":
	default:
		return nil, fmt.Errorf("--mode must be auto, proc or http, got %q", c.Mode)
	}
	c.AlphaURL = strings.TrimRight(c.AlphaURL, "/")
	for _, w := range strings.Split(procMatch, ",") {
		if w = strings.TrimSpace(w); w != "" {
			c.ProcMatch = append(c.ProcMatch, w)
		}
	}
	if len(c.ProcMatch) == 0 {
		return nil, fmt.Errorf("--proc-match must not be empty")
	}
	if limit != "" {
		if c.LimitBytes, err = parseBytes(limit); err != nil {
			return nil, fmt.Errorf("--limit-bytes: %w", err)
		}
	}
	if c.Thresholds, err = parseThresholds(thresholds); err != nil {
		return nil, fmt.Errorf("--thresholds: %w", err)
	}
	switch {
	case rise != "auto":
		if c.RiseBytes, err = parseBytes(rise); err != nil {
			return nil, fmt.Errorf("--rise-bytes: %w", err)
		}
	case c.LimitBytes > 0:
		c.RiseBytes = c.LimitBytes / 5
	default:
		c.RiseBytes = 256 << 20
	}
	if c.MaxBytes, err = parseBytes(maxBytes); err != nil {
		return nil, fmt.Errorf("--max-bytes: %w", err)
	}
	if c.Interval <= 0 || c.HTTPInterval <= 0 {
		return nil, fmt.Errorf("--interval and --http-interval must be positive")
	}
	if c.RingSize < 2 {
		return nil, fmt.Errorf("--ring-size must be at least 2")
	}
	if c.MaxCaptures < 1 {
		return nil, fmt.Errorf("--max-captures must be at least 1")
	}
	return &c, nil
}

var byteSuffixes = []struct {
	suffix string
	mult   uint64
}{
	// Longest suffixes first so "Mi" is not mistaken for "M".
	{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40},
	{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40},
	{"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12},
	{"K", 1e3}, {"M", 1e6}, {"G", 1e9}, {"T", 1e12},
	{"B", 1},
}

// parseBytes accepts a plain byte count or a number with a binary (Ki, Mi, Gi) or decimal (K, M, G) suffix, which
// covers both Kubernetes quantities and the downward API's plain integers.
func parseBytes(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	mult := uint64(1)
	for _, bs := range byteSuffixes {
		if strings.HasSuffix(s, bs.suffix) {
			s, mult = strings.TrimSpace(strings.TrimSuffix(s, bs.suffix)), bs.mult
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("invalid byte size %q", s)
	}
	return uint64(f * float64(mult)), nil
}

func parseThresholds(s string) ([]float64, error) {
	var out []float64
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		f, err := strconv.ParseFloat(part, 64)
		if err != nil || f <= 0 || f > 1 {
			return nil, fmt.Errorf("threshold %q must be a fraction in (0, 1]", part)
		}
		out = append(out, f)
	}
	sort.Float64s(out)
	return out, nil
}

func defaultConfig() *Config {
	c, err := parseConfig(nil, func(string) string { return "" })
	if err != nil {
		panic(err)
	}
	return c
}
