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
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

	lw, _ = lw.Init()
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

	lw, _ = lw.Init()
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

	lw, _ = lw.Init()
	msg := []byte("abcd")
	msg = bytes.Repeat(msg, 256)
	msg[1023] = '\n'
	for range 10000 {
		n, err := lw.Write(msg)
		require.NoError(t, err)
		require.Equal(t, n, len(msg)+20, "write length is not equal")
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

func writeToLogWriterAndVerify(t *testing.T, lw *LogWriter, path string) {
	msg := []byte("abcd")
	msg = bytes.Repeat(msg, 256)
	msg[1023] = '\n'
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 1000 {
				n, err := lw.Write(msg)
				assert.NoError(t, err)
				assert.Equal(t, len(msg), n, "write length is not equal")
			}
		})
	}
	wg.Wait()
	require.NoError(t, lw.Close())
	files, err := os.ReadDir("./log_test")
	require.NoError(t, err)

	lineCount := 0
	for _, f := range files {
		file, err := os.Open(filepath.Join(filepath.Dir(path), f.Name()))
		require.NoError(t, err)
		defer file.Close()

		var fileScanner *bufio.Scanner
		if strings.HasSuffix(file.Name(), ".gz") {
			gz, err := gzip.NewReader(file)
			require.NoError(t, err)
			all, err := io.ReadAll(gz)
			require.NoError(t, err)
			fileScanner = bufio.NewScanner(bytes.NewReader(all))
			gz.Close()
		} else {
			fileScanner = bufio.NewScanner(file)
		}
		for fileScanner.Scan() {
			lineCount = lineCount + 1
		}
	}

	require.Equal(t, 10000, lineCount)
}

func TestNextBackupName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	ts := time.Date(2026, 10, 27, 9, 40, 12, 682_000_000, time.UTC)

	first := nextBackupName(path, ts)
	require.Equal(t, filepath.Join(dir, "audit-2026-10-27T09-40-12.682.log"), first)

	// A backup already rotated within the same millisecond must not be overwritten.
	require.NoError(t, os.WriteFile(first, nil, 0600))
	second := nextBackupName(path, ts)
	require.Equal(t, filepath.Join(dir, "audit-2026-10-27T09-40-12.683.log"), second)

	// Nor may one that has already been compressed, since compress() truncates the .gz.
	require.NoError(t, os.WriteFile(second+".gz", nil, 0600))
	require.Equal(t, filepath.Join(dir, "audit-2026-10-27T09-40-12.684.log"), nextBackupName(path, ts))

	// Bumped names, plain or compressed, must stay parseable so they still age out.
	toRemove, toKeep, err := processOldLogFiles(path, 1)
	require.NoError(t, err)
	require.Len(t, append(toRemove, toKeep...), 2)
}
