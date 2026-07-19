/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"context"
	"encoding/binary"
	"sync/atomic"

	ostats "go.opencensus.io/stats"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/tok"
	"github.com/dgraph-io/dgraph/v25/x"
)

// BM25Posting is a single materialized entry of a BM25 term posting list: the
// document UID together with the term frequency and document length decoded from
// the posting value.
type BM25Posting struct {
	Uid    uint64
	TF     uint32
	DocLen uint32
}

// ReadBM25TermPostings materializes the postings of a BM25 term's standard index
// list at readTs into a UID-ascending slice, decoding (tf, docLen) from each
// posting value. getList reads a posting list for a key (e.g. LocalCache.Get),
// keeping the value encoding encapsulated in this package.
func ReadBM25TermPostings(getList func(key []byte) (*List, error), attr, encodedTerm string,
	readTs uint64) ([]BM25Posting, error) {
	key := x.BM25IndexKey(attr, encodedTerm)
	pl, err := getList(key)
	if err != nil {
		return nil, err
	}
	var out []BM25Posting
	err = pl.Iterate(readTs, 0, func(p *pb.Posting) error {
		tf, docLen, ok := decodeBM25Value(p.Value)
		if !ok {
			// Corrupt/truncated posting value: skip rather than inject a bogus
			// zero-frequency match that would silently distort scoring.
			return nil
		}
		out = append(out, BM25Posting{Uid: p.Uid, TF: tf, DocLen: docLen})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// NumBM25StatsBuckets is the number of buckets the BM25 corpus statistics (document
// count and total term count) are sharded across, keyed by uid%NumBM25StatsBuckets.
// Sharding spreads the read-modify-write contention of stats maintenance across
// independent posting lists so that concurrent mutations on different documents
// rarely conflict, while same-bucket updates still conflict (and retry) — avoiding
// lost updates. A single hot stats key would serialize all writes to the predicate.
// Exported so the bulk loader buckets corpus statistics identically to the live and
// rebuild paths.
const NumBM25StatsBuckets = 32

// numBM25StatsBuckets is the unexported alias retained for readability within this
// package.
const numBM25StatsBuckets = NumBM25StatsBuckets

// EncodeBM25Value packs a posting's term frequency and document length the same way the
// live index path does, for the bulk loader to write BM25 term postings in the standard
// format. See encodeBM25Value.
func EncodeBM25Value(tf, docLen uint32) []byte { return encodeBM25Value(tf, docLen) }

// EncodeBM25Stats encodes corpus statistics (document count, total term count) for the
// bulk loader to write the per-bucket stats postings in the standard format. See
// encodeBM25Stats.
func EncodeBM25Stats(docCount, totalTerms uint64) []byte {
	return encodeBM25Stats(docCount, totalTerms)
}

// encodeBM25Value packs a posting's term frequency and document length into the
// posting Value as two unsigned varints. Storing the document length alongside the
// term frequency makes scoring read (tf, docLen) in a single posting access — no
// separate document-length list (which would be a write-hot key) and no random
// per-candidate lookup at query time. The document length is duplicated across a
// document's unique terms, but a document's postings are always rewritten together
// on update, so they stay consistent.
func encodeBM25Value(tf, docLen uint32) []byte {
	buf := make([]byte, binary.MaxVarintLen32*2)
	n := binary.PutUvarint(buf, uint64(tf))
	n += binary.PutUvarint(buf[n:], uint64(docLen))
	return buf[:n]
}

// decodeBM25Value reverses encodeBM25Value. ok is false when the input does not
// hold two complete varints (e.g. a truncated or corrupt posting value), so the
// caller can skip it rather than silently scoring it as a zero-frequency match.
func decodeBM25Value(b []byte) (tf, docLen uint32, ok bool) {
	tf64, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, 0, false
	}
	docLen64, m := binary.Uvarint(b[n:])
	if m <= 0 {
		return 0, 0, false
	}
	return uint32(tf64), uint32(docLen64), true
}

// encodeBM25Stats encodes corpus statistics (document count, total term count) as
// two unsigned varints.
func encodeBM25Stats(docCount, totalTerms uint64) []byte {
	buf := make([]byte, binary.MaxVarintLen64*2)
	n := binary.PutUvarint(buf, docCount)
	n += binary.PutUvarint(buf[n:], totalTerms)
	return buf[:n]
}

// decodeBM25Stats reverses encodeBM25Stats. It returns (0, 0) on malformed input.
func decodeBM25Stats(b []byte) (docCount, totalTerms uint64) {
	docCount, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, 0
	}
	totalTerms, m := binary.Uvarint(b[n:])
	if m <= 0 {
		return docCount, 0
	}
	return docCount, totalTerms
}

// addBM25TermPosting writes (op=SET) or removes (op=DEL) the posting for the given
// (term, uid) pair in the term's standard index posting list. On SET the posting's
// Value packs (tf, docLen); on DEL only the UID matters. The posting is a REF
// posting (ValueId set) that carries a Value — List.encode retains such postings
// through rollup (see the len(p.Value) > 0 clause there).
func (txn *Txn) addBM25TermPosting(ctx context.Context, attr, term string, uid uint64,
	tf, docLen uint32, op pb.DirectedEdge_Op) error {
	encodedTerm := string([]byte{tok.IdentBM25}) + term
	key := x.BM25IndexKey(attr, encodedTerm)
	plist, err := txn.cache.GetFromDelta(key)
	if err != nil {
		return err
	}
	edge := &pb.DirectedEdge{
		ValueId: uid,
		Attr:    attr,
		Op:      op,
	}
	if op != pb.DirectedEdge_DEL {
		edge.Value = encodeBM25Value(tf, docLen)
		edge.ValueType = pb.Posting_BINARY
	}
	if err := plist.addMutation(ctx, txn, edge); err != nil {
		return err
	}
	ostats.Record(ctx, x.NumEdges.M(1))
	return nil
}

// bm25StatsAccum is a concurrency-safe per-bucket accumulator of corpus-statistics
// deltas. Index rebuild routes its per-document stats updates here (across many
// goroutines) instead of the read-modify-write counter, then flushes the buckets once
// as a single writer (see flush). This avoids the undercount that the streaming
// rebuild's independent per-thread caches and periodic resets would otherwise cause,
// where last-write-wins on the value posting drops every thread's partial total but
// one.
type bm25StatsAccum struct {
	count [numBM25StatsBuckets]atomic.Int64
	terms [numBM25StatsBuckets]atomic.Int64
}

func newBM25StatsAccum() *bm25StatsAccum { return &bm25StatsAccum{} }

// add records a document's contribution in its bucket (uid%numBM25StatsBuckets).
func (a *bm25StatsAccum) add(uid uint64, docCountDelta, totalTermsDelta int64) {
	bucket := uid % numBM25StatsBuckets
	a.count[bucket].Add(docCountDelta)
	a.terms[bucket].Add(totalTermsDelta)
}

// flush writes the accumulated absolute totals into the stats posting lists for attr
// through txn, one SET value posting per non-empty bucket. It is a single-writer
// operation (one txn writing all buckets), so there is no lost-update window; the
// caller commits txn. Buckets are written as absolute SETs because a rebuild deletes
// the prior stats first, so the buckets start empty.
func (a *bm25StatsAccum) flush(ctx context.Context, txn *Txn, attr string) error {
	for bucket := 0; bucket < numBM25StatsBuckets; bucket++ {
		docCount := a.count[bucket].Load()
		totalTerms := a.terms[bucket].Load()
		if docCount <= 0 && totalTerms <= 0 {
			continue
		}
		key := x.BM25StatsKey(attr, bucket)
		plist, err := txn.cache.GetFromDelta(key)
		if err != nil {
			return err
		}
		edge := &pb.DirectedEdge{
			Attr:      attr,
			Value:     encodeBM25Stats(uint64(docCount), uint64(totalTerms)),
			ValueType: pb.Posting_BINARY,
			Op:        pb.DirectedEdge_SET,
		}
		if err := plist.addMutation(ctx, txn, edge); err != nil {
			return err
		}
	}
	return nil
}

// updateBM25Stats applies (docCountDelta, totalTermsDelta) to the bucketed corpus
// statistics for attr. The bucket is selected by uid%numBM25StatsBuckets. The
// running totals are stored as a single value posting per bucket; the read at
// txn.StartTs sees this transaction's own earlier writes (read-your-own-writes),
// so multiple documents in the same transaction that land in the same bucket
// accumulate correctly.
func (txn *Txn) updateBM25Stats(ctx context.Context, attr string, uid uint64,
	docCountDelta, totalTermsDelta int64) error {
	// During index rebuild, accumulate into the shared accumulator rather than the
	// read-modify-write counter (see Txn.bm25Acc). The rebuild flushes the buckets
	// once at the end as a single writer.
	if txn.bm25Acc != nil {
		txn.bm25Acc.add(uid, docCountDelta, totalTermsDelta)
		return nil
	}
	// Serialize the whole read-modify-write against sibling goroutines applying
	// other edges of the SAME transaction (see Txn.bm25StatsMu): their entities can
	// share this uid%32 bucket, and concurrent RMWs would drop deltas.
	txn.bm25StatsMu.Lock()
	defer txn.bm25StatsMu.Unlock()

	bucket := int(uid % numBM25StatsBuckets)
	key := x.BM25StatsKey(attr, bucket)
	// Stats are maintained by read-modify-write: we must read the committed total
	// from disk (and merge this transaction's own writes), not just the in-memory
	// delta. GetFromDelta skips disk and is only safe for write-only index mutations,
	// so each transaction would otherwise overwrite the bucket instead of
	// accumulating across transactions. Get reads committed state.
	plist, err := txn.cache.Get(key)
	if err != nil {
		return err
	}

	var docCount, totalTerms uint64
	val, err := plist.Value(txn.StartTs)
	switch err {
	case nil:
		if data, ok := val.Value.([]byte); ok {
			docCount, totalTerms = decodeBM25Stats(data)
		}
	case ErrNoValue:
		// No stats yet for this bucket; start from zero.
	default:
		return err
	}

	docCount = applyBM25Delta(docCount, docCountDelta)
	totalTerms = applyBM25Delta(totalTerms, totalTermsDelta)

	edge := &pb.DirectedEdge{
		Attr:      attr,
		Value:     encodeBM25Stats(docCount, totalTerms),
		ValueType: pb.Posting_BINARY,
		Op:        pb.DirectedEdge_SET,
	}
	return plist.addMutation(ctx, txn, edge)
}

// applyBM25Delta adds a signed delta to an unsigned counter, clamping at zero.
func applyBM25Delta(v uint64, delta int64) uint64 {
	if delta >= 0 {
		return v + uint64(delta)
	}
	dec := uint64(-delta)
	if dec > v {
		return 0
	}
	return v - dec
}

// ReadBM25Stats sums the bucketed corpus statistics for attr at readTs, returning
// the document count and total term count. avgDL = totalTerms / docCount. The
// getList closure reads a posting list for a key (e.g. LocalCache.Get) so the
// caller controls caching and the read timestamp.
func ReadBM25Stats(getList func(key []byte) (*List, error), attr string,
	readTs uint64) (docCount, totalTerms uint64, err error) {
	for b := 0; b < numBM25StatsBuckets; b++ {
		key := x.BM25StatsKey(attr, b)
		pl, perr := getList(key)
		if perr != nil {
			return 0, 0, perr
		}
		val, verr := pl.Value(readTs)
		if verr == ErrNoValue {
			continue
		}
		if verr != nil {
			return 0, 0, verr
		}
		data, ok := val.Value.([]byte)
		if !ok || len(data) == 0 {
			continue
		}
		dc, tt := decodeBM25Stats(data)
		docCount += dc
		totalTerms += tt
	}
	return docCount, totalTerms, nil
}
