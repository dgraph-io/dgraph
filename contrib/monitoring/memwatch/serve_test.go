/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func captureServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	out := t.TempDir()
	for _, f := range []string{
		"20261028T180000.000Z-threshold-85/heap.pb.gz",
		"20261028T180000.000Z-threshold-85/meta.json",
		"20261028T180001.000Z-restart/timeline.csv",
	} {
		p := filepath.Join(out, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("data:"+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A stray file at the top level is not a capture.
	if err := os.WriteFile(filepath.Join(out, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerCaptureHandlers(mux, out)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, out
}

func get(t *testing.T, u string) *http.Response {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func untar(t *testing.T, r io.Reader) map[string]string {
	t.Helper()
	gz, err := gzip.NewReader(r)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	files := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			files[hdr.Name] = string(data)
		}
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestServeCaptureListing(t *testing.T) {
	srv, _ := captureServer(t)
	body, _ := io.ReadAll(get(t, srv.URL+"/captures").Body)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "20261028T180000.000Z-threshold-85\t") ||
		!strings.HasPrefix(lines[1], "20261028T180001.000Z-restart\t") {
		t.Fatalf("listing = %q", body)
	}
}

func TestServeAllCaptures(t *testing.T) {
	srv, _ := captureServer(t)
	resp := get(t, srv.URL+"/captures.tar.gz")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/gzip" {
		t.Fatalf("status %s, type %q", resp.Status, resp.Header.Get("Content-Type"))
	}
	files := untar(t, resp.Body)
	want := []string{
		"20261028T180000.000Z-threshold-85/heap.pb.gz",
		"20261028T180000.000Z-threshold-85/meta.json",
		"20261028T180001.000Z-restart/timeline.csv",
	}
	if got := keys(files); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("archive has %v, want %v", got, want)
	}
	if files[want[2]] != "data:"+want[2] {
		t.Errorf("content mismatch: %q", files[want[2]])
	}
}

func TestServeOneCapture(t *testing.T) {
	srv, _ := captureServer(t)
	resp := get(t, srv.URL+"/captures.tar.gz?name=20261028T180001.000Z-restart")
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "20261028T180001.000Z-restart.tar.gz") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if got := keys(untar(t, resp.Body)); len(got) != 1 || got[0] != "20261028T180001.000Z-restart/timeline.csv" {
		t.Fatalf("archive has %v", got)
	}
}

func TestServeRejectsBadNames(t *testing.T) {
	srv, _ := captureServer(t)
	for _, name := range []string{"../etc", "..", ".", "a/b", "notes.txt", "missing"} {
		resp := get(t, srv.URL+"/captures.tar.gz?name="+url.QueryEscape(name))
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("name %q: status %s, want 404", name, resp.Status)
		}
	}
}
