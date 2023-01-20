/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/ristretto/z"
)

const (
	backupTimeFormat = "2006-01-02T15-04-05.000"
	bufferSize       = 256 * 1024
	flushInterval    = 10 * time.Second
	VerificationText = "Hello World"
)

// This is done to ensure LogWriter always implement io.WriterCloser
var _ io.WriteCloser = (*LogWriter)(nil)

type LogWriter struct {
	FilePath      string
	MaxSize       int64
	MaxAge        int64 // number of days
	Compress      bool
	EncryptionKey []byte

	baseIv      [12]byte
	mu          sync.Mutex
	size        int64
	file        *os.File
	writer      *bufio.Writer
	flushTicker *time.Ticker
	closer      *z.Closer
	// To manage order of cleaning old logs files
	manageChannel chan bool
}

func (l *LogWriter) Init() (*LogWriter, error) {
	if l == nil {
		return nil, nil
	}

	l.manageOldLogs()
	if err := l.open(); err != nil {
		return nil, fmt.Errorf("not able to create new file %v", err)
	}
	l.closer = z.NewCloser(2)
	l.manageChannel = make(chan bool, 1)
	go func() {
		defer l.closer.Done()
		for {
			select {
			case <-l.manageChannel:
				l.manageOldLogs()
			case <-l.closer.HasBeenClosed():
				return
			}
		}
	}()

	l.flushTicker = time.NewTicker(flushInterval)
	go l.flushPeriodic()
	return l, nil
}

func (l *LogWriter) Write(p []byte) (int, error) {
	if l == nil {
		return 0, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.size+int64(len(p)) >= l.MaxSize*1024*1024 {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}

	// if encryption is enabled store the data in encyrpted way
	if l.EncryptionKey != nil {
		bytes, err := encrypt(l.EncryptionKey, l.baseIv, p)
		if err != nil {
			return 0, err
		}
		n, err := l.writer.Write(bytes)
		l.size = l.size + int64(n)
		return n, err
	}

	n, err := l.writer.Write(p)
	l.size = l.size + int64(n)
	return n, err
}

func (l *LogWriter) Close() error {
	if l == nil {
		return nil
	}
	// close all go routines first before acquiring the lock to avoid contention
	l.closer.SignalAndWait()

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	l.flush()
	l.flushTicker.Stop()
	close(l.manageChannel)
	_ = l.file.Close()
	l.writer = nil
	l.file = nil
	return nil
}

// flushPeriodic periodically flushes the log file buffers.
func (l *LogWriter) flushPeriodic() {
	if l == nil {
		return
	}
	defer l.closer.Done()
	for {
		select {
		case <-l.flushTicker.C:
			l.mu.Lock()
			l.flush()
			l.mu.Unlock()
		case <-l.closer.HasBeenClosed():
			return
		}
	}
}

// LogWriter should be locked while calling this
func (l *LogWriter) flush() {
	if l == nil {
		return
	}

	_ = l.writer.Flush()
	_ = l.file.Sync()
}

func encrypt(key []byte, baseIv [12]byte, src []byte) ([]byte, error) {
	iv := make([]byte, 16)
	copy(iv, baseIv[:])
	binary.BigEndian.PutUint32(iv[12:], uint32(len(src)))
	allocate, err := y.XORBlockAllocate(src, key, iv)
	if err != nil {
		return nil, err
	}
	allocate = append(iv[12:], allocate...)
	return allocate, nil
}

func decrypt(key []byte, baseIv [12]byte, src []byte) ([]byte, error) {
	iv := make([]byte, 16)
	copy(iv, baseIv[:])
	binary.BigEndian.PutUint32(iv[12:], uint32(len(src)))
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, iv[:])
	stream.XORKeyStream(src, src)
	return src, nil
}

func (l *LogWriter) rotate() error {
	if l == nil {
		return nil
	}

	l.flush()
	if err := l.file.Close(); err != nil {
		return err
	}

	if _, err := os.Stat(l.FilePath); err == nil {
		// move the existing file
		newname := backupName(l.FilePath)
		if err := os.Rename(l.FilePath, newname); err != nil {
			return fmt.Errorf("can't rename log file: %s", err)
		}
	}

	l.manageChannel <- true
	return l.open()
}

func (l *LogWriter) open() error {
	if l == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(l.FilePath), 0755); err != nil {
		return err
	}

	size := func() int64 {
		info, err := os.Stat(l.FilePath)
		if err != nil {
			return 0
		}
		return info.Size()
	}

	openNew := func() error {
		f, err := os.OpenFile(l.FilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
		if err != nil {
			return err
		}
		l.file = f
		l.writer = bufio.NewWriterSize(l.file, bufferSize)

		if l.EncryptionKey != nil {
			rand.Read(l.baseIv[:]) //nolint:gosec // cryptographic precision not required for randomly selecting from slice
			bytes, err := encrypt(l.EncryptionKey, l.baseIv, []byte(VerificationText))
			if err != nil {
				return err
			}
			if _, err = l.writer.Write(append(l.baseIv[:], bytes[:]...)); err != nil {
				return err
			}
		}
		l.size = size()
		return nil
	}

	info, err := os.Stat(l.FilePath)
	if err != nil { // if any error try to open new log file itself
		return openNew()
	}

	// encryption is enabled and file is corrupted as not able to read the IV
	if l.EncryptionKey != nil && info.Size() < 12 {
		return openNew()
	}

	f, err := os.OpenFile(l.FilePath, os.O_APPEND|os.O_RDWR, os.ModePerm)
	if err != nil {
		return openNew()
	}

	l.file = f
	if l.EncryptionKey != nil {
		// If not able to read the baseIv, then this file might be corrupted.
		// open the new file in that case
		if _, err = l.file.ReadAt(l.baseIv[:], 0); err != nil {
			_ = l.file.Close()
			return openNew()
		}
		text := make([]byte, 11)
		if _, err := f.ReadAt(text, 16); err != nil {
			_ = f.Close()
			return openNew()
		}
		if t, err := decrypt(l.EncryptionKey, l.baseIv, text); err != nil ||
			string(t) != VerificationText {
			// different encryption key. Better to open new file here
			_ = f.Close()
			return openNew()
		}
	}

	l.writer = bufio.NewWriterSize(l.file, bufferSize)
	l.size = size()
	return nil
}

func backupName(name string) string {
	dir := filepath.Dir(name)
	prefix, ext := prefixAndExt(name)
	timestamp := time.Now().UTC().Format(backupTimeFormat)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, timestamp, ext))
}

func compress(src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}

	defer f.Close()
	gzf, err := os.OpenFile(src+".gz", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}

	defer gzf.Close()
	gz := gzip.NewWriter(gzf)
	defer gz.Close()
	if _, err := io.Copy(gz, f); err != nil {
		os.Remove(src + ".gz")
		return err
	}
	// close the descriptors because we need to delete the file
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		return err
	}
	return nil
}

// this should be called in a serial order
func (l *LogWriter) manageOldLogs() {
	if l == nil {
		return
	}

	toRemove, toKeep, err := processOldLogFiles(l.FilePath, l.MaxAge)
	if err != nil {
		return
	}

	for _, f := range toRemove {
		errRemove := os.Remove(filepath.Join(filepath.Dir(l.FilePath), f))
		if err == nil && errRemove != nil {
			err = errRemove
		}
	}

	// if compression enabled do compress
	if l.Compress {
		for _, f := range toKeep {
			// already compressed no need
			if strings.HasSuffix(f, ".gz") {
				continue
			}
			fn := filepath.Join(filepath.Dir(l.FilePath), f)
			errCompress := compress(fn)
			if err == nil && errCompress != nil {
				err = errCompress
			}
		}
	}

	if err != nil {
		fmt.Printf("error while managing old log files %+v\n", err)
	}
}

// prefixAndExt extracts the filename and extension from a filepath.
// eg. prefixAndExt("/home/foo/file.ext") would return ("file", ".ext").
func prefixAndExt(file string) (prefix, ext string) {
	filename := filepath.Base(file)
	ext = filepath.Ext(filename)
	prefix = filename[:len(filename)-len(ext)]
	return prefix, ext
}

func processOldLogFiles(fp string, maxAge int64) ([]string, []string, error) {
	dir := filepath.Dir(fp)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("can't read log file directory: %s", err)
	}

	defPrefix, defExt := prefixAndExt(fp)
	// check only for old files. Those files have - before the time
	defPrefix = defPrefix + "-"
	toRemove := make([]string, 0)
	toKeep := make([]string, 0)

	diff := 24 * time.Hour * time.Duration(maxAge)
	cutoff := time.Now().Add(-diff)

	for _, f := range files {
		if f.IsDir() || // f is directory
			!strings.HasPrefix(f.Name(), defPrefix) || // f doesnt start with prefix
			!(strings.HasSuffix(f.Name(), defExt) || strings.HasSuffix(f.Name(), defExt+".gz")) {
			continue
		}

		_, e := prefixAndExt(fp)
		tsString := f.Name()[len(defPrefix) : len(f.Name())-len(e)]
		ts, err := time.Parse(backupTimeFormat, tsString)
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			toRemove = append(toRemove, f.Name())
		} else {
			toKeep = append(toKeep, f.Name())
		}
	}

	return toRemove, toKeep, nil
}
