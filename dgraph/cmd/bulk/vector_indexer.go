/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"github.com/dgraph-io/badger/v4"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/tok"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/golang/glog"
)

// vectorIndexer handles building vector indexes during the reduce phase.
// It uses a shared tmpDb for vector data and HNSW graph construction,
// which allows HNSW to use normal read-write transactions.
// The shared DB approach avoids the global pstore race condition.
type vectorIndexer struct {
	*reducer
	tmpDb       *badger.DB                            // Shared vector tmpDb for read-write operations
	indexSpecs  map[string]*pb.VectorIndexSpec        // pred -> vector index spec (all specs)
	indexers    map[string]index.VectorIndex[float32] // pred -> HNSW indexer instance (lazily created)
	txnCaches   map[string]*hnsw.TxnCache             // pred -> transaction cache (lazily created)
	txns        map[string]*posting.Txn               // pred -> posting txn (for CommitToDisk)
	vectorPreds map[string]bool                       // Track predicates with vectors for copy phase
	dimensions  map[string]int                        // Track expected dimensions per predicate

	// For tracking predicate → output shard mapping
	shardId           int            // This shard's ID
	predToShardMu     *sync.Mutex    // Mutex for predToOutputShard (shared across shards)
	predToOutputShard map[string]int // Predicate → output shard mapping (shared across shards)

	// For batched writes of vector posting lists to shared DB
	writeBatch *badger.WriteBatch
	writeCount int

	mu sync.Mutex
}

// vectorEntry represents a single vector to be indexed.
type vectorEntry struct {
	pred   string
	uid    uint64
	vector []float32
}

// vectorEntrySize calculates the size needed to marshal a vectorEntry.
func vectorEntrySize(ve *vectorEntry) int {
	// pred length (4) + pred bytes + uid (8) + vector length (4) + vector data
	return 4 + len(ve.pred) + 8 + 4 + len(ve.vector)*4
}

// marshalVectorEntry serializes a vectorEntry into a byte slice.
func marshalVectorEntry(dst []byte, ve *vectorEntry) {
	offset := 0

	// Write predicate length and bytes
	binary.BigEndian.PutUint32(dst[offset:offset+4], uint32(len(ve.pred)))
	offset += 4
	copy(dst[offset:], ve.pred)
	offset += len(ve.pred)

	// Write UID
	binary.BigEndian.PutUint64(dst[offset:offset+8], ve.uid)
	offset += 8

	// Write vector length
	binary.BigEndian.PutUint32(dst[offset:offset+4], uint32(len(ve.vector)))
	offset += 4

	// Write vector data
	for _, v := range ve.vector {
		binary.BigEndian.PutUint32(dst[offset:offset+4], math.Float32bits(v))
		offset += 4
	}
}

// unmarshalVectorEntry deserializes a vectorEntry from a byte slice.
// Returns nil if the data is malformed or too short.
func unmarshalVectorEntry(data []byte) *vectorEntry {
	// Minimum size: 4 (predLen) + 0 (pred) + 8 (uid) + 4 (vecLen) = 16 bytes
	if len(data) < 16 {
		glog.Errorf("unmarshalVectorEntry: data too short (%d bytes, need at least 16)", len(data))
		return nil
	}

	offset := 0

	// Read predicate length
	predLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	// Validate predicate bounds: need predLen + 8 (uid) + 4 (vecLen)
	if offset+int(predLen)+12 > len(data) {
		glog.Errorf("unmarshalVectorEntry: invalid predLen %d for data size %d", predLen, len(data))
		return nil
	}

	// Read predicate
	pred := string(data[offset : offset+int(predLen)])
	offset += int(predLen)

	// Read UID
	uid := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Read vector length
	vecLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	// Validate vector bounds
	requiredSize := offset + int(vecLen)*4
	if requiredSize > len(data) {
		glog.Errorf("unmarshalVectorEntry: invalid vecLen %d for remaining data (need %d, have %d)",
			vecLen, requiredSize, len(data))
		return nil
	}

	// Read vector data
	vector := make([]float32, vecLen)
	for i := range vector {
		bits := binary.BigEndian.Uint32(data[offset : offset+4])
		vector[i] = math.Float32frombits(bits)
		offset += 4
	}

	return &vectorEntry{
		pred:   pred,
		uid:    uid,
		vector: vector,
	}
}

// newVectorIndexerShared creates a new vectorIndexer for a shard using a shared vectorTmpDb.
// called once before any shards start. Indexers are created lazily when vectors arrive.
func newVectorIndexerShared(r *reducer, sharedVectorDb *badger.DB, indexSpecs map[string]*pb.VectorIndexSpec,
	shardId int, predToShardMu *sync.Mutex, predToOutputShard map[string]int) *vectorIndexer {

	vi := &vectorIndexer{
		reducer:           r,
		tmpDb:             sharedVectorDb,
		indexSpecs:        indexSpecs,
		indexers:          make(map[string]index.VectorIndex[float32]),
		txnCaches:         make(map[string]*hnsw.TxnCache),
		txns:              make(map[string]*posting.Txn),
		vectorPreds:       make(map[string]bool),
		dimensions:        make(map[string]int),
		shardId:           shardId,
		predToShardMu:     predToShardMu,
		predToOutputShard: predToOutputShard,
	}
	glog.Infof("Vector indexer created for shard %d (lazy initialization, %d potential predicates)",
		shardId, len(indexSpecs))

	return vi
}

// getOrCreateIndexer lazily creates an HNSW indexer for a predicate.
// This is called when the first vector for a predicate arrives.
func (vi *vectorIndexer) getOrCreateIndexer(pred string) (index.VectorIndex[float32], *hnsw.TxnCache, error) {
	vi.mu.Lock()
	defer vi.mu.Unlock()

	if indexer, ok := vi.indexers[pred]; ok {
		return indexer, vi.txnCaches[pred], nil
	}

	// Check if this predicate has a vector index spec
	spec, ok := vi.indexSpecs[pred]
	if !ok {
		return nil, nil, fmt.Errorf("no vector index spec for predicate %s", pred)
	}

	// Create the HNSW indexer
	factorySpec, err := tok.GetFactoryCreateSpecFromSpec(spec)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting factory spec for %s: %w", pred, err)
	}

	indexer, err := factorySpec.CreateIndex(pred)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating indexer for %s: %w", pred, err)
	}

	// Create transaction cache
	txn := posting.NewTxn(vi.state.writeTs)
	viTxn := posting.NewViTxn(txn)
	tc := hnsw.NewTxnCache(viTxn, vi.state.writeTs)

	vi.indexers[pred] = indexer
	vi.txnCaches[pred] = tc
	vi.txns[pred] = txn
	vi.vectorPreds[pred] = true

	glog.Infof("Lazily initialized HNSW indexer for predicate %s (shard %d)", pred, vi.shardId)

	return indexer, tc, nil
}

// isVectorPredicate returns true if the given predicate has a vector index.
func (vi *vectorIndexer) isVectorPredicate(pred string) bool {
	_, ok := vi.indexSpecs[pred]
	return ok
}

const batchFlushThreshold = 1000 // Flush write batch after this many entries

// writeVectorKV writes a single KV entry to the vectorTmpDb.
// This is used for writing vector posting lists during the reduce phase.
// Writes are batched for efficiency and flushed periodically.
func (vi *vectorIndexer) writeVectorKV(kv *bpb.KV) error {
	if kv == nil {
		return fmt.Errorf("writeVectorKV: received nil KV")
	}

	vi.mu.Lock()
	defer vi.mu.Unlock()

	// Lazy initialize write batch
	if vi.writeBatch == nil {
		vi.writeBatch = vi.tmpDb.NewManagedWriteBatch()
		vi.writeCount = 0
	}

	// Determine UserMeta - default to complete posting
	userMeta := byte(posting.BitCompletePosting)
	if len(kv.UserMeta) > 0 {
		userMeta = kv.UserMeta[0]
	}

	// Create badger entry from KV
	entry := &badger.Entry{
		Key:      kv.Key,
		Value:    kv.Value,
		UserMeta: userMeta,
	}

	if err := vi.writeBatch.SetEntryAt(entry, kv.Version); err != nil {
		// Cancel batch and reset state on error to prevent leaks
		vi.writeBatch.Cancel()
		vi.writeBatch = nil
		vi.writeCount = 0
		return fmt.Errorf("error writing vector KV to tmpDb: %w", err)
	}

	vi.writeCount++

	// Flush periodically
	if vi.writeCount >= batchFlushThreshold {
		if err := vi.writeBatch.Flush(); err != nil {
			// Cancel batch and reset state on error
			vi.writeBatch.Cancel()
			vi.writeBatch = nil
			vi.writeCount = 0
			return fmt.Errorf("error flushing vector write batch: %w", err)
		}
		vi.writeBatch = vi.tmpDb.NewManagedWriteBatch()
		vi.writeCount = 0
	}

	return nil
}

// flushWriteBatch flushes any remaining entries in the write batch.
func (vi *vectorIndexer) flushWriteBatch() error {
	vi.mu.Lock()
	defer vi.mu.Unlock()

	if vi.writeBatch != nil {
		if vi.writeCount > 0 {
			if err := vi.writeBatch.Flush(); err != nil {
				vi.writeBatch.Cancel()
				vi.writeBatch = nil
				vi.writeCount = 0
				return fmt.Errorf("error flushing final vector write batch: %w", err)
			}
		} else {
			// No entries to flush, just cancel
			vi.writeBatch.Cancel()
		}
		vi.writeBatch = nil
		vi.writeCount = 0
	}
	return nil
}

// validateVectorDimension checks that the vector has the expected dimension for the predicate.
// If this is the first vector for the predicate, it sets the expected dimension.
// Returns true if the vector is valid, false otherwise.
func (vi *vectorIndexer) validateVectorDimension(pred string, vector []float32, uid uint64) bool {
	vi.mu.Lock()
	defer vi.mu.Unlock()

	// Empty vectors are invalid
	if len(vector) == 0 {
		glog.Warningf("Empty vector for predicate %s uid %d, skipping", pred, uid)
		return false
	}

	// Check dimension consistency
	expectedDim, exists := vi.dimensions[pred]
	if !exists {
		// First vector for this predicate - set the expected dimension
		vi.dimensions[pred] = len(vector)
		return true
	}

	if len(vector) != expectedDim {

		glog.Errorf("Dimension mismatch for predicate %s uid %d: expected %d, got %d",
			pred, uid, expectedDim, len(vector))
		return false
	}

	return true
}

// addVectorEntry processes a single vector entry by inserting it into the HNSW index.
// This is the entry point called from reduce phase.
// It validates the vector dimension, creates indexer lazily, and handles errors gracefully.
func (vi *vectorIndexer) addVectorEntry(ve *vectorEntry) {
	if ve == nil {
		glog.Errorf("addVectorEntry: received nil vectorEntry, skipping")
		return
	}

	if ve.uid == 0 {
		glog.Errorf("addVectorEntry: INVALID UID=0 for pred=%s, skipping", ve.pred)
		return
	}
	if ve.uid == math.MaxUint64 {
		glog.Errorf("addVectorEntry: INVALID UID=MaxUint64 for pred=%s, skipping", ve.pred)
		return
	}

	// Validate vector dimension
	if !vi.validateVectorDimension(ve.pred, ve.vector, ve.uid) {
		return
	}

	indexer, tc, err := vi.getOrCreateIndexer(ve.pred)
	if err != nil {
		glog.Errorf("Error getting indexer for %s: %v", ve.pred, err)
		return
	}

	// Track predicate → shard mapping (for copy phase)
	// INVARIANT: Each predicate should only appear in ONE shard
	if vi.predToShardMu != nil && vi.predToOutputShard != nil {
		vi.predToShardMu.Lock()
		if existingShard, exists := vi.predToOutputShard[ve.pred]; exists {
			if existingShard != vi.shardId {
				// This is a serious invariant violation - same predicate in multiple shards
				// This could lead to data corruption as HNSW graph would be split
				glog.Errorf("INVARIANT VIOLATION: predicate %s already assigned to shard %d, "+
					"but shard %d also received vectors for it. Data may be corrupted!",
					ve.pred, existingShard, vi.shardId)
				vi.predToShardMu.Unlock()
				return // Skip this vector to prevent further corruption
			}
		} else {
			vi.predToOutputShard[ve.pred] = vi.shardId
			glog.Infof("Predicate %s assigned to output shard %d", ve.pred, vi.shardId)
		}
		vi.predToShardMu.Unlock()
	}

	// Insert into HNSW index
	_, err = indexer.Insert(context.Background(), tc, ve.uid, ve.vector)
	if err != nil {
		glog.Errorf("Error inserting vector for %s uid %d: %v", ve.pred, ve.uid, err)
		return
	}
}

// wait waits for all vector indexing operations to complete.
// It flushes any remaining writes and commits all HNSW mutations from LocalCache to vectorTmpDb.
func (vi *vectorIndexer) wait() {
	// Flush any remaining vector posting list writes
	if err := vi.flushWriteBatch(); err != nil {
		glog.Errorf("Error flushing write batch: %v", err)
	}

	vi.mu.Lock()
	defer vi.mu.Unlock()

	// If no indexers were created (no vectors arrived), nothing to commit
	if len(vi.txns) == 0 {
		glog.Infof("Vector indexer wait completed for shard %d (no vectors processed)", vi.shardId)
		return
	}

	// Commit HNSW mutations from LocalCache to vectorTmpDb
	// This is critical - without this, HNSW edge data stays in memory and never gets persisted
	// Step 1: Call Update() to move mutations from plists to deltas
	// Step 2: Call CommitToDisk() to write deltas to BadgerDB
	writer := posting.NewTxnWriter(vi.tmpDb)
	for pred, txn := range vi.txns {
		// Update moves mutations from posting lists to delta cache
		txn.Update()
		if err := txn.CommitToDisk(writer, vi.state.writeTs); err != nil {
			glog.Errorf("Error committing HNSW data for predicate %s: %v", pred, err)
		} else {
			glog.Infof("Committed HNSW data for predicate %s to tmpDb (shard %d)", pred, vi.shardId)
		}
	}
	if err := writer.Flush(); err != nil {
		glog.Errorf("Error flushing HNSW data to tmpDb: %v", err)
	}

	glog.Infof("Vector indexer wait completed for shard %d with %d predicates", vi.shardId, len(vi.vectorPreds))
}
