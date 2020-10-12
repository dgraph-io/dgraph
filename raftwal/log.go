/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package raftwal

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft/raftpb"
)

// WAL is divided up into entryFiles. Each entry file stores maxNumEntries in
// the first logFileOffset bytes. Each entry takes a fixed entrySize bytes of
// space. The variable length data for these entries is written after
// logFileOffset from file beginning. Once snapshot is taken, all the files
// containing entries below snapshot index are deleted.

const (
	// maxNumEntries is maximum number of entries before rotating the file.
	maxNumEntries = 30000
	// logFileOffset is offset in the log file where data is stored.
	logFileOffset = 1 << 20 // 1MB
	// logFileSize is the initial size of the log file.
	logFileSize = 16 << 30
	// entrySize is the size in bytes of a single entry.
	entrySize = 32
	// logSuffix is the suffix for log files.
	logSuffix = ".wal"
)

var (
	emptyEntry = entry(make([]byte, entrySize))
)

type entry []byte

func (e entry) Term() uint64       { return binary.BigEndian.Uint64(e) }
func (e entry) Index() uint64      { return binary.BigEndian.Uint64(e[8:]) }
func (e entry) DataOffset() uint64 { return binary.BigEndian.Uint64(e[16:]) }
func (e entry) Type() uint64       { return binary.BigEndian.Uint64(e[24:]) }

func marshalEntry(b []byte, term, index, do, typ uint64) {
	x.AssertTrue(len(b) == entrySize)

	binary.BigEndian.PutUint64(b, term)
	binary.BigEndian.PutUint64(b[8:], index)
	binary.BigEndian.PutUint64(b[16:], do)
	binary.BigEndian.PutUint64(b[24:], typ)
}

// logFile represents a single log file.
type logFile struct {
	*z.MmapFile
	fid int64

	registry *badger.KeyRegistry
	dataKey  *pb.DataKey
	baseIV   []byte
}

func logFname(dir string, id int64) string {
	return path.Join(dir, fmt.Sprintf("%05d%s", id, logSuffix))
}

// openLogFile opens a logFile in the given directory. The filename is
// constructed based on the value of fid.
func openLogFile(dir string, fid int64) (*logFile, error) {
	var err error
	lf := &logFile{
		fid: fid,
	}
	krOpt := badger.KeyRegistryOptions{
		ReadOnly:                      false,
		Dir:                           dir,
		EncryptionKey:                 x.WorkerConfig.EncryptionKey,
		EncryptionKeyRotationDuration: 10 * 24 * time.Hour,
		InMemory:                      false,
	}
	if lf.registry, err = badger.OpenKeyRegistry(krOpt); err != nil {
		glog.Fatalf("Failed to open KeyRegistry: %v\n", err)
	}

	glog.V(2).Infof("opening log file: %d\n", fid)
	fpath := logFname(dir, fid)
	// Open the file in read-write mode and create it if it doesn't exist yet.
	lf.MmapFile, err = z.OpenMmapFile(fpath, os.O_RDWR|os.O_CREATE, logFileSize)

	if err == z.NewFile {
		glog.V(2).Infof("New file: %d\n", fid)
		z.ZeroOut(lf.Data, 0, logFileOffset)
		if err = lf.bootstrap(); err != nil {
			glog.Fatalf("Failed to bootstrap logfile: %v\n", err)
		}
	} else if err != nil {
		x.Check(err)
	} else {
		buf := make([]byte, 20)
		copy(buf, lf.Data[logFileSize-20:logFileSize])

		keyID := binary.BigEndian.Uint64(buf[:8])
		var dk *pb.DataKey
		// retrieve datakey from the keyID of the logfile.
		if dk, err = lf.registry.DataKey(keyID); err != nil {
			glog.Fatalf("Failed to read DataKey: %v\n", err)
		}
		lf.dataKey = dk
		lf.baseIV = buf[8:]
		fmt.Println(buf)
		y.AssertTrue(len(lf.baseIV) == 12)
	}
	return lf, nil
}

// getEntry gets the entry at the slot idx.
func (lf *logFile) getEntry(idx int) entry {
	if lf == nil {
		return emptyEntry
	}
	x.AssertTrue(idx < maxNumEntries)
	offset := idx * entrySize
	return entry(lf.Data[offset : offset+entrySize])
}

// GetRaftEntry gets the entry at the index idx, reads the data from the appropriate
// offset and converts it to a raftpb.Entry object.
func (lf *logFile) GetRaftEntry(idx int) raftpb.Entry {
	entry := lf.getEntry(idx)
	re := raftpb.Entry{
		Term:  entry.Term(),
		Index: entry.Index(),
		Type:  raftpb.EntryType(int32(entry.Type())),
	}
	if entry.DataOffset() > 0 && entry.DataOffset() < logFileSize {
		data := lf.Slice(int(entry.DataOffset()))
		if len(data) > 0 {
			// Copy the data over to allow the mmaped file to be deleted later.
			re.Data = append(re.Data, data...)
		}
	}
	return re
}

// firstIndex returns the first index in the file.
func (lf *logFile) firstIndex() uint64 {
	return lf.getEntry(0).Index()
}

// firstEmptySlot returns the index of the first empty slot in the file.
func (lf *logFile) firstEmptySlot() int {
	return sort.Search(maxNumEntries, func(i int) bool {
		e := lf.getEntry(i)
		return e.Index() == 0
	})
}

// lastEntry returns the last valid entry in the file.
func (lf *logFile) lastEntry() entry {
	// This would return the first pos, where e.Index() == 0.
	pos := lf.firstEmptySlot()
	if pos > 0 {
		pos--
	}
	return lf.getEntry(pos)
}

// slotGe would return -1 if raftIndex < firstIndex in this file.
// Would return maxNumEntries if raftIndex > lastIndex in this file.
// If raftIndex is found, or the entryFile has empty slots, the offset would be between
// [0, maxNumEntries).
func (lf *logFile) slotGe(raftIndex uint64) int {
	fi := lf.firstIndex()
	// If first index is zero or the first index is less than raftIndex, this
	// raftindex should be in a previous file.
	if fi == 0 || raftIndex < fi {
		return -1
	}

	// Look at the entry at slot diff. If the log has entries for all indices between
	// fi and raftIndex without any gaps, the entry should be there. This is an
	// optimization to avoid having to perform the search below.
	if diff := int(raftIndex - fi); diff < maxNumEntries && diff >= 0 {
		e := lf.getEntry(diff)
		if e.Index() == raftIndex {
			return diff
		}
	}

	// Find the first entry which has in index >= to raftIndex.
	return sort.Search(maxNumEntries, func(i int) bool {
		e := lf.getEntry(i)
		if e.Index() == 0 {
			// We reached too far to the right and found an empty slot.
			return true
		}
		return e.Index() >= raftIndex
	})
}

// delete unmaps and deletes the file.
func (lf *logFile) delete() error {
	glog.V(2).Infof("Deleting file: %s\n", lf.Fd.Name())
	err := lf.Delete()
	if err != nil {
		glog.Errorf("while deleting file: %s, error: %v\n", lf.Fd.Name(), err)
	}
	return err
}

// getLogFiles returns all the log files in the directory sorted by the first
// index in each file.
func getLogFiles(dir string) ([]*logFile, error) {
	entryFiles := x.WalkPathFunc(dir, func(path string, isDir bool) bool {
		if isDir {
			return false
		}
		if strings.HasSuffix(path, logSuffix) {
			return true
		}
		return false
	})

	var files []*logFile
	seen := make(map[int64]struct{})

	for _, fpath := range entryFiles {
		_, fname := path.Split(fpath)
		fname = strings.TrimSuffix(fname, logSuffix)

		fid, err := strconv.ParseInt(fname, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "while parsing: %s", fpath)
		}

		if _, ok := seen[fid]; ok {
			glog.Fatalf("Entry file with id: %d is repeated", fid)
		}
		seen[fid] = struct{}{}

		f, err := openLogFile(dir, fid)
		if err != nil {
			return nil, err
		}
		glog.Infof("Found file: %d First Index: %d\n", fid, f.firstIndex())
		files = append(files, f)
	}

	// Sort files by the first index they store.
	sort.Slice(files, func(i, j int) bool {
		return files[i].getEntry(0).Index() < files[j].getEntry(0).Index()
	})
	return files, nil
}

// KeyID returns datakey's ID.
func (lf *logFile) keyID() uint64 {
	if lf.dataKey == nil {
		// If there is no datakey, then we'll return 0. Which means no encryption.
		return 0
	}
	return lf.dataKey.KeyId
}

// bootstrap will initialize the log file with key id and baseIV.
// The below figure shows the layout of log file.
// +----------------+------------------+------------------+
// | keyID(8 bytes) |  baseIV(12 bytes)|	 entry...     |
// +----------------+------------------+------------------+
func (lf *logFile) bootstrap() error {
	var err error
	if _, err = lf.Fd.Seek(0, io.SeekStart); err != nil {
		return y.Wrapf(err, "Error while SeekStart for the logfile %d in logFile.bootstarp", lf.fid)
	}

	// generate data key for the log file.
	if lf.dataKey, err = lf.registry.LatestDataKey(); err != nil {
		return y.Wrapf(err, "Error while retrieving datakey in logFile.bootstarp")
	}
	buf := make([]byte, 20)
	binary.BigEndian.PutUint64(buf[:8], lf.keyID())
	if _, err := cryptorand.Read(buf[8:]); err != nil {
		return y.Wrapf(err, "Error while creating base IV, while creating logfile")
	}
	// Initialize base IV.
	lf.baseIV = buf[8:]
	lf.Fd.WriteAt(buf, logFileSize-20)
	return nil
}
