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

// OpenManaged returns a new DB, which allows more control over setting
// transaction timestamps, aka managed mode.
//
// This is only useful for databases built on top of Badger (like Dgraph), and
// can be ignored by most users.
func OpenManaged(opts Options) (*DB, error) {
	opts.managedTxns = true
	return Open(opts)
}

// NewTransactionAt follows the same logic as DB.NewTransaction(), but uses the
// provided read timestamp.
//
// This is only useful for databases built on top of Badger (like Dgraph), and
// can be ignored by most users.
func (db *DB) NewTransactionAt(readTs uint64, update bool) *Txn {
	if !db.opt.managedTxns {
		panic("Cannot use NewTransactionAt with managedDB=false. Use NewTransaction instead.")
	}
	txn := db.newTransaction(update, true)
	txn.readTs = readTs
	return txn
}

// CommitAt commits the transaction, following the same logic as Commit(), but
// at the given commit timestamp. This will panic if not used with managed transactions.
//
// This is only useful for databases built on top of Badger (like Dgraph), and
// can be ignored by most users.
func (txn *Txn) CommitAt(commitTs uint64, callback func(error)) error {
	if !txn.db.opt.managedTxns {
		panic("Cannot use CommitAt with managedDB=false. Use Commit instead.")
	}
	txn.commitTs = commitTs
	if callback == nil {
		return txn.Commit()
	}
	txn.CommitWith(callback)
	return nil
}

// SetDiscardTs sets a timestamp at or below which, any invalid or deleted
// versions can be discarded from the LSM tree, and thence from the value log to
// reclaim disk space. Can only be used with managed transactions.
func (db *DB) SetDiscardTs(ts uint64) {
	if !db.opt.managedTxns {
		panic("Cannot use SetDiscardTs with managedDB=false.")
	}
	db.orc.setDiscardTs(ts)
}
