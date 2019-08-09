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
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/badger/table"
	"github.com/dgraph-io/badger/y"
	"github.com/pkg/errors"
)

type levelsController struct {
	nextFileID uint64 // Atomic
	elog       trace.EventLog

	// The following are initialized once and const.
	levels []*levelHandler
	kv     *DB

	cstatus compactStatus
}

var (
	// This is for getting timings between stalls.
	lastUnstalled time.Time
)

// revertToManifest checks that all necessary table files exist and removes all table files not
// referenced by the manifest.  idMap is a set of table file id's that were read from the directory
// listing.
func revertToManifest(kv *DB, mf *Manifest, idMap map[uint64]struct{}) error {
	// 1. Check all files in manifest exist.
	for id := range mf.Tables {
		if _, ok := idMap[id]; !ok {
			return fmt.Errorf("file does not exist for table %d", id)
		}
	}

	// 2. Delete files that shouldn't exist.
	for id := range idMap {
		if _, ok := mf.Tables[id]; !ok {
			kv.elog.Printf("Table file %d not referenced in MANIFEST\n", id)
			filename := table.NewFilename(id, kv.opt.Dir)
			if err := os.Remove(filename); err != nil {
				return y.Wrapf(err, "While removing table %d", id)
			}
		}
	}

	return nil
}

func newLevelsController(db *DB, mf *Manifest) (*levelsController, error) {
	y.AssertTrue(db.opt.NumLevelZeroTablesStall > db.opt.NumLevelZeroTables)
	s := &levelsController{
		kv:     db,
		elog:   db.elog,
		levels: make([]*levelHandler, db.opt.MaxLevels),
	}
	s.cstatus.levels = make([]*levelCompactStatus, db.opt.MaxLevels)

	for i := 0; i < db.opt.MaxLevels; i++ {
		s.levels[i] = newLevelHandler(db, i)
		if i == 0 {
			// Do nothing.
		} else if i == 1 {
			// Level 1 probably shouldn't be too much bigger than level 0.
			s.levels[i].maxTotalSize = db.opt.LevelOneSize
		} else {
			s.levels[i].maxTotalSize = s.levels[i-1].maxTotalSize * int64(db.opt.LevelSizeMultiplier)
		}
		s.cstatus.levels[i] = new(levelCompactStatus)
	}

	// Compare manifest against directory, check for existent/non-existent files, and remove.
	if err := revertToManifest(db, mf, getIDMap(db.opt.Dir)); err != nil {
		return nil, err
	}

	// Some files may be deleted. Let's reload.
	var flags uint32 = y.Sync
	if db.opt.ReadOnly {
		flags |= y.ReadOnly
	}

	var mu sync.Mutex
	tables := make([][]*table.Table, db.opt.MaxLevels)
	var maxFileID uint64

	// We found that using 3 goroutines allows disk throughput to be utilized to its max.
	// Disk utilization is the main thing we should focus on, while trying to read the data. That's
	// the one factor that remains constant between HDD and SSD.
	throttle := y.NewThrottle(3)

	start := time.Now()
	var numOpened int32
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()

	for fileID, tf := range mf.Tables {
		fname := table.NewFilename(fileID, db.opt.Dir)
		select {
		case <-tick.C:
			db.opt.Infof("%d tables out of %d opened in %s\n", atomic.LoadInt32(&numOpened),
				len(mf.Tables), time.Since(start).Round(time.Millisecond))
		default:
		}
		if err := throttle.Do(); err != nil {
			closeAllTables(tables)
			return nil, err
		}
		if fileID > maxFileID {
			maxFileID = fileID
		}
		go func(fname string, tf TableManifest) {
			var rerr error
			defer func() {
				throttle.Done(rerr)
				atomic.AddInt32(&numOpened, 1)
			}()
			fd, err := y.OpenExistingFile(fname, flags)
			if err != nil {
				rerr = errors.Wrapf(err, "Opening file: %q", fname)
				return
			}

			opts := table.Options{
				LoadingMode: db.opt.TableLoadingMode,
				ChkMode:     db.opt.ChecksumVerificationMode,
			}
			t, err := table.OpenTable(fd, opts)
			if err != nil {
				if strings.HasPrefix(err.Error(), "CHECKSUM_MISMATCH:") {
					db.opt.Errorf(err.Error())
					db.opt.Errorf("Ignoring table %s", fd.Name())
					// Do not set rerr. We will continue without this table.
				} else {
					rerr = errors.Wrapf(err, "Opening table: %q", fname)
				}
				return
			}

			mu.Lock()
			tables[tf.Level] = append(tables[tf.Level], t)
			mu.Unlock()
		}(fname, tf)
	}
	if err := throttle.Finish(); err != nil {
		closeAllTables(tables)
		return nil, err
	}
	db.opt.Infof("All %d tables opened in %s\n", atomic.LoadInt32(&numOpened),
		time.Since(start).Round(time.Millisecond))
	s.nextFileID = maxFileID + 1
	for i, tbls := range tables {
		s.levels[i].initTables(tbls)
	}

	// Make sure key ranges do not overlap etc.
	if err := s.validate(); err != nil {
		_ = s.cleanupLevels()
		return nil, errors.Wrap(err, "Level validation")
	}

	// Sync directory (because we have at least removed some files, or previously created the
	// manifest file).
	if err := syncDir(db.opt.Dir); err != nil {
		_ = s.close()
		return nil, err
	}

	return s, nil
}

// Closes the tables, for cleanup in newLevelsController.  (We Close() instead of using DecrRef()
// because that would delete the underlying files.)  We ignore errors, which is OK because tables
// are read-only.
func closeAllTables(tables [][]*table.Table) {
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

// dropTree picks all tables from all levels, creates a manifest changeset,
// applies it, and then decrements the refs of these tables, which would result
// in their deletion.
func (s *levelsController) dropTree() (int, error) {
	// First pick all tables, so we can create a manifest changelog.
	var all []*table.Table
	for _, l := range s.levels {
		l.RLock()
		all = append(all, l.tables...)
		l.RUnlock()
	}
	if len(all) == 0 {
		return 0, nil
	}

	// Generate the manifest changes.
	changes := []*pb.ManifestChange{}
	for _, table := range all {
		changes = append(changes, newDeleteChange(table.ID()))
	}
	changeSet := pb.ManifestChangeSet{Changes: changes}
	if err := s.kv.manifest.addChanges(changeSet.Changes); err != nil {
		return 0, err
	}

	// Now that manifest has been successfully written, we can delete the tables.
	for _, l := range s.levels {
		l.Lock()
		l.totalSize = 0
		l.tables = l.tables[:0]
		l.Unlock()
	}
	for _, table := range all {
		if err := table.DecrRef(); err != nil {
			return 0, err
		}
	}
	return len(all), nil
}

// dropPrefix runs a L0->L1 compaction, and then runs same level compaction on the rest of the
// levels. For L0->L1 compaction, it runs compactions normally, but skips over all the keys with the
// provided prefix. For Li->Li compactions, it picks up the tables which would have the prefix. The
// tables who only have keys with this prefix are quickly dropped. The ones which have other keys
// are run through MergeIterator and compacted to create new tables. All the mechanisms of
// compactions apply, i.e. level sizes and MANIFEST are updated as in the normal flow.
func (s *levelsController) dropPrefix(prefix []byte) error {
	opt := s.kv.opt
	for _, l := range s.levels {
		l.RLock()
		if l.level == 0 {
			size := len(l.tables)
			l.RUnlock()

			if size > 0 {
				cp := compactionPriority{
					level: 0,
					score: 1.74,
					// A unique number greater than 1.0 does two things. Helps identify this
					// function in logs, and forces a compaction.
					dropPrefix: prefix,
				}
				if err := s.doCompact(cp); err != nil {
					opt.Warningf("While compacting level 0: %v", err)
					return nil
				}
			}
			continue
		}

		var tables []*table.Table
		for _, table := range l.tables {
			var absent bool
			switch {
			case bytes.HasPrefix(table.Smallest(), prefix):
			case bytes.HasPrefix(table.Biggest(), prefix):
			case bytes.Compare(prefix, table.Smallest()) > 0 &&
				bytes.Compare(prefix, table.Biggest()) < 0:
			default:
				absent = true
			}
			if !absent {
				tables = append(tables, table)
			}
		}
		l.RUnlock()
		if len(tables) == 0 {
			continue
		}

		cd := compactDef{
			elog:       trace.New(fmt.Sprintf("Badger.L%d", l.level), "Compact"),
			thisLevel:  l,
			nextLevel:  l,
			top:        []*table.Table{},
			bot:        tables,
			dropPrefix: prefix,
		}
		if err := s.runCompactDef(l.level, cd); err != nil {
			opt.Warningf("While running compact def: %+v. Error: %v", cd, err)
			return err
		}
	}
	return nil
}

func (s *levelsController) startCompact(lc *y.Closer) {
	n := s.kv.opt.NumCompactors
	lc.AddRunning(n - 1)
	for i := 0; i < n; i++ {
		go s.runWorker(lc)
	}
}

func (s *levelsController) runWorker(lc *y.Closer) {
	defer lc.Done()

	randomDelay := time.NewTimer(time.Duration(rand.Int31n(1000)) * time.Millisecond)
	select {
	case <-randomDelay.C:
	case <-lc.HasBeenClosed():
		randomDelay.Stop()
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		// Can add a done channel or other stuff.
		case <-ticker.C:
			prios := s.pickCompactLevels()
			for _, p := range prios {
				if err := s.doCompact(p); err == nil {
					break
				} else if err == errFillTables {
					// pass
				} else {
					s.kv.opt.Warningf("While running doCompact: %v\n", err)
				}
			}
		case <-lc.HasBeenClosed():
			return
		}
	}
}

// Returns true if level zero may be compacted, without accounting for compactions that already
// might be happening.
func (s *levelsController) isLevel0Compactable() bool {
	return s.levels[0].numTables() >= s.kv.opt.NumLevelZeroTables
}

// Returns true if the non-zero level may be compacted.  delSize provides the size of the tables
// which are currently being compacted so that we treat them as already having started being
// compacted (because they have been, yet their size is already counted in getTotalSize).
func (l *levelHandler) isCompactable(delSize int64) bool {
	return l.getTotalSize()-delSize >= l.maxTotalSize
}

type compactionPriority struct {
	level      int
	score      float64
	dropPrefix []byte
}

// pickCompactLevel determines which level to compact.
// Based on: https://github.com/facebook/rocksdb/wiki/Leveled-Compaction
func (s *levelsController) pickCompactLevels() (prios []compactionPriority) {
	// This function must use identical criteria for guaranteeing compaction's progress that
	// addLevel0Table uses.

	// cstatus is checked to see if level 0's tables are already being compacted
	if !s.cstatus.overlapsWith(0, infRange) && s.isLevel0Compactable() {
		pri := compactionPriority{
			level: 0,
			score: float64(s.levels[0].numTables()) / float64(s.kv.opt.NumLevelZeroTables),
		}
		prios = append(prios, pri)
	}

	for i, l := range s.levels[1:] {
		// Don't consider those tables that are already being compacted right now.
		delSize := s.cstatus.delSize(i + 1)

		if l.isCompactable(delSize) {
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
	lev int, cd compactDef) ([]*table.Table, func() error, error) {
	topTables := cd.top
	botTables := cd.bot

	var hasOverlap bool
	{
		kr := getKeyRange(cd.top...)
		for i, lh := range s.levels {
			if i <= lev { // Skip upper levels.
				continue
			}
			lh.RLock()
			left, right := lh.overlappingTables(levelHandlerRLocked{}, kr)
			lh.RUnlock()
			if right-left > 0 {
				hasOverlap = true
				break
			}
		}
	}

	// Try to collect stats so that we can inform value log about GC. That would help us find which
	// value log file should be GCed.
	discardStats := make(map[uint32]int64)
	updateStats := func(vs y.ValueStruct) {
		if vs.Meta&bitValuePointer > 0 {
			var vp valuePointer
			vp.Decode(vs.Value)
			discardStats[vp.Fid] += int64(vp.Len)
		}
	}

	// Create iterators across all the tables involved first.
	var iters []y.Iterator
	if lev == 0 {
		iters = appendIteratorsReversed(iters, topTables, false)
	} else if len(topTables) > 0 {
		y.AssertTrue(len(topTables) == 1)
		iters = []y.Iterator{topTables[0].NewIterator(false)}
	}

	// Next level has level>=1 and we can use ConcatIterator as key ranges do not overlap.
	var valid []*table.Table
	for _, table := range botTables {
		if len(cd.dropPrefix) > 0 &&
			bytes.HasPrefix(table.Smallest(), cd.dropPrefix) &&
			bytes.HasPrefix(table.Biggest(), cd.dropPrefix) {
			// All the keys in this table have the dropPrefix. So, this table does not need to be
			// in the iterator and can be dropped immediately.
			continue
		}
		valid = append(valid, table)
	}
	iters = append(iters, table.NewConcatIterator(valid, false))
	it := y.NewMergeIterator(iters, false)
	defer it.Close() // Important to close the iterator to do ref counting.

	it.Rewind()

	// Pick a discard ts, so we can discard versions below this ts. We should
	// never discard any versions starting from above this timestamp, because
	// that would affect the snapshot view guarantee provided by transactions.
	discardTs := s.kv.orc.discardAtOrBelow()

	// Start generating new tables.
	type newTableResult struct {
		table *table.Table
		err   error
	}
	resultCh := make(chan newTableResult)
	var numBuilds, numVersions int
	var lastKey, skipKey []byte
	for it.Valid() {
		timeStart := time.Now()
		bopts := table.Options{
			BlockSize:         s.kv.opt.BlockSize,
			BloomFalsePostive: s.kv.opt.BloomFalsePositive,
		}
		builder := table.NewTableBuilder(bopts)
		var numKeys, numSkips uint64
		for ; it.Valid(); it.Next() {
			// See if we need to skip the prefix.
			if len(cd.dropPrefix) > 0 && bytes.HasPrefix(it.Key(), cd.dropPrefix) {
				numSkips++
				updateStats(it.Value())
				continue
			}

			// See if we need to skip this key.
			if len(skipKey) > 0 {
				if y.SameKey(it.Key(), skipKey) {
					numSkips++
					updateStats(it.Value())
					continue
				} else {
					skipKey = skipKey[:0]
				}
			}

			if !y.SameKey(it.Key(), lastKey) {
				if builder.ReachedCapacity(s.kv.opt.MaxTableSize) {
					// Only break if we are on a different key, and have reached capacity. We want
					// to ensure that all versions of the key are stored in the same sstable, and
					// not divided across multiple tables at the same level.
					break
				}
				lastKey = y.SafeCopy(lastKey, it.Key())
				numVersions = 0
			}

			vs := it.Value()
			version := y.ParseTs(it.Key())
			// Do not discard entries inserted by merge operator. These entries will be
			// discarded once they're merged
			if version <= discardTs && vs.Meta&bitMergeEntry == 0 {
				// Keep track of the number of versions encountered for this key. Only consider the
				// versions which are below the minReadTs, otherwise, we might end up discarding the
				// only valid version for a running transaction.
				numVersions++
				lastValidVersion := vs.Meta&bitDiscardEarlierVersions > 0
				if isDeletedOrExpired(vs.Meta, vs.ExpiresAt) ||
					numVersions > s.kv.opt.NumVersionsToKeep ||
					lastValidVersion {
					// If this version of the key is deleted or expired, skip all the rest of the
					// versions. Ensure that we're only removing versions below readTs.
					skipKey = y.SafeCopy(skipKey, it.Key())

					if lastValidVersion {
						// Add this key. We have set skipKey, so the following key versions
						// would be skipped.
					} else if hasOverlap {
						// If this key range has overlap with lower levels, then keep the deletion
						// marker with the latest version, discarding the rest. We have set skipKey,
						// so the following key versions would be skipped.
					} else {
						// If no overlap, we can skip all the versions, by continuing here.
						numSkips++
						updateStats(vs)
						continue // Skip adding this key.
					}
				}
			}
			numKeys++
			y.Check(builder.Add(it.Key(), it.Value()))
		}
		// It was true that it.Valid() at least once in the loop above, which means we
		// called Add() at least once, and builder is not Empty().
		s.kv.opt.Debugf("LOG Compact. Added %d keys. Skipped %d keys. Iteration took: %v",
			numKeys, numSkips, time.Since(timeStart))
		build := func(fileID uint64) (*table.Table, error) {
			fd, err := y.CreateSyncedFile(table.NewFilename(fileID, s.kv.opt.Dir), true)
			if err != nil {
				return nil, errors.Wrapf(err, "While opening new table: %d", fileID)
			}

			if _, err := fd.Write(builder.Finish()); err != nil {
				return nil, errors.Wrapf(err, "Unable to write to file: %d", fileID)
			}

			opts := table.Options{
				LoadingMode: s.kv.opt.TableLoadingMode,
				ChkMode:     s.kv.opt.ChecksumVerificationMode,
			}
			tbl, err := table.OpenTable(fd, opts)
			// decrRef is added below.
			return tbl, errors.Wrapf(err, "Unable to open table: %q", fd.Name())
		}
		if builder.Empty() {
			continue
		}
		numBuilds++
		fileID := s.reserveFileID()
		go func(builder *table.Builder) {
			defer builder.Close()
			tbl, err := build(fileID)
			resultCh <- newTableResult{tbl, err}
		}(builder)
	}

	newTables := make([]*table.Table, 0, 20)
	// Wait for all table builders to finish.
	var firstErr error
	for x := 0; x < numBuilds; x++ {
		res := <-resultCh
		newTables = append(newTables, res.table)
		if firstErr == nil {
			firstErr = res.err
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
		for j := 0; j < numBuilds; j++ {
			if newTables[j] != nil {
				_ = newTables[j].DecrRef()
			}
		}
		errorReturn := errors.Wrapf(firstErr, "While running compaction for: %+v", cd)
		return nil, nil, errorReturn
	}

	sort.Slice(newTables, func(i, j int) bool {
		return y.CompareKeys(newTables[i].Biggest(), newTables[j].Biggest()) < 0
	})
	if err := s.kv.vlog.updateDiscardStats(discardStats); err != nil {
		return nil, nil, errors.Wrap(err, "failed to update discard stats")
	}
	s.kv.opt.Debugf("Discard stats: %v", discardStats)
	return newTables, func() error { return decrRefs(newTables) }, nil
}

func buildChangeSet(cd *compactDef, newTables []*table.Table) pb.ManifestChangeSet {
	changes := []*pb.ManifestChange{}
	for _, table := range newTables {
		changes = append(changes,
			newCreateChange(table.ID(), cd.nextLevel.level))
	}
	for _, table := range cd.top {
		changes = append(changes, newDeleteChange(table.ID()))
	}
	for _, table := range cd.bot {
		changes = append(changes, newDeleteChange(table.ID()))
	}
	return pb.ManifestChangeSet{Changes: changes}
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

	dropPrefix []byte
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

	kr := getKeyRange(cd.top...)
	left, right := cd.nextLevel.overlappingTables(levelHandlerRLocked{}, kr)
	cd.bot = make([]*table.Table, right-left)
	copy(cd.bot, cd.nextLevel.tables[left:right])

	if len(cd.bot) == 0 {
		cd.nextRange = kr
	} else {
		cd.nextRange = getKeyRange(cd.bot...)
	}

	if !s.cstatus.compareAndAdd(thisAndNextLevelRLocked{}, *cd) {
		return false
	}

	return true
}

// sortByOverlap sorts tables in increasing order of overlap with next level.
func (s *levelsController) sortByOverlap(tables []*table.Table, cd *compactDef) {
	if len(tables) == 0 || cd.nextLevel == nil {
		return
	}

	tableOverlap := make([]int, len(tables))
	for i := range tables {
		// get key range for table
		tableRange := getKeyRange(tables[i])
		// get overlap with next level
		left, right := cd.nextLevel.overlappingTables(levelHandlerRLocked{}, tableRange)
		tableOverlap[i] = right - left
	}

	sort.Slice(tables, func(i, j int) bool {
		return tableOverlap[i] < tableOverlap[j]
	})
}

func (s *levelsController) fillTables(cd *compactDef) bool {
	cd.lockLevels()
	defer cd.unlockLevels()

	tables := make([]*table.Table, len(cd.thisLevel.tables))
	copy(tables, cd.thisLevel.tables)
	if len(tables) == 0 {
		return false
	}

	// We want to pick files from current level in order of increasing overlap with next level
	// tables. Idea here is to first compact file from current level which has least overlap with
	// next level. This provides us better write amplification.
	s.sortByOverlap(tables, cd)

	for _, t := range tables {
		cd.thisSize = t.Size()
		cd.thisRange = getKeyRange(t)
		if s.cstatus.overlapsWith(cd.thisLevel.level, cd.thisRange) {
			continue
		}
		cd.top = []*table.Table{t}
		left, right := cd.nextLevel.overlappingTables(levelHandlerRLocked{}, cd.thisRange)

		cd.bot = make([]*table.Table, right-left)
		copy(cd.bot, cd.nextLevel.tables[left:right])

		if len(cd.bot) == 0 {
			cd.bot = []*table.Table{}
			cd.nextRange = cd.thisRange
			if !s.cstatus.compareAndAdd(thisAndNextLevelRLocked{}, *cd) {
				continue
			}
			return true
		}
		cd.nextRange = getKeyRange(cd.bot...)

		if s.cstatus.overlapsWith(cd.nextLevel.level, cd.nextRange) {
			continue
		}
		if !s.cstatus.compareAndAdd(thisAndNextLevelRLocked{}, *cd) {
			continue
		}
		return true
	}
	return false
}

func (s *levelsController) runCompactDef(l int, cd compactDef) (err error) {
	timeStart := time.Now()

	thisLevel := cd.thisLevel
	nextLevel := cd.nextLevel

	// Table should never be moved directly between levels, always be rewritten to allow discarding
	// invalid versions.

	newTables, decr, err := s.compactBuildTables(l, cd)
	if err != nil {
		return err
	}
	defer func() {
		// Only assign to err, if it's not already nil.
		if decErr := decr(); err == nil {
			err = decErr
		}
	}()
	changeSet := buildChangeSet(&cd, newTables)

	// We write to the manifest _before_ we delete files (and after we created files)
	if err := s.kv.manifest.addChanges(changeSet.Changes); err != nil {
		return err
	}

	// See comment earlier in this function about the ordering of these ops, and the order in which
	// we access levels when reading.
	if err := nextLevel.replaceTables(cd.bot, newTables); err != nil {
		return err
	}
	if err := thisLevel.deleteTables(cd.top); err != nil {
		return err
	}

	// Note: For level 0, while doCompact is running, it is possible that new tables are added.
	// However, the tables are added only to the end, so it is ok to just delete the first table.

	s.kv.opt.Infof("LOG Compact %d->%d, del %d tables, add %d tables, took %v\n",
		thisLevel.level, nextLevel.level, len(cd.top)+len(cd.bot),
		len(newTables), time.Since(timeStart))
	return nil
}

var errFillTables = errors.New("Unable to fill tables")

// doCompact picks some table on level l and compacts it away to the next level.
func (s *levelsController) doCompact(p compactionPriority) error {
	l := p.level
	y.AssertTrue(l+1 < s.kv.opt.MaxLevels) // Sanity check.

	cd := compactDef{
		elog:       trace.New(fmt.Sprintf("Badger.L%d", l), "Compact"),
		thisLevel:  s.levels[l],
		nextLevel:  s.levels[l+1],
		dropPrefix: p.dropPrefix,
	}
	cd.elog.SetMaxEvents(100)
	defer cd.elog.Finish()

	s.kv.opt.Infof("Got compaction priority: %+v", p)

	// While picking tables to be compacted, both levels' tables are expected to
	// remain unchanged.
	if l == 0 {
		if !s.fillTablesL0(&cd) {
			return errFillTables
		}

	} else {
		if !s.fillTables(&cd) {
			return errFillTables
		}
	}
	defer s.cstatus.delete(cd) // Remove the ranges from compaction status.

	s.kv.opt.Infof("Running for level: %d\n", cd.thisLevel.level)
	s.cstatus.toLog(cd.elog)
	if err := s.runCompactDef(l, cd); err != nil {
		// This compaction couldn't be done successfully.
		s.kv.opt.Warningf("LOG Compact FAILED with error: %+v: %+v", err, cd)
		return err
	}

	s.cstatus.toLog(cd.elog)
	s.kv.opt.Infof("Compaction for level: %d DONE", cd.thisLevel.level)
	return nil
}

func (s *levelsController) addLevel0Table(t *table.Table) error {
	// We update the manifest _before_ the table becomes part of a levelHandler, because at that
	// point it could get used in some compaction.  This ensures the manifest file gets updated in
	// the proper order. (That means this update happens before that of some compaction which
	// deletes the table.)
	err := s.kv.manifest.addChanges([]*pb.ManifestChange{
		newCreateChange(t.ID(), 0),
	})
	if err != nil {
		return err
	}

	for !s.levels[0].tryAddLevel0Table(t) {
		// Stall. Make sure all levels are healthy before we unstall.
		var timeStart time.Time
		{
			s.elog.Printf("STALLED STALLED STALLED: %v\n", time.Since(lastUnstalled))
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
		for i := 0; ; i++ {
			// Passing 0 for delSize to compactable means we're treating incomplete compactions as
			// not having finished -- we wait for them to finish.  Also, it's crucial this behavior
			// replicates pickCompactLevels' behavior in computing compactability in order to
			// guarantee progress.
			if !s.isLevel0Compactable() && !s.levels[1].isCompactable(0) {
				break
			}
			time.Sleep(10 * time.Millisecond)
			if i%100 == 0 {
				prios := s.pickCompactLevels()
				s.elog.Printf("Waiting to add level 0 table. Compaction priorities: %+v\n", prios)
				i = 0
			}
		}
		{
			s.elog.Printf("UNSTALLED UNSTALLED UNSTALLED: %v\n", time.Since(timeStart))
			lastUnstalled = time.Now()
		}
	}

	return nil
}

func (s *levelsController) close() error {
	err := s.cleanupLevels()
	return errors.Wrap(err, "levelsController.Close")
}

// get returns the found value if any. If not found, we return nil.
func (s *levelsController) get(key []byte, maxVs *y.ValueStruct) (y.ValueStruct, error) {
	// It's important that we iterate the levels from 0 on upward.  The reason is, if we iterated
	// in opposite order, or in parallel (naively calling all the h.RLock() in some order) we could
	// read level L's tables post-compaction and level L+1's tables pre-compaction.  (If we do
	// parallelize this, we will need to call the h.RLock() function by increasing order of level
	// number.)
	version := y.ParseTs(key)
	for _, h := range s.levels {
		vs, err := h.get(key) // Calls h.RLock() and h.RUnlock().
		if err != nil {
			return y.ValueStruct{}, errors.Wrapf(err, "get key: %q", key)
		}
		if vs.Value == nil && vs.Meta == 0 {
			continue
		}
		if maxVs == nil || vs.Version == version {
			return vs, nil
		}
		if maxVs.Version < vs.Version {
			*maxVs = vs
		}
	}
	if maxVs != nil {
		return *maxVs, nil
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
	iters []y.Iterator, opt *IteratorOptions) []y.Iterator {
	// Just like with get, it's important we iterate the levels from 0 on upward, to avoid missing
	// data when there's a compaction.
	for _, level := range s.levels {
		iters = level.appendIterators(iters, opt)
	}
	return iters
}

// TableInfo represents the information about a table.
type TableInfo struct {
	ID       uint64
	Level    int
	Left     []byte
	Right    []byte
	KeyCount uint64 // Number of keys in the table
}

func (s *levelsController) getTableInfo(withKeysCount bool) (result []TableInfo) {
	for _, l := range s.levels {
		l.RLock()
		for _, t := range l.tables {
			var count uint64
			if withKeysCount {
				it := t.NewIterator(false)
				for it.Rewind(); it.Valid(); it.Next() {
					count++
				}
			}

			info := TableInfo{
				ID:       t.ID(),
				Level:    l.level,
				Left:     t.Smallest(),
				Right:    t.Biggest(),
				KeyCount: count,
			}
			result = append(result, info)
		}
		l.RUnlock()
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Level != result[j].Level {
			return result[i].Level < result[j].Level
		}
		return result[i].ID < result[j].ID
	})
	return
}

// verifyChecksum verifies checksum for all tables on all levels.
func (s *levelsController) verifyChecksum() error {
	var tables []*table.Table
	for _, l := range s.levels {
		l.RLock()
		tables = tables[:0]
		for _, t := range l.tables {
			tables = append(tables, t)
			t.IncrRef()
		}
		l.RUnlock()

		for _, t := range tables {
			errChkVerify := t.VerifyChecksum()
			if err := t.DecrRef(); err != nil {
				s.kv.opt.Errorf("unable to decrease reference of table: %s while "+
					"verifying checksum with error: %s", t.Filename(), err)
			}

			if errChkVerify != nil {
				return errChkVerify
			}
		}
	}

	return nil
}
