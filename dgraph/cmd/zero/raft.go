/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package zero

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"
)

type proposalCtx struct {
	ch  chan error
	ctx context.Context
}

type proposals struct {
	sync.RWMutex
	all map[string]*proposalCtx
}

func (p *proposals) Store(key string, pctx *proposalCtx) bool {
	if len(key) == 0 {
		return false
	}
	p.Lock()
	defer p.Unlock()
	if p.all == nil {
		p.all = make(map[string]*proposalCtx)
	}
	if _, has := p.all[key]; has {
		return false
	}
	p.all[key] = pctx
	return true
}

func (p *proposals) Delete(key string) {
	if len(key) == 0 {
		return
	}
	p.Lock()
	defer p.Unlock()
	delete(p.all, key)
}

func (p *proposals) Done(key string, err error) {
	if len(key) == 0 {
		return
	}
	p.Lock()
	defer p.Unlock()
	pd, has := p.all[key]
	if !has {
		// If we assert here, there would be a race condition between a context
		// timing out, and a proposal getting applied immediately after. That
		// would cause assert to fail. So, don't assert.
		return
	}
	delete(p.all, key)
	pd.ch <- err
}

type node struct {
	*conn.Node
	server      *Server
	ctx         context.Context
	props       proposals
	reads       map[uint64]chan uint64
	subscribers map[uint32]chan struct{}
	stop        chan struct{} // to send stop signal to Run
}

var errReadIndex = x.Errorf("cannot get linerized read (time expired or no configured leader)")

func (n *node) RegisterForUpdates(ch chan struct{}) uint32 {
	n.Lock()
	defer n.Unlock()
	if n.subscribers == nil {
		n.subscribers = make(map[uint32]chan struct{})
	}
	for {
		id := rand.Uint32()
		if _, has := n.subscribers[id]; has {
			continue
		}
		n.subscribers[id] = ch
		return id
	}
}

func (n *node) Deregister(id uint32) {
	n.Lock()
	defer n.Unlock()
	delete(n.subscribers, id)
}

func (n *node) triggerUpdates() {
	n.Lock()
	defer n.Unlock()
	for _, ch := range n.subscribers {
		select {
		case ch <- struct{}{}:
		// We can ignore it and don't send a notification, because they are going to
		// read a state version after now since ch is already full.
		default:
		}
	}
}

func (n *node) AmLeader() bool {
	if n.Raft() == nil {
		return false
	}
	r := n.Raft()
	return r.Status().Lead == r.Status().ID
}

func (n *node) uniqueKey() string {
	return fmt.Sprintf("z%d-%d", n.Id, n.Rand.Uint64())
}

var errInternalRetry = errors.New("Retry Raft proposal internally")

func (n *node) proposeAndWait(ctx context.Context, proposal *intern.ZeroProposal) error {
	if n.Raft() == nil {
		return x.Errorf("Raft isn't initialized yet.")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	propose := func() error {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		che := make(chan error, 1)
		pctx := &proposalCtx{
			ch: che,
			// Don't use the original context, because that's not what we're passing to Raft.
			ctx: cctx,
		}
		key := n.uniqueKey()
		x.AssertTruef(n.props.Store(key, pctx), "Found existing proposal with key: [%v]", key)
		defer n.props.Delete(key)
		proposal.Key = key

		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Proposing with key: %X", key)
		}

		data, err := proposal.Marshal()
		if err != nil {
			return err
		}

		// Propose the change.
		if err := n.Raft().Propose(cctx, data); err != nil {
			return x.Wrapf(err, "While proposing")
		}

		// Wait for proposal to be applied or timeout.
		select {
		case err := <-che:
			// We arrived here by a call to n.props.Done().
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-cctx.Done():
			return errInternalRetry
		}
	}

	// Some proposals can be stuck if leader change happens. For e.g. MsgProp message from follower
	// to leader can be dropped/end up appearing with empty Data in CommittedEntries.
	// Having a timeout here prevents the mutation being stuck forever in case they don't have a
	// timeout. We should always try with a timeout and optionally retry.
	err := errInternalRetry
	for err == errInternalRetry {
		err = propose()
	}
	return err
}

var (
	errInvalidProposal     = errors.New("Invalid group proposal")
	errTabletAlreadyServed = errors.New("Tablet is already being served")
)

func newGroup() *intern.Group {
	return &intern.Group{
		Members: make(map[uint64]*intern.Member),
		Tablets: make(map[string]*intern.Tablet),
	}
}

func (n *node) applyProposal(e raftpb.Entry) (string, error) {
	var p intern.ZeroProposal
	// Raft commits empty entry on becoming a leader.
	if len(e.Data) == 0 {
		return p.Key, nil
	}
	if err := p.Unmarshal(e.Data); err != nil {
		return p.Key, err
	}
	if p.DeprecatedId != 0 {
		p.Key = fmt.Sprint(p.DeprecatedId)
	}
	if len(p.Key) == 0 {
		return p.Key, errInvalidProposal
	}

	n.server.Lock()
	defer n.server.Unlock()

	state := n.server.state
	state.Counter = e.Index
	if p.MaxRaftId > 0 {
		if p.MaxRaftId <= state.MaxRaftId {
			return p.Key, errInvalidProposal
		}
		state.MaxRaftId = p.MaxRaftId
	}
	if p.Member != nil {
		m := n.server.member(p.Member.Addr)
		// Ensures that different nodes don't have same address.
		if m != nil && (m.Id != p.Member.Id || m.GroupId != p.Member.GroupId) {
			return p.Key, errInvalidAddress
		}
		if p.Member.GroupId == 0 {
			state.Zeros[p.Member.Id] = p.Member
			if p.Member.Leader {
				// Unset leader flag for other nodes, there can be only one
				// leader at a time.
				for _, m := range state.Zeros {
					if m.Id != p.Member.Id {
						m.Leader = false
					}
				}
			}
			return p.Key, nil
		}
		group := state.Groups[p.Member.GroupId]
		if group == nil {
			group = newGroup()
			state.Groups[p.Member.GroupId] = group
		}
		m, has := group.Members[p.Member.Id]
		if p.Member.AmDead {
			if has {
				delete(group.Members, p.Member.Id)
				state.Removed = append(state.Removed, m)
				conn.Get().Remove(m.Addr)
			}
			// else already removed.
			return p.Key, nil
		}
		if !has && len(group.Members) >= n.server.NumReplicas {
			// We shouldn't allow more members than the number of replicas.
			return p.Key, errInvalidProposal
		}

		// Create a connection to this server.
		go conn.Get().Connect(p.Member.Addr)

		group.Members[p.Member.Id] = p.Member
		// Increment nextGroup when we have enough replicas
		if p.Member.GroupId == n.server.nextGroup &&
			len(group.Members) >= n.server.NumReplicas {
			n.server.nextGroup++
		}
		if p.Member.Leader {
			// Unset leader flag for other nodes, there can be only one
			// leader at a time.
			for _, m := range group.Members {
				if m.Id != p.Member.Id {
					m.Leader = false
				}
			}
		}
		// On replay of logs on restart we need to set nextGroup.
		if n.server.nextGroup <= p.Member.GroupId {
			n.server.nextGroup = p.Member.GroupId + 1
		}
	}
	if p.Tablet != nil {
		if p.Tablet.GroupId == 0 {
			return p.Key, errInvalidProposal
		}
		group := state.Groups[p.Tablet.GroupId]
		if p.Tablet.Remove {
			x.Printf("Removing tablet for attr: [%v], gid: [%v]\n", p.Tablet.Predicate, p.Tablet.GroupId)
			if group != nil {
				delete(group.Tablets, p.Tablet.Predicate)
			}
			return p.Key, nil
		}
		if group == nil {
			group = newGroup()
			state.Groups[p.Tablet.GroupId] = group
		}

		// There's a edge case that we're handling.
		// Two servers ask to serve the same tablet, then we need to ensure that
		// only the first one succeeds.
		if tablet := n.server.servingTablet(p.Tablet.Predicate); tablet != nil {
			if p.Tablet.Force {
				originalGroup := state.Groups[tablet.GroupId]
				delete(originalGroup.Tablets, p.Tablet.Predicate)
			} else {
				if tablet.GroupId != p.Tablet.GroupId {
					x.Printf("Tablet for attr: [%s], gid: [%d] is already being served by group: [%d]\n",
						tablet.Predicate, p.Tablet.GroupId, tablet.GroupId)
					return p.Key, errTabletAlreadyServed
				}
				// This update can come from tablet size.
				p.Tablet.ReadOnly = tablet.ReadOnly
			}
		}
		group.Tablets[p.Tablet.Predicate] = p.Tablet
	}

	if p.MaxLeaseId > state.MaxLeaseId {
		state.MaxLeaseId = p.MaxLeaseId
	} else if p.MaxTxnTs > state.MaxTxnTs {
		state.MaxTxnTs = p.MaxTxnTs
	} else if p.MaxLeaseId != 0 || p.MaxTxnTs != 0 {
		// Could happen after restart when some entries were there in WAL and did not get
		// snapshotted.
		x.Printf("Could not apply proposal, ignoring: p.MaxLeaseId=%v, p.MaxTxnTs=%v maxLeaseId=%d"+
			" maxTxnTs=%d\n", p.MaxLeaseId, p.MaxTxnTs, state.MaxLeaseId, state.MaxTxnTs)
	}
	if p.Txn != nil {
		n.server.orc.updateCommitStatus(e.Index, p.Txn)
	}

	return p.Key, nil
}

func (n *node) applyConfChange(e raftpb.Entry) {
	var cc raftpb.ConfChange
	cc.Unmarshal(e.Data)

	if cc.Type == raftpb.ConfChangeRemoveNode {
		n.DeletePeer(cc.NodeID)
		n.server.removeZero(cc.NodeID)

	} else if len(cc.Context) > 0 {
		var rc intern.RaftContext
		x.Check(rc.Unmarshal(cc.Context))
		go n.Connect(rc.Id, rc.Addr)

		m := &intern.Member{Id: rc.Id, Addr: rc.Addr, GroupId: 0}
		for _, member := range n.server.membershipState().Removed {
			// It is not recommended to reuse RAFT ids.
			if member.GroupId == 0 && m.Id == member.Id {
				x.Errorf("Reusing removed id: %d. Canceling config change.\n", m.Id)
				n.DoneConfChange(cc.ID, x.ErrReuseRemovedId)
				// Cancel configuration change.
				cc.NodeID = raft.None
				n.Raft().ApplyConfChange(cc)
				return
			}
		}

		n.server.storeZero(m)
	}

	cs := n.Raft().ApplyConfChange(cc)
	n.SetConfState(cs)
	n.DoneConfChange(cc.ID, nil)

	// The following doesn't really trigger leader change. It's just capturing a leader change
	// event. The naming is poor. TODO: Fix naming, and see if we can simplify this leader change
	// logic.
	n.triggerLeaderChange()
}

func (n *node) triggerLeaderChange() {
	n.server.triggerLeaderChange()
	// We update leader information on each node without proposal. This
	// function is called on all nodes on leader change.
	n.server.updateZeroLeader()
}

func (n *node) initAndStartNode() error {
	idx, restart, err := n.PastLife()
	n.Applied.SetDoneUntil(idx)
	x.Check(err)

	if restart {
		x.Println("Restarting node for dgraphzero")
		sp, err := n.Store.Snapshot()
		x.Checkf(err, "Unable to get existing snapshot")
		if !raft.IsEmptySnap(sp) {
			var state intern.MembershipState
			x.Check(state.Unmarshal(sp.Data))
			n.server.SetMembershipState(&state)
			for _, id := range sp.Metadata.ConfState.Nodes {
				n.Connect(id, state.Zeros[id].Addr)
			}
		}

		n.SetRaft(raft.RestartNode(n.Cfg))

	} else if len(opts.peer) > 0 {
		p := conn.Get().Connect(opts.peer)
		if p == nil {
			return errInvalidAddress
		}

		gconn := p.Get()
		c := intern.NewRaftClient(gconn)
		err := errJoinCluster
		timeout := 8 * time.Second
		for i := 0; err != nil; i++ {
			ctx, cancel := context.WithTimeout(n.ctx, timeout)
			defer cancel()
			// JoinCluster can block indefinitely, raft ignores conf change proposal
			// if it has pending configuration.
			_, err = c.JoinCluster(ctx, n.RaftContext)
			if err == nil {
				break
			}
			errorDesc := grpc.ErrorDesc(err)
			if errorDesc == conn.ErrDuplicateRaftId.Error() ||
				errorDesc == x.ErrReuseRemovedId.Error() {
				log.Fatalf("Error while joining cluster: %v", errorDesc)
			}
			x.Printf("Error while joining cluster: %v\n", err)
			timeout *= 2
			if timeout > 32*time.Second {
				timeout = 32 * time.Second
			}
			time.Sleep(timeout) // This is useful because JoinCluster can exit immediately.
		}
		if err != nil {
			x.Fatalf("Max retries exceeded while trying to join cluster: %v\n", err)
		}
		x.Printf("[%d] Starting node\n", n.Id)
		n.SetRaft(raft.StartNode(n.Cfg, nil))

	} else {
		data, err := n.RaftContext.Marshal()
		x.Check(err)
		peers := []raft.Peer{{ID: n.Id, Context: data}}
		n.SetRaft(raft.StartNode(n.Cfg, peers))
	}

	go n.Run()
	go n.BatchAndSendMessages()
	return nil
}

func (n *node) updateZeroMembershipPeriodically(closer *y.Closer) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.server.updateZeroLeader()

		case <-closer.HasBeenClosed():
			closer.Done()
			return
		}
	}
}

func (n *node) snapshotPeriodically(closer *y.Closer) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.trySnapshot(1000)

		case <-closer.HasBeenClosed():
			closer.Done()
			return
		}
	}
}

func (n *node) trySnapshot(skip uint64) {
	existing, err := n.Store.Snapshot()
	x.Checkf(err, "Unable to get existing snapshot")
	si := existing.Metadata.Index
	idx := n.server.SyncedUntil()
	if idx <= si+skip {
		return
	}

	data, err := n.server.MarshalMembershipState()
	x.Check(err)

	if tr, ok := trace.FromContext(n.ctx); ok {
		tr.LazyPrintf("Taking snapshot of state at watermark: %d\n", idx)
	}
	err = n.Store.CreateSnapshot(idx, n.ConfState(), data)
	x.Checkf(err, "While creating snapshot")
	x.Printf("Writing snapshot at index: %d, applied mark: %d\n", idx, n.Applied.DoneUntil())
}

func (n *node) Run() {
	var leader bool
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	closer := y.NewCloser(4)
	// snapshot can cause select loop to block while deleting entries, so run
	// it in goroutine
	go n.snapshotPeriodically(closer)
	go n.updateZeroMembershipPeriodically(closer)

	readStateCh := make(chan raft.ReadState, 10)
	go n.RunReadIndexLoop(closer, readStateCh)
	// We only stop runReadIndexLoop after the for loop below has finished interacting with it.
	// That way we know sending to readStateCh will not deadlock.
	defer closer.SignalAndWait()

	for {
		select {
		case <-n.stop:
			n.Raft().Stop()
			return
		case <-ticker.C:
			n.Raft().Tick()
		case rd := <-n.Raft().Ready():
			for _, rs := range rd.ReadStates {
				readStateCh <- rs
			}

			n.SaveToStorage(rd.HardState, rd.Entries, rd.Snapshot)

			if !raft.IsEmptySnap(rd.Snapshot) {
				var state intern.MembershipState
				x.Check(state.Unmarshal(rd.Snapshot.Data))
				n.server.SetMembershipState(&state)
			}

			for _, entry := range rd.CommittedEntries {
				n.Applied.Begin(entry.Index)
				if entry.Type == raftpb.EntryConfChange {
					n.applyConfChange(entry)
					x.Printf("Done applying conf change at %d", n.Id)

				} else if entry.Type == raftpb.EntryNormal {
					key, err := n.applyProposal(entry)
					if err != nil && err != errTabletAlreadyServed {
						x.Printf("While applying proposal: %v\n", err)
					}
					n.props.Done(key, err)

				} else {
					x.Printf("Unhandled entry: %+v\n", entry)
				}
				n.Applied.Done(entry.Index)
			}

			// TODO: Should we move this to the top?
			if rd.SoftState != nil {
				if rd.RaftState == raft.StateLeader && !leader {
					n.server.updateLeases()
				}
				leader = rd.RaftState == raft.StateLeader
				// Oracle stream would close the stream once it steps down as leader
				// predicate move would cancel any in progress move on stepping down.
				n.triggerLeaderChange()
			}

			for _, msg := range rd.Messages {
				n.Send(msg)
			}
			// Need to send membership state to dgraph nodes on leader change also.
			if rd.SoftState != nil || len(rd.CommittedEntries) > 0 {
				n.triggerUpdates()
			}
			n.Raft().Advance()
		}
	}
}
