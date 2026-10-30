/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseStatus(t *testing.T) {
	data, err := os.ReadFile("testdata/proc/101/status")
	if err != nil {
		t.Fatal(err)
	}
	s, err := parseStatus(data)
	if err != nil {
		t.Fatal(err)
	}
	want := Sample{Total: 524288 << 10, Anon: 409600 << 10, File: 112640 << 10, Shmem: 2048 << 10, HWM: 819200 << 10}
	if s != want {
		t.Fatalf("got %+v, want %+v", s, want)
	}
	if s.Usage() != s.Anon {
		t.Errorf("usage should prefer anon")
	}
	if _, err := parseStatus([]byte("Name:\tkthreadd\n")); err == nil {
		t.Error("status without VmRSS should fail")
	}
}

func TestParseStartTime(t *testing.T) {
	got, err := parseStartTime([]byte("9 (we(ird) name) S 1 9 9 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 4242 0 0\n"))
	if err != nil || got != "4242" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := parseStartTime([]byte("garbage")); err == nil {
		t.Error("expected an error")
	}
}

func TestMatchCmdline(t *testing.T) {
	match := []string{"dgraph", "alpha"}
	cases := map[string]bool{
		"/usr/local/bin/dgraph alpha --my=a:7080": true,
		"dgraph alpha":            true,
		"dgraph zero --my=z:5080": false,
		"/bin/sh -c dgraph alpha": false,
		"dgraph-alpha":            false,
		"":                        false,
	}
	for cmd, want := range cases {
		if got := matchCmdline(strings.Fields(cmd), match); got != want {
			t.Errorf("%q: got %v, want %v", cmd, got, want)
		}
	}
}

func TestProcSampler(t *testing.T) {
	ps := newProcSampler("testdata/proc", []string{"dgraph", "alpha"})
	s, err := ps.Sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.PID != 101 || s.Incarnation != "101-7765432" || s.Total != 524288<<10 || s.Time.IsZero() {
		t.Fatalf("unexpected sample %+v", s)
	}
}

func TestProcSamplerProcessGoneAndBack(t *testing.T) {
	root := t.TempDir()
	copyProc(t, "testdata/proc/101", filepath.Join(root, "101"))
	ps := newProcSampler(root, []string{"dgraph", "alpha"})
	first, err := ps.Sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(root, "101")); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Sample(context.Background()); !errors.Is(err, errNotFound) {
		t.Fatalf("got %v, want errNotFound", err)
	}

	// The restarted alpha gets a new PID.
	copyProc(t, "testdata/proc/101", filepath.Join(root, "555"))
	second, err := ps.Sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.PID != 555 || second.Incarnation == first.Incarnation {
		t.Fatalf("expected a new incarnation, got %+v after %+v", second, first)
	}
}

// TestProcSamplerSkipsPanicwrapParent reproduces Dgraph v24, where a small parent process re-executes the binary.
// The parent's PID sorts first in directory order, so naive discovery would pick it.
func TestProcSamplerSkipsPanicwrapParent(t *testing.T) {
	root := t.TempDir()
	writeProc(t, root, 12, "dgraph\x00alpha\x00--my=a:7080\x00", 0, 60000)
	writeProc(t, root, 7, "/usr/local/bin/dgraph\x00alpha\x00--my=a:7080\x00", 12, 540000)
	// An unrelated alpha-looking process with no parent link and less memory loses on RSS.
	writeProc(t, root, 3, "dgraph\x00alpha\x00--my=b:7080\x00", 0, 1000)
	ps := newProcSampler(root, []string{"dgraph", "alpha"})
	s, err := ps.Sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.PID != 7 || s.Total != 540000<<10 {
		t.Fatalf("picked pid %d (rss %d), want the panicwrap child 7", s.PID, s.Total)
	}
}

func writeProc(t *testing.T, root string, pid int, cmdline string, ppid int, rssKB uint64) {
	t.Helper()
	dir := filepath.Join(root, fmt.Sprint(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stat := fmt.Sprintf("%d (dgraph) S %d 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 %d 0 0\n", pid, ppid, 1000+pid)
	status := fmt.Sprintf("Name:\tdgraph\nPPid:\t%d\nVmHWM:\t%d kB\nVmRSS:\t%d kB\nRssAnon:\t%d kB\n",
		ppid, rssKB, rssKB, rssKB)
	for name, data := range map[string]string{"cmdline": cmdline, "stat": stat, "status": status} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A slow alpha, or memwatch shutting down, must not look like the alpha going away: that would write a false restart
// dump and reset the timeline right when memory is under pressure.
func TestHTTPSamplerTimeoutIsNotRestart(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(block)

	hs := newHTTPSampler(srv.URL, 100*time.Millisecond)
	if _, err := hs.Sample(context.Background()); err == nil || errors.Is(err, errNotFound) {
		t.Fatalf("timeout: got %v, want a plain error", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hs.Sample(ctx); err == nil || errors.Is(err, errNotFound) {
		t.Fatalf("cancelled: got %v, want a plain error", err)
	}
}

func copyProc(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"cmdline", "stat", "status"} {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, f), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHTTPSampler(t *testing.T) {
	start := 1.7e9
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug/prometheus_metrics" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "# HELP process_resident_memory_bytes Resident memory size in bytes.\n"+
			"# TYPE process_resident_memory_bytes gauge\n"+
			"process_resident_memory_bytes 5.24288e+08\n"+
			"process_start_time_seconds %g\n"+
			"dgraph_memory_inuse_bytes 1.7e+08\n"+
			"go_memstats_alloc_bytes{le=\"x\"} 12\n", start)
	}))
	defer srv.Close()

	hs := newHTTPSampler(srv.URL, defaultConfig().FetchTimeout)
	s, err := hs.Sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 524288000 || s.Incarnation != "1700000000" || s.Anon != 0 {
		t.Fatalf("unexpected sample %+v", s)
	}

	srv.Close()
	if _, err := hs.Sample(context.Background()); !errors.Is(err, errNotFound) {
		t.Fatalf("unreachable alpha: got %v, want errNotFound", err)
	}
}
