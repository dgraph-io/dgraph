/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Sample is one reading of the alpha's memory. Anon, File, Shmem and HWM are only known in proc mode and are zero
// in http mode.
type Sample struct {
	Time time.Time
	// Incarnation identifies one run of the alpha process. It changes when the process restarts.
	Incarnation string
	PID         int
	Total       uint64 // resident set size
	Anon        uint64 // anonymous resident memory: Go heap, jemalloc, stacks
	File        uint64 // file-backed resident memory, mostly mmapped Badger files
	Shmem       uint64
	HWM         uint64 // peak resident set size since the process started
}

// Usage is the figure compared against the memory limit. Anonymous memory is what the kernel cannot reclaim, so it
// is used when known. Otherwise it falls back to total RSS.
func (s Sample) Usage() uint64 {
	if s.Anon > 0 {
		return s.Anon
	}
	return s.Total
}

// errNotFound means no alpha process is running right now, for example while its container restarts.
var errNotFound = errors.New("alpha process not found")

// Sampler reads the alpha's current memory usage.
type Sampler interface {
	Sample(ctx context.Context) (Sample, error)
	// Mode is "proc" or "http".
	Mode() string
}

// procSampler reads /proc/<pid>/status of the alpha process. It needs to share a PID namespace with the alpha,
// for example `shareProcessNamespace: true` in Kubernetes or `pid: "service:alpha"` in Docker Compose.
type procSampler struct {
	root  string
	match []string
	self  int
	pid   int
	now   func() time.Time
}

func newProcSampler(root string, match []string) *procSampler {
	return &procSampler{root: root, match: match, self: os.Getpid(), now: time.Now}
}

func (p *procSampler) Mode() string { return "proc" }

func (p *procSampler) Sample(_ context.Context) (Sample, error) {
	if p.pid != 0 {
		if s, err := p.read(p.pid); err == nil {
			return s, nil
		}
		p.pid = 0
	}
	pid, err := p.find()
	if err != nil {
		return Sample{}, err
	}
	s, err := p.read(pid)
	if err != nil {
		return Sample{}, err
	}
	p.pid = pid
	return s, nil
}

// find scans procfs for the alpha process. Some builds run as two processes with the same command line: Dgraph
// v24 wraps itself with panicwrap, so a small parent re-executes the binary and only watches the child for panics.
// The child does the work, so a match whose own child also matches is skipped. If several candidates remain, the
// one using the most memory wins.
func (p *procSampler) find() (int, error) {
	entries, err := os.ReadDir(p.root)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		pid, ppid int
		rss       uint64
	}
	var cands []candidate
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == p.self {
			continue
		}
		dir := filepath.Join(p.root, e.Name())
		cmdline, err := os.ReadFile(filepath.Join(dir, "cmdline"))
		if err != nil || len(cmdline) == 0 {
			continue
		}
		if !matchCmdline(strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00"), p.match) {
			continue
		}
		status, err := os.ReadFile(filepath.Join(dir, "status"))
		if err != nil {
			continue
		}
		ppidField, _ := statusField(status, "PPid")
		ppid, _ := strconv.Atoi(ppidField)
		rss, _ := statusValue(status, "VmRSS")
		cands = append(cands, candidate{pid: pid, ppid: ppid, rss: rss})
	}
	isParent := make(map[int]bool, len(cands))
	for _, c := range cands {
		isParent[c.ppid] = true
	}
	best := candidate{}
	for _, c := range cands {
		if isParent[c.pid] {
			continue
		}
		if best.pid == 0 || c.rss > best.rss || (c.rss == best.rss && c.pid < best.pid) {
			best = c
		}
	}
	if best.pid == 0 {
		return 0, errNotFound
	}
	return best.pid, nil
}

// statusValue returns the first number on the given line of /proc/<pid>/status.
func statusValue(status []byte, key string) (uint64, bool) {
	field, ok := statusField(status, key)
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseUint(field, 10, 64)
	return v, err == nil
}

// statusField returns the first whitespace-separated field on the given line of /proc/<pid>/status.
func statusField(status []byte, key string) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(status))
	for sc.Scan() {
		k, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok || k != key {
			continue
		}
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			return "", false
		}
		return parts[0], true
	}
	return "", false
}

// matchCmdline reports whether argv belongs to the process we're looking for. The first match word must equal the
// basename of argv[0], and every other word must equal one of the remaining arguments.
func matchCmdline(argv, match []string) bool {
	if len(argv) == 0 || len(match) == 0 || filepath.Base(argv[0]) != match[0] {
		return false
	}
	for _, want := range match[1:] {
		found := false
		for _, arg := range argv[1:] {
			if arg == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (p *procSampler) read(pid int) (Sample, error) {
	dir := filepath.Join(p.root, strconv.Itoa(pid))
	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return Sample{}, err
	}
	start, err := parseStartTime(stat)
	if err != nil {
		return Sample{}, err
	}
	status, err := os.ReadFile(filepath.Join(dir, "status"))
	if err != nil {
		return Sample{}, err
	}
	s, err := parseStatus(status)
	if err != nil {
		return Sample{}, err
	}
	s.Time = p.now()
	s.PID = pid
	s.Incarnation = fmt.Sprintf("%d-%s", pid, start)
	return s, nil
}

// parseStartTime returns field 22 of /proc/<pid>/stat, the process start time in clock ticks since boot. The
// command name in field 2 can contain spaces and parentheses, so fields are counted from the last ')'.
func parseStartTime(stat []byte) (string, error) {
	i := bytes.LastIndexByte(stat, ')')
	if i < 0 {
		return "", fmt.Errorf("malformed stat")
	}
	fields := strings.Fields(string(stat[i+1:]))
	// fields[0] is field 3 (state), so field 22 is fields[19].
	if len(fields) < 20 {
		return "", fmt.Errorf("malformed stat: %d fields", len(fields))
	}
	return fields[19], nil
}

// parseStatus extracts memory figures from /proc/<pid>/status. All of them are reported in kB.
func parseStatus(status []byte) (Sample, error) {
	var s Sample
	fields := map[string]*uint64{
		"VmRSS": &s.Total, "RssAnon": &s.Anon, "RssFile": &s.File, "RssShmem": &s.Shmem, "VmHWM": &s.HWM,
	}
	sc := bufio.NewScanner(bytes.NewReader(status))
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		dst, ok := fields[key]
		if !ok {
			continue
		}
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			continue
		}
		kb, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return Sample{}, fmt.Errorf("parsing %s: %w", key, err)
		}
		*dst = kb << 10
	}
	if s.Total == 0 {
		// Kernel threads and zombies have no VmRSS line.
		return Sample{}, fmt.Errorf("no VmRSS in status")
	}
	return s, nil
}

// httpSampler scrapes the alpha's Prometheus endpoint. It works without a shared PID namespace, at the cost of a
// coarser interval and only total RSS.
type httpSampler struct {
	url    string
	client *http.Client
	now    func() time.Time
}

func newHTTPSampler(alphaURL string, timeout time.Duration) *httpSampler {
	return &httpSampler{
		url:    alphaURL + "/debug/prometheus_metrics",
		client: &http.Client{Timeout: timeout},
		now:    time.Now,
	}
}

func (h *httpSampler) Mode() string { return "http" }

func (h *httpSampler) Sample(ctx context.Context) (Sample, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url, nil)
	if err != nil {
		return Sample{}, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		// A timeout means the alpha is alive but slow, which is likely under memory pressure, and cancellation
		// means memwatch is shutting down. Neither is a restart, so neither should reset the timeline.
		var netErr net.Error
		if ctx.Err() != nil || (errors.As(err, &netErr) && netErr.Timeout()) {
			return Sample{}, fmt.Errorf("sampling %s: %w", h.url, err)
		}
		// Anything else, such as a refused connection, means nothing is listening: the alpha is down.
		return Sample{}, fmt.Errorf("%w: %v", errNotFound, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return Sample{}, fmt.Errorf("%s returned %s", h.url, resp.Status)
	}
	vals, err := scrapeValues(resp.Body, "process_resident_memory_bytes", "process_start_time_seconds")
	if err != nil {
		return Sample{}, err
	}
	rss, ok := vals["process_resident_memory_bytes"]
	if !ok {
		return Sample{}, fmt.Errorf("process_resident_memory_bytes missing from %s", h.url)
	}
	return Sample{
		Time:        h.now(),
		Incarnation: strconv.FormatFloat(vals["process_start_time_seconds"], 'f', -1, 64),
		Total:       uint64(rss),
	}, nil
}

// scrapeValues reads unlabeled samples for the given metric names from Prometheus text exposition format.
func scrapeValues(r io.Reader, names ...string) (map[string]float64, error) {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	out := make(map[string]float64, len(names))
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 || !want[parts[0]] {
			continue
		}
		v, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", parts[0], err)
		}
		out[parts[0]] = v
	}
	return out, sc.Err()
}
