/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
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
