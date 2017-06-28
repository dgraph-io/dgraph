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

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
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

// rebuildIndex is called by node.Run to rebuild index.
func (n *node) rebuildIndex(ctx context.Context, proposalData []byte) error {
	x.AssertTrue(proposalData[0] == proposalReindex)
	var proposal protos.Proposal
	x.Check(proposal.Unmarshal(proposalData[1:]))
	x.AssertTrue(proposal.RebuildIndex != nil)

	gid := n.gid
	x.AssertTrue(gid == proposal.RebuildIndex.GroupId)
	x.Trace(ctx, "Processing proposal to rebuild index: %v", proposal.RebuildIndex)

	// Get index of last committed.
	lastIndex, err := n.store.LastIndex()
	if err != nil {
		return err
	}
	if err := n.syncAllMarks(ctx, lastIndex); err != nil {
		n.props.Done(proposal.Id, err)
		return err
	}

	// Do actual index work.
	attr := proposal.RebuildIndex.Attr
	x.AssertTrue(group.BelongsTo(attr) == gid)
	if err := posting.RebuildIndex(ctx, attr); err != nil {
		n.props.Done(proposal.Id, err)
		return err
	}
	n.props.Done(proposal.Id, nil)
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
		x.Trace(ctx, "syncAllMarks waiting, appliedUntil:%d lastIndex: %d",
			doneUntil, lastIndex)
		if doneUntil >= lastIndex {
			break // Do the check before sleep.
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func waitForSyncMark(ctx context.Context, gid uint32, lastIndex uint64) {
	// Force an aggressive evict.
	posting.CommitLists(10, gid)

	// Wait for posting lists applying.
	w := posting.SyncMarkFor(gid)
	for w.WaitingFor() {
		doneUntil := w.DoneUntil() // synced until.
		x.Trace(ctx, "syncAllMarks waiting, syncedUntil:%d lastIndex:%d",
			doneUntil, lastIndex)
		if doneUntil >= lastIndex {
			break // Do the check before sleep.
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// RebuildIndex request is used to trigger rebuilding of index for the requested
// attribute. Payload is not really used.
func (w *grpcWorker) RebuildIndex(ctx context.Context, req *protos.RebuildIndexMessage) (*protos.Payload, error) {
	if ctx.Err() != nil {
		return &protos.Payload{}, ctx.Err()
	}
	if !schema.State().IsIndexed(req.Attr) {
		return &protos.Payload{}, x.Errorf("Attribute %s is not indexed", req.Attr)
	}
	if err := proposeRebuildIndex(ctx, req); err != nil {
		return &protos.Payload{}, err
	}
	return &protos.Payload{}, nil
}

func proposeRebuildIndex(ctx context.Context, ri *protos.RebuildIndexMessage) error {
	gid := ri.GroupId
	n := groups().Node(gid)
	proposal := &protos.Proposal{RebuildIndex: ri}
	if err := n.ProposeAndWait(ctx, proposal); err != nil {
		return err
	}
	return nil
}

// RebuildIndexOverNetwork rebuilds index for attr. If it serves the attr, then
// it will rebuild index. Otherwise, it will send a request to a server that
// serves the attr.
func RebuildIndexOverNetwork(ctx context.Context, attr string) error {
	gid := group.BelongsTo(attr)
	x.Trace(ctx, "RebuildIndex attr: %v groupId: %v", attr, gid)

	if groups().ServesGroup(gid) && !schema.State().IsIndexed(attr) {
		return x.Errorf("Attribute %s is not indexed", attr)
	} else if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return proposeRebuildIndex(ctx, &protos.RebuildIndexMessage{GroupId: gid, Attr: attr})
	}

	// Send this over the network.
	addr := groups().AnyServer(gid)
	pl := pools().get(addr)

	conn, err := pl.Get()
	if err != nil {
		return x.Wrapf(err, "RebuildIndexOverNetwork: while retrieving connection.")
	}
	defer pl.Put(conn)
	x.Trace(ctx, "Sending request to %v", addr)

	c := protos.NewWorkerClient(conn)
	_, err = c.RebuildIndex(ctx, &protos.RebuildIndexMessage{Attr: attr, GroupId: gid})
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.RebuildIndex"))
		return err
	}
	x.Trace(ctx, "RebuildIndex reply from server. Addr: %v Attr: %v", addr, attr)
	return nil
}
