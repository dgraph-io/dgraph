/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dgraph-io/badger/v4"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/tok"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/types/facets"
	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func readPostingListFromDisk(key []byte, pstore *badger.DB, readTs uint64) (*posting.List, error) {
	txn := pstore.NewTransactionAt(readTs, false)
	defer txn.Discard()

	// When we do rollups, an older version would go to the top of the LSM tree, which can cause
	// issues during txn.Get. Therefore, always iterate.
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	iterOpts.PrefetchValues = false
	itr := txn.NewKeyIterator(key, iterOpts)
	defer itr.Close()
	itr.Seek(key)
	return posting.ReadPostingList(key, itr)
}

func rollup(t *testing.T, key []byte, pstore *badger.DB, readTs uint64) {
	ol, err := readPostingListFromDisk(key, pstore, readTs)
	require.NoError(t, err)
	kvs, err := ol.Rollup(nil, readTs+1)
	require.NoError(t, err)
	require.NoError(t, writePostingListToDisk(kvs))
	posting.ResetCache()
}

func writePostingListToDisk(kvs []*bpb.KV) error {
	writer := posting.NewTxnWriter(pstore)
	for _, kv := range kvs {
		if err := writer.SetAt(kv.Key, kv.Value, kv.UserMeta[0], kv.Version); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func TestEmptyTypeSchema(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps, 0, false)
	Init(ps)

	typeName := "1-temp"
	ts := uint64(10)
	txn := pstore.NewTransactionAt(ts, true)
	defer txn.Discard()
	e := &badger.Entry{
		Key:      x.TypeKey(typeName),
		Value:    make([]byte, 0),
		UserMeta: posting.BitSchemaPosting,
	}
	require.Nil(t, txn.SetEntry(e.WithDiscard()))
	require.Nil(t, txn.CommitAt(ts, nil))

	schema.Init(ps)
	require.Nil(t, schema.LoadFromDb(context.Background()))

	req := &pb.SchemaRequest{}
	types, err := GetTypes(context.Background(), req)
	require.Nil(t, err)

	require.Equal(t, 1, len(types))
	x.ParseNamespaceAttr(types[0].TypeName)
}

func TestDatetime(t *testing.T) {
	// Setup temporary directory for Badger DB
	dir, err := os.MkdirTemp("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	require.NoError(t, err)
	posting.Init(ps, 0, false)
	Init(ps)

	// Set schema
	schemaTxt := `
		t: datetime @index(year) .
	`
	err = schema.ParseBytes([]byte(schemaTxt), 1)
	require.NoError(t, err)

	ctx := context.Background()
	newRunMutation := func(startTs, commitTs uint64, edges []*pb.DirectedEdge) {
		txn := posting.Oracle().RegisterStartTs(startTs)
		for _, edge := range edges {
			require.NoError(t, newRunMutation(ctx, edge, txn))
		}
		txn.Update()
		writer := posting.NewTxnWriter(ps)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	newRunMutation(1, 3, []*pb.DirectedEdge{
		{
			Entity:    1,
			Attr:      x.AttrInRootNamespace("t"),
			Value:     []byte("2020-01-01T00:00:00Z"),
			ValueType: pb.Posting_DEFAULT,
			Op:        pb.DirectedEdge_SET,
		},
	})

}

type indexMutationInfo struct {
	tokenizers []tok.Tokenizer
	edge       *pb.DirectedEdge // Represents the original uid -> value edge.
	val        types.Val
	op         pb.DirectedEdge_Op
}

func indexTokens(ctx context.Context, info *indexMutationInfo) ([]string, error) {
	attr := info.edge.Attr
	lang := info.edge.GetLang()

	schemaType, err := schema.State().TypeOf(attr)
	if err != nil || !schemaType.IsScalar() {
		return nil, errors.Errorf("Cannot index attribute %s of type object.", attr)
	}

	if !schema.State().IsIndexed(ctx, attr) {
		return nil, errors.Errorf("Attribute %s is not indexed.", attr)
	}
	sv, err := types.Convert(info.val, schemaType)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot convert value to scalar type")
	}

	var tokens []string
	for _, it := range info.tokenizers {
		toks, err := tok.BuildTokens(sv.Value, tok.GetTokenizerForLang(it, lang))
		if err != nil {
			return tokens, errors.Wrapf(err, "Cannot build tokens for attribute %s", attr)
		}
		tokens = append(tokens, toks...)
	}
	return tokens, nil
}

func TestStringIndexWithLang(t *testing.T) {
	// Setup temporary directory for Badger DB
	dir, err := os.MkdirTemp("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	require.NoError(t, err)
	posting.Init(ps, 0, false)
	Init(ps)

	// Set schema
	schemaTxt := `
		name: string @index(fulltext, trigram, term, exact) @lang .
	`

	err = schema.ParseBytes([]byte(schemaTxt), 1)
	require.NoError(t, err)

	ctx := context.Background()
	newRunMutation := func(startTs, commitTs uint64, edges []*pb.DirectedEdge) {
		txn := posting.Oracle().RegisterStartTs(startTs)
		require.NoError(t, newRunMutations(ctx, edges, txn))
		txn.Update()
		writer := posting.NewTxnWriter(ps)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	attr := x.AttrInRootNamespace("name")

	// Prepare 20,000 mutations across 10 threads, 2,000 per thread. Kept modest for
	// unit-test runtime/memory; what this exercises is concurrent writers, not volume.
	const (
		threads     = 10
		perThread   = 2000
		total       = threads * perThread
		baseStartTs = uint64(10)
	)

	// uid -> value map
	values := make(map[uint64]string, total)
	for i := 0; i < total; i++ {
		uid := uint64(i + 1)
		// Simple deterministic values with shared tokens and unique numbers.
		values[uid] = fmt.Sprintf("title %d", i+1)
	}

	// Build expected token -> set of uids
	tokenizers := schema.State().Tokenizer(ctx, attr)
	expected := make(map[string]map[uint64]struct{}, total)
	for uid, val := range values {
		info := &indexMutationInfo{
			tokenizers: tokenizers,
			op:         pb.DirectedEdge_SET,
			val:        types.Val{Tid: types.StringID, Value: []byte(val)},
			edge: &pb.DirectedEdge{
				Attr:  attr,
				Value: []byte(val),
				Lang:  "en",
				Op:    pb.DirectedEdge_SET,
			},
		}
		toks, err := indexTokens(ctx, info)
		require.NoError(t, err)
		for _, tk := range toks {
			if expected[tk] == nil {
				expected[tk] = make(map[uint64]struct{})
			}
			expected[tk][uid] = struct{}{}
		}
	}

	// Run 4 threads; each thread writes 100 edges with distinct ts
	var wg sync.WaitGroup
	wg.Add(threads)
	for th := 0; th < threads; th++ {
		th := th
		go func() {
			defer wg.Done()
			start := th*perThread + 1
			end := start + perThread
			edges := make([]*pb.DirectedEdge, 0, perThread)
			for i := start; i < end; i++ {
				uid := uint64(i)
				edges = append(edges, &pb.DirectedEdge{
					Entity:    uid,
					Attr:      attr,
					Value:     []byte(values[uid]),
					ValueType: pb.Posting_DEFAULT,
					Lang:      "en",
					Op:        pb.DirectedEdge_SET,
				})
			}
			sTs := baseStartTs + uint64(th*10)
			cTs := sTs + 2
			newRunMutation(sTs, cTs, edges)
		}()
	}
	wg.Wait()

	// Verify all tokens have the expected UIDs.
	readTs := baseStartTs + uint64(threads*10) + 10
	for tk, uidset := range expected {
		key := x.IndexKey(attr, tk)
		txn := posting.Oracle().RegisterStartTs(readTs)
		pl, err := txn.Get(key)
		require.NoError(t, err)
		lst, err := pl.Uids(posting.ListOptions{ReadTs: readTs})
		require.NoError(t, err)
		got := make(map[uint64]struct{}, len(lst.Uids))
		for _, u := range lst.Uids {
			got[u] = struct{}{}
		}
		// Compare sets
		require.Equal(t, len(uidset), len(got), "mismatch uid count for token %q", tk)
		for u := range uidset {
			if _, ok := got[u]; !ok {
				t.Fatalf("missing uid %d for token %q", u, tk)
			}
		}
	}
}

func TestCount(t *testing.T) {
	t.Skip("Inherently racy: bypasses the Oracle conflict-checking commit path. " +
		"Legacy and new pipeline both fail. Re-enable when the harness uses real txn conflicts.")
	// Setup temporary directory for Badger DB
	dir, err := os.MkdirTemp("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	require.NoError(t, err)
	posting.Init(ps, 0, false)
	Init(ps)

	// Set schema
	schemaTxt := `
		friends: [uid] @count .
	`

	err = schema.ParseBytes([]byte(schemaTxt), 1)
	require.NoError(t, err)
	ctx := context.Background()
	newRunMutation := func(startTs, commitTs uint64, edges []*pb.DirectedEdge) {
		txn := posting.Oracle().RegisterStartTs(startTs)
		require.NoError(t, newRunMutations(ctx, edges, txn))
		txn.Update()
		writer := posting.NewTxnWriter(ps)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	pred := x.AttrInRootNamespace("friends")

	// Prepare mutations such that each subject gets multiple uid edges, and
	// each edge is added from a different thread. We also send multiple
	// batches per thread.
	const (
		subjects    = 10 // total number of subjects/entities
		edgesPer    = 5  // number of edges per subject
		threads     = 2  // one thread per edge ordinal, touching all subjects
		baseStartTs = uint64(10)
		total       = subjects * edgesPer
	)

	// 1) Pre-generate all mutations into one big slice
	edgesAll := make([]*pb.DirectedEdge, 0, total)
	for subj := 1; subj <= subjects; subj++ {
		uid := uint64(subj)
		for e := 0; e < edgesPer; e++ {
			// Unique object per (subject, edge-ordinal) pair to avoid duplicates.
			// Ensures exactly 'edgesPer' distinct UIDs per subject.
			obj := uint64(1_000_000 + subj*100 + e)
			edgesAll = append(edgesAll, &pb.DirectedEdge{
				Entity:    uid,
				Attr:      pred,
				ValueId:   obj,
				ValueType: pb.Posting_UID,
				Op:        pb.DirectedEdge_SET,
			})
		}
	}

	// Shuffle the edges to simulate randomness (determinism depends on rand.Seed in package scope)
	for i := range edgesAll {
		j := rand.Intn(i + 1)
		edgesAll[i], edgesAll[j] = edgesAll[j], edgesAll[i]
	}

	// 2) Dispatch pre-generated mutations into threads, in multiple batches per thread
	var wg sync.WaitGroup
	wg.Add(threads)
	for th := 0; th < threads; th++ {
		th := th
		go func() {
			defer wg.Done()
			// Split each thread's disjoint chunk into multiple batches/transactions
			const batches = 5
			chunk := total / threads
			chunkStart := th * chunk
			chunkEnd := chunkStart + chunk
			perBatch := chunk / batches
			for b := 0; b < batches; b++ {
				batchStart := chunkStart + b*perBatch
				batchEnd := batchStart + perBatch
				if b == batches-1 {
					batchEnd = chunkEnd
				}
				batch := edgesAll[batchStart:batchEnd]
				// Space out start/commit timestamps per thread and per batch to avoid collisions
				sTs := baseStartTs + uint64(th*100) + uint64(b*2)
				cTs := sTs + 1
				newRunMutation(sTs, cTs, batch)
			}
		}()
	}
	wg.Wait()

	// Verify the @count index for the exact number of edges per subject.
	countKey := x.CountKey(pred, edgesPer, false)
	txn := posting.Oracle().RegisterStartTs(math.MaxUint64)
	pl, err := txn.Get(countKey)
	require.NoError(t, err)
	uids, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
	require.NoError(t, err)
	fmt.Println(uids.Uids)
	require.Equal(t, subjects, len(uids.Uids))
}

// fakeOracle is an in-memory stand-in for the zero Oracle. It hands out
// monotonically increasing timestamps and rejects commits whose conflict
// keys overlap a higher commitTs — same algorithm as
// dgraph/cmd/zero/oracle.go's hasConflict.
type fakeOracle struct {
	mu        sync.Mutex
	nextTs    uint64
	keyCommit map[uint64]uint64 // conflict-key fingerprint -> commitTs
	committed atomic.Int64
	aborted   atomic.Int64
}

func newFakeOracle(initialTs uint64) *fakeOracle {
	return &fakeOracle{nextTs: initialTs, keyCommit: map[uint64]uint64{}}
}

func (o *fakeOracle) reserveStartTs() uint64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.nextTs++
	return o.nextTs
}

// tryCommit mirrors zero/oracle.go: for each conflict key, if a later
// commitTs already exists, abort. Else stamp all keys with a fresh
// commitTs and return it.
func (o *fakeOracle) tryCommit(startTs uint64, conflictKeys []uint64) (uint64, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, k := range conflictKeys {
		if last, ok := o.keyCommit[k]; ok && last > startTs {
			o.aborted.Add(1)
			return 0, false
		}
	}
	o.nextTs++
	commitTs := o.nextTs
	for _, k := range conflictKeys {
		o.keyCommit[k] = commitTs
	}
	o.committed.Add(1)
	return commitTs, true
}

// runPipelineTxn drives a single mutation through the new pipeline with
// real conflict-aware commit semantics. Returns (committed, error).
func runPipelineTxn(t *testing.T, ps *badger.DB, oracle *fakeOracle,
	edges []*pb.DirectedEdge) bool {
	t.Helper()
	startTs := oracle.reserveStartTs()
	txn := posting.Oracle().RegisterStartTs(startTs)

	if err := newRunMutations(context.Background(), edges, txn); err != nil {
		t.Fatalf("pipeline failed at startTs=%d: %v", startTs, err)
	}

	// FillContext bridges plists -> deltas (via Update) and emits the
	// txn's conflict keys as base-36 strings on ctx.Keys.
	ctxApi := &api.TxnContext{}
	txn.FillContext(ctxApi, 1, false)

	keys := make([]uint64, 0, len(ctxApi.Keys))
	for _, k := range ctxApi.Keys {
		ki, err := strconv.ParseUint(k, 36, 64)
		require.NoError(t, err)
		keys = append(keys, ki)
	}

	commitTs, ok := oracle.tryCommit(startTs, keys)
	if !ok {
		return false
	}
	writer := posting.NewTxnWriter(ps)
	require.NoError(t, txn.CommitToDisk(writer, commitTs))
	require.NoError(t, writer.Flush())
	txn.UpdateCachedKeys(commitTs)
	return true
}

// TestPipelineCountIndexConcurrent mirrors the systest's
// TestCountIndexConcurrentSetDelScalarPredicate at unit-test scope: many
// concurrent transactions setting <0x1> <name> "name<rand>" against a
// scalar string predicate with @index(exact) @count, with real
// conflict-checking commit semantics. After everything settles, the data
// list for 0x1 should hold exactly one value, the count(1) bucket should
// reference exactly 0x1, and no other count bucket should reference 0x1.
func TestPipelineCountIndexConcurrent(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := badger.OpenManaged(badger.DefaultOptions(dir))
	require.NoError(t, err)
	defer ps.Close()
	posting.Init(ps, 0, false)
	Init(ps)
	posting.Oracle().ResetTxns()

	require.NoError(t, schema.ParseBytes(
		[]byte(`name: string @index(exact) @count .`), 1))

	pred := x.AttrInRootNamespace("name")
	const target uint64 = 1

	oracle := newFakeOracle(10)

	const (
		numRoutines  = 10
		txnsPerRoute = 20
	)

	var wg sync.WaitGroup
	wg.Add(numRoutines)
	for r := 0; r < numRoutines; r++ {
		go func(seed int) {
			defer wg.Done()
			rnd := rand.New(rand.NewSource(int64(seed)))
			for i := 0; i < txnsPerRoute; i++ {
				value := []byte(fmt.Sprintf("name%d", rnd.Intn(10000)))
				// Retry on conflict — same as a client doing dg.NewTxn().Mutate().
				// Each attempt uses a fresh edge: makePostingFromEdge mutates
				// edge.ValueId during processing, and reusing the object across
				// attempts would make ValidateAndConvert see it as a uid edge.
				// Real production gets a freshly-deserialized edge per Raft apply.
				for attempt := 0; attempt < 100; attempt++ {
					edge := &pb.DirectedEdge{
						Entity:    target,
						Attr:      pred,
						Value:     value,
						ValueType: pb.Posting_STRING,
						Op:        pb.DirectedEdge_SET,
					}
					if runPipelineTxn(t, ps, oracle, []*pb.DirectedEdge{edge}) {
						break
					}
				}
			}
		}(r)
	}
	wg.Wait()

	t.Logf("committed=%d aborted=%d", oracle.committed.Load(), oracle.aborted.Load())

	// Verify final state: exactly one value on 0x1, that uid in count(1) only.
	readTxn := posting.Oracle().RegisterStartTs(math.MaxUint64)

	dataKey := x.DataKey(pred, target)
	dpl, err := readTxn.Get(dataKey)
	require.NoError(t, err)
	// Scalar string predicate: AllValues returns the live posting list values
	// (one entry for a non-list scalar with a current value).
	vals, err := dpl.AllValues(math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, 1, len(vals),
		"scalar predicate should retain exactly one value, got %v", vals)

	for c := 0; c <= 5; c++ {
		ck := x.CountKey(pred, uint32(c), false)
		cpl, err := readTxn.Get(ck)
		require.NoError(t, err)
		cuids, err := cpl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
		require.NoError(t, err)
		switch c {
		case 1:
			require.Equal(t, []uint64{target}, cuids.Uids,
				"count(1) bucket must contain exactly the target uid")
		default:
			require.NotContains(t, cuids.Uids, target,
				"count(%d) bucket must not contain the target uid", c)
		}
	}
}

// TestPipelineReverseListCount mirrors the [uid] @reverse @count shape from
// the 21million live-load test (genre predicate). One subject points at
// multiple objects in a single transaction; we then verify that BOTH the
// forward data list and the reverse data list are complete.
//
// Background: systest/21million/live's TestQueries/Run_queries/query-017
// fails consistently with one specific film's `genre = Animation` edge
// missing, while other genre edges from the same film are intact. This
// is the smallest in-process repro of the same forward/reverse fanout
// pattern that the live loader hits.
func TestPipelineReverseListCount(t *testing.T) {
	dir, err := os.MkdirTemp("", "pipelinerevcount_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := badger.OpenManaged(badger.DefaultOptions(dir))
	require.NoError(t, err)
	defer ps.Close()
	posting.Init(ps, 0, false)
	Init(ps)
	posting.Oracle().ResetTxns()

	require.NoError(t, schema.ParseBytes(
		[]byte(`genre: [uid] @reverse @count .`), 1))

	pred := x.AttrInRootNamespace("genre")

	// Subject -> list of object uids. Mirrors a film with multiple genres,
	// and several films sharing some genres.
	const (
		madagascar = uint64(100)
		brotherly  = uint64(101)
		animation  = uint64(200)
		shortFilm  = uint64(201)
		comedy     = uint64(202)
	)
	wantForward := map[uint64][]uint64{
		madagascar: {animation, shortFilm},
		brotherly:  {animation, shortFilm, comedy},
	}

	// Single transaction containing all edges (the simplest case — no
	// concurrency, no batching, no Oracle conflict path). If this fails,
	// the bug is purely in the forward+reverse fanout within ProcessList.
	edges := []*pb.DirectedEdge{}
	for subj, objs := range wantForward {
		for _, obj := range objs {
			edges = append(edges, &pb.DirectedEdge{
				Entity:    subj,
				Attr:      pred,
				ValueId:   obj,
				ValueType: pb.Posting_UID,
				Op:        pb.DirectedEdge_SET,
			})
		}
	}

	txn := posting.Oracle().RegisterStartTs(10)
	require.NoError(t, newRunMutations(context.Background(), edges, txn))
	txn.Update()
	w := posting.NewTxnWriter(ps)
	require.NoError(t, txn.CommitToDisk(w, 11))
	require.NoError(t, w.Flush())
	txn.UpdateCachedKeys(11)

	readTxn := posting.Oracle().RegisterStartTs(math.MaxUint64)

	// Forward: each subject must have all its expected objects.
	for subj, wantObjs := range wantForward {
		key := x.DataKey(pred, subj)
		pl, err := readTxn.Get(key)
		require.NoError(t, err)
		got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
		require.NoError(t, err)
		require.ElementsMatch(t, wantObjs, got.Uids,
			"forward list for subject %d", subj)
	}

	// Reverse: each object must have all the subjects that point at it.
	wantReverse := map[uint64][]uint64{}
	for subj, objs := range wantForward {
		for _, obj := range objs {
			wantReverse[obj] = append(wantReverse[obj], subj)
		}
	}
	for obj, wantSubjs := range wantReverse {
		key := x.ReverseKey(pred, obj)
		pl, err := readTxn.Get(key)
		require.NoError(t, err)
		got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
		require.NoError(t, err)
		require.ElementsMatch(t, wantSubjs, got.Uids,
			"reverse list for object %d", obj)
	}
}

// TestPipelineReverseListCountMultiBatch escalates TestPipelineReverseListCount
// by spreading edges across many sequential transactions, similar to how the
// live loader chunks edges into many batches. Each batch is one transaction.
func TestPipelineReverseListCountMultiBatch(t *testing.T) {
	dir, err := os.MkdirTemp("", "pipelinerevcount2_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := badger.OpenManaged(badger.DefaultOptions(dir))
	require.NoError(t, err)
	defer ps.Close()
	posting.Init(ps, 0, false)
	Init(ps)
	posting.Oracle().ResetTxns()

	require.NoError(t, schema.ParseBytes(
		[]byte(`genre: [uid] @reverse @count .`), 1))

	pred := x.AttrInRootNamespace("genre")

	// Many subjects, many objects, every (subject, object) combination — a
	// dense forward fanout that pushes every reverse list to grow as well.
	const (
		nSubjects = 50
		nObjects  = 20
	)
	subjBase := uint64(10000)
	objBase := uint64(20000)

	// Build every edge.
	allEdges := make([]*pb.DirectedEdge, 0, nSubjects*nObjects)
	for s := 0; s < nSubjects; s++ {
		for o := 0; o < nObjects; o++ {
			allEdges = append(allEdges, &pb.DirectedEdge{
				Entity:    subjBase + uint64(s),
				Attr:      pred,
				ValueId:   objBase + uint64(o),
				ValueType: pb.Posting_UID,
				Op:        pb.DirectedEdge_SET,
			})
		}
	}

	// Split into batches of 7 — a value chosen to make each batch carry a
	// non-trivial mix of subjects and objects per ProcessList run.
	const batchSize = 7
	ts := uint64(10)
	for start := 0; start < len(allEdges); start += batchSize {
		end := start + batchSize
		if end > len(allEdges) {
			end = len(allEdges)
		}
		batch := allEdges[start:end]

		txn := posting.Oracle().RegisterStartTs(ts)
		require.NoError(t, newRunMutations(context.Background(), batch, txn))
		txn.Update()
		w := posting.NewTxnWriter(ps)
		require.NoError(t, txn.CommitToDisk(w, ts+1))
		require.NoError(t, w.Flush())
		txn.UpdateCachedKeys(ts + 1)
		ts += 2
	}

	readTxn := posting.Oracle().RegisterStartTs(math.MaxUint64)

	// Forward: each subject must hold exactly all nObjects objects.
	wantForwardObjs := make([]uint64, nObjects)
	for o := 0; o < nObjects; o++ {
		wantForwardObjs[o] = objBase + uint64(o)
	}
	for s := 0; s < nSubjects; s++ {
		subj := subjBase + uint64(s)
		key := x.DataKey(pred, subj)
		pl, err := readTxn.Get(key)
		require.NoError(t, err)
		got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
		require.NoError(t, err)
		require.ElementsMatch(t, wantForwardObjs, got.Uids,
			"forward list for subject %d (s=%d): missing some objects", subj, s)
	}

	// Reverse: each object must hold exactly all nSubjects subjects.
	wantReverseSubjs := make([]uint64, nSubjects)
	for s := 0; s < nSubjects; s++ {
		wantReverseSubjs[s] = subjBase + uint64(s)
	}
	for o := 0; o < nObjects; o++ {
		obj := objBase + uint64(o)
		key := x.ReverseKey(pred, obj)
		pl, err := readTxn.Get(key)
		require.NoError(t, err)
		got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
		require.NoError(t, err)
		require.ElementsMatch(t, wantReverseSubjs, got.Uids,
			"reverse list for object %d (o=%d): missing some subjects", obj, o)
	}
}

// TestPipelineReverseListCountMultiPred escalates further: multiple
// list-uid predicates with @reverse @count are mutated together in each
// batch, so the per-predicate pipeline goroutines for each predicate run
// in parallel inside Process(). This is the live-loader's actual shape
// (genre + director.film + starring + rated all live in the same payload).
func TestPipelineReverseListCountMultiPred(t *testing.T) {
	dir, err := os.MkdirTemp("", "pipelinerevcount3_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := badger.OpenManaged(badger.DefaultOptions(dir))
	require.NoError(t, err)
	defer ps.Close()
	posting.Init(ps, 0, false)
	Init(ps)
	posting.Oracle().ResetTxns()

	require.NoError(t, schema.ParseBytes([]byte(`
		genre: [uid] @reverse @count .
		starring: [uid] @reverse @count .
		director_film: [uid] @reverse @count .
	`), 1))

	preds := []string{
		x.AttrInRootNamespace("genre"),
		x.AttrInRootNamespace("starring"),
		x.AttrInRootNamespace("director_film"),
	}

	const (
		nSubjects = 30
		nObjects  = 12
	)
	subjBase := uint64(10000)
	objBase := uint64(20000)

	allEdges := make([]*pb.DirectedEdge, 0, len(preds)*nSubjects*nObjects)
	for _, pred := range preds {
		for s := 0; s < nSubjects; s++ {
			for o := 0; o < nObjects; o++ {
				allEdges = append(allEdges, &pb.DirectedEdge{
					Entity:    subjBase + uint64(s),
					Attr:      pred,
					ValueId:   objBase + uint64(o),
					ValueType: pb.Posting_UID,
					Op:        pb.DirectedEdge_SET,
				})
			}
		}
	}
	// Shuffle so each batch carries an interleaved mix of predicates.
	rnd := rand.New(rand.NewSource(1))
	rnd.Shuffle(len(allEdges), func(i, j int) {
		allEdges[i], allEdges[j] = allEdges[j], allEdges[i]
	})

	const batchSize = 23
	ts := uint64(10)
	for start := 0; start < len(allEdges); start += batchSize {
		end := start + batchSize
		if end > len(allEdges) {
			end = len(allEdges)
		}
		batch := allEdges[start:end]

		txn := posting.Oracle().RegisterStartTs(ts)
		require.NoError(t, newRunMutations(context.Background(), batch, txn))
		txn.Update()
		w := posting.NewTxnWriter(ps)
		require.NoError(t, txn.CommitToDisk(w, ts+1))
		require.NoError(t, w.Flush())
		txn.UpdateCachedKeys(ts + 1)
		ts += 2
	}

	readTxn := posting.Oracle().RegisterStartTs(math.MaxUint64)

	wantForwardObjs := make([]uint64, nObjects)
	for o := 0; o < nObjects; o++ {
		wantForwardObjs[o] = objBase + uint64(o)
	}
	wantReverseSubjs := make([]uint64, nSubjects)
	for s := 0; s < nSubjects; s++ {
		wantReverseSubjs[s] = subjBase + uint64(s)
	}

	for _, pred := range preds {
		for s := 0; s < nSubjects; s++ {
			subj := subjBase + uint64(s)
			pl, err := readTxn.Get(x.DataKey(pred, subj))
			require.NoError(t, err)
			got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
			require.NoError(t, err)
			require.ElementsMatch(t, wantForwardObjs, got.Uids,
				"forward list for pred=%q subj=%d", pred, subj)
		}
		for o := 0; o < nObjects; o++ {
			obj := objBase + uint64(o)
			pl, err := readTxn.Get(x.ReverseKey(pred, obj))
			require.NoError(t, err)
			got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
			require.NoError(t, err)
			require.ElementsMatch(t, wantReverseSubjs, got.Uids,
				"reverse list for pred=%q obj=%d", pred, obj)
		}
	}
}

// TestPipelineReverseListCountConcurrent mirrors the live-loader's actual
// concurrency: many goroutines submitting batched transactions in parallel,
// going through a real conflict-checking commit (the fakeOracle harness
// also used by TestPipelineCountIndexConcurrent), against a [uid] @reverse
// @count predicate. This is the closest in-process reproduction of the
// systest/21million/live shape that we have without standing up a real
// cluster.
func TestPipelineReverseListCountConcurrent(t *testing.T) {
	dir, err := os.MkdirTemp("", "pipelinerevcount4_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ps, err := badger.OpenManaged(badger.DefaultOptions(dir))
	require.NoError(t, err)
	defer ps.Close()
	posting.Init(ps, 0, false)
	Init(ps)
	posting.Oracle().ResetTxns()

	require.NoError(t, schema.ParseBytes(
		[]byte(`genre: [uid] @reverse @count .`), 1))

	pred := x.AttrInRootNamespace("genre")

	const (
		nSubjects = 60 // films
		nObjects  = 25 // genres
	)
	subjBase := uint64(10000)
	objBase := uint64(20000)

	allEdges := make([]*pb.DirectedEdge, 0, nSubjects*nObjects)
	for s := 0; s < nSubjects; s++ {
		for o := 0; o < nObjects; o++ {
			allEdges = append(allEdges, &pb.DirectedEdge{
				Entity:    subjBase + uint64(s),
				Attr:      pred,
				ValueId:   objBase + uint64(o),
				ValueType: pb.Posting_UID,
				Op:        pb.DirectedEdge_SET,
			})
		}
	}
	rnd := rand.New(rand.NewSource(7))
	rnd.Shuffle(len(allEdges), func(i, j int) {
		allEdges[i], allEdges[j] = allEdges[j], allEdges[i]
	})

	// Chunk into many small batches so multiple goroutines compete on the
	// same predicate, mimicking `dgraph live -c 10` behavior.
	const batchSize = 17
	type batch []*pb.DirectedEdge
	batches := []batch{}
	for start := 0; start < len(allEdges); start += batchSize {
		end := start + batchSize
		if end > len(allEdges) {
			end = len(allEdges)
		}
		batches = append(batches, allEdges[start:end])
	}

	oracle := newFakeOracle(10)
	const concurrency = 10

	jobs := make(chan batch, len(batches))
	for _, b := range batches {
		jobs <- b
	}
	close(jobs)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for w := 0; w < concurrency; w++ {
		go func() {
			defer wg.Done()
			for b := range jobs {
				// Retry until commit, with a fresh edge clone each attempt
				// (the pipeline mutates edge.ValueId during processing).
				for attempt := 0; attempt < 200; attempt++ {
					clones := make([]*pb.DirectedEdge, len(b))
					for i, e := range b {
						clones[i] = &pb.DirectedEdge{
							Entity: e.Entity, Attr: e.Attr,
							ValueId: e.ValueId, ValueType: e.ValueType, Op: e.Op,
						}
					}
					if runPipelineTxn(t, ps, oracle, clones) {
						break
					}
				}
			}
		}()
	}
	wg.Wait()

	t.Logf("committed=%d aborted=%d", oracle.committed.Load(), oracle.aborted.Load())

	readTxn := posting.Oracle().RegisterStartTs(math.MaxUint64)

	wantForwardObjs := make([]uint64, nObjects)
	for o := 0; o < nObjects; o++ {
		wantForwardObjs[o] = objBase + uint64(o)
	}
	wantReverseSubjs := make([]uint64, nSubjects)
	for s := 0; s < nSubjects; s++ {
		wantReverseSubjs[s] = subjBase + uint64(s)
	}

	missingForward := []string{}
	missingReverse := []string{}
	for s := 0; s < nSubjects; s++ {
		subj := subjBase + uint64(s)
		pl, err := readTxn.Get(x.DataKey(pred, subj))
		require.NoError(t, err)
		got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
		require.NoError(t, err)
		if len(got.Uids) != nObjects {
			missingForward = append(missingForward,
				fmt.Sprintf("subj=%d has %d/%d", subj, len(got.Uids), nObjects))
		}
	}
	for o := 0; o < nObjects; o++ {
		obj := objBase + uint64(o)
		pl, err := readTxn.Get(x.ReverseKey(pred, obj))
		require.NoError(t, err)
		got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
		require.NoError(t, err)
		if len(got.Uids) != nSubjects {
			missingReverse = append(missingReverse,
				fmt.Sprintf("obj=%d has %d/%d", obj, len(got.Uids), nSubjects))
		}
	}
	require.Empty(t, missingForward, "forward lists missing entries")
	require.Empty(t, missingReverse, "reverse lists missing entries")

	// And the strict version: each list must match exactly.
	for s := 0; s < nSubjects; s++ {
		subj := subjBase + uint64(s)
		pl, err := readTxn.Get(x.DataKey(pred, subj))
		require.NoError(t, err)
		got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
		require.NoError(t, err)
		require.ElementsMatch(t, wantForwardObjs, got.Uids,
			"forward list for subj %d", subj)
	}
	for o := 0; o < nObjects; o++ {
		obj := objBase + uint64(o)
		pl, err := readTxn.Get(x.ReverseKey(pred, obj))
		require.NoError(t, err)
		got, err := pl.Uids(posting.ListOptions{ReadTs: math.MaxUint64})
		require.NoError(t, err)
		require.ElementsMatch(t, wantReverseSubjs, got.Uids,
			"reverse list for obj %d", obj)
	}
}

func TestDeleteSetWithVarEdgeCorruptsData(t *testing.T) {
	// Setup temporary directory for Badger DB
	dir, err := os.MkdirTemp("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	require.NoError(t, err)
	posting.Init(ps, 0, false)
	Init(ps)

	// Set schema
	schemaTxt := `
		room: string @index(hash) @upsert .
		person: string @index(hash) @upsert .
		office: uid @reverse @count .
	`
	err = schema.ParseBytes([]byte(schemaTxt), 1)
	require.NoError(t, err)

	ctx := context.Background()
	attrRoom := x.AttrInRootNamespace("room")
	attrPerson := x.AttrInRootNamespace("person")
	attrOffice := x.AttrInRootNamespace("office")

	uidRoom := uint64(1)
	uidJohn := uint64(2)

	newRunMutation := func(startTs, commitTs uint64, edges []*pb.DirectedEdge) {
		txn := posting.Oracle().RegisterStartTs(startTs)
		for _, edge := range edges {
			require.NoError(t, newRunMutation(ctx, edge, txn))
		}
		txn.Update()
		writer := posting.NewTxnWriter(ps)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	// Initial mutation: Set John → Leopard
	newRunMutation(1, 3, []*pb.DirectedEdge{
		{
			Entity:    uidJohn,
			Attr:      attrPerson,
			Value:     []byte("John Smith"),
			ValueType: pb.Posting_STRING,
			Op:        pb.DirectedEdge_SET,
		},
		{
			Entity:    uidRoom,
			Attr:      attrRoom,
			Value:     []byte("Leopard"),
			ValueType: pb.Posting_STRING,
			Op:        pb.DirectedEdge_SET,
		},
		{
			Entity:    uidJohn,
			Attr:      attrOffice,
			ValueId:   uidRoom,
			ValueType: pb.Posting_UID,
			Op:        pb.DirectedEdge_SET,
		},
	})

	key := x.DataKey(attrOffice, uidJohn)
	rollup(t, key, ps, 4)

	// Second mutation: Remove John from Leopard, assign Amanda
	uidAmanda := uint64(3)

	newRunMutation(6, 8, []*pb.DirectedEdge{
		{
			Entity:    uidJohn,
			Attr:      attrOffice,
			ValueId:   uidRoom,
			ValueType: pb.Posting_UID,
			Op:        pb.DirectedEdge_DEL,
		},
		{
			Entity:    uidAmanda,
			Attr:      attrPerson,
			Value:     []byte("Amanda Anderson"),
			ValueType: pb.Posting_STRING,
			Op:        pb.DirectedEdge_SET,
		},
		{
			Entity:    uidAmanda,
			Attr:      attrOffice,
			ValueId:   uidRoom,
			ValueType: pb.Posting_UID,
			Op:        pb.DirectedEdge_SET,
		},
	})

	// Read and validate: Amanda assigned, John unassigned
	txnRead := posting.Oracle().RegisterStartTs(10)

	list, err := txnRead.Get(key)
	require.NoError(t, err)

	uids, err := list.Uids(posting.ListOptions{ReadTs: 10})
	require.NoError(t, err)

	// This assertion FAILS in the broken case where both Amanda and John are assigned
	require.Equal(t, 0, len(uids.Uids), "John should no longer have an office assigned")

	keyRev := x.ReverseKey(attrOffice, uidRoom)
	listRev, err := txnRead.Get(keyRev)
	require.NoError(t, err)

	reverseUids, err := listRev.Uids(posting.ListOptions{ReadTs: 10})
	require.NoError(t, err)

	require.Equal(t, []uint64{uidAmanda}, reverseUids.Uids, "Only Amanda should be assigned on reverse edge")
}

func TestGetScalarList(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("scalarPredicateCount4: uid ."), 1)
	require.NoError(t, err)

	runM := func(startTs, commitTs uint64, edges []*pb.DirectedEdge) {
		txn := posting.Oracle().RegisterStartTs(startTs)
		for _, edge := range edges {
			x.Check(newRunMutation(context.Background(), edge, txn))
		}
		txn.Update()
		writer := posting.NewTxnWriter(pstore)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	attr := x.AttrInRootNamespace("scalarPredicateCount4")

	runM(5, 7, []*pb.DirectedEdge{{
		ValueId:   3,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_SET,
	}})

	key := x.DataKey(attr, 1)
	rollup(t, key, ps, 8)

	runM(9, 11, []*pb.DirectedEdge{{
		ValueId:   5,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_SET,
	}})

	txn := posting.Oracle().RegisterStartTs(13)
	l, err := txn.Get(key)
	require.Nil(t, err)
	uids, err := l.Uids(posting.ListOptions{ReadTs: 13})
	require.Nil(t, err)
	require.Equal(t, 1, len(uids.Uids))
}

func TestMultipleTxnListCount(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("scalarPredicateCount3: [uid] @count ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	attr := x.AttrInRootNamespace("scalarPredicateCount3")

	runM := func(startTs, commitTs uint64, edges []*pb.DirectedEdge) {
		txn := posting.Oracle().RegisterStartTs(startTs)
		for _, edge := range edges {
			x.Check(newRunMutation(ctx, edge, txn))
		}
		txn.Update()
		writer := posting.NewTxnWriter(pstore)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	runM(9, 11, []*pb.DirectedEdge{{
		ValueId:   3,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_SET,
	}, {
		ValueId:   2,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_SET,
	}})

	txn := posting.Oracle().RegisterStartTs(13)
	key := x.CountKey(attr, 1, false)
	l, err := txn.Get(key)
	require.Nil(t, err)
	uids, err := l.Uids(posting.ListOptions{ReadTs: 13})
	require.Nil(t, err)
	require.Equal(t, 0, len(uids.Uids))

	key = x.CountKey(attr, 2, false)
	l, err = txn.Get(key)
	require.Nil(t, err)
	uids, err = l.Uids(posting.ListOptions{ReadTs: 13})
	require.Nil(t, err)
	require.Equal(t, 1, len(uids.Uids))
}

func TestScalarPredicateRevCount(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("scalarPredicateCount2: uid @reverse @count ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	attr := x.AttrInRootNamespace("scalarPredicateCount2")

	runM := func(startTs, commitTs uint64, edges []*pb.DirectedEdge) {
		txn := posting.Oracle().RegisterStartTs(startTs)
		for _, edge := range edges {
			x.Check(newRunMutation(ctx, edge, txn))
		}
		txn.Update()
		writer := posting.NewTxnWriter(pstore)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	runM(9, 11, []*pb.DirectedEdge{{
		ValueId:   3,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_SET,
	}, {
		ValueId:   3,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_DEL,
	}})

	txn := posting.Oracle().RegisterStartTs(13)
	key := x.DataKey(attr, 1)
	l, err := txn.Get(key)
	require.Nil(t, err)
	l.RLock()
	require.Equal(t, 0, l.GetLength(13))
	l.RUnlock()

	runM(15, 17, []*pb.DirectedEdge{{
		ValueId:   3,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_SET,
	}})

	txn = posting.Oracle().RegisterStartTs(18)
	l, err = txn.Get(key)
	require.Nil(t, err)
	l.RLock()
	require.Equal(t, 1, l.GetLength(18))
	l.RUnlock()

	runM(18, 19, []*pb.DirectedEdge{{
		ValueId:   3,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_DEL,
	}})

	txn = posting.Oracle().RegisterStartTs(20)
	l, err = txn.Get(key)
	require.Nil(t, err)
	l.RLock()
	require.Equal(t, 0, l.GetLength(20))
	l.RUnlock()
}

func TestScalarPredicateIntCount(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("scalarPredicateCount1: string @count ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	attr := x.AttrInRootNamespace("scalarPredicateCount1")

	runM := func(startTs, commitTs uint64, edge *pb.DirectedEdge) {
		txn := posting.Oracle().RegisterStartTs(startTs)
		x.Check(newRunMutation(ctx, edge, txn))
		txn.Update()
		writer := posting.NewTxnWriter(pstore)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	runM(5, 7, &pb.DirectedEdge{
		Value:     []byte("a"),
		ValueType: pb.Posting_STRING,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_SET,
	})

	key := x.CountKey(attr, 1, false)
	rollup(t, key, ps, 8)

	runM(9, 11, &pb.DirectedEdge{
		Value:     []byte("a"),
		ValueType: pb.Posting_STRING,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_DEL,
	})

	txn := posting.Oracle().RegisterStartTs(20)
	l, err := txn.Get(key)
	require.Nil(t, err)
	l.RLock()
	require.Equal(t, 0, l.GetLength(20))
	l.RUnlock()
}

func TestScalarPredicateCount(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("scalarPredicateCount: uid @count ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	attr := x.AttrInRootNamespace("scalarPredicateCount")

	runM := func(startTs, commitTs uint64, edge *pb.DirectedEdge) {
		txn := posting.Oracle().RegisterStartTs(startTs)
		x.Check(newRunMutation(ctx, edge, txn))
		txn.Update()
		writer := posting.NewTxnWriter(pstore)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	runM(5, 7, &pb.DirectedEdge{
		ValueId:   2,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_SET,
	})

	key := x.CountKey(attr, 1, false)
	rollup(t, key, ps, 8)

	runM(9, 11, &pb.DirectedEdge{
		ValueId:   3,
		ValueType: pb.Posting_UID,
		Attr:      attr,
		Entity:    1,
		Op:        pb.DirectedEdge_SET,
	})

	txn := posting.Oracle().RegisterStartTs(15)
	l, err := txn.Get(key)
	require.Nil(t, err)
	l.RLock()
	require.Equal(t, 1, l.GetLength(15))
	l.RUnlock()
}

func TestSingleUidReplacement(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("singleUidReplaceTest: uid ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.AttrInRootNamespace("singleUidReplaceTest")

	// Txn 1. Set 1 -> 2
	x.Check(newRunMutation(ctx, &pb.DirectedEdge{
		ValueId: 2,
		Attr:    attr,
		Entity:  1,
		Op:      pb.DirectedEdge_SET,
	}, txn))

	txn.Update()
	writer := posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 7))
	require.NoError(t, writer.Flush())
	txn.UpdateCachedKeys(7)

	// Txn 2. Set 1 -> 3
	txn = posting.Oracle().RegisterStartTs(9)

	x.Check(newRunMutation(ctx, &pb.DirectedEdge{
		ValueId: 3,
		Attr:    attr,
		Entity:  1,
		Op:      pb.DirectedEdge_SET,
	}, txn))

	txn.Update()
	writer = posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 11))
	require.NoError(t, writer.Flush())
	txn.UpdateCachedKeys(11)

	key := x.DataKey(attr, 1)

	// Reading the david index, we should see 2 inserted, 1 deleted
	txn = posting.Oracle().RegisterStartTs(15)
	l, err := txn.Get(key)
	require.NoError(t, err)

	uids, err := l.Uids(posting.ListOptions{ReadTs: 15})
	require.NoError(t, err)
	require.Equal(t, uids.Uids, []uint64{3})
}

func TestSingleString(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("singleUidTest: string @index(exact) @unique ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.AttrInRootNamespace("singleUidTest")

	// Txn 1. Set 1 -> david 2 -> blush
	x.Check(newRunMutation(ctx, &pb.DirectedEdge{
		Value:  []byte("david"),
		Attr:   attr,
		Entity: 1,
		Op:     pb.DirectedEdge_SET,
	}, txn))

	x.Check(newRunMutation(ctx, &pb.DirectedEdge{
		Value:  []byte("blush"),
		Attr:   attr,
		Entity: 2,
		Op:     pb.DirectedEdge_SET,
	}, txn))

	txn.Update()
	writer := posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 7))
	require.NoError(t, writer.Flush())
	txn.UpdateCachedKeys(7)

	// Txn 2. Set 2 -> david 1 -> blush
	txn = posting.Oracle().RegisterStartTs(9)

	x.Check(newRunMutation(ctx, &pb.DirectedEdge{
		Value:  []byte("david"),
		Attr:   attr,
		Entity: 2,
		Op:     pb.DirectedEdge_SET,
	}, txn))

	x.Check(newRunMutation(ctx, &pb.DirectedEdge{
		Value:  []byte("blush"),
		Attr:   attr,
		Entity: 1,
		Op:     pb.DirectedEdge_SET,
	}, txn))

	txn.Update()
	writer = posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 11))
	require.NoError(t, writer.Flush())
	txn.UpdateCachedKeys(11)

	key := x.IndexKey(attr, string([]byte{2, 100, 97, 118, 105, 100}))

	// Reading the david index, we should see 2 inserted, 1 deleted
	txn = posting.Oracle().RegisterStartTs(15)
	l, err := txn.Get(key)
	require.NoError(t, err)

	found, mpost, err := l.FindPosting(15, 2)
	require.NoError(t, err)
	require.Equal(t, found, true)
	require.Equal(t, mpost.Op, uint32(0x1))

	found, _, err = l.FindPosting(15, 1)
	require.NoError(t, err)
	require.Equal(t, found, false)

	rollup(t, key, pstore, 16)

	txn = posting.Oracle().RegisterStartTs(18)
	l, err = txn.Get(key)
	require.NoError(t, err)

	found, mpost, err = l.FindPosting(18, 2)
	require.NoError(t, err)
	require.Equal(t, found, true)
	require.Equal(t, mpost.Op, uint32(0x0))

	found, _, err = l.FindPosting(18, 1)
	require.NoError(t, err)
	require.Equal(t, found, false)
}

func TestLangExact(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("testLang: string @index(term) @lang ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.AttrInRootNamespace("testLang")

	edge := &pb.DirectedEdge{
		Value:  []byte("english"),
		Attr:   attr,
		Entity: 1,
		Op:     pb.DirectedEdge_SET,
		Lang:   "en",
	}

	x.Check(newRunMutation(ctx, edge, txn))

	edge = &pb.DirectedEdge{
		Value:  []byte("hindi"),
		Attr:   attr,
		Entity: 1,
		Op:     pb.DirectedEdge_SET,
		Lang:   "hi",
	}

	x.Check(newRunMutation(ctx, edge, txn))

	txn.Update()
	writer := posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 2))
	require.NoError(t, writer.Flush())
	txn.UpdateCachedKeys(2)

	key := x.DataKey(attr, 1)
	rollup(t, key, pstore, 4)

	pl, err := readPostingListFromDisk(key, pstore, 6)
	require.NoError(t, err)

	val, err := pl.ValueForTag(6, "hi")
	require.NoError(t, err)
	require.Equal(t, val.Value, []byte("hindi"))
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func BenchmarkAddMutationWithIndex(b *testing.B) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("benchmarkadd: string @index(term) ."), 1)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.AttrInRootNamespace("benchmarkadd")

	n := uint64(1000)
	values := make([]string, 0)
	for range n {
		values = append(values, randStringBytes(5))
	}

	for i := 0; i < b.N; i++ {
		edge := &pb.DirectedEdge{
			Value:  []byte(values[rand.Intn(int(n))]),
			Attr:   attr,
			Entity: rand.Uint64()%n + 1,
			Op:     pb.DirectedEdge_SET,
		}

		x.Check(newRunMutation(ctx, edge, txn))
	}
}

func TestRemoveDuplicates(t *testing.T) {
	toSet := func(uids []uint64) map[uint64]struct{} {
		m := make(map[uint64]struct{})
		for _, uid := range uids {
			m[uid] = struct{}{}
		}
		return m
	}

	for _, test := range []struct {
		setIn   []uint64
		setOut  []uint64
		uidsIn  []uint64
		uidsOut []uint64
	}{
		{setIn: nil, setOut: nil, uidsIn: nil, uidsOut: nil},
		{setIn: nil, setOut: []uint64{2}, uidsIn: []uint64{2}, uidsOut: []uint64{2}},
		{setIn: []uint64{2}, setOut: []uint64{2}, uidsIn: []uint64{2}, uidsOut: []uint64{}},
		{setIn: []uint64{2}, setOut: []uint64{2}, uidsIn: []uint64{2, 2}, uidsOut: []uint64{}},
		{
			setIn:   []uint64{2, 3},
			setOut:  []uint64{2, 3, 4, 5},
			uidsIn:  []uint64{3, 4, 5},
			uidsOut: []uint64{4, 5},
		},
	} {
		set := toSet(test.setIn)
		uidsOut := removeDuplicates(test.uidsIn, set)
		require.Equal(t, uidsOut, test.uidsOut)
		require.Equal(t, set, toSet(test.setOut))
	}
}

// ---------------------------------------------------------------------------
// Pipeline-vs-legacy differential.
//
// Apply the SAME edges through the legacy ordered path and the mutation
// pipeline (forced fan-out) on a freshly wiped store, then compare the entire
// committed key-space. requireSame asserts the two paths agree.
//
// Below the harness: a broad fuzz gate (TestPipelineLegacyDifferential) plus
// one deterministic, minimal case per divergence class the fuzz surfaced. These
// are permanent correctness tests that must stay in the suite; each failing
// case must go green once its class is fixed, and the pipeline must not be made
// default-on while any of them is red.
// ---------------------------------------------------------------------------

type diffH struct {
	t  *testing.T
	ps *badger.DB
	ts uint64
}

// snapshot is one path's committed state. data holds every data/index/reverse/
// count posting (facets included). vec reduces the HNSW graph keys to "which
// uids are in the vector index": the entry point and per-level adjacency depend
// on insertion order and random level draws, so byte-comparing them would
// produce false diffs between two correct runs.
type snapshot struct {
	data map[string]string
	vec  map[string]bool
}

func newDiffH(t *testing.T) *diffH {
	dir, err := os.MkdirTemp("", "pipediff_")
	require.NoError(t, err)
	ps, err := badger.OpenManaged(badger.DefaultOptions(dir).WithLoggingLevel(badger.ERROR))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close(); _ = os.RemoveAll(dir) })

	posting.Init(ps, 0, false)
	Init(ps)
	require.NoError(t, schema.ParseBytes([]byte(`
		sv:  string        @index(exact) .
		lv:  [uid]         @reverse .
		lc:  [uid]         @reverse @count .
		lp:  string        @lang .
		li:  string        @lang @index(exact) .
		ls:  [string]      @index(term) .
		lf:  [string]      @index(fulltext) .
		lt:  [string]      @index(trigram) .
		geo: geo           @index(geo) .
		bf:  bigfloat      @index(bigfloat) .
		bo:  bool          @index(bool) .
		fl:  float         @index(float) .
		pw:  password .
		vec: float32vector @index(hnsw(metric:"euclidean")) .
		su:  uid           @reverse .
		fs:  string        @index(exact) .
		fu:  [uid] .
		fr:  [uid]         @reverse .
	`), 1))
	prevPar := x.WorkerConfig.IntraMutationParallelism
	prevEPW := x.WorkerConfig.IntraMutationEdgesPerWorker
	t.Cleanup(func() {
		x.WorkerConfig.IntraMutationParallelism = prevPar
		x.WorkerConfig.IntraMutationEdgesPerWorker = prevEPW
	})
	x.WorkerConfig.IntraMutationParallelism = x.IntraMutationParallelism{Workers: 8}
	x.WorkerConfig.IntraMutationEdgesPerWorker = 1
	return &diffH{t: t, ps: ps, ts: 10}
}

func (h *diffH) A(p string) string { return x.AttrInRootNamespace(p) }

func (h *diffH) dump() snapshot {
	s := snapshot{data: map[string]string{}, vec: map[string]bool{}}
	pwAttr := h.A("pw")
	txn := h.ps.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Rewind(); it.Valid(); it.Next() {
		key := it.Item().KeyCopy(nil)
		pk, err := x.Parse(key)
		if err != nil || (!pk.IsData() && !pk.IsIndex() && !pk.IsReverse() && !pk.IsCountOrCountRev()) {
			continue
		}
		if strings.Contains(pk.Attr, hnsw.VecKeyword) {
			if strings.HasSuffix(pk.Attr, hnsw.VecEntry) {
				continue
			}
			if pk.IsData() {
				s.vec[fmt.Sprintf("%d", pk.Uid)] = true
			}
			continue
		}
		pl, err := posting.GetNoStore(key, math.MaxUint64)
		require.NoError(h.t, err)
		var parts []string
		require.NoError(h.t, pl.Iterate(math.MaxUint64, 0, func(p *pb.Posting) error {
			v := fmt.Sprintf("%x", p.Value)
			if pk.Attr == pwAttr {
				v = "PWHASH" // bcrypt is salted; the plaintext is verified separately
			}
			parts = append(parts, fmt.Sprintf("u%d:v%s:l%s:t%d:f%s",
				p.Uid, v, p.LangTag, p.PostingType, fmtFacets(p.Facets)))
			return nil
		}))
		sort.Strings(parts)
		if len(parts) > 0 {
			s.data[fmt.Sprintf("%x", key)] = fmt.Sprintf("%v", parts)
		}
	}
	return s
}

func fmtFacets(fs []*api.Facet) string {
	if len(fs) == 0 {
		return "-"
	}
	var s []string
	for _, f := range fs {
		s = append(s, fmt.Sprintf("%s=%d:%x", f.Key, f.ValType, f.Value))
	}
	sort.Strings(s)
	return strings.Join(s, ",")
}

func cloneEdges(in []*pb.DirectedEdge) []*pb.DirectedEdge {
	out := make([]*pb.DirectedEdge, len(in))
	for i, e := range in {
		c := &pb.DirectedEdge{
			Entity:    e.Entity,
			Attr:      e.Attr,
			Value:     append([]byte(nil), e.Value...),
			ValueId:   e.ValueId,
			ValueType: e.ValueType,
			Lang:      e.Lang,
			Op:        e.Op,
		}
		for _, f := range e.Facets {
			c.Facets = append(c.Facets, &api.Facet{
				Key:     f.Key,
				Value:   append([]byte(nil), f.Value...),
				ValType: f.ValType,
				Tokens:  append([]string(nil), f.Tokens...),
				Alias:   f.Alias,
			})
		}
		out[i] = c
	}
	return out
}

// apply wipes the store, applies the batches in order through the chosen path
// (committing each one, so later batches act on committed data) and snapshots.
func (h *diffH) apply(pipeline bool, batches ...[]*pb.DirectedEdge) snapshot {
	require.NoError(h.t, h.ps.DropAll())
	posting.ResetCache()
	posting.Oracle().ResetTxns()
	for _, edges := range batches {
		startTs, commitTs := h.ts, h.ts+1
		h.ts += 10
		ctx := schema.GetWriteContext(context.Background())
		txn := posting.Oracle().RegisterStartTs(startTs)
		if pipeline {
			require.NoError(h.t, posting.NewMutationPipeline(txn).Process(ctx, cloneEdges(edges)))
		} else {
			es := cloneEdges(edges)
			sort.SliceStable(es, func(i, j int) bool {
				if es[i].Attr != es[j].Attr {
					return es[i].Attr < es[j].Attr
				}
				return es[i].Entity < es[j].Entity
			})
			for _, e := range es {
				require.NoError(h.t, runMutation(ctx, e, txn))
			}
		}
		txn.Update()
		w := posting.NewTxnWriter(h.ps)
		require.NoError(h.t, txn.CommitToDisk(w, commitTs))
		require.NoError(h.t, w.Flush())
		txn.UpdateCachedKeys(commitTs)
	}
	return h.dump()
}

// requireSame fails if the pipeline's committed state differs from legacy's.
// With several batches, later ones act on data the earlier ones committed.
func (h *diffH) requireSame(batches ...[]*pb.DirectedEdge) {
	legacy := h.apply(false, batches...)
	pipe := h.apply(true, batches...)
	var all []*pb.DirectedEdge
	for _, b := range batches {
		all = append(all, b...)
	}
	require.Equal(h.t, legacy.data, pipe.data, "pipeline diverged from legacy\n  edges: %s", fmtEdges(all))
	require.Equal(h.t, legacy.vec, pipe.vec,
		"pipeline diverged from legacy (vector-index membership)\n  edges: %s", fmtEdges(all))
}

// caseSame runs one case as an isolated subtest with the subtest's *testing.T,
// so a failing case cannot abort its siblings.
func (h *diffH) caseSame(t *testing.T, name string, batches ...[]*pb.DirectedEdge) {
	t.Run(name, func(t *testing.T) {
		hh := *h
		hh.t = t
		defer func() { h.ts = hh.ts }()
		hh.requireSame(batches...)
	})
}

// ---- edge builders ----

func eSetStr(attr string, uid uint64, v string) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(v), ValueType: pb.Posting_STRING, Op: pb.DirectedEdge_SET}
}
func eDelStr(attr string, uid uint64, v string) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(v), ValueType: pb.Posting_STRING, Op: pb.DirectedEdge_DEL}
}
func eSetUid(attr string, uid, tgt uint64) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, ValueId: tgt, ValueType: pb.Posting_UID, Op: pb.DirectedEdge_SET}
}
func eLangSet(attr string, uid uint64, v, lang string) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(v), ValueType: pb.Posting_STRING, Lang: lang, Op: pb.DirectedEdge_SET}
}
func eLangDel(attr string, uid uint64, v, lang string) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(v), ValueType: pb.Posting_STRING, Lang: lang, Op: pb.DirectedEdge_DEL}
}
func eStarDel(attr string, uid uint64) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(x.Star), Op: pb.DirectedEdge_DEL}
}

func fmtEdges(edges []*pb.DirectedEdge) string {
	var b []string
	for _, e := range edges {
		op := "SET"
		if e.Op == pb.DirectedEdge_DEL {
			op = "DEL"
		}
		v := string(e.Value)
		if e.ValueType == pb.Posting_UID {
			v = fmt.Sprintf("uid(%d)", e.ValueId)
		}
		lang := ""
		if e.Lang != "" {
			lang = "@" + e.Lang
		}
		b = append(b, fmt.Sprintf("%s %s e=%d v=%q%s", op, e.Attr, e.Entity, v, lang))
	}
	return fmt.Sprintf("%v", b)
}

// TestPipelineLegacyDifferential is the broad fuzz gate (discovery + acceptance)
// over every non-vector shape in the schema, star-deletes included, so every
// batch mixes legacy-routed and batched predicates. Vectors have their own fuzz
// with a membership compare, and password is excluded because bcrypt makes each
// seed ~100ms.
func TestPipelineLegacyDifferential(t *testing.T) {
	h := newDiffH(t)
	uids := []uint64{0x1, 0x2, 0x3, 0x4}
	strs := []string{"a", "b", "c", "d"}
	tgts := []uint64{0x100, 0x101, 0x102}
	langs := []string{"en", "de", "fr"}
	words := []string{"red apple", "green apple", "blue pear", "apple"}
	tris := []string{"apple", "zebra", "maple", "ape"}
	geos := []string{geoP1, geoP2}
	bfs := []string{"1.5", "2.25", "-3.125"}
	fls := []string{"0.5", "1.5", "2.5"}
	fks := [][2]string{{"w", "1"}, {"w", "2"}, {"tag", `"x"`}}
	starPreds := []string{"sv", "lv", "lc", "lp", "li", "ls", "lf", "lt", "geo", "bf", "bo", "fl", "su", "fs", "fu", "fr"}
	gen := func(rng *rand.Rand) []*pb.DirectedEdge {
		n := 3 + rng.Intn(8)
		var edges []*pb.DirectedEdge
		pick := func(ss []string) string { return ss[rng.Intn(len(ss))] }
		tgt := func() uint64 { return tgts[rng.Intn(len(tgts))] }
		for i := 0; i < n; i++ {
			uid := uids[rng.Intn(len(uids))]
			del := rng.Intn(2) == 1
			var e *pb.DirectedEdge
			switch rng.Intn(17) {
			case 0:
				e = eSetStr(h.A("sv"), uid, pick(strs))
			case 1:
				e = eSetUid(h.A("lv"), uid, tgt())
			case 2:
				e = eSetUid(h.A("lc"), uid, tgt())
			case 3:
				e = eLangSet(h.A("lp"), uid, pick(strs), pick(langs))
			case 4:
				e = eSetStr(h.A("ls"), uid, pick(strs))
			case 5:
				e = eSetStr(h.A("lf"), uid, pick(words))
			case 6:
				e = eSetStr(h.A("lt"), uid, pick(tris))
			case 7:
				e = eSetStr(h.A("geo"), uid, pick(geos))
			case 8:
				e = eSetStr(h.A("bf"), uid, pick(bfs))
			case 9:
				e = eSetStr(h.A("bo"), uid, pick([]string{"true", "false"}))
			case 10:
				e = eSetStr(h.A("fl"), uid, pick(fls))
			case 11:
				e = eSetUid(h.A("su"), uid, tgt())
			case 12:
				e = eSetStr(h.A("fs"), uid, pick(strs[:2]))
			case 13:
				e = eSetUid(h.A("fu"), uid, tgt())
			case 14:
				e = eSetUid(h.A("fr"), uid, tgt())
			case 15:
				e = eLangSet(h.A("li"), uid, pick(strs), pick(langs))
			case 16:
				e = eStarDel(h.A(pick(starPreds)), uid)
			}
			switch {
			case e.Op == pb.DirectedEdge_DEL: // star-delete
			case del:
				e.Op = pb.DirectedEdge_DEL
			default:
				if a := x.ParseAttr(e.Attr); a == "fs" || a == "fu" || a == "fr" {
					fk := fks[rng.Intn(len(fks))]
					e = withF(t, e, fk[0], fk[1])
				}
			}
			edges = append(edges, e)
		}
		return edges
	}
	const iters = 2000
	mism := 0
	byPred := map[string]int{}
	for seed := 0; seed < iters; seed++ {
		rng := rand.New(rand.NewSource(int64(seed) + 1))
		edges := gen(rng)
		legacy := h.apply(false, edges)
		pipe := h.apply(true, edges)
		if fmt.Sprintf("%v", legacy.data) != fmt.Sprintf("%v", pipe.data) {
			mism++
			diff := symDiff(legacy.data, pipe.data)
			for k := range diff {
				if pk, err := x.Parse(mustHex(k)); err == nil {
					byPred[x.ParseAttr(pk.Attr)]++
				}
			}
			if mism <= 5 { // a few concrete examples make the tally actionable
				t.Logf("seed %d diverged: %s", seed, fmtEdges(edges))
				for k := range diff {
					t.Logf("  key %s\n    legacy:   %s\n    pipeline: %s", k, legacy.data[k], pipe.data[k])
				}
			}
		}
	}
	t.Logf("fuzz differential: %d/%d seeds diverged; by predicate: %v", mism, iters, byPred)
	require.Zero(t, mism, "pipeline must match legacy")
}

func symDiff(a, b map[string]string) map[string]bool {
	out := map[string]bool{}
	for k, v := range a {
		if b[k] != v {
			out[k] = true
		}
	}
	for k, v := range b {
		if a[k] != v {
			out[k] = true
		}
	}
	return out
}

func mustHex(s string) []byte {
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		fmt.Sscanf(s[i*2:i*2+2], "%02x", &out[i])
	}
	return out
}

// Divergence class 1 — star-delete reorder. A SET followed by a star-delete on
// the same (pred,entity) in one proposal. The pipeline runs the delete inline
// and the set via a worker afterward, so the value (and its index) survive when
// they should be cleared.
func TestPipeline_StarDeleteThenSet(t *testing.T) {
	h := newDiffH(t)
	sv := h.A("sv")
	h.requireSame([]*pb.DirectedEdge{
		eSetStr(sv, 1, "a"),
		eStarDel(sv, 1), // after this, sv(1) must be empty
	})
}

// Star-deletes route their predicate through the legacy per-edge path for the
// batch (Process pre-pass -> PredicatePipeline.legacy), which preserves order
// against the SETs on the same list while the other predicates in the mutation
// keep the batched path.
func TestPipeline_StarDeleteRouting(t *testing.T) {
	h := newDiffH(t)
	sv, lv, lc, lp, ls, geo, v := h.A("sv"), h.A("lv"), h.A("lc"), h.A("lp"), h.A("ls"), h.A("geo"), h.A("vec")
	h.caseSame(t, "star-then-set", batch(eStarDel(sv, 1), eSetStr(sv, 1, "a"))) // the replace pattern keeps the SET
	h.caseSame(t, "interleaved",
		batch(eSetStr(sv, 1, "a"), eStarDel(sv, 1), eSetStr(sv, 1, "b"), eStarDel(sv, 1), eSetStr(sv, 1, "c")))
	h.caseSame(t, "committed+set+star", batch(eSetStr(sv, 1, "a")), batch(eSetStr(sv, 1, "b"), eStarDel(sv, 1)))
	h.caseSame(t, "committed+star+set", batch(eSetStr(sv, 1, "a")), batch(eStarDel(sv, 1), eSetStr(sv, 1, "b")))
	h.caseSame(t, "two-entities-one-star", batch(eSetStr(sv, 1, "a"), eSetStr(sv, 2, "b"), eStarDel(sv, 1)))
	h.caseSame(t, "reverse-list", batch(eSetUid(lv, 1, 0x100), eSetUid(lv, 1, 0x101), eStarDel(lv, 1)))
	h.caseSame(t, "reverse-list-committed",
		batch(eSetUid(lv, 1, 0x100), eSetUid(lv, 2, 0x100)), batch(eStarDel(lv, 1), eSetUid(lv, 1, 0x102)))
	h.caseSame(t, "count-list", batch(eSetUid(lc, 1, 0x100), eStarDel(lc, 1), eSetUid(lc, 1, 0x101)))
	h.caseSame(t, "lang", batch(eLangSet(lp, 1, "b", "en"), eLangSet(lp, 1, "c", "de"), eStarDel(lp, 1), eLangSet(lp, 1, "d", "fr")))
	h.caseSame(t, "indexed-list", batch(eSetStr(ls, 1, "c"), eStarDel(ls, 1), eSetStr(ls, 1, "d")))
	h.caseSame(t, "vector", batch(eSetStr(v, 1, "[1,0,0]"), eStarDel(v, 1), eSetStr(v, 1, "[0,1,0]")))
	// sv (star) and ls (indexed list) are legacy-routed; lv and geo stay batched.
	h.caseSame(t, "mixed-mode-batch", batch(
		eSetStr(sv, 1, "a"), eStarDel(sv, 1), eSetStr(sv, 1, "b"),
		eSetUid(lv, 2, 0x100), eSetUid(lv, 2, 0x101),
		eSetStr(geo, 3, geoP1),
		eSetStr(ls, 4, "x"), eDelStr(ls, 4, "x"), eSetStr(ls, 4, "y"),
	))
}

// Divergence class 2 — @count with repeated/multiple SETs changing a target's
// in-degree. The batched ProcessCount and legacy disagreed, and the evidence
// points at LEGACY's count index being the stale side; that is unresolved.
// @count predicates take the legacy path for now (routeToLegacy), so this case
// passes by construction and the open question is parked: whether legacy's
// count is right needs a semantic (not differential) check before @count is
// promoted back to the batched path.
func TestPipeline_CountDuplicateSets(t *testing.T) {
	h := newDiffH(t)
	lc := h.A("lc")
	h.requireSame([]*pb.DirectedEdge{
		eSetUid(lc, 1, 0x100),
		eSetUid(lc, 1, 0x100), // duplicate source->target
		eSetUid(lc, 3, 0x100), // target 0x100 in-degree 2
		eSetUid(lc, 3, 0x101), // target 0x101 in-degree 1
	})
}

// Divergence class 3 — @lang. ProcessList keys a scalar @lang slot by
// fingerprint(lang) and insertPosting is last-write-wins on that slot, so a
// value-specific delete used to land regardless of the value it named: it
// dropped a DIFFERENT value under the same tag, including an already committed
// one. Legacy applies such a delete only when value and type match
// (addMutationHelper); ProcessList now mirrors that (langDeleteMismatch).
func TestPipeline_LangValueDelete(t *testing.T) {
	h := newDiffH(t)
	lp, li := h.A("lp"), h.A("li")
	h.caseSame(t, "in-batch/nonmatching-del-other-tag-set", batch(
		eLangSet(lp, 1, "b", "en"),
		eLangSet(lp, 1, "c", "de"),
		eLangDel(lp, 1, "c", "en"), // deletes nothing (en is "b"); de must remain
	))
	h.caseSame(t, "in-batch/set-set-del-old",
		batch(eLangSet(lp, 1, "b", "en"), eLangSet(lp, 1, "c", "en"), eLangDel(lp, 1, "b", "en"))) // en stays "c"
	h.caseSame(t, "in-batch/matching-del", batch(eLangSet(lp, 1, "b", "en"), eLangDel(lp, 1, "b", "en")))
	h.caseSame(t, "committed/nonmatching-del", batch(eLangSet(lp, 1, "b", "en")), batch(eLangDel(lp, 1, "c", "en"))) // used to wipe "b"
	h.caseSame(t, "committed/matching-del", batch(eLangSet(lp, 1, "b", "en")), batch(eLangDel(lp, 1, "b", "en")))
	h.caseSame(t, "committed/double-del",
		batch(eLangSet(lp, 1, "b", "en")), batch(eLangDel(lp, 1, "b", "en"), eLangDel(lp, 1, "b", "en")))
	h.caseSame(t, "committed/del-then-set-same-tag",
		batch(eLangSet(lp, 1, "b", "en")), batch(eLangDel(lp, 1, "b", "en"), eLangSet(lp, 1, "c", "en")))
	h.caseSame(t, "committed/multi-tag-mixed",
		batch(eLangSet(lp, 1, "b", "en"), eLangSet(lp, 1, "c", "de"), eLangSet(lp, 1, "d", "fr")),
		batch(eLangDel(lp, 1, "x", "en"), eLangDel(lp, 1, "c", "de"), eLangSet(lp, 1, "e", "fr"))) // en kept, de gone, fr replaced
	// With an index the ignored delete must not emit an index Del either.
	h.caseSame(t, "indexed/committed-nonmatching-del", batch(eLangSet(li, 1, "b", "en")), batch(eLangDel(li, 1, "c", "en")))
	h.caseSame(t, "indexed/committed-matching-del", batch(eLangSet(li, 1, "b", "en")), batch(eLangDel(li, 1, "b", "en")))
	h.caseSame(t, "indexed/in-batch-churn",
		batch(eLangSet(li, 1, "b", "en"), eLangDel(li, 1, "zz", "en"), eLangSet(li, 1, "c", "de")))
	// Value-indexed @lang additionally takes the legacy path (routeToLegacy): the
	// batched index pass cannot retract an in-batch overwritten tag value, so
	// "b" would keep its index token here. Found by the fuzz (seed 432).
	h.caseSame(t, "indexed/in-batch-same-tag-replace", batch(eLangSet(li, 1, "b", "de"), eLangSet(li, 1, "a", "de")))
}

// Divergence class 4 — value-indexed list. SET/DELETE churn on a
// [string] @index(term) left a stale term in the index on the batched path,
// also on committed data. Value-indexed lists take the legacy path for now
// (routeToLegacy); these cases guard that routing until the batched path nets
// in-batch set/delete correctly.
func TestPipeline_IndexedListValueChurn(t *testing.T) {
	h := newDiffH(t)
	ls := h.A("ls")
	h.caseSame(t, "in-batch", batch(eSetStr(ls, 1, "c"), eDelStr(ls, 1, "c"), eSetStr(ls, 1, "d"))) // "c" must leave the index
	h.caseSame(t, "committed+churn", batch(eSetStr(ls, 1, "c")), batch(eSetStr(ls, 1, "c"), eDelStr(ls, 1, "c")))
	h.caseSame(t, "committed+del-old-set-new", batch(eSetStr(ls, 1, "c")), batch(eDelStr(ls, 1, "c"), eSetStr(ls, 1, "d")))
}

// ---- shape coverage: builders ----

const (
	geoP1 = `{"type":"Point","coordinates":[1.0,2.0]}`
	geoP2 = `{"type":"Point","coordinates":[3.5,-4.25]}`
)

func batch(edges ...*pb.DirectedEdge) []*pb.DirectedEdge { return edges }

// withF attaches facets built by facets.FacetFor: bare numbers parse as ints,
// string values must be quoted (e.g. `"x"`).
func withF(t *testing.T, e *pb.DirectedEdge, kv ...string) *pb.DirectedEdge {
	for i := 0; i+1 < len(kv); i += 2 {
		f, err := facets.FacetFor(kv[i], kv[i+1])
		require.NoError(t, err)
		e.Facets = append(e.Facets, f)
	}
	return e
}

// eSetVecTyped pre-converts the vector to VFLOAT binary — what typed RDF
// ("..."^^<xs:[]float32>) delivers via chunker/rdf_parser.go — so
// ValidateAndConvert is a no-op on both paths. Untyped input (JSON mutations,
// RDF without the type) arrives as a STRING/DEFAULT literal; eSetStr covers it.
func eSetVecTyped(t *testing.T, attr string, uid uint64, s string) *pb.DirectedEdge {
	dst, err := types.Convert(types.Val{Tid: types.StringID, Value: []byte(s)}, types.VFloatID)
	require.NoError(t, err)
	b := types.ValueForType(types.BinaryID)
	require.NoError(t, types.Marshal(dst, &b))
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: b.Value.([]byte),
		ValueType: pb.Posting_VFLOAT, Op: pb.DirectedEdge_SET}
}

// Divergence class 4 on other tokenizers. Same mechanism as
// TestPipeline_IndexedListValueChurn: a deleted value's tokens survive in the
// index. Fulltext keeps the shared token "appl" correct (the case the
// Dels-before-Sets sort in InsertTokenizerIndexes was written for) but leaks
// the deleted value's own token "red". The trigram values share no trigrams, so
// stale tokens cannot hide behind legitimate ones.
func TestPipeline_IndexedListValueChurnFulltext(t *testing.T) {
	h := newDiffH(t)
	lf := h.A("lf")
	h.caseSame(t, "churn-shared-token",
		batch(eSetStr(lf, 1, "red apple"), eDelStr(lf, 1, "red apple"), eSetStr(lf, 1, "green apple")))
	h.caseSame(t, "committed+churn",
		batch(eSetStr(lf, 1, "red apple")), batch(eSetStr(lf, 1, "red apple"), eDelStr(lf, 1, "red apple")))
	h.caseSame(t, "del-old-set-new-shared-token",
		batch(eSetStr(lf, 1, "red apple")), batch(eDelStr(lf, 1, "red apple"), eSetStr(lf, 1, "green apple")))
}

func TestPipeline_IndexedListValueChurnTrigram(t *testing.T) {
	h := newDiffH(t)
	lt := h.A("lt")
	h.caseSame(t, "churn", batch(eSetStr(lt, 1, "apple"), eDelStr(lt, 1, "apple"), eSetStr(lt, 1, "zebra")))
	h.caseSame(t, "committed+churn",
		batch(eSetStr(lt, 1, "apple")), batch(eSetStr(lt, 1, "apple"), eDelStr(lt, 1, "apple")))
}

// Vector values that arrive untyped (JSON mutations, RDF without
// ^^<xs:[]float32>) must be converted to VFLOAT binary by the pipeline exactly
// as runMutation does. ProcessVectorIndex used to skip ValidateAndConvert and
// stored the literal text, which also left the vector out of the HNSW index.
func TestPipeline_VectorUntypedValue(t *testing.T) {
	h := newDiffH(t)
	v := h.A("vec")
	h.caseSame(t, "single-set", batch(eSetStr(v, 1, "[1.0, 0.0, 0.0]")))
	h.caseSame(t, "set-replace-del", batch(
		eSetStr(v, 1, "[1.0, 0.0, 0.0]"),
		eSetStr(v, 1, "[0.0, 1.0, 0.0]"), // replace
		eSetStr(v, 2, "[0.0, 0.0, 1.0]"),
		eDelStr(v, 2, "[0.0, 0.0, 1.0]"), // matching delete
	))
	h.caseSame(t, "committed+replace",
		batch(eSetStr(v, 1, "[1.0, 0.0, 0.0]")), batch(eSetStr(v, 1, "[0.5, 0.5, 0.0]")))
	h.caseSame(t, "committed+star",
		batch(eSetStr(v, 1, "[1.0, 0.0, 0.0]"), eSetStr(v, 2, "[0.0, 1.0, 0.0]")), batch(eStarDel(v, 1)))
}

// TestPipelineVectorDifferential fuzzes untyped vector SET/DEL mixes and
// compares vector data exactly and HNSW membership (not adjacency).
func TestPipelineVectorDifferential(t *testing.T) {
	h := newDiffH(t)
	v := h.A("vec")
	vecs := []string{"[1,0,0]", "[0,1,0]", "[0,0,1]", "[0.5,0.5,0]", "[0.2,0.3,0.9]"}
	const iters = 200
	mism := 0
	for seed := 0; seed < iters; seed++ {
		rng := rand.New(rand.NewSource(int64(seed) + 1))
		n := 2 + rng.Intn(6)
		var edges []*pb.DirectedEdge
		for i := 0; i < n; i++ {
			e := eSetStr(v, uint64(1+rng.Intn(4)), vecs[rng.Intn(len(vecs))])
			if rng.Intn(4) == 0 {
				e.Op = pb.DirectedEdge_DEL
			}
			edges = append(edges, e)
		}
		legacy := h.apply(false, edges)
		pipe := h.apply(true, edges)
		if fmt.Sprintf("%v", legacy.data) != fmt.Sprintf("%v", pipe.data) ||
			fmt.Sprintf("%v", legacy.vec) != fmt.Sprintf("%v", pipe.vec) {
			mism++
		}
	}
	t.Logf("vector fuzz: %d/%d seeds diverged", mism, iters)
	require.Zero(t, mism, "pipeline must match legacy on vector data and index membership")
}

// TestPipelineShapes covers the predicate shapes the differential found clean,
// in-batch and against committed data, so they stay guarded.
func TestPipelineShapes(t *testing.T) {
	h := newDiffH(t)
	g, b, bo, fl, pw, su := h.A("geo"), h.A("bf"), h.A("bo"), h.A("fl"), h.A("pw"), h.A("su")
	fs, fu, fr, v := h.A("fs"), h.A("fu"), h.A("fr"), h.A("vec")

	h.caseSame(t, "Geo/set-replace-del", batch(eSetStr(g, 1, geoP1), eSetStr(g, 1, geoP2), eDelStr(g, 1, geoP2)))
	h.caseSame(t, "Geo/committed+nonmatching-del", batch(eSetStr(g, 1, geoP1)), batch(eDelStr(g, 1, geoP2)))
	h.caseSame(t, "Geo/committed+star", batch(eSetStr(g, 1, geoP1)), batch(eStarDel(g, 1)))

	h.caseSame(t, "BigFloat/set-replace-del", batch(eSetStr(b, 1, "1.5"), eSetStr(b, 1, "2.25"), eDelStr(b, 1, "2.25")))
	h.caseSame(t, "BigFloat/committed+nonmatching-del", batch(eSetStr(b, 1, "1.5")), batch(eDelStr(b, 1, "9.9")))

	h.caseSame(t, "BoolFloat/in-batch", batch(
		eSetStr(bo, 1, "true"), eSetStr(bo, 1, "false"), eDelStr(bo, 1, "false"),
		eSetStr(fl, 2, "1.5"), eSetStr(fl, 2, "2.5"), eDelStr(fl, 2, "1.5"), // non-matching del
	))
	h.caseSame(t, "BoolFloat/committed+nonmatching-del", batch(eSetStr(fl, 2, "1.5")), batch(eDelStr(fl, 2, "7.5")))

	// Password: bcrypt is salted, so the dump masks the hash; verify the
	// plaintext against what each path stored. Passwords must be >= 6 chars.
	for _, pipeline := range []bool{false, true} {
		t.Run(fmt.Sprintf("Password/verify-plaintext/pipeline=%v", pipeline), func(t *testing.T) {
			hh := *h
			hh.t = t
			hh.apply(pipeline, batch(eSetStr(pw, 1, "first1")), batch(eSetStr(pw, 1, "second")))
			h.ts = hh.ts
			pl, err := posting.GetNoStore(x.DataKey(pw, 1), math.MaxUint64)
			require.NoError(t, err)
			n := 0
			require.NoError(t, pl.Iterate(math.MaxUint64, 0, func(p *pb.Posting) error {
				n++
				return types.VerifyPassword("second", string(p.Value))
			}))
			require.Equal(t, 1, n, "expected exactly one password posting")
		})
	}
	h.caseSame(t, "Password/structure-replace", batch(eSetStr(pw, 1, "alpha1")), batch(eSetStr(pw, 1, "bravo2")))
	h.caseSame(t, "Password/committed+star", batch(eSetStr(pw, 1, "alpha1")), batch(eStarDel(pw, 1)))

	suDel := eSetUid(su, 1, 0x100)
	suDel.Op = pb.DirectedEdge_DEL
	h.caseSame(t, "SingleUidReverse/replace", batch(eSetUid(su, 1, 0x100), eSetUid(su, 1, 0x101))) // reverse must move
	h.caseSame(t, "SingleUidReverse/committed+replace", batch(eSetUid(su, 1, 0x100)), batch(eSetUid(su, 1, 0x101)))
	h.caseSame(t, "SingleUidReverse/committed+star", batch(eSetUid(su, 1, 0x100)), batch(eStarDel(su, 1)))
	h.caseSame(t, "SingleUidReverse/committed+matching-del", batch(eSetUid(su, 1, 0x100)), batch(suDel))

	h.caseSame(t, "Facets/scalar", batch(
		withF(t, eSetStr(fs, 1, "a"), "w", "1"),
		withF(t, eSetStr(fs, 1, "a"), "w", "2"), // same value, facet update
		withF(t, eSetStr(fs, 2, "b"), "tag", `"x"`, "n", "7"),
		eDelStr(fs, 2, "b"),
	))
	h.caseSame(t, "Facets/scalar-committed+facet-update",
		batch(withF(t, eSetStr(fs, 1, "a"), "w", "1")), batch(withF(t, eSetStr(fs, 1, "a"), "w", "2")))
	h.caseSame(t, "Facets/uid-list", batch(
		withF(t, eSetUid(fu, 1, 0x100), "since", "2020"),
		withF(t, eSetUid(fu, 1, 0x100), "since", "2021"), // facet replace on same target
		withF(t, eSetUid(fu, 1, 0x101), "since", "2022"),
		eSetUid(fu, 1, 0x102), // no facets
	))
	h.caseSame(t, "Facets/uid-list-committed+facet-update",
		batch(withF(t, eSetUid(fu, 1, 0x100), "since", "2020")), batch(withF(t, eSetUid(fu, 1, 0x100), "since", "2021")))
	h.caseSame(t, "Facets/uid-list-reverse", batch(
		withF(t, eSetUid(fr, 1, 0x100), "since", "2020"),
		withF(t, eSetUid(fr, 2, 0x100), "since", "2021"),
		withF(t, eSetUid(fr, 1, 0x100), "since", "2022"), // facet replace; reverse posting must follow
	))
	h.caseSame(t, "Facets/uid-list-reverse-committed+facet-update",
		batch(withF(t, eSetUid(fr, 1, 0x100), "since", "2020")), batch(withF(t, eSetUid(fr, 1, 0x100), "since", "2021")))

	vDel := eSetVecTyped(t, v, 2, "[0.0, 0.0, 1.0]")
	vDel.Op = pb.DirectedEdge_DEL
	h.caseSame(t, "VectorTyped/single-set", batch(eSetVecTyped(t, v, 1, "[1.0, 0.0, 0.0]")))
	h.caseSame(t, "VectorTyped/set-replace", batch(
		eSetVecTyped(t, v, 1, "[1.0, 0.0, 0.0]"), eSetVecTyped(t, v, 1, "[0.0, 1.0, 0.0]"), eSetVecTyped(t, v, 2, "[0.0, 0.0, 1.0]")))
	h.caseSame(t, "VectorTyped/committed+replace",
		batch(eSetVecTyped(t, v, 1, "[1.0, 0.0, 0.0]")), batch(eSetVecTyped(t, v, 1, "[0.5, 0.5, 0.0]")))
	h.caseSame(t, "VectorTyped/committed+star",
		batch(eSetVecTyped(t, v, 1, "[1.0, 0.0, 0.0]"), eSetVecTyped(t, v, 2, "[0.0, 1.0, 0.0]")), batch(eStarDel(v, 1)))
	h.caseSame(t, "VectorTyped/committed+matching-del", batch(eSetVecTyped(t, v, 2, "[0.0, 0.0, 1.0]")), batch(vDel))
}
