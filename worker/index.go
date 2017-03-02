package worker

import (
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/workerp"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

// rebuildIndex is called by node.Run to rebuild index.
func (n *node) rebuildIndex(ctx context.Context, proposalData []byte) error {
	x.AssertTrue(proposalData[0] == proposalReindex)
	var proposal taskp.Proposal
	x.Check(proposal.Unmarshal(proposalData[1:]))
	x.AssertTrue(proposal.RebuildIndex != nil)

	gid := n.gid
	x.AssertTrue(gid == proposal.RebuildIndex.GroupId)
	x.Trace(ctx, "Processing proposal to rebuild index: %v", proposal.RebuildIndex)

	if err := n.syncAllMarks(ctx); err != nil {
		return err
	}

	// Do actual index work.
	attr := proposal.RebuildIndex.Attr
	x.AssertTrue(group.BelongsTo(attr) == gid)
	if err := posting.RebuildIndex(ctx, attr); err != nil {
		return err
	}
	return nil
}

func (n *node) syncAllMarks(ctx context.Context) error {
	// Get index of last committed.
	lastIndex, err := n.store.LastIndex()
	if err != nil {
		return err
	}

	// Wait for syncing to data store.
	for n.applied.WaitingFor() {
		doneUntil := n.applied.DoneUntil() // applied until.
		x.Trace(ctx, "syncAllMarks waiting, appliedUntil:%d lastIndex: %d",
			doneUntil, lastIndex)
		if doneUntil >= lastIndex {
			break // Do the check before sleep.
		}
		time.Sleep(100 * time.Millisecond)
	}

	syncMarksToIndex(ctx, n.gid, lastIndex)
	return nil
}

func syncMarksToIndex(ctx context.Context, gid uint32, lastIndex uint64) {
	// Force an aggressive evict.
	posting.CommitLists(10)

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
func (w *grpcWorker) RebuildIndex(ctx context.Context, req *taskp.RebuildIndex) (*workerp.Payload, error) {
	if ctx.Err() != nil {
		return &workerp.Payload{}, ctx.Err()
	}
	if err := proposeRebuildIndex(ctx, req); err != nil {
		return &workerp.Payload{}, err
	}
	return &workerp.Payload{}, nil
}

func proposeRebuildIndex(ctx context.Context, ri *taskp.RebuildIndex) error {
	gid := ri.GroupId
	n := groups().Node(gid)
	proposal := &taskp.Proposal{RebuildIndex: ri}
	if err := n.ProposeAndWait(ctx, proposal); err != nil {
		return err
	}
	return nil
}

// RebuildIndexOverNetwork rebuilds index for attr. If it serves the attr, then
// it will rebuild index. Otherwise, it will send a request to a server that
// serves the attr.
func RebuildIndexOverNetwork(ctx context.Context, attr string) error {
	if !schema.State().IsIndexed(attr) {
		return x.Errorf("Attribute %s is indexed", attr)
	}

	gid := group.BelongsTo(attr)
	x.Trace(ctx, "RebuildIndex attr: %v groupId: %v", attr, gid)

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return proposeRebuildIndex(ctx, &taskp.RebuildIndex{GroupId: gid, Attr: attr})
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

	c := workerp.NewWorkerClient(conn)
	_, err = c.RebuildIndex(ctx, &taskp.RebuildIndex{Attr: attr, GroupId: gid})
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.RebuildIndex"))
		return err
	}
	x.Trace(ctx, "RebuildIndex reply from server. Addr: %v Attr: %v", addr, attr)
	return nil
}
