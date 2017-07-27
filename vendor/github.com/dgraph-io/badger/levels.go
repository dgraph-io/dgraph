/*
 * Copyright 2017 Dgraph Labs, Inc. and Contributors
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

package badger

import (
	"encoding/binary"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger/table"
	"github.com/dgraph-io/badger/y"
	"github.com/pkg/errors"
)

type levelsController struct {
	elog trace.EventLog

	// The following are initialized once and const.
	levels []*levelHandler
	clog   compactLog
	kv     *KV

	// Atomic.
	maxFileID    uint64 // Next ID to be used.
	maxCompactID uint64

	// For ending compactions.
	compactWorkersDone chan struct{}
	compactWorkersWg   sync.WaitGroup

	cstatus compactStatus
}

var (
	// This is for getting timings between stalls.
	lastUnstalled time.Time
)

func newLevelsController(kv *KV) (*levelsController, error) {
	y.AssertTrue(kv.opt.NumLevelZeroTablesStall > kv.opt.NumLevelZeroTables)
	s := &levelsController{
		kv:     kv,
		elog:   kv.elog,
		levels: make([]*levelHandler, kv.opt.MaxLevels),
	}
	s.cstatus.levels = make([]*levelCompactStatus, kv.opt.MaxLevels)

	for i := 0; i < kv.opt.MaxLevels; i++ {
		s.levels[i] = newLevelHandler(kv, i)
		if i == 0 {
			// Do nothing.
		} else if i == 1 {
			// Level 1 probably shouldn't be too much bigger than level 0.
			s.levels[i].maxTotalSize = kv.opt.LevelOneSize
		} else {
			s.levels[i].maxTotalSize = s.levels[i-1].maxTotalSize * int64(kv.opt.LevelSizeMultiplier)
		}
		s.cstatus.levels[i] = new(levelCompactStatus)
	}

	// Replay compact log. Check against files in directory.
	clogName := filepath.Join(kv.opt.Dir, "clog")
	_, err := os.Stat(clogName)
	if err == nil {
		kv.elog.Printf("Replaying compact log: %s\n", clogName)
		compactLogReplay(clogName, kv.opt.Dir, getIDMap(kv.opt.Dir))

		if err := os.Remove(clogName); err != nil { // Everything is ok. Clear compact log.
			return nil, errors.Wrapf(err, "Removing compaction log: %q", clogName)
		}
	}

	// Some files may be deleted. Let's reload.
	tables := make([][]*table.Table, kv.opt.MaxLevels)
	for fileID := range getIDMap(kv.opt.Dir) {
		fname := table.NewFilename(fileID, kv.opt.Dir)
		fd, err := y.OpenSyncedFile(fname, true)
		if err != nil {
			closeAllUnmodifiedTables(tables)
			return nil, errors.Wrapf(err, "Opening file: %q", fname)
		}

		t, err := table.OpenTable(fd, kv.opt.MapTablesTo)
		if err != nil {
			closeAllUnmodifiedTables(tables)
			return nil, errors.Wrapf(err, "Opening table: %q", fname)
		}

		// Check metadata for level information.
		tableMeta := t.Metadata()
		y.AssertTruef(len(tableMeta) == 2, "len(tableMeta). Expected=2. Actual=%d", len(tableMeta))

		level := int(binary.BigEndian.Uint16(tableMeta))
		y.AssertTruef(level < kv.opt.MaxLevels, "max(level). Expected=%d. Actual=%d",
			kv.opt.MaxLevels, level)
		tables[level] = append(tables[level], t)

		if fileID > s.maxFileID {
			s.maxFileID = fileID
		}
	}
	s.maxFileID++
	for i, tbls := range tables {
		s.levels[i].initTables(tbls)
	}

	// Make sure key ranges do not overlap etc.
	if err := s.validate(); err != nil {
		_ = s.cleanupLevels()
		return nil, errors.Wrap(err, "Level validation")
	}

	// Create new compact log.
	if err := s.clog.init(clogName); err != nil {
		_ = s.cleanupLevels()
		return nil, errors.Wrap(err, "Compaction Log")
	}

	// Sync directory (because we have at least removed/created compaction log files)
	if err := syncDir(kv.opt.Dir); err != nil {
		_ = s.close()
		return nil, errors.Wrap(err, "Directory entry for compaction log")
	}

	return s, nil
}

// Closes the tables, for cleanup in newLevelsController.  (We Close() instead of using DecrRef()
// because that would delete the underlying files.)  We ignore errors when closing the table, which
// is OK because we haven't modified the table.  (It's an LSM tree, but we do hypothetically modify
// the file with SetMetadata.)  (Even then, we open the files with O_DSYNC, so the only plausible
// error on Close() would have been from writing file metadata.)
func closeAllUnmodifiedTables(tables [][]*table.Table) {
	for _, tableSlice := range tables {
		for _, table := range tableSlice {
			_ = table.Close()
		}
	}
}

func (s *levelsController) cleanupLevels() error {
	var firstErr error
	for _, l := range s.levels {
		if err := l.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *levelsController) startCompact(lc *y.LevelCloser) {
	n := s.kv.opt.NumCompactors
	lc.AddRunning(int32(n - 1))
	for i := 0; i < n; i++ {
		go s.runWorker(lc)
	}
}

func (s *levelsController) runWorker(lc *y.LevelCloser) {
	defer lc.Done()
	if s.kv.opt.DoNotCompact {
		return
	}

	time.Sleep(time.Duration(rand.Int31n(1000)) * time.Millisecond)
	timeChan := time.Tick(time.Second)

	for {
		select {
		// Can add a done channel or other stuff.
		case <-timeChan:
			prios := s.pickCompactLevels()
			for _, p := range prios {
				if s.doCompact(p) {
					break
				}
			}
		case <-lc.HasBeenClosed():
			return
		}
	}
}

type compactionPriority struct {
	level int
	score float64
}

// pickCompactLevel determines which level to compact. Return -1 if not found.
// Based on: https://github.com/facebook/rocksdb/wiki/Leveled-Compaction
func (s *levelsController) pickCompactLevels() (prios []compactionPriority) {
	if !s.cstatus.overlapsWith(0, infRange) && // already being compacted.
		s.levels[0].numTables() >= s.kv.opt.NumLevelZeroTables {
		pri := compactionPriority{
			level: 0,
			score: float64(s.levels[0].numTables()) / float64(s.kv.opt.NumLevelZeroTables),
		}
		prios = append(prios, pri)
	}

	for i, l := range s.levels[1:] {
		// Don't consider those tables that are being compacted right now.
		delSize := s.cstatus.delSize(i + 1)

		if l.getTotalSize()-delSize >= l.maxTotalSize {
			pri := compactionPriority{
				level: i + 1,
				score: float64(l.getTotalSize()-delSize) / float64(l.maxTotalSize),
			}
			prios = append(prios, pri)
		}
	}
	sort.Slice(prios, func(i, j int) bool {
		return prios[i].score > prios[j].score
	})
	return prios
}

// compactBuildTables merge topTables and botTables to form a list of new tables.
func (s *levelsController) compactBuildTables(
	l int, cd compactDef, c *compaction) ([]*table.Table, func() error) {

	topTables := cd.top
	botTables := cd.bot

	// Create iterators across all the tables involved first.
	var iters []y.Iterator
	if l == 0 {
		iters = appendIteratorsReversed(iters, topTables, false)
	} else {
		y.AssertTrue(len(topTables) == 1)
		iters = []y.Iterator{topTables[0].NewIterator(false)}
	}

	// Next level has level>=1 and we can use ConcatIterator as key ranges do not overlap.
	iters = append(iters, table.NewConcatIterator(botTables, false))
	it := y.NewMergeIterator(iters, false)
	defer it.Close() // Important to close the iterator to do ref counting.

	it.Rewind()

	// Start generating new tables.
	newTables := make([]*table.Table, len(c.toInsert))
	che := make(chan error, len(c.toInsert))
	var i int
	newIDMin, newIDMax := c.toInsert[0], c.toInsert[len(c.toInsert)-1]
	newID := newIDMin
	for ; it.Valid(); i++ {
		y.AssertTruef(i < len(newTables), "Rewriting too many tables: %d %d", i, len(newTables))
		timeStart := time.Now()
		builder := table.NewTableBuilder()
		for ; it.Valid(); it.Next() {
			if builder.ReachedCapacity(s.kv.opt.MaxTableSize) {
				break
			}
			y.Check(builder.Add(it.Key(), it.Value()))
		}
		// It was true that it.Valid() at least once in the loop above, which means we
		// called Add() at least once, and builder is not Empty().
		y.AssertTrue(!builder.Empty())

		cd.elog.LazyPrintf("LOG Compact. Iteration to generate one table took: %v\n", time.Since(timeStart))

		y.AssertTruef(newID <= newIDMax, "%d %d", newID, newIDMax)
		go func(idx int, fileID uint64, builder *table.TableBuilder) {
			defer builder.Close()

			fd, err := y.OpenSyncedFile(table.NewFilename(fileID, s.kv.opt.Dir), true)
			if err != nil {
				che <- errors.Wrapf(err, "While opening new table: %d", fileID)
				return
			}

			// Encode the level number as table metadata.
			var levelNum [2]byte
			binary.BigEndian.PutUint16(levelNum[:], uint16(l+1))
			if _, err := fd.Write(builder.Finish(levelNum[:])); err != nil {
				che <- errors.Wrapf(err, "Unable to write to file: %d", fileID)
				return
			}

			newTables[idx], err = table.OpenTable(fd, s.kv.opt.MapTablesTo)
			// decrRef is added below.
			che <- errors.Wrapf(err, "Unable to open table: %q", fd.Name())

		}(i, newID, builder)
		newID++
	}

	// Wait for all table builders to finish.
	var firstErr error
	for x := 0; x < i; x++ {
		if err := <-che; err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if firstErr == nil {
		// Ensure created files' directory entries are visible.  We don't mind the extra latency
		// from not doing this ASAP after all file creation has finished because this is a
		// background operation.
		firstErr = syncDir(s.kv.opt.Dir)
	}

	if firstErr != nil {
		// An error happened.  Delete all the newly created table files (by calling DecrRef
		// -- we're the only holders of a ref).
		for _, table := range newTables[:i] {
			if table != nil {
				_ = table.DecrRef()
			}
		}
		errorReturn := errors.Wrapf(firstErr, "While running compaction for: %+v", c)
		return nil, func() error { return errorReturn }
	}

	out := newTables[:i]
	return out, func() error {
		for _, t := range out {
			// replaceTables will increment reference.
			if err := t.DecrRef(); err != nil {
				return err
			}
		}
		return nil
	}
}

type compactDef struct {
	elog trace.Trace

	thisLevel *levelHandler
	nextLevel *levelHandler

	top []*table.Table
	bot []*table.Table

	thisRange keyRange
	nextRange keyRange

	thisSize int64
}

func (cd *compactDef) lockLevels() {
	cd.thisLevel.RLock()
	cd.nextLevel.RLock()
}

func (cd *compactDef) unlockLevels() {
	cd.nextLevel.RUnlock()
	cd.thisLevel.RUnlock()
}

func (s *levelsController) fillTablesL0(cd *compactDef) bool {
	cd.lockLevels()
	defer cd.unlockLevels()

	cd.top = make([]*table.Table, len(cd.thisLevel.tables))
	copy(cd.top, cd.thisLevel.tables)
	if len(cd.top) == 0 {
		return false
	}
	cd.thisRange = infRange

	kr := getKeyRange(cd.top)
	left, right := cd.nextLevel.overlappingTables(kr)
	cd.bot = make([]*table.Table, right-left)
	copy(cd.bot, cd.nextLevel.tables[left:right])

	if len(cd.bot) == 0 {
		cd.nextRange = kr
	} else {
		cd.nextRange = getKeyRange(cd.bot)
	}

	if !s.cstatus.compareAndAdd(*cd) {
		return false
	}

	return true
}

func (s *levelsController) fillTables(cd *compactDef) bool {
	cd.lockLevels()
	defer cd.unlockLevels()

	tbls := make([]*table.Table, len(cd.thisLevel.tables))
	copy(tbls, cd.thisLevel.tables)
	if len(tbls) == 0 {
		return false
	}

	// Find the biggest table, and compact that first.
	// TODO: Try other table picking strategies.
	sort.Slice(tbls, func(i, j int) bool {
		return tbls[i].Size() > tbls[j].Size()
	})

	for _, t := range tbls {
		cd.thisSize = t.Size()
		cd.thisRange = keyRange{
			left:  t.Smallest(),
			right: t.Biggest(),
		}
		if s.cstatus.overlapsWith(cd.thisLevel.level, cd.thisRange) {
			continue
		}
		cd.top = []*table.Table{t}
		left, right := cd.nextLevel.overlappingTables(cd.thisRange)

		cd.bot = make([]*table.Table, right-left)
		copy(cd.bot, cd.nextLevel.tables[left:right])

		if len(cd.bot) == 0 {
			cd.bot = []*table.Table{}
			cd.nextRange = cd.thisRange
			if !s.cstatus.compareAndAdd(*cd) {
				continue
			}
			return true
		}
		cd.nextRange = getKeyRange(cd.bot)

		if s.cstatus.overlapsWith(cd.nextLevel.level, cd.nextRange) {
			continue
		}

		if !s.cstatus.compareAndAdd(*cd) {
			continue
		}
		return true
	}
	return false
}

func (s *levelsController) runCompactDef(l int, cd compactDef) {
	timeStart := time.Now()
	var readSize int64
	for _, tbl := range cd.top {
		readSize += tbl.Size()
	}
	for _, tbl := range cd.bot {
		readSize += tbl.Size()
	}

	thisLevel := cd.thisLevel
	nextLevel := cd.nextLevel

	if thisLevel.level >= 1 && len(cd.bot) == 0 {
		y.AssertTrue(len(cd.top) == 1)

		nextLevel.replaceTables(cd.top)
		thisLevel.deleteTables(cd.top)

		tbl := cd.top[0]
		tbl.UpdateLevel(l + 1)
		cd.elog.LazyPrintf("\tLOG Compact-Move %d->%d smallest:%s biggest:%s took %v\n",
			l, l+1, string(tbl.Smallest()), string(tbl.Biggest()), time.Since(timeStart))
		return
	}

	c := s.buildCompactionLogEntry(&cd)
	//			if s.kv.opt.Verbose {
	//				y.Printf("Compact start: %v\n", c)
	//			}
	s.clog.add(c)
	newTables, decr := s.compactBuildTables(l, cd, c)
	if newTables == nil {
		err := decr()
		// This compaction couldn't be done successfully.
		cd.elog.LazyPrintf("\tLOG Compact FAILED with error: %+v: %+v %+v", err, cd, c)
		return
	}
	defer decr()

	nextLevel.replaceTables(newTables)
	thisLevel.deleteTables(cd.top) // Function will acquire level lock.

	// Note: For level 0, while doCompact is running, it is possible that new tables are added.
	// However, the tables are added only to the end, so it is ok to just delete the first table.

	// Write to compact log.
	c.done = 1
	s.clog.add(c)

	cd.elog.LazyPrintf("LOG Compact %d->%d, del %d tables, add %d tables, took %v\n",
		l, l+1, len(cd.top)+len(cd.bot), len(newTables), time.Since(timeStart))
}

// doCompact picks some table on level l and compacts it away to the next level.
func (s *levelsController) doCompact(p compactionPriority) bool {
	l := p.level
	y.AssertTrue(l+1 < s.kv.opt.MaxLevels) // Sanity check.

	cd := compactDef{
		elog:      trace.New("Badger", "Compact"),
		thisLevel: s.levels[l],
		nextLevel: s.levels[l+1],
	}
	cd.elog.SetMaxEvents(100)
	defer cd.elog.Finish()

	cd.elog.LazyPrintf("Got compaction priority: %+v", p)

	// While picking tables to be compacted, both levels' tables are expected to
	// remain unchanged.
	if l == 0 {
		if !s.fillTablesL0(&cd) {
			cd.elog.LazyPrintf("fillTables failed for level: %d\n", l)
			return false
		}

	} else {
		if !s.fillTables(&cd) {
			cd.elog.LazyPrintf("fillTables failed for level: %d\n", l)
			return false
		}
	}

	cd.elog.LazyPrintf("Running for level: %d\n", cd.thisLevel.level)
	s.cstatus.toLog(cd.elog)
	s.runCompactDef(l, cd)

	// Done with compaction. So, remove the ranges from compaction status.
	s.cstatus.delete(cd)
	s.cstatus.toLog(cd.elog)
	cd.elog.LazyPrintf("Compaction for level: %d DONE", cd.thisLevel.level)
	return true
}

func (s *levelsController) addLevel0Table(t *table.Table) {
	for !s.levels[0].tryAddLevel0Table(t) {
		// Stall. Make sure all levels are healthy before we unstall.
		var timeStart time.Time
		{
			s.elog.Printf("STALLED STALLED STALLED STALLED STALLED STALLED STALLED STALLED: %v\n",
				time.Since(lastUnstalled))
			s.cstatus.RLock()
			for i := 0; i < s.kv.opt.MaxLevels; i++ {
				s.elog.Printf("level=%d. Status=%s Size=%d\n",
					i, s.cstatus.levels[i].debug(), s.levels[i].getTotalSize())
			}
			s.cstatus.RUnlock()
			timeStart = time.Now()
		}
		// Before we unstall, we need to make sure that level 0 and 1 are healthy. Otherwise, we
		// will very quickly fill up level 0 again and if the compaction strategy favors level 0,
		// then level 1 is going to super full.
		for {
			// fmt.Printf("level zero size=%d\n", s.levels[0].getTotalSize())
			// fmt.Printf("level one size=%d/%d\n", s.levels[1].getTotalSize(), s.levels[1].maxTotalSize)
			if s.levels[0].getTotalSize() == 0 && s.levels[1].getTotalSize() < s.levels[1].maxTotalSize {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		{
			s.elog.Printf("UNSTALLED UNSTALLED UNSTALLED UNSTALLED UNSTALLED UNSTALLED: %v\n",
				time.Since(timeStart))
			lastUnstalled = time.Now()
		}
	}
}

func (s *levelsController) close() error {
	cleanupErr := s.cleanupLevels()
	err := s.clog.close()
	if cleanupErr != nil {
		err = cleanupErr
	}
	return errors.Wrap(err, "levelsController.Close")
}

// get returns the found value if any. If not found, we return nil.
func (s *levelsController) get(key []byte) (y.ValueStruct, error) {
	// No need to lock anything as we just iterate over the currently immutable levelHandlers.
	for _, h := range s.levels {
		vs, err := h.get(key)
		if err != nil {
			return y.ValueStruct{}, errors.Wrapf(err, "get key: %q", key)
		}
		if vs.Value == nil && vs.Meta == 0 {
			continue
		}
		return vs, nil
	}
	return y.ValueStruct{}, nil
}

func appendIteratorsReversed(out []y.Iterator, th []*table.Table, reversed bool) []y.Iterator {
	for i := len(th) - 1; i >= 0; i-- {
		// This will increment the reference of the table handler.
		out = append(out, th[i].NewIterator(reversed))
	}
	return out
}

// appendIterators appends iterators to an array of iterators, for merging.
// Note: This obtains references for the table handlers. Remember to close these iterators.
func (s *levelsController) appendIterators(
	iters []y.Iterator, reversed bool) []y.Iterator {
	for _, level := range s.levels {
		iters = level.appendIterators(iters, reversed)
	}
	return iters
}
