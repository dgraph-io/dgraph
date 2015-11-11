/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// commit package provides commit logs for storing mutations, as they arrive
// at the server. Mutations also get stored in memory within posting.List.
// So, commit logs are useful to handle machine crashes, and re-init of a
// posting list.
// This package provides functionality to write to a rotating log, and a way
// to quickly filter relevant entries corresponding to an attribute.
package commit

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("commitlog")

type logFile struct {
	sync.RWMutex
	endTs int64
	path  string
	f     *os.File
	size  uint64
}

type ByTimestamp []*logFile

func (b ByTimestamp) Len() int      { return len(b) }
func (b ByTimestamp) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByTimestamp) Less(i, j int) bool {
	return b[i].endTs < b[j].endTs
}

type Logger struct {
	// Directory to store logs into.
	dir string

	// Prefix all filenames with this.
	filePrefix string

	// MaxSize is the maximum size of commit log file in bytes,
	// before it gets rotated.
	maxSize uint64

	// Sync every N logs. A value of zero or less would mean
	// sync every append to file.
	SyncEvery int

	sync.RWMutex
	list              []*logFile
	curFile           *os.File
	size              int64
	lastLogTs         int64
	logsSinceLastSync int
}

func NewLogger(dir string, fileprefix string, maxSize uint64) *Logger {
	l := new(Logger)
	l.dir = dir
	l.filePrefix = fileprefix
	l.maxSize = maxSize
	return l
}

func (l *Logger) handleFile(path string, info os.FileInfo, err error) error {
	if info.IsDir() {
		return nil
	}
	if !strings.HasPrefix(info.Name(), l.filePrefix+"-") {
		return nil
	}
	if !strings.HasSuffix(info.Name(), ".log") {
		return nil
	}
	lidx := strings.LastIndex(info.Name(), ".log")
	tstring := info.Name()[len(l.filePrefix)+1 : lidx]
	glog.WithField("log_ts", tstring).Debug("Found log.")
	if tstring == "current" {
		return nil
	}
	ts, err := strconv.ParseInt(tstring, 16, 64)
	if err != nil {
		return err
	}

	lf := new(logFile)
	lf.endTs = ts
	lf.path = path
	l.list = append(l.list, lf)

	return nil
}

func (l *Logger) Init() {
	l.Lock()
	defer l.Unlock()

	if err := filepath.Walk(l.dir, l.handleFile); err != nil {
		glog.WithError(err).Fatal("While walking over directory")
	}
	sort.Sort(ByTimestamp(l.list))
	l.createNew()
}

func (l *Logger) filepath(ts int64) string {
	return fmt.Sprintf("%s-%s.log", l.filePrefix, strconv.FormatInt(ts, 16))
}

type Header struct {
	ts   int64
	hash uint32
	size int32
}

func parseHeader(hdr []byte) (Header, error) {
	buf := bytes.NewBuffer(hdr)
	var h Header
	var err error
	setError(&err, binary.Read(buf, binary.LittleEndian, &h.ts))
	setError(&err, binary.Read(buf, binary.LittleEndian, &h.hash))
	setError(&err, binary.Read(buf, binary.LittleEndian, &h.size))
	if err != nil {
		glog.WithError(err).Error("While parsing header.")
		return h, err
	}
	return h, nil
}

func lastTimestamp(path string) (int64, error) {
	f, err := os.Open(path)
	defer f.Close()

	if err != nil {
		return 0, err
	}

	reader := bufio.NewReaderSize(f, 2<<20)
	var maxTs int64
	header := make([]byte, 16)
	count := 0
	for {
		n, err := reader.Read(header)
		if err == io.EOF {
			break
		}
		if n < len(header) {
			glog.WithField("n", n).Fatal("Unable to read the full 16 byte header.")
		}
		if err != nil {
			glog.WithError(err).Error("While peeking into reader.")
			return 0, err
		}
		count += 1
		h, err := parseHeader(header)
		if err != nil {
			return 0, err
		}

		if h.ts > maxTs {
			maxTs = h.ts

		} else if h.ts < maxTs {
			glog.WithFields(logrus.Fields{
				"ts":         h.ts,
				"maxts":      maxTs,
				"path":       f.Name(),
				"numrecords": count,
			}).Fatal("Log file doesn't have monotonically increasing records.")
		}

		reader.Discard(int(h.size))
	}
	return maxTs, nil
}

// Expects a lock has already been acquired.
func (l *Logger) createNew() {
	path := filepath.Join(l.dir, fmt.Sprintf("%s-current.log", l.filePrefix))
	if err := os.MkdirAll(l.dir, 0744); err != nil {
		glog.WithError(err).Fatal("Unable to create directory.")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		os.FileMode(0644))
	if err != nil {
		glog.WithError(err).Fatal("Unable to create a new file.")
	}
	l.curFile = f
	l.size = 0
}

func setError(prev *error, n error) {
	if prev == nil {
		prev = &n
	}
	return
}

func (l *Logger) AddLog(ts int64, hash uint32, value []byte) error {
	if ts < l.lastLogTs {
		return fmt.Errorf("Timestamp lower than last log timestamp.")
	}

	buf := new(bytes.Buffer)
	var err error
	setError(&err, binary.Write(buf, binary.LittleEndian, ts))
	setError(&err, binary.Write(buf, binary.LittleEndian, hash))
	setError(&err, binary.Write(buf, binary.LittleEndian, int32(len(value))))
	_, nerr := buf.Write(value)
	setError(&err, nerr)
	if err != nil {
		return err
	}
	glog.WithField("bytes", buf.Len()).WithField("ts", ts).
		Debug("Log entry buffer.")

	l.Lock()
	defer l.Unlock()
	if l.curFile == nil {
		glog.Fatalf("Current file isn't initialized.")
	}

	if _, err = l.curFile.Write(buf.Bytes()); err != nil {
		glog.WithError(err).Error("While writing to current file.")
		return err
	}
	l.logsSinceLastSync += 1
	l.lastLogTs = ts
	if l.SyncEvery <= 0 || l.logsSinceLastSync >= l.SyncEvery {
		l.logsSinceLastSync = 0
		return l.curFile.Sync()
	}
	return nil
}
