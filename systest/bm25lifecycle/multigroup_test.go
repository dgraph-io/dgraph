//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Wave-2 distributed lifecycle tests for native hybrid search (fuse + hybrid).
//
// Q17 TestHybridMultiGroupParity: 3 alphas / replicas=1 (3 groups) with the
// bm25 text predicate and the HNSW vector predicate forced onto DIFFERENT
// groups (zero /moveTablet), versus a 1-alpha control cluster loaded with the
// identical corpus. The same hybrid()/fuse()/single-channel query battery must
// return the identical uid order and scores (1e-9) on both topologies. Any
// divergence localizes to channel content (single-channel legs), not fusion.
//
// Q19 TestBM25ReplicaConvergenceUnderFailure: 3 alphas / replicas=3 (one
// group). Mixed mutation load with a follower stopped mid-load and restarted;
// every replica must then serve identical bm25 rows (each alpha queried
// directly). A >=512-edge single-txn probe then exercises the confirmed
// intra-proposal parallel stats RMW race, and finally the group leader is
// SIGKILLed and post-failover scores must be unchanged.
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
)

// ---------------------------------------------------------------------------
// Shared helpers (mg*/rc* prefixes to avoid collisions with sibling files in
// this package).
// ---------------------------------------------------------------------------

// mgRow is one {uid, val(f)} row of a score-ordered result block named "q".
type mgRow struct {
	UID   string  `json:"uid"`
	Score float64 `json:"val(f)"`
}

func mgQueryRows(dg *dgraphapi.GrpcClient, query string) ([]mgRow, error) {
	resp, err := dg.Query(query)
	if err != nil {
		return nil, err
	}
	var out struct {
		Q []mgRow `json:"q"`
	}
	if err := json.Unmarshal(resp.GetJson(), &out); err != nil {
		return nil, err
	}
	return out.Q, nil
}

// mgCanonical returns rows sorted by (score desc, uid asc). Score ties are
// possible by construction (RRF gives byte-equal scores to docs holding equal
// ranks in disjoint channels) and the server's sort is not guaranteed stable
// across processes, so all cross-process comparisons are done on this
// canonical form; the per-uid scores and the score-distinct portion of the
// ordering keep full discriminating power.
func mgCanonical(rows []mgRow) []mgRow {
	out := make([]mgRow, len(rows))
	copy(out, rows)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].UID < out[j].UID
	})
	return out
}

func mgRowsEqual(a, b []mgRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].UID != b[i].UID || a[i].Score != b[i].Score {
			return false
		}
	}
	return true
}

func mgSortedScores(rows []mgRow) []float64 {
	out := make([]float64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Score)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(out)))
	return out
}

// mgAlphaMembership fetches an alpha's /state and parses the full membership
// snapshot (groups, tablets, members incl. leader flags).
func mgAlphaMembership(hc *dgraphapi.HTTPClient) (*pb.MembershipState, error) {
	raw, err := hc.GetAlphaState()
	if err != nil {
		return nil, err
	}
	var state pb.MembershipState
	if err := protojson.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// mgTabletGroup returns the group serving pred (tablet keys may be plain or
// namespaced like "0-pred"), or 0 if unassigned.
func mgTabletGroup(state *pb.MembershipState, pred string) uint32 {
	for gid, group := range state.Groups {
		for key := range group.Tablets {
			if key == pred || strings.HasSuffix(key, "-"+pred) {
				return gid
			}
		}
	}
	return 0
}

// mgAlphaIdxFromAddr maps a member address like "alpha2:7080" to the
// LocalCluster alpha index (aliasName format is "alpha%d", dgraphtest/dgraph.go).
func mgAlphaIdxFromAddr(addr string) int {
	if !strings.HasPrefix(addr, "alpha") {
		return -1
	}
	numStr := strings.Split(strings.TrimPrefix(addr, "alpha"), ":")[0]
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return -1
	}
	return n
}

func mgLogTablets(t *testing.T, hc *dgraphapi.HTTPClient, label string) {
	t.Helper()
	state, err := mgAlphaMembership(hc)
	if err != nil {
		t.Logf("%s: could not fetch membership state: %v", label, err)
		return
	}
	for gid, group := range state.Groups {
		for key := range group.Tablets {
			t.Logf("%s: group %d serves tablet %q", label, gid, key)
		}
	}
}

// ---------------------------------------------------------------------------
// Q17: multi-group parity for hybrid()/fuse().
// ---------------------------------------------------------------------------

const mgSchema = `
	mgp_text: string @index(bm25) .
	mgp_vec: float32vector @index(hnsw(metric:"euclidean")) .
`

// mgCorpusNquads builds 96 docs (3 full uid%32 bucket cycles) with explicit
// uids so both clusters hold byte-identical data. Text has varying "fox"/"dog"
// term frequencies and doc lengths; vectors are distinct 4-dim points.
func mgCorpusNquads() string {
	var b strings.Builder
	for i := 0; i < 96; i++ {
		uid := 0x1000 + i
		text := strings.TrimSpace(
			strings.Repeat("fox ", i%5+1) + strings.Repeat("dog ", i%3) + fmt.Sprintf("word%d", i%11))
		fmt.Fprintf(&b, "<%#x> <mgp_text> %q .\n", uid, text)
		fmt.Fprintf(&b, "<%#x> <mgp_vec> \"[%d.0, %d.0, %d.0, 1.0]\" .\n", uid, i%13, (i*7)%17, i%5)
	}
	return b.String()
}

// mgParityBattery is the identical query set run against both topologies. The
// single-channel legs localize any divergence to a channel; the fuse/hybrid
// legs assert coordinator-side fusion parity on top.
var mgParityBattery = []struct {
	name string
	// exact: demand bit-identical cross-topology parity. Only the bm25 leg is
	// exact — it is a deterministic algorithm over identical data. Legs touching
	// similar_to are APPROXIMATE: the two topologies build independent HNSW
	// graphs (different insertion interleaving), which legitimately differ on
	// boundary-rank candidates. For those legs we assert the invariants the
	// product does promise: row counts, score-descending order, >= n-1 uid-set
	// overlap (one boundary swap allowed), and — the misbinding detector —
	// bit-equal scores for every uid both topologies return.
	exact bool
	// scoreCheck: per-uid scores for uids BOTH topologies return must be equal.
	// True for single-channel legs (bm25 is deterministic; a similarity score is
	// a pure function of stored vector + query vector, so equality holds even
	// when the approximate graphs disagree on boundary candidates). False for
	// fused legs: fused scores inherit rank noise from the approximate vector
	// channel (a boundary uid in one topology's top-k but not the other's shifts
	// every RRF contribution), and per-uid fusion correctness is already pinned
	// by the single-cluster oracle test in query/query_hybrid_test.go.
	scoreCheck bool
	query      string
}{
	{"bm25_channel_only", true, true, `{
		f as var(func: bm25(mgp_text, "fox dog"))
		q(func: uid(f), orderdesc: val(f)) { uid val(f) }
	}`},
	{"vector_channel_only", false, true, `{
		f as var(func: similar_to(mgp_vec, 12, "[1.0, 2.0, 3.0, 4.0]"))
		q(func: uid(f), orderdesc: val(f)) { uid val(f) }
	}`},
	{"fuse_rrf", false, false, `{
		txt as var(func: bm25(mgp_text, "fox dog"))
		vec as var(func: similar_to(mgp_vec, 12, "[1.0, 2.0, 3.0, 4.0]"))
		f as var(func: fuse(txt, vec, method: "rrf", k: 60))
		q(func: uid(f), orderdesc: val(f)) { uid val(f) }
	}`},
	{"fuse_linear", false, false, `{
		txt as var(func: bm25(mgp_text, "fox dog"))
		vec as var(func: similar_to(mgp_vec, 12, "[1.0, 2.0, 3.0, 4.0]"))
		f as var(func: fuse(txt, vec, method: "linear", weights: "0.3,0.7", normalize: "max"))
		q(func: uid(f), orderdesc: val(f)) { uid val(f) }
	}`},
	{"hybrid_sugar", false, false, `{
		f as var(func: hybrid(mgp_text, "fox dog", mgp_vec, "[1.0, 2.0, 3.0, 4.0]", topk: 12, method: "rrf", k: 60))
		q(func: uid(f), orderdesc: val(f)) { uid val(f) }
	}`},
}

func TestHybridMultiGroupParity(t *testing.T) {
	// 3 alphas x replicas=1 => three single-member groups.
	conf3 := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(1).WithReplicas(1)
	c3, err := dgraphtest.NewLocalCluster(conf3)
	require.NoError(t, err)
	defer func() { c3.Cleanup(t.Failed()) }()
	require.NoError(t, c3.Start())

	// 1-alpha control cluster: same data, trivially colocated tablets.
	conf1 := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c1, err := dgraphtest.NewLocalCluster(conf1)
	require.NoError(t, err)
	defer func() { c1.Cleanup(t.Failed()) }()
	require.NoError(t, c1.Start())

	dg3, cleanup3, err := c3.Client()
	require.NoError(t, err)
	defer cleanup3()
	dg1, cleanup1, err := c1.Client()
	require.NoError(t, err)
	defer cleanup1()

	require.NoError(t, dg3.DropAll())
	require.NoError(t, dg1.DropAll())
	require.NoError(t, dg3.SetupSchema(mgSchema))
	require.NoError(t, dg1.SetupSchema(mgSchema))
	time.Sleep(2 * time.Second) // post-alter settling, as existing systest does

	hc3, err := c3.HTTPClient()
	require.NoError(t, err)

	// Force mgp_text and mgp_vec onto DIFFERENT groups before any data lands.
	// The move attempt lives inside the poll: retried until zero's state shows
	// the split (MoveTablet is a no-op error once the tablet already moved).
	var textGid, vecGid uint32
	require.Eventually(t, func() bool {
		state, err := mgAlphaMembership(hc3)
		if err != nil {
			return false
		}
		textGid = mgTabletGroup(state, "mgp_text")
		vecGid = mgTabletGroup(state, "mgp_vec")
		if textGid == 0 || vecGid == 0 {
			return false
		}
		if textGid != vecGid {
			return true
		}
		var target uint32
		for gid := range state.Groups {
			if gid != 0 && gid != textGid {
				target = gid
				break
			}
		}
		if target == 0 {
			return false
		}
		if err := hc3.MoveTablet("mgp_vec", target); err != nil {
			t.Logf("moveTablet(mgp_vec -> group %d): %v (will re-check)", target, err)
		}
		return false
	}, 60*time.Second, 2*time.Second,
		"mgp_text and mgp_vec must end up on different groups")
	t.Logf("placement: mgp_text on group %d, mgp_vec on group %d", textGid, vecGid)

	// Identical corpus into both clusters. 192 edges per txn (< 512) keeps the
	// proposal application serial, so this load is not subject to the Q19 race.
	corpus := mgCorpusNquads()
	_, err = dg3.Mutate(&api.Mutation{SetNquads: []byte(corpus), CommitNow: true})
	require.NoError(t, err)
	_, err = dg1.Mutate(&api.Mutation{SetNquads: []byte(corpus), CommitNow: true})
	require.NoError(t, err)

	// Observe where the HNSW auxiliary __vector_* tablets landed (Q17's
	// statically undecidable hazard). Logged for diagnosis; the parity battery
	// below is the load-bearing assertion.
	mgLogTablets(t, hc3, "3-group cluster post-load")

	// Wait until the (possibly just-moved) vector tablet is queryable on both
	// clusters: membership propagation to the alphas can lag the move, and
	// ErrNonExistentTablet silently yields empty results in the interim.
	vecProbe := mgParityBattery[1].query
	require.Eventually(t, func() bool {
		r3, err3 := mgQueryRows(dg3, vecProbe)
		r1, err1 := mgQueryRows(dg1, vecProbe)
		return err3 == nil && err1 == nil && len(r3) == 12 && len(r1) == 12
	}, 60*time.Second, 2*time.Second,
		"similar_to must return topk results on both clusters before the parity battery")

	for _, tc := range mgParityBattery {
		t.Run(tc.name, func(t *testing.T) {
			rows3, err := mgQueryRows(dg3, tc.query)
			require.NoError(t, err)
			rows1, err := mgQueryRows(dg1, tc.query)
			require.NoError(t, err)
			require.NotEmpty(t, rows1, "control cluster returned no rows")
			require.NotEmpty(t, rows3, "3-group cluster returned no rows")

			// Server-side ordering must be score-descending on both.
			for i := 1; i < len(rows3); i++ {
				require.GreaterOrEqual(t, rows3[i-1].Score, rows3[i].Score,
					"3-group rows not score-descending at index %d", i)
			}
			for i := 1; i < len(rows1); i++ {
				require.GreaterOrEqual(t, rows1[i-1].Score, rows1[i].Score,
					"control rows not score-descending at index %d", i)
			}

			want := mgCanonical(rows1)
			got := mgCanonical(rows3)
			require.Equal(t, len(want), len(got),
				"row count differs between topologies")

			if tc.exact {
				// Deterministic leg: identical uid order and scores (1e-9), on the
				// canonical form (score desc, uid asc) so exact-tie permutations
				// cannot flake.
				for i := range want {
					require.Equalf(t, want[i].UID, got[i].UID,
						"uid order diverges at rank %d (control %s / 3-group %s)",
						i, want[i].UID, got[i].UID)
					require.InDeltaf(t, want[i].Score, got[i].Score, 1e-9,
						"score diverges for uid %s at rank %d", want[i].UID, i)
				}
				return
			}

			// Approximate leg (touches HNSW): allow one boundary-rank swap between
			// the independently built graphs, but scores for uids BOTH return must
			// be bit-close — a mismatch there is score misbinding, not
			// approximation.
			wantScores := make(map[string]float64, len(want))
			for _, r := range want {
				wantScores[r.UID] = r.Score
			}
			shared := 0
			for _, r := range got {
				if ws, ok := wantScores[r.UID]; ok {
					shared++
					if tc.scoreCheck {
						require.InDeltaf(t, ws, r.Score, 1e-9,
							"score for uid %s differs between topologies (misbinding)", r.UID)
					}
				}
			}
			require.GreaterOrEqualf(t, shared, len(want)-1,
				"uid overlap %d/%d below n-1: more than a boundary-rank divergence",
				shared, len(want))
		})
	}
}

// ---------------------------------------------------------------------------
// Q19: single-group replica convergence under follower outage, heavy-txn
// stats race, and leader failover.
// ---------------------------------------------------------------------------

func rcDocNquads(start, count int) string {
	var b strings.Builder
	for i := start; i < start+count; i++ {
		uid := 0x2000 + i
		text := strings.TrimSpace(
			strings.Repeat("zephyr ", i%4+1) + strings.Repeat("quill ", i%3) + fmt.Sprintf("tag%d", i%9))
		fmt.Fprintf(&b, "<%#x> <rc_text> %q .\n", uid, text)
	}
	return b.String()
}

func rcMutateBatches(dg *dgraphapi.GrpcClient, startDoc, batches, perBatch int) error {
	for b := 0; b < batches; b++ {
		nq := rcDocNquads(startDoc+b*perBatch, perBatch)
		if _, err := dg.Mutate(&api.Mutation{SetNquads: []byte(nq), CommitNow: true}); err != nil {
			return err
		}
	}
	return nil
}

func rcRankQuery(pred, terms string) string {
	return fmt.Sprintf(`{
	f as var(func: bm25(%s, %q))
	q(func: uid(f), orderdesc: val(f)) { uid val(f) }
}`, pred, terms)
}

const rcScoreQuery = `{
	f as var(func: bm25(rc_text, "zephyr quill"))
	q(func: uid(f), orderdesc: val(f)) { uid val(f) }
}`

func TestBM25ReplicaConvergenceUnderFailure(t *testing.T) {
	// 3 alphas x replicas=3 => one group, three replicas.
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(1).WithReplicas(3)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	dg, cleanupAll, err := c.Client()
	require.NoError(t, err)
	defer cleanupAll()

	require.NoError(t, dg.DropAll())
	// Explicit uids go up to 0xA000+1024 (~42k) in the heavy-txn subtest; zero's
	// default lease is far below that, so extend it up front.
	require.NoError(t, c.AssignUids(dg.Dgraph, 50000))
	require.NoError(t, dg.SetupSchema(`rc_text: string @index(bm25) .`))
	time.Sleep(2 * time.Second) // post-alter settling

	// Phase A: 8 x 20-doc txns (docs 0..159) with all replicas up. Every txn
	// stays far below the 512-edge parallel-apply threshold, so replicas apply
	// stats strictly serially and MUST agree.
	require.NoError(t, rcMutateBatches(dg, 0, 8, 20))

	// Identify the group leader and the two followers.
	hc, err := c.HTTPClient()
	require.NoError(t, err)
	leaderIdx := -1
	var followerIdxs []int
	require.Eventually(t, func() bool {
		state, err := mgAlphaMembership(hc)
		if err != nil {
			return false
		}
		leaderIdx = -1
		followerIdxs = followerIdxs[:0]
		for _, group := range state.Groups {
			for _, m := range group.Members {
				idx := mgAlphaIdxFromAddr(m.Addr)
				if idx < 0 {
					return false
				}
				if m.Leader {
					if leaderIdx >= 0 {
						return false // stale double-leader view; re-poll
					}
					leaderIdx = idx
				} else {
					followerIdxs = append(followerIdxs, idx)
				}
			}
		}
		return leaderIdx >= 0 && len(followerIdxs) == 2
	}, 60*time.Second, 2*time.Second, "must observe one leader and two followers")
	sort.Ints(followerIdxs)
	stopIdx, pinIdx := followerIdxs[0], followerIdxs[1]
	t.Logf("leader alpha %d; stopping follower alpha %d; pinning writes to follower alpha %d",
		leaderIdx, stopIdx, pinIdx)

	// dgPin targets an alpha that is never stopped or killed, so the mixed load
	// keeps flowing while a follower is down and after the leader dies.
	dgPin, cleanupPin, err := c.AlphaClient(pinIdx)
	require.NoError(t, err)
	defer cleanupPin()

	// Phase B: stop a follower mid-load; insert docs 160..319 and rewrite the
	// text (paired DEL+SET stats path) of docs 0..19 while it is down.
	require.NoError(t, c.StopAlpha(stopIdx))
	require.NoError(t, rcMutateBatches(dgPin, 160, 8, 20))
	var upd strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&upd, "<%#x> <rc_text> %q .\n", 0x2000+i,
			fmt.Sprintf("quill quill zephyr renumbered%d", i))
	}
	_, err = dgPin.Mutate(&api.Mutation{SetNquads: []byte(upd.String()), CommitNow: true})
	require.NoError(t, err)

	// Phase C: restart the follower, then keep mutating: insert docs 320..479
	// and delete docs 20..29 entirely (S P * on an un-lang-tagged predicate).
	require.NoError(t, c.StartAlpha(stopIdx))
	require.NoError(t, rcMutateBatches(dgPin, 320, 8, 20))
	var del strings.Builder
	for i := 20; i < 30; i++ {
		fmt.Fprintf(&del, "<%#x> <rc_text> * .\n", 0x2000+i)
	}
	_, err = dgPin.Mutate(&api.Mutation{DelNquads: []byte(del.String()), CommitNow: true})
	require.NoError(t, err)

	// Reference ranking from the pinned alpha after the load quiesces.
	// 470 live docs all match "zephyr".
	var ref []mgRow
	require.Eventually(t, func() bool {
		rows, err := mgQueryRows(dgPin, rcScoreQuery)
		if err != nil || len(rows) != 470 {
			return false
		}
		ref = mgCanonical(rows)
		return true
	}, 60*time.Second, 2*time.Second, "pinned alpha must serve the full corpus")

	// Q19 assertion 1: every replica — including the stopped-and-restarted
	// follower, which must catch up over Raft — serves the identical ranking
	// with identical (bitwise) scores. Expected to PASS: all txns above were
	// below the parallel-apply threshold.
	t.Run("AllReplicasIdenticalAfterCatchup", func(t *testing.T) {
		for idx := 0; idx < 3; idx++ {
			require.Eventuallyf(t, func() bool {
				dgi, cl, err := c.AlphaClient(idx)
				if err != nil {
					return false
				}
				defer cl()
				rows, err := mgQueryRows(dgi, rcScoreQuery)
				if err != nil {
					return false
				}
				return mgRowsEqual(ref, mgCanonical(rows))
			}, 120*time.Second, 2*time.Second,
				"alpha %d must converge to the reference bm25 ranking", idx)
		}
	})

	// Q19 assertion 2 (confirmed-vulnerable probe): one >=512-edge proposal
	// splits into concurrent apply goroutines whose unlocked stats
	// read-modify-writes on shared uid%32 buckets can lose contributions
	// (worker/draft.go:561-601 + posting/bm25.go:236-263). Oracle: the same
	// 1024 texts loaded as 32-doc txns must yield the identical sorted score
	// sequence; replicas must also agree among themselves. EXPECTED TO FAIL
	// (possibly intermittently — the race is scheduling-dependent).
	t.Run("HeavySingleTxnStatsOracle", func(t *testing.T) {
		require.NoError(t, dgPin.SetupSchema(`
			rc_heavy: string @index(bm25) .
			rc_heavy_ctl: string @index(bm25) .`))
		time.Sleep(2 * time.Second) // post-alter settling

		var heavy, ctl strings.Builder
		for i := 0; i < 1024; i++ {
			text := strings.TrimSpace(
				strings.Repeat("gryphon ", i%6+1) + fmt.Sprintf("mark%d", i%13))
			fmt.Fprintf(&heavy, "<%#x> <rc_heavy> %q .\n", 0x8000+i, text)
			fmt.Fprintf(&ctl, "<%#x> <rc_heavy_ctl> %q .\n", 0xA000+i, text)
		}
		// Heavy: 1024 edges in ONE txn => parallel apply on every replica.
		_, err := dgPin.Mutate(&api.Mutation{SetNquads: []byte(heavy.String()), CommitNow: true})
		require.NoError(t, err)
		// Control: identical texts (same uid%32 bucket distribution) in 32-doc
		// txns => serial apply, exact stats.
		ctlLines := strings.Split(strings.TrimSpace(ctl.String()), "\n")
		for i := 0; i < len(ctlLines); i += 32 {
			end := i + 32
			if end > len(ctlLines) {
				end = len(ctlLines)
			}
			_, err := dgPin.Mutate(&api.Mutation{
				SetNquads: []byte(strings.Join(ctlLines[i:end], "\n")),
				CommitNow: true,
			})
			require.NoError(t, err)
		}

		heavyQ := rcRankQuery("rc_heavy", "gryphon")
		ctlQ := rcRankQuery("rc_heavy_ctl", "gryphon")
		var refHeavy []mgRow
		for idx := 0; idx < 3; idx++ {
			var heavyRows, ctlRows []mgRow
			require.Eventuallyf(t, func() bool {
				dgi, cl, err := c.AlphaClient(idx)
				if err != nil {
					return false
				}
				defer cl()
				h, err := mgQueryRows(dgi, heavyQ)
				if err != nil || len(h) != 1024 {
					return false
				}
				cr, err := mgQueryRows(dgi, ctlQ)
				if err != nil || len(cr) != 1024 {
					return false
				}
				heavyRows, ctlRows = h, cr
				return true
			}, 90*time.Second, 2*time.Second,
				"alpha %d must serve both heavy and control corpora", idx)

			// Within-alpha oracle: identical corpora => identical score sets.
			// A lost stats RMW shifts N/avgDL and therefore every score.
			require.Equalf(t, mgSortedScores(ctlRows), mgSortedScores(heavyRows),
				"alpha %d: 1024-doc single-txn corpus scores differ from the serially-loaded "+
					"identical corpus — intra-proposal parallel stats RMW lost an update (Q19/Q15)", idx)

			// Cross-replica identity for the heavy corpus.
			canon := mgCanonical(heavyRows)
			if idx == 0 {
				refHeavy = canon
			} else {
				require.Truef(t, mgRowsEqual(refHeavy, canon),
					"alpha %d heavy-corpus ranking diverges from alpha 0 — replicas resolved "+
						"the stats race differently (Q19)", idx)
			}
		}
	})

	// Q19 assertion 3: SIGKILL the group leader; after failover the two
	// surviving replicas must serve the unchanged reference ranking. Expected
	// to PASS.
	t.Run("LeaderFailoverScoresUnchanged", func(t *testing.T) {
		require.NoError(t, c.KillAlpha(leaderIdx))
		for _, idx := range []int{stopIdx, pinIdx} {
			require.Eventuallyf(t, func() bool {
				dgi, cl, err := c.AlphaClient(idx)
				if err != nil {
					return false
				}
				defer cl()
				rows, err := mgQueryRows(dgi, rcScoreQuery)
				if err != nil {
					return false
				}
				return mgRowsEqual(ref, mgCanonical(rows))
			}, 120*time.Second, 2*time.Second,
				"alpha %d must serve the unchanged ranking after leader failover", idx)
		}
	})
}
