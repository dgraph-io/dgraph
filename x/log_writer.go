/*
 * Copyright 2017-2021 Dgraph Labs, Inc. and Contributors
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
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"github.com/dgraph-io/badger/v2/y"
	"io"
	"os"
	"sync"
)

var _ io.WriteCloser = (*LogWriter)(nil)

type LogWriter struct {
	FileName      string
	MaxSize       int64
	MaxAge        int // number of days
	Compress      bool
	EncryptionKey []byte
	size          int64
	file          io.Writer
	mu            sync.Mutex

	millCh    chan bool
	startMill sync.Once
}

func (l *LogWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		// open file
	}

	if l.size+int64(len(p)) >= l.MaxSize {
		// rotate
	}

	n, err := l.file.Write(p)
	l.size = l.size + int64(n)
	return n, err
}


func (l *LogWriter) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	l.file = nil
	return nil
}

func encrypt(key, src []byte) ([]byte, error) {
	iv, err := y.GenerateIV()
	if err != nil {
		return nil, err
	}
	allocate, err := y.XORBlockAllocate(src, key, iv)
	if err != nil {
		return nil, err
	}

	allocate = append(allocate, iv...)
	var lenCrcBuf [4]byte
	binary.BigEndian.PutUint32(lenCrcBuf[0:4], uint32(len(allocate)))
	allocate = append(lenCrcBuf[:], allocate...)
	return allocate, nil
}

func compress(src string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}

	defer f.Close()
	gzf, err := os.OpenFile(src+".gz", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open compressed log file: %v", err)
	}

	defer gzf.Close()
	gz := gzip.NewWriter(gzf)
	defer gz.Close()
	if _, err := io.Copy(gz, f); err != nil {
		os.Remove(src + ".gz")
		err = fmt.Errorf("failed to compress log file: %v", err)
		return err
	}

	if err := os.Remove(src); err != nil {
		return err
	}
	return nil
}

func manageOldLogs() {
	// delete old log files.
	// compress log files
}