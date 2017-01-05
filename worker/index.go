package worker

import (
	"fmt"
	"math/rand"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

func (n *node) rebuildIndex(ctx context.Context, req *IndexPayload) error {
	gid := n.gid
	x.AssertTrue(gid == req.GroupId)
	x.Printf("Pausing")
	doResume := n.Pause() // Enter lame duck mode.
	defer doResume()

	lastIndex, err := n.store.LastIndex()
	if err != nil {
		return err
	}

	// Wait for syncing to data store.
	for n.applied.WaitingFor() {
		// watermark: add function waitingfor that returns an index that it is waiting for, and if it is zero, we can skip the wait
		doneUntil := n.applied.DoneUntil() // applied until.
		x.Printf("Waiting for aplied until to reach last index: %d %d", doneUntil, lastIndex)
		if doneUntil >= lastIndex {
			// Do the check before sleep.
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Aggressive evict.
	x.Printf("Aggressive evict")
	posting.CommitLists(10)

	// Wait for posting lists applying.
	w := posting.WaterMarkFor(gid)
	for w.WaitingFor() {
		doneUntil := w.DoneUntil() // synced until.
		x.Printf("Waiting for synced until to reach last index: %d %d", doneUntil, lastIndex)
		if doneUntil >= lastIndex {
			break // Do the check before sleep.
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Do actual index work.
	for _, attr := range req.Attr {
		x.AssertTrue(group.BelongsTo(attr) == gid)
		if err = posting.RebuildIndex(ctx, attr); err != nil {
			return err
		}
	}
	return nil
}

func handleIndexForGroup(ctx context.Context, req *IndexPayload) *IndexPayload {
	gid := req.GroupId
	n := groups().Node(gid)
	if n.AmLeader() {
		x.Trace(ctx, "Leader of group: %d. Running rebuild index.", gid)
		if err := n.rebuildIndex(ctx, req); err != nil {
			x.TraceError(ctx, err)
			return &IndexPayload{
				ReqId:  req.ReqId,
				Status: IndexPayload_FAILED,
			}
		}
		x.Trace(ctx, "RebuildIndex done for group: %d.", gid)
		return &IndexPayload{
			ReqId:   req.ReqId,
			Status:  IndexPayload_SUCCESS,
			GroupId: gid,
		}
	}

	// I'm not the leader. Relay to someone who I think is.
	var addrs []string
	{
		// Try in order: leader of given group, any server from given group, leader of group zero.
		_, addr := groups().Leader(gid)
		addrs = append(addrs, addr)
		addrs = append(addrs, groups().AnyServer(gid))
		_, addr = groups().Leader(0)
		addrs = append(addrs, addr)
	}

	var conn *grpc.ClientConn
	for _, addr := range addrs {
		pl := pools().get(addr)
		var err error
		conn, err = pl.Get()
		if err == nil {
			x.Trace(ctx, "Relaying rebuild index request for group %d to %q", gid, pl.Addr)
			defer pl.Put(conn)
			break
		}
		x.TraceError(ctx, err)
	}

	// Unable to find any connection to any of these servers. This should be
	// exceedingly rare. But probably not worthy of crashing the server. We can just
	// skip the rebuilding of index.
	if conn == nil {
		x.Trace(ctx, fmt.Sprintf("Unable to find a server to rebuild index for group: %d", gid))
		return &IndexPayload{
			ReqId:   req.ReqId,
			Status:  IndexPayload_FAILED,
			GroupId: gid,
		}
	}

	c := NewWorkerClient(conn)
	nr := &IndexPayload{
		ReqId:   req.ReqId,
		GroupId: gid,
	}
	nrep, err := c.RebuildIndex(ctx, nr)
	if err != nil {
		x.TraceError(ctx, err)
		return &IndexPayload{
			ReqId:   req.ReqId,
			Status:  IndexPayload_FAILED,
			GroupId: gid,
		}
	}
	return nrep
}

// RebuildIndex request is used to trigger rebuilding of index for the requested
// list of attributes. It is assumed that the attributes all belong to the same
// group. If a server receives request to index a group that it doesn't handle, it
// would automatically relay that request to the server that it thinks should
// handle the request.
func (w *grpcWorker) RebuildIndex(ctx context.Context, req *IndexPayload) (*IndexPayload, error) {
	reply := &IndexPayload{ReqId: req.ReqId}
	reply.Status = IndexPayload_FAILED // Set by default.

	if ctx.Err() != nil {
		return reply, ctx.Err()
	}
	if !w.addIfNotPresent(req.ReqId) {
		reply.Status = IndexPayload_DUPLICATE
		return reply, nil
	}

	chb := make(chan *IndexPayload, 1)
	go func() {
		chb <- handleIndexForGroup(ctx, req)
	}()

	select {
	case rep := <-chb:
		return rep, nil
	case <-ctx.Done():
		return reply, ctx.Err()
	}
}

func RebuildIndexOverNetwork(ctx context.Context, attrs []string) error {
	// If we haven't even had a single membership update, don't run rebuild index.
	if len(*peerAddr) > 0 && groups().LastUpdate() == 0 {
		x.Trace(ctx, "This server hasn't yet been fully initiated. Please retry later.")
		return x.Errorf("Uninitiated server. Please retry later")
	}
	// Let's first collect all groups.
	gids := groups().KnownGroups()

	// Map groupID to list of attributes that we want to re-index.
	gidMap := make(map[uint32][]string)
	for _, gid := range gids {
		gidMap[gid] = []string{} // Distinguish from nil.
	}

	for _, attr := range attrs {
		gid := group.BelongsTo(attr)
		x.AssertTruef(gidMap[gid] != nil, "Attr %s in unknown group %d", attr, gid)
		gidMap[gid] = append(gidMap[gid], attr)
	}

	ch := make(chan *IndexPayload, len(gidMap))
	var numJobs int
	for gid, a := range gidMap {
		if len(a) == 0 {
			continue
		}
		numJobs++
		go func(gid uint32, a []string) {
			ch <- handleIndexForGroup(ctx, &IndexPayload{
				ReqId:   uint64(rand.Int63()),
				GroupId: gid,
				Attr:    a,
			})
		}(gid, a)
	}

	for i := 0; i < numJobs; i++ {
		r := <-ch
		if r.Status != IndexPayload_SUCCESS {
			x.Trace(ctx, "RebuildIndex status: %v for group id: %d", r.Status, r.GroupId)
			return fmt.Errorf("RebuildIndex status: %v for group id: %d", r.Status, r.GroupId)
		} else {
			x.Trace(ctx, "RebuildIndex successful for group: %v", r.GroupId)
		}
	}
	x.Trace(ctx, "DONE rebuild index")
	return nil
}
