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
	"strconv"
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
	"github.com/dgraph-io/dgraph/v25/types"
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
	tokenizers   []tok.Tokenizer
	factorySpecs []*tok.FactoryCreateSpec
	edge         *pb.DirectedEdge // Represents the original uid -> value edge.
	val          types.Val
	op           pb.DirectedEdge_Op
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

	// Prepare 400 mutations across 4 threads, 100 per thread (kept modest for stability).
	const (
		threads     = 10
		perThread   = 20000
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
