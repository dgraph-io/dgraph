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
	"encoding/hex"

	"github.com/pkg/errors"
)

var (
	// ErrValueLogSize is returned when opt.ValueLogFileSize option is not within the valid
	// range.
	ErrValueLogSize = errors.New("Invalid ValueLogFileSize, must be between 1MB and 2GB")

	// ErrValueThreshold is returned when ValueThreshold is set to a value close to or greater than
	// uint16.
	ErrValueThreshold = errors.New("Invalid ValueThreshold, must be lower than uint16.")

	// ErrKeyNotFound is returned when key isn't found on a txn.Get.
	ErrKeyNotFound = errors.New("Key not found")

	// ErrTxnTooBig is returned if too many writes are fit into a single transaction.
	ErrTxnTooBig = errors.New("Txn is too big to fit into one request")

	// ErrConflict is returned when a transaction conflicts with another transaction. This can happen if
	// the read rows had been updated concurrently by another transaction.
	ErrConflict = errors.New("Transaction Conflict. Please retry")

	// ErrReadOnlyTxn is returned if an update function is called on a read-only transaction.
	ErrReadOnlyTxn = errors.New("No sets or deletes are allowed in a read-only transaction")

	// ErrDiscardedTxn is returned if a previously discarded transaction is re-used.
	ErrDiscardedTxn = errors.New("This transaction has been discarded. Create a new one")

	// ErrEmptyKey is returned if an empty key is passed on an update function.
	ErrEmptyKey = errors.New("Key cannot be empty")

	// ErrRetry is returned when a log file containing the value is not found.
	// This usually indicates that it may have been garbage collected, and the
	// operation needs to be retried.
	ErrRetry = errors.New("Unable to find log file. Please retry")

	// ErrThresholdZero is returned if threshold is set to zero, and value log GC is called.
	// In such a case, GC can't be run.
	ErrThresholdZero = errors.New(
		"Value log GC can't run because threshold is set to zero")

	// ErrNoRewrite is returned if a call for value log GC doesn't result in a log file rewrite.
	ErrNoRewrite = errors.New(
		"Value log GC attempt didn't result in any cleanup")

	// ErrRejected is returned if a value log GC is called either while another GC is running, or
	// after DB::Close has been called.
	ErrRejected = errors.New("Value log GC request rejected")

	// ErrInvalidRequest is returned if the user request is invalid.
	ErrInvalidRequest = errors.New("Invalid request")

	// ErrManagedTxn is returned if the user tries to use an API which isn't
	// allowed due to external management of transactions, when using ManagedDB.
	ErrManagedTxn = errors.New(
		"Invalid API request. Not allowed to perform this action using ManagedDB")

	// ErrInvalidDump if a data dump made previously cannot be loaded into the database.
	ErrInvalidDump = errors.New("Data dump cannot be read")

	// ErrZeroBandwidth is returned if the user passes in zero bandwidth for sequence.
	ErrZeroBandwidth = errors.New("Bandwidth must be greater than zero")

	// ErrInvalidLoadingMode is returned when opt.ValueLogLoadingMode option is not
	// within the valid range
	ErrInvalidLoadingMode = errors.New("Invalid ValueLogLoadingMode, must be FileIO or MemoryMap")

	// ErrReplayNeeded is returned when opt.ReadOnly is set but the
	// database requires a value log replay.
	ErrReplayNeeded = errors.New("Database was not properly closed, cannot open read-only")

	// ErrWindowsNotSupported is returned when opt.ReadOnly is used on Windows
	ErrWindowsNotSupported = errors.New("Read-only mode is not supported on Windows")

	// ErrTruncateNeeded is returned when the value log gets corrupt, and requires truncation of
	// corrupt data to allow Badger to run properly.
	ErrTruncateNeeded = errors.New("Value log truncate required to run DB. This might result in data loss.")

	// ErrBlockedWrites is returned if the user called DropAll. During the process of dropping all
	// data from Badger, we stop accepting new writes, by returning this error.
	ErrBlockedWrites = errors.New("Writes are blocked possibly due to DropAll")
)

// Key length can't be more than uint16, as determined by table::header.
const maxKeySize = 1<<16 - 8 // 8 bytes are for storing timestamp

func exceedsMaxKeySizeError(key []byte) error {
	return errors.Errorf("Key with size %d exceeded %d limit. Key:\n%s",
		len(key), maxKeySize, hex.Dump(key[:1<<10]))
}

func exceedsMaxValueSizeError(value []byte, maxValueSize int64) error {
	return errors.Errorf("Value with size %d exceeded ValueLogFileSize (%d). Key:\n%s",
		len(value), maxValueSize, hex.Dump(value[:1<<10]))
}
