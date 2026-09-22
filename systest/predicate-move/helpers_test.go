//go:build integration2 || largemove

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Helpers shared by the predicate move tests in this package: the CI-run cancellation test
// (integration2) and the manually-invoked large move test (largemove).
package main

import (
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
)

const (
	predicate = "payload"

	// rawValueSize bytes of random data per triple, base64-encoded to valueSize on the wire.
	// 48KiB is divisible by 3, so the encoding is exactly 64KiB with no padding.
	rawValueSize = 48 << 10
	valueSize    = 64 << 10
	batchSize    = 32 // triples per mutation, ~2MiB per txn
	loaders      = 8
)

// loadPayload writes incompressible base64 values under the payload predicate until targetBytes
// of value data has been committed, and returns the number of triples written.
func loadPayload(t *testing.T, gc *dgraphapi.GrpcClient, targetBytes int64) int64 {
	var loaded, triples atomic.Int64
	errCh := make(chan error, loaders)
	done := make(chan struct{})
	var wg sync.WaitGroup

	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		start := time.Now()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				n := loaded.Load()
				rate := float64(n) / (1 << 20) / time.Since(start).Seconds()
				t.Logf("loaded %.1f GiB of %.1f GiB (%.0f MiB/s)",
					float64(n)/(1<<30), float64(targetBytes)/(1<<30), rate)
			}
		}
	}()

	for w := 0; w < loaders; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw := make([]byte, rawValueSize)
			for loaded.Load() < targetBytes {
				var b strings.Builder
				b.Grow(batchSize * (valueSize + 64))
				for i := 0; i < batchSize; i++ {
					if _, err := crand.Read(raw); err != nil {
						errCh <- err
						return
					}
					fmt.Fprintf(&b, "_:b%d <%s> \"%s\" .\n",
						i, predicate, base64.StdEncoding.EncodeToString(raw))
				}
				if _, err := gc.Mutate(&api.Mutation{
					SetNquads: []byte(b.String()),
					CommitNow: true,
				}); err != nil {
					errCh <- err
					return
				}
				loaded.Add(int64(batchSize * valueSize))
				triples.Add(batchSize)
			}
		}()
	}
	wg.Wait()
	close(done)
	select {
	case err := <-errCh:
		require.NoError(t, err, "loader failed")
	default:
	}
	return triples.Load()
}

// membershipState returns the cluster membership state as seen through the Alpha behind hc.
func membershipState(t *testing.T, hc *dgraphapi.HTTPClient) *pb.MembershipState {
	stateBytes, err := hc.GetAlphaState()
	require.NoError(t, err)
	state := &pb.MembershipState{}
	require.NoError(t, protojson.Unmarshal(stateBytes, state))
	return state
}

// lookupTablet returns the payload tablet and the group serving it from the membership state as
// seen through the Alpha behind hc, or false if that Alpha does not know the tablet yet.
func lookupTablet(t *testing.T, hc *dgraphapi.HTTPClient) (*pb.Tablet, uint32, bool) {
	state := membershipState(t, hc)
	for gid, group := range state.Groups {
		for name, tab := range group.Tablets {
			if name == predicate || name == "0-"+predicate {
				return tab, gid, true
			}
		}
	}
	return nil, 0, false
}

// findTablet returns the payload tablet and the group currently serving it, from the membership
// state as seen through the Alpha behind hc. The tablet must already be known to that Alpha.
func findTablet(t *testing.T, hc *dgraphapi.HTTPClient) (*pb.Tablet, uint32) {
	tab, gid, ok := lookupTablet(t, hc)
	require.True(t, ok, "tablet %q not found in membership state", predicate)
	return tab, gid
}

// waitForTablet polls until the Alpha behind hc knows the payload tablet, which happens shortly
// after the first write to the predicate: Zero assigns the tablet on that write, and Alphas learn
// about it through Zero's membership stream.
func waitForTablet(t *testing.T, hc *dgraphapi.HTTPClient, timeout time.Duration) (*pb.Tablet, uint32) {
	deadline := time.Now().Add(timeout)
	for {
		if tab, gid, ok := lookupTablet(t, hc); ok {
			return tab, gid
		}
		require.False(t, time.Now().After(deadline),
			"tablet %q did not appear in the membership state within %v", predicate, timeout)
		time.Sleep(500 * time.Millisecond)
	}
}

// waitForTabletGroup polls the membership state until the payload tablet is served by gid.
func waitForTabletGroup(t *testing.T, hc *dgraphapi.HTTPClient, gid uint32, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		_, group := findTablet(t, hc)
		if group == gid {
			return
		}
		require.False(t, time.Now().After(deadline),
			"tablet must be served by group %d after the move, still on group %d", gid, group)
		time.Sleep(2 * time.Second)
	}
}

// retryMove asks Zero to move the payload tablet to gid until a move succeeds or timeout passes.
// Attempts right after a failed move can fail quickly while the source Alpha's previous move task
// winds down or Zero reconnects to a restarted group; those failures are expected.
func retryMove(t *testing.T, hc *dgraphapi.HTTPClient, gid uint32, timeout time.Duration) {
	start := time.Now()
	deadline := start.Add(timeout)
	for {
		err := hc.MoveTablet(predicate, gid)
		if err == nil {
			t.Logf("move completed in %v", time.Since(start).Round(time.Second))
			return
		}
		require.False(t, time.Now().After(deadline),
			"move did not succeed within %v, last error: %v", timeout, err)
		t.Logf("retrying move: %v", err)
		time.Sleep(5 * time.Second)
	}
}

// waitForHealth polls the cluster health check until it passes or timeout expires.
func waitForHealth(t *testing.T, c *dgraphtest.LocalCluster, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		err := c.HealthCheck(false)
		if err == nil {
			return
		}
		require.False(t, time.Now().After(deadline),
			"cluster did not become healthy within %v: %v", timeout, err)
		time.Sleep(5 * time.Second)
	}
}

// otherGroup returns a group id different from gid.
func otherGroup(t *testing.T, state *pb.MembershipState, gid uint32) uint32 {
	for g := range state.Groups {
		if g != gid && g > 0 {
			return g
		}
	}
	t.Fatalf("no group other than %d in membership state", gid)
	return 0
}

// alphaInGroup returns the container index of an Alpha serving the given group, parsed from the
// member address (e.g. "alpha1:7080").
func alphaInGroup(t *testing.T, state *pb.MembershipState, gid uint32) int {
	group, ok := state.Groups[gid]
	require.True(t, ok, "group %d not in membership state", gid)
	for _, member := range group.Members {
		addr := strings.TrimPrefix(member.Addr, "alpha")
		if addr == member.Addr {
			continue
		}
		if n, err := strconv.Atoi(strings.Split(addr, ":")[0]); err == nil && n >= 0 {
			return n
		}
	}
	t.Fatalf("no alpha member found for group %d", gid)
	return -1
}

// countPayload counts nodes carrying the payload predicate, queried through the given Alpha.
func countPayload(t *testing.T, c *dgraphtest.LocalCluster, alphaIdx int) int64 {
	gc, cleanup, err := c.AlphaClient(alphaIdx)
	require.NoError(t, err)
	defer cleanup()

	resp, err := gc.Query(`{ q(func: has(` + predicate + `)) { count(uid) } }`)
	require.NoError(t, err)
	var out struct {
		Q []struct {
			Count int64 `json:"count"`
		} `json:"q"`
	}
	require.NoError(t, json.Unmarshal(resp.Json, &out))
	require.Len(t, out.Q, 1)
	return out.Q[0].Count
}
