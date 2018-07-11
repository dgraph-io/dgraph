/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

// Package posting takes care of posting lists. It contains logic for mutation
// layers, merging them with BadgerDB, etc.
//
// Q. How should we do posting list rollups?
// Posting lists are stored such that we have a immutable state, and mutations on top of it. In
// Dgraph, we only store committed mutations to disk, uncommitted mutations are just kept in memory.
// Thus, we have the immutable state + mutations as different versions in Badger.
// Periodically, we roll up the mutations into the immutable state, to make reads faster. The
// question is when should we do these rollups.
//
// We can do rollups when snapshots happen. At that time, we can first iterate over all the posting
// lists in memory, and call rollup(snapshotTs) on them. Then, we can do key-only iteration over
// Badger, and call rollup(snapshotTs) on any posting list which doesn't have full state as the
// first version.
//
// Any request for snapshot can then be served directly by only reading entries until snapshotTs.
// This would be simple to understand, and consistent within the entire Raft group.
// We can also get rid of SyncIfDirty calls, because all commits are already stored on disk.
package posting
