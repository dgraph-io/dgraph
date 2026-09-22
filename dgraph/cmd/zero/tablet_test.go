/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

func TestMoveTimeout(t *testing.T) {
	base := predicateMoveTimeout

	// Tablets small enough to move within the base timeout keep it.
	small := &pb.Tablet{OnDiskBytes: 1 << 30}
	require.Equal(t, base, moveTimeout(base, small))
	require.Equal(t, base, moveTimeout(base, &pb.Tablet{}))

	// Large tablets get size/minMoveRate.
	big := &pb.Tablet{OnDiskBytes: 20 << 30}
	require.Equal(t, time.Duration((20<<30)/minMoveRate)*time.Second, moveTimeout(base, big))

	// The larger of on-disk and uncompressed size drives the scaling.
	inflated := &pb.Tablet{OnDiskBytes: 1 << 30, UncompressedBytes: 64 << 30}
	require.Equal(t, time.Duration((64<<30)/minMoveRate)*time.Second, moveTimeout(base, inflated))
}

func TestMoveCooldown(t *testing.T) {
	require.Equal(t, time.Hour, moveCooldown(1, time.Minute))
	require.Equal(t, 2*time.Hour, moveCooldown(2, time.Minute))
	require.Equal(t, 16*time.Hour, moveCooldown(5, time.Minute))
	require.Equal(t, moveBackoffMax, moveCooldown(6, time.Minute))
	// Large failure counts must not overflow the doubling.
	require.Equal(t, moveBackoffMax, moveCooldown(100, time.Minute))
	// An attempt that outlasted the doubling cooldown sets the floor.
	require.Equal(t, 30*time.Hour, moveCooldown(1, 30*time.Hour))
}

func TestMoveBackoff(t *testing.T) {
	s := &Server{moveBackoff: new(sync.Map)}
	pred := "name"
	errMove := errors.New("context deadline exceeded")

	require.False(t, s.skipMove(pred))

	// Quick validation failures are cheap to retry and set no backoff.
	s.recordMoveResult(pred, 5*time.Second, errMove)
	require.False(t, s.skipMove(pred))

	// A failure that did real work does.
	s.recordMoveResult(pred, 2*time.Hour, errMove)
	require.True(t, s.skipMove(pred))

	// Other tablets are unaffected.
	require.False(t, s.skipMove("other"))

	// A successful move clears the backoff.
	s.recordMoveResult(pred, time.Minute, nil)
	require.False(t, s.skipMove(pred))
}

// TestRebalanceDisabled checks that a zero rebalance interval turns the automatic rebalancer off:
// the loop must return instead of ticking (or, as time.Tick(0) would have it, blocking forever).
func TestRebalanceDisabled(t *testing.T) {
	prev := opts.rebalanceInterval
	opts.rebalanceInterval = 0
	t.Cleanup(func() { opts.rebalanceInterval = prev })

	done := make(chan struct{})
	go func() {
		(&Server{}).rebalanceTablets()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("rebalanceTablets must return immediately when rebalance_interval is 0")
	}
}

func TestCancelMove(t *testing.T) {
	s := &Server{inflightMoves: new(sync.Map)}
	pred := "0-name"

	_, err := s.CancelMove(pred)
	require.Error(t, err, "cancelling a predicate with no move in flight must fail")

	ctx, cancel := context.WithCancelCause(context.Background())
	untrack := s.trackMove(pred, 1, 2, cancel)
	require.NoError(t, ctx.Err())

	msg, err := s.CancelMove(pred)
	require.NoError(t, err)
	require.Contains(t, msg, pred)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.ErrorIs(t, context.Cause(ctx), errMoveCancelled)

	// A move is cancelled once; other tablets are unaffected.
	_, err = s.CancelMove(pred)
	require.Error(t, err)
	_, err = s.CancelMove("0-other")
	require.Error(t, err)
	untrack()

	// A move that finished, or passed the point of no return, can no longer be cancelled.
	ctx, cancel = context.WithCancelCause(context.Background())
	untrack = s.trackMove(pred, 1, 2, cancel)
	untrack()
	_, err = s.CancelMove(pred)
	require.Error(t, err)
	require.NoError(t, ctx.Err())

	// Untracking a stale registration must not drop a newer move of the same predicate.
	staleCtx, staleCancel := context.WithCancelCause(context.Background())
	stale := s.trackMove(pred, 1, 2, staleCancel)
	ctx, cancel = context.WithCancelCause(context.Background())
	defer s.trackMove(pred, 2, 1, cancel)()
	stale()
	_, err = s.CancelMove(pred)
	require.NoError(t, err)
	require.NoError(t, staleCtx.Err())
	require.ErrorIs(t, context.Cause(ctx), errMoveCancelled)
}

func TestCancelMoveHandler(t *testing.T) {
	st := &state{zero: &Server{inflightMoves: new(sync.Map)}}
	do := func(method, target string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		st.cancelMove(rr, httptest.NewRequest(method, target, nil))
		return rr
	}

	require.Equal(t, http.StatusBadRequest, do(http.MethodPost, "/cancelMove?tablet=name").Code,
		"only GET is accepted, like /moveTablet")
	require.Equal(t, http.StatusBadRequest, do(http.MethodGet, "/cancelMove").Code,
		"tablet is mandatory")
	require.Equal(t, http.StatusBadRequest, do(http.MethodGet, "/cancelMove?tablet=name&namespace=x").Code,
		"namespace must be an integer")
	require.Equal(t, http.StatusBadRequest, do(http.MethodGet, "/cancelMove?tablet=name").Code,
		"no move in flight")

	ctx, cancel := context.WithCancelCause(context.Background())
	defer st.zero.trackMove(x.NamespaceAttr(x.RootNamespace, "name"), 1, 2, cancel)()
	rr := do(http.MethodGet, "/cancelMove?tablet=name")
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "from group 1 to 2")
	require.ErrorIs(t, context.Cause(ctx), errMoveCancelled)

	// Tablets outside the root namespace are addressed with the namespace query parameter.
	ctx, cancel = context.WithCancelCause(context.Background())
	defer st.zero.trackMove(x.NamespaceAttr(5, "name"), 1, 2, cancel)()
	require.Equal(t, http.StatusBadRequest, do(http.MethodGet, "/cancelMove?tablet=name").Code)
	require.Equal(t, http.StatusOK, do(http.MethodGet, "/cancelMove?tablet=name&namespace=5").Code)
	require.ErrorIs(t, context.Cause(ctx), errMoveCancelled)
}
