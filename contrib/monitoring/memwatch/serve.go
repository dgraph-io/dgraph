/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The memwatch image has no shell or tar, so `kubectl cp` can't copy captures out of it. Instead memwatch serves
// them on its own port:
//
//	GET /captures                      one line per capture: name, size in bytes, modification time
//	GET /captures.tar.gz               every capture as a gzipped tar
//	GET /captures.tar.gz?name=<dir>    a single capture
func registerCaptureHandlers(mux *http.ServeMux, outDir string) {
	mux.HandleFunc("GET /captures", func(w http.ResponseWriter, _ *http.Request) {
		dirs, err := listCaptures(outDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, d := range dirs {
			fmt.Fprintf(w, "%s\t%d\t%s\n", d.name, d.size, d.mod.UTC().Format(time.RFC3339))
		}
	})
	mux.HandleFunc("GET /captures.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		names, err := selectCaptures(outDir, r.URL.Query().Get("name"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		file := "memwatch-captures.tar.gz"
		if len(names) == 1 && r.URL.Query().Get("name") != "" {
			file = names[0] + ".tar.gz"
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file))
		// Headers are already sent once streaming starts, so a failure part way through can only truncate the
		// archive, which gzip and tar both detect on extraction.
		_ = writeTarGz(w, outDir, names)
	})
}

type captureInfo struct {
	name string
	size uint64
	mod  time.Time
}

func listCaptures(outDir string) ([]captureInfo, error) {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return nil, err
	}
	var out []captureInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		size, err := dirSize(filepath.Join(outDir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, captureInfo{name: e.Name(), size: size, mod: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// selectCaptures returns the capture directories to archive: all of them, or the one named. A name must be an
// existing directory directly inside outDir, so a request can't reach anything else on the filesystem.
func selectCaptures(outDir, name string) ([]string, error) {
	if name != "" {
		if name != filepath.Base(name) || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			return nil, fmt.Errorf("invalid capture name %q", name)
		}
		if fi, err := os.Stat(filepath.Join(outDir, name)); err != nil || !fi.IsDir() {
			return nil, fmt.Errorf("no capture named %q", name)
		}
		return []string{name}, nil
	}
	dirs, err := listCaptures(outDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(dirs))
	for i, d := range dirs {
		names[i] = d.name
	}
	return names, nil
}

// writeTarGz writes the named capture directories, with paths relative to outDir.
func writeTarGz(w io.Writer, outDir string, names []string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		root := filepath.Join(outDir, name)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// A capture being pruned or written concurrently; skip what disappeared.
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if !d.IsDir() && !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, err := filepath.Rel(outDir, path)
			if err != nil {
				return err
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = filepath.ToSlash(rel)
			if d.IsDir() {
				hdr.Name += "/"
				return tw.WriteHeader(hdr)
			}
			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			_, err = io.CopyN(tw, f, hdr.Size)
			return err
		})
		if err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}
