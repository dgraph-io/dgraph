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
	"github.com/dgraph-io/badger/options"
)

// NOTE: Keep the comments in the following to 75 chars width, so they
// format nicely in godoc.

// Options are params for creating DB object.
//
// This package provides DefaultOptions which contains options that should
// work for most applications. Consider using that as a starting point before
// customizing it for your own needs.
type Options struct {
	// 1. Mandatory flags
	// -------------------
	// Directory to store the data in. If it doesn't exist, Badger will
	// try to create it for you.
	Dir string
	// Directory to store the value log in. Can be the same as Dir. If it
	// doesn't exist, Badger will try to create it for you.
	ValueDir string

	// 2. Frequently modified flags
	// -----------------------------
	// Sync all writes to disk. Setting this to false would achieve better
	// performance, but may cause data to be lost.
	SyncWrites bool

	// How should LSM tree be accessed.
	TableLoadingMode options.FileLoadingMode

	// How should value log be accessed.
	ValueLogLoadingMode options.FileLoadingMode

	// How many versions to keep per key.
	NumVersionsToKeep int

	// 3. Flags that user might want to review
	// ----------------------------------------
	// The following affect all levels of LSM tree.
	MaxTableSize        int64 // Each table (or file) is at most this size.
	LevelSizeMultiplier int   // Equals SizeOf(Li+1)/SizeOf(Li).
	MaxLevels           int   // Maximum number of levels of compaction.
	// If value size >= this threshold, only store value offsets in tree.
	ValueThreshold int
	// Maximum number of tables to keep in memory, before stalling.
	NumMemtables int
	// The following affect how we handle LSM tree L0.
	// Maximum number of Level 0 tables before we start compacting.
	NumLevelZeroTables int

	// If we hit this number of Level 0 tables, we will stall until L0 is
	// compacted away.
	NumLevelZeroTablesStall int

	// Maximum total size for L1.
	LevelOneSize int64

	// Size of single value log file.
	ValueLogFileSize int64

	// Max number of entries a value log file can hold (approximately). A value log file would be
	// determined by the smaller of its file size and max entries.
	ValueLogMaxEntries uint32

	// Number of compaction workers to run concurrently. Setting this to zero would stop compactions
	// to happen within LSM tree. If set to zero, writes could block forever.
	NumCompactors int

	// When closing the DB, force compact Level 0. This ensures that both reads and writes are
	// efficient when the DB is opened later.
	CompactL0OnClose bool

	// Transaction start and commit timestamps are managed by end-user.
	// This is only useful for databases built on top of Badger (like Dgraph).
	// Not recommended for most users.
	managedTxns bool

	// 4. Flags for testing purposes
	// ------------------------------
	maxBatchCount int64 // max entries in batch
	maxBatchSize  int64 // max batch size in bytes

	// Open the DB as read-only. With this set, multiple processes can
	// open the same Badger DB. Note: if the DB being opened had crashed
	// before and has vlog data to be replayed, ReadOnly will cause Open
	// to fail with an appropriate message.
	ReadOnly bool

	// Truncate value log to delete corrupt data, if any. Would not truncate if ReadOnly is set.
	Truncate bool

	// DB-specific logger which will override the global logger.
	Logger Logger
}

// DefaultOptions sets a list of recommended options for good performance.
// Feel free to modify these to suit your needs.
var DefaultOptions = Options{
	LevelOneSize:        256 << 20,
	LevelSizeMultiplier: 10,
	TableLoadingMode:    options.LoadToRAM,
	ValueLogLoadingMode: options.MemoryMap,
	// table.MemoryMap to mmap() the tables.
	// table.Nothing to not preload the tables.
	MaxLevels:               7,
	MaxTableSize:            64 << 20,
	NumCompactors:           2, // Compactions can be expensive. Only run 2.
	NumLevelZeroTables:      5,
	NumLevelZeroTablesStall: 10,
	NumMemtables:            5,
	SyncWrites:              true,
	NumVersionsToKeep:       1,
	CompactL0OnClose:        true,
	// Nothing to read/write value log using standard File I/O
	// MemoryMap to mmap() the value log files
	// (2^30 - 1)*2 when mmapping < 2^31 - 1, max int32.
	// -1 so 2*ValueLogFileSize won't overflow on 32-bit systems.
	ValueLogFileSize: 1<<30 - 1,

	ValueLogMaxEntries: 1000000,
	ValueThreshold:     32,
	Truncate:           false,
	Logger:             defaultLogger,
}

// LSMOnlyOptions follows from DefaultOptions, but sets a higher ValueThreshold
// so values would be colocated with the LSM tree, with value log largely acting
// as a write-ahead log only. These options would reduce the disk usage of value
// log, and make Badger act more like a typical LSM tree.
var LSMOnlyOptions = Options{}

func init() {
	LSMOnlyOptions = DefaultOptions

	LSMOnlyOptions.ValueThreshold = 65500 // Max value length which fits in uint16.
	// Let's not set any other options, because they can cause issues with the
	// size of key-value a user can pass to Badger. For e.g., if we set
	// ValueLogFileSize to 64MB, a user can't pass a value more than that.
	// Setting it to ValueLogMaxEntries to 1000, can generate too many files.
	// These options are better configured on a usage basis, than broadly here.
	// The ValueThreshold is the most important setting a user needs to do to
	// achieve a heavier usage of LSM tree.
	// NOTE: If a user does not want to set 64KB as the ValueThreshold because
	// of performance reasons, 1KB would be a good option too, allowing
	// values smaller than 1KB to be colocated with the keys in the LSM tree.
}
