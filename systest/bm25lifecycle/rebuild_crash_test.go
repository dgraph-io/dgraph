//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

// Wave-2 lifecycle tests for BM25 index rebuilds (questions Q16 and Q20-lite).
//
// Q16 (KNOWN-FAILING when the overlap manifests): a background @index(bm25)
// rebuild DropPrefix'es the stats buckets and flushes its absolute totals at the
// rebuild's startTs. Live mutations committing between the DropPrefix and the
// flush read an EMPTY bucket, and their zero-based read-modify-write absolute
// total shadows the rebuild's flush via MVCC (higher commitTs wins). The final
// corpus stats therefore UNDERCOUNT, which shows up as a wrong IDF in every
// BM25 score. We assert final scores against an independently computed
// closed-form expectation, which discriminates regardless of mechanism.
//
// Q20-lite (expected PASS): the forever-empty hazard was refuted — the schema
// commits only after BuildIndexes succeeds, and Raft replay re-runs the rebuild
// after a crash. This is a crash-RECOVERY sanity test: SIGKILL the alpha
// mid-rebuild, restart it, and poll until bm25 queries return the full expected
// result set with exact closed-form scores.
//
// Corpus design: every document tokenizes to exactly 2 terms with tf=1 each
// (distinct non-stopword, stem-stable nonsense words). Then avgDL == dl == 2,
// so the BM25 term factor (k+1)*tf/(k*(1-b+b*dl/avgDL)+tf) collapses to 1 and
// every score equals the smoothed IDF: ln(1 + (N-df+0.5)/(df+0.5)). docCount
// (via IDF) and totalTerms (via avgDL) are thus both pinned by score equality,
// even though ReadBM25Stats is not client-reachable.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

type scoredNode struct {
	UID   string  `json:"uid"`
	Score float64 `json:"score"`
}

// bm25Probe runs an unpaginated bm25() root function binding the score to a
// value variable, returning every match with its score. No `first` is used so
// the worker takes the score-all path (topK == 0) and emits the full corpus of
// matches.
func bm25Probe(dg *dgraphapi.GrpcClient, pred, term string) ([]scoredNode, error) {
	q := fmt.Sprintf(`{
		s as var(func: bm25(%s, %q))
		q(func: uid(s)) {
			uid
			score: val(s)
		}
	}`, pred, term)
	resp, err := dg.Query(q)
	if err != nil {
		return nil, err
	}
	var out struct {
		Q []scoredNode `json:"q"`
	}
	if err := json.Unmarshal(resp.GetJson(), &out); err != nil {
		return nil, err
	}
	return out.Q, nil
}

// expectedBM25Score mirrors worker/bm25wand.go exactly: smoothed IDF
// (math.Log1p((N-df+0.5)/(df+0.5))) times the BM25 term factor with default
// k=1.2, b=0.75, and our fixed dl=2, tf=1. With totalTerms/nDocs == 2 the term
// factor is 1 and the score is the IDF alone.
func expectedBM25Score(nDocs, df, totalTerms float64) float64 {
	const k, b = 1.2, 0.75
	const dl, tf = 2.0, 1.0
	avgDL := totalTerms / nDocs
	idf := math.Log1p((nDocs - df + 0.5) / (df + 0.5))
	return idf * (k + 1) * tf / (k*(1-b+b*dl/avgDL) + tf)
}

// loadTwoTokenDocs loads `count` docs all carrying the same two-token text,
// in batches of 1000 (the predicate has no bm25 index at load time, so batch
// size cannot trip the Q15/Q19 intra-proposal stats races). Returns the
// assigned uids in blank-node order.
func loadTwoTokenDocs(dg *dgraphapi.GrpcClient, pred, text, labelPrefix string, count int) ([]string, error) {
	const batchSize = 1000
	uids := make([]string, 0, count)
	for start := 0; start < count; start += batchSize {
		end := start + batchSize
		if end > count {
			end = count
		}
		var sb strings.Builder
		for i := start; i < end; i++ {
			fmt.Fprintf(&sb, "_:%s%d <%s> %q .\n", labelPrefix, i, pred, text)
		}
		resp, err := dg.Mutate(&api.Mutation{SetNquads: []byte(sb.String()), CommitNow: true})
		if err != nil {
			return nil, err
		}
		for i := start; i < end; i++ {
			uid, ok := resp.Uids[fmt.Sprintf("%s%d", labelPrefix, i)]
			if !ok {
				return nil, fmt.Errorf("no uid assigned for blank node %s%d", labelPrefix, i)
			}
			uids = append(uids, uid)
		}
	}
	return uids, nil
}

// mutateWithRetry commits a single mutation, retrying transaction aborts.
// Single-doc txns on a bm25 predicate conflict on the shared uid%32 stats
// buckets, so aborts are expected under concurrency and must be retried.
func mutateWithRetry(dg *dgraphapi.GrpcClient, mu *api.Mutation) (*api.Response, error) {
	var lastErr error
	for attempt := 0; attempt < 100; attempt++ {
		resp, err := dg.Mutate(mu)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		low := strings.ToLower(err.Error())
		if !strings.Contains(low, "abort") && !strings.Contains(low, "retry") &&
			!strings.Contains(low, "conflict") {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond) // retry backoff, not a convergence wait
	}
	return nil, lastErr
}

// alphaIndexingInProgress reports whether alpha 0's /health?all lists `pred`
// as currently being indexed (schema.GetIndexingPredicates). Best-effort:
// any error reads as false.
func alphaIndexingInProgress(c *dgraphtest.LocalCluster, pred string) bool {
	port, err := c.GetAlphaHttpPublicPort(0)
	if err != nil {
		return false
	}
	httpc := http.Client{Timeout: 5 * time.Second}
	resp, err := httpc.Get("http://0.0.0.0:" + port + "/health?all")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}
	return strings.Contains(string(body), `"indexing"`) && strings.Contains(string(body), pred)
}

// pollUntil runs f every tick until it returns true or the timeout elapses.
// Used for best-effort window detection (NOT for correctness assertions —
// those use require/assert.Eventually).
func pollUntil(timeout, tick time.Duration, f func() bool) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		if f() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		<-ticker.C
	}
}

// TestBM25RebuildUnderLiveWrites — Q16, KNOWN-FAILING when the rebuild/live-
// write overlap manifests.
//
// Load 5000 unindexed docs, add @index(bm25) with RunInBackground, and while
// the rebuild runs commit concurrent adds/updates/deletes in single-doc txns.
// Afterwards every score must equal the closed-form expectation computed from
// the true final corpus (see the ground-truth math below). The failing
// signature is a score whose implied docCount collapsed toward the number of
// live writes (bucket totals clobbered by zero-based RMW totals shadowing the
// rebuild's absolute flush), or a mid-rebuild-committed doc that bm25() never
// finds.
func TestBM25RebuildUnderLiveWrites(t *testing.T) {
	// KNOWN-FAILING (Q16), gated: live writes committing between the rebuild's
	// DropPrefix and its absolute stats flush read an EMPTY bucket and their
	// zero-based RMW total shadows the flush via MVCC — empirically confirmed
	// (uid 0x5b scored 0.0062, closed-form expectation 4.137; docCount collapsed).
	// The fix needs a Zero-mediated conflicting flush or delta-merged stats; this
	// test is its acceptance gate. Set BM25_KNOWN_FAILING=1 to run.
	if os.Getenv("BM25_KNOWN_FAILING") == "" {
		t.Skip("KNOWN-FAILING (Q16 rebuild-vs-live stats undercount); set BM25_KNOWN_FAILING=1 to run")
	}
	const (
		pred        = "rebuild_text"
		baseDocs    = 5000
		liveAdds    = 60
		liveUpdates = 20
		liveDeletes = 20
	)
	// Final ground truth:
	//   docCount   = 5000 + 60 adds - 20 deletes                  = 5040
	//   totalTerms = 5000*2 + 60*2 - 20*2 (deletes; updates net 0) = 10080
	//   df(quokka)   = 60 adds + 20 updates                        = 80
	//   df(zeppelin) = 5000 - 20 deleted - 20 updated              = 4960
	const (
		wantDocCount   = float64(baseDocs + liveAdds - liveDeletes)
		wantTotalTerms = float64(2 * (baseDocs + liveAdds - liveDeletes))
		wantQuokkaDF   = float64(liveAdds + liveUpdates)
		wantZeppelinDF = float64(baseDocs - liveDeletes - liveUpdates)
	)

	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	dg, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, dg.DropAll())
	require.NoError(t, dg.SetupSchema(pred+`: string .`))

	baseUids, err := loadTwoTokenDocs(dg, pred, "zeppelin marzipan", "b", baseDocs)
	require.NoError(t, err)
	require.Len(t, baseUids, baseDocs)

	// Sanity: all base docs are present before the index exists.
	resp, err := dg.Query(fmt.Sprintf(`{ q(func: has(%s)) { count(uid) } }`, pred))
	require.NoError(t, err)
	require.Contains(t, string(resp.GetJson()), fmt.Sprintf(`"count":%d`, baseDocs))

	// Kick off the background rebuild. The alter is acknowledged before
	// BuildIndexes runs, so the live writes below overlap the rebuild window.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, dg.Alter(ctx, &api.Operation{
		Schema:          pred + `: string @index(bm25) .`,
		RunInBackground: true,
	}))

	// Observability only: record whether /health?all reported the predicate as
	// indexing while the live writes were in flight, so a PASS can be
	// distinguished from "rebuild finished before the writes started".
	var sawIndexing atomic.Bool
	stopMonitor := make(chan struct{})
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopMonitor:
				return
			case <-ticker.C:
				if alphaIndexingInProgress(c, pred) {
					sawIndexing.Store(true)
				}
			}
		}
	}()

	// Live writes, all single-doc txns (< 512 edges, so the Q19 intra-proposal
	// batching race cannot confound this test), three concurrent streams.
	updateUids := baseUids[:liveUpdates]
	deleteUids := baseUids[liveUpdates : liveUpdates+liveDeletes]
	addUids := make([]string, liveAdds)
	errs := make(chan error, 3)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { // adds: new docs matching "quokka"
		defer wg.Done()
		for i := 0; i < liveAdds; i++ {
			r, err := mutateWithRetry(dg, &api.Mutation{
				SetNquads: []byte(fmt.Sprintf("_:n <%s> \"quokka lutefisk\" .", pred)),
				CommitNow: true,
			})
			if err != nil {
				errs <- fmt.Errorf("live add %d: %w", i, err)
				return
			}
			addUids[i] = r.Uids["n"]
		}
		errs <- nil
	}()
	go func() { // updates: flip existing docs from "zeppelin marzipan" to "quokka gruyere"
		defer wg.Done()
		for i, uid := range updateUids {
			_, err := mutateWithRetry(dg, &api.Mutation{
				SetNquads: []byte(fmt.Sprintf("<%s> <%s> \"quokka gruyere\" .", uid, pred)),
				CommitNow: true,
			})
			if err != nil {
				errs <- fmt.Errorf("live update %d: %w", i, err)
				return
			}
		}
		errs <- nil
	}()
	go func() { // deletes: remove existing docs entirely
		defer wg.Done()
		for i, uid := range deleteUids {
			_, err := mutateWithRetry(dg, &api.Mutation{
				DelNquads: []byte(fmt.Sprintf("<%s> <%s> * .", uid, pred)),
				CommitNow: true,
			})
			if err != nil {
				errs <- fmt.Errorf("live delete %d: %w", i, err)
				return
			}
		}
		errs <- nil
	}()
	wg.Wait()
	close(stopMonitor)
	<-monitorDone
	for i := 0; i < 3; i++ {
		require.NoError(t, <-errs)
	}
	t.Logf("Q16: indexing observed during live writes: %v "+
		"(false means the rebuild won the race and the undercount window never opened)",
		sawIndexing.Load())

	// Wait for the rebuild to finish: the bm25() query errors with "not
	// indexed" until the schema commits after BuildIndexes. Converge on the
	// full expected match count, then assert scores separately for a crisp
	// failure signature.
	lastState := ""
	converged := assert.Eventually(t, func() bool {
		nodes, err := bm25Probe(dg, pred, "quokka")
		state := ""
		if err != nil {
			state = "query error: " + err.Error()
		} else {
			state = fmt.Sprintf("%d quokka matches (want %d)", len(nodes), liveAdds+liveUpdates)
		}
		if state != lastState {
			t.Logf("Q16 convergence: %s", state)
			lastState = state
		}
		return err == nil && len(nodes) == liveAdds+liveUpdates
	}, 90*time.Second, 500*time.Millisecond)
	require.True(t, converged,
		"Q16: bm25 never returned the full live-write result set; last state: %s "+
			"(a mid-rebuild-committed doc that bm25() never finds is a Q16 failure signature)",
		lastState)

	// Final assertions against the independently computed expectation.
	quokkaNodes, err := bm25Probe(dg, pred, "quokka")
	require.NoError(t, err)
	require.Len(t, quokkaNodes, liveAdds+liveUpdates)

	expectedQuokkaUids := make(map[string]struct{}, liveAdds+liveUpdates)
	for _, uid := range addUids {
		expectedQuokkaUids[uid] = struct{}{}
	}
	for _, uid := range updateUids {
		expectedQuokkaUids[uid] = struct{}{}
	}
	for _, n := range quokkaNodes {
		_, ok := expectedQuokkaUids[n.UID]
		require.True(t, ok, "unexpected uid %s in quokka results (deleted or phantom doc)", n.UID)
	}

	wantQuokkaScore := expectedBM25Score(wantDocCount, wantQuokkaDF, wantTotalTerms)
	for _, n := range quokkaNodes {
		// KNOWN-FAILING (Q16): when live writes overlap the rebuild's
		// DropPrefix→flush window, the stats undercount makes the observed
		// score imply a docCount collapsed toward the number of live writes.
		require.InDeltaf(t, wantQuokkaScore, n.Score, 1e-6,
			"Q16 UNDERCOUNT SIGNATURE: uid %s scored %v, want %v "+
				"(closed-form with docCount=%v totalTerms=%v df=%v; a lower observed score implies "+
				"the rebuild's absolute stats flush was shadowed by a zero-based live-write RMW)",
			n.UID, n.Score, wantQuokkaScore, wantDocCount, wantTotalTerms, wantQuokkaDF)
	}

	// Cross-check via the other term: count pins deletes/updates having taken
	// effect in the term postings, score re-pins the same (N, avgDL).
	zeppelinNodes, err := bm25Probe(dg, pred, "zeppelin")
	require.NoError(t, err)
	require.Len(t, zeppelinNodes, int(wantZeppelinDF),
		"zeppelin match count wrong: deleted/updated docs must not match after rebuild")
	wantZeppelinScore := expectedBM25Score(wantDocCount, wantZeppelinDF, wantTotalTerms)
	for _, n := range zeppelinNodes {
		require.InDeltaf(t, wantZeppelinScore, n.Score, 1e-6,
			"Q16 UNDERCOUNT SIGNATURE (zeppelin channel): uid %s scored %v, want %v",
			n.UID, n.Score, wantZeppelinScore)
	}
}

// TestBM25CrashRecoveryMidRebuild — Q20-lite, expected PASS.
//
// SIGKILL the sole alpha while a fresh @index(bm25) rebuild is running, restart
// it, and require convergence: the schema mutation lives in the Raft log, so
// replay re-runs the rebuild and bm25() must eventually return the full
// expected result set with exact closed-form scores. (The forever-empty hazard
// was refuted: the schema commits only after BuildIndexes succeeds.)
func TestBM25CrashRecoveryMidRebuild(t *testing.T) {
	const (
		pred      = "crash_text"
		fillDocs  = 2950 // "zeppelin marzipan"
		probeDocs = 50   // "quokka lutefisk"
	)
	const (
		wantDocCount   = float64(fillDocs + probeDocs) // 3000
		wantTotalTerms = float64(2 * (fillDocs + probeDocs))
		wantQuokkaDF   = float64(probeDocs)
		wantZeppelinDF = float64(fillDocs)
	)

	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	dg, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, dg.DropAll())
	require.NoError(t, dg.SetupSchema(pred+`: string .`))

	_, err = loadTwoTokenDocs(dg, pred, "zeppelin marzipan", "f", fillDocs)
	require.NoError(t, err)
	probeUids, err := loadTwoTokenDocs(dg, pred, "quokka lutefisk", "p", probeDocs)
	require.NoError(t, err)
	require.Len(t, probeUids, probeDocs)

	// Start the rebuild in the background; the ack precedes BuildIndexes.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, dg.Alter(ctx, &api.Operation{
		Schema:          pred + `: string @index(bm25) .`,
		RunInBackground: true,
	}))

	// Best-effort: catch the rebuild in flight before killing. If the rebuild
	// outruns us the test degrades to a plain crash-recovery check, which is
	// still a valid (weaker) run — log which one we got.
	caughtMidRebuild := pollUntil(15*time.Second, 20*time.Millisecond, func() bool {
		return alphaIndexingInProgress(c, pred)
	})
	t.Logf("Q20: SIGKILL with indexing in progress: %v", caughtMidRebuild)

	require.NoError(t, c.KillAlpha(0))
	require.NoError(t, c.StartAlpha(0))
	require.NoError(t, c.HealthCheck(false))

	// The pre-kill client's connection may be stale; dial fresh.
	dg2, cleanup2, err := c.Client()
	require.NoError(t, err)
	defer cleanup2()

	// Converge: Raft replay must re-run the rebuild; poll until bm25 returns
	// the complete probe set with exact scores. Generous timeout, no fixed
	// sleeps.
	wantQuokkaScore := expectedBM25Score(wantDocCount, wantQuokkaDF, wantTotalTerms)
	lastState := ""
	converged := assert.Eventually(t, func() bool {
		nodes, err := bm25Probe(dg2, pred, "quokka")
		state := ""
		switch {
		case err != nil:
			state = "query error: " + err.Error()
		case len(nodes) != probeDocs:
			state = fmt.Sprintf("%d quokka matches (want %d)", len(nodes), probeDocs)
		default:
			maxDiff := 0.0
			for _, n := range nodes {
				if d := math.Abs(n.Score - wantQuokkaScore); d > maxDiff {
					maxDiff = d
				}
			}
			if maxDiff > 1e-6 {
				state = fmt.Sprintf("scores off by up to %v (want %v)", maxDiff, wantQuokkaScore)
			} else {
				state = "converged"
			}
		}
		if state != lastState {
			t.Logf("Q20 convergence: %s", state)
			lastState = state
		}
		return state == "converged"
	}, 120*time.Second, time.Second)
	require.True(t, converged,
		"Q20: bm25 never converged to the full exact result set after crash-restart; last state: %s "+
			"(schema listing bm25 while results stay empty/short would be the lost-rebuild signature)",
		lastState)

	// Final exact assertions for crisp failure messages.
	nodes, err := bm25Probe(dg2, pred, "quokka")
	require.NoError(t, err)
	require.Len(t, nodes, probeDocs)
	expectedUids := make(map[string]struct{}, probeDocs)
	for _, uid := range probeUids {
		expectedUids[uid] = struct{}{}
	}
	for _, n := range nodes {
		_, ok := expectedUids[n.UID]
		require.True(t, ok, "unexpected uid %s in post-recovery quokka results", n.UID)
		require.InDeltaf(t, wantQuokkaScore, n.Score, 1e-6,
			"post-recovery score for uid %s is %v, want closed-form %v (stats not exact after replayed rebuild)",
			n.UID, n.Score, wantQuokkaScore)
	}

	// Cross-check the other term: full corpus stats must be exact, not merely
	// the probe docs' postings.
	zeppelinNodes, err := bm25Probe(dg2, pred, "zeppelin")
	require.NoError(t, err)
	require.Len(t, zeppelinNodes, fillDocs)
	wantZeppelinScore := expectedBM25Score(wantDocCount, wantZeppelinDF, wantTotalTerms)
	for _, n := range zeppelinNodes {
		require.InDeltaf(t, wantZeppelinScore, n.Score, 1e-6,
			"post-recovery zeppelin score for uid %s is %v, want %v", n.UID, n.Score, wantZeppelinScore)
	}
}
