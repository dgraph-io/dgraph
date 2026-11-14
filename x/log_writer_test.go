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

// TestLogWriterRotateBackupNamesUnique rotates faster than the millisecond
// resolution of the backup file name. Every rotation must still end up in its
// own backup file, and the names must still parse for the MaxAge cleanup.
func TestLogWriterRotateBackupNamesUnique(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	lw, err := (&LogWriter{FilePath: path, MaxSize: 1, MaxAge: 1}).Init()
	require.NoError(t, err)

	const rotations = 5
	for i := range rotations {
		_, err := lw.Write(fmt.Appendf(nil, "line %d\n", i))
		require.NoError(t, err)
		lw.mu.Lock()
		err = lw.rotate()
		lw.mu.Unlock()
		require.NoError(t, err)
	}
	require.NoError(t, lw.Close())

	_, kept, err := processOldLogFiles(path, lw.MaxAge)
	require.NoError(t, err)
	require.Len(t, kept, rotations)
}

// TestLogWriterReinitKeepsEarlierBackups runs two writers on the same path back
// to back, the way the audit logger is closed and re-initialised. The second
// writer starts without knowledge of the first one's backup timestamps, which
// may be ahead of the clock, and must still not rename onto any of its backups.
func TestLogWriterReinitKeepsEarlierBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	const writers, rotations = 2, 3
	for range writers {
		lw, err := (&LogWriter{FilePath: path, MaxSize: 1, MaxAge: 1}).Init()
		require.NoError(t, err)
		for i := range rotations {
			_, err := lw.Write(fmt.Appendf(nil, "line %d\n", i))
			require.NoError(t, err)
			lw.mu.Lock()
			err = lw.rotate()
			lw.mu.Unlock()
			require.NoError(t, err)
		}
		require.NoError(t, lw.Close())
	}

	_, kept, err := processOldLogFiles(path, 1)
	require.NoError(t, err)
	require.Len(t, kept, writers*rotations)
}

// TestLogWriterRotateSkipsExistingBackups occupies every backup name, plain and
// compressed, for the next second and then rotates once. The rotation must
// leave all of them untouched and land in a name of its own.
func TestLogWriterRotateSkipsExistingBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	const earlier = "earlier backup\n"
	start := time.Now().UTC().Truncate(time.Millisecond)
	occupied := make([]string, 0, 1000)
	for i := range 1000 {
		name := backupName(path, start.Add(time.Duration(i)*time.Millisecond))
		if i%2 == 1 {
			name += ".gz"
		}
		require.NoError(t, os.WriteFile(name, []byte(earlier), 0o600))
		occupied = append(occupied, name)
	}

	lw, err := (&LogWriter{FilePath: path, MaxSize: 1, MaxAge: 1}).Init()
	require.NoError(t, err)
	_, err = lw.Write([]byte("fresh\n"))
	require.NoError(t, err)
	lw.mu.Lock()
	err = lw.rotate()
	lw.mu.Unlock()
	require.NoError(t, err)
	require.NoError(t, lw.Close())

	for _, name := range occupied {
		content, err := os.ReadFile(name)
		require.NoError(t, err)
		require.Equal(t, earlier, string(content), name)
	}
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
