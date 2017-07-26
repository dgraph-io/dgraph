/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

import (
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

func (n *node) rebuildOrDelIndex(ctx context.Context, attr string, rebuild bool) error {
	rv := ctx.Value("raft").(x.RaftValue)
	x.AssertTrue(rv.Group == n.gid)

	// Current raft index has pending applied watermark
	// Raft index starts from 1
	if err := n.syncAllMarks(ctx, rv.Index-1); err != nil {
		return err
	}

	x.AssertTruef(schema.State().IsIndexed(attr) == rebuild, "Attr %s index mismatch", attr)
	// Remove index edges
	// For delete we since mutations would have been applied, we needn't
	// wait for synced watermarks if we delete through mutations, but
	// it would use by lhmap
	posting.DeleteIndex(ctx, attr)
	if rebuild {
		if err := posting.RebuildIndex(ctx, attr); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) rebuildOrDelRevEdge(ctx context.Context, attr string, rebuild bool) error {
	rv := ctx.Value("raft").(x.RaftValue)
	x.AssertTrue(rv.Group == n.gid)

	// Current raft index has pending applied watermark
	// Raft index starts from 1
	if err := n.syncAllMarks(ctx, rv.Index-1); err != nil {
		return err
	}

	x.AssertTruef(schema.State().IsReversed(attr) == rebuild, "Attr %s reverse mismatch", attr)
	posting.DeleteReverseEdges(ctx, attr)
	if rebuild {
		// Remove reverse edges
		if err := posting.RebuildReverseEdges(ctx, attr); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) rebuildOrDelCountIndex(ctx context.Context, attr string, rebuild bool) error {
	rv := ctx.Value("raft").(x.RaftValue)
	x.AssertTrue(rv.Group == n.gid)

	// Current raft index has pending applied watermark
	// Raft index starts from 1
	if err := n.syncAllMarks(ctx, rv.Index-1); err != nil {
		return err
	}
	posting.DeleteCountIndex(ctx, attr)
	if rebuild {
		if err := posting.RebuildCountIndex(ctx, attr); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) syncAllMarks(ctx context.Context, lastIndex uint64) error {
	n.waitForAppliedMark(ctx, lastIndex)
	waitForSyncMark(ctx, n.gid, lastIndex)
	return nil
}

func (n *node) waitForAppliedMark(ctx context.Context, lastIndex uint64) error {
	// Wait for applied to reach till lastIndex
	for n.applied.WaitingFor() {
		doneUntil := n.applied.DoneUntil() // applied until.
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("syncAllMarks waiting, appliedUntil:%d lastIndex: %d",
				doneUntil, lastIndex)
		}
		if doneUntil >= lastIndex {
			break // Do the check before sleep.
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func (n *node) waitForSyncMark(ctx context.Context, lastIndex uint64) {
	waitForSyncMark(ctx, n.gid, lastIndex)
}

func waitForSyncMark(ctx context.Context, gid uint32, lastIndex uint64) {
	// Force an aggressive evict.
	posting.CommitLists(10, gid)

	// Wait for posting lists applying.
	w := posting.SyncMarkFor(gid)
	for w.WaitingFor() {
		doneUntil := w.DoneUntil() // synced until.
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("syncAllMarks waiting, syncedUntil:%d lastIndex: %d",
				doneUntil, lastIndex)
		}
		if doneUntil >= lastIndex {
			break // Do the check before sleep.
		}
		time.Sleep(100 * time.Millisecond)
	}
}
