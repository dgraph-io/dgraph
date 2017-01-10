package worker

import (
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

// rebuildIndex is called by node.Run to rebuild index.
func (n *node) rebuildIndex(ctx context.Context, proposalData []byte) error {
	x.AssertTrue(proposalData[0] == proposalIndex)
	var proposal task.Proposal
	x.Check(proposal.Unmarshal(proposalData[1:]))
	x.AssertTrue(proposal.RebuildIndex != nil)

	gid := n.gid
	x.AssertTrue(gid == proposal.RebuildIndex.GroupId)

	x.Printf("Processing proposal to rebuild index: %v", proposal.RebuildIndex)

	// Get index of last committed.
	lastIndex, err := n.store.LastIndex()
	if err != nil {
		return err
	}

	// Wait for syncing to data store.
	for n.applied.WaitingFor() {
		// watermark: add function waitingfor that returns an index that it is waiting for, and if it is zero, we can skip the wait
		doneUntil := n.applied.DoneUntil() // applied until.
		x.Printf("RebuildIndex waiting, appliedUntil:%d lastIndex: %d", doneUntil, lastIndex)
		if doneUntil >= lastIndex {
			break // Do the check before sleep.
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Aggressive evict.
	posting.CommitLists(10)

	// Wait for posting lists applying.
	w := posting.WaterMarkFor(gid)
	for w.WaitingFor() {
		doneUntil := w.DoneUntil() // synced until.
		x.Printf("RebuildIndex waiting, syncedUntil:%d lastIndex:%d", doneUntil, lastIndex)
		if doneUntil >= lastIndex {
			break // Do the check before sleep.
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Do actual index work.
	attr := proposal.RebuildIndex.Attr
	x.AssertTrue(group.BelongsTo(attr) == gid)
	if err = posting.RebuildIndex(ctx, attr); err != nil {
		return err
	}
	return nil
}

// RebuildIndex request is used to trigger rebuilding of index for the requested
// attribute. Payload is not really used.
func (w *grpcWorker) RebuildIndex(ctx context.Context, req *task.RebuildIndex) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}
	if err := processRebuildIndex(ctx, req); err != nil {
		return &Payload{}, err
	}
	return &Payload{}, nil
}

func processRebuildIndex(ctx context.Context, ri *task.RebuildIndex) error {
	gid := ri.GroupId
	n := groups().Node(gid)
	proposal := &task.Proposal{RebuildIndex: ri}
	if err := n.ProposeAndWait(ctx, proposal); err != nil {
		return err
	}
	return nil
}

func RebuildIndexOverNetwork(ctx context.Context, attr string) error {
	gid := group.BelongsTo(attr)
	x.Trace(ctx, "RebuildIndex attr: %v groupId: %v", attr, gid)

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processRebuildIndex(ctx, &task.RebuildIndex{GroupId: gid, Attr: attr})
	}

	// Send this over the network.
	addr := groups().AnyServer(gid)
	pl := pools().get(addr)

	conn, err := pl.Get()
	if err != nil {
		return x.Wrapf(err, "ProcessTaskOverNetwork: while retrieving connection.")
	}
	defer pl.Put(conn)
	x.Trace(ctx, "Sending request to %v", addr)

	c := NewWorkerClient(conn)
	_, err = c.RebuildIndex(ctx, &task.RebuildIndex{
		Attr:    attr,
		GroupId: gid,
	})
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.RebuildIndex"))
		return err
	}
	x.Trace(ctx, "Reply from server. Addr: %v Attr: %v", addr, attr)
	return nil
}
