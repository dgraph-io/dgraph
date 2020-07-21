// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"sync"

	log "github.com/ChainSafe/log15"
)

type requestTracker struct {
	logger   log.Logger
	blockIDs *sync.Map // track BlockRequest IDs that we have requested
}

// newRequestTracker returns a new requestTracker
func newRequestTracker(logger log.Logger) *requestTracker {
	return &requestTracker{
		logger:   logger.New("module", "syncer"),
		blockIDs: &sync.Map{},
	}
}

// addRequestedBlockID adds a requested block id to non-persistent state
func (t *requestTracker) addRequestedBlockID(blockID uint64) {
	t.logger.Trace("Adding block to network syncer...", "block", blockID)
	t.blockIDs.Store(blockID, true)
}

// hasRequestedBlockID returns true if the block id has been requested
func (t *requestTracker) hasRequestedBlockID(blockID uint64) bool {
	if requested, ok := t.blockIDs.Load(blockID); ok {
		t.logger.Trace("Checking block in network syncer...", "block", blockID, "requested", requested)
		return requested.(bool)
	}

	return false
}

// removeRequestedBlockID removes a requested block id from non-persistent state
func (t *requestTracker) removeRequestedBlockID(blockID uint64) {
	t.logger.Trace("Removing block from network syncer...", "block", blockID)
	t.blockIDs.Delete(blockID)
}
