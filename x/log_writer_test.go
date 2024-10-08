/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
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

	time.Sleep(time.Second * 10)
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
	for range 10 {
		go func() {
			for range 1000 {
				n, err := lw.Write(msg)
				require.NoError(t, err)
				require.Equal(t, n, len(msg), "write length is not equal")
			}
		}()
	}
	time.Sleep(time.Second * 10)
	require.NoError(t, lw.Close())
	files, err := os.ReadDir("./log_test")
	require.NoError(t, err)

	lineCount := 0
	for _, f := range files {
		file, _ := os.Open(filepath.Join(filepath.Dir(path), f.Name()))

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

	require.Equal(t, lineCount, 10000)
}
