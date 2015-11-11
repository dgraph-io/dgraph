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
	"container/list"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("commitlog")

type logFile struct {
	sync.RWMutex
	startTs uint64
	f       *os.File
	size    uint64
}

type Logger struct {
	// Directory to store logs into.
	dir string

	// Prefix all filenames with this.
	filePrefix string

	// MaxSize is the maximum size of commit log file in bytes,
	// before it gets rotated.
	maxSize uint64

	flistm sync.RWMutex
	flist  *list.List
}

func NewLogger(dir string, fileprefix string, maxSize uint64) *Logger {
	l := new(Logger)
	l.dir = dir
	l.filePrefix = fileprefix
	l.maxSize = maxSize
	l.flist = list.New()
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
	return nil
}

func (l *Logger) Init() {
	if err := filepath.Walk(l.dir, l.handleFile); err != nil {
		glog.WithError(err).Fatal("While walking over directory")
	}
}

func (l *Logger) filepath(ts int64) string {
	return fmt.Sprintf("%s-%s.log", l.filePrefix, strconv.FormatInt(ts, 16))
}

func (l *Logger) AddLog(ts uint64, hash uint32, value []byte) {
}
