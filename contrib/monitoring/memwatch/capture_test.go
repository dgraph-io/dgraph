/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAlpha serves the debug endpoints memwatch reads. hang makes one path block until the request is cancelled.
func fakeAlpha(t *testing.T, hang string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == hang {
			<-r.Context().Done()
			return
		}
		switch r.URL.Path {
		case "/debug/pprof/heap":
			if r.URL.Query().Get("gc") != "1" {
				http.Error(w, "want gc=1", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("\x1f\x8bfake-heap"))
		case "/debug/pprof/goroutine":
			if r.URL.Query().Get("debug") != "2" {
				http.Error(w, "want debug=2", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("goroutine 1 [running]:\nmain.main()\n"))
		case "/debug/jemalloc":
			_, _ = w.Write([]byte("Num Allocated Bytes: 43 MiB [45088768]\n"))
		case "/debug/prometheus_metrics":
			_, _ = w.Write([]byte("process_resident_memory_bytes 5.24288e+08\n"))
		case "/health":
			_, _ = w.Write([]byte(`[{"instance":"alpha","address":"alpha-0:7080","version":"v24.0.5"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testCapturer(t *testing.T, alphaURL string, args ...string) (*Capturer, *Config, *atomic.Int64) {
	t.Helper()
	all := append([]string{"--alpha-url=" + alphaURL, "--out-dir=" + t.TempDir(), "--proc-root=testdata/proc",
		"--limit-bytes=1Gi"}, args...)
	cfg, err := parseConfig(all, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	var errs atomic.Int64
	return NewCapturer(cfg, "proc", func() { errs.Add(1) }), cfg, &errs
}

func readMeta(t *testing.T, dir string) Meta {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCaptureWritesAllArtifacts(t *testing.T) {
	c, _, errs := testCapturer(t, fakeAlpha(t, "").URL)
	cur := at(10, 900<<20)
	cur.PID = 101
	dir, err := c.Capture(context.Background(), "threshold-85", cur, []Sample{at(9, 800<<20), cur})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dir, "20261028T180010.000Z-threshold-85") {
		t.Errorf("dir name = %s", dir)
	}
	for _, f := range []string{"heap.pb.gz", "goroutine.txt", "jemalloc.txt", "metrics.txt", "health.json",
		"proc-status.txt", "proc-smaps_rollup.txt", "timeline.csv", "meta.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	m := readMeta(t, dir)
	if m.Reason != "threshold-85" || m.AlphaVersion != "v24.0.5" || m.PID != 101 || m.LimitBytes != 1<<30 ||
		len(m.Errors) != 0 {
		t.Errorf("meta = %+v", m)
	}
	if errs.Load() != 0 {
		t.Errorf("errors counted: %d", errs.Load())
	}
}

func TestCaptureSurvivesHungAndMissingEndpoints(t *testing.T) {
	c, _, errs := testCapturer(t, fakeAlpha(t, "/debug/pprof/heap").URL, "--profile-timeout=200ms")
	cur := at(0, 1)
	cur.PID = 999 // no such process in testdata/proc
	start := time.Now()
	dir, err := c.Capture(context.Background(), "rise", cur, nil)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("capture did not honor the profile timeout")
	}
	m := readMeta(t, dir)
	if _, ok := m.Errors["heap.pb.gz"]; !ok {
		t.Errorf("hung heap fetch not reported: %+v", m.Errors)
	}
	if _, ok := m.Errors["proc-status.txt"]; !ok {
		t.Errorf("missing proc status not reported: %+v", m.Errors)
	}
	if _, err := os.Stat(filepath.Join(dir, "heap.pb.gz")); !os.IsNotExist(err) {
		t.Errorf("partial heap profile left behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "goroutine.txt")); err != nil {
		t.Errorf("other endpoints should still be saved: %v", err)
	}
	if errs.Load() != 2 {
		t.Errorf("error count = %d, want 2", errs.Load())
	}
}

func TestCaptureWithAlphaDown(t *testing.T) {
	srv := fakeAlpha(t, "")
	srv.Close()
	c, _, _ := testCapturer(t, srv.URL)
	cur := at(0, 1)
	cur.PID = 0 // http mode: no proc files
	dir, err := c.Capture(context.Background(), "rise", cur, []Sample{cur})
	if err != nil {
		t.Fatal(err)
	}
	if m := readMeta(t, dir); len(m.Errors) != len(endpoints) {
		t.Errorf("errors = %+v", m.Errors)
	}
	if _, err := os.Stat(filepath.Join(dir, "timeline.csv")); err != nil {
		t.Errorf("timeline should be written even with the alpha down: %v", err)
	}
}

func TestRestartDump(t *testing.T) {
	c, _, _ := testCapturer(t, "http://unused")
	last := at(30, 1900<<20)
	dir, err := c.RestartDump(last, []Sample{at(29, 900<<20), last}, t0.Add(31*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !isRestartDir(filepath.Base(dir)) {
		t.Errorf("not recognized as a restart dump: %s", dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "timeline.csv"))
	if err != nil || strings.Count(string(data), "\n") != 3 {
		t.Errorf("timeline = %q, %v", data, err)
	}
	if m := readMeta(t, dir); m.Reason != "restart" || m.Sample.Total != 1900<<20 {
		t.Errorf("meta = %+v", m)
	}
}

func TestPruneKeepsRestartDumps(t *testing.T) {
	c, cfg, _ := testCapturer(t, "http://unused", "--max-captures=3")
	mk := func(name string) {
		if err := os.MkdirAll(filepath.Join(cfg.OutDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg.OutDir, name, "f"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{
		"20261028T180000.000Z-restart",
		"20261028T180001.000Z-periodic",
		"20261028T180002.000Z-restart-2",
		"20261028T180003.000Z-rise",
		"20261028T180004.000Z-threshold-70",
	} {
		mk(n)
	}
	if err := c.Prune(); err != nil {
		t.Fatal(err)
	}
	if got := listDirs(t, cfg.OutDir); strings.Join(got, ",") !=
		"20261028T180000.000Z-restart,20261028T180002.000Z-restart-2,20261028T180004.000Z-threshold-70" {
		t.Fatalf("kept %v", got)
	}

	// Once only restart dumps are left to evict, the oldest of them go.
	cfg.MaxCaptures = 1
	if err := c.Prune(); err != nil {
		t.Fatal(err)
	}
	if got := listDirs(t, cfg.OutDir); len(got) != 1 || got[0] != "20261028T180004.000Z-threshold-70" {
		t.Fatalf("kept %v", got)
	}
}

func TestPruneByBytes(t *testing.T) {
	c, cfg, _ := testCapturer(t, "http://unused", "--max-bytes=250")
	for _, n := range []string{"20261028T180000.000Z-a", "20261028T180001.000Z-b", "20261028T180002.000Z-c"} {
		if err := os.MkdirAll(filepath.Join(cfg.OutDir, n), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg.OutDir, n, "f"), make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Prune(); err != nil {
		t.Fatal(err)
	}
	if got := listDirs(t, cfg.OutDir); len(got) != 2 || got[0] != "20261028T180001.000Z-b" {
		t.Fatalf("kept %v", got)
	}
}

func listDirs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func TestDirSizeToleratesVanishedDir(t *testing.T) {
	size, err := dirSize(filepath.Join(t.TempDir(), "pruned-meanwhile"))
	if err != nil || size != 0 {
		t.Fatalf("dirSize of a missing dir = %d, %v; want 0, nil", size, err)
	}
}

func TestIsRestartDir(t *testing.T) {
	for name, want := range map[string]bool{
		"20261028T180000.000Z-restart":      true,
		"20261028T180000.000Z-restart-3":    true,
		"20261028T180000.000Z-threshold-70": false,
		"20261028T180000.000Z-rise-2":       false,
	} {
		if got := isRestartDir(name); got != want {
			t.Errorf("%s: got %v", name, got)
		}
	}
}

// scriptedSampler returns a fixed sequence of samples and errors.
type scriptedSampler struct {
	steps []func() (Sample, error)
	i     int
}

func (s *scriptedSampler) Mode() string { return "proc" }

func (s *scriptedSampler) Sample(context.Context) (Sample, error) {
	f := s.steps[s.i]
	s.i++
	return f()
}

func TestWatcherRestartDetection(t *testing.T) {
	c, cfg, _ := testCapturer(t, fakeAlpha(t, "").URL, "--periodic=0", "--rise-bytes=0")
	ok := func(sec int, inc string, anon uint64) func() (Sample, error) {
		return func() (Sample, error) {
			s := at(sec, anon+1)
			s.Incarnation, s.Anon = inc, anon
			return s, nil
		}
	}
	gone := func() (Sample, error) { return Sample{}, errNotFound }
	w := &watcher{
		cfg: cfg, log: slog.New(slog.NewTextHandler(io.Discard, nil)), metrics: NewMetrics(cfg.Thresholds),
		sampler: &scriptedSampler{steps: []func() (Sample, error){
			ok(0, "a", 100<<20), ok(1, "a", 200<<20), // first incarnation
			gone, gone, // OOM-killed, container restarting
			ok(5, "b", 50<<20),  // back with a new incarnation
			ok(6, "c", 60<<20),  // replaced again without a gap
			ok(7, "c", 990<<20), // crosses 95% of 1Gi
		}},
		ring: NewRing(cfg.RingSize), trigger: NewTrigger(cfg, t0), capturer: c,
	}
	for range 7 {
		w.tick(context.Background())
	}
	w.wg.Wait()

	var restarts, thresholds int
	for _, name := range listDirs(t, cfg.OutDir) {
		switch {
		case isRestartDir(name):
			restarts++
		case strings.HasSuffix(name, "-threshold-95"):
			thresholds++
		}
	}
	if restarts != 2 || thresholds != 1 {
		t.Fatalf("restart dumps = %d, threshold captures = %d; dirs %v", restarts, thresholds,
			listDirs(t, cfg.OutDir))
	}
	if n := w.ring.Len(); n != 2 {
		t.Errorf("ring should only hold the current incarnation, has %d", n)
	}
}
