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
	path, _ := filepath.Abs("./log_test/audit.log")
	defer os.RemoveAll(filepath.Dir(path))
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
	path, _ := filepath.Abs("./log_test/audit.log")
	defer os.RemoveAll(filepath.Dir(path))
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

// if this test failed and you changed anything, please check the dgraph audit decrypt command.
// The dgraph audit decrypt command uses the same decryption method
func TestLogWriterWithEncryption(t *testing.T) {
	path, _ := filepath.Abs("./log_test/audit.log.enc")
	defer os.RemoveAll(filepath.Dir(path))
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
	for range 10000 {
		n, err := lw.Write(msg)
		require.NoError(t, err)
		require.Equal(t, len(msg)+20, n, "write length is not equal")
	}

	require.NoError(t, lw.Close())
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	outPath, _ := filepath.Abs("./log_test/audit_out.log")
	outfile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	require.NoError(t, err)
	defer outfile.Close()

	block, err := aes.NewCipher(lw.EncryptionKey)
	require.NoError(t, err)
	stat, err := os.Stat(path)
	require.NoError(t, err)
	iv := make([]byte, aes.BlockSize)
	_, err = file.ReadAt(iv, 0)
	require.NoError(t, err)

	var iterator int64 = 16
	for {
		length := make([]byte, 4)
		_, err = file.ReadAt(length, iterator)
		require.Nil(t, err)
		iterator = iterator + 4
		content := make([]byte, binary.BigEndian.Uint32(length))
		_, err = file.ReadAt(content, iterator)
		require.NoError(t, err)
		iterator = iterator + int64(binary.BigEndian.Uint32(length))
		stream := cipher.NewCTR(block, iv)
		stream.XORKeyStream(content, content)
		//require.True(t, bytes.Equal(content, msg))
		_, err = outfile.Write(content)
		require.NoError(t, err)
		if iterator >= stat.Size() {
			break
		}
		iv := make([]byte, 16)
		_, err = file.ReadAt(iv, iterator)
		require.NoError(t, err)
		iterator = iterator + 16
	}
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

// TestProcessOldLogFilesSeesCompressedBackups checks that compressed backups
// are classified by age like plain ones. Otherwise MaxAge never removes them.
func TestProcessOldLogFilesSeesCompressedBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
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

	require.Equal(t, writers*linesPerWriter, countLines(t, filepath.Dir(path)))
}

// countLines returns the number of lines across every plain or gzipped file in dir.
func countLines(t *testing.T, dir string) int {
	files, err := os.ReadDir(dir)
	require.NoError(t, err)

	lineCount := 0
	for _, f := range files {
		file, err := os.Open(filepath.Join(dir, f.Name()))
		require.NoError(t, err)

		var r io.Reader = file
		if strings.HasSuffix(f.Name(), ".gz") {
			gz, err := gzip.NewReader(file)
			require.NoError(t, err)
			r = gz
		}
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			lineCount++
		}
		require.NoError(t, scanner.Err())
		require.NoError(t, file.Close())
	}
	return lineCount
}
