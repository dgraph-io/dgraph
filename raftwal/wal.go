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

package raftwal

import (
	"bytes"
	"sort"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"

	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

var errNotFound = errors.New("Unable to find raft entry")

// wal represents the entire entry log. It consists of one or more
// entryFile objects. This object is not lock protected but it's used by
// DiskStorage, which has a lock protecting the calls to this object.
type wal struct {
	// files is the list of all log files ordered in ascending order by the first
	// index in the file. current is the file currently being written to, and is
	// added to files only after it is full.
	files   []*logFile
	current *logFile
	// nextEntryIdx is the index of the next entry to write to. When this value exceeds
	// maxNumEntries the file will be rotated.
	nextEntryIdx int
	// dir is the directory to use to store files.
	dir string
}

// allEntries returns all the entries in the range [lo, hi).
func (l *wal) allEntries(lo, hi, maxSize uint64) []raftpb.Entry {
	var entries []raftpb.Entry
	fileIdx, offset := l.slotGe(lo)
	var size uint64

	if offset < 0 {
		// Start from the beginning of the entry file.
		offset = 0
	}

	currFile := l.getEntryFile(fileIdx)
	for {
		// If offset is greater than maxNumEntries, then we need to move to the next file.
		if offset >= maxNumEntries {
			if fileIdx == -1 {
				// We're already at the latest file. Return the entries we have.
				return entries
			}

			// Move to the next file.
			fileIdx++
			if fileIdx >= len(l.files) {
				// We're beyond the list of files in l.files. Move to the latest file.
				fileIdx = -1
			}
			currFile = l.getEntryFile(fileIdx)
			x.AssertTrue(currFile != nil)

			// Reset the offset to start reading the next file from the beginning.
			offset = 0
		}

		re := currFile.GetRaftEntry(offset)
		// fmt.Printf("Got raft entry: %v\n", re.Index)
		if re.Index >= hi {
			return entries
		}

		if re.Index == 0 {
			// This entry and all the following ones in this file are empty.
			// Setting the offset to maxNumEntries will trigger a move to the next
			// file in the next iteration.
			offset = maxNumEntries
			continue
		}

		size += uint64(re.Size())
		if len(entries) > 0 && size > maxSize {
			break
		}
		entries = append(entries, re)
		offset++
	}
	return entries
}

// truncateEntriesUntil deletes the data field of every raft entry
// of type EntryNormal and index ∈ [0, lastIdx).
func (l *wal) truncateEntriesUntil(lastIdx uint64) {
	files := append(l.files, l.current)
	for _, file := range files {
		if file == nil {
			continue
		}

		for idx := 0; idx < maxNumEntries; idx++ {
			entry := file.getEntry(idx)
			if entry.Index() >= lastIdx {
				return
			}

			// Truncate the data of normal Raft entries.
			// Here, we set the length field of the entry data to zero.
			// As long as we never directly iterate through the data section,
			// this operation is safe.
			// Suppose we truncate indexes [0, 100) and then call AddEntries
			// with indexes [95, 105]. AddEntries will do the following:
			// 1. It will zero out all data at index 95 and above
			// 2. It will find the data offset at index 94 (say 'x')
			// 3. We start writing new data at x+sliceSize(data, x),
			//    which will be x+4 if the entry is of type EntryNormal.
			// Since all entries of index 95 and above have been invalidated,
			// we can be sure that we don't overwrite any useful data.
			if entry.Type() == uint64(raftpb.EntryNormal) {
				offset := int(entry.DataOffset())
				z.ZeroOut(file.Data, offset, offset+4)
			}
		}
	}
}

// AddEntries adds the entries to the log. If there are entries in the log with the same index
// they will be overwritten and the entries after that zeroed out from the log.
func (l *wal) AddEntries(entries []raftpb.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	// glog.Infof("AddEntries: %+v\n", entries)
	fidx, eidx := l.slotGe(entries[0].Index)

	// The first entry in the input is already in the log. We must remove the existing entry
	// and all the entries after from the log.
	if eidx >= 0 {
		if fidx == -1 {
			// The existing entry was found in the current file. We only have to zero out
			// from the entries after the one in which the entry was found.
			if l.nextEntryIdx > eidx {
				z.ZeroOut(l.current.Data, entrySize*eidx, entrySize*l.nextEntryIdx)
			}
		} else {
			// The existing entry was found in one of the previous file.
			// The logic must do the following.
			// 1. Delete all the files after the one in which the entry was found.
			// 2. Zero out all the entries after the slot in which the entry was found
			//    in the file in which it was found.
			// 3. Update the pointer to the current file and the list of previous files.
			x.AssertTrue(fidx < len(l.files))
			extra := l.files[fidx+1:]
			extra = append(extra, l.current)
			l.current = l.files[fidx]

			for _, ef := range extra {
				glog.V(2).Infof("Deleting extra file: %d\n", ef.fid)
				if err := ef.delete(); err != nil {
					glog.Errorf("deleting file: %s. error: %v\n", ef.Fd.Name(), err)
				}
			}
			z.ZeroOut(l.current.Data, entrySize*eidx, logFileOffset)
			l.files = l.files[:fidx]
		}
		l.nextEntryIdx = eidx
	}

	// Look at the previous entry to find the right offset at which to start writing the value of
	// the Data field for each entry.
	prev := l.nextEntryIdx - 1
	var offset int
	if prev >= 0 {
		// There was a previous entry. Retrieve the offset and the size data from that entry to
		// calculate the next offset.
		e := l.current.getEntry(prev)
		offset = int(e.DataOffset())
		offset += sliceSize(l.current.Data, offset)
	} else {
		// At the start of the file so use entryFileOffset.
		offset = logFileOffset
	}

	for _, re := range entries {
		// Write upto maxNumEntries or 1GB, whatever happens first.
		if l.nextEntryIdx >= maxNumEntries || offset+4+len(re.Data) > 1<<30 {
			if err := l.rotate(re.Index, offset); err != nil {
				return err
			}
			l.nextEntryIdx, offset = 0, logFileOffset
		}
		// If encryption is enabled then encrypt the data.
		if l.current.dataKey != nil {
			var ebuf bytes.Buffer
			curr := l.current
			if err := y.XORBlockStream(
				&ebuf, re.Data, curr.dataKey.Data, curr.generateIV(uint64(offset))); err != nil {
				return err
			}
			re.Data = ebuf.Bytes()
		}

		// Allocate slice for the data and copy bytes.
		destBuf, next, err := l.current.AllocateSlice(len(re.Data), offset)
		if err != nil {
			return err
		}
		x.AssertTrue(copy(destBuf, re.Data) == len(re.Data))

		// Write the entry at the given slot.
		buf := l.current.getEntry(l.nextEntryIdx)
		marshalEntry(buf, re.Term, re.Index, uint64(offset), uint64(re.Type))

		// Update values for the next entry.
		offset = next
		l.nextEntryIdx++
	}
	return nil
}

// firstIndex returns the first index available in the entry log.
func (l *wal) firstIndex() uint64 {
	if l == nil {
		return 0
	}
	var fi uint64
	if len(l.files) == 0 {
		fi = l.current.getEntry(0).Index()
	} else {
		fi = l.files[0].getEntry(0).Index()
	}

	// If fi is zero return one because RAFT expects the first index to always
	// be greater than zero.
	if fi == 0 {
		return 1
	}
	return fi
}

// LastIndex returns the last index in the log.
func (l *wal) LastIndex() uint64 {
	if l.nextEntryIdx-1 >= 0 {
		e := l.current.getEntry(l.nextEntryIdx - 1)
		return e.Index()
	}
	for i := len(l.files) - 1; i >= 0; i-- {
		ef := l.files[i]
		e := ef.lastEntry()
		if e.Index() > 0 {
			return e.Index()
		}
	}
	return 0
}

// getEntryFile returns right logFile corresponding to the fidx. A value of -1
// is meant to represent the current file, which is not yet stored in l.files.
func (l *wal) getEntryFile(fidx int) *logFile {
	if fidx == -1 {
		return l.current
	}
	if fidx >= len(l.files) {
		return nil
	}
	return l.files[fidx]
}

// slotGe returns the file index and the slot within that file containing the
// entry with an index greater than equals to the provided raftIndex. A
// value of -1 for the file index means that the entry is in the current file.
// A value of -1 for slot means that the raftIndex is lower than whatever is
// present in the WAL, thus it is not present in the WAL.
func (l *wal) slotGe(raftIndex uint64) (int, int) {
	// Look for the offset in the current file.
	if offset := l.current.slotGe(raftIndex); offset >= 0 {
		return -1, offset
	}

	// No previous files, therefore we can only go back to the start of the current file.
	if len(l.files) == 0 {
		return -1, -1
	}

	fileIdx := sort.Search(len(l.files), func(i int) bool {
		return l.files[i].firstIndex() >= raftIndex
	})
	// fileIdx points to the first log file, whose firstIndex is >= raftIndex.
	// If the firstIndex == raftIndex, then return.
	if fileIdx < len(l.files) && l.files[fileIdx].firstIndex() == raftIndex {
		return fileIdx, 0
	}
	// Otherwise, go back one file to the file which has firstIndex < raftIndex.
	if fileIdx > 0 {
		fileIdx--
	}

	offset := l.files[fileIdx].slotGe(raftIndex)
	return fileIdx, offset
}

// seekEntry returns the entry with the given raftIndex if it exists. If the
// raftIndex is lower than all the entries in the log, raft.ErrCompacted is
// returned. If the raftIndex is higher than all the entries in the log,
// raft.ErrUnavailable is returned. If no match is found, errNotFound is
// returned. Finally, if an entry matches the raftIndex, it is returned.
func (l *wal) seekEntry(raftIndex uint64) (entry, error) {
	if raftIndex == 0 {
		return emptyEntry, nil
	}

	fidx, off := l.slotGe(raftIndex)
	if off == -1 {
		// The entry is not in the log because it was already processed and compacted.
		return emptyEntry, raft.ErrCompacted
	} else if off >= maxNumEntries {
		// The log has not advanced past the given raftIndex.
		return emptyEntry, raft.ErrUnavailable
	}

	ef := l.getEntryFile(fidx)
	ent := ef.getEntry(off)
	if ent.Index() == 0 {
		// The log has not advanced past the given raftIndex.
		return emptyEntry, raft.ErrUnavailable
	}
	if ent.Index() != raftIndex {
		return emptyEntry, errNotFound
	}
	return ent, nil
}

// Term returns the term of entry with raft index = idx. It returns an error if
// a matching entry is not found.
func (l *wal) Term(idx uint64) (uint64, error) {
	ent, err := l.seekEntry(idx)
	return ent.Term(), err
}

// deleteBefore deletes all the files before the logFile containing the given raftIndex.
func (l *wal) deleteBefore(raftIndex uint64) {
	fidx, off := l.slotGe(raftIndex)

	if off < 0 || fidx >= len(l.files) {
		return
	}

	var before []*logFile
	if fidx == -1 { // current file
		before = l.files
		l.files = l.files[:0]
	} else {
		before = l.files[:fidx]
		l.files = l.files[fidx:]
	}

	for _, ef := range before {
		if err := ef.delete(); err != nil {
			glog.Errorf("while deleting file: %s, err: %v\n", ef.Fd.Name(), err)
		}
	}
}

// reset deletes all the previous log files, and resets the current log file.
func (l *wal) reset() error {
	for _, ef := range l.files {
		if err := ef.delete(); err != nil {
			return errors.Wrapf(err, "while deleting %s", ef.Fd.Name())
		}
	}
	l.files = l.files[:0]
	z.ZeroOut(l.current.Data, 0, logFileOffset)
	l.nextEntryIdx = 0
	return nil
}

// Moves the current logFile into l.files and creates a new logFile.
func (l *wal) rotate(firstIndex uint64, offset int) error {
	// Select the name for the new file based on the names of the existing files.
	nextFid := l.current.fid
	x.AssertTrue(nextFid > 0)
	for _, ef := range l.files {
		if ef.fid > nextFid {
			nextFid = ef.fid
		}
	}
	nextFid++

	err := l.current.Truncate(int64(offset))
	if err != nil {
		return errors.Wrapf(err, "while truncating entry file")
	}

	ef, err := openLogFile(l.dir, nextFid)
	if err != nil {
		return errors.Wrapf(err, "while creating a new entry file")
	}

	// Move the existing current file to the end of the list of files and
	// update the current file to the file that was just created.
	l.files = append(l.files, l.current)
	l.current = ef
	return nil
}

func openWal(dir string) (*wal, error) {
	e := &wal{
		dir: dir,
	}
	files, err := getLogFiles(dir)
	if err != nil {
		return nil, err
	}
	out := files[:0]
	var nextFid int64
	for _, ef := range files {
		if nextFid < ef.fid {
			nextFid = ef.fid
		}
		if ef.firstIndex() == 0 {
			if err := ef.delete(); err != nil {
				return nil, err
			}
		} else {
			out = append(out, ef)
		}
	}
	e.files = out
	if sz := len(e.files); sz > 0 {
		e.current = e.files[sz-1]
		e.nextEntryIdx = e.current.firstEmptySlot()

		e.files = e.files[:sz-1]
		return e, nil
	}

	// No files found. Create a new file.
	nextFid += 1
	ef, err := openLogFile(dir, nextFid)
	e.current = ef
	return e, err
}
