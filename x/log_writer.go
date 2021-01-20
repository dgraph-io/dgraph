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
	"github.com/dgraph-io/badger/v3/y"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	backupTimeFormat = "2021-01-19T15-04-05.000"
)

var _ io.WriteCloser = (*LogWriter)(nil)

type LogWriter struct {
	FilePath      string
	MaxSize       int64
	MaxAge        int // number of days
	Compress      bool
	EncryptionKey []byte

	baseIv          [12]byte
	mu              sync.Mutex
	size            int64
	file            *os.File
	mch             chan bool
	startDirManager sync.Once
}

func (l *LogWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var err error
	if l.file == nil {
		l.manageLogDir()
		// open file
		if err = l.open(); err != nil {
			return 0, fmt.Errorf("not able to create new file %v", err)
		}
	}

	if l.size+int64(len(p)) >= l.MaxSize*1024*1024 {
		if err = l.rotate(); err != nil {
			return 0, err
		}
	}

	// if encryption is enabled store the data in encyrpted way
	if l.EncryptionKey != nil {
		bytes, err := encrypt(l.EncryptionKey, l.baseIv, p)
		if err != nil {
			return 0, err
		}
		n, err := l.file.Write(bytes)
		l.size = l.size + int64(n)
		return n, err
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

func (l *LogWriter) open() error {
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
		if l.EncryptionKey != nil {
			rand.Read(l.baseIv[:])
			if _, err = f.Write(l.baseIv[:]); err != nil {
				return err
			}
		}
		l.file = f
		l.size = size()
		return nil
	}

	info, err := os.Stat(l.FilePath)
	if os.IsNotExist(err) {
		return openNew()
	}
	if err != nil {
		return err
	}

	// encryption is enabled and file is corrupted as not able to read the IV
	if l.EncryptionKey != nil && info.Size() < 12 {
		return openNew()
	}

	f, err := os.OpenFile(l.FilePath, os.O_APPEND|os.O_RDWR, os.ModePerm)
	if err != nil {
		return openNew()
	}

	// If not able to read the baseIv, then this file might be corrupted.
	// open the new file in that case
	if _, err = f.ReadAt(l.baseIv[:], 0); err != nil {
		return openNew()
	}
	l.file = f
	l.size = size()
	return nil
}

func (l *LogWriter) rotate() error {
	var err error
	// file not open
	if l.file == nil {
		return l.open()
	}

	if err = l.file.Close(); err != nil {
		return err
	}

	if _, err = os.Stat(l.FilePath); err != nil {
		// move the existing file
		newname := backupName(l.FilePath)
		if err := os.Rename(l.FilePath, newname); err != nil {
			return fmt.Errorf("can't rename log file: %s", err)
		}
	}

	err = l.open()
	l.manageLogDir()
	return err
}

func backupName(name string) string {
	dir := filepath.Dir(name)
	prefix, ext := prefixAndExt(name)
	timestamp := time.Now().Format(backupTimeFormat)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, timestamp, ext))
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

func (l *LogWriter) manageLogDir() {
	l.startDirManager.Do(func() {
		l.mch = make(chan bool, 1)
		go func() {
			for range l.mch {
				l.manageOldLogs()
			}
		}()
	})

	select {
	case l.mch <- true:
	default:
	}
}

// this should be called in a serial order
func (l *LogWriter) manageOldLogs() {
	toRemove, toKeep, err := processOldLogFiles(l.FilePath, l.MaxSize)
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

	diff := time.Duration(int64(24*time.Hour) * int64(maxAge))
	cutoff := time.Now().Add(-1 * diff)

	for _, f := range files {
		if f.IsDir() ||
			!strings.HasPrefix(f.Name(), defPrefix) ||
			!strings.HasSuffix(f.Name(), defExt) ||
			!strings.HasSuffix(f.Name(), defExt+".gz") {
			continue
		}

		p, e := prefixAndExt(fp)
		ts, err := time.Parse(backupTimeFormat, f.Name()[len(p):len(f.Name())-len(e)])
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
