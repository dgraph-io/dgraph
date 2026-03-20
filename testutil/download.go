/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package testutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxRetries = 3

	benchmarkRepo = "dgraph-io/dgraph-benchmarks"

	// Environment variable to override the benchmark data ref at runtime.
	envDataRef = "DGRAPH_TEST_DATA_REF"

	// benchmarkDataVersionFile is the filename (co-located with this source file
	// in testutil/) that pins the git ref for benchmark data downloads.
	// Changing its contents invalidates CI caches (workflows key on
	// hashFiles('testutil/benchmark-data-version')).
	//
	// To override at runtime without editing the file:
	//   - Set the DGRAPH_TEST_DATA_REF environment variable, or
	//   - Pass --data-ref=<ref> to the t test runner.
	benchmarkDataVersionFile = "benchmark-data-version"
)

var (
	// lfsPointerPrefix is the first line of every Git LFS pointer file.
	lfsPointerPrefix = []byte("version https://git-lfs.github.com")

	// gzipMagic is the two-byte header for gzip files.
	gzipMagic = []byte{0x1f, 0x8b}
)

// testutilDir returns the directory containing this source file (testutil/).
func testutilDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Dir(thisFile)
}

// BenchmarkDataRef returns the git ref (branch, tag, or SHA) to use when
// downloading benchmark data from dgraph-benchmarks. Resolution order:
//  1. refOverride argument (non-empty string, e.g. from a --data-ref CLI flag)
//  2. DGRAPH_TEST_DATA_REF environment variable
//  3. Contents of testutil/benchmark-data-version file
//  4. Falls back to "main"
func BenchmarkDataRef(refOverride string) string {
	if refOverride != "" {
		return refOverride
	}
	if v := os.Getenv(envDataRef); v != "" {
		return v
	}
	versionFile := filepath.Join(testutilDir(), benchmarkDataVersionFile)
	if data, err := os.ReadFile(versionFile); err == nil {
		if ref := strings.TrimSpace(string(data)); ref != "" {
			return ref
		}
	}
	return "main"
}

// BenchmarkRawURL returns a raw.githubusercontent.com URL for a non-LFS file
// in the dgraph-benchmarks repo at the given ref.
func BenchmarkRawURL(ref, path string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", benchmarkRepo, ref, path)
}

// BenchmarkLFSURL returns a media.githubusercontent.com URL for an LFS-tracked
// file in the dgraph-benchmarks repo at the given ref.
func BenchmarkLFSURL(ref, path string) string {
	return fmt.Sprintf("https://media.githubusercontent.com/media/%s/%s/%s", benchmarkRepo, ref, path)
}

// DownloadFile downloads a file from url into dir/fname using wget with retry
// logic (3 Go-level attempts, exponential backoff). wget itself uses --tries=1
// so there is a single retry layer. On final failure, any partial file is
// removed to prevent corrupt data from persisting in caches.
//
// After a successful download the file is validated with ValidateFile; if
// validation fails the file is removed and an error is returned.
func DownloadFile(fname, url, dir string) error {
	fpath := filepath.Join(dir, fname)
	for attempt := 1; attempt <= defaultMaxRetries; attempt++ {
		cmd := exec.Command("wget", "--tries=1", "--retry-connrefused", "-O", fname, url)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("attempt %d/%d failed to download %s: %v\n%s",
				attempt, defaultMaxRetries, fname, err, string(out))
			if attempt < defaultMaxRetries {
				time.Sleep(time.Duration(attempt*5) * time.Second)
				continue
			}
			_ = os.Remove(fpath)
			return fmt.Errorf("failed to download %s after %d attempts: %w", fname, defaultMaxRetries, err)
		}
		if err := ValidateFile(fpath); err != nil {
			_ = os.Remove(fpath)
			return fmt.Errorf("downloaded file %s is invalid: %w", fname, err)
		}
		return nil
	}
	return nil
}

// lfsPointerInfo holds the expected SHA256 and size parsed from an LFS pointer.
type lfsPointerInfo struct {
	SHA256 string
	Size   int64
}

// fetchLFSPointer fetches the raw LFS pointer file for the given repo path
// and ref, then parses out the SHA256 hash and file size.
func fetchLFSPointer(ref, repoPath string) (*lfsPointerInfo, error) {
	pointerURL := BenchmarkRawURL(ref, repoPath)
	resp, err := http.Get(pointerURL)
	if err != nil {
		return nil, fmt.Errorf("fetching LFS pointer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LFS pointer returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading LFS pointer body: %w", err)
	}

	info := &lfsPointerInfo{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "oid sha256:") {
			info.SHA256 = strings.TrimPrefix(line, "oid sha256:")
		}
		if strings.HasPrefix(line, "size ") {
			info.Size, err = strconv.ParseInt(strings.TrimPrefix(line, "size "), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing LFS pointer size: %w", err)
			}
		}
	}
	if info.SHA256 == "" || info.Size == 0 {
		return nil, fmt.Errorf("LFS pointer missing oid or size fields")
	}
	return info, nil
}

// verifyFileChecksum computes the SHA256 of the file at fpath and compares it
// against the expected hash. Also verifies the file size matches.
func verifyFileChecksum(fpath string, expected *lfsPointerInfo) error {
	fi, err := os.Stat(fpath)
	if err != nil {
		return err
	}
	if fi.Size() != expected.Size {
		return fmt.Errorf("size mismatch: local %d != expected %d", fi.Size(), expected.Size)
	}

	f, err := os.Open(fpath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("computing SHA256: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected.SHA256 {
		return fmt.Errorf("SHA256 mismatch: local %s != expected %s", actual, expected.SHA256)
	}
	return nil
}

// DownloadLFSFile downloads an LFS-tracked file and verifies its integrity
// against the LFS pointer (SHA256 + size). It first fetches the tiny pointer
// from raw.githubusercontent.com to get the expected hash and size, then
// downloads the actual content from media.githubusercontent.com and verifies.
//
// This provides full integrity verification: truncation, corruption, and
// wrong-file detection are all caught.
func DownloadLFSFile(fname, ref, repoPath, dir string) error {
	pointer, err := fetchLFSPointer(ref, repoPath)
	if err != nil {
		log.Printf("warning: could not fetch LFS pointer for %s, falling back to basic download: %v", fname, err)
		return DownloadFile(fname, BenchmarkLFSURL(ref, repoPath), dir)
	}

	url := BenchmarkLFSURL(ref, repoPath)
	if err := DownloadFile(fname, url, dir); err != nil {
		return err
	}

	fpath := filepath.Join(dir, fname)
	if err := verifyFileChecksum(fpath, pointer); err != nil {
		_ = os.Remove(fpath)
		return fmt.Errorf("integrity check failed for %s: %w", fname, err)
	}
	log.Printf("verified %s: SHA256=%s size=%d", fname, pointer.SHA256, pointer.Size)
	return nil
}

// FileExistsAndValid returns true if the file at fpath exists, is a regular
// file, has size > 0, and passes ValidateFile content checks.
func FileExistsAndValid(fpath string) bool {
	fi, err := os.Stat(fpath)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return false
	}
	return ValidateFile(fpath) == nil
}

// ValidateFile performs content-level integrity checks on a downloaded file:
//   - Rejects Git LFS pointer files (text stubs left when LFS content wasn't fetched).
//   - Verifies .gz files start with the gzip magic bytes (0x1f 0x8b).
func ValidateFile(fpath string) error {
	f, err := os.Open(fpath)
	if err != nil {
		return err
	}
	defer f.Close()

	header := make([]byte, 64)
	n, err := f.Read(header)
	if err != nil {
		return fmt.Errorf("reading file header: %w", err)
	}
	header = header[:n]

	if bytes.HasPrefix(header, lfsPointerPrefix) {
		return fmt.Errorf("file is a Git LFS pointer, not actual content")
	}

	if strings.HasSuffix(fpath, ".gz") {
		if n < 2 || !bytes.HasPrefix(header, gzipMagic) {
			return fmt.Errorf("file does not have valid gzip header")
		}
	}

	return nil
}
