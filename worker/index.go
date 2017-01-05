package worker

import (
	"fmt"
	"math/rand"
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

func (n *node) rebuildIndex(proposalData []byte) error {
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
	for _, attr := range proposal.RebuildIndex.Attrs {
		x.AssertTrue(group.BelongsTo(attr) == gid)
		if err = posting.RebuildIndex(attr); err != nil {
			return err
		}
	}
	return nil
}

// RebuildIndex request is used to trigger rebuilding of index for the requested
// list of attributes. It is assumed that the attributes all belong to the same
// group.
func (w *grpcWorker) RebuildIndex(ctx context.Context, req *IndexPayload) (*IndexPayload, error) {
	reply := &IndexPayload{
		ReqId:  req.ReqId,
		Status: IndexPayload_FAILED, // Default.
	}

	if ctx.Err() != nil {
		return reply, ctx.Err()
	}
	if !w.addIfNotPresent(req.ReqId) {
		// We don't really expect duplicates actually. But if we change the
		// propagation strategy, this might happen. Maybe keep this logic?
		reply.Status = IndexPayload_DUPLICATE
		return reply, nil
	}

	ch := make(chan *IndexPayload, 1)
	go func() {
		ch <- handleIndexForGroup(ctx, req)
	}()

	select {
	case rep := <-ch:
		return rep, nil
	case <-ctx.Done():
		return reply, ctx.Err()
	}
}

func (n *node) proposeRebuildIndex(ctx context.Context, req *IndexPayload) *IndexPayload {
	proposal := &task.Proposal{RebuildIndex: req.RebuildIndex}
	err := n.ProposeAndWait(ctx, proposal)
	if err != nil {
		return &IndexPayload{
			ReqId:        req.ReqId,
			Status:       IndexPayload_FAILED,
			RebuildIndex: req.RebuildIndex,
		}
	}
	return &IndexPayload{
		ReqId:        req.ReqId,
		Status:       IndexPayload_SUCCESS,
		RebuildIndex: req.RebuildIndex,
	}
}

func handleIndexForGroup(ctx context.Context, req *IndexPayload) *IndexPayload {
	gid := req.RebuildIndex.GroupId
	n := groups().Node(gid)

	if !req.DoBroadcast {
		return n.proposeRebuildIndex(ctx, req)
	}

	// CAUTION
	// Parallelize across groups, and members inside group seems bad. We will
	// be trying to open too many connections. Perhaps batch the groups to be
	// processed for each addr.

	addrs := groups().Servers(gid)
	chp := make(chan *IndexPayload, len(addrs)+1)

	// Send jobs to other machines in the group.
	for _, addr := range addrs {
		go func(addr string) {
			pl := pools().get(addr)
			conn, err := pl.Get()
			if err != nil {
				x.TraceError(ctx, err)
				chp <- &IndexPayload{
					ReqId:        req.ReqId,
					Status:       IndexPayload_FAILED,
					RebuildIndex: req.RebuildIndex,
				}
				return
			}
			if conn == nil {
				x.TraceError(ctx, x.Errorf("Failed to get connection to %s for gid %d", addr, gid))
				chp <- &IndexPayload{
					ReqId:        req.ReqId,
					Status:       IndexPayload_FAILED,
					RebuildIndex: req.RebuildIndex,
				}
				return
			}
			defer pl.Put(conn)

			x.Trace(ctx, "Relaying rebuild index request for group %d to %s", gid, pl.Addr)
			c := NewWorkerClient(conn)
			req := &IndexPayload{
				ReqId:        req.ReqId,
				RebuildIndex: req.RebuildIndex,
				DoBroadcast:  false,
			}
			resp, err := c.RebuildIndex(ctx, req)
			if err != nil {
				x.TraceError(ctx, err)
			}
			chp <- resp
		}(addr)
	}

	// Do indexing on our own server.
	chp <- n.proposeRebuildIndex(ctx, req)

	// Collect results.
	for i := 0; i < len(addrs)+1; i++ {
		p := <-chp
		if p.GetStatus() == IndexPayload_FAILED {
			return p
		}
	}
	return &IndexPayload{
		ReqId:        req.ReqId,
		Status:       IndexPayload_SUCCESS,
		RebuildIndex: req.RebuildIndex,
	}
}

func RebuildIndexOverNetwork(ctx context.Context, attrs []string) error {
	// If we haven't even had a single membership update, don't run rebuild index.
	if len(*peerAddr) > 0 && groups().LastUpdate() == 0 {
		x.Trace(ctx, "This server hasn't yet been fully initiated. Please retry later.")
		return x.Errorf("Uninitiated server. Please retry later")
	}

	gids := groups().KnownGroups()

	// Map gid to list of attributes.
	gidMap := make(map[uint32][]string)
	for _, gid := range gids {
		gidMap[gid] = []string{} // Distinguish from nil.
	}
	for _, attr := range attrs {
		gid := group.BelongsTo(attr)
		x.AssertTruef(gidMap[gid] != nil, "Attr %s in unknown group %d", attr, gid)
		gidMap[gid] = append(gidMap[gid], attr)
	}

	// Parallelize across different groups.
	ch := make(chan *IndexPayload, len(gidMap))
	var numJobs int
	for gid, a := range gidMap {
		if len(a) == 0 {
			continue
		}
		numJobs++
		go func(gid uint32, a []string) {
			ri := &task.RebuildIndex{
				GroupId: gid,
				Attrs:   a,
			}
			ch <- handleIndexForGroup(ctx, &IndexPayload{
				ReqId:        uint64(rand.Int63()),
				RebuildIndex: ri,
				DoBroadcast:  true,
			})
		}(gid, a)
	}

	// Collect results from groups.
	for i := 0; i < numJobs; i++ {
		r := <-ch
		if r.Status != IndexPayload_SUCCESS {
			x.Trace(ctx, "RebuildIndex status: %v for group id: %d", r.Status, r.RebuildIndex.GroupId)
			return fmt.Errorf("RebuildIndex status: %v for group id: %d", r.Status, r.RebuildIndex.GroupId)
		} else {
			x.Trace(ctx, "RebuildIndex successful for group: %v", r.RebuildIndex.GroupId)
		}
	}
	x.Trace(ctx, "DONE rebuild index")
	return nil
}
