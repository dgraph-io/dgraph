/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLogWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	lw := &LogWriter{
		FilePath: path,
		MaxSize:  1,
		MaxAge:   1,
		Compress: false,
	}

	lw, err := lw.Init()
	require.NoError(t, err)
	writeToLogWriterAndVerify(t, lw, path)
}

func TestLogWriterWithCompression(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	lw := &LogWriter{
		FilePath: path,
		MaxSize:  1,
		MaxAge:   1,
		Compress: true,
	}

	lw, err := lw.Init()
	require.NoError(t, err)
	writeToLogWriterAndVerify(t, lw, path)
}

// TestLogWriterWithEncryption decrypts every file the writer produced the way
// the dgraph audit decrypt command does: each file opens with a header record
// holding VerificationText, and every record carries its own IV. If this test
// fails after a change to the format, check that command as well.
func TestLogWriterWithEncryption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log.enc")
	lw := &LogWriter{
		FilePath:      path,
		MaxSize:       1,
		MaxAge:        1,
		Compress:      false,
		EncryptionKey: []byte("1234567890123456"), // 16 bytes
	}

	lw, err := lw.Init()
	require.NoError(t, err)
	msg := []byte("abcd")
	msg = bytes.Repeat(msg, 256)
	msg[1023] = '\n'
	const lines = 10000
	for range lines {
		n, err := lw.Write(msg)
		require.NoError(t, err)
		require.Equal(t, len(msg)+20, n, "write length is not equal")
	}
	require.NoError(t, lw.Close())

	files, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Greater(t, len(files), 1, "the writer never rotated")
	records := 0
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(path), f.Name()))
		require.NoError(t, err)
		plain := decryptRecords(t, lw.EncryptionKey, data)
		require.NotEmpty(t, plain, f.Name())
		require.Equal(t, VerificationText, string(plain[0]), "%s: header does not decrypt", f.Name())
		for _, record := range plain[1:] {
			require.Equal(t, msg, record, "%s: record does not decrypt", f.Name())
		}
		records += len(plain) - 1
	}
	require.Equal(t, lines, records)
}

// decryptRecords splits an encrypted log file into its records, each of the
// form [16]byte iv + [4]byte uint32(len(p)) + p, and decrypts every record
// with its own IV.
func decryptRecords(t *testing.T, key, data []byte) [][]byte {
	block, err := aes.NewCipher(key)
	require.NoError(t, err)

	var records [][]byte
	for off := 0; off < len(data); {
		require.GreaterOrEqual(t, len(data)-off, aes.BlockSize+4, "truncated record header at %d", off)
		iv := data[off : off+aes.BlockSize]
		n := int(binary.BigEndian.Uint32(data[off+aes.BlockSize : off+aes.BlockSize+4]))
		off += aes.BlockSize + 4
		require.LessOrEqual(t, off+n, len(data), "truncated record at %d", off)
		plain := make([]byte, n)
		cipher.NewCTR(block, iv).XORKeyStream(plain, data[off:off+n])
		records = append(records, plain)
		off += n
	}
	return records
}

// pinnedClock returns the current millisecond and a clock stuck on it. A writer
// using that clock sees the same time at every rotation, so its backup names
// collide at the resolution of backupTimeFormat on any storage.
func pinnedClock() (time.Time, func() time.Time) {
	at := time.Now().UTC().Truncate(time.Millisecond)
	return at, func() time.Time { return at }
}

// rotateWith writes content and rotates, so the backup created holds content.
func rotateWith(t *testing.T, lw *LogWriter, content string) {
	_, err := lw.Write([]byte(content))
	require.NoError(t, err)
	lw.mu.Lock()
	err = lw.rotate()
	lw.mu.Unlock()
	require.NoError(t, err)
}

// TestLogWriterRotateBackupNamesUnique rotates five times at one clock
// reading. Every rotation must end up in its own backup file, named one
// millisecond after the previous one so MaxAge cleanup still parses it, and
// every backup must still hold the line written before its rotation.
func TestLogWriterRotateBackupNamesUnique(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	lw := &LogWriter{FilePath: path, MaxSize: 1, MaxAge: 1}
	at, clock := pinnedClock()
	lw.now = clock
	lw, err := lw.Init()
	require.NoError(t, err)

	const rotations = 5
	want := make(map[string]string, rotations) // backup name -> content
	for i := range rotations {
		content := fmt.Sprintf("line %d\n", i)
		rotateWith(t, lw, content)
		want[backupName(path, at.Add(time.Duration(i)*time.Millisecond))] = content
	}
	require.NoError(t, lw.Close())

	_, kept, err := processOldLogFiles(path, lw.MaxAge)
	require.NoError(t, err)
	require.Len(t, kept, rotations)
	for name, content := range want {
		got, err := os.ReadFile(name)
		require.NoError(t, err)
		require.Equal(t, content, string(got), name)
	}
}

// TestLogWriterReinitKeepsEarlierBackups runs two writers on the same path back
// to back, the way the audit logger is closed and re-initialised, both at the
// same clock reading. The first writer's backup names are then ahead of the
// clock the second one sees. The second writer must skip every one of them
// and leave their content alone.
func TestLogWriterReinitKeepsEarlierBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	at, clock := pinnedClock()
	const writers, rotations = 2, 3
	want := make(map[string]string, writers*rotations) // backup name -> content
	for w := range writers {
		lw := &LogWriter{FilePath: path, MaxSize: 1, MaxAge: 1}
		lw.now = clock
		lw, err := lw.Init()
		require.NoError(t, err)
		for i := range rotations {
			content := fmt.Sprintf("writer %d line %d\n", w, i)
			rotateWith(t, lw, content)
			want[backupName(path, at.Add(time.Duration(len(want))*time.Millisecond))] = content
		}
		require.NoError(t, lw.Close())
	}

	_, kept, err := processOldLogFiles(path, 1)
	require.NoError(t, err)
	require.Len(t, kept, writers*rotations)
	for name, content := range want {
		got, err := os.ReadFile(name)
		require.NoError(t, err)
		require.Equal(t, content, string(got), name)
	}
}

// TestLogWriterRotateSkipsExistingBackups occupies the names a rotation at the
// pinned clock reading would try first: a plain backup, a compressed one, and
// one present in both forms. The rotation must leave all of them untouched
// and land on the first free name. A compressed backup counts as taken
// because compressing the new backup would otherwise overwrite it.
func TestLogWriterRotateSkipsExistingBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	at, clock := pinnedClock()
	const earlier = "earlier backup\n"
	occupied := []string{
		backupName(path, at),
		backupName(path, at.Add(time.Millisecond)) + ".gz",
		backupName(path, at.Add(2*time.Millisecond)),
		backupName(path, at.Add(2*time.Millisecond)) + ".gz",
	}
	for _, name := range occupied {
		require.NoError(t, os.WriteFile(name, []byte(earlier), 0o600))
	}

	lw := &LogWriter{FilePath: path, MaxSize: 1, MaxAge: 1}
	lw.now = clock
	lw, err := lw.Init()
	require.NoError(t, err)
	rotateWith(t, lw, "fresh\n")
	require.NoError(t, lw.Close())

	for _, name := range occupied {
		content, err := os.ReadFile(name)
		require.NoError(t, err)
		require.Equal(t, earlier, string(content), name)
	}
	landed := backupName(path, at.Add(3*time.Millisecond))
	content, err := os.ReadFile(landed)
	require.NoError(t, err, "the rotation did not land on the first free name")
	require.Equal(t, "fresh\n", string(content))
	_, kept, err := processOldLogFiles(path, 1)
	require.NoError(t, err)
	require.Len(t, kept, len(occupied)+1)
}

// TestProcessOldLogFilesSeesCompressedBackups checks that compressed backups
// are classified by age like plain ones. Otherwise MaxAge never removes them.
func TestProcessOldLogFilesSeesCompressedBackups(t *testing.T) {
	// audit.gz: a plain backup already ends in .gz and must keep it.
	// audit: no extension, so a compressed backup is <prefix>-<ts>.gz.
	for _, base := range []string{"audit.log", "audit.gz", "audit"} {
		t.Run(base, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), base)
			now := time.Now().UTC()
			old := now.Add(-72 * time.Hour)
			oldPlain := backupName(path, old)
			oldGz := backupName(path, old.Add(time.Second)) + ".gz"
			recentPlain := backupName(path, now.Add(-2*time.Second))
			recentGz := backupName(path, now.Add(-time.Second)) + ".gz"
			for _, name := range []string{oldPlain, oldGz, recentPlain, recentGz} {
				require.NoError(t, os.WriteFile(name, []byte("x\n"), 0o600))
			}

			toRemove, toKeep, err := processOldLogFiles(path, 1)
			require.NoError(t, err)
			require.ElementsMatch(t, []string{filepath.Base(oldPlain), filepath.Base(oldGz)}, toRemove)
			require.ElementsMatch(t, []string{filepath.Base(recentPlain), filepath.Base(recentGz)}, toKeep)
		})
	}
}

func writeToLogWriterAndVerify(t *testing.T, lw *LogWriter, path string) {
	msg := []byte("abcd")
	msg = bytes.Repeat(msg, 256)
	msg[1023] = '\n'

	// require must not be called from a non-test goroutine, so the writers
	// report through a channel and the test asserts once they are done.
	const writers, linesPerWriter = 10, 1000
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range linesPerWriter {
				n, err := lw.Write(msg)
				if err != nil {
					errs <- err
					return
				}
				if n != len(msg) {
					errs <- fmt.Errorf("wrote %d bytes, want %d", n, len(msg))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.NoError(t, lw.Close())

	// The line count alone is also right when Write never rotates or nothing
	// gets compressed, so check both happened. Close waits for the running
	// manageOldLogs, so the compressed files are complete by now.
	lines, files, compressed := scanLogDir(t, filepath.Dir(path))
	require.Greater(t, files, 1, "the writer never rotated")
	if lw.Compress {
		require.Greater(t, compressed, 0, "no backup was compressed")
	}
	require.Equal(t, writers*linesPerWriter, lines)
}

// scanLogDir returns the number of lines across every plain or gzipped file in
// dir, the number of files, and how many of them are gzipped.
func scanLogDir(t *testing.T, dir string) (lines, files, compressed int) {
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, f := range entries {
		file, err := os.Open(filepath.Join(dir, f.Name()))
		require.NoError(t, err)

		var r io.Reader = file
		if strings.HasSuffix(f.Name(), ".gz") {
			gz, err := gzip.NewReader(file)
			require.NoError(t, err)
			r = gz
			compressed++
		}
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lines++
		}
		require.NoError(t, scanner.Err())
		require.NoError(t, file.Close())
		files++
	}
	return lines, files, compressed
}
