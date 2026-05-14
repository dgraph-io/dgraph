/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */
package benchdata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	benchmarkRepo   = "dgraph-io/dgraph-benchmarks"
	envDataRef      = "DGRAPH_TEST_DATA_REF"
	dataVersionFile = "benchmark-data-version"
	maxRetries      = 3
	concurrentLimit = 5
)

// TestDataFile is a type-safe identifier for benchmark data files.
// Using a named type prevents typos and lets callers reference files
// via the exported constants instead of raw strings.
type TestDataFile string

// Load-test data files.
const (
	OneMillionNoIndexSchema TestDataFile = "1million-noindex.schema"
	OneMillionSchema        TestDataFile = "1million.schema"
	OneMillionRDF           TestDataFile = "1million.rdf.gz"
	TwentyOneMillSchema     TestDataFile = "21million.schema"
	TwentyOneMillRDF        TestDataFile = "21million.rdf.gz"
)

// LDBC data files.
const (
	LDBCTypesSchema  TestDataFile = "ldbcTypes.schema"
	LDBCDeltas       TestDataFile = "Deltas.rdf"
	LDBCComment      TestDataFile = "comment_0.rdf"
	LDBCContainerOf  TestDataFile = "containerOf_0.rdf"
	LDBCForum        TestDataFile = "forum_0.rdf"
	LDBCHasCreator   TestDataFile = "hasCreator_0.rdf"
	LDBCHasInterest  TestDataFile = "hasInterest_0.rdf"
	LDBCHasMember    TestDataFile = "hasMember_0.rdf"
	LDBCHasModerator TestDataFile = "hasModerator_0.rdf"
	LDBCHasTag       TestDataFile = "hasTag_0.rdf"
	LDBCHasType      TestDataFile = "hasType_0.rdf"
	LDBCIsLocatedIn  TestDataFile = "isLocatedIn_0.rdf"
	LDBCIsPartOf     TestDataFile = "isPartOf_0.rdf"
	LDBCIsSubclassOf TestDataFile = "isSubclassOf_0.rdf"
	LDBCKnows        TestDataFile = "knows_0.rdf"
	LDBCLikes        TestDataFile = "likes_0.rdf"
	LDBCOrganisation TestDataFile = "organisation_0.rdf"
	LDBCPerson       TestDataFile = "person_0.rdf"
	LDBCPlace        TestDataFile = "place_0.rdf"
	LDBCPost         TestDataFile = "post_0.rdf"
	LDBCReplyOf      TestDataFile = "replyOf_0.rdf"
	LDBCStudyAt      TestDataFile = "studyAt_0.rdf"
	LDBCTag          TestDataFile = "tag_0.rdf"
	LDBCTagclass     TestDataFile = "tagclass_0.rdf"
	LDBCWorkAt       TestDataFile = "workAt_0.rdf"
)

// Pre-built file groups for common test suites.
var (
	LoadTestFiles []TestDataFile
	LDBCFiles     []TestDataFile
	AllFiles      []TestDataFile
)

// repoFilePath maps each TestDataFile to its path within the dgraph-benchmarks repo.
var repoFilePath = map[TestDataFile]string{
	OneMillionNoIndexSchema: "data/1million-noindex.schema",
	OneMillionSchema:        "data/1million.schema",
	OneMillionRDF:           "data/1million.rdf.gz",
	TwentyOneMillSchema:     "data/21million.schema",
	TwentyOneMillRDF:        "data/21million.rdf.gz",
	LDBCTypesSchema:         "ldbc/sf0.3/ldbcTypes.schema",
	LDBCDeltas:              "ldbc/sf0.3/ldbc_rdf_0.3/Deltas.rdf",
	LDBCComment:             "ldbc/sf0.3/ldbc_rdf_0.3/comment_0.rdf",
	LDBCContainerOf:         "ldbc/sf0.3/ldbc_rdf_0.3/containerOf_0.rdf",
	LDBCForum:               "ldbc/sf0.3/ldbc_rdf_0.3/forum_0.rdf",
	LDBCHasCreator:          "ldbc/sf0.3/ldbc_rdf_0.3/hasCreator_0.rdf",
	LDBCHasInterest:         "ldbc/sf0.3/ldbc_rdf_0.3/hasInterest_0.rdf",
	LDBCHasMember:           "ldbc/sf0.3/ldbc_rdf_0.3/hasMember_0.rdf",
	LDBCHasModerator:        "ldbc/sf0.3/ldbc_rdf_0.3/hasModerator_0.rdf",
	LDBCHasTag:              "ldbc/sf0.3/ldbc_rdf_0.3/hasTag_0.rdf",
	LDBCHasType:             "ldbc/sf0.3/ldbc_rdf_0.3/hasType_0.rdf",
	LDBCIsLocatedIn:         "ldbc/sf0.3/ldbc_rdf_0.3/isLocatedIn_0.rdf",
	LDBCIsPartOf:            "ldbc/sf0.3/ldbc_rdf_0.3/isPartOf_0.rdf",
	LDBCIsSubclassOf:        "ldbc/sf0.3/ldbc_rdf_0.3/isSubclassOf_0.rdf",
	LDBCKnows:               "ldbc/sf0.3/ldbc_rdf_0.3/knows_0.rdf",
	LDBCLikes:               "ldbc/sf0.3/ldbc_rdf_0.3/likes_0.rdf",
	LDBCOrganisation:        "ldbc/sf0.3/ldbc_rdf_0.3/organisation_0.rdf",
	LDBCPerson:              "ldbc/sf0.3/ldbc_rdf_0.3/person_0.rdf",
	LDBCPlace:               "ldbc/sf0.3/ldbc_rdf_0.3/place_0.rdf",
	LDBCPost:                "ldbc/sf0.3/ldbc_rdf_0.3/post_0.rdf",
	LDBCReplyOf:             "ldbc/sf0.3/ldbc_rdf_0.3/replyOf_0.rdf",
	LDBCStudyAt:             "ldbc/sf0.3/ldbc_rdf_0.3/studyAt_0.rdf",
	LDBCTag:                 "ldbc/sf0.3/ldbc_rdf_0.3/tag_0.rdf",
	LDBCTagclass:            "ldbc/sf0.3/ldbc_rdf_0.3/tagclass_0.rdf",
	LDBCWorkAt:              "ldbc/sf0.3/ldbc_rdf_0.3/workAt_0.rdf",
}

func init() {
	LoadTestFiles = []TestDataFile{
		OneMillionNoIndexSchema, OneMillionSchema, OneMillionRDF,
		TwentyOneMillSchema, TwentyOneMillRDF,
	}
	LDBCFiles = []TestDataFile{
		LDBCTypesSchema,
		LDBCDeltas, LDBCComment, LDBCContainerOf, LDBCForum,
		LDBCHasCreator, LDBCHasInterest, LDBCHasMember, LDBCHasModerator,
		LDBCHasTag, LDBCHasType, LDBCIsLocatedIn, LDBCIsPartOf,
		LDBCIsSubclassOf, LDBCKnows, LDBCLikes, LDBCOrganisation,
		LDBCPerson, LDBCPlace, LDBCPost, LDBCReplyOf,
		LDBCStudyAt, LDBCTag, LDBCTagclass, LDBCWorkAt,
	}
	AllFiles = make([]TestDataFile, 0, len(repoFilePath))
	for k := range repoFilePath {
		AllFiles = append(AllFiles, k)
	}
}

// DataRef returns the git ref (branch, tag, or SHA) to use when downloading
// benchmark data. Resolution order:
//  1. refOverride argument (e.g. from a --data-ref CLI flag)
//  2. DGRAPH_TEST_DATA_REF environment variable
//  3. Contents of benchdata/benchmark-data-version file
//  4. Falls back to "main"
func DataRef(refOverride string) string {
	if refOverride != "" {
		return refOverride
	}
	if v := os.Getenv(envDataRef); v != "" {
		return v
	}
	versionFile := filepath.Join(pkgDir(), dataVersionFile)
	if data, err := os.ReadFile(versionFile); err == nil {
		if ref := strings.TrimSpace(string(data)); ref != "" {
			return ref
		}
	}
	return "main"
}

// EnsureFiles ensures the specified test data files exist in destDir,
// downloading any missing or invalid ones from dgraph-benchmarks at the given
// git ref. If no files are specified, all registered files are ensured.
// Returns the local paths of all requested files.
//
// LFS-tracked files are automatically detected via the GitHub Contents API
// and downloaded from the appropriate CDN URL. SHA256 checksums from the LFS
// pointer are verified after download.
func EnsureFiles(destDir, ref string, files ...TestDataFile) ([]string, error) {
	if len(files) == 0 {
		files = AllFiles
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating dest dir: %w", err)
	}

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = filepath.Join(destDir, string(f))
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(concurrentLimit)

	for i, f := range files {
		dest := paths[i]
		if fileExistsAndValid(dest) {
			log.Printf("Skipping %s (already exists)", f)
			continue
		}
		g.Go(func() error {
			start := time.Now()
			if err := resolveAndDownload(f, ref, dest); err != nil {
				return fmt.Errorf("downloading %s: %w", f, err)
			}
			log.Printf("Downloaded %s in %s", f, time.Since(start))
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return paths, nil
}

type lfsPointerInfo struct {
	OID  string
	Size int64
}

// detectLFS checks whether a file in the benchmark repo is LFS-tracked by
// fetching the raw git blob via the GitHub Contents API. For LFS-tracked files
// the API returns the pointer text (~130 bytes) regardless of the actual file
// size, so this works reliably for files of any size.
func detectLFS(repoPath, ref string) (*lfsPointerInfo, error) {
	token := os.Getenv("GITHUB_TOKEN")
	content, err := fetchRawBlob(repoPath, ref, token)
	if err != nil {
		return nil, err
	}
	return parseLFSPointer(content), nil
}

func fetchRawBlob(path, ref, token string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s",
		benchmarkRepo, path, ref)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3.raw")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub Contents API raw %d: %s", resp.StatusCode, body)
	}

	// LFS pointers are ~130 bytes. Cap the read to avoid buffering large
	// non-LFS file content just for the LFS check.
	const maxPointerSize = 1024
	buf := make([]byte, maxPointerSize)
	n, err := io.ReadFull(resp.Body, buf)
	if err == io.ErrUnexpectedEOF {
		// File is smaller than 1KB -- that's fine, use what we got.
		return buf[:n], nil
	}
	if err != nil {
		return nil, err
	}
	// Read exactly 1KB. If there's more data, it's not an LFS pointer.
	return buf[:n], nil
}

func parseLFSPointer(content []byte) *lfsPointerInfo {
	s := string(content)
	if !strings.HasPrefix(s, "version https://git-lfs.github.com/spec/v1") {
		return nil
	}
	info := &lfsPointerInfo{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "oid sha256:") {
			info.OID = strings.TrimPrefix(line, "oid sha256:")
		}
		if strings.HasPrefix(line, "size ") {
			if _, err := fmt.Sscanf(line, "size %d", &info.Size); err != nil {
				return nil
			}
		}
	}
	if info.OID == "" {
		return nil
	}
	return info
}

// resolveAndDownload determines the correct download URL for a file (auto-
// detecting LFS), downloads it with retry, and validates the result.
func resolveAndDownload(file TestDataFile, ref, destPath string) error {
	path, ok := repoFilePath[file]
	if !ok {
		return fmt.Errorf("unknown test data file: %s", file)
	}

	lfsInfo, err := detectLFS(path, ref)
	if err != nil {
		// GitHub API unavailable (rate limit, network). Try media URL first
		// (handles LFS), fall back to raw URL (handles non-LFS).
		log.Printf("warning: LFS detection failed for %s, trying both CDN URLs: %v", file, err)
		mediaURL := fmt.Sprintf("https://media.githubusercontent.com/media/%s/%s/%s",
			benchmarkRepo, ref, path)
		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s",
			benchmarkRepo, ref, path)
		if dlErr := downloadWithRetry(mediaURL, destPath); dlErr == nil {
			if valErr := validateFile(destPath); valErr == nil {
				return nil
			}
		}
		if dlErr := downloadWithRetry(rawURL, destPath); dlErr != nil {
			return dlErr
		}
		return validateFile(destPath)
	}

	if lfsInfo != nil {
		url := fmt.Sprintf("https://media.githubusercontent.com/media/%s/%s/%s",
			benchmarkRepo, ref, path)
		if err := downloadWithRetry(url, destPath); err != nil {
			return err
		}
		if err := verifyChecksum(destPath, lfsInfo); err != nil {
			_ = os.Remove(destPath)
			return fmt.Errorf("integrity check failed: %w", err)
		}
		log.Printf("verified %s: SHA256=%s size=%d", file, lfsInfo.OID, lfsInfo.Size)
	} else {
		url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s",
			benchmarkRepo, ref, path)
		if err := downloadWithRetry(url, destPath); err != nil {
			return err
		}
	}

	return validateFile(destPath)
}

// downloadWithRetry downloads a URL to destPath with Go-level retry
// (3 attempts, exponential backoff). On final failure the partial file
// is removed.
func downloadWithRetry(url, destPath string) error {
	token := os.Getenv("GITHUB_TOKEN")
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := downloadToFile(url, destPath, token); err != nil {
			log.Printf("attempt %d/%d failed to download %s: %v",
				attempt, maxRetries, filepath.Base(destPath), err)
			_ = os.Remove(destPath)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*5) * time.Second)
				continue
			}
			return fmt.Errorf("failed after %d attempts: %w", maxRetries, err)
		}
		return nil
	}
	return nil
}

// downloadToFile streams a URL to a local file.
func downloadToFile(url, destPath, token string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if token != "" && strings.Contains(url, "githubusercontent.com") {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

var (
	lfsPointerPrefix = []byte("version https://git-lfs.github.com")
	gzipMagic        = []byte{0x1f, 0x8b}
)

func fileExistsAndValid(fpath string) bool {
	fi, err := os.Stat(fpath)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return false
	}
	return validateFile(fpath) == nil
}

func validateFile(fpath string) error {
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
	if strings.HasSuffix(fpath, ".gz") && (n < 2 || !bytes.HasPrefix(header, gzipMagic)) {
		return fmt.Errorf("file does not have valid gzip header")
	}
	return nil
}

func verifyChecksum(fpath string, info *lfsPointerInfo) error {
	fi, err := os.Stat(fpath)
	if err != nil {
		return err
	}
	if fi.Size() != info.Size {
		return fmt.Errorf("size mismatch: local %d != expected %d", fi.Size(), info.Size)
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
	if actual != info.OID {
		return fmt.Errorf("SHA256 mismatch: local %s != expected %s", actual, info.OID)
	}
	return nil
}

func pkgDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Dir(thisFile)
}
