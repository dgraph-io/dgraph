/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package zero

import (
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	otrace "go.opencensus.io/trace"
	"google.golang.org/grpc"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/google/uuid"
	"golang.org/x/net/context"
)

type node struct {
	*conn.Node
	server      *Server
	ctx         context.Context
	reads       map[uint64]chan uint64
	subscribers map[uint32]chan struct{}
	stop        chan struct{} // to send stop signal to Run
}

var errReadIndex = x.Errorf("cannot get linerized read (time expired or no configured leader)")

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

func (n *node) proposeAndWait(ctx context.Context, proposal *pb.ZeroProposal) error {
	if n.Raft() == nil {
		return x.Errorf("Raft isn't initialized yet.")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	span := otrace.FromContext(ctx)

	propose := func(timeout time.Duration) error {
		if !n.AmLeader() {
			return x.Errorf("Not Zero leader. Aborting proposal: %+v", proposal)
		}
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		che := make(chan error, 1)
		pctx := &conn.ProposalCtx{
			Ch: che,
			// Don't use the original context, because that's not what we're passing to Raft.
			Ctx: cctx,
		}
		key := n.uniqueKey()
		x.AssertTruef(n.Proposals.Store(key, pctx), "Found existing proposal with key: [%v]", key)
		defer n.Proposals.Delete(key)
		proposal.Key = key
		if span != nil {
			span.Annotatef(nil, "Proposing with key: %s. Timeout: %v", key, timeout)
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
	timeout := 4 * time.Second
	for err == errInternalRetry {
		err = propose(timeout)
		timeout *= 2 // Exponential backoff
		if timeout > time.Minute {
			timeout = 32 * time.Second
		}
	}
	return err
}

var (
	errInvalidProposal     = errors.New("Invalid group proposal")
	errTabletAlreadyServed = errors.New("Tablet is already being served")
)

func newGroup() *pb.Group {
	return &pb.Group{
		Members: make(map[uint64]*pb.Member),
		Tablets: make(map[string]*pb.Tablet),
	}
}

func (n *node) handleMemberProposal(member *pb.Member) error {
	n.server.AssertLock()
	state := n.server.state

	m := n.server.member(member.Addr)
	// Ensures that different nodes don't have same address.
	if m != nil && (m.Id != member.Id || m.GroupId != member.GroupId) {
		return x.Errorf("Found another member %d with same address: %v", m.Id, m.Addr)
	}
	if member.GroupId == 0 {
		state.Zeros[member.Id] = member
		if member.Leader {
			// Unset leader flag for other nodes, there can be only one
			// leader at a time.
			for _, m := range state.Zeros {
				if m.Id != member.Id {
					m.Leader = false
				}
			}
		}
		return nil
	}
	group := state.Groups[member.GroupId]
	if group == nil {
		group = newGroup()
		state.Groups[member.GroupId] = group
	}
	m, has := group.Members[member.Id]
	if member.AmDead {
		if has {
			delete(group.Members, member.Id)
			state.Removed = append(state.Removed, m)
			conn.Get().Remove(m.Addr)
		}
		// else already removed.
		return nil
	}
	if !has && len(group.Members) >= n.server.NumReplicas {
		// We shouldn't allow more members than the number of replicas.
		return x.Errorf("Group reached replication level. Can't add another member: %+v", member)
	}

	// Create a connection to this server.
	go conn.Get().Connect(member.Addr)

	group.Members[member.Id] = member
	// Increment nextGroup when we have enough replicas
	if member.GroupId == n.server.nextGroup &&
		len(group.Members) >= n.server.NumReplicas {
		n.server.nextGroup++
	}
	if member.Leader {
		// Unset leader flag for other nodes, there can be only one
		// leader at a time.
		for _, m := range group.Members {
			if m.Id != member.Id {
				m.Leader = false
			}
		}
	}
	// On replay of logs on restart we need to set nextGroup.
	if n.server.nextGroup <= member.GroupId {
		n.server.nextGroup = member.GroupId + 1
	}
	return nil
}

func (n *node) handleTabletProposal(tablet *pb.Tablet) error {
	n.server.AssertLock()
	state := n.server.state

	if tablet.GroupId == 0 {
		return x.Errorf("Tablet group id is zero: %+v", tablet)
	}
	group := state.Groups[tablet.GroupId]
	if tablet.Remove {
		glog.Infof("Removing tablet for attr: [%v], gid: [%v]\n", tablet.Predicate, tablet.GroupId)
		if group != nil {
			delete(group.Tablets, tablet.Predicate)
		}
		return nil
	}
	if group == nil {
		group = newGroup()
		state.Groups[tablet.GroupId] = group
	}

	// There's a edge case that we're handling.
	// Two servers ask to serve the same tablet, then we need to ensure that
	// only the first one succeeds.
	if prev := n.server.servingTablet(tablet.Predicate); prev != nil {
		if tablet.Force {
			// TODO: Try and remove this whole Force flag logic.
			originalGroup := state.Groups[prev.GroupId]
			delete(originalGroup.Tablets, tablet.Predicate)
		} else {
			if prev.GroupId != tablet.GroupId {
				glog.Infof(
					"Tablet for attr: [%s], gid: [%d] already served by group: [%d]\n",
					prev.Predicate, tablet.GroupId, prev.GroupId)
				return errTabletAlreadyServed
			}
			// This update can come from tablet size.
			tablet.ReadOnly = prev.ReadOnly
		}
	}
	group.Tablets[tablet.Predicate] = tablet
	return nil
}

func (n *node) applyProposal(e raftpb.Entry) (string, error) {
	var p pb.ZeroProposal
	// Raft commits empty entry on becoming a leader.
	if len(e.Data) == 0 {
		return p.Key, nil
	}
	if err := p.Unmarshal(e.Data); err != nil {
		return p.Key, err
	}
	if len(p.Key) == 0 {
		return p.Key, errInvalidProposal
	}
	span := otrace.FromContext(n.Proposals.Ctx(p.Key))

	n.server.Lock()
	defer n.server.Unlock()

	state := n.server.state
	state.Counter = e.Index
	if len(p.Cid) > 0 {
		if len(state.Cid) > 0 {
			return p.Key, errInvalidProposal
		}
		state.Cid = p.Cid
	}
	if p.MaxRaftId > 0 {
		if p.MaxRaftId <= state.MaxRaftId {
			return p.Key, errInvalidProposal
		}
		state.MaxRaftId = p.MaxRaftId
	}
	if p.SnapshotTs != nil {
		for gid, ts := range p.SnapshotTs {
			if group, ok := state.Groups[gid]; ok {
				group.SnapshotTs = x.Max(group.SnapshotTs, ts)
			}
		}
		purgeTs := uint64(math.MaxUint64)
		for _, group := range state.Groups {
			purgeTs = x.Min(purgeTs, group.SnapshotTs)
		}
		if purgeTs < math.MaxUint64 {
			n.server.orc.purgeBelow(purgeTs)
		}
	}
	if p.Member != nil {
		if err := n.handleMemberProposal(p.Member); err != nil {
			span.Annotatef(nil, "While applying membership proposal: %+v", err)
			glog.Errorf("While applying membership proposal: %+v", err)
			return p.Key, err
		}
	}
	if p.Tablet != nil {
		if err := n.handleTabletProposal(p.Tablet); err != nil {
			span.Annotatef(nil, "While applying tablet proposal: %+v", err)
			glog.Errorf("While applying tablet proposal: %+v", err)
			return p.Key, err
		}
	}

	if p.MaxLeaseId > state.MaxLeaseId {
		state.MaxLeaseId = p.MaxLeaseId
	} else if p.MaxTxnTs > state.MaxTxnTs {
		state.MaxTxnTs = p.MaxTxnTs
	} else if p.MaxLeaseId != 0 || p.MaxTxnTs != 0 {
		// Could happen after restart when some entries were there in WAL and did not get
		// snapshotted.
		glog.Infof("Could not apply proposal, ignoring: p.MaxLeaseId=%v, p.MaxTxnTs=%v maxLeaseId=%d"+
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
		var rc pb.RaftContext
		x.Check(rc.Unmarshal(cc.Context))
		go n.Connect(rc.Id, rc.Addr)

		m := &pb.Member{Id: rc.Id, Addr: rc.Addr, GroupId: 0}
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
	_, restart, err := n.PastLife()
	x.Check(err)

	if restart {
		glog.Infoln("Restarting node for dgraphzero")
		sp, err := n.Store.Snapshot()
		x.Checkf(err, "Unable to get existing snapshot")
		if !raft.IsEmptySnap(sp) {
			// It is important that we pick up the conf state here.
			n.SetConfState(&sp.Metadata.ConfState)

			var state pb.MembershipState
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
			return x.Errorf("Unhealthy connection to %v", opts.peer)
		}

		gconn := p.Get()
		c := pb.NewRaftClient(gconn)
		timeout := 8 * time.Second
		for {
			ctx, cancel := context.WithTimeout(n.ctx, timeout)
			defer cancel()
			// JoinCluster can block indefinitely, raft ignores conf change proposal
			// if it has pending configuration.
			_, err := c.JoinCluster(ctx, n.RaftContext)
			if err == nil {
				break
			}
			errorDesc := grpc.ErrorDesc(err)
			if errorDesc == conn.ErrDuplicateRaftId.Error() ||
				errorDesc == x.ErrReuseRemovedId.Error() {
				log.Fatalf("Error while joining cluster: %v", errorDesc)
			}
			glog.Errorf("Error while joining cluster: %v\n", err)
			timeout *= 2
			if timeout > 32*time.Second {
				timeout = 32 * time.Second
			}
			time.Sleep(timeout) // This is useful because JoinCluster can exit immediately.
		}
		glog.Infof("[%d] Starting node\n", n.Id)
		n.SetRaft(raft.StartNode(n.Cfg, nil))

	} else {
		data, err := n.RaftContext.Marshal()
		x.Check(err)
		peers := []raft.Peer{{ID: n.Id, Context: data}}
		n.SetRaft(raft.StartNode(n.Cfg, peers))

		go func() {
			// This is a new cluster. So, propose a new ID for the cluster.
			for {
				id := uuid.New().String()
				err := n.proposeAndWait(context.Background(), &pb.ZeroProposal{Cid: id})
				if err == nil {
					glog.Infof("CID set for cluster: %v", id)
					return
				}
				if err == errInvalidProposal {
					return
				}
				glog.Errorf("While proposing CID: %v. Retrying...", err)
				time.Sleep(3 * time.Second)
			}
		}()
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

	err = n.Store.CreateSnapshot(idx, n.ConfState(), data)
	x.Checkf(err, "While creating snapshot")
	glog.Infof("Writing snapshot at index: %d, applied mark: %d\n", idx, n.Applied.DoneUntil())
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
				var state pb.MembershipState
				x.Check(state.Unmarshal(rd.Snapshot.Data))
				n.server.SetMembershipState(&state)
			}

			for _, entry := range rd.CommittedEntries {
				n.Applied.Begin(entry.Index)
				if entry.Type == raftpb.EntryConfChange {
					n.applyConfChange(entry)
					glog.Infof("Done applying conf change at %d", n.Id)

				} else if entry.Type == raftpb.EntryNormal {
					key, err := n.applyProposal(entry)
					if err != nil {
						glog.Errorf("While applying proposal: %v\n", err)
					}
					n.Proposals.Done(key, err)

				} else {
					glog.Infof("Unhandled entry: %+v\n", entry)
				}
				n.Applied.Done(entry.Index)
			}

			if rd.SoftState != nil {
				if rd.RaftState == raft.StateLeader && !leader {
					glog.Infoln("I've become the leader, updating leases.")
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
			n.Raft().Advance()
		}
	}
}
