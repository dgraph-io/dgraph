/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// dirTimeFormat sorts lexically in time order and has no colons, so the names are safe on every filesystem, in tar
// archives, and in URLs.
const dirTimeFormat = "20060102T150405.000Z"

const restartSuffix = "-restart"

// endpoint is one alpha debug URL saved into every capture. All of them exist in both v24 and v25.
type endpoint struct {
	file    string
	path    string
	profile bool // uses the longer profile timeout
}

var endpoints = []endpoint{
	// gc=1 runs a garbage collection first. Without it the in-use figures describe the heap as of the last GC,
	// which during a spike can be seconds stale and miss the allocation entirely.
	{file: "heap.pb.gz", path: "/debug/pprof/heap?gc=1", profile: true},
	{file: "goroutine.txt", path: "/debug/pprof/goroutine?debug=2", profile: true},
	{file: "jemalloc.txt", path: "/debug/jemalloc"},
	{file: "metrics.txt", path: "/debug/prometheus_metrics"},
	{file: "health.json", path: "/health"},
}

// Meta is written to meta.json in every capture directory.
type Meta struct {
	Reason       string            `json:"reason"`
	Time         time.Time         `json:"time"`
	Mode         string            `json:"mode"`
	AlphaVersion string            `json:"alpha_version,omitempty"`
	PID          int               `json:"pid,omitempty"`
	LimitBytes   uint64            `json:"limit_bytes,omitempty"`
	Sample       Sample            `json:"sample"`
	Errors       map[string]string `json:"errors,omitempty"`
}

// Capturer writes capture directories and enforces retention.
type Capturer struct {
	cfg    *Config
	mode   string
	client *http.Client
	// onError is called once for every artifact that could not be saved.
	onError func()
}

func NewCapturer(cfg *Config, mode string, onError func()) *Capturer {
	if onError == nil {
		onError = func() {}
	}
	return &Capturer{cfg: cfg, mode: mode, client: &http.Client{}, onError: onError}
}

// Capture saves the alpha's profiles and debug output, the proc files of the process, and the recent timeline. It
// returns the directory written. Each artifact is best-effort; failures are listed in meta.json.
func (c *Capturer) Capture(ctx context.Context, reason string, cur Sample, timeline []Sample) (string, error) {
	dir, err := c.mkdir(cur.Time, reason)
	if err != nil {
		return "", err
	}
	meta := Meta{Reason: reason, Time: cur.Time.UTC(), Mode: c.mode, PID: cur.PID, LimitBytes: c.cfg.LimitBytes,
		Sample: cur, Errors: map[string]string{}}

	var mu sync.Mutex
	fail := func(name string, err error) {
		mu.Lock()
		meta.Errors[name] = err.Error()
		mu.Unlock()
		c.onError()
	}

	var wg sync.WaitGroup
	for _, ep := range endpoints {
		wg.Add(1)
		go func(ep endpoint) {
			defer wg.Done()
			timeout := c.cfg.FetchTimeout
			if ep.profile {
				timeout = c.cfg.ProfileTimeout
			}
			if err := c.fetch(ctx, ep.path, filepath.Join(dir, ep.file), timeout); err != nil {
				fail(ep.file, err)
			}
		}(ep)
	}
	wg.Wait()

	if cur.PID != 0 {
		procDir := filepath.Join(c.cfg.ProcRoot, strconv.Itoa(cur.PID))
		if err := copyFile(filepath.Join(procDir, "status"), filepath.Join(dir, "proc-status.txt")); err != nil {
			fail("proc-status.txt", err)
		}
		// smaps_rollup needs ptrace read access to the alpha, which a sidecar running as a different user doesn't
		// have. Its absence is expected, so it is not counted as an error.
		_ = copyFile(filepath.Join(procDir, "smaps_rollup"), filepath.Join(dir, "proc-smaps_rollup.txt"))
	}
	if err := writeTimeline(filepath.Join(dir, "timeline.csv"), timeline); err != nil {
		fail("timeline.csv", err)
	}
	meta.AlphaVersion = readVersion(filepath.Join(dir, "health.json"))
	if len(meta.Errors) == 0 {
		meta.Errors = nil
	}
	if err := writeJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
		return dir, err
	}
	return dir, c.Prune()
}

// RestartDump records the timeline of an alpha incarnation that just ended. When the alpha was OOM-killed, this is
// the memory trajectory leading into the kill.
func (c *Capturer) RestartDump(last Sample, timeline []Sample, now time.Time) (string, error) {
	dir, err := c.mkdir(now, strings.TrimPrefix(restartSuffix, "-"))
	if err != nil {
		return "", err
	}
	meta := Meta{Reason: "restart", Time: now.UTC(), Mode: c.mode, PID: last.PID, LimitBytes: c.cfg.LimitBytes,
		Sample: last}
	if err := writeTimeline(filepath.Join(dir, "timeline.csv"), timeline); err != nil {
		return dir, err
	}
	if err := writeJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
		return dir, err
	}
	return dir, c.Prune()
}

func (c *Capturer) mkdir(t time.Time, reason string) (string, error) {
	base := filepath.Join(c.cfg.OutDir, t.UTC().Format(dirTimeFormat)+"-"+reason)
	dir := base
	for i := 2; ; i++ {
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			return dir, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
		dir = fmt.Sprintf("%s-%d", base, i)
	}
}

func (c *Capturer) fetch(ctx context.Context, path, dst string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.AlphaURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(dst)
		return err
	}
	return f.Close()
}

type captureDir struct {
	path    string
	size    uint64
	restart bool
}

// Prune deletes the oldest capture directories until at most MaxCaptures remain and their total size is within
// MaxBytes. Ordinary captures go first; restart dumps are only deleted once none are left, because they record
// what the cluster looked like right before a crash.
func (c *Capturer) Prune() error {
	entries, err := os.ReadDir(c.cfg.OutDir)
	if err != nil {
		return err
	}
	var dirs []captureDir
	var total uint64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(c.cfg.OutDir, e.Name())
		size, err := dirSize(p)
		if err != nil {
			return err
		}
		dirs = append(dirs, captureDir{path: p, size: size, restart: isRestartDir(e.Name())})
		total += size
	}
	// Names start with a sortable timestamp, so name order is time order.
	sort.Slice(dirs, func(i, j int) bool { return filepath.Base(dirs[i].path) < filepath.Base(dirs[j].path) })

	over := func() bool { return len(dirs) > c.cfg.MaxCaptures || total > c.cfg.MaxBytes }
	for _, restartPass := range []bool{false, true} {
		for i := 0; i < len(dirs) && over(); {
			// Never delete the newest directory: it is the capture that was just written.
			if dirs[i].restart != restartPass || i == len(dirs)-1 {
				i++
				continue
			}
			if err := os.RemoveAll(dirs[i].path); err != nil {
				return err
			}
			total -= dirs[i].size
			dirs = append(dirs[:i], dirs[i+1:]...)
		}
	}
	return nil
}

func isRestartDir(name string) bool {
	if strings.HasSuffix(name, restartSuffix) {
		return true
	}
	// A name collision adds a "-N" suffix after the reason.
	if i := strings.LastIndexByte(name, '-'); i > 0 {
		if _, err := strconv.Atoi(name[i+1:]); err == nil {
			return strings.HasSuffix(name[:i], restartSuffix)
		}
	}
	return false
}

// dirSize sums the sizes of the files under dir. Files can disappear while it walks, because a capture running
// concurrently removes a partial artifact when a fetch fails, so missing files are skipped rather than failing the
// whole prune.
func dirSize(dir string) (uint64, error) {
	var size uint64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			size += uint64(info.Size())
		}
		return nil
	})
	return size, err
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func writeTimeline(path string, samples []Sample) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := WriteTimelineCSV(f, samples); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// readVersion pulls the version out of a saved /health response, which in v24 and v25 is a JSON array of objects
// with a "version" field.
func readVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var health []struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &health); err != nil || len(health) == 0 {
		return ""
	}
	return health[0].Version
}
