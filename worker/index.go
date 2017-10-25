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
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

func (n *node) rebuildOrDelIndex(ctx context.Context, attr string, rebuild bool, txn *posting.Txn) error {
	rv := ctx.Value("raft").(x.RaftValue)
	x.AssertTrue(rv.Group == n.gid)

	if schema.State().IsIndexed(attr) != rebuild {
		return x.Errorf("Predicate %s index mismatch, rebuild %v", attr, rebuild)
	}
	// Remove index edges
	posting.DeleteIndex(ctx, attr)
	if rebuild {
		txn.RebuildIndex(ctx, attr)
	}
	return nil
}

func (n *node) rebuildOrDelRevEdge(ctx context.Context, attr string, rebuild bool, txn *posting.Txn) error {
	rv := ctx.Value("raft").(x.RaftValue)
	x.AssertTrue(rv.Group == n.gid)

	if schema.State().IsReversed(attr) != rebuild {
		return x.Errorf("Predicate %s reverse mismatch, rebuild %v", attr, rebuild)
	}
	posting.DeleteReverseEdges(ctx, attr)
	if rebuild {
		// Remove reverse edges
		txn.RebuildReverseEdges(ctx, attr)
	}
	return nil
}

func (n *node) rebuildOrDelCountIndex(ctx context.Context, attr string, rebuild bool, txn *posting.Txn) error {
	rv := ctx.Value("raft").(x.RaftValue)
	x.AssertTrue(rv.Group == n.gid)

	posting.DeleteCountIndex(ctx, attr)
	if rebuild {
		txn.RebuildCountIndex(ctx, attr)
	}
	return nil
}

func (n *node) syncAllMarks(ctx context.Context, lastIndex uint64) error {
	if err := n.Applied.WaitForMark(ctx, lastIndex); err != nil {
		return err
	}
	return waitForSyncMark(ctx, n.gid, lastIndex)
}

func waitForSyncMark(ctx context.Context, gid uint32, lastIndex uint64) error {
	// Wait for posting lists applying.
	w := posting.SyncMarks()
	if w.DoneUntil() >= lastIndex {
		return nil
	}

	return w.WaitForMark(ctx, lastIndex)
}
